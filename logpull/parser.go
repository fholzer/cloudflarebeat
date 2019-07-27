package logpull

import (
	"bytes"
	"encoding/hex"
	//"fmt"
	"time"
	//"encoding/json"

	"github.com/json-iterator/go"

	cloudflare "github.com/cloudflare/cloudflare-go"
	"github.com/fholzer/cloudflarebeat/state"

	"github.com/elastic/beats/libbeat/beat"
	"github.com/elastic/beats/libbeat/logp"
)

var badIpFields = []string{
	"EdgeServerIP",
	"OriginIP",
}

var timestampsTo = []string{
	"EdgeStartTimestamp",
	"EdgeEndTimestamp",
}

type LogLinePublisher struct {
	client           beat.Client
	zone             *cloudflare.Zone
	unpubFields      []string
	latestT          time.Time
	NextState        *state.State
	log              *logp.Logger
	EdgeResponseTime bool
}

func NewLogLinePublisher(client beat.Client, zone *cloudflare.Zone, unpubFields []string) *LogLinePublisher {
	return &LogLinePublisher{
		client:      client,
		log:         logp.NewLogger("Parser"),
		zone:        zone,
		unpubFields: unpubFields,
	}
}

func (llp *LogLinePublisher) Write(p []byte) (n int, err error) {
	//logp.Warn("Parsing event...")
	var log map[string]interface{}
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(p, &log); err != nil {
		llp.log.Errorf("Failed to parse log: %+v", err)
		llp.log.Warnf("Log data: %+v", hex.Dump(p))
		return len(p), nil
	}

	// get timestamp
	var ts time.Time
	if startTs, ok := log["EdgeStartTimestamp"]; ok {
		nanots := int64(startTs.(float64))
		ts = time.Unix(nanots/1000000000, nanots%1000000000)
	} else {
		llp.log.Panicf("Log doesn't contain EdgeStartTimestamp: %v", log)
	}

	// set zone name
	log["Zone"] = llp.zone.Name

	// generate _id
	var buffer bytes.Buffer
	buffer.WriteString(llp.zone.ID)
	buffer.WriteString(log["RayID"].(string))

	if llp.unpubFields != nil {
		for i := range llp.unpubFields {
			delete(log, llp.unpubFields[i])
		}
	}

	// some fields may be empty, remove those
	for key := range log {
		if log[key] == nil {
			delete(log, key)
			continue
		}
		if _, ok := log[key].(string); ok {
			if len(log[key].(string)) < 1 {
				delete(log, key)
			}
		}
	}

	// workaround for Cloudflare logpull API bug that causes EdgeServerIP
	// and/or OriginIP fields to be string "<nil>"
	for _, key := range badIpFields {
		if _, ok := log[key].(string); ok {
			if log[key] == "<nil>" {
				delete(log, key)
			}
		}
	}

	// need to convert timestamps from nanoseconds to milliseconds because
	// elastic doesn't understand nanosecond timestamps
	for _, key := range timestampsTo {
		if ts, ok := log[key].(float64); ok {
			log[key] = int64(ts / 1000000)
		}
	}

	if llp.EdgeResponseTime {
		start, hasStart := log["EdgeStartTimestamp"].(int64)
		end, hasEnd := log["EdgeEndTimestamp"].(int64)
		if hasStart && hasEnd {
			log["EdgeResponseTime"] = end - start
		}
	}

	event := beat.Event{
		Timestamp: ts,
		Fields:    log,
		Private:   llp.NextState,
	}
	event.SetID(buffer.String())
	llp.NextState.Add(1)
	llp.client.Publish(event)
	llp.latestT = ts
	return len(p), nil
}

func (llp *LogLinePublisher) GetLatestTime() time.Time {
	return llp.latestT
}
