// Package service provides base functionality and common patterns for
// long-running services in SemEngine. It includes health monitoring, lifecycle
// management, and metric collection capabilities.
package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semengine/health"
	"github.com/c360studio/semengine/internal/lifecycleguard"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
)

// healthCheckInterval is how often a started BaseService runs its health check
// after the first, which runs at Start. No production caller set another.
const healthCheckInterval = 30 * time.Second

// Status represents the current status of a service
type Status int

// Possible service statuses
const (
	StatusStopped Status = iota
	StatusStarting
	StatusRunning
	StatusStopping
)

// String returns the string representation of Status
func (s Status) String() string {
	switch s {
	case StatusStopped:
		return "stopped"
	case StatusStarting:
		return "starting"
	case StatusRunning:
		return "running"
	case StatusStopping:
		return "stopping"
	default:
		return "unknown"
	}
}

// Info holds runtime information for a service
type Info struct {
	Name               string        `json:"name"`
	Status             Status        `json:"status"`
	Uptime             time.Duration `json:"uptime"`
	StartTime          time.Time     `json:"start_time"`
	MessagesProcessed  int64         `json:"messages_processed"`
	LastActivity       time.Time     `json:"last_activity"`
	HealthChecks       int64         `json:"health_checks"`
	FailedHealthChecks int64         `json:"failed_health_checks"`
}

// HealthCheckFunc defines a custom health check function
type HealthCheckFunc func() error

// Option is a functional option for configuring BaseService
type Option func(*BaseService)

// BaseService provides common functionality for all services
type BaseService struct {
	name            string
	nats            *natsclient.Client
	metricsRegistry *metric.MetricsRegistry
	logger          *slog.Logger // Structured logger for the service

	status    atomic.Value // Status
	startTime atomic.Value // time.Time
	healthy   atomic.Bool

	// Metrics
	messagesProcessed  atomic.Int64
	healthChecks       atomic.Int64
	failedHealthChecks atomic.Int64
	lastActivity       atomic.Value // time.Time

	// lifecycle admits one Start and makes Stop the only stop (ADR-113).
	lifecycle lifecycleguard.Guard

	// mu guards the fields below it.
	mu              sync.RWMutex
	healthCheckFunc HealthCheckFunc
	// cancel ends the run context Start creates for the health goroutine
	// (ADR-113). Only the cancel is kept, never the context.
	cancel context.CancelFunc
	// healthDone is closed when the health goroutine returns; Stop waits on it.
	healthDone chan struct{}
}

// NewBaseServiceWithOptions creates a new base service using functional options pattern
func NewBaseServiceWithOptions(name string, opts ...Option) *BaseService {
	service := &BaseService{
		name:   name,
		logger: slog.Default().With("service", name), // Default logger with service name
	}

	// Apply options (can override the default logger)
	for _, opt := range opts {
		opt(service)
	}

	// Initialize status and metrics
	service.setStatus(StatusStopped)
	service.startTime.Store(time.Time{})
	service.lastActivity.Store(time.Time{})

	return service
}

// WithNATS sets the NATS client for the service
func WithNATS(client *natsclient.Client) Option {
	return func(s *BaseService) {
		s.nats = client
	}
}

// WithMetrics sets the metrics registry for the service
func WithMetrics(registry *metric.MetricsRegistry) Option {
	return func(s *BaseService) {
		s.metricsRegistry = registry
	}
}

// WithLogger sets a custom logger for the service
func WithLogger(logger *slog.Logger) Option {
	return func(s *BaseService) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// Name returns the service name
func (s *BaseService) Name() string {
	return s.name
}

// Status returns the current service status
func (s *BaseService) Status() Status {
	return s.status.Load().(Status)
}

// IsHealthy returns whether the service is healthy
func (s *BaseService) IsHealthy() bool {
	return s.healthy.Load()
}

// Health returns the standard health status for the service
func (s *BaseService) Health() health.Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if unhealthy
	if !s.healthy.Load() {
		// BaseService doesn't track specific errors, just unhealthy state
		// Services that embed BaseService can override Health() for more detail
		failedChecks := s.failedHealthChecks.Load()
		message := fmt.Sprintf("Service is unhealthy (failed checks: %d)", failedChecks)
		return health.NewUnhealthy(s.name, message)
	}

	// Check lifecycle state for degraded conditions
	status := s.Status()
	switch status {
	case StatusRunning:
		return health.NewHealthy(s.name, "Service operating normally")
	case StatusStarting:
		return health.NewDegraded(s.name, "Service is starting")
	case StatusStopping:
		return health.NewDegraded(s.name, "Service is stopping")
	case StatusStopped:
		return health.NewUnhealthy(s.name, "Service is stopped")
	default:
		return health.NewUnhealthy(s.name, fmt.Sprintf("Unknown status: %v", status))
	}
}

