# config

Package `config` reads a SemEngine process's configuration: one JSON document, laid over built-in defaults, with a
few environment variables on top. It also declares the JetStream streams that configuration needs and creates them on
a NATS server, and its `Manager` keeps the configuration in a NATS key-value bucket so it outlives the process.

What it does not do: it does not connect to NATS (the caller hands `StreamsManager` and `Manager` a connected client),
and it does not write configuration back to a file. A change the `Manager` sees in its bucket while it runs updates
what `GetConfig` and `WatchModelRegistry` report; the `Manager` itself never starts, stops or rewires a component.

## Loading a configuration

```go
cfg, err := config.NewLoader().LoadFile("config.json")
if err != nil {
    return err // the file could not be read, or the document was refused
}
if err := cfg.Validate(); err != nil {
    return err // the document loaded but breaks a rule, such as a missing platform.id
}
```

`LoadFromBytes(data)` does the same for a document already in memory, for example after the caller has expanded
`${VAR}` references in it. Both loaders run the same steps, in this order:

1. Read the document and refuse it if it breaks a size, depth or key rule (below).
2. Lay it over the defaults. A key the document sets replaces the default; a key it leaves out keeps the default.
   Objects merge key by key, so `{"platform":{"id":"p1"}}` keeps the default `platform.region`. A value written in
   the document, including `false`, `0` or an empty list, replaces the default.

