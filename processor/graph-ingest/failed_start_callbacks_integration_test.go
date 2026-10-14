//go:build integration

package graphingest

// Checkpoint 3 B2 (PR #93): natsclient can run a callback during an acquisition that then
// fails and returns no handle, so a failed Start's rollback has nothing to drain or join for
// it. These tests run such a callback inside a real Start on a fixture broker and check that
// it does no work on what the rollback clears: a request handler refuses before Start
// completes (graph-transport-boundary, "graph-ingest does no request work before Start
// completes"), and a stream callback that runs after the rollback Naks its message
// (graph-ingest-recovery, "Settlement order", "A delivery after a failed consumer setup").

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// failedStartEntityID is under the fixture deployment's authority (testDeploymentOrg,
// testDeploymentPlatform), so every lane accepts it.
const failedStartEntityID = "c360.platform.robotics.gcs.drone.b2"

// awaitClosed waits for ch to close, failing the test when ctx ends first.
func awaitClosed(ctx context.Context, t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatalf("%s: %v", what, ctx.Err())
	}
}

// heldMetadataMsg is a streamLaneMsg whose Metadata blocks until release is closed. entered
// closes when the first call is made.
type heldMetadataMsg struct {
	*streamLaneMsg
	enterOnce sync.Once
	entered   chan struct{}
	release   chan struct{}
}

func (m *heldMetadataMsg) Metadata() (*jetstream.MsgMetadata, error) {
	m.enterOnce.Do(func() { close(m.entered) })
	<-m.release
	return m.streamLaneMsg.Metadata()
}

// TestFailedRegistrationRunsNoRequestWork: one request registration runs its handler on
// goroutine G during the registration and then fails with a transient DeadlineExceeded,
// returning no subscription (natsclient's T6 shape, subscribe_registration_test.go). G must
// refuse with a transient error matching errs.ErrNotStarted and do no work; Start fails with
// the registration's error, the rollback leaves nothing held, and a later Stop returns nil.
func TestFailedRegistrationRunsNoRequestWork(t *testing.T) {
	cases := []struct {
		name string
		// subject picks the registration that fails.
		subject func(t *testing.T, c *Component) string
		request func(t *testing.T) []byte
		// seed runs on the failing registration, before G starts.
		seed func(t *testing.T, c *Component)
		// check runs after Start has returned and G has returned.
		check func(t *testing.T, c *Component)
	}{
		{
			// A batch query reads the seeded entity and reaches repopulateHook (query.go)
			// before it touches the entity cache. A not-found ID returns before the hook.
			name: "query",
			subject: func(t *testing.T, _ *Component) string {
				for _, verb := range graph.QueryVerbs() {
					if verb.Name == "batch" {
						return verb.Subject
					}
				}
				t.Fatal("the verb table declares no batch query")
				return ""
			},
			request: func(t *testing.T) []byte {
				return mustCanonicalJSON(t, map[string][]string{"ids": {failedStartEntityID}})
			},
			seed: func(t *testing.T, c *Component) {
				seedEntityState(t, c, &graph.EntityState{
					ID:          failedStartEntityID,
					MessageType: testEntityType(),
					Triples: withTestMetadata(message.Triple{
						Subject: failedStartEntityID, Predicate: "test.state.value", Object: "seeded", Confidence: 1,
					}),
				})
			},
		},
		{
			name: "mutation",
			subject: func(t *testing.T, c *Component) string {
				routes, err := c.canonicalMutationRoutes()
				if err != nil {
					t.Fatalf("canonicalMutationRoutes: %v", err)
				}
				for _, route := range routes {
					if route.operation == graphmutation.CreateEntity {
						return route.subject
					}
				}
				t.Fatal("no create route")
				return ""
			},
			request: func(t *testing.T) []byte {
				return mustCanonicalJSON(t, graph.CreateEntityRequest{
					Entity:  canonicalMutationEntity(failedStartEntityID),
					Triples: []message.Triple{canonicalTriple(failedStartEntityID, "test.state.value", "created")},
				})
			},
			seed: func(*testing.T, *Component) {},
			check: func(t *testing.T, c *Component) {
				// A fresh component's metric vectors hold no series until a request is metered.
				if n := testutil.CollectAndCount(c.mutationOutcomes); n != 0 {
					t.Errorf("mutation outcome series = %d, want 0: the refusal was metered", n)
				}
				if n := testutil.CollectAndCount(c.mutationRejections); n != 0 {
					t.Errorf("mutation rejection series = %d, want 0: the refusal was metered", n)
				}
				if _, _, ok := storedRevision(t, c, failedStartEntityID); ok {
					t.Errorf("ENTITY_STATES holds %s: the mutation ran before Start completed", failedStartEntityID)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			comp := createTestComponentForLifecycle(t)
			if err := comp.Initialize(); err != nil {
				t.Fatalf("Initialize: %v", err)
			}
			failing := tc.subject(t, comp)

			hookEntered := make(chan struct{})
			releaseHook := make(chan struct{})
			var hookOnce sync.Once
			comp.repopulateHook = func(string) {
				hookOnce.Do(func() { close(hookEntered) })
				<-releaseHook
			}
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(releaseHook) }) }
			defer release()

			registrationErr := errs.WrapTransient(context.DeadlineExceeded,
				"test", "SubscribeForRequests", "registration round trip")
			gReturned := make(chan struct{})
			var gErr error
			var gPanic any
			comp.subscribeRequests = func(
				subCtx context.Context,
				subject string,
				handler func(context.Context, []byte) ([]byte, error),
			) (*natsclient.Subscription, error) {
				if subject != failing {
					return comp.natsClient.SubscribeForRequests(subCtx, subject, handler)
				}
				tc.seed(t, comp)
				request := tc.request(t)
				go func() {
					defer close(gReturned)
					defer func() { gPanic = recover() }()
					_, gErr = handler(subCtx, request)
				}()
				select {
				case <-gReturned:
				case <-hookEntered:
				case <-ctx.Done():
				}
				return nil, registrationErr
			}

			startErr := comp.Start(ctx)
			if !errors.Is(startErr, registrationErr) {
				t.Fatalf("Start error = %v, want it to wrap the registration's error", startErr)
			}
			select {
			case <-hookEntered:
				t.Error("the query read the entity before Start completed: repopulateHook was entered")
			default:
			}
			if held := (&suiteOwner{c: comp}).Observe().Unresolved; len(held) != 0 {
				t.Errorf("held after the failed Start = %v, want nothing", held)
			}

			release()
			awaitClosed(ctx, t, gReturned, "the handler returning")
			if gPanic != nil {
				t.Fatalf("the handler panicked: %v", gPanic)
			}
			if !errors.Is(gErr, errs.ErrNotStarted) {
				t.Errorf("handler error = %v, want it to match errs.ErrNotStarted", gErr)
			}
			var classified *errs.ClassifiedError
			if !errors.As(gErr, &classified) || classified.Class != errs.ErrorTransient {
				t.Errorf("handler error = %T %v, want a transient classified error", gErr, gErr)
			}
			if tc.check != nil {
				tc.check(t, comp)
			}

			if err := comp.Stop(ctx); err != nil {
				t.Fatalf("Stop after the failed Start: %v", err)
			}
		})
	}
}

