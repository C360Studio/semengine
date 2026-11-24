# Booting SemEngine from your own main

This page is for a developer writing the `main` of a program that runs SemEngine: the code that loads a configuration,
connects to NATS and starts the services and components. SemEngine gives you the parts. Your `main` calls them in
order, and some mistakes in that order produce no error. This page lists the order, what each step reads, and what
happens when you skip it.

The order below is the one the smoke test runs (`internal/smoke/stack_integration_test.go`, the function that builds
the stack, lines 242-320). That test is the tested reference; if this page and the test disagree, the test is right.

Terms used here:

- **The file configuration** is the `*config.Config` you load from a file. It is what you wrote.
- **The effective configuration** is `mgr.GetConfig().Get()` after `config.Manager.Start`. It is what the deployment
  runs: the file's content, or the stored content when the stored copy wins. Start never changes the file
  configuration you passed in.
- **The configuration bucket** is the NATS key-value bucket that holds the configuration between boots.
- **A stem** is the `platform.id` you declare. At the first boot `Start` adds a random suffix to it (`dep` becomes
  `dep-800dff`); the result is the platform ID the deployment uses from then on.

## The steps

| Step | Call | Reads which configuration |
| --- | --- | --- |
| 1 | Load the file; optionally `cfg.Validate()` | the file, just loaded |
| 2 | `natsclient.NewClient`, `Connect`, `WaitForConnection` | the file's NATS URLs |
| 3 | `config.NewConfigManager`, then `Start` | the file, compared with the bucket |
| 4 | `effective := mgr.GetConfig().Get()` | the effective configuration, from here on |
| 5 | `config.NewStreamsManager(client, logger).EnsureStreams(ctx, effective)` | effective |
| 6 | Register components, vocabulary and payload types | none |
| 7 | `service.RegisterAll(services)` | none |
| 8 | `manager.ConfigureFromServices(effective.Services, deps)` | effective, with `deps.Platform` from it |
| 9 | Optional and conditional calls | none |
| 10 | `manager.StartAll(ctx)` | none |
| 11 | Teardown: `StopAll`, then the configuration manager's `Stop`, then the client's `Close` | none |

Use the effective configuration for every step from 4 on. The file configuration is still the stem and the file's
services after `Start`, so a step that reads it runs on something the deployment did not adopt.

### 1. Load the file

`config.NewLoader().LoadFromBytes` (or a file loader) returns the file configuration. `cfg.Validate()` is optional:
`Start` validates as well, so calling it only moves the error earlier and makes it clearer. If you skip it, a bad
file fails at step 3 instead.

Credentials and TLS are not file keys. A file that sets `nats.username`, `password`, `token`, `tls`, `max_reconnects`
or `reconnect_wait` is refused at load, naming the key (`config/config.go:707-713`). You build those into the NATS
client in step 2 with `natsclient` options.

### 2. Connect, and wait for the connection

Create the client, call `Connect`, then `WaitForConnection`. `Connect` returns before the connection is up;
`WaitForConnection` polls until the client reports it is connected or its context ends
(`natsclient/client.go:379-396`). Every later step uses the connection. Both reference programs wait; I did not run a
boot that skips the wait, so this page does not say what that looks like.

The client's defaults are to reconnect forever, two seconds apart (`natsclient/client.go:311-312`). After a broker
restart, watches can take a while to resume. How long is up to nats.go and is not promised, so this page states no
time. A program that would rather exit than wait out a long outage sets `natsclient.WithConnectionLossTimeout` and a
callback (`natsclient/options.go:50-62`); the callback only signals, and your `main` decides what to do.

### 3. Start the configuration manager

`config.NewConfigManager(cfg, client, logger)` then `Start(ctx)`. `Start` opens the configuration bucket, which is
named from the declared `platform.org` and stem, and decides which copy wins:

- First boot: the file is written to the bucket, and the stem gets its suffix.
- Later boots: a file whose `version` is newer than the bucket's is written. A file with an equal or older version
  loses, and the bucket's content is used; this is logged only at debug level (`config/manager.go:492-496`). If you
  edit the file and never raise `version`, the edits are ignored without a warning. The full table is in
  [config/README.md](../config/README.md#the-configuration-manager).

Start comes before step 4 because the effective platform ID exists only after it. Install the `Stop` call before you
call `Start`, so a failed start can still clean up.

`Start` runs once. A `config.Manager` and a `service.Manager` each start once per process; to start again, build a new
one, or restart the process.

Component writes (`PutComponentToKV`, `DeleteComponentFromKV`) made after this step and before step 8 land in this
boot. The component manager reads one snapshot of the effective configuration when step 8 builds it
(`service/component_manager.go:186-189`), so a write made after step 8 is stored for the next boot only.

### 4. Read the effective configuration

`effective := mgr.GetConfig().Get()`. This is the copy to pass on. Step 3 replaced the platform ID in a copy inside the
manager, so your `cfg.Platform.ID` still holds the stem while `effective.Platform.ID` holds the minted ID
(`config/config.go:123-133`).

A program that does not run `config.Manager`, such as a separate producer of entity IDs, reads the minted ID from the
`platform_identity` key of the configuration bucket: a JSON object with exactly the fields `org`, `stem` and `id`. See
[Platform identity](../config/README.md#platform-identity).

### 5. Create the streams

`EnsureStreams(ctx, effective)` creates the JetStream streams the configuration declares. If you skip it, a component
that consumes a stream waits for the stream and then fails its start: graph-ingest polls 30 times, with growing
pauses, and returns `stream ... not available after N retries` (`processor/graph-ingest/consume.go:218`). By the
constants that is about 49 seconds; I computed that and did not time it. A composition with no consuming component
boots, and its health publishes to the missing stream fail quietly (a debug log line and a counter).

### 6. Register components, vocabulary and payload types

Three registrations, all in your `main` and all before step 8:

- Components: build `component.NewRegistry()` and call each component package's `Register`. An enabled component whose
  factory you did not register makes the boot fail, and the error names every such component
  (`service/component_manager.go:261-262`).
- Vocabulary: call `vocabulary.Register` for your own predicates (names and label predicates) before the components
  that read them are built. The text and name indexes read the label predicates once, when each component is
  constructed or started (`graph/entitytext/text.go:38-41`). A predicate registered later is not seen: name lookup and
  text search miss your names, with no error and no log line. If your vocabulary registers from an `init` function in a
  package you import, the import is enough.
- Payload types: build `payloadregistry.New()`, then call `message.RegisterPayloads` and your own packages'
  `RegisterPayloads`. If you pass no registry, graph-ingest refuses to construct
  (`processor/graph-ingest/component.go:272`), and so does the object store when it has inputs. If you set
  `enable_hierarchy`, also register `inference.RegisterPayloads`; its absence is refused at construction, naming the
  fix. A message whose type you did not register is not a boot error: it is counted and logged at run time, once per
  message.

### 7. Register the services

`service.NewServiceRegistry()` and `service.RegisterAll(services)` offer every built-in service. Then
`service.NewServiceManager(services)`. If you skip registration, the configured services have no constructor, and a
composition whose component-manager has none is refused.

### 8. Compose, with the effective copies

```go
err := manager.ConfigureFromServices(effective.Services, &service.Dependencies{
    NATSClient:        client,
    MetricsRegistry:   metric.NewMetricsRegistry(),
    Logger:            logger,
    Platform:          types.PlatformMeta{Org: effective.Platform.Org, Platform: effective.Platform.ID},
    Manager:           mgr,
    ComponentRegistry: components,
    PayloadRegistry:   payloads,
})
```

Two values here are easy to take from the wrong copy:

- `deps.Platform` must be built from the effective configuration. If it holds a pair other than the one the
  configuration manager holds (for example the stem instead of the minted ID, or another org), `ConfigureFromServices`
  fails with an invalid error that names both pairs, and no component is built. A `deps.Platform` with both fields
  empty is not refused here; a component that needs one, such as graph-ingest, refuses it when it is built
  (`processor/graph-ingest/component.go:280`). The refusal is in `service/component_manager.go`, in
  `checkPlatform`.
- `effective.Services`, not `cfg.Services`. The component manager reads the effective components, so passing the
  file's services runs the file's services next to the effective components. Nothing checks this for you.

Nothing is composed unless every constructor succeeds.

The default ports are fixed, and they bind every network interface: the main listener is `http_port`, default 8080
(`service/service_manager.go:165-166`), and metrics is on at 9090 unless the `metrics` service config sets
`"enabled": false` (`service/service_configs.go:90`). A second process on one host fails to bind. The service
manager has no way to ask for a free port, so set the ports in the configuration.

### 9. Optional and conditional calls

Between step 8 and step 10:

- `manager.StartHealthListener(ctx, port)`: probes answer while a slow start runs, such as step 5's wait. A second call
  is refused (`service/service_manager_http.go:127`).
- `manager.UseHTTPMiddleware(...)`: before `StartAll` (`service/service_manager_http.go:44`).
- `service.ConfigureRulePackMutations(manager)`: after `ConfigureFromServices` and before `StartAll`; without it, no
  rule-pack component receives its mutation client (`service/rule_pack_bind.go:76-79`).

### 10. Start

`manager.StartAll(ctx)` starts the services in registration order, and the component manager starts the components.
A failed `StartAll` rolls back what it started. Call `StopAll` after a failure anyway: if the rollback itself
failed, `StopAll` runs it again (`service/service_manager.go:330-338`). A second `StartAll` on the same manager returns
`errs.ErrAlreadyStarted`.

### 11. Tear down in reverse

Stop the services (`manager.StopAll`), then the configuration manager (`mgr.Stop`), then close the client
(`client.Close`). Deferring each call right after it succeeds gives this order. Give each a context that has not
ended; `Stop` needs one even when the run context is done.

## A short sketch, in the same order

```go
cfg, err := config.NewLoader().LoadFromBytes(raw) // 1
if err != nil {
    return err
}
if err := cfg.Validate(); err != nil { // 1, optional
    return err
}

client, err := natsclient.NewClient(strings.Join(cfg.NATS.URLs, ","), natsclient.WithLogger(logger)) // 2
if err != nil {
    return err
}
defer closeClient(client) // 11, last
if err := client.Connect(ctx); err != nil {
    return err
}
if err := client.WaitForConnection(ctx); err != nil {
    return err
}

mgr, err := config.NewConfigManager(cfg, client, logger) // 3
if err != nil {
    return err
}
if err := mgr.Start(ctx); err != nil {
    return err
}
defer stopManager(mgr) // 11, second to last
effective := mgr.GetConfig().Get() // 4

if err := config.NewStreamsManager(client, logger).EnsureStreams(ctx, effective); err != nil { // 5
    return err
}

components := component.NewRegistry() // 6: components, vocabulary, payloads
// ... register each component package, vocabulary.Register(...), payload registrations ...

services := service.NewServiceRegistry() // 7
if err := service.RegisterAll(services); err != nil {
    return err
}
manager := service.NewServiceManager(services)
// 8: ConfigureFromServices(effective.Services, deps) as shown above
// 9: optional calls
if err := manager.StartAll(ctx); err != nil { // 10
    stopServices(manager)
    return err
}
defer stopServices(manager) // 11, first
```

`closeClient`, `stopManager` and `stopServices` stand for a call with a context that has not ended; the smoke test
calls them `stop`.

## What each skipped step looks like

| Skipped or wrong | What you see | Found out |
| --- | --- | --- |
| Step 5, streams | graph-ingest's start fails after about 49 s | boot error |
| Step 6, component not registered | the boot fails, naming every such component | boot error |
| Step 6, no payload registry | graph-ingest or the object store refuses to build | boot error |
| Step 6, product payload type missing | each message is counted and logged as refused | run time, per message |
| Step 6, vocabulary registered late | name lookup and text search miss your names | nowhere |
| Step 8, `deps.Platform` from the file | `ConfigureFromServices` fails, naming both pairs | boot error |
| Step 8, `cfg.Services` instead of effective | the file's services run | nowhere |
| Step 3, file edited, `version` not raised | the stored configuration keeps winning | debug log |
| Step 8, default ports in use twice | the second process fails to bind | boot error |
