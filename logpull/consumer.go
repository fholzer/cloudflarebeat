package logpull

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	cloudflare "github.com/cloudflare/cloudflare-go"
	//"github.com/davecgh/go-spew/spew"
	"github.com/fholzer/cloudflarebeat/config"
	"github.com/fholzer/cloudflarebeat/registrar"
	"github.com/fholzer/cloudflarebeat/state"
	"github.com/fholzer/logshare"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/logp"
)

type Consumer struct {
	cgConfig     *config.ConsumerGroupConfig
	client       *logshare.Client
	zone         *cloudflare.Zone
	writer       *LogLinePublisher
	importCb     chan interface{}
	pullOffset   time.Duration
	lastImported time.Time
	maxPullSize  time.Duration
	fields       []string
	unpubFields  []string
	log          *logp.Logger
	reg          *registrar.Registrar
}

func newConsumer(cf *cloudflare.API, org *cloudflare.Organization, cgc *config.ConsumerGroupConfig, zone *string, pullOffset time.Duration) (*Consumer, error) {
	if zone == nil {
		return nil, errors.New("No zone name or id provided")
	}

	var (
		z   *cloudflare.Zone
		err error
	)

	if !idMatcher.MatchString(*zone) {
		zones, err := cf.ListZones(*zone)
		if err != nil {
			return nil, err
		}
		if len(zones) < 1 {
			return nil, fmt.Errorf("No zone found matching name '%q'", *zone)
		}

		matching := make([]cloudflare.Zone, 0)
		if org != nil {
			/*
			   for _, i := range zones {
			   }
			*/
		} else {
			matching = zones
		}
		if len(zones) > 1 {
			return nil, fmt.Errorf("More than one zone matching criteria for '%q'", *zone)
		}
		z = &matching[0]
	} else {
		*z, err = cf.ZoneDetails(*zone)
		if err != nil {
			return nil, fmt.Errorf("Failed retrieving details of zone with id '%q'", *zone)
		}
	}

	fields, unpubFields := ensureRequiredFields(cgc.Fields)
	return &Consumer{
		cgConfig:    cgc,
		zone:        z,
		pullOffset:  pullOffset,
		maxPullSize: cgc.MaxPullSize,
		log:         logp.NewLogger("Consumer"),
		fields:      fields,
		unpubFields: unpubFields,
	}, nil
}

func (lc *Consumer) Start(c beat.Client, reg *registrar.Registrar, shutdown <-chan struct{}, done *sync.WaitGroup) {
	lc.log.Infof("Start[%v]: Starting consumer for zone %v (%v)", lc.zone.Name, lc.zone.Name, lc.zone.ID)
	lc.writer = NewLogLinePublisher(c, lc.zone, lc.unpubFields)
	lc.writer.EdgeResponseTime = *lc.cgConfig.EdgeResponseTime
	lc.importCb = make(chan interface{})
	lc.reg = reg

	// check registry for old state
	for _, st := range reg.GetStates() {
		if st.ZoneId == lc.zone.ID {
			lc.log.Infof("Start[%v]: Found state in registry. Resuming from %v", lc.zone.Name, st.Offset)
			lc.lastImported = st.Offset
			break
		}
	}

	defer done.Done()

	// create logshare client
	logshare, err := logshare.New(
		*lc.cgConfig.ApiKey,
		*lc.cgConfig.ApiEmail,
		&logshare.Options{
			Dest:       lc.writer,
			ByReceived: true,
			Fields:     lc.fields,
		})
	if err != nil {
		return
	}
	lc.client = logshare

	// wait for pull offset
	if lc.pullOffset != 0 {
		lc.log.Debugf("Start[%v]: Waiting for pull_offset %v before starting initial log pull", lc.zone.Name, lc.pullOffset)
		select {
		case <-shutdown:
			lc.log.Debugf("Start[%v]: Shutting down.", lc.zone.Name)
			return
		case <-time.After(lc.pullOffset):
		}
	}

	// create ticker
	pullTicker := time.NewTicker(lc.cgConfig.PullInterval)

	// initialize state and trigger first import
	shuttingdown := false
	running := true
	go lc.catchUp(shutdown)

	for {
		select {
		case <-pullTicker.C:
			if !shuttingdown && !running {
				running = true
				go lc.catchUp(shutdown)
			}

		case <-lc.importCb:
			running = false
			if shuttingdown {
				lc.log.Debugf("Start[%v]: Shutting down.", lc.zone.Name)
				return
			}

		case <-shutdown:
			pullTicker.Stop()
			shuttingdown = true
			if !running {
				lc.log.Debugf("Start[%v]: Shutting down.", lc.zone.Name)
				return
			}
		}
	}
}

