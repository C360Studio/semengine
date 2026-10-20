//go:build integration

package config

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/model"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/types"
)

// TestConfigManager_ShutdownSequence verifies clean shutdown without panics: Stop closes every
// subscriber channel, so each reader sees its channel closed and returns.
func TestConfigManager_ShutdownSequence(t *testing.T) {
	natsClient := fixtureClient(t)

	// Create initial config
	cfg := &Config{
		Version: "1.0.0",
		Platform: PlatformConfig{
			ID:     "test-platform",
			Type:   "edge",
			Org:    "test-org",
			Region: "us-west",
		},
		NATS: NATSConfig{
			URLs: []string{"nats://a.invalid:14222"},
		},
	}

	// Create Manager
	cm, err := NewConfigManager(cfg, natsClient, slog.Default())
	require.NoError(t, err)

	// Start the manager
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	err = cm.Start(ctx)
	require.NoError(t, err)

	// Create multiple subscribers
	var subscribers []<-chan update
	for i := 0; i < 5; i++ {
		ch := cm.onChange("services.*")
		subscribers = append(subscribers, ch)
	}

	// Each reader reports once it has received the initial update onChange sends, and returns
	// only when its channel is closed. The pin slept 10ms here; the started channel replaces it.
	var readers sync.WaitGroup
	started := make(chan struct{}, len(subscribers))
	for _, ch := range subscribers {
		readers.Add(1)
		go func(updates <-chan update) {
			defer readers.Done()
			first := true
			for range updates {
				if first {
					started <- struct{}{}
					first = false
				}
			}
		}(ch)
	}
	for range subscribers {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("a subscriber never received its initial update")
		}
	}

	// Stop the manager - this should close all channels
	stopManager(t, cm)

	// Every reader returns because its channel was closed. The pin slept 10ms and checked
	// nothing; this waits for the readers, bounded.
	readersDone := make(chan struct{})
	go func() {
		readers.Wait()
		close(readersDone)
	}()
	select {
	case <-readersDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop returned but a subscriber channel was never closed")
	}
}

// TestConfigManager_ConcurrentShutdown tests concurrent Stop calls. Concurrent Stops may be
// refused as transient while another runs the cleanup (internal/lifecycleguard); their errors
// are ignored as at the pin. Once they have all returned, one has released everything, so a
// further Stop returns nil.
func TestConfigManager_ConcurrentShutdown(t *testing.T) {
	natsClient := fixtureClient(t)

	cfg := &Config{
		Version: "1.0.0",
		Platform: PlatformConfig{
			ID:     "test-platform",
			Type:   "edge",
			Org:    "test-org",
			Region: "us-west",
		},
		NATS: NATSConfig{
			URLs: []string{"nats://a.invalid:14222"},
		},
	}

	cm, err := NewConfigManager(cfg, natsClient, slog.Default())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	err = cm.Start(ctx)
	require.NoError(t, err)

	// Subscribe
	ch := cm.onChange("services.*")

	// A reader that drains until Stop closes the channel.
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for range ch {
		}
	}()

	// Multiple goroutines calling Stop concurrently. The pin slept 100ms for them; the wait
	// group replaces it.
	var stops sync.WaitGroup
	for i := 0; i < 3; i++ {
		stops.Add(1)
		go func() {
			defer stops.Done()
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer stopCancel()
			_ = cm.Stop(stopCtx)
		}()
	}
	stops.Wait()

	// Should handle multiple Stop calls gracefully
	stopManager(t, cm)

	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop returned but the subscriber channel was never closed")
	}
}

// TestStartContextEndingKeepsDelivering is P2 as a test: ending the context given to Start, after
// Start returned, stops nothing. A put to services.x by another writer still reaches the
// subscriber, and Stop then returns nil (design D2).
//
// spec: config-manager / Start bounds Start and Stop is the only stop
func TestStartContextEndingKeepsDelivering(t *testing.T) {
	tc := fixtureClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	manager := newIdentityManager(t, tc, "acme", "dep")
	startCtx, cancelStart := context.WithCancel(ctx)
	require.NoError(t, manager.Start(startCtx))
	defer stopManager(t, manager)

	updates := manager.onChange("services.*")
	select {
	case <-updates: // the initial update
	case <-time.After(5 * time.Second):
		t.Fatal("no initial update from onChange")
	}

	cancelStart()

	putConfigValue(t, ctx, manager, "services.x", types.ServiceConfig{Enabled: true})
	select {
	case up, ok := <-updates:
		require.True(t, ok, "the subscriber channel closed when the Start context ended")
		require.Equal(t, "services.x", up.Path)
		require.Contains(t, up.Config.Get().Services, "x", "the put is applied after the Start context ended")
	case <-time.After(10 * time.Second):
		t.Fatal("after the Start context ended, a put to services.x was never delivered")
	}
}

