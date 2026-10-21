package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/health"
	"github.com/c360studio/semengine/internal/httpadmission"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// StorageObservabilityServiceName is the key this service is configured and
// registered under.
const StorageObservabilityServiceName = "storage-observability"

// startupComponentPrefix is where a componentStartup component-manager's
// startup routes, and its routes after the commit, are mounted.
const startupComponentPrefix = "/components"

// UseHTTPMiddleware appends product-supplied middleware to the chain wrapped
// around every route the main listener serves, the startup routes as well as
// the full set. Order is outermost-first: the first middleware passed, in this
// call or across calls, is the outermost wrapper.
//
// This is the framework's only HTTP middleware seam. The framework ships no
// default middleware; auth, request logging, panic recovery, rate limiting and
// CORS are product policy. The dedicated health listener takes no middleware.
//
// It must be called before StartAll binds the main listener. A later call is
// ignored with a Warn: the listener's handler is built at the bind, so the
// middleware would never run.
func (m *Manager) UseHTTPMiddleware(mws ...HTTPMiddleware) {
	if len(mws) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.httpStarted {
		m.logger.Warn("UseHTTPMiddleware called after HTTP server started; ignoring",
			"middleware_count", len(mws))
		return
	}
	m.httpMiddleware = append(m.httpMiddleware, mws...)
}

// startMainListener binds http_port and serves the startup routes; anything
// else answers 503 NOT READY until StartAll commits the full route set.
func (m *Manager) startMainListener() error {
	m.mu.Lock()
	m.httpStarted = true
	middleware := m.httpMiddleware
	cfg := m.config
	components, _ := m.services[componentManagerName].(componentStartup)
	m.mu.Unlock()

	startup := http.NewServeMux()
	m.registerDiagnosticEndpoints(startup)
	if components != nil {
		components.registerStartupHTTPHandlers(startupComponentPrefix, startup)
	}
	startup.HandleFunc("/", answerNotReady)

	listener, err := net.Listen("tcp", ":"+strconv.Itoa(cfg.HTTPPort))
	if err != nil {
		return fmt.Errorf("bind HTTP listener: %w", err)
	}
	dispatch := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if full := m.committedRoutes.Load(); full != nil {
			full.ServeHTTP(w, r)
			return
		}
		startup.ServeHTTP(w, r)
	})
	// The admission wrapper is outermost, so a request admitted before a stop
	// began is waited for whatever middleware does with it.
	requests := &httpadmission.Requests{}
	server := &http.Server{
		Handler:      requests.Wrap(chainMiddleware(dispatch, middleware), "service manager is stopping"),
		ReadTimeout:  cfg.ResolvedHTTPReadTimeout(),
		WriteTimeout: cfg.ResolvedHTTPWriteTimeout(),
		IdleTimeout:  60 * time.Second,
	}

	m.mu.Lock()
	m.httpServer, m.httpListener, m.httpRequests = server, listener, requests
	m.httpServeDone = serveHTTP(server, listener, m.logger, "main")
	m.mu.Unlock()
	return nil
}

// stopMainListener stops the main listener (stopHTTP). One that does not stop is
// kept for the next stop.
func (m *Manager) stopMainListener(ctx context.Context) error {
	m.mu.RLock()
	server, serveDone, requests := m.httpServer, m.httpServeDone, m.httpRequests
	m.mu.RUnlock()
	if server == nil {
		return nil
	}
	if err := stopHTTP(ctx, server, serveDone, requests); err != nil {
		return err
	}
	m.mu.Lock()
	m.httpServer, m.httpListener, m.httpServeDone, m.httpRequests = nil, nil, nil, nil
	m.mu.Unlock()
	return nil
}

// StartHealthListener binds a dedicated listener on port serving only /health
// and /healthz, for container probes that want a port apart from http_port.
// Port 0 does nothing. A bind failure is returned. ctx bounds the bind only:
// ending it after StartHealthListener returned stops nothing. A second call,
// and one after StopAll was called, are refused. StopAll stops the listener,
// whether or not StartAll ran.
func (m *Manager) StartHealthListener(ctx context.Context, port int) error {
	return m.startHealthListener(ctx, port, net.Listen)
}

