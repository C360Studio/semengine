package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/internal/httpadmission"
	"github.com/c360studio/semengine/internal/lifecycleguard"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/types"
)

// componentManagerName is the mandatory service every composition holds.
const componentManagerName = "component-manager"

// healthPublishInterval is how often the health publisher publishes each
// service's health after its first publish.
const healthPublishInterval = 5 * time.Second

// Manager composes services, starts them in registration order and stops them
// in reverse, and serves the HTTP surface around them: the main listener, an
// optional dedicated health listener, and the metrics server it claims from the
// metrics service. StartAll and StopAll are its lifecycle; the Manager is not
// itself a Service.
type Manager struct {
	registry *Registry

	// lifecycle admits one StartAll; StopAll, and a failed StartAll's rollback,
	// run stopAll through it (ADR-113).
	lifecycle lifecycleguard.Guard

	// committedRoutes is the full route set, stored when StartAll commits and
	// set to nil when a stop begins; the main listener serves it while set.
	committedRoutes atomic.Pointer[http.ServeMux]

	// mu guards the fields below it.
	mu       sync.RWMutex
	services map[string]Service
	order    []string // registration order: start order, and stop order reversed
	sealed   bool

	bootServiceConfigs types.ServiceConfigs
	sealedServices     []string
	serviceOutcomes    map[string]*serviceInvocationOutcome
	// stopping is set when a stop of the started services begins.
	stopping bool
	// stopCalled is set by the first StopAll given a live context: from then
	// /readyz answers NOT READY, even when that StopAll's context ends before
	// the stop runs and a start still in progress commits, and a later
	// StartHealthListener is refused.
	stopCalled bool

	config         ManagerConfig
	httpMiddleware []HTTPMiddleware // applied outermost-first; see ADR-030
	// httpStarted is set once StartAll built the main listener's handler; later
	// middleware is ignored.
	httpStarted bool

	// The main listener, set from its bind until a stop released it. Only tests
	// read httpListener, for the bound port (design D2, "Test-only code").
	httpServer    *http.Server
	httpListener  net.Listener
	httpServeDone chan struct{}
	httpRequests  *httpadmission.Requests

	// The dedicated health listener, set from its bind until a stop released it.
	healthServer    *http.Server
	healthServeDone chan struct{}
	healthRequests  *httpadmission.Requests
	healthUsed      bool

	// The health publisher: its run context's cancel and the channel closed when
	// its goroutine returns, set from StartAll until a stop joined it.
	healthPublisherCancel context.CancelFunc
	healthPublisherDone   chan struct{}

	// metricsServer is the server claimed from metricsService, set from the claim
	// until the manager released it.
	metricsServer  *metric.Server
	metricsService *Metrics

	logger        *slog.Logger
	natsClient    *natsclient.Client
	configManager *config.Manager
	dependencies  *Dependencies
}

// serviceInvocationOutcome is one admitted service's startup record.
type serviceInvocationOutcome struct {
	service        Service
	startInvoked   bool
	startCompleted bool
	startErr       error
}

// startupSnapshot is the startup state /services reports and /readyz reads.
type startupSnapshot struct {
	Status     string               `json:"status"`
	Services   serviceStartupCounts `json:"services"`
	Components startupUnitCounts    `json:"components"`
}

// admittedService is one composed service, by name.
type admittedService struct {
	name    string
	service Service
}

// NewServiceManager returns a Manager that constructs services from registry.
// It logs through slog's default logger until ConfigureFromServices is given a
// logger.
func NewServiceManager(registry *Registry) *Manager {
	return &Manager{
		registry: registry,
		services: make(map[string]Service),
		logger:   slog.Default(),
	}
}

func (m *Manager) bindConstructorDependencies(deps *Dependencies) *Dependencies {
	if deps == nil {
		return &Dependencies{ServiceManager: m}
	}
	bound := *deps
	bound.ServiceManager = m
	return &bound
}

