//go:build integration

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/types"
)

// Boot through the service manager and a real component manager on a live server (pin
// framework_owned_bucket_guards_integration_test.go; design D3, D7): a framework-owned bucket a
// rival created dirty during this boot is reconciled inside its owner's Start, and a component
// whose Start fails fails the boot, naming it. The pin's recording log handler is not carried:
// no assertion read what it recorded.

// guardsBucketAdopter is a lifecycle component whose Start reproduces the create race inside
// the boot: a rival's raw create of bucket (a foreign 7-day TTL and one stored key, no
// reconcile), then the owner's acquisition through kvcatalog.EnsureCatalogBucket, which must
// reconcile the bucket it adopts. It records the TTL the bucket carried after the rival's create,
// proving the dirt existed mid-boot; nothing after Start strips it.
type guardsBucketAdopter struct {
	baseDiscoverable
	client *natsclient.Client
	bucket string

	mu       sync.Mutex
	dirtyTTL time.Duration
}

func (c *guardsBucketAdopter) Initialize() error { return nil }

func (c *guardsBucketAdopter) Start(ctx context.Context) error {
	kv, err := c.client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket: c.bucket,
		TTL:    7 * 24 * time.Hour,
	})
	if err != nil {
		return err
	}
	if _, err := kv.Put(ctx, "entity.key.one", []byte("survivor")); err != nil {
		return err
	}
	dirtyTTL, _, err := natsclient.BucketRetention(ctx, kv)
	if err != nil {
		return err
	}
	if _, err := kvcatalog.EnsureCatalogBucket(ctx, c.client, c.bucket); err != nil {
		return err
	}
	c.mu.Lock()
	c.dirtyTTL = dirtyTTL
	c.mu.Unlock()
	return nil
}

func (c *guardsBucketAdopter) Stop(context.Context) error { return nil }

func (c *guardsBucketAdopter) dirtyTTLBeforeSeam() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dirtyTTL
}

var _ component.LifecycleComponent = (*guardsBucketAdopter)(nil)

// guardsFailingStart is a lifecycle component whose Start always fails with
// errGuardsStartFailure.
type guardsFailingStart struct{ baseDiscoverable }

func (guardsFailingStart) Initialize() error           { return nil }
func (guardsFailingStart) Start(context.Context) error { return errGuardsStartFailure }
func (guardsFailingStart) Stop(context.Context) error  { return nil }

var _ component.LifecycleComponent = guardsFailingStart{}

var errGuardsStartFailure = errors.New("simulated component start failure")

// newGuardsComponentManager builds a component manager through NewComponentManager over a
// config.Manager holding components, each built by registry's factory of its configured name, as
// the service manager's composition root does at boot.
func newGuardsComponentManager(
	t *testing.T, client *natsclient.Client, registry *component.Registry, components config.ComponentConfigs,
) *ComponentManager {
	t.Helper()
	deps := createTestServiceDependencies()
	configManager, err := config.NewConfigManager(&config.Config{
		Version:    "1.0.0",
		Platform:   config.PlatformConfig{Org: "test", ID: "guards-test", Type: "test"},
		Services:   make(types.ServiceConfigs),
		Components: components,
	}, client, deps.Logger)
	require.NoError(t, err)
	deps.NATSClient = client
	deps.Manager = configManager
	deps.ComponentRegistry = registry
	built, err := NewComponentManager(json.RawMessage(`{}`), deps)
	require.NoError(t, err)
	cm, ok := built.(*ComponentManager)
	require.True(t, ok, "NewComponentManager returned %T, want *ComponentManager", built)
	return cm
}

// guardsProcessor is the enabled processor instance a guards test configures, built by factory.
func guardsProcessor(factory string) types.ComponentConfig {
	return types.ComponentConfig{
		Type: types.ComponentTypeProcessor, Name: factory, Enabled: true, Config: json.RawMessage(`{}`),
	}
}