Write every key in lowercase, exactly as shown in this README. A key in another letter case is not reliably applied:
where the defaults carry that key, it is silently ignored. `{"Components":{...}}` loads no components, `"URLs"` keeps
the default URL, and `"JetStream":{"enabled":false}` leaves JetStream enabled (#325). The refused keys listed below
are the exception: they are refused in any letter case.
3. Apply the environment overrides.
4. Check that `platform.org` and `platform.id` together leave room for the entity IDs this deployment will create
   (ADR-104).

Loading does not call `Validate`. The caller calls it, as in the example.

The defaults are: `platform.region` `gulf_mexico`, `nats.urls` `["nats://localhost:4222"]`, `nats.jetstream.enabled`
`true`, and two services, `discovery` (disabled) and `heartbeat` (enabled, every 30 seconds).

### An example document

This document loads and validates as written:

```json
{
  "version": "1.0.0",
  "platform": {
    "org": "c360",
    "id": "vessel-001",
    "type": "vessel",
    "region": "gulf_mexico",
    "capabilities": ["radar", "ctd"],
    "environment": "prod"
  },
  "nats": {
    "urls": ["nats://nats-1.example.internal:4222", "nats://nats-2.example.internal:4222"],
    "jetstream": {
      "enabled": true,
      "max_memory": 1073741824,
      "max_file_store": 10737418240
    }
  },
  "services": {
    "metrics": {
      "enabled": true,
      "config": {"port": 9090, "path": "/metrics"}
    }
  },
  "components": {
    "udp-mavlink": {
      "type": "input",
      "name": "udp",
      "enabled": true,
      "config": {"port": 14550, "subject": "input.udp.mavlink"}
    }
  },
  "streams": {
    "SENSOR": {
      "subjects": ["sensor.>"],
      "max_age": "7d",
      "max_bytes": 1073741824,
      "discard": "old"
    }
  }
}
```

The sections:

- `platform`: who this deployment is. `org` and `id` are required by `Validate`; they are the first two parts of
  every entity ID the deployment creates (ADR-102). `environment` is a label for logs only and does not separate two
  deployments: give each deployment its own `id`. `platform.instance_id` was removed and is refused at load.
- `nats`: the server URLs, and whether JetStream is used. `max_memory` and `max_file_store` are what the operator
  expects the server's account limits to be; `StreamsManager.VerifyJetStreamLimits` logs a warning when the server's
  real limit is lower. They do not change the server's limits, which are set in the server's own configuration.
- `services` and `components`: one entry per service or component instance, keyed by its name. Each `config` object
  is passed to that service or component unread. A component entry needs `type` (one of `input`, `processor`,
  `output`, `storage`, `gateway`) and `name` (the factory that builds it).
- `streams`: JetStream streams to create, keyed by stream name (see "Streams" below).

### What loading refuses

Loading fails, and returns no `Config`, when:

- The document is not exactly one JSON object (a document that is `null` is refused too), is larger than 10 MB, or
  nests deeper than 100 levels.
- A value does not fit its field: for example `"urls": "nats://a:4222"` (a string where a list is expected), a
  fraction in `max_memory`, or an unknown key inside a `services` entry. The error carries the decoder's message.
- The document sets a NATS setting that nothing in SemEngine applies yet. These keys are refused when present,
  whatever their value and in any letter case: `nats.username`, `nats.password`, `nats.token`, `nats.tls`,
  `nats.max_reconnects`, `nats.reconnect_wait`, `nats.jetstream.domain`, `nats.jetstream.retention_policy` and
  `nats.jetstream.replication_factor`. Each comes back when the code that connects to NATS uses it (#74).
- `SEMENGINE_NATS_USERNAME`, `SEMENGINE_NATS_PASSWORD` or `SEMENGINE_NATS_TOKEN` is set to a value that is not empty,
  for the same reason.
- The document sets `platform.instance_id`, in any letter case.

Other unknown keys at the top level, or inside `platform` or `nats`, are accepted and ignored, and so is a key in
another letter case where the defaults carry that key (above).

### Which files LoadFile reads

The path must end in `.json` or `.json5`; either way the content is read as plain JSON. A relative path must stay
inside the current working directory once `.` and `..` are resolved, so `../config.json` is refused. An absolute path
may point anywhere, but is refused if a file or directory name in it contains `..`. Symbolic links are followed: a
link to a regular file is read, wherever the file is. A directory or a device is refused.

### Environment overrides

These variables, when set and not empty, replace the document's value:

| Variable | Replaces |
| --- | --- |
| `SEMENGINE_PLATFORM_ID` | `platform.id` |
| `SEMENGINE_PLATFORM_TYPE` | `platform.type` |
| `SEMENGINE_PLATFORM_REGION` | `platform.region` |
| `SEMENGINE_NATS_URLS` | `nats.urls`, split at commas; spaces are not trimmed, so write `a,b`, not `a, b` |

An empty variable is the same as an unset one, so an override cannot set a value to empty.

## Sharing a configuration between goroutines

`NewSafeConfig(cfg)` wraps a `Config` for concurrent use. `Get` returns a deep copy, so a caller can change it without
affecting anyone else. `Update` validates a new `Config` and swaps it in. To change part of the configuration, use
`Mutate`, which holds the lock from the read to the swap:

```go
err := safe.Mutate(func(cfg *config.Config) error {
    c := cfg.Components["udp-mavlink"]
    c.Enabled = false
    cfg.Components["udp-mavlink"] = c
    return nil
})
```

`Get`, then a change, then `Update` can lose a change another goroutine made between the two calls. The function
passed to `Mutate` must not call `Get`, `Update` or `Mutate` itself: it would wait forever on the lock it is running
under.

## The configuration manager

`Manager` stores a deployment's configuration in a NATS key-value bucket, the configuration bucket, so that it survives
a restart and another process (an operator's tool, a UI) can change what the next boot runs. The bucket is named
`semengine_config_<org>_<stem>` from the document's `platform.org` and the `platform.id` it declares:
`config.BucketName("acme", "dep")` returns `semengine_config_acme_dep`.

```go
func run(ctx context.Context, cfg *config.Config, client *natsclient.Client, logger *slog.Logger) (err error) {
    mgr, err := config.NewConfigManager(cfg, client, logger)
    if err != nil {
        return err
    }
    if err := mgr.Start(ctx); err != nil {
        return err // nothing is held: there is nothing to stop
    }
    defer func() {
        // ctx may have ended by now; Stop needs a context that has not.
        stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
        defer cancel()
        err = errors.Join(err, mgr.Stop(stopCtx))
    }()

    platformID := mgr.GetConfig().Get().Platform.ID // the effective identifier, such as "dep-800dff"
    logger.Info("configuration ready", "platform_id", platformID)
    <-ctx.Done()
    return nil
}
```

### Start and Stop

`Start(ctx)` does all of its work under `ctx`: it acquires the bucket, establishes the platform identity (below),
decides whether the file or the bucket wins, and opens the watches. It refuses a `nil` or ended `ctx`, and refuses
every call after the first with `errs.ErrAlreadyStarted`, including a call after `Stop`: build a new `Manager` to start
again. Once `Start` has returned, ending its `ctx` stops nothing; the watches run until `Stop` (ADR-113).

`Stop(ctx)` closes every channel the `Manager` handed out, stops every watch and waits for the `Manager`'s goroutines.
It returns `nil` only when all of that is done. If `ctx` ends first it returns `ctx`'s error, and a later `Stop` tries
again. A `ctx` that has already ended stops nothing, which is why the example builds a fresh bounded context instead
of passing the signal context that is ending the program.

### Platform identity

The first `Start` on an empty bucket mints the deployment's effective `platform.id`, the declared value plus a
six-character hexadecimal suffix such as `dep-800dff`, and records it in the bucket's `platform_identity` key. Every
later `Start`, in this process or another with the same document, adopts the recorded value. `Start` refuses a bucket
whose record names another `platform.org` or another declared `platform.id`, and a bucket that holds configuration but
no record (ADR-104).

### Which copy wins

| At `Start` | Result |
| --- | --- |
| The bucket holds no configuration (first boot) | The file is written to the bucket |
| The file's `version` is newer than the bucket's | The file is written to the bucket |
| The versions are equal, the file's is older, or the bucket's does not parse | The bucket wins |

Versions are compared with `CompareVersions`. When the bucket wins, its `services` and `components` entries replace
the file's, so a component deleted at run time stays deleted at the next boot; to bring the file's set back, raise the
file's `version`. A stored `model_registry` replaces the file's; a stored `nats` is decoded over the file's `nats`
section, so a key it leaves out keeps the file's value. The stored `platform` key is a copy for readers and is never
applied. When the file is written, `version` is written last, so a write cut short leaves the bucket's old `version`
(or none) and the next boot writes the file again.

### Names

Each service and component name is stored as the key `services.<name>` or `components.<name>`, with every space
written as `_`. The name must then be one key token: no `.`, `*` or `>`. `Start` refuses such a name, and two names
that map to the same key (`a b` and `a_b`), before it touches the bucket. A name with a space appears under its key's
name from `Start` on: a file component `a b` is `a_b` in `GetConfig`.

### When Start fails

`Start` returns an error naming the key, holds nothing, and leaves every writer refusing, when:

- a name or a value cannot be written (found before the bucket is touched, so nothing is written, not even the
  identity record);
- the bucket cannot be listed or read;
- a stored `services`, `components`, `nats` or `model_registry` entry does not decode;
- a stored `nats` entry sets a key loading refuses (see "What loading refuses");
- writing the file to the bucket fails;
- any watch does not open.

A change written to the bucket while the `Manager` runs that does not decode is logged and skipped.

### Next-boot writes

`PutComponentToKV(ctx, name, cfg)` and `DeleteComponentFromKV(ctx, name)` write or delete `components.<name>` and
update `GetConfig` before they return; a put whose key the watcher has already applied at a newer revision keeps that
newer value. They record what the next boot runs: they never start, stop or restart a component in the running
process. `WatchModelRegistry()` returns a channel that receives the model registry each time its key changes, and
closes when `Stop` runs.

### Key families

A key family gives another owner its own keys, `<prefix>.<name>`, in the same bucket:

```go
rules, err := config.NewKeyFamily("rules", func(e config.KeyFamilyEntry) {
    logger.Info("rule changed", "name", e.Name, "deleted", e.Operation == config.KeyFamilyDelete, "initial", e.Initial)
})
if err != nil {
    return err
}
mgr, err := config.NewConfigManager(cfg, client, logger, config.WithKeyFamily(rules))
```

After `Start`, the handler receives every entry present when the watch opened (`Initial` is true), then each put and
delete. It runs on the `Manager`'s goroutine and must not block: `Stop` waits for it. The family's `Get`, `Put`,
`Create`, `Delete` and `Names` reach only `rules.*` keys, and return an error until `Start` has succeeded. The
`Manager` never decodes a family's values. A prefix the `Manager` uses itself (`services`, `components`, `platform`,
`nats`, `model_registry`, `platform_identity`) is refused by `NewConfigManager`.

## Streams

`NewStreamsManager(client, logger).EnsureStreams(ctx, cfg)` creates the JetStream streams the configuration needs. For
a stream that already exists, it compares the stream's settings with the declaration and updates the ones that
differ. The streams come from three places: the framework's own
streams, the `streams` section, and the JetStream output ports of the configured components. A stream named in
`streams` wins over one derived from a port.

Every stream must be bounded. Each needs a finite `max_age` (a Go duration such as `168h`, or a number of days such as
`7d`), a `max_bytes` above zero, and a `discard` policy: `old` drops the oldest messages when the stream is full, and
`new` refuses new ones. No default fills these in. A stream that must keep everything is declared under
`archival_streams` with an `owner` and a `reason`. An existing stream that cannot take bounds yet is declared under
`stream_migration_overrides` with an `owner` and an `expires` date. `Validate` checks all of this without a server
(`ValidateStreamDeclarations`), and `EnsureStreams` checks it again before it creates anything, so a configuration
with a missing bound creates no streams at all.

`EnsureStreams` also refuses to create a stream whose name belongs to a NATS key-value bucket or object store
(`ErrBackingStreamNotProvisionable`). It fails, changing nothing on that stream, when an existing stream's storage
type or retention policy differs from the declaration (`ErrStreamNonEditableDrift`): those two are not updated in
place.

## Comparing versions

`CompareVersions("1.2.0", "1.10.0")` compares two semantic version strings, such as `Config.Version`, and returns -1,
0 or 1.