// ConfigureFromServices configures the manager from the services configuration
// and constructs every enabled service but service-manager from its registered
// constructor. The service-manager entry configures the manager itself: http_port
// defaults to 8080, and the API document's title, description and version to
// "SemEngine API", "SemEngine framework API" and "0.0.0". component-manager and
// service-manager may not be disabled, and a composition whose component-manager
// has no registered constructor is refused. Nothing is composed unless every
// constructor succeeds. It is refused once StartAll sealed the composition.
func (m *Manager) ConfigureFromServices(services types.ServiceConfigs, deps *Dependencies) error {
	resolved, err := ResolveServiceConfigs(services)
	if err != nil {
		return fmt.Errorf("resolve service configs: %w", err)
	}
	for _, name := range []string{"service-manager", componentManagerName} {
		if !resolved[name].Enabled {
			return &MandatoryServiceDisabledError{Name: name}
		}
	}

	var cfg ManagerConfig
	if err := decodeStrictServiceJSON(resolved["service-manager"].Config, &cfg); err != nil {
		return fmt.Errorf("parse service-manager config: %w", err)
	}
	if cfg.HTTPPort == 0 {
		cfg.HTTPPort = 8080
	}
	if cfg.ServerInfo.Title == "" {
		cfg.ServerInfo.Title = "SemEngine API"
	}
	if cfg.ServerInfo.Description == "" {
		cfg.ServerInfo.Description = "SemEngine framework API"
	}
	if cfg.ServerInfo.Version == "" {
		cfg.ServerInfo.Version = "0.0.0"
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate service-manager config: %w", err)
	}

	names := make([]string, 0, len(resolved))
	for name := range resolved {
		names = append(names, name)
	}
	sort.Strings(names)

	// A configured identity may not alias a prebuilt instance. Check before any
	// constructor runs, so a collision leaves the manager and constructor
	// effects untouched.
	if err := m.checkConfigurable(names); err != nil {
		return err
	}

	constructed := make([]admittedService, 0, len(names))
	for _, name := range names {
		serviceConfig := resolved[name]
		if name == "service-manager" || !serviceConfig.Enabled {
			continue
		}
		constructor, exists := m.registry.Constructor(name)
		if !exists {
			return fmt.Errorf("create configured service %s: no constructor registered for service %s", name, name)
		}
		instance, err := constructor(serviceConfig.Config, m.bindConstructorDependencies(deps))
		if err != nil {
			return fmt.Errorf("create configured service %s: %w", name, err)
		}
		constructed = append(constructed, admittedService{name: name, service: instance})
	}

	// Commit the validated composition at once, checking again: another writer
	// may have sealed or composed the same name while the constructors ran.
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkConfigurableLocked(names); err != nil {
		return err
	}

	m.config = cfg
	m.dependencies = m.bindConstructorDependencies(deps)
	if deps != nil {
		if deps.Logger != nil {
			m.logger = deps.Logger
		}
		if deps.NATSClient != nil {
			m.natsClient = deps.NATSClient
		}
		if deps.Manager != nil {
			m.configManager = deps.Manager
		}
	}
	m.bootServiceConfigs = cloneResolvedServiceConfigs(resolved)
	for _, admitted := range constructed {
		m.services[admitted.name] = admitted.service
		m.order = append(m.order, admitted.name)
	}

	m.logger.Debug("Manager configured", "http_port", m.config.HTTPPort, "swagger_ui", m.config.SwaggerUI)
	return nil
}

func (m *Manager) checkConfigurable(names []string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.checkConfigurableLocked(names)
}

func (m *Manager) checkConfigurableLocked(names []string) error {
	if m.sealed {
		return &CompositionSealedError{Operation: "configure", Name: "service-manager"}
	}
	for _, name := range names {
		if name == "service-manager" {
			continue
		}
		if _, exists := m.services[name]; exists {
			return &DuplicateServiceError{Name: name}
		}
	}
	return nil
}

