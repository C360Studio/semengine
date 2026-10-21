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

// The pin's TestServiceManager_StopAllCleanup, TestServiceManager_ReverseOrderShutdown and
// TestServiceManager_ConcurrentStartStop drive StartAll and StopAll and move with the manager's
// tests (task 2.1, part 3). The pin's mockService.RegisterMetrics is not carried: Service has no
// such method here and metric has no MetricsRegistrar.

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
