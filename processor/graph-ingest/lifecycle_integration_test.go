//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// The pin ran these tests on one NATS client shared through TestMain
// (natsclient.NewSharedTestClient), terminated with no deadline. natsfixture.Open
// takes the test (design D3), so each test here opens a broker of its own, which its
// cleanup stops under a bounded context. The pin's TestMain also registered four test
// predicates for merge_entity_integration_test.go, which a later stage ports, and
// checked the owner-child request first; TestGraphIngestOwnerChildFixture checks that
// request itself.

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
func createTestComponentForLifecycle(t *testing.T) *Component {
	t.Helper()
	config := DefaultConfig()
	deps := component.Dependencies{
		NATSClient:      newFixtureClient(t, entityStream),
		PayloadRegistry: newTestPayloadRegistry(t),
		Platform:        component.PlatformMeta{Org: testDeploymentOrg, Platform: testDeploymentPlatform},
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	comp, err := CreateGraphIngest(configJSON, deps)
	if err != nil {
		t.Fatalf("create component: %v", err)
	}

	return comp.(*Component)
}

func TestGraphIngest_OneShotLifecycleAgainstNATS(t *testing.T) {
	comp := createTestComponentForLifecycle(t)
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := comp.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := comp.Stop(t.Context()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := comp.Stop(t.Context()); err != nil {
		t.Fatalf("repeated completed Stop: %v", err)
	}
	if err := comp.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("Start after terminal Stop error = %v, want ErrAlreadyStarted", err)
	}
}

type graphIngestTrackingConsumeContext struct {
	jetstream.ConsumeContext
	drains atomic.Int32
}

func (c *graphIngestTrackingConsumeContext) Drain() {
	c.drains.Add(1)
	c.ConsumeContext.Drain()
}

type graphIngestObservedContext struct {
	context.Context
	doneOnce sync.Once
	doneSeen chan struct{}
}

func (c *graphIngestObservedContext) Done() <-chan struct{} {
	c.doneOnce.Do(func() { close(c.doneSeen) })
	return c.Context.Done()
}

func TestGraphIngest_FailedStartRollbackOwnsExactConsumerAndPreservesDurable(t *testing.T) {
	config := DefaultConfig()
	mutation := config.Ports.Inputs[1]
	config.Ports.Inputs = []component.PortDefinition{
		{Name: "entity_one", Config: component.JetStreamPort{StreamName: "ENTITY", Subjects: []string{"entity.one"}, DeliverPolicy: "all"}},
		{Name: "entity_two", Config: component.JetStreamPort{StreamName: "ENTITY", Subjects: []string{"entity.two"}, DeliverPolicy: "all"}},
		mutation,
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	created, err := CreateGraphIngest(configJSON, testDependencies(t, newFixtureClient(t, entityStream)))
	if err != nil {
		t.Fatalf("CreateGraphIngest: %v", err)
	}
	comp := created.(*Component)
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	secondEntered := make(chan struct{})
	releaseSecond := make(chan struct{})
	sentinel := errors.New("second consumer acquisition failed")
	var calls atomic.Int32
	var first *graphIngestTrackingConsumeContext
	comp.consumeStream = func(
		ctx context.Context,
		owner natsclient.PortConsumerContext,
		cfg natsclient.StreamConsumerConfig,
		handler func(context.Context, jetstream.Msg),
	) (jetstream.ConsumeContext, error) {
		if calls.Add(1) == 1 {
			handle, consumeErr := comp.natsClient.ConsumeStreamWithConfig(ctx, owner, cfg, handler)
			if consumeErr != nil {
				return nil, consumeErr
			}
			first = &graphIngestTrackingConsumeContext{ConsumeContext: handle}
			return first, nil
		}
		close(secondEntered)
		<-releaseSecond
		return nil, sentinel
	}

	startResult := make(chan error, 1)
	go func() { startResult <- comp.Start(t.Context()) }()
	<-secondEntered
	// The pin's lifecycleMu, now handlesMu: it guards the component's handles, not the
	// guard's state, and stays (design D22).
	comp.handlesMu.Lock()
	if len(comp.consumers) != 1 || comp.consumers[0].handle != first {
		comp.handlesMu.Unlock()
		t.Fatal("first exact handle was not published before second acquisition")
	}
	comp.handlesMu.Unlock()

	stopCtx := &graphIngestObservedContext{Context: t.Context(), doneSeen: make(chan struct{})}
	stopResult := make(chan error, 1)
	go func() { stopResult <- comp.Stop(stopCtx) }()
	<-stopCtx.doneSeen
	select {
	case stopErr := <-stopResult:
		t.Fatalf("Stop returned before Start finalized: %v", stopErr)
	default:
	}
	close(releaseSecond)
	if startErr := <-startResult; !errors.Is(startErr, sentinel) {
		t.Fatalf("Start error = %v, want sentinel", startErr)
	}
	if stopErr := <-stopResult; stopErr != nil {
		t.Fatalf("overlapping Stop: %v", stopErr)
	}
	if first == nil || first.drains.Load() != 1 {
		t.Fatalf("first native Drain calls = %v, want 1", first)
	}

	consumerName := "graph-ingest-" + strings.ReplaceAll("entity.one", ".", "-")
	if _, observeErr := comp.observeOutstandingWork(t.Context(), "ENTITY", consumerName); observeErr != nil {
		t.Fatalf("durable consumer was deleted during failed-Start rollback: %v", observeErr)
	}
}

// TestGraphIngest_FailedBootSweepFailsStart makes the boot sweep of ENTITY_STATES fail
// inside a real Start, at each of its three failure exits, and checks that Start returns
// the failure, classified transient, and that the rollback left nothing running: no
// consumer on the input stream, no responder on a query subject, the sweep's watcher
// stopped, and a guard that refuses a later Start and lets Stop return nil (design D13).
func TestGraphIngest_FailedBootSweepFailsStart(t *testing.T) {
	sentinel := errors.New("ENTITY_STATES watch refused")
	cases := []struct {
		name string
		// watch is the sweep's Watch; opened is closed once it has been called.
		watch func(opened chan<- struct{}, watcher *ingestGuardWatcher) (jetstream.KeyWatcher, error)
		// cancelDuringSweep cancels Start's context once the watcher is open.
		cancelDuringSweep bool
		want              error
	}{
		{
			name: "watch refused",
			watch: func(opened chan<- struct{}, _ *ingestGuardWatcher) (jetstream.KeyWatcher, error) {
				close(opened)
				return nil, sentinel
			},
			want: sentinel,
		},
		{
			name: "updates closed before the end of the snapshot",
			watch: func(opened chan<- struct{}, watcher *ingestGuardWatcher) (jetstream.KeyWatcher, error) {
				watcher.closeUpdates()
				close(opened)
				return watcher, nil
			},
		},
		{
			name: "start cancelled during the sweep",
			watch: func(opened chan<- struct{}, watcher *ingestGuardWatcher) (jetstream.KeyWatcher, error) {
				close(opened)
				return watcher, nil
			},
			cancelDuringSweep: true,
			want:              context.Canceled,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comp := createTestComponentForLifecycle(t)
			if err := comp.Initialize(); err != nil {
				t.Fatalf("Initialize: %v", err)
			}
			watcher := newIngestGuardWatcher(0)
			opened := make(chan struct{})
			comp.watchEntityStates = func(context.Context, string) (jetstream.KeyWatcher, error) {
				return tc.watch(opened, watcher)
			}

			startCtx, cancelStart := context.WithCancel(t.Context())
			defer cancelStart()
			startResult := make(chan error, 1)
			go func() { startResult <- comp.Start(startCtx) }()
			<-opened
			if tc.cancelDuringSweep {
				cancelStart()
			}
			startErr := <-startResult

			if startErr == nil {
				t.Fatal("Start returned nil after the boot sweep failed")
			}
			if tc.want != nil && !errors.Is(startErr, tc.want) {
				t.Fatalf("Start error = %v, want it to wrap %v", startErr, tc.want)
			}
			var classified *errs.ClassifiedError
			if !errors.As(startErr, &classified) || classified.Class != errs.ErrorTransient {
				t.Fatalf("Start error = %T %v, want a transient classified error", startErr, startErr)
			}
			// Every case but the refused watch hands the sweep a watcher, which it must stop.
			if tc.want != sentinel && !watcher.stopped() {
				t.Fatal("the sweep's watcher was not stopped")
			}

			health := comp.Health()
			if health.Healthy || health.Status != "stopped" {
				t.Fatalf("Health after a failed Start = %+v, want stopped and not healthy", health)
			}
			js, err := comp.natsClient.JetStream()
			if err != nil {
				t.Fatalf("JetStream: %v", err)
			}
			stream, err := js.Stream(t.Context(), entityStream.name)
			if err != nil {
				t.Fatalf("stream %s: %v", entityStream.name, err)
			}
			info, err := stream.Info(t.Context())
			if err != nil {
				t.Fatalf("stream %s info: %v", entityStream.name, err)
			}
			if info.State.Consumers != 0 {
				t.Fatalf("stream %s has %d consumers after a failed Start, want 0", entityStream.name, info.State.Consumers)
			}
			for _, verb := range graph.QueryVerbs() {
				if verb.Responder != queryResponder {
					continue
				}
				_, err := comp.natsClient.RequestClassified(t.Context(), verb.Subject, []byte(`{}`), 5*time.Second)
				if !natsclient.IsNoResponders(err) {
					t.Fatalf("request on %s after a failed Start: %v, want no responders", verb.Subject, err)
				}
			}

			if err := comp.Stop(t.Context()); err != nil {
				t.Fatalf("Stop after a failed Start: %v", err)
			}
			if err := comp.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
				t.Fatalf("Start after a failed Start = %v, want ErrAlreadyStarted", err)
			}
		})
	}
}

