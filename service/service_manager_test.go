package service

import (
	"context"
	"log/slog"
	"net"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/health"
	"github.com/c360studio/semengine/metric"
)

// MockService is a Service whose status and health a test sets (pin service_manager_test.go:149).
// Its Start honours an ended context and its Stop does nothing.
type MockService struct {
	name    string
	status  Status
	healthy bool
}

func (m *MockService) Name() string { return m.name }

func (m *MockService) Start(ctx context.Context) error { return ctx.Err() }

func (m *MockService) Stop(context.Context) error { return nil }
func (m *MockService) Status() Status             { return m.status }
func (m *MockService) IsHealthy() bool            { return m.healthy }
func (m *MockService) GetStatus() Info            { return Info{Name: m.name, Status: m.status} }

func (m *MockService) Health() health.Status {
	if !m.healthy {
		return health.NewUnhealthy(m.name, "Mock service unhealthy")
	}
	switch m.status {
	case StatusRunning:
		return health.NewHealthy(m.name, "Mock service running")
	case StatusStarting:
		return health.NewDegraded(m.name, "Mock service starting")
	case StatusStopping:
		return health.NewDegraded(m.name, "Mock service stopping")
	default:
		return health.NewUnhealthy(m.name, "Mock service stopped")
	}
}

// newStandInComponentManager returns the plain Service named component-manager that every
// composition here holds until PR 3 registers the real constructor (design D3, "Tests"). It does
// not implement componentStartup, so the manager takes the pin's not-concrete path: no component
// routes, zero component counts, readiness from services.
func newStandInComponentManager() *MockService {
	return &MockService{name: "component-manager", status: StatusRunning, healthy: true}
}

// createTestServiceDependencies returns Dependencies with an Error-level logger and a fresh metrics
// registry. The pin's mockNATSClient argument is not carried: the pin set an empty
// natsclient.Client whatever the mock said, so no test read the mock.
func createTestServiceDependencies() *Dependencies {
	return &Dependencies{
		Logger:          slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		MetricsRegistry: metric.NewMetricsRegistry(),
	}
}

// createTestServiceManager returns a Manager whose configuration the test sets, as the pin's helper
// does (pin :607): port 0 binds a kernel-picked main port, which no configuration path offers.
// The Manager is not a Service (design D2, S11), so the pin's embedded BaseService goes; logger and
// natsClient are set from deps as ConfigureFromServices sets them.
func createTestServiceManager(config ManagerConfig, deps *Dependencies) *Manager {
	m := NewServiceManager(NewServiceRegistry())
	m.config = config
	m.dependencies = deps
	if deps != nil {
		m.logger = deps.Logger
		m.natsClient = deps.NATSClient
	}
	return m
}

// observedManagerHTTPURL returns the loopback URL of a manager's bound main listener (pin
// startup_observability_test.go:37). The listener field is set from the bind until it is released.
func observedManagerHTTPURL(t *testing.T, manager *Manager) string {
	t.Helper()
	manager.mu.RLock()
	listener := manager.httpListener
	manager.mu.RUnlock()
	require.NotNil(t, listener, "main HTTP listener not bound")
	addr, ok := listener.Addr().(*net.TCPAddr)
	require.True(t, ok, "main HTTP listener must have a TCP endpoint")
	return "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(addr.Port))
}

// startTestManager starts manager and stops it when the test ends under a bounded detached
// context.
func startTestManager(t *testing.T, manager *Manager) {
	t.Helper()
	require.NoError(t, manager.StartAll(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		require.NoError(t, manager.StopAll(ctx))
	})
}
