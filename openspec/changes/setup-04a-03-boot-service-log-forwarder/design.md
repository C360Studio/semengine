# Design: setup-04a-03-boot-service-log-forwarder

base: `9c66931f82cbfae3d55ce8af584e605d927426ae` (draft PR #393). Inputs: `sweep.md` (this change; L-ids and S-ids),
the whole-package inventory (`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md`) and PR
6's archived design (`openspec/changes/archive/2026-10-23-setup-04a-03-boot-service-heartbeat-metrics-forwarder/
design.md`), whose shape this copies. Pin lines are SemStreams `8b99efe9`; `lf:n` is `service/log_forwarder.go`,
`pol:n` is `internal/logforwarderpolicy/policy.go`. One lifecycle owner, 124 non-test pin lines (inventory § 2.3 row 7;
owner, #9 comment 6296023694, option B).

Owner intent applied, not re-asked: ruling (b), #9 comment 6272104271: "once `service` lands, the `log-forwarder`
service is ported and says plainly that it forwards nothing yet (option b). `pkg/logging` and the boot wiring stay out
of the port set. The same PR decides whether the mandatory `flow-service.websocket` exclusion stays." ADR-113 (context
ownership and lifecycle control, settled); no context-value bags; no back doors on production types; port whole, drop
only dead surface; every real defect found and not fixed is filed; simple idiomatic Go; S6 takes the simple fix, the
strict decoder already in the package (owner 2026-10-19).

## Inventory addendum

- **Callers outside `service`.** Pin, Go: none for any exported name of this file (`sweep.md` L1-L3). Consumers reach
  it only through configuration: semteams enables `log-forwarder` in `configs/flow-bootstrap.json`,
  `configs/e2e-flow-bootstrap.json` and two examples, and reads that block itself with its own struct to build its own
  NATS log handler (`cmd/semteams/logging.go:71-93`), importing nothing of this file. semsource, semconnect and
  semboids: no use. The surface reached from outside is the `log-forwarder` configuration block and the record the
  service writes.
- **How it is reached today.** `RegisterAll` does not register it (`service/register.go:19-27`); `config`'s defaults
  do not enable it (`config/config.go:491-498`: `discovery` and `heartbeat` only). A document that enables
  `log-forwarder` is refused at composition through the same path PR 6's tree probe showed for `heartbeat` ("no
  constructor registered for service ...").
- **Open pull requests and issues:** `sweep.md` § 4. This PR retires #30 (D9).

## D1. Files and surface (surface audit, #190)

- **Carried, adapted:** `log_forwarder.go`, module path rewritten.
- **Dropped, no reader** (pin-wide and consumer search, `sweep.md` L1-L3): `NewLogForwarder` with its nil-config
  branch (folded into `NewLogForwarderService`); `LogForwarderConfig.Validate`; `LogForwarder.Config`; the `config`
  field (the service keeps the resolved `logforwarderpolicy.Policy`, L8); the `Stop` override (L7; `BaseService.Stop`
  is promoted); the file-level package comment (L11); the two `validateLifecycleContext` calls (the guard refuses nil
  and ended contexts).
- **Kept with contract lines** (README, ledger `contract`, D10): `NewLogForwarderService` (through `RegisterAll`),
  `LogForwarderConfig` (`min_level`, `exclude_sources`; the strict decode's target, D3), `LogForwarder` with the
  `Service` methods `Start`, `Stop`, `Health`.
- **Base-service options (L9; #387).** The base is built with `WithLogger` and `WithMetrics(deps.MetricsRegistry)`,
  as at the pin (`lf:27-33`), so the service's status is recorded as `semengine_service_status{service=
  "log-forwarder"}`.
- **Logger (L10).** One logger: `deps.Logger`, or `slog.Default()` when none is given, with `service=log-forwarder`,
  given to the base and used for the service's records.
- **Text (L4):** every comment that says logs reach NATS is rewritten to say the service forwards nothing (D4).
- **Options for the dropped accessors:** (a) drop `NewLogForwarder`, `Validate`, `Config`, chosen: nothing reads them
  at the pin or in a consumer, and "can be used by other components" (`lf:121`) names no one; (b) keep them for the
  boot wiring that ruling (b) leaves out: that is the for-future-use shape, and the wiring, when admitted, reads the
  policy package directly, as the pin's does (`internal/bootstrapobservability/bootstrap.go:260`).

## D2. Lifecycle: the base service's, unchanged

Governed by ADR-113. `LogForwarder` starts no goroutine and holds no run context or cancel of its own: its one
continuing task is `BaseService`'s health check, under the run context `BaseService.Start` creates and `Stop` ends
(`service/base.go:196-215`). It composes no guard of its own; `BaseService` composes `internal/lifecycleguard`
(ADR-108 Decision 2; `service/base.go:83-84`), and `Start`'s and `Stop`'s refusals are the base's. Observable
behaviour, each with its proving test (D7 units in brackets):

- **Start** refuses a nil or ended context, a second `Start`, and a `Start` after any `Stop`
  (`errs.ErrAlreadyStarted`); a refused `Start` holds nothing and writes no record. Otherwise it starts the base
  service, writes the record of D4, and returns nil. Ending the Start context after `Start` returned stops nothing.
- **Stop** is `BaseService.Stop`, promoted: it refuses nil; given an ended context it returns that error and stops
  nothing; otherwise it returns nil once the health goroutine has exited, and a later `Stop` returns nil.
- **Proofs:** `TestLogForwarderPassesTheLifecycleSuite` [U-a] (adapter: `Start` and `Stop` on the `LogForwarder`,
  `Observe` as `baseServiceOwner`'s over its `BaseService`, `service/base_lifecycle_test.go:217-234`; must-fail owner a
  log forwarder stopped before `Start`; plus "StartedOwnerReportsTheHealthLoop");
  `TestLogForwarderSaysItForwardsNothing`
  [U-a] (a refused `Start` after `Stop` writes no record).
- **Not needed here** (PR 6 lessons, checked): no `synctest` bubble (no clock is read), no listener, no NATS
  connection.

## D3. Configuration and construction

`NewLogForwarderService(raw, deps)`:

1. Refuses nil `deps` (L5; new). `deps.NATSClient` is not required: nothing here publishes.
2. Decodes `raw` with `decodeStrictServiceJSON` into a `LogForwarderConfig` (`service/service_configs.go:137-152`;
   S6): an unknown key and a key in another letter case are refused, naming the key. This is a check only; the value
   decoded is not used.
3. Resolves `raw` with `logforwarderpolicy.Resolve`, the one home for the defaults, the normalisation and the level
   check (`pol:29-59`): an empty, whitespace or `null` payload gives the defaults; `min_level` defaults to `INFO`, is
   trimmed and upper-cased, and must be `DEBUG`, `INFO`, `WARN` or `ERROR`; `exclude_sources` is trimmed, empties
   dropped and duplicates removed in first-seen order (and, under D6 option A, `flow-service.websocket` put first).

Every refusal is invalid data (`errs.WrapInvalid`, L6), naming the input. No I/O. Configuration is applied by restart.

Options for the letter-case check: (a) the strict decode before `Resolve`, chosen: it is the package's one check
(`service-lifecycle`, "A service configuration key in another letter case is refused") and leaves the carried policy
package untouched; the cost is a second parse of a payload of two keys; (b) `keycase.Check` inside `Resolve`: edits a
carried file for a rule that belongs to `service`, and `internal/logforwarderpolicy` would import `internal/keycase`;
(c) accept miscased keys: a second rule for one payload in a package that refuses them everywhere else.

Proofs [U-a]: `TestLogForwarderConstructionRefusals`, one subtest per refusal, each asserting `errs.IsInvalid` and
the named input: nil `deps`; malformed JSON (`{"min_level":`); a second JSON value; `min_level` `TRACE` and `INVALID`
(`invalid log level`); the unknown key `min_levl`; the keys `Min_Level` and `Exclude_Sources` (`config key Min_Level
must be written min_level`). The pin's accepted rows (`log_forwarder_test.go:14-114`) move to
`TestLogForwarderSaysItForwardsNothing` (D4), which reads the applied values from the record. The pin's `Validate`
row for an empty level (`:150-155`) has no counterpart: `Resolve` defaults an empty level to `INFO`.

## D4. What it says: it forwards nothing

Ruling (b). Nothing in this module publishes a log record to NATS; the service accepts and checks its configuration,
runs the base service's lifecycle, and says so:

- At `Start`, after the base service started, it writes one record at **Warn** through its logger, with the message
  `log-forwarder forwards nothing: no log handler in this module publishes logs to NATS`, and the attributes
  `service` (`log-forwarder`), `min_level` (the resolved level's name) and `exclude_sources` (the resolved list). No
  other record is written while it runs. It publishes nothing, subscribes to nothing, and holds no NATS connection.
- **Why Warn.** An operator who enables `log-forwarder` asks for something the module does not do. Warn is the level
  this package already uses for a configured request it cannot honour as asked: a composition finding
  (`service/component_manager.go:366`) and a profiling-server bind failure (ledger `service` row, PR 1). Options: Warn
  (chosen; owner ruling below); Info (the pin's level for its "started" line, `lf:104`); refuse to construct
  (contradicts ruling (b)).
- **Health.** `Health()` is the base's: healthy while running. The service does everything this module promises of it,
  so it does not report degraded (degraded would read as a fault in `/health` and `/readyz` for a configuration
  choice).
- **Docs:** `service/README.md` and `service/doc.go` say, in their first sentence on the service, that it forwards
  nothing yet and why (ruling (b)); `LogForwarderConfig`'s field comments say the values are checked and reported,
  not applied.
- **Proofs [U-a]:** `TestLogForwarderSaysItForwardsNothing`: the composition's logger is `slog.NewJSONHandler` over a
  buffer (a production handler, the independent oracle); configs `{}`, `null`, `{"min_level":"debug"}`, `{"min_level":
  "warn","exclude_sources":[" a ","a","","b"]}`; after `Start` exactly one record, at `WARN`, with the message above,
  `service` `log-forwarder`, the expected `min_level` (`INFO`, `INFO`, `DEBUG`, `WARN`) and `exclude_sources` (`[]`,
  `[]`, `[]`, `["a","b"]` under D6 option B); after `Stop`, still one record; a second `Start` refused, still one.
  `TestLogForwarderReportsItsStatus`: `semengine_service_status{service="log-forwarder"}` reads 0 when built, 2 after
  `Start`, 0 after `Stop` on the injected registry (`requireGauge`, `service/gauge_helpers_test.go:50`), as
  `TestMetricsForwarderReportsItsStatus` does (`service/metrics_forwarder_test.go:188`).

## D5. Composition

`RegisterAll` gains `log-forwarder` → `NewLogForwarderService`, with no OpenAPI spec (no route); its doc comment names
it. `config`'s defaults do not enable it, so a document that names no services composes no log forwarder. Proofs
[U-a]: `TestLogForwarderIsRegistered` (the registry offers a constructor under `log-forwarder` that builds a
`*LogForwarder` from `{}` and `createTestServiceDependencies()`); `TestLogForwarderComposedWhenEnabled` (the document
`{"platform":{"org":"acme","id":"p1"},"services":{"log-forwarder":{"enabled":true,"config":{"min_level":"warn"}}}}`
loaded with `config.NewLoader().LoadFromBytes`, then `ConfigureFromServices` with `RegisterAll` and
`testComponentDependencies`: `GetService("log-forwarder")` finds a `*LogForwarder`; the same document without the
`services` block: not found).

## D6. The `flow-service.websocket` exclusion (Q1: dropped)

`pol:14`, `pol:87` put `flow-service.websocket` first in every resolved exclusion list (`sweep.md` L12). At the pin it
kept SemStreams' flow-service WebSocket worker logs out of the NATS log stream, so streaming logs to a browser did not
log about itself (semteams' copy says "to prevent feedback loops", `cmd/semteams/logging.go:73`). Here no handler
forwards, no `flow-service` exists or is planned in the port set, and semteams keeps its own default in its own code.

| Option | What changes | Cost |
| --- | --- | --- |
| A. Keep | Nothing; `policy.go` stays byte-equal to the pin (ledger `carry`) | The framework's reported config names a SemStreams service; docs must explain a list entry nobody configured; the decision moves to whoever admits the handler |
| B. Drop now (recommended) | `pol:14` and `pol:87` removed, `policy_test.go`'s expected lists lose the inserted entry (configured ones stay); ledger row `carry` to `adapt`; the spec states the list is exactly what was configured, normalised | About 4 production and 5 test lines in a second package, which the ruling put in this PR |
| C. Keep and defer by issue | As A, plus an issue scheduling the decision with the `pkg/logging` admission | An issue that waits on an admission nobody has scheduled; the ruling asked this PR to decide |
| D. Replace with a SemEngine safety exclusion | A different mandatory source, such as the forwarder's own | A guard against a loop no code here can make; no handler to test it against |

Recommendation: **B.** The entry is a consumer's source name (`AGENTS.md` § What this is for: SemEngine "owns
primitives and contracts, never a consumer's domain semantics"), nothing reads the list, and no consumer depends on
SemEngine's default (semteams carries its own). A feedback-loop guard is a property of a log handler that publishes to
NATS; if `pkg/logging` is admitted, its design states the guard against that handler. Under B the `Policy` doc comment
(`pol:16`, "consumed by boot composition") is also corrected to name the log-forwarder service, the file's only other
untrue line (archived `port-logforwarderpolicy` design, shape findings). Under A or C, P-2 and U-b below are dropped
and D4's expected lists keep `flow-service.websocket` first.

## D7. Tests

| Pin test lines | Here |
| --- | --- |
| `:14-114` config table | refusal rows → `TestLogForwarderConstructionRefusals`; accepted rows → `TestLogForwarderSaysItForwardsNothing` (read from the record, not the private field); deps built by `createTestServiceDependencies()`, not `&natsclient.Client{}` |
| `:116-179` `Validate` | dropped with `Validate` (L2); its level rows are covered by the construction refusals |
| `:181-207` lifecycle | `TestLogForwarderPassesTheLifecycleSuite` on the production constructor |
| `:209-220` interface | dropped: `NewLogForwarderService` returns `Service`, so the compiler checks it |
| `:222-248` `Config()`, nil config | dropped with L1 and L3; the defaults are `{}` and `null` rows of D4 |
| `:250-270` `exclude_sources` | the fourth config of `TestLogForwarderSaysItForwardsNothing` |

**Test units** (task 2.1; parallel; each brief cites only the D-sections and line ranges named; each unit compiles
against its own overlay stub, a scratch `log_forwarder.go` declaring the names its tests call with no behaviour, kept
outside the repository, so `notes/2.1.md` records assertion failures, not only compile errors):

- **U-a** `service/log_forwarder_test.go` (new) and one line in `internal/keycase/guard_test.go:23-29` (the list gains
  `service.LogForwarderConfig`). D1-D5.
  - `TestLogForwarderConstructionRefusals`, `TestLogForwarderSaysItForwardsNothing`, `TestLogForwarderReportsItsStatus`,
    `TestLogForwarderIsRegistered`, `TestLogForwarderComposedWhenEnabled`, `TestLogForwarderPassesTheLifecycleSuite`.
  - Calls: `NewLogForwarderService`, `Start`, `Stop`, `Status`, `RegisterAll`, `NewServiceRegistry`,
    `Registry.Constructor`, `NewServiceManager`, `ConfigureFromServices`, `GetService`, `config.NewLoader`,
    `LoadFromBytes`, `createTestServiceDependencies`, `testComponentDependencies`, `metric.NewMetricsRegistry`,
    `requireGauge`, `stopContext`, `baseServiceOwner`, `lifecycletest.Run`, `errs.IsInvalid`, `slog.NewJSONHandler`.
- **U-b** (D6 option B only) `internal/logforwarderpolicy/policy_test.go`: each expected list loses only the
  automatically inserted `flow-service.websocket`. A source the case configures itself stays, in its first-seen
  position: "normalizes and deduplicates" configures `["metrics-forwarder", "flow-service.websocket",
  "metrics-forwarder"]` and expects `["metrics-forwarder", "flow-service.websocket"]`. The case
  `exclude_sources: []` expects an empty list. D6.
- **U-c** (#30) `internal/harness/contract/servicedeps_test.go` (new): `TestServiceReachesNoAgenticPackage` (`go list
  -deps ./service` names no path with an `agentic` element, the test `componentDepViolations` makes for
  `./component`, `internal/harness/contract/componentdeps_test.go:86-121`, with the agentic check only) and its paired
  `TestServiceReachesNoAgenticPackageSensitivity` (a fixture module whose `service` reaches `agentic` through a middle
  package fails; the clean fixture passes; an empty listing fails). D9. A structural guard passes on the base by
  design; `notes/2.1.md` records the sensitivity test's failure on a mutated fixture instead.

**Port units** (task 2.2), each ending with `go test -race` green on the package it touches, `task verify` at the end
of the last:

- **P-1** `service/log_forwarder.go` (new) and the `log-forwarder` line and doc comment of `service/register.go`.
  D1-D5. U-a turns green.
- **P-2** (D6 option B only) `internal/logforwarderpolicy/policy.go`: `pol:14` and `pol:87` removed, `pol:16`
  corrected. D6. U-b turns green.

U-c needs no port unit. Harness rules as PR 6's D6: no `time.Sleep`, `Skip` or fixed port; cleanup stops under
`stopContext`. No real-clock window: every assertion follows a synchronous call. Generated checks: none; the inputs
are a two-key payload with independent cases and no history beyond the guard's, which `internal/lifecycleguard` and
the suite already cover. Examples suffice.

**Sensitivity** (task 2.3). Every row through `task mutate:check`; a row counts as a detection only when the named test
fails under the mutant. A hang or timeout is inconclusive, not a detection. Each mutant leaves the package compiling.

| Mutant | Caught by |
| --- | --- |
| the `decodeStrictServiceJSON` check removed | `TestLogForwarderConstructionRefusals` (the `Min_Level` subtest) |
| the nil-`deps` refusal replaced by `deps = &Dependencies{}` | `TestLogForwarderConstructionRefusals` (the nil-`deps` subtest builds a service) |
| `WithMetrics` dropped from the base options (#387) | `TestLogForwarderReportsItsStatus` |
| the base built with `WithLogger(nil)` | `TestLogForwarderSaysItForwardsNothing` (no record in the buffer) |
| the record at Info | `TestLogForwarderSaysItForwardsNothing` (level) |
| the record written before `BaseService.Start`'s result is checked | `TestLogForwarderSaysItForwardsNothing` (a refused `Start` adds a record) |
| `min_level` reported as `INFO` whatever was resolved | `TestLogForwarderSaysItForwardsNothing` (the `debug` row) |
| `RegisterAll` without `log-forwarder` | `TestLogForwarderIsRegistered`; `TestLogForwarderComposedWhenEnabled` |
| (B) `pol:87` restored | `policy_test.go`'s table; `TestLogForwarderSaysItForwardsNothing` (`exclude_sources`) |
| (#30) the agentic check in the new violations function removed | `TestServiceReachesNoAgenticPackageSensitivity` |

Uncovered, recorded in `notes/2.3.md`: the archived policy design's five survivors (S1-S5 there) stay as recorded.

**Brief line ranges** (this file; a brief cites only these):

| Unit | Lines |
| --- | --- |
| U-a | 1-16, 31-141, 164-186, 207-211 |
| U-b | 142-163, 187-191, 207-211 |
| U-c | 192-198, 207-211, 274-278, 283-296 |
| P-1 | 1-16, 31-141, 199-203 |
| P-2 | 142-163, 204-206 |
| task 2.3 | 212-228 |

## D8. Adopter seam (a config author enabling `log-forwarder`)

1. **They must know:** (i) it forwards nothing: logs reach NATS only if their composition root installs a handler
   that publishes them, as semteams does in its own code; (ii) the keys are `min_level` and `exclude_sources`, spelled
   in lower case.
2. **If they do nothing:** a document that enables it boots (today it is refused, no constructor), and the service
   writes one Warn record saying it forwards nothing. A misspelt or miscased key is refused at boot, naming it, where
   the pin applied it or dropped it.
3. **Where they find out:** (i) a log line at boot, and the README; (ii) a boot error.
4. **What they should have to know:** nothing, once a handler is admitted; until then (i) is the gap, and ruling (b)
   accepts it as a stated gap rather than an unstated one.

Intent check: no boundary moves in this PR. Log forwarding to NATS stays deferred by ruling (b) (#9 comment
6272104271); every other capability of `AGENTS.md` § What this is for is unchanged by this file.

## D9. Ledger, text, ADRs, issues, merge order

- **Ledger, `service` row** (edited in place): `consumer_purpose` gains PR 7 (framework API through configuration; the
  record; semteams enables the block and reads it itself); `contract` gains the D10 requirements, ruling (b), ADR-113,
  ADR-108 Decision 2 through `BaseService`, ADR-110 (no `ENTITY_STATES` entry) and the kept names of D1;
  `dependencies_and_side_effects` gains `internal/logforwarderpolicy` and "starts no goroutine of its own";
  `known_risks` gains PR 7's dropped names (D1), changes (strict keys, nil `deps` refused, Warn record, the service
  name on its records) and #30: `class:port-refactor`, `service/milestone_service.go` not ported (03B design D4a);
  `proving_tests` gains `TestServiceReachesNoAgenticPackage`.
- **Ledger, `internal/logforwarderpolicy` row:** `consumer_purpose` names its importer, now in the tree; `known_risks`
  drops "no importer". Under B, `disposition: adapt`, `contract` without the mandatory exclusion, and `known_risks`
  names the owner's Q1 answer.
- **Text:** `service/README.md` (a "The log forwarder" section; the `RegisterAll` list; the intro) and
  `service/doc.go` (the same); ADR-058's port note (`docs/adr/058-boot-lifecycle-phases.md:31-32`) no longer lists
  `LogForwarder` as not yet ported; `docs/repository-map.md` rows `service/` ("The last service follows in PR 7" goes)
  and `internal/logforwarderpolicy/` ("Nothing imports it" goes; under B, "always excludes" goes).
- **ADRs:** cited as `sweep.md` § 5; none carried, none changed or dropped. **Guidance:** none to carry.
  **Coverage:** the `service` floor holds; no new target.
- **Issues:** retires **#30** (`Closes #30`; task 2.1 U-c and task 3.1's ledger text; proof
  `TestServiceReachesNoAgenticPackage`): the work it names, not porting `milestone_service.go`, is done, and this, the
  last `service` PR, is where the inventory scheduled its proof (inventory § 9). Comments on #196 (`LogForwarder`
  composes the guard through `BaseService`) and, after merge, #364 (unblocked). No other open issue names this file
  or the policy package (`sweep.md` § 4).
