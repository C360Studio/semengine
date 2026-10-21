package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/c360studio/semengine/internal/lifecycleguard"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/security"
)

// Metrics is a service that provides the Prometheus metrics endpoint. On its
// own it serves a metric.Server on its configured port between Start and Stop.
// Composed by the service manager, the manager claims that server before any
// service starts, serves it around every service, and reports whether it
// serves; Metrics is then healthy while it does.
type Metrics struct {
	*BaseService

	config   MetricsConfig
	registry *metric.MetricsRegistry
	security security.Config

	// lifecycle admits one Start and makes Stop the only stop (ADR-113).
	lifecycle lifecycleguard.Guard

	// mu guards the fields below it.
	mu sync.Mutex
	// server is the server Metrics started on its own, set from its bind until
	// a Stop released it. It stays nil when the manager claimed the server.
	server *metric.Server
	// lifecycleCalled is set once Start runs or Stop is called, after which the
	// manager's claim is refused.
	lifecycleCalled bool
	managerClaimed  bool
	managerHealthy  bool

	// testListener, when set by a test, is the listener the server serves on:
	// metric.NewServer maps port 0 to 9090, so no configuration asks for a
	// kernel-picked port (setup-04a-03-boot-service-manager design D4, S9).
	testListener net.Listener
}

// MetricsConfig holds configuration for the metrics service
// Simple struct - no UnmarshalJSON, no Enabled field
type MetricsConfig struct {
	Port int    `json:"port"`
	Path string `json:"path"`
}

// Validate checks if the configuration is valid
func (c MetricsConfig) Validate() error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	if c.Path == "" {
		return fmt.Errorf("metrics path cannot be empty")
	}
	return nil
}

// NewMetrics creates the metrics service from its JSON configuration: port
// (default 9090) and path (default "/metrics"). A key the configuration does not
// define, or one in another letter case, is refused. The server uses the
// platform TLS settings from deps.Manager and logs through deps.Logger.
func NewMetrics(rawConfig json.RawMessage, deps *Dependencies) (Service, error) {
	var cfg MetricsConfig
	if err := decodeStrictServiceJSON(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parse metrics config: %w", err)
	}

	if cfg.Port == 0 {
		cfg.Port = 9090
	}
	if cfg.Path == "" {
		cfg.Path = "/metrics"
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate metrics config: %w", err)
	}

	var securityCfg security.Config
	if deps.Manager != nil {
		if fullConfig := deps.Manager.GetConfig(); fullConfig != nil {
			securityCfg = fullConfig.Get().Security
		}
	}

	m := &Metrics{
		BaseService: NewBaseServiceWithOptions(
			"metrics",
			WithLogger(deps.Logger),
			WithMetrics(deps.MetricsRegistry),
		),
		config:   cfg,
		registry: deps.MetricsRegistry,
		security: securityCfg,
	}
	m.SetHealthCheck(m.healthCheck)
	return m, nil
}

// Start starts the metrics HTTP server, unless the manager claimed it, and then
// the base service. A bind failure is returned and nothing is held. ctx bounds
// Start only: ending it after Start returned stops nothing. A second Start, and
// a Start after any Stop, are refused with errs.ErrAlreadyStarted.
func (m *Metrics) Start(ctx context.Context) error {
	return m.lifecycle.Start(ctx, m.start, m.cleanup)
}

// start is Start's work, run by the lifecycle guard; a failure is rolled back
// through cleanup.
func (m *Metrics) start(ctx context.Context) error {
	m.mu.Lock()
	m.lifecycleCalled = true
	claimed := m.managerClaimed
	m.mu.Unlock()

	if !claimed {
		server := metric.NewServer(m.config.Port, m.config.Path, m.registry, m.security)
		m.logger.Info("Starting metrics server", "port", m.config.Port, "path", m.config.Path)
		if err := m.startServer(ctx, server); err != nil {
			return fmt.Errorf("start metrics server: %w", err)
		}
		m.mu.Lock()
		m.server = server
		m.mu.Unlock()
	}

	if err := m.BaseService.Start(ctx); err != nil {
		return fmt.Errorf("start metrics base service: %w", err)
	}
	// The base service runs its first check in its own goroutine; running one
	// here as well means health already reflects the server when Start returns.
	m.performHealthCheck()
	m.logger.Info("Metrics service started", "port", m.config.Port, "path", m.config.Path,
		"manager_owned", claimed)
	return nil
}

// Stop stops the metrics server Metrics started, then the base service. It
// returns nil only once both are released; if ctx ends first it returns ctx's
// error and a later Stop tries again. A Stop while another runs is refused as
// transient.
func (m *Metrics) Stop(ctx context.Context) error {
	if ctx != nil && ctx.Err() == nil {
		m.mu.Lock()
		m.lifecycleCalled = true
		m.mu.Unlock()
	}
	return m.lifecycle.Stop(ctx)
}

// cleanup is the one cleanup the lifecycle guard runs for a failed Start and for
// every Stop. A server that did not stop is kept, so the next Stop tries again.
func (m *Metrics) cleanup(ctx context.Context) error {
	m.mu.Lock()
	server := m.server
	m.mu.Unlock()

	var serverErr error
	if server != nil {
		if serverErr = server.Stop(ctx); serverErr != nil {
			m.logger.Error("Error stopping metrics server", "error", serverErr)
			serverErr = fmt.Errorf("stop metrics server: %w", serverErr)
		} else {
			m.mu.Lock()
			m.server = nil
			m.mu.Unlock()
		}
	}
	baseErr := m.BaseService.Stop(ctx)
	if err := errors.Join(serverErr, baseErr); err != nil {
		return err
	}
	m.logger.Info("Metrics service stopped")
	return nil
}

// healthCheck reports the server's state: the manager's report of the server it
// claimed, or whether Metrics holds the server it started.
func (m *Metrics) healthCheck() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.managerClaimed {
		if !m.managerHealthy {
			return fmt.Errorf("manager-owned metrics server not running")
		}
		return nil
	}
	if m.server == nil {
		return fmt.Errorf("metrics server not running")
	}
	return nil
}

// claimManagerServer hands the metrics server to the service manager, which
// serves it around every service. It is refused once Start or Stop has been
// called or a server was already claimed.
func (m *Metrics) claimManagerServer() (*metric.Server, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lifecycleCalled || m.managerClaimed || m.server != nil {
		return nil, errs.WrapFatal(
			errs.ErrAlreadyStarted,
			"Metrics", "claimManagerServer", "metrics runtime already owned",
		)
	}
	m.managerClaimed = true
	return metric.NewServer(m.config.Port, m.config.Path, m.registry, m.security), nil
}

// startServer starts server on the configured port, or on testListener when a
// test set one; the standalone and manager-owned servers start the same way.
func (m *Metrics) startServer(ctx context.Context, server *metric.Server) error {
	if m.testListener != nil {
		return server.StartWithListener(ctx, m.testListener)
	}
	return server.Start(ctx)
}

// setManagerServerHealthy records whether the server the manager claimed serves,
// and runs the health check at once when Metrics is running.
func (m *Metrics) setManagerServerHealthy(healthy bool) {
	m.mu.Lock()
	m.managerHealthy = healthy
	m.mu.Unlock()
	m.performHealthCheck()
}