// TestFailedConsumerSetupCallbackNaksAfterRollback: the entity consumer's setup runs its
// callback on goroutine G with a valid Graphable message whose Metadata blocks, then fails
// with a transient DeadlineExceeded and no handle (natsclient's commitConsumer, context-ended
// branch). The failed consumer is not recorded, so the rollback cannot drain it. Once Start
// has returned, the hold is released: G must Nak the message, never ack or terminate it, and
// must not panic.
func TestFailedConsumerSetupCallbackNaksAfterRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	comp := createTestComponentForLifecycle(t)
	registerMergeTestPayload(t, comp)
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// A message that decodes and is accepted, so the callback reaches its submit. One that
	// does not is acked or terminated before it, and would show nothing.
	payload := &mergeTestGraphable{entityID: failedStartEntityID, triples: []message.Triple{
		{Subject: failedStartEntityID, Predicate: "test.state.value", Object: "streamed", Confidence: 1},
	}}
	msg := &heldMetadataMsg{
		streamLaneMsg: newStreamLaneMsg(streamLaneData(t, payload, "b2-producer"), 1),
		entered:       make(chan struct{}),
		release:       make(chan struct{}),
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(msg.release) }) }
	defer release()

	setupErr := errs.WrapTransient(context.DeadlineExceeded, "test", "ConsumeStreamWithConfig", "commit consumer")
	gReturned := make(chan struct{})
	var gPanic any
	var calls int
	comp.consumeStream = func(
		_ context.Context,
		_ natsclient.PortConsumerContext,
		_ natsclient.StreamConsumerConfig,
		handler func(context.Context, jetstream.Msg),
	) (jetstream.ConsumeContext, error) {
		calls++
		if calls > 1 {
			t.Error("Start bound another consumer after the first one failed")
			return nil, setupErr
		}
		go func() {
			defer close(gReturned)
			defer func() { gPanic = recover() }()
			// natsclient's handler context is the client's (stream.go:446): the rollback does not end it.
			handler(t.Context(), msg)
		}()
		select {
		case <-msg.entered:
		case <-ctx.Done():
		}
		return nil, setupErr
	}

	startErr := comp.Start(ctx)
	if !errors.Is(startErr, setupErr) {
		t.Fatalf("Start error = %v, want it to wrap the consumer setup's error", startErr)
	}

	release()
	awaitClosed(ctx, t, gReturned, "the callback returning")
	if gPanic != nil {
		t.Fatalf("the callback panicked after the rollback: %v", gPanic)
	}
	select {
	case disposition := <-msg.settled:
		if disposition != "nak" {
			t.Fatalf("disposition = %s, want nak", disposition)
		}
	default:
		t.Fatal("the callback returned without settling the message")
	}
	select {
	case extra := <-msg.settled:
		t.Fatalf("a second disposition after the nak: %s", extra)
	default:
	}
}

