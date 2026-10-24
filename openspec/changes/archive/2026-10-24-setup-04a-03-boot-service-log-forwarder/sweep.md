# Shape sweep: setup-04a-03-boot-service-log-forwarder (PR 7 of 7)

base: `9c66931f82cbfae3d55ce8af584e605d927426ae` (`main` `66c797a` plus the claim commit; draft PR #393)

The per-PR read the architect contract asks for (§ The shape sweep), of the one file this pull request carries, on the
scratch copy of the SemStreams pin `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` that PR 1's inventory built (local
only). It re-uses the whole-package inventory, `INVENTORY PASS` round 2 at `ad946b7`
(`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md`, "the inventory" below), wherever
that already pins a line, and adds what it did not pin. Disposition is proposed here and settled in `design.md`. Pin
lines are `service/log_forwarder.go:<line>`, written `lf:n`; `pol:n` is `internal/logforwarderpolicy/policy.go`, which
is byte-equal to this tree's copy (`cmp` of the two files: equal; ledger row `internal/logforwarderpolicy`,
`disposition: carry`).

Files: `log_forwarder.go`, 124 non-test pin lines; `log_forwarder_test.go`, 270 test lines (§ 6).

## 1. Findings the inventory already pins (cited, not repeated)

| Id | Shape | Pin lines in this file | Disposition |
| --- | --- | --- | --- |
| S1 | run context descends from the Start context | none in this file: `LogForwarder` has no goroutine of its own; its one, the health check, is `BaseService`'s (`base.go:254` at the pin) | none here; `BaseService` already passes the suite in this tree (`service/base_lifecycle_test.go:250`) |
| S3 | copied lifecycle state machine | none: `lf:96-118` delegate to `BaseService`, which composes `internal/lifecycleguard` here (`service/base.go:83-84`) | none; design D2 |
| S6 | config decoder accepts a key in another letter case | `lf:17` — `policy, err := logforwarderpolicy.Resolve(rawConfig)`; `pol:32-34` decode with `DisallowUnknownFields` only (probe § 3) | required by the package rule: `service-lifecycle`, "A service configuration key in another letter case is refused", covers every strictly decoded service payload; simple fix, the strict decoder already in the package (owner, 2026-10-19); design D3 |
| § 5 (b) | config read by no behaviour | `lf:47` `MinLevel`, `lf:53` `ExcludeSources`, read only by the log line `lf:104-106` | covered by ruling (b) (#9 comment 6272104271): kept, and the service says it forwards nothing; design D4 |
| § 7 | lifecycle suite on the pin's `LogForwarder` | 7 of 8 pass; `StartContextEndStopsNothing` fails, "stopped holding [status=running]", through the pin's `BaseService` | none here (inherited fix, design D2) |

**What the scratch build needs beyond the module path.** `NewBaseServiceWithOptions` lost its config argument (PR 1's
Q1), so `lf:83` changes; `validateLifecycleContext` (`lf:97`, `lf:113`) does not exist in this tree (the guard in
`BaseService` refuses nil and ended contexts). Each is a row below.

## 2. Findings added by this sweep

Each is `path:line` — the line's text at the pin, then the shape and the proposed disposition.

- **L1, an exported constructor with no reader outside the package (required: surface audit (a)).** `lf:70` —
  `func NewLogForwarder(config *LogForwarderConfig, opts ...Option) (*LogForwarder, error) {`. Readers at the pin:
  `lf:35` and the pin's tests only (`grep -rn --include='*.go' -e LogForwarder` over the pin copy, outside
  `service/log_forwarder*.go` and `internal/logforwarderpolicy/`: no Go hit; `internal/bootstrapobservability`
  calls `logforwarderpolicy.Resolve` directly, `bootstrap.go:260`). Consumers: none (same search over the semsource,
  semconnect, semteams and semboids tarballs; semteams reads the `log-forwarder` block with its own struct,
  `cmd/semteams/logging.go:80-90`). Its nil-config branch `lf:71-80` is reachable only from tests: `lf:35` always
  passes a config. Recommended: folded into `NewLogForwarderService`.
- **L2, `Validate` with no reader (required: surface audit (a)).** `lf:57` — `func (c LogForwarderConfig) Validate()
  error {`. No reader outside the pin's tests (same search). The pin's `gh955` proposal kept the named type
  `service.LogForwarderConfig` and had `Validate` delegate (`docs/proposals/gh955-bootstrap-logger-design.md:379-382`
  at the pin); it names no caller of `Validate`. Dropped (PR 6 H4 precedent).