func (m *Manager) startHealthListener(ctx context.Context, port int, listen func(string, string) (net.Listener, error)) error {
	if ctx == nil {
		return errs.WrapInvalid(errs.ErrInvalidData, "Manager", "StartHealthListener", "nil context")
	}
	if err := ctx.Err(); err != nil {
		return errs.WrapInvalid(err, "Manager", "StartHealthListener", "context already canceled")
	}
	if port == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.healthUsed {
		return fmt.Errorf("health listener already started")
	}
	if m.stopCalled {
		return fmt.Errorf("health listener refused: StopAll was called")
	}
	listener, err := listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return fmt.Errorf("bind health listener: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", m.handleSystemHealth)
	mux.HandleFunc("/healthz", m.handleLiveness)
	requests := &httpadmission.Requests{}
	server := &http.Server{
		Handler: requests.Wrap(mux, "health listener is stopping"),
		// Probes are short; tight timeouts keep a misbehaving probe client from
		// holding a connection.
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
	m.healthUsed = true
	m.healthServer, m.healthRequests = server, requests
	m.healthServeDone = serveHTTP(server, listener, m.logger, "health")
	m.logger.Info("Started dedicated health listener", "port", port, "routes", []string{"/health", "/healthz"})
	return nil
}

// stopHealthListener stops the health listener (stopHTTP). One that does not
// stop is kept for the next stop.
func (m *Manager) stopHealthListener(ctx context.Context) error {
	m.mu.RLock()
	server, serveDone, requests := m.healthServer, m.healthServeDone, m.healthRequests
	m.mu.RUnlock()
	if server == nil {
		return nil
	}
	if err := stopHTTP(ctx, server, serveDone, requests); err != nil {
		return err
	}
	m.mu.Lock()
	m.healthServer, m.healthServeDone, m.healthRequests = nil, nil, nil
	m.mu.Unlock()
	return nil
}

// serveHTTP serves server on listener in a goroutine and returns the channel
// closed when Serve returns.
func serveHTTP(server *http.Server, listener net.Listener, logger *slog.Logger, name string) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			logger.Error("HTTP listener stopped serving", "listener", name, "error", err)
		}
	}()
	return done
}

// stopHTTP refuses the server's new requests, closes its listener and waits,
// within ctx, for the requests in flight. If ctx ends first it closes every
// connection still open, which ends each open request's context, and returns
// ctx's error. net/http stops tracking a handler once its connection is closed,
// so a later call waits for the handlers requests admitted
// (setup-04a-03-boot-service-manager design D2, Q-B). It returns nil only once
// Serve and every admitted handler have returned.
func stopHTTP(ctx context.Context, server *http.Server, serveDone <-chan struct{}, requests *httpadmission.Requests) error {
	requests.CloseAdmission()
	err := server.Shutdown(ctx)
	if err != nil {
		if closeErr := server.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			err = errors.Join(err, closeErr)
		}
	}
	// Serve returns once Shutdown closed its listener.
	<-serveDone
	if err != nil {
		return err
	}
	return requests.Wait(ctx)
}

// answerNotReady answers 503 NOT READY.
func answerNotReady(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("NOT READY"))
}

// prepareFullMux builds the full route set: the diagnostic routes,
// /graph/triples, every HTTPHandler service's routes under its prefix, the
// routes of components that register their own, and the API document.
func (m *Manager) prepareFullMux() (*http.ServeMux, error) {
	mux := http.NewServeMux()
	m.registerDiagnosticEndpoints(mux)
	mux.HandleFunc("/graph/triples", m.handleGraphTriples)
	for _, admitted := range m.sealedServiceList() {
		if handler, ok := admitted.service.(HTTPHandler); ok {
			handler.RegisterHTTPHandlers("/"+serviceNameToPrefix(admitted.name), mux)
		}
	}
	if err := m.registerComponentHandlers(mux); err != nil {
		return nil, fmt.Errorf("failed to register component handlers: %w", err)
	}
	mux.HandleFunc("/openapi.json", m.handleOpenAPISpec)
	if m.config.SwaggerUI {
		mux.HandleFunc("/docs", handleSwaggerUI)
	}
	return mux, nil
}

