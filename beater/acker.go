package beater

import (
	"github.com/elastic/beats/libbeat/logp"
	"github.com/fholzer/cloudflarebeat/state"
)

// eventAcker handles publisher pipeline ACKs and forwards
// them to the registrar.
type eventACKer struct {
	out successLogger
	log *logp.Logger
}

type successLogger interface {
	Published(states []state.State)
}

func newEventACKer(out successLogger) *eventACKer {
	return &eventACKer{
		out: out,
		log: logp.NewLogger("EventACKer"),
	}
}

func (a *eventACKer) ackEvents(data []interface{}) {
	states := make([]state.State, 0, len(data))
	for _, datum := range data {
		if datum == nil {
			continue
		}

		//logp.Info("ackEvents: datum=%+v", datum)
		st, ok := datum.(*state.State)
		if !ok {
			a.log.Panicf("Got unknown type of state: %+v", datum)
			continue
		}

		st.Dec()
	}

	if len(states) > 0 {
		a.log.Infof("Publishing states to the registry")
		a.out.Published(states)
	}
}
