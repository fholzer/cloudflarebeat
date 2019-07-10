package logpull

import (
	"fmt"
	"sync"
	"time"

	cloudflare "github.com/cloudflare/cloudflare-go"
	"github.com/fholzer/cloudflarebeat/config"
	"github.com/fholzer/cloudflarebeat/registrar"

	"github.com/elastic/beats/libbeat/beat"
)

type ConsumerGroup struct {
	zones []Consumer
}

// TODO: ensure RayID and EdgeStartTimestamp fields are specified!!
func New(cfg *config.ConsumerGroupConfig) (*ConsumerGroup, error) {
	cf, err := cloudflare.New(*cfg.ApiKey, *cfg.ApiEmail)
	if err != nil {
		return nil, err
	}

	// list organizations
	orgs, _, err := cf.ListOrganizations()
	if err != nil {
		return nil, err
	}

	zones := make([]Consumer, len(cfg.Zones))
	pullOffset := cfg.PullInterval / time.Duration(len(cfg.Zones))
	for i, zcfg := range cfg.Zones {
		var org *cloudflare.Organization
		if cfg.Organization != nil {
			org = orgId(orgs, cfg.Organization)
			if err != nil {
				return nil, fmt.Errorf("Can't find organization '%q'", *cfg.Organization)
			}
		}

		var offset time.Duration
		if cfg.PullOffset != nil && *cfg.PullOffset {
			offset = time.Duration(i) * pullOffset
		}
		z, err := newConsumer(cf, org, cfg, &zcfg, offset)
		if err != nil {
			return nil, err
		}
		zones[i] = *z
	}
	return &ConsumerGroup{zones: zones}, nil
}

func (cg *ConsumerGroup) Start(c beat.Client, reg *registrar.Registrar, shutdown chan struct{}, done *sync.WaitGroup) {
	for i := range cg.zones {
		done.Add(1)
		go cg.zones[i].Start(c, reg, shutdown, done)
	}
}
