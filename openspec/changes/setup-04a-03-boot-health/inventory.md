# Inventory: setup-04a-03-boot-health

base: `63328c4` (origin/main; the claim commit `c942aac` is empty; draft PR #318)

Question: what porting `health` from the SemStreams pin `8b99efe9` carries, what it drops, and what the owner must
rule on before the design. Owner intent: #9 comment 6270875286 ("Change 3 (`setup-04a-03-boot`) starts now,
beside #316, with `health` first"). Review: `INVENTORY PASS`, round 2. Owner ruling on Q1: #318 comment 6271450376.

Repositories read (`docs/inventory-scope.md`): this tree; the pin through a `gh api .../tarball/8b99efe9` copy (checked
byte for byte against `gh api contents?ref=8b99efe9`); the default branches of semsource `e4febc0d`, semconnect
`d0d06e00`, semteams `512b4bf2` and semboids `8c03cc53`. The tarball copies are local only.

**The package.** The 625 non-test lines are `doc.go` (293), `helpers.go` (72), `monitor.go` (118) and `status.go`
(142). It imports `regexp`, `strings`, `sync` and `time`, plus `component`, which is already in the tree. It holds no
context, starts no goroutine and touches no NATS. It has no lifecycle owner, so the lifecycle suite does not apply.
Pin probes (go1.26.4): `go test -race -cpu 1 -count=1 ./health/` passes, and so does `-count=5 -shuffle=on`.
Test-text rule hits: `monitor_test.go:359` and `:373` (`time.Sleep`). The pin's testify import is not a hit, because
this repo's `go.mod` already requires testify.

## 1. Shape sweep (pin paths)

- **S1, optional.** One fact is spelled twice: `status.go:27-28` has `Healthy bool` beside `Status string`.
  `IsHealthy`, `IsDegraded` and `IsUnhealthy` (`status.go:44-56`) read only the string, and so does `Aggregate`
  (`helpers.go:52-54`). Every pin producer goes through the `New*` helpers, so no real-world trigger makes the two
  fields disagree.
- **S1b, optional or a doc fix.** `Aggregate` fails open. With no inputs it returns healthy (`helpers.go:44-46`). Any
  word other than `unhealthy` or `degraded`, `""` included, counts as healthy (`:51-65`). No real-world trigger.
- **S2, owner question Q1, now ruled.** `FromComponentHealth` (`status.go:117-121`) reads only `ch.Healthy`, so a
  component that reports `Status: "degraded"` becomes `unhealthy`. This happens for real: a poisoned graph-ingest
  reports `degraded` (`processor/graph-ingest/component.go:866-871`), and the pin's `/components/health` then answers
  503 (`service/component_manager_http.go:298-321`). Two sources pull against each other. The accepted 03B design
  keeps `/components/health` "served as at the pin"
  (`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:725`).
  Bounded port scope protects metadata, and pin ADR-079 Decision 3 says operators alert on "degraded Health". The
  component's status words are not health's words: graph-ingest says `running`, `stopped` or `degraded`. So mapping
  `ch.Status` straight through would turn `stopped` into healthy. Adjacent: #110 item 3.
- **S3, optional.** The error sanitizer redacts by text and loses detail (`status.go:13-22`, `:83-114`). Probes:
  - `"at 12:30:45"` becomes `"at 12[PORT][PORT]"`.
  - `"invalid key=<entity id>"` becomes `"invalid [REDACTED]"`.
  - `"graph/ingest"` becomes `"graph[PATH]"`.
  - `doc.go:159` says the result is `:[PORT]`, but the code produces `[PORT]`.

  Real trigger: graph-ingest's poison message puts entity IDs into `LastError`
  (`processor/graph-ingest/poison_inventory.go:236-246`).
  Dotted IDs pass unchanged, unless they sit next to `key=`, a path, a port or an IP. It stays optional, because
  operators list poisoned entities through DebugStatus (pin ADR-079 `:70`).
- **S4, required (surface audit).** These exports are read only by `health`'s own tests:
  - all 12 `Monitor` symbols (`monitor.go:9-118`);
  - `Status.WithMetrics`, `Status.WithSubStatus` and `Status.IsHealthy`;
  - `Metrics.MessagesProcessed`, which nothing ever writes.

  The two `time.Sleep` calls go with `Monitor`.
- **S5, required (surface audit, items b and c).** Text that claims what the code does not do:
  - `doc.go:80-81` and `:198` say aggregation is recursive, and the example uses the wrong `Aggregate` signature.
  - `:87-101` shows `Metrics` as a map.
  - `:141` says statuses are immutable, but the `*Metrics` pointer is shared.
  - `:169` names a `semstreams/errors` package that does not exist.
  - `:175` claims 100% coverage.
  - `:215` and `:220` put `Monitor` in the HTTP data flow.
  - `:248-264` shows an example goroutine that is never joined.
  - `:283` shows degraded answering 200.
  - `README.md:205-232` shows config that no pin code parses.

  The fix for the recursion claim is the doc, not the code: pin `service/storage_observability.go:468-470` relies on
  aggregation going one level deep.
- **Optional.** `LastActivity: ch.LastCheck` labels a check time as activity (`status.go:131`), and `doc.go:1` still
  says "StreamKit".
- **Shapes 2, 3 and 5** (copied shell, an interface with one implementation, an error classified by its text): searched,
  no hit. The sanitizer rewrites text but classifies nothing.

## 2. Pin ADRs

All 85 pin ADRs were searched for `health/`, `health.Status`, the helper names, `FromComponentHealth` and
`package health`.

- ADR-033 is about semspec's own `pkg/health`. ADR-058 (`:85`) and ADR-094 (`:109`) mention health in passing.
- The readiness records (066, 082-085, 088 and 098) govern readiness, not this package.
- So no pin ADR governs `health`.
- Pin ADR-079 Decision 3 (`079-per-entity-poison-response-authoritative-surface.md:38-42`, `:70`) bears on S2. It is
  not in `docs/adr`.

## 3. Open issues

`gh issue list --state open --search health` returns #110, #155, #109, #216, #291, #162, #161 and #9. Only #9 names
the package, and it is cited as `Addresses #9`. The others use the word, not the package:

- #110 is about `component.HealthStatus.Status`.
- #155 is about `model/health.go`.
- #109, #216 and #291 are about component and graph-ingest health.
- #161 and #162 are harness checks.

This change retires none of them. Adjacent: #30, which is about not porting `milestone_service.go`, one of the
`NewUnhealthy` callers.

## 4. Callers

- **This repo:** none. `component.HealthStatus` (`component/discovery.go:65-72`) is `FromComponentHealth`'s input.
- **Pin:** only `service` imports `health`:
  - `NewHealthy`, `NewDegraded` and `NewUnhealthy`: `base.go:212-227`, `service_manager.go:1747-1750` and
    `milestone_service.go:165`.
  - `Aggregate`: `service_manager.go:1757` and `:1945`, and `component_manager_http.go:304`.
  - `FromComponentHealth`: `component_manager_http.go:300`.
  - `IsUnhealthy` and `IsDegraded`: `service_manager.go:1760-1762` and `component_manager_http.go:319-321`.
  - `storage_observability.go:344` and `:496`.
- **Consumers:** none imports `health` in Go.
  - semteams' e2e tests read only the HTTP status of `/health`.
  - semteams `cmd/semteams/rule_config_service.go:30` is a comment.
  - semconnect `cmd/cs-api-server/main.go:46` probes `:8080/health` (not traced further).
- **Live wire:** the `Status` and `Metrics` JSON is served at `/health`, `/services/health` and `/components/health`.
  It is also published on `health.service.<name>` (pin `service/service_manager.go:785-798`).
- **03B design:** these endpoints are "served as at the pin" (`design.md:722-725`). `health` was admitted as `carry`
  because `service` needs it (`:229`), and it is not held to a coverage floor (`:461`).

## 5. Overlapping pull requests

Draft #316 (`claude/ingest-start-reconnect`) changes `processor/graph-ingest/component.go`, its integration test
and its own OpenSpec change. It shares no file or capability with this change, so either can merge first.

## 6. Already settled

- `/readyz` reads `service.IsHealthy()` and `component.HealthStatus.Healthy`, never `health.Status` (pin
  `service/service_manager.go:1841-1854`). Under ADR-109 (`docs/adr/109-ready-after-boot.md:35-37`, `:92-100`) it is
  the one readiness endpoint.
- ADR-108 Decision 1 (`docs/adr/108-processor-shell.md:58-66`): services report `health.Status`, and components report
  `component.HealthStatus`.
- ADR-113 and `internal/lifecycleguard` say nothing about `health`.