// CreateService constructs the service registered as name from rawConfig and
// composes it. It is refused for a name already composed and once StartAll
// sealed the composition; a refused call runs no constructor.
func (m *Manager) CreateService(name string, rawConfig json.RawMessage, deps *Dependencies) (Service, error) {
	m.mu.RLock()
	if m.sealed {
		m.mu.RUnlock()
		return nil, &CompositionSealedError{Operation: "create", Name: name}
	}
	if _, exists := m.services[name]; exists {
		m.mu.RUnlock()
		return nil, &DuplicateServiceError{Name: name}
	}
	m.mu.RUnlock()

	constructor, exists := m.registry.Constructor(name)
	if !exists {
		return nil, fmt.Errorf("no constructor registered for service %s", name)
	}

	service, err := constructor(rawConfig, m.bindConstructorDependencies(deps))
	if err != nil {
		return nil, fmt.Errorf("failed to create service %s: %w", name, err)
	}

	// Check again: another writer may have sealed the composition or composed
	// the same name while the constructor ran.
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sealed {
		return nil, &CompositionSealedError{Operation: "create", Name: name}
	}
	if _, exists := m.services[name]; exists {
		return nil, &DuplicateServiceError{Name: name}
	}
	m.services[name] = service
	m.order = append(m.order, name)
	return service, nil
}

// RegisterInstance composes a prebuilt service under name, as CreateService
// composes a constructed one. It is refused for a name already composed and once
// StartAll sealed the composition.
func (m *Manager) RegisterInstance(name string, svc Service) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sealed {
		return &CompositionSealedError{Operation: "register", Name: name}
	}
	if _, exists := m.services[name]; exists {
		return &DuplicateServiceError{Name: name}
	}
	m.services[name] = svc
	m.order = append(m.order, name)
	return nil
}

// GetService returns the service composed under name. After a StopAll that
// returned nil it finds nothing.
func (m *Manager) GetService(name string) (Service, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	service, exists := m.services[name]
	return service, exists
}

// StartAll starts the composition. It refuses a nil or ended context, a second
// StartAll and a StartAll after StopAll (errs.ErrAlreadyStarted); a refused
// StartAll starts nothing. Otherwise it creates component-manager from its
// registered constructor if no service has that name, seals the composition,
// binds the main listener (http_port) serving only the startup routes, starts
// the metrics server it claims from the metrics service, starts every service in
// registration order with ctx, builds the full route set, starts the health
// publisher, and commits the full route set. A failure is returned joined with
// the rollback's error: the rollback stops what started, and a rollback that
// fails is run again by StopAll.
//
// ctx bounds StartAll only: ending it after StartAll returned stops nothing, and
// no request's context derives from it. StopAll is the only stop.
func (m *Manager) StartAll(ctx context.Context) error {
	return m.lifecycle.Start(ctx, m.startAll, m.stopAll)
}

// startAll is StartAll's work, run by the lifecycle guard; a failure is rolled
// back through stopAll.
func (m *Manager) startAll(ctx context.Context) error {
	if err := m.createComponentManager(); err != nil {
		return fmt.Errorf("create mandatory services: %w", err)
	}
	services, err := m.sealComposition()
	if err != nil {
		return err
	}
	if err := m.initializeStartupMetrics(); err != nil {
		return fmt.Errorf("initialize startup metrics: %w", err)
	}
	if err := m.startMainListener(); err != nil {
		return fmt.Errorf("start HTTP diagnostics: %w", err)
	}
	if err := m.startMetricsServer(ctx); err != nil {
		return fmt.Errorf("start metrics diagnostics: %w", err)
	}

	m.logger.Debug("Starting services", "count", len(services))
	for _, admitted := range services {
		m.recordStartInvoked(admitted.name)
		startErr := admitted.service.Start(ctx)
		m.recordStartCompleted(admitted.name, startErr)
		if startErr != nil {
			m.logger.Error("Service start failed", "service", admitted.name, "error", startErr)
			return fmt.Errorf("failed to start service %s: %w", admitted.name, startErr)
		}
		m.logger.Debug("Service started", "service", admitted.name)
	}

	// The full route set is built off the listener; the main listener serves
	// only the startup routes until the commit below.
	fullMux, err := m.prepareFullMux()
	if err != nil {
		return fmt.Errorf("complete HTTP setup: %w", err)
	}
	if err := m.startHealthPublisher(ctx); err != nil {
		return fmt.Errorf("start health publisher: %w", err)
	}
	m.committedRoutes.Store(fullMux)
	m.logger.Info("All services started", "count", len(services), "port", m.config.HTTPPort)
	return nil
}

