//go:build integration

package objectstore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/service"
	"github.com/c360studio/semengine/types"
)

// The JetStream input the Component tests bind, and the durable consumer the binding plan names
// for it: objectstore-<instance>-<subject with dots as dashes> (pin legacyObjectStoreConsumerName),
// spelled out here rather than computed by the plan under test.
const (
	inputStream   = "OBJECTSTORE_WRITES"
	inputSubject  = "objectstore.test.write"
	inputConsumer = "objectstore-objectstore-objectstore-test-write"
	inputPort     = "js-write"
)

// coreConfig is a Component with one core-NATS input; jetStreamConfig one with a JetStream input
// on inputStream. Each stores into its own bucket.
const (
	coreConfig = `{"bucket_name": "COMPONENT_CORE", "ports": {"inputs": [` +
		`{"name": "write", "config": {"kind": "nats", "subject": "objectstore.test.core"}}], ` +
		`"outputs": [` + storedOutput + `]}}`
	jetStreamConfig = `{"bucket_name": "COMPONENT_JS", "ports": {"inputs": [` +
		`{"name": "` + inputPort + `", "config": {"kind": "jetstream", "stream_name": "` + inputStream +
		`", "subjects": ["` + inputSubject + `"]}}], "outputs": [` + storedOutput + `]}}`
)

func openSharedClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	return openTestClient(ctx, url, natsclient.WithMaxReconnects(0))
}

// openReconnectingClient connects a client that reconnects quickly, as a production client does,
// so a Component on it keeps running across a broker restart.
func openReconnectingClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	return openTestClient(ctx, url, natsclient.WithReconnectWait(50*time.Millisecond))
}

func openTestClient(ctx context.Context, url string, opt natsclient.ClientOption) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url, natsclient.WithTimeout(15*time.Second), natsclient.WithHealthInterval(0), opt)
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

// newStartable builds an initialized Component from raw on client through the factory, or
// returns the construction error.
func newStartable(client *natsclient.Client, raw string) (*Component, error) {
	reg, err := testRegistry()
	if err != nil {
		return nil, err
	}
	built, err := newComponent(json.RawMessage(raw), component.Dependencies{
		NATSClient:      client,
		PayloadRegistry: reg,
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		return nil, err
	}
	c := built.(*Component)
	return c, c.Initialize()
}

// startComponent builds and starts a Component, and returns it with the Stop the caller defers.
func startComponent(ctx context.Context, t *testing.T, client *natsclient.Client, raw string) (*Component, func()) {
	t.Helper()
	c, err := newStartable(client, raw)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return c, func() {
		stopCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		if err := c.Stop(stopCtx); err != nil {
			t.Errorf("Stop: %v", err)
		}
	}
}

func createInputStream(ctx context.Context, t *testing.T, js jetstream.JetStream, storage jetstream.StorageType) {
	t.Helper()
	if _, err := natsfixture.CreateStream(ctx, js, storage, inputStream, inputSubject); err != nil {
		t.Fatalf("CreateStream %s: %v", inputStream, err)
	}
}

func clientJetStream(t *testing.T, client *natsclient.Client) jetstream.JetStream {
	t.Helper()
	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	return js
}

// awaitHealth waits, up to 30 seconds, for done to accept the Component's Health. The input check
// that writes what Health reads runs every five seconds on its own goroutine; the wait orders the
// assertion after that write.
func awaitHealth(ctx context.Context, t *testing.T, c *Component, done func(component.HealthStatus) bool) (component.HealthStatus, error) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return probe.Await(waitCtx, func(context.Context) (component.HealthStatus, error) {
		return c.Health(), nil
	}, done)
}

