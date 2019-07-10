// Config is put into a different package to prevent cyclic imports in case
// it is needed in several locations

package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
)

type Config struct {
	RegistryFile    string                `config:"registry_file"`
	RegistryFlush   time.Duration         `config:"registry_flush"`
	ShutdownTimeout time.Duration         `config:"shutdown_timeout"`
	ConsumerGroups  []ConsumerGroupConfig `config:"consumers"`
	CommonConfig    `config:",inline"`
}

const CloudflareMinStart = time.Minute * 5

var DefaultConfig = Config{
	RegistryFile:    "registry",
	ShutdownTimeout: 10,
	CommonConfig:    DefaultCommonConfig,
}

var DefaultCommonConfig = CommonConfig{
	//OldestAllowed: time.Minute * 20,
	//PullInterval:  time.Minute * 5,
	Margin:           time.Minute * 1,
	Backoff:          time.Second * 30,
	NewestAllowed:    time.Minute * 10,
	OldestAllowed:    time.Minute * 120,
	PullInterval:     time.Minute * 5,
	MaxPullSize:      time.Minute * 60,
	PullOffset:       newBool(true),
	EdgeResponseTime: newBool(true),
}

type CommonConfig struct {
	ApiEmail         *string       `config:"api_email"`
	ApiKey           *string       `config:"api_key"`
	Organization     *string       `config:"organization"`
	Fields           []string      `config:"fields"`
	MaxPullSize      time.Duration `config:"max_pull_size"`
	PullInterval     time.Duration `config:"pull_interval"`
	OldestAllowed    time.Duration `config:"oldest_allowed"`
	NewestAllowed    time.Duration `config:"newest_allowed"`
	Margin           time.Duration `config:"margin"`
	Backoff          time.Duration `config:"backoff"`
	PullOffset       *bool         `config:"pull_offset_enabled"`
	EdgeResponseTime *bool         `config:"edge_response_time"`
}

type ConsumerGroupConfig struct {
	Title        string   `config:"title"`
	Zones        []string `config:"zones"`
	CommonConfig `config:",inline"`
}

func Read(b *beat.Beat, rawConfig *common.Config) (*Config, error) {
	c := DefaultConfig
	if err := rawConfig.Unpack(&c); err != nil {
		return nil, fmt.Errorf("Error reading config file: %v", err)
	}

	if len(c.ConsumerGroups) < 1 {
		return nil, errors.New("No log pulls defined. What do you want me to pull?")
	}

	for i := range c.ConsumerGroups {
		cg := &c.ConsumerGroups[i]
		// inherit from Config
		if cg.ApiEmail == nil {
			cg.ApiEmail = c.ApiEmail
		}
		if cg.ApiKey == nil {
			cg.ApiKey = c.ApiKey
		}
		if cg.Organization == nil {
			cg.Organization = c.Organization
		}
		if cg.Fields == nil {
			cg.Fields = c.Fields
		}
		if cg.Margin == 0 {
			cg.Margin = c.Margin
		}
		if cg.Backoff == 0 {
			cg.Backoff = c.Backoff
		}
		if cg.NewestAllowed == 0 {
			cg.NewestAllowed = c.NewestAllowed
		}
		if cg.OldestAllowed == 0 {
			cg.OldestAllowed = c.OldestAllowed
		}
		if cg.PullInterval == 0 {
			cg.PullInterval = c.PullInterval
		}
		if cg.MaxPullSize == 0 {
			cg.MaxPullSize = c.MaxPullSize
		}

		if cg.Margin == 0 || cg.Backoff == 0 || cg.NewestAllowed == 0 || cg.OldestAllowed == 0 || cg.PullInterval == 0 || cg.MaxPullSize == 0 {
			return nil, errors.New("max_pull_size, pull_interval, oldest_allowed, newest_allowed, margin and backoff settings must not be 0")
		}

		if cg.NewestAllowed < CloudflareMinStart {
			return nil, fmt.Errorf("newest_allowed must be at least %v", CloudflareMinStart)
		}

		if cg.OldestAllowed < cg.NewestAllowed {
			return nil, errors.New("oldest_allowed must be larger than newest_allowed")
		}

		cg.NewestAllowed = -cg.NewestAllowed
		cg.OldestAllowed = -cg.OldestAllowed

		if cg.ApiEmail == nil || *cg.ApiEmail == "" || cg.ApiKey == nil || *cg.ApiKey == "" {
			return nil, errors.New("api_email and api_key are mandatory settings")
		}

		if len(cg.Zones) < 1 {
			if !b.InSetupCmd {
				return nil, errors.New("You defined no zones to watch. What do you want me to do?")
			}

			// in the `setup` command, log this only as a warning
			logp.NewLogger("Config").Warnf("Setup called, but you defined no zones to watch.")
		}

		if len(cg.Fields) < 1 {
			return nil, errors.New("You defined no fields to import. What do you want me to do?")
		}

		if cg.PullOffset == nil {
			cg.PullOffset = c.PullOffset
		}

		if cg.EdgeResponseTime == nil {
			cg.EdgeResponseTime = c.EdgeResponseTime
		}
	}
	c.NewestAllowed = -c.NewestAllowed
	c.OldestAllowed = -c.OldestAllowed
	return &c, nil
}

func newBool(val bool) *bool {
	b := val
	return &b
}
