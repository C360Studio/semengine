package websocket

import (
	"context"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
)

// suiteOwner is the websocket output as the lifecycle suite's owner. Observe lists what the
// component holds.
type suiteOwner struct{ c *output }

func (o suiteOwner) Start(ctx context.Context) error { return o.c.Start(ctx) }
func (o suiteOwner) Stop(ctx context.Context) error  { return o.c.Stop(ctx) }

// Observe reads the component's handles without the component's lock for them: the suite observes
// after a Start or Stop has returned to it. Clients and admitted requests are read under their own
// locks.
func (o suiteOwner) Observe() lifecycletest.Observation {
	var held []string
	if o.c.listen != nil {
		held = append(held, "listener")
	}
	if o.c.sub != nil {
		held = append(held, "subscription")
	}
	if o.c.served != nil {
		held = append(held, "server")
	}
	o.c.clientsMu.Lock()
	clients := len(o.c.clients)
	o.c.clientsMu.Unlock()
	if clients > 0 {
		held = append(held, "client")
	}
	if o.c.requests.Pending() > 0 {
		held = append(held, "admitted request")
	}
	return lifecycletest.Observation{Unresolved: held}
}