- **L3, `Config()` with no reader (required: surface audit (a)).** `lf:122` — `func (lf *LogForwarder) Config()
  LogForwarderConfig {`, "can be used by other components" (`lf:121`). No reader outside the pin's tests (same
  search). "Can be used" is the for-future-use shape (contract, inventory category 4). Dropped.
- **L4, text that says logs reach NATS (required: text; ruling (b)).** `lf:13-15` — "logs are published to NATS
  directly by the NATSLogHandler in pkg/logging"; `lf:41-43`, `lf:45-46`, `lf:49-52` (prefix matching is
  `pkg/logging`'s behaviour, `pkg/logging/doc.go:84-85` at the pin), `lf:61-63`, `lf:94-95` ("handled by
  NATSLogHandler in main.go"), `lf:104` — `lf.logger.Info("LogForwarder started (NATS bridge mode)",`. Neither
  `pkg/logging` nor the boot wiring is in the port set (ruling (b)), so every one of these is documented and not
  implemented (surface audit (c)). Rewritten to say it forwards nothing (design D4).
- **L5, nil dependencies panic the constructor (optional, #185 MEDIUM at most).** `lf:28` — `if deps.Logger != nil
  {`: `NewLogForwarderService(raw, nil)` dereferences nil. The service manager always passes non-nil dependencies
  (`service/service_manager.go:150-157`), so no composition reaches it. Recommended: refused as invalid, as the
  heartbeat and the forwarder do (`service/heartbeat.go:67-69`).
- **L6, construction errors are not classified (optional).** `lf:19` — `return nil, fmt.Errorf("parse log-forwarder
  config: %w", err)`, `lf:74`. Recommended `errs.WrapInvalid` (PR 5 O10, PR 6 H10 precedent).
- **L7, a Stop override that only logs (optional; shape 2, a copied shell).** `lf:112-118`: checks the context, logs
  "LogForwarder stopping", then calls `BaseService.Stop`. The check is `BaseService`'s guard's; the record says
  nothing the base does not. Recommended: no override; `BaseService.Stop` is promoted.
- **L8, two spellings of the level (optional, retained).** `lf:22` — `MinLevel: policy.MinLevel.String(),` turns
  the policy's `slog.Level` back into the string the policy parsed (`pol:49`, `pol:56`); the archived
  `port-logforwarderpolicy` design already retained "the two level spellings" as optional. The service keeps the
  resolved `Policy` and writes `MinLevel.String()` only in its record (design D1).
- **L9, the base-service options (keep both; #387 lesson).** `lf:27-33` — `opts = append(opts,
  WithLogger(deps.Logger))` and `opts = append(opts, WithMetrics(deps.MetricsRegistry))`. `WithMetrics` makes the
  base record `semengine_service_status{service="log-forwarder"}` (`service/base.go:281-285`). PR 6 dropped the
  forwarder's by accident (#387, closed). Both kept, with a test.
- **L10, records lack the service name when a logger is injected (optional; recommended).** `lf:29` passes
  `deps.Logger` itself, so `WithLogger` replaces the base's default `slog.Default().With("service", name)`
  (`service/base.go:98-101`) with a logger that carries no `service` attribute. The heartbeat adds it
  (`service/heartbeat.go:82-86`). Recommended: one logger, `deps.Logger` (or the default) with `service=log-forwarder`,
  given to the base and used for the record.
- **L11, a second package comment (required: text).** `lf:1` — `// Package service provides the LogForwarder service
  for log management.` `service/doc.go` is the package's (PR 6 H9 precedent). Dropped.
- **L12, the mandatory exclusion names a service that exists in neither tree (owner question; record:
  `openspec/changes/archive/2026-10-19-port-logforwarderpolicy/design.md`, Owner question).** `pol:14` — `const
  mandatoryWebSocketExclusion = "flow-service.websocket"`, prepended by `pol:87` —
  `appendUnique(mandatoryWebSocketExclusion)`
  to every resolved policy (probe § 3: `{"exclude_sources":[]}` resolves to `["flow-service.websocket"]`). At the
  pin it is a spec requirement (`openspec/specs/service-composition/spec.md:167-168`: "Effective exclusions SHALL
  union configured exact/dotted-prefix exclusions with the mandatory `flow-service.websocket` safety exclusion"),
  read only by `pkg/logging.NATSLogHandler` through `internal/bootstrapobservability/bootstrap.go:260`. In this tree:
  no `flow-service` package, no source named `flow-service.websocket` (`git grep -n flow-service` outside archives:
  `config/streams.go:109`, a comment example; the policy, its test, the ledger and the repository map), and no reader
  of `ExcludeSources` (nothing forwards). semteams keeps its own default in its own code
  (`cmd/semteams/logging.go:74` — `excludeSources := []string{"flow-service.websocket"}`), "to prevent feedback
  loops" (`:73`). Options in design D6.

Not findings: `NewLogForwarderService` (registered by `RegisterAll` at the pin, `service/register.go:12`);
`LogForwarderConfig` (the configuration shape, kept as a named type by the pin's `gh955` design, and the strict
decode's target here); `LogForwarder` (the constructor's concrete type); `Start`, `Stop`, `Health` (the `Service`
methods the manager calls).

Searched and empty in this file: new roots or context values (`grep -n -E 'context\.Background|context\.TODO|
WithoutCancel|WithValue|ctx\.Value|select \{\}'`: 0 hits); a `context.Context` field: none; a goroutine: none
(`grep -n 'go '`: 0); shape 5 (`strings\.Contains\(err|err\.Error\(\)`: 0); shape 3 (no interface declared);
`GenericJSON`: 0; `init()`: 0; OpenAPI or HTTP routes: none; test-only production code: none.

## 3. Pin probes

The inventory's § 7 ran the lifecycle suite on `LogForwarder` at the pin (7 of 8; `StartContextEndStopsNothing`
fails through the pin's `BaseService`), the unit runs (once under `-race`, five times shuffled) and the test-text
guards on the whole package; `log_forwarder_test.go` has no line any guard rejects (`grep -n -E
'time\.Sleep|Skip|//go:build'`: 0). Added here, 2026-10-24, the configuration decode: the pin's `policy.go` (equal to
this tree's) copied into a scratch module `lfprobe` as package `policy`, with one test calling `Resolve` on each
input. go1.26.4 darwin/arm64 (the stdlib JSON key matching is the same at the tree's go1.26.10; not re-run there).
Command: `go test -count=1 -v ./policy/`. Output:

```text
{"Min_Level":"debug"}                     -> level=DEBUG exclude=["flow-service.websocket"] err=<nil>
{"MIN_LEVEL":"ERROR"}                     -> level=ERROR exclude=["flow-service.websocket"] err=<nil>
{"min_level":"DEBUG","MIN_LEVEL":"ERROR"} -> level=ERROR exclude=["flow-service.websocket"] err=<nil>
{"Exclude_Sources":["a"]}                 -> level=INFO exclude=["flow-service.websocket" "a"] err=<nil>
{"min_level":"warn","exclude_sources":["flow-service.websocket"]} -> level=WARN exclude=["flow-service.websocket"] err=<nil>
{"exclude_sources":[]}                    -> level=INFO exclude=["flow-service.websocket"] err=<nil>
{"min_levl":"debug"}                      -> level=INFO exclude=[] err=decode log-forwarder config: json: unknown field "min_levl"
null                                      -> level=INFO exclude=["flow-service.websocket"] err=<nil>
```

Read: an unknown key is refused; a key in another letter case is applied, and beside the correct spelling the later
one wins (S6, the #344 shape); the mandatory exclusion is always first (L12). The service constructor at the pin:
`lf:28` dereferences `deps` (L5, read from the code; not run).

## 4. Open issues and pull requests

Queries, 2026-10-24, `gh issue list --state open --search <q> --limit 200`: `LogForwarder`, `logforwarderpolicy`,
`log-forwarder`, `log forwarder` (#9 each); `log_forwarder`, `exclude_sources`, `min_level`, `NATSLogHandler` (none);
`forwarder` (#384, #9); `flow-service` (#363, #9, #7); `websocket` (#34, #284, #196, #195, #9); `pkg/logging` (#9).
`service` and `lifecycleguard` as in PR 6's sweep (§ 4 there), re-read for this file.

- **#30** (do not port `milestone_service.go`; `class:port-refactor`) names `service/milestone_service.go:10`. The
  inventory's § 9 schedules it: "Retired by the last `service` PR, when `go list -deps ./service` can show no
  `agentic` package for the whole package." This is the last. Measured on the base: `go list -deps ./service | grep
  -c agentic` = 0. Its proving test ("Slice 04A's `go list -deps ./service` reaches no `agentic*` package") does not
  exist yet (`git grep -n -i agentic -- internal/harness/contract`: only `componentdeps_test.go`, for `./component`),
  and the `service` ledger row names neither #30 nor `milestone_service.go`. Proposed retired here (design D8).
- **#196** (lifecycleguard adoption) names the copied state machines; `LogForwarder` has none of its own (§ 1 S3).
  A comment; stays open for changes 4-7.
- **#364** (collapse the `componentStartup` seam once all service PRs have landed) is unblocked by this merge and is
  not this PR's work (PR 2's seam, `service/service_manager.go:40-47`). Stays; a comment after merge.
- **#195** (`natsfixture.Open`): this PR's tests open no NATS connection. Not touched.
- Name the word, not the package: #384 (`config`'s phantom `discovery` default); #363 (flow-path reachability);
  #34 (`output/websocket`); #284 (`tlsutil`); #9, #7 (epics).

Open pull requests (`gh pr list --state open --json number,title,isDraft,changedFiles,files`, 2026-10-24): **#393**,
this claim (3 files). **#382**, ready (not draft), `fix(natsclient): Subscribe stops parenting handlers on its setup
context` (29 files), head `35d0eb1`: shares `service/README.md`, `service/doc.go`, `docs/admission-ledger.yaml`,
`docs/repository-map.md` and `openspec/specs/service-lifecycle/spec.md` with this PR's tasks 3.1-3.2. Its hunks:
the spec's Purpose (`:8-10`) and the message logger's requirements (`:537-576`); the README's message-logger
section (`:446-475`); `doc.go` `:340-366`; the repository map's `natsclient` row (`:59`); ledger `:1012-1027`,
`:1128-1136` (not the `service` or `internal/logforwarderpolicy` rows). No hunk overlaps a line this PR edits.
Merge order in design D8.

## 5. Pin decision records for this file (inventory § 8)

Searched the 85 records at the pin (`grep -l -i -E 'LogForwarder|log-forwarder|log_forwarder|exclude_sources|
NATSLogHandler|logforwarderpolicy|flow-service.websocket'`): 058 only. Read by title for logs and observability: 098.

- **ADR-058** (boot lifecycle phases; in this tree, `docs/adr/058-boot-lifecycle-phases.md`) lists `LogForwarder` as
  an exemplar boot citizen (pin `058:56`, tree `:88`) and its port note says "Not yet ported ... `LogForwarder`"
  (tree `:31-32`), stale with this PR; task 3.1 edits the note. Cited.
- **ADR-098** Decision 3, "Substrate observability is logs and metrics, out of band" (tree `:80-81`). Kept: logs
  still leave through whatever handler the composition root gives `deps.Logger`; this service adds no path.
- **ADR-095** (one-shot running lifecycle; not in this tree): single use, through `BaseService`'s guard.
- No record governs `internal/logforwarderpolicy` (archived `port-logforwarderpolicy` design: "ADRs: none binds").
  The pin's `docs/proposals/gh955-bootstrap-logger-design.md` is a proposal, not a record; it decided the resolver
  and the named type (L2), cited, not carried.
- This repository: ADR-113 (settled), ADR-108 Decision 2 (through `BaseService`), ADR-110 (no `ENTITY_STATES`
  entry).

Pin guidance: no contract section or skill names the log forwarder (`grep -rn -i 'log.forwarder' .agents/` on the pin
copy: no hit). Nothing to carry.

## 6. Pin tests (270 lines)

| Lines | What it reaches |
| --- | --- |
| `:14-114` | `NewLogForwarderService` config table (four levels, `{}`, nil, lower case, two bad levels, malformed JSON) over `Dependencies{NATSClient: &natsclient.Client{}}`; reads the private `config` (L3) |
| `:116-179` | `LogForwarderConfig.Validate` table (L2) |
| `:181-207` | Start/Stop on `NewLogForwarder` (L1) under `context.Background()` |
| `:209-220` | `var _ Service = lf` |
| `:222-248` | `Config()` (L3) and the nil-config defaults, including `["flow-service.websocket"]` (L1, L12) |
| `:250-270` | `exclude_sources` parsed, read from the private `config` |
