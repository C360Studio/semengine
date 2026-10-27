# service

Package `service` holds what every long-running SemEngine service shares: `BaseService`, which a service embeds for
its name, status and health check, and the `Service` interface a service manager drives. `Manager` is that service
manager: it builds the services a configuration names, starts them, serves the HTTP routes around them and stops
them. `ComponentManager` is the service that owns the components. `Metrics` is the service that serves the Prometheus
endpoint. `MessageLogger` is the service that keeps the newest messages seen on chosen NATS subjects for
inspection. `StorageObservabilityService` is the service that reports how full the NATS account's storage is.
`HeartbeatService` is the service that logs a liveness line at a fixed interval, `MetricsForwarder` is the service
that publishes the process's metrics on NATS, and `LogForwarder` is the service that checks the log-forwarding
settings and says it forwards nothing yet. The package also holds the helpers a composition root (the process's
`main` function, which builds and wires everything) calls itself: `MaybeStartPProf`, `WireGraphRuntime` and the
OpenAPI registry, plus the service-configuration and schema helpers the service manager reads.

The package is ported from SemStreams in seven pull requests. The first carried `BaseService` and the shared files;
the second `Manager` and `Metrics`; the third `ComponentManager`, the service that owns the components
([The component manager](#the-component-manager)); the fourth `MessageLogger`
([The message logger](#the-message-logger)); the fifth `StorageObservabilityService`
([The storage observability service](#the-storage-observability-service)).

The sixth carried `HeartbeatService` ([The heartbeat service](#the-heartbeat-service)) and `MetricsForwarder`
([The metrics forwarder](#the-metrics-forwarder)). Of the two, only the heartbeat is on by default; the forwarder, like
the message logger and storage observability, runs only when the configuration enables it.

The seventh pull request, this one and the last, carries `LogForwarder` ([The log forwarder](#the-log-forwarder)). It
forwards nothing yet. It is the last service the port carries.

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

## The service manager

### Composing and starting

A composition root builds a `Manager` from a `Registry` of service constructors, hands it the `services` section of
the configuration, and starts it:

```go
services := service.NewServiceRegistry()
if err := service.RegisterAll(services); err != nil { // offers every built-in service
    return err
}

manager := service.NewServiceManager(services)
manager.UseHTTPMiddleware(authMiddleware) // optional; before StartAll

deps := &service.Dependencies{
    NATSClient: client, MetricsRegistry: registry, Logger: logger,
    Platform: platform, Manager: configManager,
    ComponentRegistry: components, PayloadRegistry: payloads,
}
if err := manager.ConfigureFromServices(cfg.Services, deps); err != nil {
    return err
}
if err := manager.StartAll(startCtx); err != nil {
    return err
}

// ... run ...

stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
if err := manager.StopAll(stopCtx); err != nil {
    logger.Warn("stop services", "error", err)
}
```

`ConfigureFromServices` resolves the configuration with `ResolveServiceConfigs` and constructs every enabled service
from its registered constructor. Nothing is composed unless every constructor succeeds. `component-manager` and
`service-manager` may not be disabled, and a composition whose `component-manager` has no registered constructor is
refused. `RegisterAll` registers one, and the component manager needs the `Manager`, `ComponentRegistry`, `Platform` and
`PayloadRegistry` fields shown above (see [Composing it](#composing-it)). `CreateService` and `RegisterInstance`
compose one more service by hand, and `GetService` finds a composed one.

The `service-manager` entry configures the `Manager` itself (`ManagerConfig`):

| Key | Default | What it does |
| --- | --- | --- |
| `http_port` | `8080` | the main listener's port, on every interface |
| `swagger_ui` | `false` | serves `/docs` when true |
| `server_info` | `SemEngine API`, `SemEngine framework API`, `0.0.0` | `title`, `description` and `version` of the API document |
| `http_read_timeout` | `30s` | how long a request body may take (a Go duration string) |
| `http_write_timeout` | `120s` | how long a response may take |

A service's `config` is decoded strictly. An unknown key is refused, and so is a key that differs from a field's name
only in letter case (`"Port"` for `"port"`), with both spellings named in the error. This holds for the `service-manager`,
`metrics` and `component-manager` entries, which this package decodes. A service defined elsewhere decodes its own
`config`, and the check applies to it only if it makes it.

### Starting and stopping

`StartAll(ctx)` creates `component-manager` if no service has that name, then seals the composition:
`CreateService`, `RegisterInstance` and `ConfigureFromServices` are refused from then on. It binds the main listener
on `http_port`, starts the metrics server it claims, starts every service in registration order, builds the full
route set, and starts a publisher that sends each service's health on `health.service.<name>` every 5 seconds. It
runs once: a second `StartAll`, and a `StartAll` after `StopAll`, are refused with `errs.ErrAlreadyStarted`. If a step
fails, `StartAll` stops what it started and returns both errors; if that stop fails too, `StopAll` runs it again.

The context given to `StartAll` bounds `StartAll` and nothing else. Ending it after `StartAll` returned stops no
service, and no request's context derives from it (ADR-113). `StopAll(ctx)` is the only stop. It stops the services
in reverse registration order, then the health publisher, the listeners and the metrics server. Each listener
refuses new requests and `StopAll` waits for those in flight. If `ctx` ends first, `StopAll` closes every connection
still open and returns `ctx`'s error, and a later `StopAll` waits again for the handlers that are still running. So a
service that serves a long-lived response, such as a stream, ends it in its own `Stop`.

A `StopAll` given a context that has already ended stops nothing: give it a fresh bounded context, never the signal
context that is ending the program. A service whose `Stop` returns `errs.ErrAlreadyStopped` counts as stopped. Any
other failure is returned naming the service, and the next `StopAll` runs the whole stop again. A `StopAll` after one
that returned `nil` returns `nil`; one while another runs is refused as transient.

`StartHealthListener(ctx, port)` binds a second listener that serves only `/health` and `/healthz`, for container
probes that want a port apart from `http_port`. Call it before `StartAll` and probes work while a slow boot runs.
Port 0 does nothing, a second call is refused, and `StopAll` stops the listener whether or not `StartAll` ran.

### Middleware

`UseHTTPMiddleware(mws...)` wraps every route the main listener serves, the startup routes as well as the full set.
The first middleware passed is the outermost. The framework ships none: authentication, request logging, panic
recovery, rate limiting and CORS are the product's. The dedicated health listener takes no middleware. Call it before
`StartAll`; a later call is ignored with a Warn, because the listener's handler is built when it binds. This is
phase 1 of SemStreams ADR-030, carried as
[docs/adr/030-http-middleware-and-identity-pattern.md](../docs/adr/030-http-middleware-and-identity-pattern.md).

### Boot seals the composition

A change to the `services` section of the configuration after `StartAll` is desired state for the next boot. It
starts, stops and reconfigures nothing, and `/services` reports it with `restart_required` set. This is the part of
SemStreams ADR-094 that the `Manager` carries, in
[docs/adr/094-boot-only-composition-and-observable-rule-activation.md](../docs/adr/094-boot-only-composition-and-observable-rule-activation.md).

### The routes

The main listener serves the first five routes below from the moment `StartAll` binds it, and answers every other path
`503 NOT READY` until `StartAll` has started every service and committed the full route set. The last four come with
the commit.

| Route | Answer |
| --- | --- |
| `/health` | every service's health and the NATS connection's, aggregated; 503 when unhealthy |
| `/healthz` | `200 OK` while the listener serves |
| `/readyz` | `200 READY` once every service has started and is running and healthy; `503 NOT READY` while starting, after a start failed, and from the moment `StopAll` is entered, even while a service's `Start` is still running |
| `/services` | each service's status and health, the startup counts, and `restart_required` with the pending configuration changes |
| `/services/health` | each service's health and their aggregate; 503 when unhealthy |
| `/graph/triples` | triples read from the graph's `ENTITY_STATES` bucket (below) |
| each `HTTPHandler` service's routes | under a prefix taken from the service's name: the name with its hyphens removed, except `component-manager` at `/components` and `message-logger` and `storage-observability` as named |
| `/openapi.json` | the API document, in OpenAPI 3.0 form: the paths of every `HTTPHandler` service under its prefix, and the schemas of the types registered with `RegisterOpenAPISpec` |
| `/docs` | a Swagger UI page for `/openapi.json`, only when `swagger_ui` is true; it loads its script and style sheet from `unpkg.com`, so the browser needs network access to it |

The counts `/services` reports are also exported as the gauge `semengine_startup_units`, with an `owner` label
(`services` or `components`) and a `stage` label. It has nine series: `admitted`, `starts_invoked`, `starts_completed`
and `starts_failed` for services, and those four plus `lifecycle_participants` for components. The gauge is read from
the same counts each time Prometheus collects, so it is never older than `/services`. `StartAll` registers it on the
injected metrics registry, else on the `metrics` service's, and registers nothing when neither exists.

`GET /graph/triples` is for an operator or a test, not a query API: it scans the bucket. Its query parameters are all
optional:

| Parameter | Meaning |
| --- | --- |
| `subject` | a literal entity ID |
| `predicate` | a three-part predicate |
| `object` | compared as text |
| `limit` | default 100, at most 1000 |

It answers a JSON array sorted by entity ID, never `null`. A malformed parameter is `400` naming the parameter, a
method other than `GET` is `405`, and a bucket that cannot be read is `500`. With no NATS client in the
`Dependencies`, it answers `[]` for every query: there is no store to read. The `Manager` only reads the bucket
(ADR-110).

## The component manager

A *component* is one unit of a running composition: an input, processor, output or storage unit. A binary makes the
component factories available through a `component.Registry`, and the `components` section of the configuration names
the instances to build. `ComponentManager` is the service that builds every enabled component, starts them, serves
their status over HTTP and stops them. Every composition has one: `component-manager` may not be disabled.

### Composing it

`RegisterAll` registers the constructor, `NewComponentManager`, and the component manager's OpenAPI spec (see
[OpenAPI](#the-component-managers-openapi-spec)). The `component-manager` entry takes no configuration: `"config": {}`
or none at all. Any key is refused, naming it. The components come from the `Dependencies` the composition root passes
to `ConfigureFromServices`:

| `Dependencies` field | What the component manager does with it |
| --- | --- |
| `Manager` (`*config.Manager`) | required; the boot configuration it reads, once |
| `ComponentRegistry` | required; the factories it builds from, and the registry it seals |
| `PayloadRegistry` | given to each component |
| `NATSClient` | given to each component; with none, the manager publishes no component health |
| `Platform` | the deployment's identity, given to each component |
| `MetricsRegistry`, `Logger` | given to each component; the manager's own metrics and log lines |

`NewComponentManager` reads the boot configuration once, and later writes to it change nothing in the running set: they
are desired state for the next boot (SemStreams ADR-094 and ADR-096, carried in
[docs/adr](../docs/adr/094-boot-only-composition-and-observable-rule-activation.md)). It builds each enabled component
through `Registry.CreateComponent`, in instance-name order, and skips the disabled ones. It then checks the whole set
with `composition.Analyze` and seals the registry (ADR-100). It returns an error, and no service, when:

- the `Manager` or the `ComponentRegistry` is missing;
- any enabled component cannot be built. The error names every one that failed, because a boot that quietly runs
  without a configured component would report ready while doing less than it was told;
- the composition has an error-severity finding. The error reads `composition validation refused boot with N error
  finding(s): ...` and names each finding.

### Starting and stopping

`Start(ctx)` refuses a `nil` or ended `ctx`, a second `Start`, and a `Start` after any `Stop`
(`errs.ErrAlreadyStarted`). The start order is fixed when the manager is built: components that provide stores
(`component.StoreProvider`) by instance name, then the rest by instance name. `Start` runs in two phases, providers
first. Each phase starts its components concurrently, and the next phase begins only after every `Start` in the
current one returned, so a provider's stores are in the shared store registry before any consumer starts. A failure in
the first phase starts nothing of the second. A component that does not implement `component.LifecycleComponent` is
built and listed but never started or stopped.

Each component's `Start` receives the context the manager's `Start` received, the same value. The manager keeps no
context of its own for a component and never ends the one it was given, `Stop` included (ADR-113). A component
therefore makes its own run context in `Start` and ends it in `Stop`; a component that hangs its continuing work on its
`Start` context stops when the caller ends that context, which for a program that passes its signal context is the
start of shutdown, before `Stop` reaches the component. For the same reason ending the manager's `Start` context after
`Start` returned stops nothing. The manager's health publisher and override-expiry reporter (below) run under a run
context that `Start` creates and `Stop` ends.

A failed `Start` returns every component failure joined, each naming its component, after one rollback: it stops every
component it launched, the failing ones included, in reverse start order. A component whose `Start` returned `nil`
after `ctx` ended counts as failed. If the rollback fails too, its error is joined into `Start`'s and the next `Stop`
runs it again.

`Stop(ctx)` is the only stop. It refuses a `nil` `ctx`. Given an ended `ctx` it returns that error and stops nothing,
so give it a fresh bounded context, never the signal context that is ending the program. Before any `Start` it returns
`nil`, and a later `Start` is refused. Otherwise it marks the stop begun (`/readyz` answers `503 NOT READY` from then),
stops each started component with `ctx` in reverse start order (just before a provider's own `Stop`, it removes that
provider's stores from the store registry), then ends the manager's own loops and its base service. It returns `nil`
only once everything is released. A component whose `Stop` failed, or was not reached because `ctx` ended, is stopped
again by the next `Stop`; one whose `Stop` returned `nil` is not called again. A `Stop` after one that returned `nil`
returns `nil`, and a `Stop` while another runs is refused as transient. A `Stop` that arrives while `Start` is still
running waits for every component's `Start` to return, or returns its own context's error first; if that `Start`
failed, it then stops only what the rollback left. A caller cuts a slow boot short only by ending the context it gave `Start`.

### Reading status during and after Stop

`GetComponentHealth`, `GetComponentStatus`, the `/components` routes and the manager's own health check wait on no lock
and not on `Start` or `Stop`. Each reads the component's record and asks the component itself, so a request that
arrives during or after `Stop` gets whatever the component answers then. SemStreams kept such requests away from a
stopping component; this manager does not. The line a component author holds is in `component.LifecycleComponent`:
`Health`, `DataFlow` and `DebugStatus` may be called at any time and return without waiting for `Start` or `Stop`. The
tree enforces it for graph-ingest (`TestGraphIngestHealthAndDataFlowAnswerDuringAndAfterStop`); for a component
outside this module it is review only, and its author checks it.

The manager's own health, which `BaseService` runs and `/readyz` reads, is unhealthy while any component is in the
failed state. It names each failed component in name order with its last error, and reads every component each time.

### What it publishes and reports

- **Component health.** With a NATS client, the manager publishes `{"timestamp","name","health"}` for each admitted
  component on `health.component.<name>`, once as `Start` returns and then every 5 seconds. A publish or encode that
  fails is counted on `semengine_errors_total{service="component-manager",type="health_publish"}` and logged at Debug
  with the subject. The next component and the next round are still published, because a missed report is replaced by
  the next one.
- **Stream migration overrides.** A `stream_migration_overrides` entry admits an existing unbounded stream as a named,
  time-limited bridge. The manager evaluates the live configuration when it is built and then every minute from
  `Start` to `Stop`, and sets the gauge `semengine_streams_migration_override_expired{stream,owner}` to 0 for each
  declared bridge and to 1 for a lapsed one, with a Warn line per lapsed bridge. It only reports: readiness is
  unaffected, and the refusal comes at the next boot.

### The routes

The routes are mounted under `/components` and answer `GET` only (`405` otherwise). A component name that is empty,
`.`, `..` or decodes to a path separator is `400`, and an unknown one is `404`. Until `StartAll` commits the full route
set, only `health`, `list` and `status/{name}` are served, and every other path answers `503 NOT READY`.

| Route | Answer |
| --- | --- |
| `/components/health` | each admitted component's health and their aggregate; `503` when any component is unhealthy |
| `/components/list` | each admitted component's name, state, factory, category, `enabled` and health, in no fixed order |
| `/components/types` | the registered factories with their schemas and default ports, as an array |
| `/components/types/{id}` | one factory's entry |
| `/components/status/{name}` | one component's state, health and last error; `?debug=true` adds its `DebugStatus` when it has one |
| `/components/config/{name}` | the boot configuration the instance was built from |
| `/components/flowgraph` | the composition's graph as JSON, or Mermaid text with `?format=mermaid` |
| `/components/validate` | the composition check's result: status, errors and warnings |
| `/components/paths` | for each input component, every component reachable from it, with counts |

All three read the composition result kept when the manager was built, so they cannot disagree with the check that
admitted the boot (ADR-100). `validate` serves that result as is. `flowgraph` serves its graph, as JSON or as
`composition.Mermaid`. `paths` serves `composition.FlowPaths` of its graph, with counts of that answer (the number of
inputs, the longest path, the total and the average). `GetFlowPaths` is the same answer as a call.
`BuildComponentTypeCatalog(registry, logger)` returns what `/components/types` serves, for a product that serves the
catalog elsewhere. `GetComponentStatus` and `ComponentStatus` are read inside this package only.

A declared difference you may meet: `GET /components/types/{id}` for an unregistered type answers `404` with the body
`{"error":"Component type <id> not found"}` sent as `text/plain`, not JSON, with the request's own path segment in it.
SemStreams answers it the same way, and no contract line promises the body, so read the status code and not the text.

### Rule packs

A rule processor (a component this repository has not ported yet) can declare *contracts* for the predicate groups it
writes, and replace a whole group with one reconcile call (`projection.PredicateReconciler`, implemented by
`projection.MutationClient`). It implements `ProjectionBinder` to receive that client. The component manager does not
hand it one; the composition root does, in this order, after `ConfigureFromServices` and before `StartAll`:

```go
if err := manager.ConfigureFromServices(cfg.Services, deps); err != nil {
    return err
}
if err := service.ConfigureRulePackMutations(manager); err != nil {
    return err
}
if err := manager.StartAll(startCtx); err != nil {
    return err
}
```

`ConfigureRulePackMutations` finds the admitted components that implement `ProjectionBinder` and runs every binder's
`PreflightProjectionMutations` before changing anything, so one failure injects nothing. It then validates each
`pack_id` and refuses a duplicate, copies and validates each pack's contracts, refuses contracts when there is no NATS
client, and builds one `projection.MutationClient` per pack that has contracts and hands it to that binder's
`SetPredicateReconciler`. If a composition root never makes the call, no binder is given a reconciler. No rule
processor is in this tree yet, so today that has no effect; what a rule processor does without one is stated when it
is ported. `ComponentsImplementing[T](manager)` returns the admitted components that implement `T`, in start order, and
`nil` before a component manager is composed. `Manager.ProjectionBinders` is that call for `ProjectionBinder`.

### The component manager's OpenAPI spec

`/openapi.json` takes its schemas, here `Graph` and `Result` for the flowgraph and validate responses, only from the
process-wide registry that `RegisterOpenAPISpec` fills. `RegisterAll` registers the component manager's spec there,
because importing the package registers nothing. A composition that registers `NewComponentManager` by hand instead of
calling `RegisterAll` must register the spec itself, or those two references name schemas the document does not carry:

```go
if svc, ok := manager.GetService("component-manager"); ok {
    service.RegisterOpenAPISpec("component-manager", svc.(service.HTTPHandler).OpenAPISpec())
}
```

The rule for any service that does this is in the developer contract: every `SchemaRef` type is listed in its spec's
`ResponseTypes` or `RequestBodyTypes`, and the composition root registers the spec.

### Not ported

`Initialize` (construction does that work), `IsInitialized`, `IsStarted`, `GetHealthyComponents`,
`GetUnhealthyComponents`, `DefaultComponentManagerConfig`, the configuration's `enabled_components` key and its
`Validate`, and the component manager's config subscribers and live-configuration routes, which ADR-094 retired.

## The message logger

`MessageLogger` is a service that subscribes to NATS subjects and keeps the newest messages it sees in memory, so a
developer can look at what flows through a running system. It also reads NATS KV buckets (NATS's key-value store) over
HTTP. It writes nothing to NATS and nothing to the graph.

### Enabling it

The message logger is off unless the configuration turns it on: `ResolveServiceConfigs` adds `component-manager`,
`service-manager` and `metrics` when they are missing, and does not add `message-logger`. `RegisterAll` registers its
constructor, `NewMessageLoggerService`, and its OpenAPI spec. Enable it in the `services` section:

```json
{
  "services": {
    "message-logger": {
      "enabled": true,
      "config": {
        "monitor_subjects": ["agent.>", "tool.>"],
        "max_entries": 10000
      }
    }
  }
}
```

Its `Dependencies` fields: `NATSClient` and `PayloadRegistry` are required, `ComponentRegistry` is required when
`monitor_subjects` holds `"*"`, and `MetricsRegistry` and `Logger` are optional.

| Key | Meaning |
| --- | --- |
| `monitor_subjects` | NATS subjects to watch. `"*"` stands for the NATS, NATS-request and JetStream port subjects of every admitted component (never `*`, `>` or `_INBOX` subjects). Empty means `["*"]`. |
| `max_entries` | How many messages to keep, 0 to 100000. 0 means 10000. The oldest are overwritten. |
| `sample_rate` | Store one message in every `sample_rate` and only count the rest. 0 and 1 store every message. |
| `output_to_stdout` | Also print one line per stored message. |

Construction refuses, naming the problem: an unknown key (or a key in another letter case), a nil NATS client, a nil
payload registry, `"*"` without a component registry, `max_entries` below 0 or above 100000, and `sample_rate` below 0.

### What it captures

`Start` works out the subject set once. Each subject is then checked against one grammar, owned by
`internal/flowgraph`: one or more `.`-separated tokens, none empty; `*` and `>` only as a whole token; `>` only as the
last; no space, tab, line feed, carriage return or form feed. This is stricter than NATS in two ways: NATS takes a token
such as `a*` as plain text and accepts a form feed, and the logger refuses both. A subject that fails the check
makes `Start` fail, as invalid data, with an error naming the subject and where it came from (`monitor_subjects`, or
the component and port that declare it). The check runs before the base service starts and before any NATS call, so a
refused `Start` holds nothing, connected or not.

A subject that another subject in the set covers (`a.>` covers `a.b`) is not subscribed, and the omission is listed
in `/stats` under `subject_overlaps`. Two subscribed subjects can still overlap without either covering the other
(`a.b.*` and `*.b.c` both match `a.b.c`); a message on such a subject is then stored twice.

Each message is stored as an entry: a sequence number, the time, the subject, the message type and summary, the
trace and span IDs when the message carries them, and the payload as `raw_data`. A message that decodes as a typed
envelope (a `message.BaseMessage` whose type is in the payload registry) is stored under its type name; any other is
stored with type `raw`. A payload that is not JSON is stored as a JSON string, so every entry can be served.

### Starting and stopping

`Start(ctx)` refuses a nil or ended `ctx`, a second `Start`, and a `Start` after any `Stop`
(`errs.ErrAlreadyStarted`); a refused `Start` holds nothing. `ctx` bounds `Start` only: ending it afterwards stops
nothing, and messages keep being stored, because each message's context belongs to the NATS client, not to `ctx`.
`Start` checks `ctx` before each subscribe and once more after the last; if `ctx` has ended at any check, `Start`
returns its error and releases what the logger holds. Ending `ctx` also ends a running subscribe's wait for the
server's answer, but not its waits for the connection itself (see "A write the server is not reading holds the
connection" in the transport-client spec), so how long `Start` can outlast its context is not promised.

A subscribe that fails for any other reason does not fail `Start`. The logger keeps running and retries that subject
every 100 ms until it succeeds. While any subject is unsubscribed:

- `/stats` reports `subject_reconciliation_degraded` as true and `subject_reconciliation_error` names each subject with
  its last error;
- the logger's health check fails, naming the subjects, so `IsHealthy` turns false and `/readyz` answers not ready.
  Health checks run every 30 seconds, so the change shows within one interval of the failure and again within one
  interval of the retry succeeding. On a normal boot the first check runs before any subscribe and is healthy.

`Stop(ctx)` is the only stop. It refuses a nil `ctx`; given an ended `ctx` it returns that error and stops nothing.
Otherwise it ends the retry, drains every subscription (a message being handled when `Stop` begins is stored), ends
every open KV watch stream, and stops the base service, returning nil once all are released. If `ctx` ends first, or a
drain fails, `Stop` returns the error and leaves what is left for the next `Stop`. A `Stop` after one that returned nil
returns nil; a `Stop` while another runs is refused as transient. Messages published after `Stop` are not stored.

### What `/stats` reports

| Key | Meaning |
| --- | --- |
| `total_messages` | Every message received and not dropped, sampled or not. |
| `dropped_messages` | Messages not stored (and not in `total_messages`) because their handler context had ended; the NATS client ends it once its `Close` has begun. Each is logged at Warn with its subject. |
| `sampled_messages` | Messages stored under `sample_rate`. |
| `valid_messages` | Stored messages that decoded as typed envelopes. |
| `invalid_messages` | Stored messages that did not (type `raw`). |
| `sample_rate`, `max_entries` | The configured values, after defaults. |
| `start_time`, `last_message_time`, `uptime_seconds` | Logger start, last message received, and seconds since start. |
| `monitored_subjects` | The subjects currently subscribed, sorted; empty after `Stop`. |
| `subject_overlaps` | Each omitted covered subject: `broader`, `covered`, and `resolution` (`covered subscription omitted`). |
| `subject_reconciliation_degraded` | True while a subject is unsubscribed. |
| `subject_reconciliation_error` | The unsubscribed subjects and their last errors; empty when none. |

### The routes

Mounted under `/message-logger/` and answering `GET` and `HEAD` (405 for any other method). `entries`, `stats` and
`trace` answer from memory at any time, including after `Stop`.

- `entries`: the newest entries, newest first. `limit` defaults to 100 and is capped at 10000; a missing, non-numeric,
  zero or negative value gives 100. `subject` is a NATS subject pattern (`*` one token, `>` the remaining tokens)
  applied before the limit; a pattern NATS cannot parse matches nothing. `subject=*` matches only one-token subjects:
  write `>` for all, and `agent.>` for everything under `agent`.
- `stats`: the object above.
- `subjects`: the subscribed subjects as a JSON array; `[]` after `Stop`.
- `trace/{traceID}`: the entries still held for a trace ID (32 hex characters, else 400), oldest first, as
  `{"trace_id", "count", "entries"}`. An entry the buffer has overwritten is gone.
- `kv/{bucket}`: the entries of an existing KV bucket, keys sorted before `limit` applies. `pattern` is a key glob: one
  `*` as prefix, suffix, both ends or in the middle, or none for an exact key; the default matches every key. `limit`
  defaults to 100 and is capped at 1000. `status` keeps only records whose top-level JSON `status` equals it. Values
  are answered as stored (a value that is not JSON as a JSON string). A bucket that does not exist is 404 and is never
  created; a bucket name that is empty, `.`, `..` or holds `/`, `\`, `*` or `>` is 400; a lookup, listing or read
  failure is 500, naming the key for a read.
- `kv/{bucket}/watch`: a stream of Server-Sent Events (SSE), described next.

### The watch stream

`kv/{bucket}/watch` answers 503 once `Stop` has begun, 400 for a bad bucket name or `pattern` (empty or `*` watches
every key; anything else must be a NATS KV key filter, and a final `>` is accepted), 404 for a missing bucket and 500
for another lookup failure; none of these opens a stream. Otherwise it sends:

1. `event: connected` with `{"bucket","pattern","message"}`, then `retry: 5000`;
2. one `event: kv_change` for each key's current value, then one with `operation` `initial_sync_complete`;
3. one `kv_change` per later change. `operation` is what NATS records for the entry: `put`, `delete` or `purge`.
   `value` is present for a `put` only.

The logger does not read `Last-Event-ID`: a client that reconnects gets a new initial replay. Each stream buffers up to
100 events between its KV watcher and the client. If the client reads too slowly and an event does not fit, the stream
ends: the events already buffered are sent, then one `event: error` with `{"error":"watch fell behind: events were
dropped"}`, and the response ends. No `kv_change`, and no `initial_sync_complete`, follows a dropped event, so a
client that sees `initial_sync_complete` has seen every initial value. Each stream runs its own KV watcher.

### Not ported

`GetMessages`, `ConfigSchema`, `DefaultMessageLoggerConfig`, the exported `NewMessageLogger` and `SetDecoder`, the
reconciliation of a changing subject set, and the test-only seams the pin carried. `service/configurable.go`
(`ConfigSchema`, `PropertySchema`, `NewConfigSchema`) is removed with them.

## The storage observability service

`StorageObservabilityService` watches how full the NATS account's storage is. Every minute it lists the account's
JetStream streams and key-value buckets (JetStream is NATS's persistent storage; each stream or bucket is one
"resource"), works out how much each holds, how fast that grows and how close it is to its limit, and publishes the
result as a report in the `STORAGE_REPORT` bucket. It then serves that one report three ways: Prometheus metrics, a
health message and an HTTP route. It writes no graph entity.

The service is report-only. It rejects no write, throttles no component, fails no readiness or health check and applies
no retention because of what it reports. A pressure state ("normal", "warning", "high" or "critical", set by how much
of a limit is still free and how soon growth would use it up) is a fact for a human or an alert rule to act on. The
SemStreams pin's storage observability spec records why nothing gates on pressure yet: capacity admission (refusing a
write when storage is tight) stays deferred until four conditions hold. The report runs in production; the projection
has been checked against what happened on at least three real resources; no critical state has cleared without an
operator acting; and the rejection path is proven to classify a refusal as transient and NAK it (ask the sender to
retry) instead of acknowledging and dropping the write. That text is the pin's
(`openspec/specs/storage-observability/spec.md:169-218`) and is not ported here. A consumer that wants backpressure
today reads the report and decides in its own code. The reasoning behind reporting through logs and metrics, not graph
triples, is [ADR-098](../docs/adr/098-observability-splits-by-subject.md).

### Enabling it

The service is off unless the configuration turns it on: `ResolveServiceConfigs` does not add it. `RegisterAll`
registers its constructor, `NewStorageObservabilityService`, under `storage-observability`, and no OpenAPI spec for it
(the route's spec names no schema). Enable it in the `services` section; every key is optional:

```json
{
  "services": {
    "storage-observability": {
      "enabled": true,
      "config": {
        "interval": "1m",
        "timeout": "15s",
        "pressure_thresholds": {"critical_headroom": 0.05}
      }
    }
  }
}
```

It needs `Dependencies.NATSClient` and `Dependencies.MetricsRegistry`; `Logger` is optional. The service manager passes
the registry it holds.

| Key | Meaning |
| --- | --- |
| `interval` | How often the account is listed. Default `1m`. Every process that polls multiplies the cost, so increase the interval to poll less often when many instances run. |
| `timeout` | The bound on one collection, both listings included. Default `15s`; it must not exceed `interval`. |
| `pressure_thresholds` | When a resource counts as `warning`, `high` or `critical`: `warning_headroom` (default 0.25), `high_headroom` (0.15) and `critical_headroom` (0.05) are the free fraction at or below which the state applies; `warning_horizon` (`72h`), `high_horizon` (`24h`) and `critical_horizon` (`4h`) are the projected time until the critical level is reached. |

Construction refuses, as invalid data naming the input: an unknown key (nested keys under `pressure_thresholds`
included, and a key in another letter case), nil dependencies, a nil NATS client, a nil metrics registry, an `interval`
or `timeout` that does not parse or is not positive, a `timeout` above the `interval`, and thresholds that are out of
order or out of range. Configuration is read once; to change a value, restart.

### Starting and stopping

`Start(ctx)` refuses a nil or ended `ctx`, a second `Start`, and a `Start` after any `Stop` (`errs.ErrAlreadyStarted`);
a refused `Start` holds nothing. It does no I/O and never fails on account or bucket state: it starts two background
loops and returns. One collects every `interval`. The other watches `STORAGE_REPORT` and feeds the metrics, the health
message and the route. The service creates or reconciles `STORAGE_REPORT` on first use, so a deployment that names this
service still boots when the account cannot be read; each loop retries on its own schedule (the watch every 5 seconds).
`ctx` bounds `Start` only: ending it afterwards stops nothing.

`Stop(ctx)` is the only stop. It ends both loops, waits for them and stops the base service, returning nil once all
three are done. If `ctx` ends first, `Stop` returns that error and the next `Stop` waits again. How soon a collection
already inside the account listing notices that `Stop` began depends on nats.go's listing honouring its context, which
nats.go does not promise; `Stop` stays bounded by its own `ctx` either way. A `Stop` after one that returned nil
returns nil; a `Stop` while another runs is refused as transient.

### The health message

`Health` returns the base service's verdict untouched and appends the storage picture to its message, joined with a semicolon.
Pressure never changes `Healthy`, `Status` or a sub-status, so critical pressure leaves `/readyz` answering `READY` and
`/health` answering 200. The picture is one line, for example
`storage pressure (report-only): critical=1 high=0 warning=0 normal=12 not-evaluated=0; worst: ORDERS critical
(headroom)`, then any over-committed tier, then how long ago the last collection ran. Before the first report is read
it says `storage report has not been read yet`; after a failed collection it says so and serves the last result. After
`Stop` the verdict is the base service's own (`Service is unhealthy (failed checks: 0)`) with the last picture appended.
Both reads do no I/O.

### The route

`GET /storage-observability/report` (mounted by the service manager; `HEAD` is answered as `GET`, any other method
gets 405) always answers 200, whatever the report says. The body:

- `report_only`: always `true`.
- `synced`: whether the watch has read every current value at least once. Until then an empty `resources` means "not
  read yet", not "the account holds nothing".
- `updated_at`: when the last change reached this process; absent before the first.
- `summary`: `resources` (a count), `pressure` (rows per evaluated state, `{}` when none), `not_evaluated` (rows with no
  pressure state because their capacity could not be read, or because they are unbounded and their tier has no limit
  either) and `worst_pressure` (absent when nothing was evaluated; absent does not mean normal).
- `account`: per storage tier (`file`, `memory`, `unknown`), the declared bounds against the account limit; absent until
  an account row is read.
- `resources`: every published row, sorted by name, never `null`, never filtered. A resource with no bound of its own is
  evaluated against its tier's account limit and says so in `pressure.evaluated_against` (`account-tier`): that state is
  shared by every unbounded resource in the tier, and changing one resource cannot relieve it.

The health message, the metrics and the route read one consumer of the report, so they cannot disagree.

### The metrics

The service registers 19 gauges on the injected registry under the name `storage-observability`, all named
`semengine_storage_*`. The SemStreams pin published the same series as `semstreams_storage_*`; nothing renames them for
you, so an alert or dashboard still on the old names reads nothing.

- Per resource, labelled `resource`, `owner`, `kind` and `tier`: `resource_used_bytes`, `resource_limit_bytes`,
  `resource_headroom_bytes`, `resource_headroom_ratio`, `resource_growth_bytes_per_second`,
  `resource_time_to_threshold_seconds` and `resource_pressure` (0 normal, 1 warning, 2 high, 3 critical).
- Counts: `resources` (by `tier` and `capacity_state`) and `pressure_resources` (by `tier` and `pressure_state`,
  including `not-evaluated`).
- Per storage tier, labelled `tier` (and `state` for the three that carry one): `account_limit_bytes`,
  `account_used_bytes`, `account_declared_bytes`, `account_limit_state`, `account_overcommitment`,
  `account_growth_bytes_per_second`, `account_headroom_bytes`, `account_time_to_threshold_seconds` and
  `account_pressure_state`.
- `report_collected_timestamp_seconds`: when the freshest row was collected (not scraped), so
  `time() - semengine_storage_report_collected_timestamp_seconds > 900` catches a collector that stopped. It is zero
  until a row with a collection time is read.

A value that was not measured has no series; it is never zero, because zero means "measured, and not growing". A series
is deleted when its value goes away, when a resource leaves the report, and, for the per-tier series, when the account
row is dropped. The count and state families are always emitted in full.

Example Prometheus alert rules are at `configs/prometheus/rules/storage-pressure.yml`: 13 alerts, each reading only
published names, none firing because a report row disappeared, with unbounded and unreadable resources kept visible.
Copy the file and tune it; the engine does not load it. `service/storage_observability_http_test.go` checks it against
the metric names.

### Not ported

`Snapshot` and `RegisterMetrics` (nothing read them), the `schema` struct tags on `StorageObservabilityConfig`, and the
pin's function fields and nil checks on the two surfaces.

## The heartbeat service

`HeartbeatService` writes one log line at a fixed interval so an operator can see the process is alive and how many
of its components are healthy. It is on by default: the configuration loader's defaults turn on a `heartbeat` service
with a 30 second interval, so a composition that names no services still gets it. `RegisterAll` registers its
constructor, `NewHeartbeatService`, under `heartbeat`, with no OpenAPI spec because it serves no route. To change the
interval, or to turn it off, name it in `services`:

```json
{
  "services": {
    "heartbeat": {"enabled": true, "config": {"interval": "1m"}}
  }
}
```

| Key | Meaning |
| --- | --- |
| `interval` | How often a line is written. Default `30s`; an empty value takes the default; it must be at least `1s`. |

It needs `Dependencies.ServiceManager`; `Logger` is optional. Construction refuses, as invalid data naming the input:
an unknown key or a key in another letter case, nil dependencies, a nil service manager, and an `interval` that does
not parse or is under `1s`. A miscased key matters because the loader merges your `services` block with the defaults:
`{"Interval":"5s"}` beside the default `{"interval":"30s"}` used to run at 30 seconds without a word; now the
composition is refused, naming both spellings. Configuration is read once; to change a value, restart.

The line is written at Info, once when the service starts and then once per interval. Its message is
`System heartbeat`, and it carries `uptime` (the service's, rounded to the second), `goroutines` (the process's
count), `components_healthy` and `components_total` (how many of the component manager's components report healthy,
out of how many it holds). The SemStreams pin wrote it at Debug, which shows nothing at the default log level; the
owner chose Info (2026-10-23).

`Start(ctx)` looks up the component manager through the service manager the heartbeat was built with and refuses,
naming `component-manager`, when there is none or it is not the framework's `*ComponentManager`; the component manager
need only be composed, not started. It also refuses a nil or ended `ctx`, a second `Start` and a `Start` after any
`Stop` (`errs.ErrAlreadyStarted`); a refused `Start` holds nothing. It starts one background loop and returns; `ctx`
bounds `Start` only, and ending it afterwards stops nothing. `Stop(ctx)` is the only stop: it ends the loop, waits for
it and stops the base service. If `ctx` ends first, `Stop` returns that error and the next `Stop` waits again. A
`Stop` after one that returned nil returns nil; a `Stop` while another runs is refused as transient.

### Not ported

`HeartbeatConfig.Validate`, the test-only constructor and the interface the pin used to swap the component manager in
tests.

## The metrics forwarder

`MetricsForwarder` copies the process's own metrics onto NATS. At `Start` and then every `push_interval` it reads every
metric from the metrics registry it was given (`Dependencies.MetricsRegistry`) and publishes each sample as one JSON
object through the NATS client's JetStream publish. It is off unless the configuration enables it:
`ResolveServiceConfigs` does not add it. `RegisterAll` registers its constructor, `NewMetricsForwarderService`, under
`metrics-forwarder`, with no OpenAPI spec because it serves no route.

```json
{
  "services": {
    "metrics-forwarder": {
      "enabled": true,
      "config": {"push_interval": "5s", "include_go_metrics": false, "include_proc_metrics": false}
    }
  }
}
```

| Key | Meaning |
| --- | --- |
| `push_interval` | How often a round is published. Default `5s`; an empty value takes the default; it must be positive. |
| `include_go_metrics` | Also forward the `go_*` runtime families (goroutines, memory, garbage collection). Default `false`. |
| `include_proc_metrics` | Also forward the `process_*` families (CPU, open files, memory). Default `false`. |

It needs `Dependencies.NATSClient` and `Dependencies.MetricsRegistry`; `Logger` is optional. Construction refuses, as
invalid data naming the input: an unknown key or a key in another letter case, nil dependencies, a nil NATS client, a
nil metrics registry, and a `push_interval` that does not parse or is not positive. Configuration is read once; to
change a value, restart.

Each sample goes to the subject `metrics.<component>.<name>`, where `<component>` is the sample's `component` label
(`system` when it has none) and `<name>` is the metric's name. The payload is one JSON object with six fields:
`timestamp` (Unix milliseconds; the samples of one round share it), `name`, `component`, `type` (`counter`, `gauge`,
`histogram`, `summary` or `untyped`), `value` and `labels` (an object, `{}` when the sample has none). `value` is the
counter, gauge or untyped value; for a histogram or summary it is the number of observations, not a distribution. A
JetStream stream must capture `metrics.>` for the samples to be kept. The forwarder creates none: `config` already
declares the framework's `METRICS` stream over `metrics.>` (`config/streams.go`), and the composition provisions it
with `config.StreamsManager.EnsureStreams`, or points at an existing stream that captures `metrics.>`. The
SemStreams pin's metrics carried the prefix `semstreams_`; these are named `semengine_*`, and nothing renames the old
series, so a reader still on the old names finds none.

A sample that cannot be published is dropped and counted; the loop goes on and the next round publishes the series
again. A sample that does not encode (a NaN or infinite value) or does not publish increments
`semengine_errors_total{service="metrics-forwarder",type="metrics_publish"}`. A gather that fails increments
`type="metrics_gather"` and skips the whole round. Each is also logged at Debug. A label value is spliced into the
subject unchecked, and a publish that fails for that reason is counted like any other. The forwarder also records its
state on `semengine_service_status{service="metrics-forwarder"}` (0 stopped, 2 running), as the other services do.

`Start(ctx)` refuses a nil or ended `ctx`, a second `Start` and a `Start` after any `Stop`
(`errs.ErrAlreadyStarted`); a refused `Start` holds nothing. It does no I/O: it starts one background loop and
returns, and a NATS client that is not connected makes each publish fail and be counted. `ctx` bounds `Start` only;
ending it afterwards stops nothing (at the pin it marked the service stopped while the loop went on publishing).
`Stop(ctx)` is the only stop and behaves as the
heartbeat's does.

### Not ported

The `MetricsGatherer` interface, `MetricsForwarderConfig.Validate` and the test-only constructors.

## The log forwarder

`LogForwarder` forwards nothing yet. Its name comes from SemStreams, where a log handler that boot installed
(`pkg/logging`) sent log records to NATS and this service only checked the handler's settings. This module has no log
handler that publishes logs to NATS, and `pkg/logging` and the boot wiring that would install one are not ported. What
the service does today is check its settings, run the base service's lifecycle, and write one record saying so. It is
off unless the configuration enables it: `ResolveServiceConfigs` does not add it. `RegisterAll` registers its
constructor, `NewLogForwarderService`, under `log-forwarder`, with no OpenAPI spec because it serves no route.

```json
{
  "services": {
    "log-forwarder": {
      "enabled": true,
      "config": {"min_level": "INFO", "exclude_sources": ["a.b"]}
    }
  }
}
```

| Key | Meaning |
| --- | --- |
| `min_level` | The lowest level a forwarded record would need: `DEBUG`, `INFO`, `WARN` or `ERROR`, in any letter case. Default `INFO`; an empty value takes the default. |
| `exclude_sources` | Source prefixes, in dotted notation, whose records would not be forwarded: `a.b` would also exclude `a.b.c` but not `a`. Each entry is trimmed, empty entries are dropped and duplicates are removed. Nothing is added to the list. |

Both values are checked and reported, not applied: nothing reads them to filter a record. A composition root that
wants logs on NATS installs its own `slog.Handler` that publishes them (semteams does so in its own code;
`cmd/semteams/logging.go:71-93` at the time the design was written).

Construction needs `Dependencies` and nothing else: no NATS client, no I/O. `Logger` and `MetricsRegistry` are
optional. It refuses, as invalid data naming the input: nil dependencies, a document that does not parse or holds more
than one JSON value, an unknown key or a key in another letter case (`Min_Level` is refused, `min_level` is
written), and a `min_level` other than the four names above. An empty, whitespace-only or `null` configuration takes
the defaults. Configuration is read once; to change a value, restart.

At `Start` the service writes one record at Warn, through the injected logger (`slog.Default()` when none), with the
message `log-forwarder forwards nothing: no log handler in this module publishes logs to NATS` and the attributes
`min_level` (the resolved level's name) and `exclude_sources` (the resolved list, `[]` when empty). The logger carries
`service=log-forwarder`, so the record has that attribute once. It writes no other record while it runs, and none when
`Start` is refused. The service also records its state on `semengine_service_status{service="log-forwarder"}` (0
stopped, 2 running), as the other services do.

`Start(ctx)` refuses a nil or ended `ctx`, a second `Start` and a `Start` after any `Stop` (`errs.ErrAlreadyStarted`);
a refused `Start` holds nothing. It starts no goroutine of its own; the one the base service runs is its health check.
`ctx` bounds `Start` only; ending it afterwards stops nothing. `Stop(ctx)` is the base service's, and the only stop: it
refuses a nil `ctx`, returns an ended `ctx`'s error and stops nothing, and otherwise returns nil once the base service
has stopped; a `Stop` after one that returned nil returns nil.

### Not ported

`NewLogForwarder`, `LogForwarderConfig.Validate`, `LogForwarder.Config` and the `Stop` override. The SemStreams
service also always excluded the source `flow-service.websocket`; this one adds nothing to the configured list.
`service/milestone_service.go`, the one file in SemStreams' `service` package that reached an agentic package, is not
ported either; a contract test (`TestServiceReachesNoAgenticPackage`) fails if `service` comes to depend on one.

## The metrics service

`Metrics` serves the Prometheus endpoint. Its config keys are `port` (default `9090`) and `path` (default
`/metrics`). Started by itself, it binds the port between `Start` and `Stop`. Composed under a `Manager`, the
`Manager` claims the server before any service starts, serves it until `StopAll`, and `Metrics` reports healthy while
the server runs. A `Metrics` runs once: a second `Start`, and a `Start` after any `Stop`, are refused with
`errs.ErrAlreadyStarted`, and the context given to `Start` bounds `Start` only. `RegisterAll` registers `metrics`,
`component-manager`, `message-logger`, `storage-observability`, `heartbeat`, `metrics-forwarder` and `log-forwarder`.

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

These are read by the service manager.

- `ResolveServiceConfigs(configs)` turns the `services` section of a configuration into the map a service manager
  starts from. Each entry's `config` is rewritten in one canonical JSON form, and `component-manager`,
  `service-manager` and `metrics` are added, enabled, when the configuration leaves them out (an explicit
  `"enabled": false` stays). It never reads inside a service's `config`; the service that decodes it does.
- `PendingServiceChange`, `CompositionSealedError`, `DuplicateServiceError` and `MandatoryServiceDisabledError` are what
  the service manager reports about that map.
- `SchemaFromType` and `TypeNameFromReflect` build the JSON schema of a Go type for an OpenAPI document.
