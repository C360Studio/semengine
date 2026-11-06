//go:build integration

package graphquery

import (
	"bytes"
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
	return newConfigured(t, client, `{}`)
}

func newConfigured(t *testing.T, client *natsclient.Client, config string) *Component {
	t.Helper()
	built, err := CreateGraphQuery(json.RawMessage(config), component.Dependencies{NATSClient: client})
	if err != nil {
		t.Fatalf("CreateGraphQuery: %v", err)
	}
	c := built.(*Component)
	if err := c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c
}

// suiteOwner is graph-query as the lifecycle suite's owner: start is how it starts, the component's
// own Start for a good owner.
type suiteOwner struct {
	c     *Component
	start func(context.Context) error
}

func (o suiteOwner) Start(ctx context.Context) error { return o.start(ctx) }

func (o suiteOwner) Stop(ctx context.Context) error { return o.c.Stop(ctx) }

// Observe reads the component's handles without a lock: the suite observes after a Start or Stop
// has returned to it, and only a call that overran its bound is observed while it runs, a check
// that has already failed. The community view is held while its supervisor runs: supervised stays
// set after the supervisor has returned, until cleanup clears it, so its being closed is read too.
func (o suiteOwner) Observe() lifecycletest.Observation {
	var held []string
	if len(o.c.subscriptions) > 0 {
		held = append(held, "query subscriptions")
	}
	if o.c.supervised != nil {
		select {
		case <-o.c.supervised:
		default:
			held = append(held, "community view")
		}
	}
	return lifecycletest.Observation{Unresolved: held}
}