// createComponentManager creates the mandatory component-manager from its
// registered constructor when no service has that name.
func (m *Manager) createComponentManager() error {
	m.mu.RLock()
	_, exists := m.services[componentManagerName]
	deps := m.dependencies
	m.mu.RUnlock()
	if exists {
		return nil
	}
	if _, err := m.CreateService(componentManagerName, json.RawMessage("{}"), deps); err != nil {
		return fmt.Errorf("failed to create mandatory service %s: %w", componentManagerName, err)
	}
	m.logger.Info("Created mandatory service", "service", componentManagerName)
	return nil
}

// sealComposition fixes the composed services and returns them in registration
// order.
func (m *Manager) sealComposition() ([]admittedService, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, configured := range m.bootServiceConfigs {
		if name == "service-manager" || !configured.Enabled {
			continue
		}
		if _, exists := m.services[name]; !exists {
			return nil, fmt.Errorf("enabled configured service %s was not constructed", name)
		}
	}
	if _, exists := m.services[componentManagerName]; !exists {
		return nil, fmt.Errorf("mandatory service %s was not constructed", componentManagerName)
	}

	identities := make([]string, 0, len(m.services))
	for name := range m.services {
		identities = append(identities, name)
	}
	sort.Strings(identities)
	m.sealedServices = identities
	m.sealed = true

	m.serviceOutcomes = make(map[string]*serviceInvocationOutcome, len(m.order))
	services := make([]admittedService, 0, len(m.order))
	for _, name := range m.order {
		m.serviceOutcomes[name] = &serviceInvocationOutcome{service: m.services[name]}
		services = append(services, admittedService{name: name, service: m.services[name]})
	}
	return services, nil
}

// recordStartInvoked records that the service's Start was called.
func (m *Manager) recordStartInvoked(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if outcome := m.serviceOutcomes[name]; outcome != nil {
		outcome.startInvoked = true
	}
}

// recordStartCompleted records that the service's Start returned startErr.
func (m *Manager) recordStartCompleted(name string, startErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if outcome := m.serviceOutcomes[name]; outcome != nil {
		outcome.startCompleted, outcome.startErr = true, startErr
	}
}

func (m *Manager) serviceStartupCountsLocked() serviceStartupCounts {
	counts := serviceStartupCounts{Admitted: len(m.serviceOutcomes)}
	for _, outcome := range m.serviceOutcomes {
		if outcome.startInvoked {
			counts.StartsInvoked++
		}
		if outcome.startCompleted {
			counts.StartsCompleted++
		}
		if outcome.startErr != nil {
			counts.StartsFailed++
		}
	}
	return counts
}

func (m *Manager) serviceStartupCounts() serviceStartupCounts {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.serviceStartupCountsLocked()
}