func (lc *Consumer) catchUp(shutdown <-chan struct{}) {
	defer func() {
		lc.importCb <- struct{}{}
	}()

	// check how long import takes
	pullStarted := time.Now()
	defer func() {
		pullTaken := time.Now().Sub(pullStarted)
		if pullTaken > lc.cgConfig.PullInterval {
			lc.log.Errorf("catchUp[%v]: Pull took too long: %v! Longer than effective pull_interval (%v)", lc.zone.Name, pullTaken, lc.cgConfig.PullInterval)
		} else if lc.cgConfig.PullInterval-pullTaken < lc.cgConfig.Margin {
			lc.log.Warnf("catchUp[%v]: Pull took too long: %v! Almost the same as effective pull_interval (%v)", lc.zone.Name, pullTaken, lc.cgConfig.PullInterval)
		}
	}()

	for {
		now := time.Now()
		earliest := now.Add(lc.cgConfig.OldestAllowed)

		start := lc.lastImported
		if start.Before(earliest) {
			lc.log.Debugf("catchUp[%v]: %v is before erliest possible import time %v. Using that instead. This is controlled by the oldest_allowed setting.", lc.zone.Name, start, earliest)
			start = earliest
		}

		end := now.Add(lc.cgConfig.NewestAllowed)
		if end.Sub(start) > lc.maxPullSize {
			lc.log.Debugf("catchUp[%v]: Time between start and end is bigger than %v, truncating import time range. This is controlled by the max_pull_size and newest_allowed settings.", lc.zone.Name, lc.maxPullSize)
			end = start.Add(lc.maxPullSize)
		}

		// if duration to import is smaller than margin, return
		if end.Sub(start) < lc.cgConfig.Margin {
			lc.log.Debugf("catchUp[%v]: Time between start and end is less than %v, not importing anything until next iteration. This is controlled by the margin setting.", lc.zone.Name, lc.cgConfig.Margin)
			return
		}

		lc.writer.NextState = state.NewState(lc.zone.ID, end)
		meta, err := lc.pullLogs(start, end)

		lc.writer.NextState.SetSuccess(err == nil && meta != nil && meta.StatusCode == 200)
		lc.writer.NextState.Wait()
		lc.log.Debugf("catchUp[%v]: Publishing states to the registry", lc.zone.Name)
		lc.reg.Channel <- []state.State{*lc.writer.NextState}

		if err != nil {
			err = getRootCause(err)
			if err == io.ErrUnexpectedEOF {
				// TODO: in case the response is bigger than 1G, use reveiced timestamp, which isn't available yet
				lc.log.Errorf("catchUp[%v]: ErrUnexpectedEOF", lc.zone.Name)
				if lc.maxPullSize > time.Minute {
					lc.maxPullSize /= 2
					if lc.maxPullSize < time.Minute {
						lc.maxPullSize = time.Minute
					}
					lc.log.Warnf("catchUp[%v]: Unexpected end of log stream. Halving effective max_pull_size for this consumer to %v.", lc.zone.Name, lc.maxPullSize)
				} else {
					lc.log.Errorf("catchUp[%v]: Unexpected end of log stream. Effective max_pull_size is already minimum allowed.", lc.zone.Name)
				}
			}

			lc.log.Warnf("catchUp[%v]: Waiting for %v after error occurred. This is controlled by the backoff setting.", lc.zone.Name, lc.cgConfig.Backoff)
			select {
			case <-shutdown:
				lc.log.Infof("catchUp[%v]: Shutdown signal received. Aborting.", lc.zone.Name)
				return
			case <-time.After(lc.cgConfig.Backoff):
			}

			continue
		}

		if meta != nil {
			if meta.StatusCode == 400 {
				lc.log.Panicf("catchUp[%v]: An error occurred while requesting logs from Cloudflare: HTTP status %d | %dms | %s", lc.zone.Name, meta.StatusCode, meta.Duration, meta.URL)
				panic(meta)
			} else if meta.StatusCode == 429 {
				lc.log.Warnf("catchUp[%v]: Whoops, got 429 'Too many requests' from Cloudflare. Waiting for %v until next try. This is controlled by the backoff setting.", lc.zone.Name, lc.cgConfig.Backoff)
				select {
				case <-shutdown:
					lc.log.Infof("catchUp[%v]: Shutdown signal received. Aborting.", lc.zone.Name)
					return
				case <-time.After(lc.cgConfig.Backoff):
				}
			}
		}
		// TODO: change this once >1G response is handled properly
		lc.lastImported = end
	}
}

func (lc *Consumer) pullLogs(start, end time.Time) (*logshare.Meta, error) {
	lc.log.Debugf("pullLogs[%v]: calling GetFromTimestamp(%v, %v, %v, 0)", lc.zone.Name, lc.zone.ID, start, end)
	meta, err := lc.client.GetFromTimestamp(lc.zone.ID, start.Unix(), end.Unix(), 0)
	if err != nil {
		lc.log.Errorf("pullLogs[%v]: Error while pulling logs %+v", lc.zone.Name, err)
	}

	if meta != nil {
		lc.log.Debugf("pullLogs[%v]: HTTP status %d | %dms | %s", lc.zone.Name, meta.StatusCode, meta.Duration, meta.URL)
		lc.log.Infof("pullLogs[%v]: Retrieved %d logs", lc.zone.Name, meta.Count)
	}

	return meta, err
}
