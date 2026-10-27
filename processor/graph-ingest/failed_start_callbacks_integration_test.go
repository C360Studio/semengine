//go:build integration

package graphingest

// Checkpoint 3 B2 (PR #93): natsclient can run a callback during an acquisition that then
// fails and returns no handle, so a failed Start's rollback has nothing to drain or join for
// it. A request handler refuses before Start completes (graph-transport-boundary,
// "graph-ingest refuses request work until Start completes"); a real request reaches it
// while a real Start is held, then is served once Start completes. The tests below it run a
// stream callback inside a real Start on a fixture broker and check that one that runs after
// the rollback Naks its message (graph-ingest-recovery, "Settlement order", "A delivery
// after a failed consumer setup").

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
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

// TestRequestBeforeStartCompletesIsRefused holds graph-transport-boundary, "graph-ingest
// refuses request work until Start completes", scenarios "A query before Start completes"
// and "A mutation before Start completes" (fix-301 design D2, "Not-started refusal").
// graph-ingest's logger handler pauses Start's goroutine on the record its query or
// mutation setup logs once those subscriptions are registered, before Start completes. A
// real request on the paused component's subject is refused with a transient error with no
// code whose message is errs.ErrNotStarted's (the sentinel does not cross the wire; the
// requester sees the class, code and message), and does no work. The pause is then
// released, Start completes, and the same request is served.
func TestRequestBeforeStartCompletesIsRefused(t *testing.T) {
	cases := []struct {
		name string
		// record is the Info record logged once the case's subscriptions are registered.
		record  string
		subject func(t *testing.T, c *Component) string
		request func(t *testing.T) []byte
		// seed runs while Start is held, before the refused request.
		seed func(t *testing.T, c *Component)
		// refused runs after the refused request; served after the served one.
		refused func(t *testing.T, c *Component)
		served  func(t *testing.T, c *Component, reply []byte)
	}{
		{
			// The entity is seeded so a batch query that ran would succeed: only the
			// not-started refusal returns the error the test asserts.
			name:   "query",
			record: "query handlers registered",
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
			refused: func(*testing.T, *Component) {},
			served: func(t *testing.T, _ *Component, reply []byte) {
				var response struct {
					Entities []graph.EntityState `json:"entities"`
				}
				if err := json.Unmarshal(reply, &response); err != nil {
					t.Fatalf("decode the batch reply %s: %v", reply, err)
				}
				if len(response.Entities) != 1 || response.Entities[0].ID != failedStartEntityID {
					t.Errorf("batch reply after Start = %s, want the seeded %s", reply, failedStartEntityID)
				}
			},
		},
		{
			name:   "mutation",
			record: "mutation handlers registered",
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
			refused: func(t *testing.T, c *Component) {
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
			served: func(t *testing.T, c *Component, _ []byte) {
				if _, _, ok := storedRevision(t, c, failedStartEntityID); !ok {
					t.Errorf("ENTITY_STATES does not hold %s after the mutation was served", failedStartEntityID)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			client := newFixtureClient(t, entityStream)
			pause := newPauseOnRecord(tc.record)
			deps := testDependencies(t, client)
			deps.Logger = slog.New(pause)
			configJSON, err := json.Marshal(DefaultConfig())
			if err != nil {
				t.Fatalf("marshal config: %v", err)
			}
			created, err := CreateGraphIngest(configJSON, deps)
			if err != nil {
				t.Fatalf("CreateGraphIngest: %v", err)
			}
			comp := created.(*Component)
			if err := comp.Initialize(); err != nil {
				t.Fatalf("Initialize: %v", err)
			}
			subject := tc.subject(t, comp)
			request := tc.request(t)

			owner := newGraphIngestTestOwner(comp)
			defer owner.finish(ctx, t)
			startCtx := owner.startContext(ctx)
			var startErr error
			startReturned := make(chan struct{})
			go func() {
				defer close(startReturned)
				startErr = comp.Start(startCtx)
			}()
			// Before owner.finish: Stop must not run beside a held Start.
			defer func() {
				pause.Release()
				<-startReturned
			}()

			// Start's goroutine is held on the record: the subscriptions are live and
			// Start has not completed. What Start wrote before the record (the entity
			// bucket the seed and the checks use) is ordered before this wait.
			pause.awaitEntered(ctx, t, 30*time.Second)
			tc.seed(t, comp)
			reply, err := client.RequestClassified(ctx, subject, request, wireRequestTimeout)
			if err == nil {
				t.Fatalf("request before Start completed = %s, want a refusal", reply)
			}
			var classified *errs.ClassifiedError
			if !errors.As(err, &classified) || classified.Class != errs.ErrorTransient || classified.Code != "" {
				t.Errorf("refusal = %T %v, want a transient classified error with no code", err, err)
			}
			if !strings.Contains(err.Error(), errs.ErrNotStarted.Error()) {
				t.Errorf("refusal = %v, want it to carry %q", err, errs.ErrNotStarted)
			}
			// The reply above was sent after the handler returned, so its work, if any, is done.
			tc.refused(t, comp)

			pause.Release()
			<-startReturned // orders the read of startErr after Start's write
			if startErr != nil {
				t.Fatalf("Start after the pause was released: %v", startErr)
			}
			reply, err = client.RequestClassified(ctx, subject, request, wireRequestTimeout)
			if err != nil {
				t.Fatalf("the same request after Start completed: %v", err)
			}
			tc.served(t, comp, reply)
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