// TestStopWithAModelRegistryWatcherReturnsPromptly is P1 as a test: a running model.Watch does
// not hold Stop. At the pin the forwarder waited for Stop and Stop waited for the forwarder, so
// Stop ran out its timeout. The apply callback receiving the initial registry is the point at
// which the forwarder has taken its first update, the condition P1 needed a 200ms sleep for.
//
// spec: config-manager / Start bounds Start and Stop is the only stop
func TestStopWithAModelRegistryWatcherReturnsPromptly(t *testing.T) {
	tc := fixtureClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cfg := identityTestConfig("acme", "dep")
	cfg.ModelRegistry = &model.Registry{
		Endpoints: map[string]*model.EndpointConfig{
			"initial": {Provider: "ollama", URL: "http://x/v1", Model: "initial-model", MaxTokens: 1024},
		},
		Defaults: model.DefaultsConfig{Model: "initial"},
	}
	manager, err := NewConfigManager(cfg, tc, nil)
	require.NoError(t, err)
	require.NoError(t, manager.Start(ctx))

	applied := make(chan struct{}, 1)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		// The watch context stays live: only the channel closing can end model.Watch.
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
	direct := manager.WatchModelRegistry()

	stopCtx, stopCancel := context.WithTimeout(ctx, time.Second)
	defer stopCancel()
	require.NoError(t, manager.Stop(stopCtx), "a live model-registry watcher must not hold Stop")
	require.NoError(t, stopCtx.Err(), "Stop returned nil only after its one-second context had ended")

	select {
	case <-watchDone:
	case <-time.After(5 * time.Second):
		t.Fatal("model.Watch did not return after Stop: its channel was not closed")
	}
	requireClosedSoon(t, direct, "the channel WatchModelRegistry returned before Stop")
}

// TestChannelsRequestedAfterStopAreClosed: onChange and WatchModelRegistry after Stop return a
// closed channel and start nothing the Manager would have to join (design D2).
//
// spec: config-manager / Start bounds Start and Stop is the only stop
func TestChannelsRequestedAfterStopAreClosed(t *testing.T) {
	tc := fixtureClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	manager := newIdentityManager(t, tc, "acme", "dep")
	require.NoError(t, manager.Start(ctx))
	stopManager(t, manager)

	requireClosedSoon(t, manager.onChange("services.*"), "onChange after Stop")
	requireClosedSoon(t, manager.WatchModelRegistry(), "WatchModelRegistry after Stop")

	observer := newManagerSuiteOwner(t, tc, manager)
	require.Empty(t, observer.Observe().Unresolved, "a channel requested after Stop must start nothing")
}

// TestSecondStartIsRefusedAsAlreadyStarted is P3 as a test. The bucket records the declared
// identifier unsuffixed, so the identity guard that refused a second Start at the pin, by
// accident, has nothing to refuse: only the one-shot lifecycle refuses here, before contacting
// NATS (design D2).
//
// spec: config-manager / Start bounds Start and Stop is the only stop
func TestSecondStartIsRefusedAsAlreadyStarted(t *testing.T) {
	tc := fixtureClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	manager := newIdentityManager(t, tc, "acme", "dep")
	seedDeclaredIdentity(t, ctx, manager)
	require.NoError(t, manager.Start(ctx))

	err := manager.Start(ctx)
	require.ErrorIs(t, err, errs.ErrAlreadyStarted, "a second Start on a running Manager")

	stopManager(t, manager)
	err = manager.Start(ctx)
	require.ErrorIs(t, err, errs.ErrAlreadyStarted, "a Start after Stop")
}