// TestGraphQueryLifecycleSuite runs the lifecycle suite on the production Start. The failing owner
// has the shape of graph-ingest's: its start function runs the component's own start, which begins
// the community view's supervisor and subscribes every declared verb, and then fails, so its rollback
// has real subscriptions and a real supervisor to release.
func TestGraphQueryLifecycleSuite(t *testing.T) {
	client := fixtureClient(t)
	factory := func() lifecycletest.Owner {
		c := newComponent(t, client)
		return suiteOwner{c, c.Start}
	}
	mustFail := func() lifecycletest.Owner {
		c := newComponent(t, client)
		return suiteOwner{c, func(ctx context.Context) error {
			return c.lifecycle.Start(ctx, func(ctx context.Context) error {
				if err := c.start(ctx); err != nil {
					return err
				}
				return errors.New("start fails after acquiring")
			}, c.cleanup)
		}}
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	// lifecycle-suite, "Started owner reports its handles": the suite judges only what is left
	// after a Stop, so an adapter that omitted a kind would pass it.
	t.Run("StartedOwnerReportsItsHandles", func(t *testing.T) {
		owner := factory()
		if err := owner.Start(t.Context()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		want := []string{"query subscriptions", "community view"}
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

// TestStartThroughTheTable: the production Start on a connected client serves every forward after
// its context has ended and until Stop. Health follows, a second Start is refused, and a Start
// context that ends after Start returned changes neither Health nor serving (ADR-113). A Start whose
// context has already ended is refused.
func TestStartThroughTheTable(t *testing.T) {
	client := fixtureClient(t)
	c := newComponent(t, client)
	for _, f := range forwardRows(t) {
		serve(t, client, f.downstream.Subject, func(context.Context, []byte) ([]byte, error) {
			return []byte(f.downstream.Subject), nil
		})
	}

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
	for _, f := range forwardRows(t) {
		reply, err := client.RequestClassified(t.Context(), f.verb.Subject, []byte(`{}`), requestTimeout)
		if err != nil {
			t.Fatalf("request to %s once Start has ended: %v", f.verb.Subject, err)
		}
		if string(reply) != f.downstream.Subject {
			t.Errorf("reply on %s = %q, want the one from %s", f.verb.Subject, reply, f.downstream.Subject)
		}
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
	// One connection carries the unsubscribe and this request, in that order (the probe is this
	// assertion).
	for _, f := range declaredRows(t) {
		if reply, err := client.RequestClassified(t.Context(), f.verb.Subject, []byte(`{}`), requestTimeout); !errors.Is(err, nats.ErrNoResponders) {
			t.Errorf("request to %s after Stop = %q, %v, want no responders", f.verb.Subject, reply, err)
		}
	}
}

// TestFailedCleanupKeepsHandlesForLaterStop proves graph-query's branch of lifecycle-suite, "Failed
// start whose own cleanup fails": a Start that fails holding a subscription whose handler is still
// running, under a rollback whose context has already ended, returns both failures and keeps the
// subscriptions on record, and a later Stop that can finish returns nil and leaves nothing.
//
// As graph-ingest's TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop does, it reaches
// the shell's Start with a start function that fails after acquiring, and a rollback that is the
// component's own cleanup under an ended context. The subscriptions are the component's own. The
// busy verb's handler is the real forward, waiting on a downstream stub that holds its reply: that
// is what Drain must wait for. The other verbs are idle, and are released even though the busy one
// could not be. query_timeout_ms is set high so the stub, not that bound, ends the forward; natsclient's
// 30 s handler budget still applies, and a run that slow fails here loudly.
func TestFailedCleanupKeepsHandlesForLaterStop(t *testing.T) {
	client := fixtureClient(t)
	c := newConfigured(t, client, `{"query_timeout_ms": 600000}`)
	owner := suiteOwner{c: c}

	rows := declaredRows(t)
	busy, idle := rows[0], rows[1:]
	request := []byte(`{"held": true}`)
	entered, release := make(chan struct{}), make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	t.Cleanup(releaseOnce) // before the fixture's Close, which joins the handler
	serve(t, client, busy.downstream.Subject, func(_ context.Context, data []byte) ([]byte, error) {
		close(entered)
		<-release
		return data, nil
	})

	type outcome struct {
		reply []byte
		err   error
	}
	replied := make(chan outcome, 1)
	errStart := errors.New("start fails after acquiring")
	expired, expire := context.WithCancel(t.Context())
	expire()
	rolledBack := false // the guard runs the cleanup one call at a time
	err := c.lifecycle.Start(t.Context(),
		func(ctx context.Context) error {
			if err := c.subscribeQueries(ctx); err != nil {
				return err
			}
			go func() {
				reply, err := client.RequestClassified(context.WithoutCancel(ctx), busy.verb.Subject, request, 2*time.Minute)
				replied <- outcome{reply, err}
				close(replied) // so the wait below ends even if the arm under this took the value
			}()
			select {
			case <-entered: // Drain now has a running handler to wait for
			case got := <-replied:
				t.Errorf("the request returned before the stub received it: %q, %v", got.reply, got.err)
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
	if got := len(c.subscriptions); got != len(rows) {
		t.Fatalf("subscriptions kept after the failed cleanup = %d, want %d", got, len(rows))
	}
	if err := c.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("Start while cleanup is pending = %v, want ErrAlreadyStarted", err)
	}
	// One connection carries the unsubscribes and these requests, in that order (the probe is this
	// assertion), so the idle verbs are already unserved although the busy one could not be released.
	for _, f := range idle {
		if reply, err := client.RequestClassified(t.Context(), f.verb.Subject, []byte(`{}`), time.Minute); !errors.Is(err, nats.ErrNoResponders) {
			t.Fatalf("request to idle %s after the failed cleanup = %q, %v, want no responders", f.verb.Subject, reply, err)
		}
	}

	releaseOnce()
	got := <-replied
	if got.err != nil {
		t.Errorf("the request in flight during the failed cleanup: %v", got.err)
	}
	if !bytes.Equal(got.reply, request) {
		t.Errorf("the request in flight during the failed cleanup replied %q, want %q", got.reply, request)
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
