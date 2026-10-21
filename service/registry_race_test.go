package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/health"
)

// The pin's mockService.RegisterMetrics is not carried: Service has no such method here and
// metric has no MetricsRegistrar. The pin's StopAllCleanup and ReverseOrderShutdown started each
// service by hand and called StopAll without StartAll, which stops no service here (design D2),
// so they compose through StartAll, which also creates component-manager from its constructor.

// mockService implements the Service interface for testing
type mockService struct {
	name    string
	started bool
	stopped bool
	healthy bool
	mu      sync.RWMutex
}

func newMockService(name string) *mockService {
	return &mockService{
		name:    name,
		healthy: true,
	}
}

func (m *mockService) Start(ctx context.Context) error {
	// Check for cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return fmt.Errorf("service %s already started", m.name)
	}
	m.started = true
	m.stopped = false
	return nil
}

func (m *mockService) Stop(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started {
		return fmt.Errorf("service %s not started", m.name)
	}
	m.started = false
	m.stopped = true
	return nil
}

func (m *mockService) IsHealthy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.healthy && m.started
}

func (m *mockService) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status()
}

// status is Status for a caller holding mu. The pin's GetStatus called Status while holding the
// read lock, a recursive RLock that deadlocks when a writer waits between the two.
func (m *mockService) status() Status {
	if m.started {
		return StatusRunning
	}
	return StatusStopped
}

func (m *mockService) GetStatus() Info {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Info{
		Name:   m.name,
		Status: m.status(),
	}
}

func (m *mockService) Name() string {
	return m.name
}

func (m *mockService) Health() health.Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.healthy {
		return health.NewUnhealthy(m.name, "Mock service is unhealthy")
	}

	if !m.started {
		return health.NewUnhealthy(m.name, "Mock service not started")
	}

	if m.stopped {
		return health.NewUnhealthy(m.name, "Mock service stopped")
	}

	return health.NewHealthy(m.name, "Mock service operating normally")
}

// shutdownTrackingService wraps mockService to track shutdown order
type shutdownTrackingService struct {
	*mockService
	shutdownCallback func()
}

func (s *shutdownTrackingService) Stop(ctx context.Context) error {
	s.shutdownCallback()
	return s.mockService.Stop(ctx)
}

func (m *mockService) SetHealthy(healthy bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = healthy
}

func (m *mockService) IsStarted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}

func (m *mockService) IsStopped() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stopped
}

// TestServiceManager_ConcurrentOperations tests concurrent registry operations. Every name
// registered concurrently is offered afterwards; the pin counted Registry.Constructors, which is
// dropped (design D1).
func TestServiceManager_ConcurrentOperations(t *testing.T) {
	registry := NewServiceRegistry()

	var wg sync.WaitGroup
	const numGoroutines = 50
	const servicesPerGoroutine = 10

	// Register factories concurrently
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range servicesPerGoroutine {
				factoryName := fmt.Sprintf("factory-%d-%d", id, j)
				constructor := func(_ json.RawMessage, _ *Dependencies) (Service, error) {
					return newMockService(factoryName), nil
				}
				assert.NoError(t, registry.Register(factoryName, constructor))
			}
		}(i)
	}

	wg.Wait()

	for i := range numGoroutines {
		for j := range servicesPerGoroutine {
			name := fmt.Sprintf("factory-%d-%d", i, j)
			_, exists := registry.Constructor(name)
			assert.True(t, exists, "constructor %s was not registered", name)
		}
	}
}

// TestServiceManager_ServiceLifecycle tests service creation, start, and stop
func TestServiceManager_ServiceLifecycle(t *testing.T) {
	registry := NewServiceRegistry()
	manager := NewServiceManager(registry)

	// Register test constructor
	err := registry.Register("test-service", func(_ json.RawMessage, _ *Dependencies) (Service, error) {
		return newMockService("test-service"), nil
	})
	require.NoError(t, err)

	// Create service
	service, err := manager.CreateService("test-service", json.RawMessage(`{}`), &Dependencies{})
	require.NoError(t, err)

	mockSvc := service.(*mockService)
	assert.False(t, mockSvc.IsStarted(), "Service should not be started initially")

	// Start service
	err = service.Start(t.Context())
	require.NoError(t, err)
	assert.True(t, mockSvc.IsStarted(), "Service should be started")
	assert.True(t, service.IsHealthy(), "Service should be healthy")

	// Stop service
	err = service.Stop(stopContext(t))
	require.NoError(t, err)
	assert.True(t, mockSvc.IsStopped(), "Service should be stopped")
	assert.False(t, service.IsHealthy(), "Service should not be healthy after stop")
}

// TestServiceManager_ErrorHandling tests error scenarios. The pin's GetServiceStatus check is not
// carried: that method is dropped (design D1).
func TestServiceManager_ErrorHandling(t *testing.T) {
	registry := NewServiceRegistry()
	manager := NewServiceManager(registry)

	// Test duplicate constructor registration
	constructor := func(_ json.RawMessage, _ *Dependencies) (Service, error) {
		return newMockService("test"), nil
	}

	err := registry.Register("test-constructor", constructor)
	assert.NoError(t, err)

	err = registry.Register("test-constructor", constructor)
	assert.Error(t, err, "Expected error for duplicate constructor registration")

	// Test creating service with non-existent constructor
	_, err = manager.CreateService("non-existent", json.RawMessage(`{}`), &Dependencies{})
	assert.Error(t, err, "Expected error for non-existent constructor")

	// Test getting non-existent service
	_, exists := manager.GetService("non-existent")
	assert.False(t, exists, "Expected false for non-existent service")
}