// drainedConsumeContext is a consumer handle whose Drain is counted and closes Closed, as a
// drained consumer with nothing in flight does.
type drainedConsumeContext struct {
	drains atomic.Int32
	once   sync.Once
	closed chan struct{}
}

func newDrainedConsumeContext() *drainedConsumeContext {
	return &drainedConsumeContext{closed: make(chan struct{})}
}

func (h *drainedConsumeContext) Stop()                   { h.once.Do(func() { close(h.closed) }) }
func (h *drainedConsumeContext) Closed() <-chan struct{} { return h.closed }
func (h *drainedConsumeContext) Drain() {
	h.drains.Add(1)
	h.once.Do(func() { close(h.closed) })
}

// onlyDisposition returns msg's one disposition, or "none" when it has none yet. A second
// disposition fails the test.
func onlyDisposition(t *testing.T, msg *streamLaneMsg) string {
	t.Helper()
	select {
	case disposition := <-msg.settled:
		select {
		case extra := <-msg.settled:
			t.Errorf("a second disposition after %s: %s", disposition, extra)
		default:
		}
		return disposition
	default:
		return "none"
	}
}

// TestFailedStartDrainsAndNaksHeldInput: the entity consumer binds while its callback is running on
// an input, and a later registration fails Start. The callback runs on goroutine G with a context
// the rollback does not end, as natsclient's is, and is held in Metadata until the rollback has
// drained the consumer. Released, G accepts the input and reaches the pool Start captured, which
// refuses it, and G negatively acknowledges the input (checkpoint 3 B2).
func TestFailedStartDrainsAndNaksHeldInput(t *testing.T) {
	payload := &mergeTestGraphable{entityID: failedStartEntityID, triples: []message.Triple{
		{Subject: failedStartEntityID, Predicate: "test.state.value", Object: "streamed", Confidence: 1},
	}}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	comp := createTestComponentForLifecycle(t)
	registerMergeTestPayload(t, comp)
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	msg := &heldMetadataMsg{
		streamLaneMsg: newStreamLaneMsg(streamLaneData(t, payload, "r3-producer"), 1),
		entered:       make(chan struct{}),
		release:       make(chan struct{}),
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(msg.release) }) }
	defer release()
	comp.waitForStreamInput = func(context.Context, string) (time.Time, error) { return time.Unix(1, 0), nil }
	handle := newDrainedConsumeContext()
	gReturned := make(chan struct{})
	var gPanic any
	comp.consumeStream = func(
		_ context.Context,
		_ natsclient.PortConsumerContext,
		_ natsclient.StreamConsumerConfig,
		handler func(context.Context, jetstream.Msg),
	) (jetstream.ConsumeContext, error) {
		go func() {
			defer close(gReturned)
			defer func() { gPanic = recover() }()
			handler(t.Context(), msg)
		}()
		select {
		case <-msg.entered:
		case <-ctx.Done():
		}
		return handle, nil
	}
	registrationErr := errs.WrapTransient(context.DeadlineExceeded,
		"test", "SubscribeForRequests", "registration round trip")
	comp.subscribeRequests = func(
		context.Context, string, func(context.Context, []byte) ([]byte, error),
	) (*natsclient.Subscription, error) {
		return nil, registrationErr
	}

	startErr := comp.Start(ctx)
	if !errors.Is(startErr, registrationErr) {
		t.Fatalf("Start error = %v, want it to wrap the registration's error", startErr)
	}
	if n := handle.drains.Load(); n != 1 {
		t.Errorf("the rollback drained the consumer %d times, want 1", n)
	}

	release()
	awaitClosed(ctx, t, gReturned, "the callback returning")
	if gPanic != nil {
		t.Fatalf("the callback panicked after the rollback: %v", gPanic)
	}
	if got := onlyDisposition(t, msg.streamLaneMsg); got != "nak" {
		t.Errorf("disposition = %s, want nak", got)
	}
}
