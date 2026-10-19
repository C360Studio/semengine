# health

The health report a service gives (`health.Status`), and two functions that build one report from others:
`Aggregate` rolls several reports into one, and `FromComponentHealth` turns a component's own report
(`component.HealthStatus`) into a `health.Status`.

The package is a port of SemStreams' `health` at the pin `8b99efe9`; `docs/admission-ledger.yaml` (row `health`)
records what was kept, what was dropped and what changed. It holds no state between calls, runs no goroutine and
does no I/O.

```go
import "github.com/c360studio/semengine/health"
```

## The report

| Field | JSON | Meaning |
| --- | --- | --- |
| `Component` | `component` | What the report describes |
| `Healthy` | `healthy` | True only when `Status` is `healthy` |
| `Status` | `status` | `healthy`, `degraded` or `unhealthy` |
| `Message` | `message` | Text for a person to read |
| `Timestamp` | `timestamp` | When the report was made |
| `SubStatuses` | `sub_statuses` | The reports this one was built from, if any |
| `Metrics` | `metrics` | Uptime, error count and last activity, if known |

`Healthy` and `Status` state one fact twice. `NewHealthy`, `NewDegraded` and `NewUnhealthy` set both, with the
current time, so build reports with them. `IsDegraded`, `IsUnhealthy` and `Aggregate` read the `Status` word only.

The JSON field names are a wire format that operators read; keep them stable.

A `Status` is a plain value, but `Metrics` is a pointer, so a copy of a `Status` shares its `Metrics`.

## Rolling reports up

```go
overall := health.Aggregate("platform", []health.Status{
    health.NewHealthy("nats", "connected"),
    health.NewDegraded("cache", "running without a warm index"),
})
// overall.Status == "degraded", overall.Healthy == false, len(overall.SubStatuses) == 2
```

`Aggregate` takes the worst word among its inputs:

- `unhealthy` if any input is `unhealthy`;
- otherwise `degraded` if any input is `degraded`;
- otherwise `healthy`.

It reads only each input's own word, one level deep: a `degraded` report nested inside an input's `SubStatuses`
does not change the result. An input with any other word, the empty word included, counts as healthy, and an empty
list gives `healthy`. The result's `SubStatuses` is a copy of the input list.

## Converting a component's report

`FromComponentHealth(name, ch)`:

- The word comes from `ch.Healthy` alone: `true` gives `healthy`, `false` gives `unhealthy`. The component's own
  `ch.Status` is not read, so a component that reports itself `degraded` with `Healthy` false becomes `unhealthy`
  (owner ruling, PR #318 comment 6271450376).
- `Healthy` is `ch.Healthy`.
- `Metrics` carries `ch.Uptime`, `ch.ErrorCount` and `ch.LastCheck` (as `LastActivity`).
- `Message` is "Component healthy" or "Component unhealthy", following `ch.Healthy`, when `ch.LastError` is empty.
  SemStreams wrote "Component healthy" in both cases; the owner ruled the fix (PR #318 comment 6271710866).
- Otherwise `Message` is `ch.LastError` with likely secrets replaced.

### What the error text loses

The replacements run in this order, on text:

| Matches | Becomes |
| --- | --- |
| `http://`, `https://`, `nats://`, `ws://`, `wss://` URLs | `[URL]` |
| Unix paths (any run starting with `/`) and Windows paths (`C:\...`) | `[PATH]` |
| IPv4 addresses | `[IP]` |
| `:` followed by two to five digits | `[PORT]` |
| `password`, `token`, `key`, `secret` or `credential`, then `:` or `=` and a value | `[REDACTED]` |

Matching on text removes more than secrets:

| Input | Message |
| --- | --- |
| `failed to bind to :8080` | `failed to bind to [PORT]` |
| `at 12:30:45` | `at 12[PORT][PORT]` |
| `graph/ingest` | `graph[PATH]` |
| `invalid key=acme.prod.sensor.device.unit.one` | `invalid [REDACTED]` |

The messages given to `NewHealthy`, `NewDegraded` and `NewUnhealthy` are used as given.

## What this package does not do

It does not decide readiness; that is a separate question with its own endpoint (`docs/adr/109-ready-after-boot.md`).
It keeps no list of components and does not watch them: the caller builds reports when it is asked for one.
