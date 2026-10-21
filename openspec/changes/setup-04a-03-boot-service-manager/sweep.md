# Shape sweep: setup-04a-03-boot-service-manager (service PR 2 of 7)

base: `db7ef2c17e61f77150f22d587ace630d2ebe3bd5` (`main` `3453d9b` plus the claim stub; draft PR #353)

The per-PR read the architect contract asks for (§ The shape sweep), of the nine files this pull request carries, on a
scratch copy of the SemStreams pin `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` (`gh api .../tarball/8b99efe9`, local
only). It re-uses the whole-package inventory, `INVENTORY PASS` round 2 at `ad946b7`
(`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md`, "the inventory" below), wherever
that already pins a line, and adds what it did not pin. Disposition is proposed here and settled in `design.md`.

Files (non-test pin lines, 2,960 in all): `service_manager.go` 1,958, `metrics.go` 382, `graph_triples_http.go` 250,
`startup_metrics.go` 104, `service_manager_config.go` 82, `registry.go` 72, `dependencies.go` 43, `middleware.go` 42,
`register.go` 27.

## 1. Findings the inventory already pins (cited, not repeated)

| Id | Shape | Pin lines in these files | Disposition |
| --- | --- | --- | --- |
| S1 | run context from the Start context | `service_manager.go:707`, `:1181`, `:1308`; `metrics.go:134` | required (ADR-113; probe § 3) |
| S2 | provider closure keeps a context | `service_manager.go:1189-1190`, `:1314` | required (probe § 3) |
| S3 | copied lifecycle state machines | `service_manager.go:62-80` (three), `metrics.go:29-38` | optional; design D2, D4 |
| S5 | second stop sentinel read | `service_manager.go:888` | required (03B SS#1218; owner Q2) |
| S6 | strict decoder takes another letter case | `service_configs.go:130-139`, read at `service_manager.go:178`, `metrics.go:74` | required (PR 1 D5) |
| S8 | health publisher drops failures | `service_manager.go:680-693`, `:792-794`, `:798` | required in part |
| S9 | test-only fields and seams | `service_manager.go:84-88`, `:1284`; `metrics.go:40-44` | optional in part; D2, D4 |
| S10 | one-implementation interfaces | `metrics.go:47`; `dependencies.go:20` | optional; D1, D4 |
| S11 | two verb pairs on `Manager` | `service_manager.go:1040`, `:1054` | required (surface audit) |
| S12 | `"component-manager"` by name | 9 sites in `service_manager.go` | optional |
| S13 | direct `PrometheusRegistry().Register` | `startup_metrics.go:50` | optional; retained |
| S14 | context rules otherwise clean | `graph_triples_http.go:147`, `:225`, `:239` request-scoped | none |
| § 2.4 | renamed names | `service_manager.go:1752`, `:185`, `:191`; `startup_metrics.go:43`; `dependencies.go:34`, `:36` | required (build) |
| § 5 (a) | exported, no reader | `service_manager.go:354`, `:367`, `:379`, `:970`, `:988`, `:1006`, `:1019`, `:1040`, `:1054`, `:1355`; `registry.go:53` | required (audit) |

## 2. Findings added by this sweep

Each is `path:line` — the line's text at the pin, then the shape and the proposed disposition.

- **P1, shape 4 (dead field).** `service_manager.go:91` — `isHTTPManager bool`. Set only by the pin's test helper
  (`service_manager_test.go:610`), read by nothing (`grep -w isHTTPManager`: one production line, the declaration).
  Required (surface audit: truly dead): dropped.
- **P2, shape 4 (test-only code).** `service_manager.go:1035` — `func (m *Manager) hasNATSAccess() bool {`, called
  only by `service_manager_test.go:854-870`; `:1243` — `func (m *Manager) completeHTTPSetup(ctx context.Context)
  error {`, called only by tests; `:1404` — `func (m *Manager) stopHTTPRuntime(ctx context.Context) error {`, called
  by nothing. Required (truly dead, or unexported with a test as its only caller, #186): dropped.
- **P3, shapes 2 and 4 (copied shell, promoted surface with no reader).** `service_manager.go:32` —
  `*BaseService // Embed BaseService to implement Service interface`; `:138` —
  `m.BaseService = NewBaseServiceWithOptions("service-manager-registry", nil)`. Nothing starts this `BaseService`
  (`StartAll` never calls it; only `Start` at `:1045`, S11, does) and nothing reads the `Manager` as a `Service`:
  pin-wide and in the four consumers the only method called on `Dependencies.ServiceManager` is `GetService`
  (`heartbeat.go:147`), and `internal/boot` names the manager through `StartAll`, `StartHealthListener`, `StopAll`
  (`internal/boot/run.go:533-537`). The promoted `Name`, `Status`, `IsHealthy`, `Health`, `GetStatus`,
  `SetHealthCheck`, `Start`, `Stop` are exported surface with no reader. Required (surface audit): the embedding goes
  with S11, and the `Manager` keeps a logger field.
- **P4, defect (the injected logger is ignored).** `service_manager.go:171-175` — `logger := slog.Default()` / `if deps
  != nil && deps.Logger != nil {` / `logger = deps.Logger`: a local, never stored. `StartAll`, `StopAll` and the HTTP
  handlers log through the embedded service's default logger (`:138`, `:398-402` — "Use the injected logger from
  BaseService if available"), so a composition root's logger never reaches them; the probe (§ 3) shows `StartAll`
  logging at INFO with `service=service-manager-registry` though the dependencies carried an Error-level logger.
  Required (behaviour breaks the comment): `ConfigureFromServices` keeps `deps.Logger`; fixed with P3.
- **P5, shape 4 (dead fields).** `metrics.go:27` — `natsClient     *natsclient.Client      // For JetStream metrics
  publishing`, never set or read; `:31` — `running        bool`, written at `:183` and `:254`, never read. Required:
  dropped.
- **P6, shape 1 (two loggers).** `metrics.go:145` — `slog.Info("Starting metrics server", ...)`, also `:192-196`,
  `:259`, `:281`, on the process-global logger while the service's `BaseService` holds the injected one (`:101-106`).
  Optional; a simple fix the owner's 2026-10-19 intent favours.
- **P7, shape 4 (no reader).** `metrics.go:336` — `func (m *Metrics) Port() int {`, `:341` `Path`, `:346` `URL`,
  `:356` — `func (m *Metrics) ConfigSchema() ConfigSchema {`. `gopls references` at the pin returns only each
  declaration (tests included); no consumer calls them (`grep` of the four tarballs); `ConfigSchema`'s one reader was
  the `Configurable` interface PR 1 dropped (PR 1 design D1). This settles the inventory's "not settled by grep".
  Required: dropped. `intPtr` (`:380`) stays for `message_logger.go` (PR 4).
- **P8, shape 4 (no reader after § 5 (a)).** `registry.go:65` — `func (r *Registry) Constructors()
  map[string]Constructor {`: its only reader is `ListConstructors` (`service_manager.go:372`; `gopls references`),
  which § 5 (a) drops. Required: dropped with it.
- **P9, rule (sort before limit).** `graph_triples_http.go:186` — `keys, err := graphTripleKeys(ctx, reader)` then
  `:193-195` — `if len(results) >= params.limit {` / `break`: the limit is applied while scanning keys in the order
  the bucket lists them. Developer contract § Semantic identity and graph contracts: "Sort and deduplicate a complete
  result set before applying a limit" (`.agents/contracts/semengine-developer.md:150`). Required.
- **P10, rule (validate where it enters).** `graph_triples_http.go:61-62` — `subject:   q.Get("subject"),` /
  `predicate: q.Get("predicate"),`: a literal entity ID and a predicate enter unvalidated (developer contract
  `:141-146`: `vocabulary.ParsePredicate`, `types.ValidateEntityID`). Required.
- **P11, defect (skips more than it says).** `graph_triples_http.go:198-203` — `entry, err := graphTripleEntry(ctx,
  reader, key)` / `if err != nil {` / `// Key may have been deleted between Keys() and Get(); skip gracefully.` /
  `... continue`: every `Get` error, a timeout included, is skipped with a Debug line and the answer is a partial
  200, although `graphTripleEntry` already tells a missing key apart (`:244-246`). Required (behaviour breaks the
  comment; "failure paths fail closed").
- **P12, declared degrade.** `graph_triples_http.go:175-179` — `if m.natsClient == nil {` / `// NATS not configured —
  return empty result, not an error.` / `return []message.Triple{}, nil`. No deployment reaches it (a composition
  root always passes `Dependencies.NATSClient`), so under #185 it is declared in the README, not changed. Optional.
- **P13, documented, not implemented (§ 5 (c) class).** `middleware.go:20-23` and `service_manager.go:1073-1076` name
  `agenticdispatch.WithIdentity` and `IdentityFromRequest`, not in this tree; `service_manager.go:55-56` —
  `// cmd/semstreams's -health-port (or SEMSTREAMS_HEALTH_PORT env) to`, a flag and variable not in this tree;
  `service_manager.go:1594` — `<title>SemStreams API Documentation</title>`. Required: rewritten.
- **P14, shape 1 (one fact in four places).** The component manager is mandatory at `service_configs.go:83` (added by
  default), `service_manager.go:165-169` (`MandatoryServiceDisabledError`), `:385-387` with `:804-833`
  (`mandatoryServices`, created at `StartAll`) and `:517-519` ("mandatory service component-manager was not
  constructed"). Optional: retained (Bounded port scope); the refusals agree today.
- **P15, shape 1 (two parsers of one value).** `service_manager_config.go:43-52` — `Validate` parses
  `http_read_timeout` and `http_write_timeout`; `:60-82` — `ResolvedHTTPReadTimeout` parses again and falls back to
  the default on a value `Validate` already refused. Optional: retained.
- **P16, placement (no caller in this PR).** `service_manager.go:1490` — `func attributeShutdownError(owner string,
  phase errs.ShutdownPhase, err error) error {` and `startup_metrics.go:82` — `func (w *startupMetricWriter)
  publishComponents() {`: their callers are in `component_manager.go` (PR 3); `dependencies.go:20` — `type
  natsPublisher interface {`: its reader is `metrics_forwarder.go:74` (PR 6). Each lands with its caller.
- **P17, dead tests.** `service_manager_test.go:637-911`: five `ConfigWatcher` tests drive the dropped
  `Manager.Start`/`Stop` (S11) on fixed ports `8081`-`8084` and assert nothing about a config watcher ("We cannot
  directly test configUpdates channel as it's not accessible", `:663`); `:790-852` only logs; `:882-911` tests
  `strings.ReplaceAll`; `:854-880` tests P2's `hasNATSAccess`. Not carried (no independent oracle, `docs/testing.md`).
- **P18, retained cost.** `service_manager.go:1595-1599`: `/docs` loads the Swagger UI from `unpkg.com`, so it needs
  network access; `/openapi.json` does not. Served as at the pin (03B operator-surface row); declared in the README.

Searched and empty in these nine files: shape 5 (`grep -n 'err.Error()\|strings.Contains'`: 0 hits); new roots
(`grep -n 'context.Background\|context.TODO\|WithoutCancel\|select {}'`: 0 hits); `GenericJSON`: 0 hits.

## 3. Pin probe for the two owners (inventory § 7, "Not run")

Scratch copy of the pin, go1.26.4 darwin/arm64, about 10 minutes of wall clock; source, command and output in
`probe-note.md`. This repository's `internal/harness/lifecycletest` copied into the scratch pin, with test-side
adapters (`Start` = `StartAll`, `Stop` = `StopAll` for the `Manager`), run under `-race`:

- **`Manager`:** 7 of 8 checks pass. `StartContextEndStopsNothing` fails: "after the Start context ended the owner
  stopped holding [health-publisher child-service]" (the main listener and the claimed metrics server stay). The
  adapter's `mustFail` is a `Manager` with no `component-manager`, which `StartAll` refuses.
- **`Metrics`** (standalone, on a test listener): 7 of 8 pass. `StartContextEndStopsNothing` fails: "stopped holding
  [base-health]"; the server stays.
- **S2:** after the `StartAll` context is cancelled, `GET /healthz` still answers 200 and the request's context is
  already cancelled (`requestCtxErr=context canceled`; before the cancel, `<nil>`).
- **Default composition with only the metrics constructor:** `ConfigureFromServices` returns "create configured
  service component-manager: no constructor registered for service component-manager"; `StartAll` then returns
  "create mandatory services: failed to create mandatory service component-manager: no constructor registered for
  service component-manager".
- The unit runs (once under `-race`, five times shuffled) and the test-text guards are the inventory's § 7, run on
  the whole package; their hits in this PR's tests are `service_manager_prop_test.go:168` (`t.Skip`, inside a `rapid`
  action) and `service_manager_stopall_test.go:121` (`require.Eventually`, in a test that is not carried).