// TestIntegration_StartAll_OwnerSeamReconcilesCreateRaceDirtInsideBoot: through the service
// manager's StartAll and the component manager's launch, a framework-owned bucket that did not
// exist before boot and carried a foreign TTL mid-boot is clean, with its stored key, when
// StartAll returns. Only the owner's Start can have cleaned it: StartAll runs no sweep.
func TestIntegration_StartAll_OwnerSeamReconcilesCreateRaceDirtInsideBoot(t *testing.T) {
	client := fixtureClient(t)

	var adopter *guardsBucketAdopter
	registry := component.NewRegistry()
	require.NoError(t, registry.Register(component.Registration{
		Name: "bucket-adopter", Type: types.ComponentTypeProcessor,
		// NewComponentManager runs the factory on the test goroutine (design D3).
		Factory: func(_ json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
			adopter = &guardsBucketAdopter{client: deps.NATSClient, bucket: graph.BucketEmbeddingIndex}
			return adopter, nil
		},
		Ports: func(json.RawMessage, string) (component.PortConfig, error) {
			return component.PortConfig{}, nil
		},
	}))
	cm := newGuardsComponentManager(t, client, registry, config.ComponentConfigs{
		"embedding-index-adopter": guardsProcessor("bucket-adopter"),
	})
	require.NotNil(t, adopter, "the factory did not build the adopting component")

	deps := createTestServiceDependencies()
	deps.NATSClient = client
	manager := createTestServiceManager(ManagerConfig{}, deps)
	require.NoError(t, manager.RegisterInstance("component-manager", cm))

	_, err := client.GetKeyValueBucket(t.Context(), graph.BucketEmbeddingIndex)
	require.ErrorIs(t, err, jetstream.ErrBucketNotFound,
		"precondition: the guarded bucket must not exist before StartAll")

	startTestManager(t, manager)

	status := cm.GetComponentStatus()
	require.Contains(t, status, "embedding-index-adopter")
	require.Equal(t, component.StateStarted, status["embedding-index-adopter"].State,
		"the adopting component did not start through the component manager")
	require.Equal(t, 7*24*time.Hour, adopter.dirtyTTLBeforeSeam(),
		"the bucket did not carry the foreign TTL mid-boot, before the owner's acquisition")

	fresh, err := client.GetKeyValueBucket(t.Context(), graph.BucketEmbeddingIndex)
	require.NoError(t, err)
	maxAge, maxBytes, err := natsclient.BucketRetention(t.Context(), fresh)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), maxAge,
		"the owner's acquisition must strip the create-race TTL before StartAll returns")
	assert.LessOrEqual(t, maxBytes, int64(0), "the owner's acquisition must leave MaxBytes non-binding")

	entry, err := fresh.Get(t.Context(), "entity.key.one")
	require.NoError(t, err, "the stored key must survive the strip")
	assert.Equal(t, []byte("survivor"), entry.Value())
}

// TestIntegration_StartAll_BootFailsClosedOnComponentStartFailure: a component whose Start fails
// fails StartAll with an error that names the component and wraps its error. While StartAll
// rolls back (held in the Stop of a service started before the component manager), /services on
// the listener bound before any component started reports startup failed with one failed service
// and one failed component. After StartAll returns, the component is stopped with no error left
// on it and the listener is released. The pin read these from private fields (currentStartupSnapshot,
// httpServer, httpUsed, httpTerminal); PR 2 dropped the last three.
//
// spec: service-lifecycle / The component manager's Start bounds Start
func TestIntegration_StartAll_BootFailsClosedOnComponentStartFailure(t *testing.T) {
	client := fixtureClient(t)

	registry := component.NewRegistry()
	admitTestRegistryComponent(t, registry, "failing-start", guardsFailingStart{})
	cm := newGuardsComponentManager(t, client, registry, config.ComponentConfigs{
		"doomed-component": guardsProcessor("failing-start"),
	})

	release := newReleaser(t)
	held := make(chan struct{})
	var heldOnce sync.Once
	holder := &failedStartRollbackService{
		MockService: MockService{name: "rollback-holder", status: StatusRunning, healthy: true},
		start:       func(context.Context) error { return nil },
		stop: func(ctx context.Context) error {
			heldOnce.Do(func() { close(held) })
			select {
			case <-release.ch:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}

	deps := createTestServiceDependencies()
	deps.NATSClient = client
	manager := createTestServiceManager(ManagerConfig{}, deps)
	// Registered first, so it starts before the component manager and is stopped after it.
	require.NoError(t, manager.RegisterInstance("rollback-holder", holder))
	require.NoError(t, manager.RegisterInstance("component-manager", cm))

	start := beginObservedManagerStart(t, manager, release.release)
	start.requireSignal(t, held, "the rollback's stop of rollback-holder")
	addr := managerAddr(t, manager)
	response := getStartupCounts(t, "http://"+addr)
	assert.Equal(t, "failed", response.Startup.Status)
	assert.Equal(t, 1, response.Startup.Services.StartsFailed)
	assert.Equal(t, 1, response.Startup.Components.StartsFailed)
	requireAnswer(t, "http://"+addr+"/readyz", http.StatusServiceUnavailable, "NOT READY")

	release.release()
	err := start.wait(t)
	require.Error(t, err, "a component Start failure must fail StartAll")
	assert.ErrorContains(t, err, "doomed-component", "the boot error must name the failed component")
	assert.ErrorIs(t, err, errGuardsStartFailure, "the boot error must wrap the component's own Start error")

	status := cm.GetComponentStatus()
	require.Contains(t, status, "doomed-component")
	assert.Equal(t, component.StateStopped, status["doomed-component"].State)
	assert.NoError(t, status["doomed-component"].LastError)

	manager.mu.RLock()
	listener := manager.httpListener
	manager.mu.RUnlock()
	assert.Nil(t, listener, "a failed boot must release the main listener")
	requireListenerReleased(t, addr)
}
