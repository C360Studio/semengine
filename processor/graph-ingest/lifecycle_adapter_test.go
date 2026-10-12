package graphingest

import (
	"context"
	"maps"
	"sync"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// suiteOwner adapts Component to the lifecycle suite (lifecycle-suite, "Observe adapter
// contract"). Unresolved lists, by a stable name each, what a started Component holds and
// what clearLifecycleHandles clears once a cleanup has succeeded: the runtime cancel, the
// two ingest cancels, the keyed ingest pool (whose lanes run until its Shutdown returns),
// the JetStream consumers, the request subscriptions, the status loop, the status
// publisher, the entity cache and the consumers bound for readiness.
//
// The guard's own state is not listed: the guard has no read-out (design D22), and every
// call on it either changes its state or returns before reading it. The suite's checks
// observe it through Start and Stop, as design D13 states it.
//
// Calls counts the consumers the Component binds and the Drain and Stop calls it makes on
// them, through the consumeStream acquisition seam, which wraps each handle the Client
// returns. Request subscriptions, the pool's Shutdown and the cache's Close have no seam
// and are not counted; their handles are listed under Unresolved until a cleanup succeeds.
type suiteOwner struct {
	c     *Component
	mu    sync.Mutex
	calls map[string]int
}

func newSuiteOwner(c *Component) *suiteOwner {
	o := &suiteOwner{c: c, calls: map[string]int{}}
	c.consumeStream = func(
		ctx context.Context,
		owner natsclient.PortConsumerContext,
		cfg natsclient.StreamConsumerConfig,
		handler func(context.Context, jetstream.Msg),
	) (jetstream.ConsumeContext, error) {
		handle, err := c.natsClient.ConsumeStreamWithConfig(ctx, owner, cfg, handler)
		if err != nil {
			return nil, err
		}
		o.count("consumer bind")
		return &countedConsumeContext{ConsumeContext: handle, count: o.count}, nil
	}
	return o
}

func (o *suiteOwner) count(op string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls[op]++
}

func (o *suiteOwner) Start(ctx context.Context) error { return o.c.Start(ctx) }
func (o *suiteOwner) Stop(ctx context.Context) error  { return o.c.Stop(ctx) }

func (o *suiteOwner) Observe() lifecycletest.Observation {
	o.mu.Lock()
	calls := maps.Clone(o.calls)
	o.mu.Unlock()

	c := o.c
	var held []string
	c.handlesMu.Lock()
	if c.cancel != nil {
		held = append(held, "runtime cancel")
	}
	if c.ingestSubmitCancel != nil {
		held = append(held, "ingest submit cancel")
	}
	if c.ingestPoolCancel != nil {
		held = append(held, "ingest pool cancel")
	}
	if c.ingestPool != nil {
		held = append(held, "ingest pool lanes")
	}
	if len(c.consumers) > 0 {
		held = append(held, "consumers")
	}
	if len(c.subscriptions) > 0 {
		held = append(held, "request subscriptions")
	}
	if c.statusDone != nil {
		held = append(held, "status loop")
	}
	if c.statusPublisher != nil {
		held = append(held, "status publisher")
	}
	if c.entityCache != nil {
		held = append(held, "entity cache")
	}
	c.handlesMu.Unlock()
	if len(c.boundConsumerSnapshot()) > 0 {
		held = append(held, "readiness bound consumers")
	}
	return lifecycletest.Observation{Unresolved: held, Calls: calls}
}

// countedConsumeContext counts the cleanup calls the Component makes on one consumer.
type countedConsumeContext struct {
	jetstream.ConsumeContext
	count func(string)
}

func (h *countedConsumeContext) Drain() {
	h.count("consumer drain")
	h.ConsumeContext.Drain()
}

func (h *countedConsumeContext) Stop() {
	h.count("consumer stop")
	h.ConsumeContext.Stop()
}