// TestObjectStoreLifecycleSuite runs the lifecycle suite on a Component with a core-NATS input
// and on one with a JetStream input (design D5). Observe lists the input check while it runs, so
// the suite's "nothing retained after Stop" proves Stop joined it, and its "Start context end
// stops nothing" proves the check and the inputs outlive the Start context. The owner that must
// fail names a bucket NATS refuses, so Start fails at the Store before binding anything; a failure
// after binding is TestFailedStartReleasesBoundInputs.
func TestObjectStoreLifecycleSuite(t *testing.T) {
	for _, lane := range []struct {
		name, raw string
		stream    bool
	}{
		{"core", coreConfig, false},
		{"jetstream", jetStreamConfig, true},
	} {
		t.Run(lane.name, func(t *testing.T) {
			client := natsfixture.Open(t, SharedServer.Reset(t), openSharedClient)
			if lane.stream {
				createInputStream(t.Context(), t, clientJetStream(t, client), jetstream.FileStorage)
			}
			owner := func(raw string) lifecycletest.Factory {
				return func() lifecycletest.Owner {
					c, err := newStartable(client, raw)
					if err != nil {
						t.Errorf("build: %v", err) // the suite then reports a nil owner
						return nil
					}
					return suiteOwner{c}
				}
			}
			lifecycletest.Run(t, owner(lane.raw), owner(`{"bucket_name": "not.a.bucket"}`), lifecycletest.Promise{})
		})
	}
}

