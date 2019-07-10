package state

import (
	"sync"
	"time"
)

type State struct {
	Finished  bool           `json:"-"`
	ZoneId    string         `json:"zone_id"`
	Timestamp time.Time      `json:"timestamp"`
	Offset    time.Time      `json:"offset"`
	TTL       time.Duration  `json:"ttl"`
	wg        sync.WaitGroup `json:",ommit"`
	done      chan struct{}  `json:",ommit"`
	success   bool           `json:",ommit"`
}

// NewState creates a new zonelog state
func NewState(zoneId string, offset time.Time) *State {
	return &State{
		Finished:  false,
		ZoneId:    zoneId,
		Timestamp: time.Now(),
		Offset:    offset,
		TTL:       -1,
	}
}

// ID returns a unique id for the state as a string
func (s *State) ID() string {
	return s.ZoneId
}

// IsEqual compares the state to an other state supporing stringer based on the unique string
func (s *State) IsEqual(c *State) bool {
	return s.ID() == c.ID()
}

// IsEmpty returns true if the state is empty
func (s *State) IsEmpty() bool {
	return *s == State{}
}

func (s *State) Add(delta int) {
	s.wg.Add(delta)
}

func (s *State) Dec() {
	s.wg.Add(-1)
}

func (s *State) SetSuccess(success bool) {
	s.success = success
}

func (s *State) Wait() {
	s.wg.Wait()
}