// Start starts the service: it reports StatusRunning and runs the health check
// at once and then every 30 seconds. It refuses a nil or ended ctx, a second
// Start, and a Start after any Stop (errs.ErrAlreadyStarted); a refused Start
// starts nothing.
//
// Ending ctx after Start returned stops nothing: the health check runs under a
// run context Start creates and Stop ends (ADR-113). Stop is the only stop.
func (s *BaseService) Start(ctx context.Context) error {
	return s.lifecycle.Start(ctx, s.start, s.stop)
}

// start is Start's work, run by the lifecycle guard. It cannot fail.
func (s *BaseService) start(ctx context.Context) error {
	s.setStatus(StatusStarting)

	// The run context: Start's values without its cancellation, ended by Stop.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})
	s.mu.Lock()
	s.cancel, s.healthDone = cancel, done
	s.mu.Unlock()

	startTime := time.Now()
	s.startTime.Store(startTime)
	s.lastActivity.Store(startTime)
	s.setStatus(StatusRunning)

	go s.healthMonitor(runCtx, done)
	return nil
}

// Stop stops the service. It refuses a nil ctx, and given a ctx that has
// already ended it returns that context's error and stops nothing. Otherwise it
// reports StatusStopping, starts no further health check, and returns nil once
// the health goroutine has exited, leaving StatusStopped. If ctx ends while it
// waits it returns that error, and a later Stop waits again. Stop before Start,
// and a Stop after one that returned nil, return nil.
func (s *BaseService) Stop(ctx context.Context) error {
	return s.lifecycle.Stop(ctx)
}

// stop is the cleanup the lifecycle guard runs for Stop. It is safe to repeat,
// so a Stop whose context ended while it waited can run it again.
func (s *BaseService) stop(ctx context.Context) error {
	s.mu.Lock()
	cancel, done := s.cancel, s.healthDone
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}

	s.setStatus(StatusStopping)
	cancel()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	s.mu.Lock()
	s.cancel, s.healthDone = nil, nil
	s.mu.Unlock()
	s.healthy.Store(false)
	s.setStatus(StatusStopped)
	return nil
}

// SetHealthCheck sets a custom health check function
func (s *BaseService) SetHealthCheck(fn HealthCheckFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthCheckFunc = fn
}

// GetStatus returns the current service information
func (s *BaseService) GetStatus() Info {
	startTime := s.startTime.Load().(time.Time)
	lastActivity := s.lastActivity.Load().(time.Time)

	uptime := time.Duration(0)
	if !startTime.IsZero() && s.Status() == StatusRunning {
		uptime = time.Since(startTime)
	}

	return Info{
		Name:               s.name,
		Status:             s.Status(),
		Uptime:             uptime,
		StartTime:          startTime,
		MessagesProcessed:  s.messagesProcessed.Load(),
		LastActivity:       lastActivity,
		HealthChecks:       s.healthChecks.Load(),
		FailedHealthChecks: s.failedHealthChecks.Load(),
	}
}

// setStatus stores status and records it on the injected metrics registry.
func (s *BaseService) setStatus(status Status) {
	s.status.Store(status)
	if s.metricsRegistry != nil {
		s.metricsRegistry.CoreMetrics().RecordServiceStatus(s.name, int(status))
	}
}

// healthMonitor runs the health check at once and then on every tick, until
// ctx ends. It closes done when it returns.
func (s *BaseService) healthMonitor(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	s.performHealthCheck()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.performHealthCheck()
		}
	}
}

// performHealthCheck executes the health check
func (s *BaseService) performHealthCheck() {
	if s.Status() != StatusRunning {
		return
	}
	s.healthChecks.Add(1)

	var err error
	s.mu.RLock()
	healthCheck := s.healthCheckFunc
	s.mu.RUnlock()

	// Custom health check has priority
	if healthCheck != nil {
		err = healthCheck()
	}

	// Default health checks (only if no custom health check or custom passed)
	if err == nil && s.nats != nil && !s.nats.IsHealthy() {
		err = natsclient.ErrNotConnected
	}

	if err != nil {
		s.failedHealthChecks.Add(1)
	}
	// A check that finishes after the stop began does not report health.
	if s.Status() != StatusRunning {
		return
	}
	s.healthy.Store(err == nil)
}

// Service interface defines the contract for all services
type Service interface {
	Name() string
	Start(ctx context.Context) error
	// Stop stops the service. It returns nil only once the service has
	// released everything it started; if ctx ends first it returns ctx's
	// error and a later Stop tries again. A Stop after one that returned nil
	// returns nil.
	Stop(ctx context.Context) error
	Status() Status
	IsHealthy() bool       // Keep for compatibility during migration
	GetStatus() Info       // Keep for compatibility during migration
	Health() health.Status // NEW: Standard health reporting
}