// TestFailedStartReleasesBoundInputs: a Start that binds a core-NATS input and then fails, because
// its JetStream input's stream does not exist and the Start context ends while waiting for it,
// returns that failure, holds nothing, and leaves the client with no subscription it made.
func TestFailedStartReleasesBoundInputs(t *testing.T) {
	client := natsfixture.Open(t, SharedServer.Reset(t), openSharedClient)
	// One JetStream request first, so the client's reply subscription, which nats.go keeps for the
	// connection's life, is in the baseline.
	if _, err := clientJetStream(t, client).AccountInfo(t.Context()); err != nil {
		t.Fatalf("AccountInfo: %v", err)
	}
	baseline := client.GetConnection().NumSubscriptions()

	c, err := newStartable(client, `{"bucket_name": "FAILED_START", "ports": {"inputs": [`+
		`{"name": "write", "config": {"kind": "nats", "subject": "objectstore.test.core"}}, `+
		`{"name": "late", "config": {"kind": "jetstream", "stream_name": "MISSING", "subjects": ["missing.write"]}}`+
		`], "outputs": [`+storedOutput+`]}}`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	startCtx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := c.Start(startCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Start = %v, want the Start context's deadline while waiting for stream MISSING", err)
	}
	if held := (suiteOwner{c}).Observe().Unresolved; len(held) != 0 {
		t.Fatalf("held after the failed Start = %v, want nothing", held)
	}
	if status := c.Health().Status; status != "stopped" {
		t.Fatalf("Health after the failed Start = %q, want stopped", status)
	}
	if got := client.GetConnection().NumSubscriptions(); got != baseline {
		t.Fatalf("client subscriptions after the failed Start = %d, want the %d it had before", got, baseline)
	}
	stopCtx, cancelStop := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancelStop()
	if err := c.Stop(stopCtx); err != nil {
		t.Fatalf("Stop after the failed Start: %v", err)
	}
}

// TestProvidedStoresUnderObjectstore: ProvidedStores offers nothing before Start, the Store under
// the one instance name objectstore while running, stamping that name, and nothing after Stop
// (object-store spec, "One objectstore store per process").
func TestProvidedStoresUnderObjectstore(t *testing.T) {
	client := natsfixture.Open(t, SharedServer.Reset(t), openSharedClient)
	c, err := newStartable(client, coreConfig)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got := c.ProvidedStores(); got != nil {
		t.Fatalf("ProvidedStores before Start = %v, want nil", got)
	}
	if err := c.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	provided := c.ProvidedStores()
	if keys := slices.Sorted(maps.Keys(provided)); !slices.Equal(keys, []string{"objectstore"}) {
		t.Fatalf("ProvidedStores keys = %v, want [objectstore]", keys)
	}
	store, ok := provided["objectstore"].(*Store)
	if !ok || store.InstanceName() != "objectstore" {
		t.Fatalf("provided store = %T, want a *Store stamping objectstore", provided["objectstore"])
	}
	if err := store.Put(t.Context(), "probe", []byte("body")); err != nil {
		t.Fatalf("Put through the provided store: %v", err)
	}
	if got, err := store.Get(t.Context(), "probe"); err != nil || string(got) != "body" {
		t.Fatalf("Get through the provided store = %q, %v; want body", got, err)
	}
	stopCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := c.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := c.ProvidedStores(); got != nil {
		t.Fatalf("ProvidedStores after Stop = %v, want nil", got)
	}
}

// TestSecondComponentRefusedAtStart: a component manager composing two objectstore Components
// starts the first and refuses the second at Start, naming the instance objectstore, because both
// offer their Store under that one name (design D7, owner ruling 3).
func TestSecondComponentRefusedAtStart(t *testing.T) {
	client := natsfixture.Open(t, SharedServer.Reset(t), openSharedClient)
	registry := component.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatalf("Register: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	storeOnly := json.RawMessage(`{"bucket_name": "TWO_COMPONENTS", "ports": {}}`)
	manager, err := config.NewConfigManager(&config.Config{
		Version:  "1.0.0",
		Platform: config.PlatformConfig{Org: "acme", ID: "objectstore-test", Type: "test"},
		Services: make(types.ServiceConfigs),
		Components: config.ComponentConfigs{
			"store-a": {Name: "objectstore", Type: types.ComponentTypeStorage, Enabled: true, Config: storeOnly},
			"store-b": {Name: "objectstore", Type: types.ComponentTypeStorage, Enabled: true, Config: storeOnly},
		},
	}, client, logger)
	if err != nil {
		t.Fatalf("config manager: %v", err)
	}
	cm, err := service.NewComponentManager(nil, &service.Dependencies{
		NATSClient:        client,
		MetricsRegistry:   metric.NewMetricsRegistry(),
		Logger:            logger,
		Manager:           manager,
		ComponentRegistry: registry,
	})
	if err != nil {
		t.Fatalf("component manager: %v", err)
	}
	startErr := cm.Start(t.Context())
	stopCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := cm.Stop(stopCtx); err != nil {
		t.Errorf("component manager Stop: %v", err)
	}
	if startErr == nil || !strings.Contains(startErr.Error(), `instance "objectstore"`) {
		t.Fatalf("component manager Start = %v, want the second Component refused naming instance \"objectstore\"", startErr)
	}
}

// TestLostJetStreamInputReportsUnhealthy is #216's ruling for this Component (design D2): with the
// Component running, its JetStream input's durable consumer is deleted, which ends the consume
// loop, and Health becomes unhealthy naming the input and the restart that recovers it.
func TestLostJetStreamInputReportsUnhealthy(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	client := natsfixture.Open(t, SharedServer.Reset(t), openSharedClient)
	js := clientJetStream(t, client)
	createInputStream(ctx, t, js, jetstream.FileStorage)
	c, stop := startComponent(ctx, t, client, jetStreamConfig)
	defer stop()
	if health := c.Health(); !health.Healthy {
		t.Fatalf("Health after Start = %+v, want healthy", health)
	}
	// nats.go ends a consume loop on the deletion notice the server sends to a waiting pull
	// request (jetstream/pull.go:740 at v1.54, read, not otherwise measured); a deletion that
	// lands before the loop's first pull reaches the server leaves the loop pulling, and only the
	// lookup reads the loss. The test waits for that pull, so the scenario is a running input
	// losing its consumer.
	pullCtx, cancelPull := context.WithTimeout(ctx, 10*time.Second)
	defer cancelPull()
	if _, err := probe.Await(pullCtx, func(ctx context.Context) (int, error) {
		consumer, err := js.Consumer(ctx, inputStream, inputConsumer)
		if err != nil {
			return 0, err
		}
		info, err := consumer.Info(ctx)
		if err != nil {
			return 0, err
		}
		return info.NumWaiting, nil
	}, func(waiting int) bool { return waiting > 0 }); err != nil {
		t.Fatalf("the input's consume loop never had a pull request waiting at the server: %v", err)
	}

	if err := js.DeleteConsumer(ctx, inputStream, inputConsumer); err != nil {
		t.Fatalf("DeleteConsumer %s: %v", inputConsumer, err)
	}
	health, err := awaitHealth(ctx, t, c, func(h component.HealthStatus) bool {
		return !h.Healthy && strings.Contains(h.LastError, `"`+inputPort+`"`) && strings.Contains(h.LastError, "restart")
	})
	if err != nil {
		t.Fatalf("Health must read the lost input as unhealthy, naming %q and the restart: %v", inputPort, err)
	}
	if health.Status != "degraded" {
		t.Fatalf("Health status = %q, want degraded", health.Status)
	}
}

// TestMemoryInputLostAtBrokerRestartStaysUnhealthy is the memory case of #216's ruling (design
// D2), as graph-ingest's TestIntegration_MemoryStreamLostAtBrokerRestartStaysUnhealthy: the
// memory stream and the input's consumer on it do not survive a broker restart, and the consume
// loop does not end, so the input check's lookup is what reads the loss. Once the stream is
// created again, Health is unhealthy naming the consumer, before and after a message is published
// to it, while the loop still runs.
func TestMemoryInputLostAtBrokerRestartStaysUnhealthy(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	// Own server: restarts the server mid-test.
	f := natsfixture.New(t)
	if err := f.Start(ctx); err != nil {
		t.Fatalf("fixture Start: %v", err)
	}
	createInputStream(ctx, t, f.JetStream(), jetstream.MemoryStorage)
	// The client reconnects, so the Component keeps running across the restart, as it does in
	// production. Restart's doc says a caller stops what it built on the URL first; this test
	// departs from that on purpose, and fails loudly below if Docker remaps the port.
	client := natsfixture.Open(t, f.URL(), openReconnectingClient)
	c, stop := startComponent(ctx, t, client, jetStreamConfig)
	defer stop()
	loopClosed := c.consumers[0].handle.Closed() // read after Start returned, before Stop

	before := f.URL()
	if err := f.Restart(ctx); err != nil {
		t.Fatalf("fixture Restart: %v", err)
	}
	if f.URL() != before {
		t.Fatalf("the broker's URL changed across Restart (%s to %s), so the client cannot reconnect", before, f.URL())
	}
	createInputStream(ctx, t, f.JetStream(), jetstream.MemoryStorage)
	// The Component's own client sees the stream again, so the check's next lookup reaches the
	// broker and fails on the consumer, not on the connection.
	js := clientJetStream(t, client)
	visibleCtx, cancelVisible := context.WithTimeout(ctx, 20*time.Second)
	defer cancelVisible()
	if _, err := probe.Await(visibleCtx, func(ctx context.Context) (bool, error) {
		_, err := js.Stream(ctx, inputStream)
		return err == nil, nil
	}, func(found bool) bool { return found }); err != nil {
		t.Fatalf("the Component's client must reach %s after the restart: %v", inputStream, err)
	}

	namesConsumer := func(h component.HealthStatus) bool {
		return !h.Healthy && strings.Contains(h.LastError, inputConsumer)
	}
	if _, err := awaitHealth(ctx, t, c, namesConsumer); err != nil {
		t.Fatalf("Health must read the lost consumer as unhealthy and name it: %v", err)
	}
	if err := client.PublishToStream(ctx, inputSubject, []byte("after-restart")); err != nil {
		t.Fatalf("publish after the restart: %v", err)
	}
	if _, err := awaitHealth(ctx, t, c, namesConsumer); err != nil {
		t.Fatalf("Health must stay unhealthy after a message is published to the new stream: %v", err)
	}
	// The open channel shows the loss was read by the lookup: the loop Start began is still
	// running, so the lost-binding record is not what made Health unhealthy.
	if isClosed(loopClosed) {
		t.Fatal("the consume loop ended across the restart; the scenario needs it still running")
	}
}
