//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// shared is this package's NATS server. It starts at the first reset; TestMain removes it after the tests.
var shared natsfixture.Shared

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	code := m.Run()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := shared.Stop(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

func fixtureClient(t *testing.T) *natsclient.Client {
	t.Helper()
	return natsfixture.Open(t, shared.Reset(t), openClient)
}

func openClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url,
		natsclient.WithTimeout(15*time.Second),
		natsclient.WithMaxReconnects(0),
		natsclient.WithHealthInterval(0),
	)
	if err != nil {
		return nil, nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	if err := client.WaitForConnection(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	return client, client.Close, nil
}

// newComponent builds graph-query through its factory with the default configuration and
// initializes it.
func newComponent(t *testing.T, client *natsclient.Client) *Component {
	t.Helper()
	built, err := CreateGraphQuery(json.RawMessage(`{}`), component.Dependencies{NATSClient: client})
	if err != nil {
		t.Fatalf("CreateGraphQuery: %v", err)
	}
	c := built.(*Component)
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c
}

// A row the table does not declare, for a test that needs a walk with something to hold: the table
// has no graph-query rows until task 3.2.
func testVerb(name string) graph.QueryVerb {
	return graph.QueryVerb{Name: name, Subject: "graph.query." + name, Responder: graph.ResponderGraphQuery}
}

func echo(_ context.Context, data []byte) ([]byte, error) { return data, nil }

// suiteOwner is graph-query as the lifecycle suite's owner. It starts through the shell with the
// component's own walk and cleanup, over rows the test names, because the table has no graph-query
// rows until task 3.2 and a suite over an owner that holds nothing would judge nothing.
type suiteOwner struct {
	c        *Component
	verbs    []graph.QueryVerb
	handlers map[string]queryHandler
}

func (o suiteOwner) Start(ctx context.Context) error {
	return o.c.lifecycle.Start(ctx, func(ctx context.Context) error {
		return o.c.subscribeVerbs(ctx, o.verbs, o.handlers)
	}, o.c.cleanup)
}

func (o suiteOwner) Stop(ctx context.Context) error { return o.c.Stop(ctx) }

// Observe reads the component's handles without a lock: the suite observes after a Start or Stop
// has returned to it, and only a call that overran its bound is observed while it runs, a check
// that has already failed.
func (o suiteOwner) Observe() lifecycletest.Observation {
	var held []string
	if len(o.c.subscriptions) > 0 {
		held = append(held, "query subscriptions")
	}
	return lifecycletest.Observation{Unresolved: held}
}

// TestGraphQueryLifecycleSuite runs the lifecycle suite. The failing owner's walk subscribes its
// first verb and then finds the second has no handler, so its rollback has a real subscription to
// release.
func TestGraphQueryLifecycleSuite(t *testing.T) {
	client := fixtureClient(t)
	first, second := testVerb("first"), testVerb("second")
	factory := func() lifecycletest.Owner {
		return suiteOwner{newComponent(t, client), []graph.QueryVerb{first, second},
			map[string]queryHandler{first.Name: echo, second.Name: echo}}
	}
	mustFail := func() lifecycletest.Owner {
		return suiteOwner{newComponent(t, client), []graph.QueryVerb{first, second},
			map[string]queryHandler{first.Name: echo}}
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	// lifecycle-suite, "Started owner reports its handles": the suite judges only what is left
	// after a Stop, so an adapter that omitted a kind would pass it.
	t.Run("StartedOwnerReportsItsHandles", func(t *testing.T) {
		owner := factory()
		if err := owner.Start(t.Context()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		want := []string{"query subscriptions"}
		if held := owner.Observe().Unresolved; !slices.Equal(held, want) {
			t.Errorf("held while started = %v, want %v", held, want)
		}
		if err := owner.Stop(t.Context()); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		if held := owner.Observe().Unresolved; len(held) != 0 {
			t.Errorf("held after a nil Stop = %v, want nothing", held)
		}
	})
}

// TestStartThroughTheTable: the production Start on a connected client: Health follows, a second
// Start is refused, and a Start context that ends after Start returned does not change Health
// (ADR-113). A Start whose context has already ended is refused. The table has no graph-query row
// yet, so this Start subscribes nothing and the test proves nothing about serving; task 3.2 sends
// the request that does.
func TestStartThroughTheTable(t *testing.T) {
	client := fixtureClient(t)
	c := newComponent(t, client)

	ended, end := context.WithCancel(t.Context())
	end()
	if err := newComponent(t, client).Start(ended); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start under an ended context = %v, want context.Canceled", err)
	}

	startCtx, endStart := context.WithCancel(t.Context())
	if err := c.Start(startCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	endStart()
	if health := c.Health(); !health.Healthy || health.Status != "running" {
		t.Errorf("Health once started and the Start context ended = %+v, want running", health)
	}
	if err := c.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Errorf("second Start = %v, want ErrAlreadyStarted", err)
	}
	if err := c.Stop(t.Context()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if health := c.Health(); health.Healthy || health.Status != "stopped" {
		t.Errorf("Health after Stop = %+v, want stopped", health)
	}
}

// TestFailedCleanupKeepsHandlesForLaterStop proves graph-query's branch of lifecycle-suite, "Failed
// start whose own cleanup fails": a Start that fails holding a subscription whose handler is still
// running, under a rollback whose context has already ended, returns both failures and keeps the
// subscriptions on record, and a later Stop that can finish returns nil and leaves nothing.
//
// As graph-ingest's TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop does, it reaches
// the shell's Start with a start function that fails after acquiring, and a rollback that is the
// component's own cleanup under an ended context. The subscriptions are real, bound by the
// component's own walk. The first has a request in its handler, which is what Drain must wait for;
// the second is idle, and is released even though the first could not be.
func TestFailedCleanupKeepsHandlesForLaterStop(t *testing.T) {
	client := fixtureClient(t)
	c := newComponent(t, client)
	owner := suiteOwner{c: c}

	busy, idle := testVerb("busy"), testVerb("idle")
	entered, release := make(chan struct{}), make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce) // before the fixture's Close, which joins the handler
	handlers := map[string]queryHandler{
		busy.Name: func(_ context.Context, data []byte) ([]byte, error) {
			close(entered)
			<-release
			return data, nil
		},
		idle.Name: echo,
	}

	replied := make(chan error, 1)
	errStart := errors.New("start fails after acquiring")
	expired, expire := context.WithCancel(t.Context())
	expire()
	rolledBack := false // the guard runs the cleanup one call at a time
	err := c.lifecycle.Start(t.Context(),
		func(ctx context.Context) error {
			if err := c.subscribeVerbs(ctx, []graph.QueryVerb{busy, idle}, handlers); err != nil {
				return err
			}
			go func() {
				_, err := client.RequestClassified(context.WithoutCancel(ctx), busy.Subject, []byte(`{}`), 2*time.Minute)
				replied <- err
				close(replied) // so the wait below ends even if the arm under this took the value
			}()
			select {
			case <-entered: // Drain now has a running handler to wait for
			case err := <-replied:
				t.Errorf("the request returned before its handler ran: %v", err)
			}
			return errStart
		},
		func(ctx context.Context) error {
			if !rolledBack { // the rollback: cleanup under the ended context fails
				rolledBack = true
				return c.cleanup(expired)
			}
			return c.cleanup(ctx)
		})
	if !errors.Is(err, errStart) || !errors.Is(err, context.Canceled) {
		t.Fatalf("failed Start whose cleanup fails = %v, want the start failure and context.Canceled", err)
	}
	want := []string{"query subscriptions"}
	if got := owner.Observe().Unresolved; !slices.Equal(got, want) {
		t.Fatalf("held after the failed cleanup = %v, want %v", got, want)
	}
	if got := len(c.subscriptions); got != 2 {
		t.Fatalf("subscriptions kept after the failed cleanup = %d, want 2", got)
	}
	if err := c.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("Start while cleanup is pending = %v, want ErrAlreadyStarted", err)
	}
	// One connection carries the unsubscribe and this request, in that order (the probe is this
	// assertion), so the idle verb is already unserved although the busy one could not be released.
	if reply, err := client.RequestClassified(t.Context(), idle.Subject, []byte(`{}`), time.Minute); !errors.Is(err, nats.ErrNoResponders) {
		t.Fatalf("request to the idle verb after the failed cleanup = %q, %v, want no responders", reply, err)
	}

	releaseOnce()
	if err := <-replied; err != nil {
		t.Errorf("the request in flight during the failed cleanup: %v", err)
	}
	if err := c.Stop(t.Context()); err != nil {
		t.Fatalf("later Stop: %v", err)
	}
	if got := owner.Observe().Unresolved; len(got) != 0 {
		t.Fatalf("held after the later Stop = %v, want nothing", got)
	}
	if err := c.Stop(t.Context()); err != nil {
		t.Fatalf("repeated Stop: %v", err)
	}
}
