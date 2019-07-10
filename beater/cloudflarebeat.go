package beater

import (
	"fmt"
	"sync"
	"time"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/monitoring"

	"github.com/fholzer/cloudflarebeat/config"
	"github.com/fholzer/cloudflarebeat/logpull"
	"github.com/fholzer/cloudflarebeat/registrar"
)

type Cloudflarebeat struct {
	done           chan struct{}
	config         *config.Config
	consumerGroups []logpull.ConsumerGroup
	client         beat.Client
	log            *logp.Logger
}

// Creates beater
func New(b *beat.Beat, rawConfig *common.Config) (beat.Beater, error) {
	c, err := config.Read(b, rawConfig)
	if err != nil {
		return nil, err
	}

	consumerGroups := make([]logpull.ConsumerGroup, len(c.ConsumerGroups))
	for i, cgc := range c.ConsumerGroups {
		cg, err := logpull.New(&cgc)
		if err != nil {
			return nil, err
		}
		consumerGroups[i] = *cg
	}
	cb := &Cloudflarebeat{
		done:           make(chan struct{}),
		config:         c,
		consumerGroups: consumerGroups,
		log:            logp.NewLogger("Cloudflarebeat"),
	}

	return cb, nil
}

func (bt *Cloudflarebeat) Run(b *beat.Beat) error {
	var err error
	config := bt.config

	waitFinished := newSignalWait()
	waitEvents := newSignalWait()

	// count active events for waiting on shutdown
	wgEvents := &eventCounter{
		count: monitoring.NewInt(nil, "cloudflarebeat.events.active"),
		added: monitoring.NewUint(nil, "cloudflarebeat.events.added"),
		done:  monitoring.NewUint(nil, "cloudflarebeat.events.done"),
	}
	finishedLogger := newFinishedLogger(wgEvents)

	// Setup registrar to persist state
	registrar, err := registrar.New(config.RegistryFile, config.RegistryFlush, finishedLogger)
	if err != nil {
		bt.log.Errorf("Could not init registrar: %v", err)
		return err
	}

	// Make sure all events that were published in
	registrarChannel := newRegistrarLogger(registrar)

	err = b.Publisher.SetACKHandler(beat.PipelineACKHandler{
		ACKEvents: newEventACKer(registrarChannel).ackEvents,
	})
	if err != nil {
		bt.log.Errorf("Failed to install the registry with the publisher pipeline: %v", err)
		return err
	}

	bt.client, err = b.Publisher.Connect()
	if err != nil {
		return err
	}

	outDone := make(chan struct{}) // outDone closes down all active pipeline connections

	// The order of starting and stopping is important. Stopping is inverted to the starting order.
	// The current order is: registrar, publisher, spooler, crawler
	// That means, crawler is stopped first.

	// Start the registrar
	err = registrar.Start()
	if err != nil {
		return fmt.Errorf("Could not start registrar: %v", err)
	}

	// Stopping registrar will write last state
	defer registrar.Stop()

	var cwg sync.WaitGroup
	// Stopping publisher (might potentially drop items)
	defer func() {
		// Closes first the registrar logger to make sure not more events arrive at the registrar
		// registrarChannel must be closed first to potentially unblock (pretty unlikely) the publisher
		registrarChannel.Close()
		close(outDone) // finally close all active connections to publisher pipeline
		cwg.Wait()
	}()

	// Wait for all events to be processed or timeout
	defer waitEvents.Wait()

	for _, cg := range bt.consumerGroups {
		cg.Start(bt.client, registrar, outDone, &cwg)
	}

	// Add done channel to wait for shutdown signal
	waitFinished.AddChan(bt.done)
	waitFinished.Add(waitGroup(&cwg))
	waitFinished.Wait()

	// Stop autodiscover -> Stop crawler -> stop prospectors -> stop harvesters
	// Note: waiting for crawlers to stop here in order to install wgEvents.Wait
	//       after all events have been enqueued for publishing. Otherwise wgEvents.Wait
	//       or publisher might panic due to concurrent updates.

	timeout := bt.config.ShutdownTimeout
	// Checks if on shutdown it should wait for all events to be published
	waitPublished := bt.config.ShutdownTimeout > 0
	if waitPublished {
		// Wait for registrar to finish writing registry
		waitEvents.Add(withLog(wgEvents.Wait,
			"Continue shutdown: All enqueued events being published."))
		// Wait for either timeout or all events having been ACKed by outputs.
		if bt.config.ShutdownTimeout > 0 {
			bt.log.Infof("Shutdown output timer started. Waiting for max %v.", timeout)
			waitEvents.Add(withLog(waitDuration(timeout),
				"Continue shutdown: Time out waiting for events being published."))
		} else {
			waitEvents.AddChan(bt.done)
		}
	}

	return nil

	select {
	case <-time.After(5 * time.Second):
	case <-bt.done:
	}
	return nil
}

func (bt *Cloudflarebeat) Stop() {
	bt.client.Close()
	close(bt.done)
}