// initializeStartupMetrics registers the startup collector on the injected
// metrics registry, else the metrics service's, and claims the metrics
// service's server. With neither registry no collector is registered. The
// component series read the component-manager's counts when it is the
// framework's *ComponentManager, and zero otherwise.
func (m *Manager) initializeStartupMetrics() error {
	m.mu.RLock()
	var registry *metric.MetricsRegistry
	if m.dependencies != nil {
		registry = m.dependencies.MetricsRegistry
	}
	components, _ := m.services[componentManagerName].(*ComponentManager)
	metricsService, _ := m.services["metrics"].(*Metrics)
	m.mu.RUnlock()
	if registry == nil && metricsService != nil {
		registry = metricsService.registry
	}

	if registry != nil {
		componentSnapshot := func() startupUnitCounts { return startupUnitCounts{} }
		if components != nil {
			componentSnapshot = components.startupSnapshot
		}
		if err := registerStartupCollector(registry, m.serviceStartupCounts, componentSnapshot); err != nil {
			return err
		}
	}

	if metricsService == nil {
		return nil
	}
	server, err := metricsService.claimManagerServer()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.metricsServer, m.metricsService = server, metricsService
	m.mu.Unlock()
	return nil
}

// startMetricsServer starts the claimed metrics server, if any.
func (m *Manager) startMetricsServer(ctx context.Context) error {
	m.mu.RLock()
	server, service := m.metricsServer, m.metricsService
	m.mu.RUnlock()
	if server == nil {
		return nil
	}
	if err := service.startServer(ctx, server); err != nil {
		return err
	}
	service.setManagerServerHealthy(true)
	return nil
}

// stopMetricsServer stops the claimed metrics server. One that does not stop is
// kept for the next stop.
func (m *Manager) stopMetricsServer(ctx context.Context) error {
	m.mu.RLock()
	server, service := m.metricsServer, m.metricsService
	m.mu.RUnlock()
	if server == nil {
		return nil
	}
	service.setManagerServerHealthy(false)
	if err := server.Stop(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	m.metricsServer, m.metricsService = nil, nil
	m.mu.Unlock()
	return nil
}

// startHealthPublisher starts the health publisher under a run context of its
// own, ended by a stop; it refuses a ctx that has ended.
func (m *Manager) startHealthPublisher(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// The run context: StartAll's values without its cancellation, ended by a
	// stop (ADR-113).
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})
	m.mu.Lock()
	m.healthPublisherCancel, m.healthPublisherDone = cancel, done
	m.mu.Unlock()
	go func() {
		defer close(done)
		m.publishHealthLoop(runCtx)
	}()
	return nil
}

// stopHealthPublisher ends the publisher's run context and waits for its
// goroutine, or returns ctx's error first and leaves it for the next stop.
func (m *Manager) stopHealthPublisher(ctx context.Context) error {
	m.mu.RLock()
	cancel, done := m.healthPublisherCancel, m.healthPublisherDone
	m.mu.RUnlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	m.mu.Lock()
	m.healthPublisherCancel, m.healthPublisherDone = nil, nil
	m.mu.Unlock()
	return nil
}

// publishHealthLoop publishes every service's health at once and then every
// healthPublishInterval until ctx ends.
func (m *Manager) publishHealthLoop(ctx context.Context) {
	m.publishServiceHealth(ctx)
	ticker := time.NewTicker(healthPublishInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.publishServiceHealth(ctx)
		}
	}
}

// publishServiceHealth publishes each composed service's health on
// health.service.<name>. With no NATS client it publishes nothing. A publish or
// an encode that fails is counted on
// semengine_errors_total{service="service-manager",type="health_publish"} and
// logged at Debug; the next service, and the next round, are published anyway,
// since a missed health report is replaced by the next one.
func (m *Manager) publishServiceHealth(ctx context.Context) {
	m.mu.RLock()
	client := m.natsClient
	var registry *metric.MetricsRegistry
	if m.dependencies != nil {
		registry = m.dependencies.MetricsRegistry
	}
	services := make(map[string]Service, len(m.services))
	for name, svc := range m.services {
		services[name] = svc
	}
	m.mu.RUnlock()
	if client == nil {
		return
	}

	timestamp := time.Now().UnixMilli()
	for name, svc := range services {
		subject := "health.service." + name
		data, err := json.Marshal(map[string]any{
			"timestamp": timestamp,
			"name":      name,
			"status":    svc.Status().String(),
			"health":    svc.Health(),
		})
		if err == nil {
			err = client.PublishToStream(ctx, subject, data)
		}
		if err != nil {
			m.logger.Debug("Health publish failed", "subject", subject, "error", err)
			if registry != nil {
				registry.CoreMetrics().RecordError("service-manager", "health_publish")
			}
		}
	}
}

