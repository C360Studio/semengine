# Design: setup-04a-03-boot-health

This design depends on `INVENTORY PASS` for `inventory.md` (round 2). Its spec delta is
`specs/health-status/spec.md`. Pin paths are at `8b99efe9`, repository lines at `63328c4`.

**Decision records.** No pin ADR governs `health`. All 85 pin records were searched for `health/`, `health.Status`,
the helper names and `package health`. The three hits mention the word only: ADR-033 is about semspec's own
`pkg/health`, and ADR-058 and ADR-094 mention health in passing. The readiness records (066, 082-085, 088, 098) govern
readiness, not this package (inventory § 2). Records this design touches:

- **Pin ADR-079 Decision 3** (`079-per-entity-poison-response-authoritative-surface.md:38-42`, `:70`): operators
  alert on "degraded Health". **Not carried.** The owner ruled on 2026-10-19 (#318 comment 6271450376) to keep the
  pin's flattening of degraded into unhealthy. The degraded signal is recorded on #110 item 3 (comment 6271450747),
  where the component status words get settled. ADR-079 is not copied into `docs/adr`, because this port implements
  none of its decisions.
- **ADR-108 Decision 1** (`docs/adr/108-processor-shell.md:58-66`): services report `health.Status`, components
  report `component.HealthStatus`. Kept: both types stay.
- **ADR-109** (`docs/adr/109-ready-after-boot.md:35-37`): `/readyz` is the one readiness endpoint. At the pin it
  reads `service.IsHealthy()` and `component.HealthStatus.Healthy`, never `health.Status`
  (`service/service_manager.go:1841-1854`). This package is not on the readiness path.
- ADR-113 does not apply: `health` holds no context, starts no goroutine and owns no resource.

## D1. Port the live surface, drop the dead surface (S4, required)

- **Kept:**
  - `Status`, `Metrics`, `NewHealthy`, `NewDegraded`, `NewUnhealthy`, `Aggregate`, `FromComponentHealth`,
    `Status.IsDegraded`, `Status.IsUnhealthy`.
  - Each has a production reader at the pin in `service`, an admitted package not yet ported (inventory § 4). The
    JSON of `Status` and `Metrics` is the operator wire at the three endpoints and on `health.service.<name>`.
- **Dropped:**
  - `Monitor`, `NewMonitor` and the 11 `Monitor` methods.
  - `Status.WithMetrics`, `Status.WithSubStatus`, `Status.IsHealthy`.
  - `Metrics.MessagesProcessed`.
  - Basis: the surface audit (architect contract, Extraction slices; reviewer contract, Port-time review). None of
    these has a reader in this repository, in the pin outside `health`'s own tests, or in semsource, semconnect,
    semteams or semboids at symbol level. No consumer imports `health`.
- **Tests:**
  - Dropped: `monitor_test.go` (its `time.Sleep` at `:359` and `:373` is the required test-text fix);
    `TestStatus_IsHealthy`, `TestStatus_WithMetrics` and `TestStatus_WithSubStatus` in `status_test.go`;
    `TestWithSubStatus_SliceIsolation` in `security_test.go`.
  - Changed: `helpers_test.go:30` checks `Status == "healthy"` in place of `IsHealthy()`.
  - Kept: every other pin test, unchanged apart from the module path.
- **Options:**
  - (a) Chosen: port the live surface.
  - (b) Port the package whole: this carries 14 symbols nothing reads, plus a verify failure.
  - (c) Fold `health.Status` into `component.HealthStatus`: one health type, but this reverses ADR-108 D1, changes
    the wire, and belongs to the `service` design.
  - (d) Do nothing: `service` cannot build.

## D2. Behaviour as at the pin, as ruled (S2, ruled; S1, optional)

- `FromComponentHealth` sets `Status` from `Healthy` alone: `true` gives `healthy`, `false` gives `unhealthy`.
  `ch.Status` is ignored (owner ruling, Q1). So a component that reports itself `degraded` with `Healthy=false` is
  `unhealthy`, and the pin's `/components/health` answers 503.
- The ruling touches `/components/health` only (03B `design.md:725`). `/services/health` (`:723`) is built from
  `BaseService.Health()` (pin `service/base.go:202-227`), which already reports starting and stopping as degraded.
  That belongs to the `service` port and is not changed here.
- `Aggregate` is a worst-case roll-up: `unhealthy` if any input is unhealthy, else `degraded` if any is degraded,
  else `healthy`. It reads only each input's `Status` word, one level deep. Any other word, `""` included, counts as
  healthy, and no inputs gives healthy. The pin's `service/storage_observability.go:468-470` relies on the
  one-level rule, so the doc is fixed and the code is not (S5).
- S1 is optional and retained: `Healthy` and `Status` are two fields for one fact, and S1b, the fail-open on an
  unknown word, goes with it. Every pin producer sets both through the `New*` helpers, and no real-world trigger
  makes them disagree (inventory § 1, S1). The spec states the real rule so nobody reads more into it; it records
  current behaviour, not a promise to keep failing open, and a later fail-closed fix changes the spec with it.
- S3 is optional and retained: `FromComponentHealth` passes `LastError` through the pin's text sanitizer. The probe
  on the pin copy (inventory § 1, S3) shows it over-redacts: `at 12:30:45` becomes `at 12[PORT][PORT]`, and
  `key=acme.prod.sensor.device.unit.one` becomes `[REDACTED]`. Operators list poisoned entities through DebugStatus
  (pin ADR-079 `:70`), so nothing is lost that has no other home. The doc states what the sanitizer replaces and
  that it can over-redact.
- **Q3 (owner ruled 2026-10-19: fix; #318 comment 6271710866).** A component with `Healthy=false` and an empty
  `LastError` gets the message "Component healthy" (pin `status.go:123-126`, pinned by `status_test.go:208-218`).
  Trigger in the tree: graph-ingest's `Health()` collects stuck states only while running (`processor/graph-ingest/component.go:853-871`),
  so a graph-ingest that is not yet started, failed to start or is stopping reports `Healthy=false`, an empty
  `LastError` and `Status:"stopped"`, and `/components/health` shows `unhealthy` with "Component healthy". The status
  and HTTP code are right; only the text contradicts them. It is the owner's call because the 03B design keeps
  `/components/health` "served as at the pin" (`design.md:725`) and the pin's test pins the message. Recommended:
  "Component unhealthy" when `Healthy` is false. The status, the HTTP code and the JSON fields stay as at the pin. The
  owner accepted the recommendation.
- **Tests:** `TestFromComponentHealth` gains a row (`Healthy=false`, `Status:"degraded"` gives `unhealthy`), plus,
  under Q3's recommendation, a changed row at `:208-218`. `TestAggregate` gains rows for an unknown word and `""`
  (both give healthy). Sensitivity: `task mutate:check` detects one mutant per rule: a `FromComponentHealth` that
  maps a `degraded` `ch.Status` to `degraded`; an `Aggregate` that counts `""` as unhealthy; an `Aggregate` that
  looks into `SubStatuses`; and, under Q3's fix, the message reverted to "Component healthy".

## D3. True text (S5, required)

`doc.go` and `README.md` are rewritten to the kept surface (surface audit, item c). They must not say:

- aggregation recurses (`doc.go:80-81`, `:198`);
- `Metrics` is a map (`:87-101`);
- coverage is 100% (`:175`);
- a `semstreams/errors` package exists (`:169`);
- statuses are immutable while `Metrics` is a shared pointer (`:141`); the text says the pointer is shared;
- `Monitor` feeds `/health` (`:215`, `:220`);
- `:159` should read `[PORT]`, not `:[PORT]`.

They also drop the README's `health:` config block (`README.md:205-232`), which no pin code parses, and the examples
that start goroutines without joining them (`doc.go:248-264`). They state D2's rules, including the ruling. They say
the package does not decide readiness (ADR-109). Sweep shapes 2, 3 and 5 found nothing (inventory § 1). The pin's
agent contracts and skills carry no `health` guidance, and the package builds no `GenericJSON`.

## D4. Ledger row

```yaml
- source_path: health
  source_sha: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128
  consumer_purpose: >-
    the health report services give (health.Status) and the roll-up and component conversion the
    service manager builds /health, /services/health, /components/health and health.service.<name>
    from; imported by service (change 3, setup-04a-03-boot); 03B design D4 row "carry"
  destination: health
  contract: >-
    Status, Metrics, NewHealthy, NewDegraded, NewUnhealthy, Aggregate, FromComponentHealth,
    Status.IsDegraded and Status.IsUnhealthy as at the pin, JSON unchanged; the health-status spec
    (worst-case one-level roll-up; Healthy=false is unhealthy whatever the component's status word,
    owner ruling #318 comment 6271450376)
  dependencies_and_side_effects: component (in tree), regexp, strings, time; no goroutine, no I/O
  known_risks: >-
    dropped as unread (surface audit): Monitor, NewMonitor and its 11 methods, Status.WithMetrics,
    Status.WithSubStatus, Status.IsHealthy, Metrics.MessagesProcessed, with their tests; doc.go and
    README.md rewritten (S5). Retained and optional: Healthy beside Status (S1), Aggregate counts an
    unknown word as healthy (S1b), the text sanitizer over-redacts (S3). Pin ADR-079 D3's degraded
    signal is not carried (#110 item 3). One change from the pin (Q3, owner ruling #318 comment
    6271710866): an unhealthy component with no error text reads "Component unhealthy".
  proving_tests: health/helpers_test.go, health/status_test.go, health/security_test.go
  owner: "setup-04a-03-boot-health developer"
  disposition: adapt
```

**Merge order:** independent of #316, which shares no file or capability with this change (inventory § 5).