// registerComponentManagerConstructor registers the constructor StartAll uses to create the
// mandatory component-manager.
func registerComponentManagerConstructor(t *testing.T, registry *Registry) {
	t.Helper()
	require.NoError(t, registry.Register("component-manager", func(_ json.RawMessage, _ *Dependencies) (Service, error) {
		return newMockService("component-manager"), nil
	}))
}

// createMockServices registers and creates one mockService per name, in order.
func createMockServices(t *testing.T, registry *Registry, manager *Manager, names []string) []*mockService {
	t.Helper()
	created := make([]*mockService, 0, len(names))
	for _, name := range names {
		require.NoError(t, registry.Register(name, func(_ json.RawMessage, _ *Dependencies) (Service, error) {
			return newMockService(name), nil
		}))
		service, err := manager.CreateService(name, json.RawMessage(`{}`), &Dependencies{})
		require.NoError(t, err)
		mock, ok := service.(*mockService)
		require.True(t, ok, "CreateService returned %T", service)
		created = append(created, mock)
	}
	return created
}

// TestServiceManager_StopAllCleanup: StopAll stops every service StartAll started, and the
// manager then holds none (pin registry_race_test.go:204, which read the dropped GetAllServices).
func TestServiceManager_StopAllCleanup(t *testing.T) {
	registry := NewServiceRegistry()
	manager := NewServiceManager(registry)
	registerComponentManagerConstructor(t, registry)
	names := []string{"service-1", "service-2", "service-3", "service-4", "service-5"}
	services := createMockServices(t, registry, manager, names)

	require.NoError(t, manager.StartAll(t.Context()))
	for _, service := range services {
		assert.True(t, service.IsHealthy(), "service %s is not healthy after StartAll", service.Name())
	}

	require.NoError(t, manager.StopAll(stopContext(t)))
	for _, service := range services {
		assert.True(t, service.IsStopped(), "service %s was not stopped", service.Name())
		assert.False(t, service.IsHealthy(), "service %s is healthy after StopAll", service.Name())
		_, found := manager.GetService(service.Name())
		assert.False(t, found, "the manager still holds %s after StopAll", service.Name())
	}
}

// TestServiceManager_ReverseOrderShutdown: StopAll stops services in reverse registration order
// (pin :247).
func TestServiceManager_ReverseOrderShutdown(t *testing.T) {
	registry := NewServiceRegistry()
	manager := NewServiceManager(registry)
	registerComponentManagerConstructor(t, registry)

	var shutdownOrder []string
	var mu sync.Mutex
	for _, name := range []string{"first", "second", "third", "fourth"} {
		require.NoError(t, registry.Register(name, func(_ json.RawMessage, _ *Dependencies) (Service, error) {
			return &shutdownTrackingService{
				mockService: newMockService(name),
				shutdownCallback: func() {
					mu.Lock()
					shutdownOrder = append(shutdownOrder, name)
					mu.Unlock()
				},
			}, nil
		}))
		_, err := manager.CreateService(name, json.RawMessage(`{}`), &Dependencies{})
		require.NoError(t, err)
	}

	require.NoError(t, manager.StartAll(t.Context()))
	require.NoError(t, manager.StopAll(stopContext(t)))
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"fourth", "third", "second", "first"}, shutdownOrder,
		"services did not stop in reverse registration order")
}

// TestServiceManager_ConcurrentStartStop: services read concurrently while StartAll runs; once it
// returned, ending its context stops nothing, and only StopAll stops the services (design D2,
// ADR-113; pin :289, which counted the dropped GetHealthyServices and read GetAllServices).
//
// spec: service-lifecycle / The service manager's StartAll bounds StartAll
func TestServiceManager_ConcurrentStartStop(t *testing.T) {
	registry := NewServiceRegistry()
	manager := NewServiceManager(registry)
	registerComponentManagerConstructor(t, registry)
	const numServices = 20
	names := make([]string, numServices)
	for i := range names {
		names[i] = fmt.Sprintf("service-%d", i)
	}
	services := createMockServices(t, registry, manager, names)

	startCtx, endStart := context.WithCancel(t.Context())
	defer endStart()
	var wg sync.WaitGroup
	startErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		startErr <- manager.StartAll(startCtx)
	}()
	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if service, exists := manager.GetService(fmt.Sprintf("service-%d", id)); exists {
				_ = service.IsHealthy()
				_ = service.Status()
			}
		}(i)
	}
	wg.Wait()
	require.NoError(t, <-startErr)

	endStart()
	for _, service := range services {
		assert.True(t, service.IsHealthy(), "service %s is not healthy after StartAll's context ended", service.Name())
		assert.False(t, service.IsStopped(), "ending StartAll's context stopped %s", service.Name())
	}

	require.NoError(t, manager.StopAll(stopContext(t)))
	for _, service := range services {
		assert.True(t, service.IsStopped(), "StopAll did not stop %s", service.Name())
		_, found := manager.GetService(service.Name())
		assert.False(t, found, "the manager still holds %s after StopAll", service.Name())
	}
}
