//go:build integration

package config

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/model"
	"github.com/c360studio/semengine/natsclient"
)

// managerStopBound is the fresh finite context a test's Stop gets.
const managerStopBound = 10 * time.Second

// watchCountBound bounds how long the adapter polls the server for the watch consumer count to
// match what the Manager reports.
const watchCountBound = 5 * time.Second

// stopManager stops manager under a fresh bounded context and requires it to release everything.
func stopManager(t *testing.T, manager *Manager) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), managerStopBound)
	defer cancel()
	require.NoError(t, manager.Stop(ctx))
}

// watchConsumerCount reads, from the server, how many consumers the configuration bucket's backing
// stream has. A KV watch is an ordered consumer on that stream (nats.go v1.54.0 js.go:2026), so this
// counts the Manager's open watches without anything added to the Manager. A bucket that does not
// exist has none.
func watchConsumerCount(ctx context.Context, js jetstream.JetStream, bucket string) (int, error) {
	stream, err := js.Stream(ctx, natsclient.KVStreamPrefix+bucket)
	if errors.Is(err, jetstream.ErrStreamNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return 0, err
	}
	return info.State.Consumers, nil
}

// awaitWatchConsumers waits until the bucket's backing stream has exactly want consumers. Opening a
// watch and the server listing its consumer are not promised to be ordered for a reader on another
// connection, so this polls rather than reading once.
func awaitWatchConsumers(t *testing.T, ctx context.Context, client *natsclient.Client, bucket string, want int) {
	t.Helper()
	js, err := client.JetStream()
	require.NoError(t, err)
	waitCtx, cancel := context.WithTimeout(ctx, watchCountBound)
	defer cancel()
	_, err = probe.Await(waitCtx,
		func(ctx context.Context) (int, error) { return watchConsumerCount(ctx, js, bucket) },
		func(n int) bool { return n == want })
	require.NoError(t, err, "watch consumers on %s", bucket)
}

// managerSuiteOwner adapts Manager to the lifecycle suite (lifecycle-suite, "Observe adapter
// contract"; design D2). Observe reads only state production code uses: the Manager's watcher
// slice, its subscriber map, and the done channel of each goroutine it started, listed while that
// channel is open. Watcher stops are counted on the server: the configuration bucket's backing
// stream lists one consumer per open watch, and a successful KeyWatcher.Stop deletes it before it
// returns (nats.go v1.54.0 jetstream/kv.go:1239, nats.go:5409-5411). An ordered-consumer reset
// deletes the old consumer in the background (js.go:2390, :2425), so the count is polled with
// probe.Await under watchCountBound, never read once. Nothing is added to the Manager for this
// adapter (#186, #190).
//
// Calls is empty. The suite's no-op checks (a repeated Stop, a refused second Start, a Stop after
// a failed Start) compare Calls before and after a call, so a count of the adapter's own Start and
// Stop calls would fail them by construction; what a no-op must not change is what the Manager
// holds, which Unresolved lists.
type managerSuiteOwner struct {
	t       *testing.T
	manager *Manager
	js      jetstream.JetStream
}

// newManagerSuiteOwner wraps manager. observer is a connection of the test's own, used only to
// read the backing stream's consumer count.
func newManagerSuiteOwner(t *testing.T, observer *natsclient.Client, manager *Manager) *managerSuiteOwner {
	t.Helper()
	js, err := observer.JetStream()
	require.NoError(t, err)
	return &managerSuiteOwner{t: t, manager: manager, js: js}
}

func (o *managerSuiteOwner) Start(ctx context.Context) error { return o.manager.Start(ctx) }
func (o *managerSuiteOwner) Stop(ctx context.Context) error  { return o.manager.Stop(ctx) }

