# service

Package `service` holds what every long-running SemEngine service shares: `BaseService`, which a service embeds for
its name, status and health check, and the `Service` interface a service manager drives. It also holds the helpers a
composition root (the process's `main` function, which builds and wires everything) calls itself: `MaybeStartPProf`,
`WireGraphRuntime` and the OpenAPI registry, plus the service-configuration and schema helpers the service manager
reads.

The package is ported from SemStreams in seven pull requests. This first one carries `BaseService` and the shared
files. The service manager, the component manager and the services themselves arrive with the later ones; until then
nothing in this repository starts a `Service` for you.

## Embedding BaseService

A service embeds the `*BaseService` that `NewBaseServiceWithOptions` builds from its name and options:

```go
type ruleConfigService struct {
    *service.BaseService
}

svc := &ruleConfigService{
    BaseService: service.NewBaseServiceWithOptions("rule-config",
        service.WithNATS(client),
        service.WithMetrics(registry),
        service.WithLogger(logger),
    ),
}
```

| Option | What it does |
| --- | --- |
| `WithNATS(client)` | the health check also fails while `client.IsHealthy()` is false |
| `WithMetrics(registry)` | every status change is recorded on `registry` (`CoreMetrics().RecordServiceStatus`) |
| `WithLogger(logger)` | replaces the default logger, `slog.Default()` with a `service` attribute; `nil` keeps the default. `BaseService` itself logs nothing; the services in this package log through it |

### Start and Stop

`Start(ctx)` reports `StatusRunning` and runs the health check at once and then every 30 seconds. It refuses a `nil`
or ended `ctx`, a second `Start`, and a `Start` after any `Stop` (`errs.ErrAlreadyStarted`). A `BaseService` runs
once: build a new one to start again. Once `Start` has returned, ending its `ctx` stops nothing; `Stop` is the only
stop (ADR-113).

`Stop(ctx)` reports `StatusStopping`, starts no further health check, and returns `nil` once the health goroutine has
exited, leaving `StatusStopped` and `IsHealthy()` false. If `ctx` ends while it waits, `Stop` returns `ctx`'s error
and a later `Stop` waits again. A `ctx` that has already ended stops nothing: `Stop` returns its error at once and the
service keeps running. So give `Stop` a fresh bounded context, never the signal context that is ending the program:

```go
stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
if err := svc.Stop(stopCtx); err != nil {
    logger.Warn("rule-config stop", "error", err)
}
```

A `Stop` after one that returned `nil` returns `nil`. A `Stop` while another `Stop` is still waiting is refused as a
transient error (`errs.IsTransient`). `Stop` before `Start` returns `nil`, and the service then refuses to start.

A service that relied on the end of its `Start` context to stop its health loop, as SemStreams' `BaseService`
allowed, now calls `Stop`.

### Health

The health check fails when the function set with `SetHealthCheck` returns an error, or when the client from
`WithNATS` is not healthy. `IsHealthy()` is then false and `Health()` reports unhealthy; the next passing check makes
the service healthy again. While the last check passed, `Health()` reports a running service as healthy and a
stopping one as degraded; a stopped service is unhealthy. `GetStatus()` returns the counts of checks run and failed.

## The profiling server

`MaybeStartPProf(debug, port, logger)` serves `http.DefaultServeMux` on `:port`, on every interface, when `debug` is
true and `port` is positive, and returns `nil` otherwise. The pprof handlers are on that mux only if the caller's
`main` has the blank import `import _ "net/http/pprof"`; importing `service` does not add them. `MaybeStartPProf`
binds the port before it returns. A bind failure is logged at Warn on `logger` (`slog.Default()` when `nil`) and
returns `nil`: profiling is a debug aid, never a reason to fail the boot.

It is not a `Service`. The composition root calls it early, before connecting to NATS, so that a slow boot can be
profiled (SemStreams ADR-058, carried as [docs/adr/058-boot-lifecycle-phases.md](../docs/adr/058-boot-lifecycle-phases.md)).
It shuts the server down on its exit path, after its services have stopped:

```go
pprofServer := service.MaybeStartPProf(debug, port, logger)

// ... start the services, run, stop the services ...

stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := pprofServer.Shutdown(stopCtx); err != nil {
    logger.Warn("pprof shutdown", "error", err)
}
```

`Shutdown(ctx)` stops accepting connections and returns `nil` once every request in progress has finished and the
serving goroutine has exited. If `ctx` ends first it returns `ctx`'s error, and a later `Shutdown` waits again.
`Shutdown` on a `nil` `*PProfServer` returns `nil`, so the code above is safe whatever `MaybeStartPProf` returned. A
`nil` context is refused, on a `nil` server too.

## The graph runtime

`WireGraphRuntime(ctx, client, logger, contracts...)` builds the graph mutation client a composition root hands to the
code that writes graph state through graph-ingest (ADR-110):

```go
mutations, err := service.WireGraphRuntime(ctx, client, logger, contracts...)
if err != nil {
    return err
}
```

It first runs `kvcatalog.EnsureCatalogRetentionClean`, which removes a retention limit, such as a TTL, left on a
framework catalog bucket whose entries must never expire (SemStreams ADR-068 D1), and returns its error if that
fails; a `nil` context or client is refused there. Then it builds the client with `projection.NewMutationClient`. It
starts no goroutine and owns nothing that needs stopping.

## OpenAPI specs

A service that serves HTTP implements `HTTPHandler`: `RegisterHTTPHandlers(prefix, mux)` adds its routes, and
`OpenAPISpec()` describes them with `OpenAPISpec`, `PathSpec`, `OperationSpec`, `ParameterSpec`, `RequestBodySpec`,
`ResponseSpec`, `TagSpec` and `Schema`. `OpenAPIDocument` and its parts are the whole document a service manager
serves.

`RegisterOpenAPISpec(name, spec)` adds a spec to a registry shared by the whole process, keyed by service name. Call
it from the composition root, never from an `init` function: importing a package registers nothing.
`GetAllOpenAPISpecs()` returns a copy of the registry's map, but its values are the registered pointers themselves, so
a change made through one is seen by every later caller.

## Configuration helpers

These are read by the service manager, which arrives in the next pull request.

- `ResolveServiceConfigs(configs)` turns the `services` section of a configuration into the map a service manager
  starts from. Each entry's `config` is rewritten in one canonical JSON form, and `component-manager`,
  `service-manager` and `metrics` are added, enabled, when the configuration leaves them out (an explicit
  `"enabled": false` stays). It never reads inside a service's `config`; the service that decodes it does.
- `PendingServiceChange`, `CompositionSealedError`, `DuplicateServiceError` and `MandatoryServiceDisabledError` are what
  the service manager reports about that map.
- `ConfigSchema`, `PropertySchema` and `NewConfigSchema` describe a service's configuration keys. `SchemaFromType` and
  `TypeNameFromReflect` build the JSON schema of a Go type for an OpenAPI document.

A key in a service's `config` written in another letter case (`"Port"` for `"port"`) is accepted today. `config`
already refuses such keys in its own document; the service manager is to adopt the same check when it arrives.
