package natsclient

import (
	"context"
	"log/slog"
	"sync"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/pkg/errs"
)

// ownedDelivery tracks the handler invocations of one subscription or consumer the client created,
// so that Close knows when they have all returned (design D3, natsclient-close-is-final). nats.go
// v1.54.0 cannot tell it: a forced close does not wait for a running callback (nats.go:6196-6227),
// a drain gives up after its timeout with a callback running (:6349-6372, :6389-6390), and a
// consumer's Closed() closes at once for an invalid subscription (jetstream/pull.go:822-837).
//
// The native library delivers on one goroutine per subscription, so at most one invocation runs
// at a time. Once end has been recorded no invocation starts: a delivery handed over after that is
// refused (refuse).
type ownedDelivery struct {
	m     *Client
	attrs []any // what the refusal log names besides the subject

	mu      sync.Mutex
	running int
	ended   bool

	// done closes once the end is recorded and no invocation runs.
	done chan struct{}
}

func newOwnedDelivery(m *Client, attrs ...any) *ownedDelivery {
	return &ownedDelivery{m: m, attrs: attrs, done: make(chan struct{})}
}

// run runs fn as one handler invocation, unless the end has already been recorded, in which case
// the delivery is refused and fn never runs.
func (d *ownedDelivery) run(subject func() string, fn func()) {
	if !d.enter() {
		d.refuse(subject())
		return
	}
	defer d.exit()
	fn()
}

func (d *ownedDelivery) enter() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.ended {
		return false
	}
	d.running++
	d.m.handlersRunning.Add(1)
	return true
}

func (d *ownedDelivery) exit() {
	d.m.handlersRunning.Add(-1)
	d.mu.Lock()
	d.running--
	last := d.ended && d.running == 0
	d.mu.Unlock()
	if last {
		d.complete()
	}
}

// end records that native delivery has ended, or claims to have: no invocation starts after it.
// It may be called more than once.
func (d *ownedDelivery) end() {
	d.mu.Lock()
	first := !d.ended
	d.ended = true
	idle := first && d.running == 0
	d.mu.Unlock()
	if idle {
		d.complete()
	}
}

// complete runs exactly once: from end when nothing runs, or from the exit of the invocation that
// was running when end was recorded.
func (d *ownedDelivery) complete() {
	close(d.done)
}

// refuse declares a delivery dropped because its subscription's end was already recorded
// (design D3, natsclient-close-is-final). It is safe to drop: with an explicit or all ack policy the
// message stays unacknowledged and the server redelivers it after its ack wait. With AckNone, and
// on a core subscription, it is lost, as the buffered messages the native library discards at the
// same moment are (nats.go:3801-3803, :3817-3828; jetstream/pull.go:58-61). The owner accepted that AckNone
// loss (question 4, #9 comment 5980769459). The late_delivery_refused label of the JetStream
// error metric also counts core subscriptions.
func (d *ownedDelivery) refuse(subject string) {
	attrs := append([]any{slog.String("subject", subject)}, d.attrs...)
	d.m.logger.Warn("NATS client refused a delivery after its subscription ended", attrs...)
	if d.m.jsMetrics != nil {
		d.m.jsMetrics.recordError("late_delivery_refused")
	}
}

// consumerOwnership is the client's hold on one consumer from just before native Consume is called
// until every handler invocation has returned. Its goroutine is admitted as client-owned work before
// native Consume, so Close joins it whatever happens next.
type consumerOwnership struct {
	delivery *ownedDelivery
	handle   chan jetstream.ConsumeContext // nil: native Consume failed
	released chan struct{}
}

// ownConsumer admits the consumer's ownership goroutine, which waits for the native handle's
// Closed(), records the end, waits for the running invocation, and then calls release. It reports
// false, and admits nothing, once Close has begun.
func (c *Client) ownConsumer(d *ownedDelivery, release func(started bool)) (*consumerOwnership, bool) {
	o := &consumerOwnership{delivery: d, handle: make(chan jetstream.ConsumeContext, 1), released: make(chan struct{})}
	if !c.startBackground(workClaimRelease, func() {
		defer close(o.released)
		h := <-o.handle
		if h != nil {
			// Closed() alone is not the end of the handlers (jetstream/pull.go:822-837): it can
			// close while one still runs. d.done is.
			<-h.Closed()
		}
		d.end()
		<-d.done
		release(h != nil)
	}) {
		return nil, false
	}
	return o, true
}

// failed hands over a native Consume that returned an error and returns once the claim has been
// released, so a caller that retries does not meet its own claim.
func (o *consumerOwnership) failed() {
	o.handle <- nil
	<-o.released
}

// commitConsumer hands the started native handle to the ownership goroutine. If Close has begun
// meanwhile, delivery is stopped and the setup returns nats.ErrConnectionClosed with no handle once
// every handler invocation has returned, or ctx's error first if ctx ends; either way the claim is
// held until the handlers have returned and Close joins them.
func (c *Client) commitConsumer(
	ctx context.Context, operation string, o *consumerOwnership, h jetstream.ConsumeContext,
) (jetstream.ConsumeContext, error) {
	o.handle <- h
	if !c.isClosing() {
		c.resetCircuit()
		return h, nil
	}
	h.Stop()
	select {
	case <-o.released:
		return nil, errs.Wrap(nats.ErrConnectionClosed, "Client", operation, "client closed while starting consumer")
	case <-ctx.Done():
		return nil, errs.WrapTransient(ctx.Err(), "Client", operation,
			"setup context ended while the refused consumer's handlers run")
	}
}

// consumerAttrs names a consumer in the refusal log.
func consumerAttrs(stream, consumer string, ack jetstream.AckPolicy) []any {
	return []any{slog.String("stream", stream), slog.String("consumer", consumer), slog.String("ack_policy", ack.String())}
}
