// Package service holds what every long-running SemEngine service shares:
// BaseService, which a service embeds for its name, status and health check,
// and the Service interface a service manager drives. Manager is that service
// manager: it builds the services a configuration names, starts them, serves the
// HTTP routes around them and stops them. ComponentManager is the service that
// owns the components. Metrics is the service that serves the Prometheus
// endpoint. MessageLogger is the service that keeps the newest messages seen on
// chosen NATS subjects for inspection. StorageObservabilityService is the
// service that reports how full the NATS account's storage is. HeartbeatService
// is the service that logs a liveness line at a fixed interval, and
// MetricsForwarder is the service that publishes the process's metrics on NATS.
// The package also holds the helpers a composition root (the process's main function, which
// builds and wires everything) calls itself:
// MaybeStartPProf, WireGraphRuntime and the OpenAPI registry, plus the
// service-configuration and schema helpers the service manager reads.
//
// This package is ported from SemStreams in seven pull requests. The first
// carried BaseService and the shared files; the second Manager and Metrics; the
// third ComponentManager, the service that owns the components (see the
// component manager section); the fourth MessageLogger (see the message logger
// section); the fifth StorageObservabilityService (see the storage observability
// section); this one carries HeartbeatService and MetricsForwarder (see the
// heartbeat and metrics forwarder sections). The last service arrives with the
// seventh.
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
//	if err := service.RegisterAll(services); err != nil { // every built-in service
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
// # The message logger
//
// MessageLogger subscribes to NATS subjects and keeps the newest messages in
// memory so a developer can inspect what flows through a running system. It
// also reads NATS KV buckets over HTTP. It writes nothing to NATS or the graph.
// It is off unless the configuration enables it: ResolveServiceConfigs does not
// add it. RegisterAll registers its constructor, NewMessageLoggerService, and
// its OpenAPI spec. Its config keys are monitor_subjects ("*" stands for the
// NATS, NATS-request and JetStream port subjects of every admitted component;
// empty means ["*"]), max_entries (0 to 100000; 0 means 10000), sample_rate
// (store one message in every sample_rate; 0 and 1 store all) and
// output_to_stdout. Construction refuses an unknown key (or one in another
// letter case), a nil NATS client, a nil payload registry, "*" without a
// component registry, max_entries outside 0 to 100000 and a negative
// sample_rate, naming the problem.
//
// Start works out the subject set once and checks every subject against the
// grammar of internal/flowgraph before it starts the base service or makes any
// NATS call: tokens separated by ".", none empty; "*" and ">" only as a whole
// token and ">" only last; no space, tab, line feed, carriage return or form
// feed. That is stricter than NATS, which takes "a*" as plain text and accepts a
// form feed. A subject that fails the check refuses Start as invalid data,
// naming the subject and its source (monitor_subjects, or the declaring
// component and port), and holds nothing. A subject another subject covers
// is not subscribed and is listed in /stats under subject_overlaps; two
// subscribed subjects can still overlap without either covering the other, and
// a message on both is stored twice.
//
// Start refuses a nil or ended ctx, a second Start, and a Start after any Stop
// (errs.ErrAlreadyStarted); ctx bounds Start only and ending it afterwards stops
// nothing (ADR-113). A subscribe that fails does not fail Start: the subject is
// retried every 100 ms until it succeeds. Meanwhile /stats reports
// subject_reconciliation_degraded true with subject_reconciliation_error, and
// the logger's health check fails naming the subjects, so IsHealthy turns false
// and /readyz answers not ready; health checks run every 30 seconds, so each
// change shows within one interval. Stop is the only stop: it ends the retry,
// drains every subscription so a message being handled is stored, ends every
// open KV watch stream and stops the base service. It refuses a nil ctx and,
// given an ended ctx, returns that error and stops nothing; if ctx ends first
// the next Stop finishes the rest.
//
// A message that decodes as a typed envelope is stored under its type name and
// counted in valid_messages; any other is stored with type "raw" and counted in
// invalid_messages. A payload that is not JSON is stored as a JSON string.
// /stats also reports total_messages, sampled_messages, sample_rate,
// max_entries, start_time, last_message_time, uptime_seconds,
// monitored_subjects (empty after Stop) and subject_overlaps.
//
// The routes, mounted under /message-logger and answering GET and HEAD (405
// for any other method), are entries (newest first; limit defaults to 100, capped at
// 10000; subject is a NATS pattern applied before the limit, and a pattern NATS
// cannot parse matches nothing), stats, subjects, trace/{traceID} (32 hex
// characters, else 400; the entries still held, oldest first), kv/{bucket} (an
// existing bucket's entries, keys sorted before limit, default 100, capped at
// 1000; pattern is a key glob; status keeps records whose top-level JSON
// status equals it; 404 for a missing bucket, which is never created) and
// kv/{bucket}/watch. The watch answers 503 once Stop has begun, 400 for a bad
// bucket name or key filter, 404 for a missing bucket. It then streams Server-Sent
// Events: connected, one kv_change per key's current value, a kv_change with
// operation initial_sync_complete, then each change with operation put, delete
// or purge. Each stream buffers 100 events for a slow client. If an event does
// not fit, the stream sends the buffered events, one error event
// {"error":"watch fell behind: events were dropped"} and ends, so no
// kv_change and no initial_sync_complete follows a dropped event. The logger
// does not read Last-Event-ID; a client that reconnects gets a new initial
// replay.
//
// # The storage observability service
//
// StorageObservabilityService reports how full the NATS account's storage is.
// Each interval it lists the account's JetStream streams and key-value buckets
// (its resources), derives usage, growth and pressure for each, and publishes
// the result as a report in the STORAGE_REPORT bucket. A second loop watches
// that bucket and feeds three surfaces from one view: Prometheus metrics, the
// health message and GET /storage-observability/report. It writes no graph
// entity.
//
// It is report-only. It rejects no write, throttles no component, fails no
// readiness or health check and applies no retention because of pressure; the
// pin's storage observability spec defers capacity admission until four
// conditions hold (the report runs in production, the projection is checked
// against outcome on at least three real resources, no critical state clears
// without an operator, and the rejection path classifies transient and NAKs),
// and a consumer that wants backpressure reads the report and decides itself.
//
// It is off unless the configuration enables it: ResolveServiceConfigs does not
// add it. RegisterAll registers its constructor, NewStorageObservabilityService,
// under storage-observability and no OpenAPI spec, because the route's spec
// names no schema. It needs Dependencies.NATSClient and
// Dependencies.MetricsRegistry. Its keys are interval (default 1m), timeout
// (default 15s, at most the interval) and pressure_thresholds (warning_headroom
// 0.25, high_headroom 0.15, critical_headroom 0.05, warning_horizon 72h,
// high_horizon 24h, critical_horizon 4h). Construction refuses, as invalid data
// naming the input, an unknown key (nested keys and another letter case
// included), nil dependencies, a nil NATS client, a nil metrics registry, an
// interval or timeout that does not parse or is not positive, a timeout above
// the interval and thresholds that are out of order or out of range.
// Configuration is read once; restart to change it.
//
// Start refuses a nil or ended ctx, a second Start and a Start after any Stop
// (errs.ErrAlreadyStarted), and does no I/O: it starts the collection loop and
// the watch loop and returns, so a deployment still boots when the account
// cannot be read; each loop retries on its own schedule (the watch every 5
// seconds). ctx bounds Start only (ADR-113). Stop is the only stop: it ends both
// loops, waits for them and stops the base service, and if its ctx ends first
// the next Stop waits again. How soon a collection already inside the account
// listing sees Stop begin depends on nats.go, which does not promise it.
//
// Health returns the base service's verdict untouched and appends the storage
// picture to its message, so critical pressure leaves /readyz answering READY
// and /health answering 200. After Stop the verdict is the base service's own.
// GET /storage-observability/report (HEAD is answered as GET, other methods get
// 405) always answers 200 with report_only, synced, updated_at (once a change
// was read), a summary (resources, pressure, not_evaluated, and worst_pressure
// when anything was evaluated), account (once an account row was read) and
// every published row in resources, unfiltered.
//
// The 19 metrics are named semengine_storage_*, where the SemStreams pin used
// semstreams_storage_*. A value that was not measured has no series, and a
// series is deleted when its value goes away. configs/prometheus/rules holds
// example alert rules over these names; the engine does not load them. See
// service/README.md for the full list.
//
// # The heartbeat service
//
// HeartbeatService logs one line, "System heartbeat", at Info when it starts
// and then every interval, so an operator can see the process is alive. The
// line carries uptime, goroutines, components_healthy and components_total.
// It is on by default: the configuration loader's defaults enable a heartbeat
// service at 30 s, so a composition that names no services still gets it. Its
// only key is interval (default 30s, at least 1s; an empty value takes the
// default). Construction refuses an unknown key, a key in another letter case,
// nil dependencies, a nil service manager and a bad interval, as invalid data
// naming the input. Start refuses, naming component-manager, when the service
// manager holds no *ComponentManager; that manager need only be composed. Start
// bounds Start only and Stop is the only stop. See service/README.md.
//
// # The metrics forwarder
//
// MetricsForwarder publishes the metrics registry it is given on NATS, at
// Start and then every push_interval (default 5s). Each sample is one JSON
// object {timestamp, name, component, type, value, labels} on
// metrics.<component>.<name>, where component is the sample's component label,
// else "system". The go_* and process_* families are left out unless
// include_go_metrics or include_proc_metrics is set. It is off unless the
// configuration enables it. A sample that does not encode or publish is
// counted on semengine_errors_total with type metrics_publish, a failed gather
// with type metrics_gather, and the loop goes on. Construction refuses an
// unknown key, a key in another letter case, nil dependencies, a nil NATS
// client, a nil metrics registry and a bad push_interval. See service/README.md.
//
// # The metrics service
//
// Metrics serves the Prometheus endpoint. Its config keys are port (default
// 9090) and path (default "/metrics"). Started by itself, it binds the port
// between Start and Stop. Composed under a Manager, the Manager claims the server
// before any service starts, serves it until StopAll, and Metrics reports healthy
// while the server runs. A Metrics runs once: a second Start, and a Start after
// any Stop, are refused with errs.ErrAlreadyStarted, and the context given to
// Start bounds Start only. RegisterAll registers Metrics, the component manager,
// the message logger, storage observability, the heartbeat and the metrics
// forwarder.
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
// that map. SchemaFromType and TypeNameFromReflect build the JSON schema of a
// Go type for an OpenAPI document.
package service
