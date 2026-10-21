// Package service holds what every long-running SemEngine service shares:
// BaseService, which a service embeds for its name, status and health check,
// and the Service interface a service manager drives. Manager is that service
// manager: it builds the services a configuration names, starts them, serves the
// HTTP routes around them and stops them. ComponentManager is the service that
// owns the components. Metrics is the service that serves the Prometheus
// endpoint. The package also holds the helpers a composition root (the
// process's main function, which builds and wires everything) calls itself:
// MaybeStartPProf, WireGraphRuntime and the OpenAPI registry, plus the
// service-configuration and schema helpers the service manager reads.
//
// This package is ported from SemStreams in seven pull requests. The first
// carried BaseService and the shared files; the second Manager and Metrics; this
// one carries ComponentManager, the service that owns the components (see the
// component manager section). The other services arrive with the later ones.
//
// # Embedding BaseService
//
// A service embeds the *BaseService that NewBaseServiceWithOptions builds from
// its name and options:
//
//	type ruleConfigService struct {
//		*service.BaseService
//	}
//
//	svc := &ruleConfigService{
//		BaseService: service.NewBaseServiceWithOptions("rule-config",
//			service.WithNATS(client), service.WithMetrics(registry), service.WithLogger(logger)),
//	}
//
// WithNATS adds the client's health to the health check, WithMetrics records
// every status change on that registry, and WithLogger replaces the default
// logger (slog.Default() with a "service" attribute). BaseService itself logs
// nothing; the services in this package log through it.
//
// Start(ctx) reports StatusRunning and runs the health check at once and then
// every 30 seconds. It refuses a nil or ended ctx, a second Start, and a Start
// after any Stop (errs.ErrAlreadyStarted): a BaseService runs once, so build a
// new one to start again. Ending ctx after Start returned stops nothing; Stop
// is the only stop (ADR-113).
//
// Stop(ctx) reports StatusStopping, starts no further health check, and
// returns nil once the health goroutine has exited, leaving StatusStopped and
// IsHealthy false. If ctx ends while it waits, Stop returns ctx's error and a
// later Stop waits again. A ctx that has already ended stops nothing: Stop
// returns its error at once and the service keeps running. So give Stop a
// fresh bounded context, never the signal context that is ending the program:
//
//	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
//	defer cancel()
//	if err := svc.Stop(stopCtx); err != nil {
//		logger.Warn("rule-config stop", "error", err)
//	}
//
// A Stop after one that returned nil returns nil. A Stop while another Stop is
// still waiting is refused as a transient error. Stop before Start returns nil,
// and the service then refuses to start.
//
// The health check fails when the function set by SetHealthCheck returns an
// error, or when the NATS client from WithNATS is not healthy. IsHealthy is
// then false and Health reports unhealthy; the next passing check makes the
// service healthy again.
//
// # The service manager
//
// A composition root builds a Manager from a Registry of service constructors,
// hands it the services section of the configuration, and starts it:
//
//	services := service.NewServiceRegistry()
//	if err := service.RegisterAll(services); err != nil { // "metrics", "component-manager"
//		return err
//	}
//
//	manager := service.NewServiceManager(services)
//	manager.UseHTTPMiddleware(authMiddleware) // optional; before StartAll
//
//	deps := &service.Dependencies{
//		NATSClient: client, MetricsRegistry: registry, Logger: logger,
//		Platform: platform, Manager: configManager,
//		ComponentRegistry: components, PayloadRegistry: payloads,
//	}
//	if err := manager.ConfigureFromServices(cfg.Services, deps); err != nil {
//		return err
//	}
//	if err := manager.StartAll(startCtx); err != nil {
//		return err
//	}
//
//	// ... run ...
//
//	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//	if err := manager.StopAll(stopCtx); err != nil {
//		logger.Warn("stop services", "error", err)
//	}
//
// ConfigureFromServices resolves the configuration with ResolveServiceConfigs
// and constructs every enabled service from its registered constructor. Nothing
// is composed unless every constructor succeeds. component-manager and
// service-manager may not be disabled, and a composition whose component-manager
// has no registered constructor is refused. The service-manager entry configures
// the Manager itself (ManagerConfig): http_port (default 8080), swagger_ui
// (default false), server_info (title, description and version of the API
// document, default "SemEngine API", "SemEngine framework API" and "0.0.0"),
// http_read_timeout (default 30s) and http_write_timeout (default 120s). The
// configs of the services this package defines, service-manager, metrics and
// component-manager, are decoded strictly: an unknown key is refused, and so is a key that differs
// from a field's name only in letter case ("Port" for "port"), naming both
// spellings. A service defined elsewhere receives its raw JSON and decodes it
// itself. CreateService and RegisterInstance compose one more service by hand,
// and GetService finds a composed one.
//
// StartAll creates component-manager if no service has that name, seals the
// composition (CreateService, RegisterInstance and ConfigureFromServices are
// refused from then on), binds the main listener on http_port, starts the
// metrics server it claims, starts every service in registration order, builds
// the full route set and starts a publisher that sends each service's health on
// health.service.<name> every 5 seconds. It runs once: a second StartAll, and a
// StartAll after StopAll, are refused with errs.ErrAlreadyStarted. If a step
// fails, StartAll stops what it started and returns both errors; if that stop
// fails too, StopAll runs it again.
//
// The context given to StartAll bounds StartAll and nothing else. Ending it after
// StartAll returned stops no service, and no request's context derives from it
// (ADR-113). StopAll(ctx) is the only stop: it stops the services in reverse
// registration order, then the health publisher, the listeners and the metrics
// server. Each listener refuses new requests and StopAll waits for those in
// flight; if ctx ends first it closes every connection still open and returns
// ctx's error, and a later StopAll waits again for the handlers that are still
// running. So a service that serves a long-lived response, such as a stream,
// must end it in its own Stop. A StopAll given a context that has already ended
// stops nothing, so give it a fresh bounded context, never the signal context
// that is ending the program. A service whose Stop returns errs.ErrAlreadyStopped
// counts as stopped; any other failure is returned naming the service, and the
// next StopAll runs the whole stop again.
//
// UseHTTPMiddleware wraps every route the main listener serves, the startup
// routes as well as the full set, outermost first. It must be called before
// StartAll; a later call is ignored with a Warn. The dedicated health listener
// takes no middleware, and the framework supplies none (SemStreams ADR-030,
// docs/adr/030-http-middleware-and-identity-pattern.md).
//
// StartHealthListener(ctx, port) binds a second listener that serves only
// /health and /healthz, for container probes that want a port apart from
// http_port. It may be called before StartAll, so probes work while a slow boot
// runs; port 0 does nothing, and a second call is refused. StopAll stops it
// whether or not StartAll ran.
//
// Boot seals the composition: a change to the services section of the
// configuration after StartAll is desired state for the next boot and starts,
// stops or reconfigures nothing, and /services reports it as restart_required
// (SemStreams ADR-094, docs/adr/094-boot-only-composition-and-observable-rule-activation.md).
//
// # The routes
//
// The main listener serves /health, /healthz, /readyz, /services and
// /services/health from the moment StartAll binds it, and answers every other
// path 503 NOT READY until StartAll has started every service and committed the
// full route set. After the commit it also serves /graph/triples, each
// HTTPHandler service's routes under a prefix taken from its name (the name with
// its hyphens removed, except component-manager at /components and
// message-logger and storage-observability as named), /openapi.json, and /docs
// when swagger_ui is true.
//
// /health answers every service's health and the NATS connection's, aggregated,
// 503 when unhealthy. /healthz answers 200 OK while the listener serves.
// /readyz answers 200 READY once every service has started and is running and
// healthy, and 503 NOT READY otherwise: while starting, after a start failed,
// and from the moment StopAll is entered, even while a service's Start is still
// running. /services lists each service's status and health, the startup counts
// and whether the configuration has changed since boot. /services/health answers
// each service's health and their aggregate, 503 when unhealthy.
//
// GET /graph/triples reads triples out of the graph's ENTITY_STATES bucket for
// an operator or a test; it is a low-volume scan, not a query API. Its query
// parameters, all optional: subject (a literal entity ID), predicate (a
// three-part predicate), object (compared as text) and limit (default 100, at
// most 1000). It answers a JSON array, sorted by entity ID and never null, 400
// naming the parameter for a malformed one, 405 for a method other than GET, and
// 500 when the bucket cannot be read. With no NATS client in the Dependencies it
// answers [] for every query: there is no store to read. The Manager only reads
// the bucket (ADR-110).
//
// /openapi.json answers the API document in OpenAPI 3.0 form: the paths of every
// HTTPHandler service under its prefix, and the schemas of the types registered
// with RegisterOpenAPISpec. /docs serves a Swagger UI page that loads its script
// and style sheet from unpkg.com, so a browser needs network access to it.
//
// # The component manager
//
// A component is one unit of a running composition: an input, processor,
// output or storage unit. A binary makes the component factories available
// through a component.Registry, and the components section of the
// configuration names the instances to build. ComponentManager builds every
// enabled component, starts them, serves their status over HTTP and stops them.
// Every composition has one: component-manager may not be disabled. RegisterAll
// registers its constructor, NewComponentManager, and its OpenAPI spec. Its
// config takes no key; any key is refused, naming it. Its Dependencies fields:
// Manager (the *config.Manager whose boot configuration it reads) and
// ComponentRegistry are required; PayloadRegistry, NATSClient, Platform,
// MetricsRegistry and Logger are given to each component, and with no
// NATSClient the manager publishes no component health.
//
// NewComponentManager reads the boot configuration once; later writes to it are
// desired state for the next boot and change nothing in the running set
// (SemStreams ADR-094 and ADR-096, docs/adr). It builds each enabled component
// through Registry.CreateComponent in instance-name order, checks the whole set
// with composition.Analyze, and seals the registry (ADR-100). It returns an
// error, and no service, when the Manager or ComponentRegistry is missing, when
// any enabled component cannot be built (the error names every one that
// failed), or when the composition has an error-severity finding.
//
// Start(ctx) refuses a nil or ended ctx, a second Start, and a Start after any
// Stop (errs.ErrAlreadyStarted). The start order is fixed at construction:
// components that provide stores (component.StoreProvider) by instance name,
// then the rest by instance name. Start runs two phases, providers first; each
// phase starts its components concurrently and the next begins only after every
// Start in the current one returned. A failure in the first phase starts
// nothing of the second. A component that is not a component.LifecycleComponent
// is built and listed but never started or stopped.
//
// Each component's Start receives the context the manager's Start received.
// The manager keeps no context of its own for a component and never ends the
// one it was given, Stop included (ADR-113). A component makes its own run
// context in Start and ends it in Stop; one that hangs its continuing work on
// its Start context stops when the caller ends that context, before Stop
// reaches it. The manager's health publisher and override-expiry reporter run
// under a run context that Start creates and Stop ends.
//
// A failed Start returns every component failure joined, each naming its
// component, after one rollback: it stops every component it launched, the
// failing ones included, in reverse start order. If the rollback fails too, the
// next Stop runs it again.
//
// Stop(ctx) is the only stop. It refuses a nil ctx; given an ended ctx it
// returns that error and stops nothing. Before any Start it returns nil and a
// later Start is refused. Otherwise it marks the stop begun (/readyz answers
// 503 NOT READY from then), stops each started component with ctx in reverse
// start order (just before a provider's own Stop it removes that provider's
// stores from the store registry), then ends the manager's own loops and its
// base service, and returns nil only once all are released. A component whose Stop failed, or was not reached because ctx ended,
// is stopped again by the next Stop; one whose Stop returned nil is not called
// again. A Stop while another runs is refused as transient. A Stop that arrives
// while Start is running waits for every component's Start to return, or
// returns its own context's error first; a caller cuts a slow boot short only
// by ending the context it gave Start.
//
// GetComponentHealth, GetComponentStatus, the /components routes and the
// manager's health check wait on no lock and not on Start or Stop. Each asks the
// component itself, so a request during or after Stop gets whatever the
// component answers then; SemStreams kept such requests away from a stopping
// component and this manager does not. A component author holds the line in
// component.LifecycleComponent: Health, DataFlow and DebugStatus may be called
// at any time and return without waiting for Start or Stop. The tree enforces
// it for graph-ingest only. The manager's own health is unhealthy while any
// component is in the failed state, naming each one with its last error.
//
// With a NATS client the manager publishes each admitted component's health on
// health.component.<name> once as Start returns and then every 5 seconds. A
// publish that fails is counted on
// semengine_errors_total{service="component-manager",type="health_publish"} and
// logged at Debug; the next component and round are still published. From Start
// to Stop it also evaluates the live configuration's stream migration overrides
// every minute and sets the gauge
// semengine_streams_migration_override_expired{stream,owner} to 1 for a lapsed
// bridge, 0 for an open one, with a Warn line per lapsed bridge. It only
// reports; readiness is unaffected.
//
// The routes, mounted under /components and answering GET only (405 otherwise),
// are health, list, types, types/{id}, status/{name} (?debug=true adds a
// component's DebugStatus), config/{name}, flowgraph (JSON, or Mermaid with
// ?format=mermaid), validate and paths. A name that is empty, "." or ".." or
// decodes to a path separator is 400, an unknown one 404. Until StartAll commits
// the full route set only health, list and status/{name} are served. flowgraph,
// validate and paths project the composition result kept at construction and
// compute nothing. BuildComponentTypeCatalog returns what types serves, for a
// product that serves the catalog elsewhere. One declared difference: GET
// /components/types/{id} for an unregistered type answers 404 with the body
// {"error":"Component type <id> not found"} sent as text/plain, not JSON, as
// SemStreams does; read the status code, not the text.
//
// ConfigureRulePackMutations(manager) hands a reconciler to each rule
// processor. A composition root must call it after ConfigureFromServices and
// before StartAll; without the call no ProjectionBinder is given one. It finds
// the admitted components that implement ProjectionBinder (ComponentsImplementing
// returns the components that implement any T, in start order), runs every
// binder's PreflightProjectionMutations before changing anything, validates
// each pack ID and refuses a duplicate, copies and validates each pack's
// contracts, refuses contracts when there is no NATS client, and gives each pack
// that has contracts one projection.MutationClient through SetPredicateReconciler.
// No rule processor is in this tree yet, so today the call has no effect.
//
// /openapi.json takes its schemas only from the registry RegisterOpenAPISpec
// fills, and the component manager's flowgraph and validate responses refer to
// the Graph and Result schemas there. RegisterAll registers the component
// manager's spec; a composition that registers NewComponentManager by hand must
// register the spec itself, as
//
//	if svc, ok := manager.GetService("component-manager"); ok {
//		service.RegisterOpenAPISpec("component-manager", svc.(service.HTTPHandler).OpenAPISpec())
//	}
//
// or those references name schemas the document does not carry.
//
// # The metrics service
//
// Metrics serves the Prometheus endpoint. Its config keys are port (default
// 9090) and path (default "/metrics"). Started by itself, it binds the port
// between Start and Stop. Composed under a Manager, the Manager claims the server
// before any service starts, serves it until StopAll, and Metrics reports healthy
// while the server runs. RegisterAll registers Metrics and the component manager.
// A Metrics runs once: a second Start, and a Start after any Stop, are refused
// with errs.ErrAlreadyStarted, and the context given to Start bounds Start only.
//
// # The profiling server
//
// MaybeStartPProf(debug, port, logger) serves http.DefaultServeMux on :port,
// every interface, when debug is true and port is positive, and returns nil
// otherwise. The pprof handlers are on that mux only if the caller's main has
// the blank import `import _ "net/http/pprof"`. It binds before it returns; a
// bind failure is logged at Warn on logger and returns nil, because profiling
// is a debug aid, never a reason to fail the boot. It is not a Service: the
// composition root calls it early, before NATS, so a slow boot can be
// profiled (SemStreams ADR-058, docs/adr/058-boot-lifecycle-phases.md).
//
// The composition root shuts it down on its exit path, after its services
// have stopped. Shutdown on a nil *PProfServer returns nil, so this is safe
// whatever MaybeStartPProf returned:
//
//	pprofServer := service.MaybeStartPProf(debug, port, logger)
//
//	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	defer cancel()
//	if err := pprofServer.Shutdown(stopCtx); err != nil {
//		logger.Warn("pprof shutdown", "error", err)
//	}
//
// Shutdown stops accepting connections and returns nil once every request in
// progress has finished and the serving goroutine has exited. If its context
// ends first it returns that context's error, and a later Shutdown waits
// again. It refuses a nil context, on a nil server too.
//
// # The graph runtime
//
// WireGraphRuntime(ctx, client, logger, contracts...) first runs
// kvcatalog.EnsureCatalogRetentionClean, which strips a retention limit left
// on a framework catalog bucket this boot does not use, and returns its error
// if it fails. Then it builds the graph mutation client for contracts with
// projection.NewMutationClient. It starts no goroutine.
//
// # OpenAPI
//
// A service that serves HTTP implements HTTPHandler, whose OpenAPISpec method
// describes its endpoints with OpenAPISpec, PathSpec, OperationSpec,
// ParameterSpec, ResponseSpec, TagSpec and Schema. RegisterOpenAPISpec adds a
// spec to a process-wide registry, keyed by service name; call it from the
// composition root, never from an init function, since importing a package
// registers nothing. GetAllOpenAPISpecs returns a copy of the registry's map
// whose values are the registered pointers themselves.
//
// # Configuration helpers
//
// ResolveServiceConfigs turns the services section of a configuration into
// the map a service manager starts from: each entry's config canonicalized,
// and component-manager, service-manager and metrics added, enabled, when the
// configuration leaves them out. It does not read inside a service's config;
// the service that decodes it does. PendingServiceChange,
// CompositionSealedError, DuplicateServiceError and
// MandatoryServiceDisabledError are what the service manager reports about
// that map. ConfigSchema, PropertySchema and NewConfigSchema describe a
// service's configuration keys; SchemaFromType and TypeNameFromReflect build
// the JSON schema of a Go type for an OpenAPI document.
package service