// sealedServiceList returns the sealed services in name order.
func (m *Manager) sealedServiceList() []admittedService {
	m.mu.RLock()
	defer m.mu.RUnlock()
	services := make([]admittedService, 0, len(m.sealedServices))
	for _, name := range m.sealedServices {
		if svc, exists := m.services[name]; exists {
			services = append(services, admittedService{name: name, service: svc})
		}
	}
	return services
}

// registerComponentHandlers mounts, at /<name>, the routes of every component
// that registers its own, when component-manager implements componentStartup.
func (m *Manager) registerComponentHandlers(mux *http.ServeMux) error {
	m.mu.RLock()
	components, ok := m.services[componentManagerName].(componentStartup)
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return components.withComponents(func(managed map[string]*component.ManagedComponent) error {
		for name, mc := range managed {
			if gateway, ok := mc.Component.(interface {
				RegisterHTTPHandlers(prefix string, mux *http.ServeMux)
			}); ok {
				prefix := "/" + name
				gateway.RegisterHTTPHandlers(prefix, mux)
				m.logger.Debug("Registered component HTTP handlers", "component", name, "prefix", prefix)
			}
		}
		return nil
	})
}

// handleOpenAPISpec serves the API document.
func (m *Manager) handleOpenAPISpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if err := json.NewEncoder(w).Encode(m.generateOpenAPIDocument()); err != nil {
		http.Error(w, "Failed to encode OpenAPI spec", http.StatusInternalServerError)
	}
}