- **Merge order:** #382 merged first (`c770283`) and `origin/main` is merged into this branch. Open PR #395 (docs,
  `metric/README.md` and `docs/repository-map.md`) also edits the repository map that task 3.1 updates; whichever
  merges second merges `origin/main` (never a rebase) and keeps every edit of both, before task 3.2's spec sync.

## D10. Spec deltas (written after acceptance; `skip_specs` removed then)

- **`service-lifecycle`, ADDED:**
  - "The log forwarder refuses an invalid configuration" (D3, D5; scenarios: a refusal; the constructor registered,
    composed only when enabled; under B, the exclusion list is what was configured)
  - "The log forwarder's Start bounds Start and Stop is the only stop" (D2)
  - "The log forwarder forwards nothing and says so" (D4)
  - "The service package reaches no agentic package" (D9, #30)
- **`service-lifecycle`, MODIFIED:** "A service configuration key in another letter case is refused" names the log
  forwarder's payload and adds `service.LogForwarderConfig` to the guard's list.
- **`operator-surface`:** none; the service serves no route.

Draft: `specs/service-lifecycle/spec.md` beside this file, written for D6 option B, which the owner chose.

## Owner rulings

Asked and answered in the Enjoy design stop, 2026-10-24 (record: PR #393 comment):

- **Q1. Does the mandatory `flow-service.websocket` exclusion stay?** Dropped (D6 option B, as recommended). Units U-b
  and P-2 stay in.
- **Q2. The level of the forwards-nothing record (D4).** Warn, as recommended.
- **Q3. Does this PR retire #30?** Yes (`Closes #30`); unit U-c stays in.

## Addenda from task 2.1

Recorded at the end so the brief line ranges above stay valid.

- **Empty exclusion list.** `Resolve` returns a non-nil empty slice when nothing is configured, and the record writes
  it as `[]`, never `null` (U-a and U-b assert this; today's `normalizeExclusions` already builds a non-nil slice).
- **The `service` attribute** appears once on the record: set through the logger, not repeated on the record (D1).
- **Text that P-2 makes untrue, for task 3.1:** `docs/admission-ledger.yaml:2595` (the policy row's mandatory
  exclusion) and `:2605` (cites `TestResolveOwnsDefaultsNormalizationValidationAndSafetyExclusion`, whose name keeps
  "SafetyExclusion"; task 3.1 renames the test and the citation together), and `docs/repository-map.md:86` ("always
  excludes `flow-service.websocket`").