func (o *managerSuiteOwner) Observe() lifecycletest.Observation {
	cm := o.manager
	var held []string

	// The suite observes after a Start or Stop has returned to it. The subscriber map and the
	// done channels change under cm.mu when a channel is requested, so they are read under it.
	cm.mu.RLock()
	watchers := len(cm.watchers)
	subscribers := 0
	for _, channels := range cm.subscribers {
		subscribers += len(channels)
	}
	running := 0
	for _, done := range cm.workDone {
		if !isClosed(done) {
			running++
		}
	}
	cm.mu.RUnlock()

	if watchers > 0 {
		held = append(held, "watchers")
	}
	if subscribers > 0 {
		held = append(held, "subscriber channels")
	}
	if running > 0 {
		held = append(held, "goroutines")
	}

	ctx, cancel := context.WithTimeout(o.t.Context(), watchCountBound)
	defer cancel()
	consumers, err := probe.Await(ctx,
		func(ctx context.Context) (int, error) { return watchConsumerCount(ctx, o.js, cm.bucketName) },
		func(n int) bool { return n == watchers })
	if err != nil {
		// The count never matched the Manager's own watcher slice; report what the server holds.
		o.t.Logf("watch consumers on %s: %v", cm.bucketName, err)
	}
	if consumers > 0 {
		held = append(held, "watch consumers on the server")
	}
	return lifecycletest.Observation{Unresolved: held}
}

// isClosed reports whether done has been closed, without waiting.
func isClosed(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

// TestManagerPassesTheLifecycleSuite runs the lifecycle suite on the Manager through
// managerSuiteOwner (design D2, ADR-113). Each owner is a fresh Manager on a connection of its own,
// declaring a pair of its own, so each has its own bucket and its own backing stream to count. The
// must-fail owner's bucket records a foreign identity, so its Start acquires the bucket and then
// refuses, before it opens a watch. The Manager is one-shot: no restart is promised.
//
// spec: config-manager / Start bounds Start and Stop is the only stop
func TestManagerPassesTheLifecycleSuite(t *testing.T) {
	url := shared.Reset(t)
	observer := natsfixture.Open(t, url, openClient)

	owners := 0
	nextManager := func() (*Manager, *natsclient.Client) {
		owners++
		client := natsfixture.Open(t, url, openClient)
		manager, err := NewConfigManager(identityTestConfig("acme", fmt.Sprintf("suite%d", owners)), client, nil)
		require.NoError(t, err)
		return manager, client
	}
	factory := func() lifecycletest.Owner {
		manager, _ := nextManager()
		return newManagerSuiteOwner(t, observer, manager)
	}
	mustFail := func() lifecycletest.Owner {
		manager, _ := nextManager()
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		seedIdentityRecord(t, ctx, manager, platformIdentityRecord{Org: "foreignorg", Stem: "foreign", ID: "foreign-0a1b2c"})
		return newManagerSuiteOwner(t, observer, manager)
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})

	// lifecycle-suite, "Started owner reports its handles". The suite judges only what is left
	// after a Stop, so an adapter that never reported a kind would pass it. A started Manager with
	// a subscriber and a model-registry forwarder reports every kind, and a Stop releases them.
	t.Run("StartedOwnerReportsEveryKind", func(t *testing.T) {
		manager, _ := nextManager()
		owner := newManagerSuiteOwner(t, observer, manager)
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		require.NoError(t, manager.Start(ctx))
		_ = manager.onChange("services.*")
		applied := make(chan struct{}, 1)
		watchDone := make(chan struct{})
		go func() {
			defer close(watchDone)
			model.Watch(ctx, manager, func(*model.Registry) {
				select {
				case applied <- struct{}{}:
				default:
				}
			})
		}()
		select {
		case <-applied:
		case <-time.After(5 * time.Second):
			t.Fatal("model.Watch never received the initial registry")
		}
		require.ElementsMatch(t,
			[]string{"watchers", "subscriber channels", "goroutines", "watch consumers on the server"},
			owner.Observe().Unresolved)

		stopManager(t, manager)
		require.Empty(t, owner.Observe().Unresolved, "a Stop that returned nil left something held")
		select {
		case <-watchDone:
		case <-time.After(5 * time.Second):
			t.Fatal("model.Watch did not return after Stop")
		}
	})
}