// handleSwaggerUI serves a Swagger UI page, loaded from unpkg.com, for
// /openapi.json.
func handleSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	const page = `<!DOCTYPE html>
<html>
<head>
    <title>SemEngine API Documentation</title>
    <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@3.52.5/swagger-ui.css" />
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@3.52.5/swagger-ui-bundle.js"></script>
    <script>
        SwaggerUIBundle({
            url: '/openapi.json',
            dom_id: '#swagger-ui',
            presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.presets.standalone],
        });
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(page))
}

// generateOpenAPIDocument merges every HTTPHandler service's paths under its
// prefix, and the schemas of every registered spec.
func (m *Manager) generateOpenAPIDocument() *OpenAPIDocument {
	doc := &OpenAPIDocument{
		OpenAPI: "3.0.0",
		Info:    m.config.ServerInfo,
		Servers: []ServerSpec{{
			URL:         fmt.Sprintf("http://localhost:%d", m.config.HTTPPort),
			Description: "Development server",
		}},
		Paths: make(map[string]PathSpec),
		Tags:  make([]TagSpec, 0),
	}

	for _, admitted := range m.sealedServiceList() {
		handler, ok := admitted.service.(HTTPHandler)
		if !ok {
			continue
		}
		spec := handler.OpenAPISpec()
		if spec == nil {
			continue
		}
		prefix := "/" + serviceNameToPrefix(admitted.name)
		for path, pathSpec := range spec.Paths {
			doc.Paths[prefix+path] = pathSpec
		}
		doc.Tags = append(doc.Tags, spec.Tags...)
	}

	schemas := make(map[string]any)
	seen := make(map[reflect.Type]bool)
	for _, spec := range GetAllOpenAPISpecs() {
		for _, group := range [][]reflect.Type{spec.ResponseTypes, spec.RequestBodyTypes} {
			for _, t := range group {
				if !seen[t] {
					seen[t] = true
					schemas[TypeNameFromReflect(t)] = SchemaFromType(t)
				}
			}
		}
	}
	if len(schemas) > 0 {
		doc.Components = &ComponentsSpec{Schemas: schemas}
	}
	return doc
}

// serviceNameToPrefix returns the URL prefix, without its slash, a service's
// routes are mounted under: component-manager at components, message-logger and
// storage-observability as named, any other name with its hyphens removed.
func serviceNameToPrefix(serviceName string) string {
	switch serviceName {
	case componentManagerName:
		return "components"
	case "message-logger":
		return "message-logger"
	case StorageObservabilityServiceName:
		// Hyphens kept, as for message-logger: the default would mount the
		// storage report at /storageobservability, which no operator would guess.
		return StorageObservabilityServiceName
	default:
		return strings.ReplaceAll(serviceName, "-", "")
	}
}

// registerDiagnosticEndpoints registers the routes the main listener serves
// before the commit as well as after it.
func (m *Manager) registerDiagnosticEndpoints(mux *http.ServeMux) {
	mux.HandleFunc("/health", m.handleSystemHealth)
	mux.HandleFunc("/healthz", m.handleLiveness)
	mux.HandleFunc("/readyz", m.handleReadiness)
	mux.HandleFunc("/services", m.handleServiceList)
	mux.HandleFunc("/services/health", m.handleServicesHealth)
}

// composedServices returns every composed service.
func (m *Manager) composedServices() []Service {
	m.mu.RLock()
	defer m.mu.RUnlock()
	services := make([]Service, 0, len(m.services))
	for _, svc := range m.services {
		services = append(services, svc)
	}
	return services
}

// handleSystemHealth answers the health of every service and of the NATS
// connection, aggregated: 503 when unhealthy, 200 otherwise.
func (m *Manager) handleSystemHealth(w http.ResponseWriter, _ *http.Request) {
	services := m.composedServices()
	m.mu.RLock()
	natsClient, logger := m.natsClient, m.logger
	m.mu.RUnlock()

	subStatuses := make([]health.Status, 0, len(services)+1)
	for _, svc := range services {
		subStatuses = append(subStatuses, svc.Health())
	}
	if natsClient != nil {
		natsStatus := natsClient.GetStatus()
		if natsStatus.Status == natsclient.StatusConnected {
			subStatuses = append(subStatuses, health.NewHealthy("nats",
				fmt.Sprintf("Connected (RTT: %v)", natsStatus.RTT)))
		} else {
			subStatuses = append(subStatuses, health.NewUnhealthy("nats",
				fmt.Sprintf("Disconnected: %s", natsStatus.Status.String())))
		}
	}
	systemHealth := health.Aggregate("system", subStatuses)

	w.Header().Set("Content-Type", "application/json")
	if systemHealth.IsUnhealthy() {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	if err := json.NewEncoder(w).Encode(systemHealth); err != nil {
		logger.Error("Failed to encode system health response", "error", err)
	}
}

// handleLiveness answers 200 OK while the listener serves.
func (m *Manager) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// handleReadiness answers READY only while the startup snapshot says ready, and
// 503 NOT READY otherwise. The bodies are kept for existing probes.
func (m *Manager) handleReadiness(w http.ResponseWriter, _ *http.Request) {
	if m.currentStartupSnapshot().Status == "ready" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("READY"))
		return
	}
	answerNotReady(w, nil)
}

// currentStartupSnapshot reports startup: failed when a service or component
// start failed, stopping once StopAll was called or a stop began, starting
// until every service and component lifecycle participant has started,
// not_ready until the commit or while a service is not running and healthy or
// a component is unhealthy or missing from health, and ready otherwise.
func (m *Manager) currentStartupSnapshot() startupSnapshot {
	m.mu.RLock()
	sealed, stopping := m.sealed, m.stopping || m.stopCalled
	serviceCounts := m.serviceStartupCountsLocked()
	services := make(map[string]Service, len(m.serviceOutcomes))
	for name, outcome := range m.serviceOutcomes {
		services[name] = outcome.service
	}
	m.mu.RUnlock()

	componentManager, hasComponentManager := services[componentManagerName]
	components, hasComponents := componentManager.(componentStartup)
	var componentCounts startupUnitCounts
	if hasComponents {
		componentCounts = components.startupSnapshot()
	}

	snapshot := startupSnapshot{Services: serviceCounts, Components: componentCounts}
	servicesComplete := sealed && hasComponentManager && serviceCounts.Admitted > 0 &&
		serviceCounts.StartsInvoked == serviceCounts.Admitted &&
		serviceCounts.StartsCompleted == serviceCounts.Admitted
	componentsComplete := !hasComponents ||
		(componentCounts.StartsInvoked == componentCounts.LifecycleParticipants &&
			componentCounts.StartsCompleted == componentCounts.LifecycleParticipants)
	switch {
	case serviceCounts.StartsFailed > 0 || componentCounts.StartsFailed > 0:
		snapshot.Status = "failed"
		return snapshot
	case stopping || (hasComponents && components.startupStopBegun()):
		snapshot.Status = "stopping"
		return snapshot
	case !servicesComplete || !componentsComplete:
		snapshot.Status = "starting"
		return snapshot
	case m.committedRoutes.Load() == nil:
		snapshot.Status = "not_ready"
		return snapshot
	}

	for _, svc := range services {
		if svc.Status() != StatusRunning || !svc.IsHealthy() {
			snapshot.Status = "not_ready"
			return snapshot
		}
	}
	if hasComponents {
		healthByName := components.GetComponentHealth()
		if len(healthByName) != componentCounts.Admitted {
			snapshot.Status = "not_ready"
			return snapshot
		}
		for _, status := range healthByName {
			if !status.Healthy {
				snapshot.Status = "not_ready"
				return snapshot
			}
		}
	}
	// A StopAll called, or a stop begun, while the checks above ran.
	m.mu.RLock()
	stopCalled := m.stopCalled
	m.mu.RUnlock()
	if stopCalled || m.committedRoutes.Load() == nil {
		snapshot.Status = "stopping"
		return snapshot
	}
	snapshot.Status = "ready"
	return snapshot
}

// handleServiceList answers the composed services with their status and
// health, the startup snapshot, and the service configuration changes that
// wait for a restart.
func (m *Manager) handleServiceList(w http.ResponseWriter, _ *http.Request) {
	m.mu.RLock()
	identities := append([]string(nil), m.sealedServices...)
	if len(identities) == 0 {
		for name := range m.services {
			identities = append(identities, name)
		}
		sort.Strings(identities)
	}
	instances := make(map[string]Service, len(m.services))
	for name, svc := range m.services {
		instances[name] = svc
	}
	boot := cloneResolvedServiceConfigs(m.bootServiceConfigs)
	configManager := m.configManager
	m.mu.RUnlock()

	services := make([]map[string]any, 0, len(identities))
	for _, name := range identities {
		svc, exists := instances[name]
		if !exists {
			continue
		}
		services = append(services, map[string]any{
			"name":    name,
			"status":  svc.Status().String(),
			"healthy": svc.IsHealthy(),
		})
	}

	desired := boot
	if configManager != nil {
		resolved, err := ResolveServiceConfigs(configManager.GetConfig().Get().Services)
		if err != nil {
			http.Error(w, "Failed to resolve desired service configuration", http.StatusInternalServerError)
			return
		}
		desired = resolved
	}
	pending := pendingServiceChanges(boot, desired)

	response := map[string]any{
		"services":                services,
		"count":                   len(services),
		"startup":                 m.currentStartupSnapshot(),
		"restart_required":        len(pending) > 0,
		"pending_service_changes": pending,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		m.logger.Error("Failed to encode services list", "error", err)
	}
}

// handleServicesHealth answers every service's health and their aggregate: 503
// when the aggregate is unhealthy.
func (m *Manager) handleServicesHealth(w http.ResponseWriter, _ *http.Request) {
	services := m.composedServices()
	statuses := make([]health.Status, 0, len(services))
	for _, svc := range services {
		statuses = append(statuses, svc.Health())
	}
	response := struct {
		Overall  health.Status   `json:"overall"`
		Services []health.Status `json:"services"`
	}{
		Overall:  health.Aggregate("services", statuses),
		Services: statuses,
	}
	w.Header().Set("Content-Type", "application/json")
	if response.Overall.IsUnhealthy() {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		m.logger.Error("Failed to encode services health response", "error", err)
	}
}
