// Package service holds what every long-running SemEngine service shares:
// BaseService, which a service embeds for its name, status and health check,
// and the Service interface a service manager drives. It also holds the helpers
// a composition root (the process's main function, which builds and wires
// everything) calls itself: MaybeStartPProf, WireGraphRuntime and the OpenAPI
// registry, plus the service-configuration and schema helpers the service
// manager reads.
//
// This package is ported from SemStreams in seven pull requests. This first
// one carries BaseService and the shared files; the service manager, the
// component manager and the services arrive with the later ones.
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
// logger (slog.Default() with a "service" attribute).
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