// TestGraphIngestLifecycleSuite runs the lifecycle suite on graph-ingest through its
// adapter, suiteOwner. Each healthy owner is a fresh component on a client of its own,
// connected to one fixture broker that holds the ENTITY stream. The must-fail owner's
// client dials a listener that closes every connection before the handshake, so Start
// fails on its connection, before the component binds anything, and the cleanup Start
// runs succeeds: this is the
// "Cleanup succeeds after a failed start" scenario of lifecycle-suite's "Failed start
// whose own cleanup fails". The branch where that cleanup fails is
// TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop's. graph-ingest is
// one-shot (design D13: a Start after Stop is refused), so no restart is promised.
func TestGraphIngestLifecycleSuite(t *testing.T) {
	f := natsfixture.New(t)
	if err := f.Start(t.Context()); err != nil {
		t.Fatalf("natsfixture Start: %v", err)
	}
	if _, err := f.CreateStream(t.Context(), entityStream.name, entityStream.subjects...); err != nil {
		t.Fatalf("CreateStream %s: %v", entityStream.name, err)
	}
	configJSON, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	newOwner := func(client *natsclient.Client) lifecycletest.Owner {
		created, err := CreateGraphIngest(configJSON, testDependencies(t, client))
		if err != nil {
			t.Fatalf("CreateGraphIngest: %v", err)
		}
		comp := created.(*Component)
		if err := comp.Initialize(); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		return newSuiteOwner(comp)
	}
	refused := refusingBrokerURL(t)
	factory := func() lifecycletest.Owner {
		return newOwner(natsfixture.Open(t, f, openFixtureClient))
	}
	mustFail := func() lifecycletest.Owner {
		client, err := natsclient.NewClient(refused,
			natsclient.WithMaxReconnects(0),
			natsclient.WithHealthInterval(0),
		)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		return newOwner(client)
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	// lifecycle-suite, "Started owner reports its handles". The suite judges only what is
	// left after a Stop, so an adapter that omitted a kind would pass it; this names every
	// kind a started component holds.
	t.Run("StartedOwnerReportsItsHandles", func(t *testing.T) {
		owner := factory()
		startCtx, cancelStart := context.WithCancel(t.Context())
		defer cancelStart()
		if err := owner.Start(startCtx); err != nil {
			t.Fatalf("Start: %v", err)
		}
		started := owner.Observe()
		want := []string{
			"runtime cancel", "ingest submit cancel", "ingest pool cancel", "ingest pool lanes",
			"consumers", "request subscriptions", "status loop", "status publisher", "entity cache",
			"readiness bound consumers",
		}
		if !slices.Equal(started.Unresolved, want) {
			t.Errorf("held while started = %v, want %v", started.Unresolved, want)
		}
		binds := started.Calls["consumer bind"]
		if binds == 0 || started.Calls["consumer drain"] != 0 {
			t.Errorf("calls while started = %v, want consumer binds and no drain", started.Calls)
		}
		stopCtx, cancelStop := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancelStop()
		if err := owner.Stop(stopCtx); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		stopped := owner.Observe()
		if len(stopped.Unresolved) != 0 {
			t.Errorf("held after a nil Stop = %v, want nothing", stopped.Unresolved)
		}
		if stopped.Calls["consumer drain"] != binds || stopped.Calls["consumer stop"] != 0 {
			t.Errorf("calls after Stop = %v, want one drain per bound consumer and no force stop", stopped.Calls)
		}
	})
}

// refusingBrokerURL returns the NATS URL of a loopback listener that stays bound for the
// test's life and closes every connection it accepts before sending the server's INFO, so
// a client's Connect to it fails on the handshake. A port that was bound and released can
// be handed to another process, whose broker would accept the dial (#89). The listener is
// closed, and its accept loop joined, on cleanup.
func refusingBrokerURL(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	accepting := make(chan struct{})
	go func() {
		defer close(accepting)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // the listener is closed
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-accepting
	})
	return "nats://" + listener.Addr().String()
}