// StopAll stops the composition. It refuses a nil context, and given a context
// that has already ended it returns that context's error and stops nothing.
// Before any StartAll it stops the health listener, if one was started, and
// returns nil; a later StartAll is refused. Otherwise it makes /readyz answer
// NOT READY at once, stops every started service in reverse registration order
// with ctx, then the health publisher, the health and main listeners, and the
// metrics server last. A listener refuses new requests and StopAll waits for
// those in flight; if ctx ends first, every connection still open is closed,
// which ends each open request's context, and a later StopAll waits for their
// handlers to return. StopAll returns nil only once everything is released. A
// service whose Stop returns errs.ErrAlreadyStopped counts as stopped. Any other
// failure is returned naming the service or listener, and the next StopAll runs
// the whole stop again. A StopAll after one that returned nil returns nil and
// calls nothing; one while another runs is refused as transient.
func (m *Manager) StopAll(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "Manager", "StopAll", "nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	m.stopCalled = true
	m.mu.Unlock()
	if err := m.lifecycle.Stop(ctx); err != nil {
		return err
	}
	// Before any StartAll the guard ran no stop, and a health listener may serve.
	if err := m.stopHealthListener(ctx); err != nil {
		return fmt.Errorf("stop health listener: %w", err)
	}
	return nil
}

// stopAll is the stop the lifecycle guard runs for StopAll and for a failed
// StartAll's rollback. It is safe to repeat: each step keeps what it did not
// release for the next run.
func (m *Manager) stopAll(ctx context.Context) error {
	m.mu.Lock()
	m.stopping = true
	m.committedRoutes.Store(nil)
	var started []admittedService
	for _, name := range slices.Backward(m.order) {
		if outcome := m.serviceOutcomes[name]; outcome != nil && outcome.startInvoked {
			started = append(started, admittedService{name: name, service: outcome.service})
		}
	}
	m.mu.Unlock()

	var stopErrs []error
	for _, admitted := range started {
		m.logger.Debug("Stopping service", "service", admitted.name)
		if err := admitted.service.Stop(ctx); err != nil && !errors.Is(err, errs.ErrAlreadyStopped) {
			m.logger.Error("Service stop failed", "service", admitted.name, "error", err)
			stopErrs = append(stopErrs, fmt.Errorf("failed to stop service %s: %w", admitted.name, err))
		}
	}
	if err := m.stopHealthPublisher(ctx); err != nil {
		stopErrs = append(stopErrs, fmt.Errorf("stop health publisher: %w", err))
	}
	if err := m.stopHealthListener(ctx); err != nil {
		stopErrs = append(stopErrs, fmt.Errorf("stop health listener: %w", err))
	}
	if err := m.stopMainListener(ctx); err != nil {
		stopErrs = append(stopErrs, fmt.Errorf("stop main HTTP listener: %w", err))
	}
	if err := m.stopMetricsServer(ctx); err != nil {
		stopErrs = append(stopErrs, fmt.Errorf("stop metrics server: %w", err))
	}
	if len(stopErrs) > 0 {
		return fmt.Errorf("stop errors: %w", errors.Join(stopErrs...))
	}

	m.mu.Lock()
	m.services = make(map[string]Service)
	m.order = nil
	m.mu.Unlock()
	m.logger.Debug("All services stopped", "count", len(started))
	return nil
}
