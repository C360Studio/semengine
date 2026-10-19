# health-status Specification

## Purpose

`health` holds the health report a service gives (`health.Status`) and the two functions the service manager builds
its operator answers from: `Aggregate`, which rolls several reports into one, and `FromComponentHealth`, which turns
a component's own report into a `health.Status`. It does not decide readiness; `/readyz` is the one readiness
endpoint (ADR-109).

## Requirements

### Requirement: Health reports roll up worst first, one level deep

`health.Aggregate(name, statuses)` SHALL return a `Status` named `name` whose `Status` word is `unhealthy` when any
input's word is `unhealthy`, otherwise `degraded` when any input's word is `degraded`, otherwise `healthy`. It SHALL
read only each input's `Status` word and SHALL NOT look into an input's `SubStatuses`. An input whose word is
anything else, the empty word included, SHALL count as healthy. With no inputs it SHALL return `healthy`. The result's
`Healthy` SHALL be true only when its word is `healthy`, and its `SubStatuses` SHALL be a copy of the inputs.

#### Scenario: One unhealthy input

- **WHEN** `Aggregate` reads one `healthy`, one `degraded` and one `unhealthy` status
- **THEN** it returns `unhealthy` with `Healthy` false and three sub-statuses

#### Scenario: A degraded status nested one level down

- **WHEN** `Aggregate` reads one `healthy` status whose own `SubStatuses` holds a `degraded` status
- **THEN** it returns `healthy`

#### Scenario: An unknown status word

- **WHEN** `Aggregate` reads one status whose word is `stopped` and whose `Healthy` is false
- **THEN** it returns `healthy`

### Requirement: A component's report becomes unhealthy whenever it is not healthy

`health.FromComponentHealth(name, ch)` SHALL return a `Status` named `name` whose word is `healthy` when
`ch.Healthy` is true and `unhealthy` when it is false, whatever `ch.Status` says. `Healthy` SHALL equal
`ch.Healthy`. `Metrics` SHALL carry `ch.Uptime`, `ch.ErrorCount` and `ch.LastCheck`. When `ch.LastError` is not
empty, `Message` SHALL be that text after the package's sanitizer has replaced URLs with `[URL]`, Unix and Windows
paths with `[PATH]`, IPv4 addresses with `[IP]`, `:` followed by two to five digits with `[PORT]`, and credential
assignments with `[REDACTED]`, in that order; it can over-redact. When `ch.LastError` is empty, `Message` SHALL be
"Component healthy" when `ch.Healthy` is true and "Component unhealthy" when it is false.

#### Scenario: A component that reports itself degraded

- **WHEN** `FromComponentHealth` converts a report with `Healthy` false and `Status` `degraded`
- **THEN** the result's word is `unhealthy`

#### Scenario: An unhealthy component with no error text

- **WHEN** `FromComponentHealth` converts a report with `Healthy` false and an empty `LastError`
- **THEN** the result's word is `unhealthy` and its `Message` is "Component unhealthy"

#### Scenario: Secrets in the error text

- **WHEN** `FromComponentHealth` converts a report whose `LastError` is
  `failed to connect to https://api.example.com/v1 with password=secret123`
- **THEN** the result's `Message` contains neither `https://api.example.com/v1` nor `secret123`

#### Scenario: A clock time in the error text

- **WHEN** `FromComponentHealth` converts a report whose `LastError` is `at 12:30:45`
- **THEN** the result's `Message` is `at 12[PORT][PORT]`