// TestBlockedKeyFamilyHandlerHoldsStop: a key-family handler that has not returned keeps Stop from
// claiming everything is released. Stop returns its context's error, and a later Stop succeeds
// once the handler returns (design D2).
//
// spec: config-manager / Start bounds Start and Stop is the only stop
func TestBlockedKeyFamilyHandlerHoldsStop(t *testing.T) {
	tc := fixtureClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	family, err := NewKeyFamily("rules", func(KeyFamilyEntry) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
	})
	require.NoError(t, err)
	manager, err := NewConfigManager(identityTestConfig("acme", "dep"), tc, nil, WithKeyFamily(family))
	require.NoError(t, err)
	require.NoError(t, manager.Start(ctx))
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()

	require.NoError(t, family.Put(ctx, "a", []byte(`"a"`)))
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the family handler was never called")
	}

	blockedCtx, blockedCancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer blockedCancel()
	err = manager.Stop(blockedCtx)
	require.True(t, errors.Is(err, context.DeadlineExceeded),
		"Stop while a family handler is blocked must return its context's error, got %v", err)

	close(release)
	released = true
	stopManager(t, manager)
}

// requireClosedSoon reads ch until it is closed, failing if it is not closed within five seconds.
// Values already buffered are read and dropped.
func requireClosedSoon[T any](t *testing.T, ch <-chan T, what string) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-timeout:
			t.Fatalf("%s: channel not closed within 5s", what)
		}
	}
}

// TestStopHonoursItsDeadlineWhenAConsumerDeleteIsDenied: a deployment whose credentials may open
// the watches but not delete their consumers, the same failure a broker outage gives. Each watch's
// Stop deletes its consumer with a request nats.go v1.54.0 bounds by its own timeout, not Stop's
// context (measured 25 s for five watches on the Codex review of PR #329). Stop must still return
// its context's error once its context ends, and a later Stop must join the teardown and return nil.
//
// The server is an embedded nats-server (the module's pinned v2.15.1) on an ephemeral port, not the
// shared Docker fixture: the fixture cannot deny one subject to one user (notes/2.2.md).
//
// spec: config-manager / Stop(ctx) stops the Manager
func TestStopHonoursItsDeadlineWhenAConsumerDeleteIsDenied(t *testing.T) {
	server, err := natsserver.NewServer(&natsserver.Options{
		Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true, JetStream: true, StoreDir: t.TempDir(),
		Users: []*natsserver.User{{Username: "edge", Password: "edge", Permissions: &natsserver.Permissions{
			Publish: &natsserver.SubjectPermission{Allow: []string{">"}, Deny: []string{"$JS.API.CONSUMER.DELETE.>"}},
		}}},
	})
	require.NoError(t, err)
	server.Start()
	t.Cleanup(func() {
		done := make(chan struct{})
		go func() { server.Shutdown(); server.WaitForShutdown(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("embedded server did not shut down within 5s")
		}
	})
	require.True(t, server.ReadyForConnections(5*time.Second))

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	client, closeClient, err := openClient(ctx, strings.Replace(server.ClientURL(), "nats://", "nats://edge:edge@", 1))
	require.NoError(t, err)
	t.Cleanup(func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		require.NoError(t, closeClient(closeCtx))
	})
	manager, err := NewConfigManager(identityTestConfig("acme", "denied"), client, nil)
	require.NoError(t, err)
	require.NoError(t, manager.Start(ctx))
	// Runs after the later Stop below has already returned nil, and returns nil again; it releases
	// the Manager if an assertion fails first. The bound covers the denied deletes' own timeout.
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		require.NoError(t, manager.Stop(stopCtx))
	})

	stopCtx, stopCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer stopCancel()
	started := time.Now()
	err = manager.Stop(stopCtx)
	elapsed := time.Since(started)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	// A 100 ms context with a 1 s ceiling: generous for scheduling, far below one delete's timeout.
	require.Less(t, elapsed, time.Second, "Stop waited %s past its 100ms context", elapsed)

	laterCtx, laterCancel := context.WithTimeout(ctx, 30*time.Second)
	defer laterCancel()
	require.NoError(t, manager.Stop(laterCtx), "a later Stop must join the outstanding watch teardown")
}
