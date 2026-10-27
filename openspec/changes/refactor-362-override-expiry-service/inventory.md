# Inventory: refactor-362-override-expiry-placement

base: 0e2f17c (main); claim branch claude/expiry-reporter-362 at 8540948 is an empty claim commit
(`git diff --stat origin/main...HEAD`: empty). Claim: draft PR #421 (0 files).

Port-refactor #362 of `service`, carried from the SemStreams pin `8b99efe9`. No package is carried and no statement
below is about pin behaviour, so no pin probe runs. The pin location `service/component_manager.go:77-88` is the
issue's citation, not re-measured.

## 1. The claimed gap: what the reporter is, how it runs, what it reads

- `service/stream_override_expiry.go:48` — `type streamOverrideExpiryReporter struct {` (139 lines; unexported:
  type, constructor `:60`, `register` `:82`, `run` `:95`, `evaluate` `:110`). Fields: `configOf func() *config.Config`,
  `logger`, `expired *prometheus.GaugeVec`.
- Gauge: `:66-74` — namespace `semengine`, subsystem `streams`, name `migration_override_expired`, labels
  `stream`,`owner`. Registered `:83` — `metric.RegisterOrGet(registry, "streams", "migration_override_expired", r.expired)`
  and the returned collector kept (`:87`).
- Interval: `:39` — `const overrideExpiryCheckInterval = time.Minute`. `run` `:95-108`: evaluate at once, then per tick,
  returns on `ctx.Done()`.
- `evaluate` `:110-139`: `cfg := r.configOf()`; nil returns; `r.expired.Reset()` (`:119`); 0 for every declared
  override (`:120-122`); 1 plus a Warn line per entry of `config.ExpiredMigrationOverrides(cfg, now)` (`:124-137`).
- Reads: only the configuration's `StreamMigrationOverrides` through `config.ExpiredMigrationOverrides`
  (`config/stream_bounds.go:671`). It reads no NATS stream and holds no client.
- Hosting, construction: `service/component_manager.go:79` — `overrideExpiry *streamOverrideExpiryReporter`
  (comment `:73-78`: "hosted here, not by an optional service, because the component manager is the one service no
  composition can disable (MandatoryServiceDisabledError)"). Built `:239-242` with `currentConfig.Get` (the
  `*config.SafeConfig` from `deps.Manager.GetConfig()`, `:197`) and logger source
  `component-manager.stream-overrides`; registered only when `deps.MetricsRegistry != nil` (`:245-250`); evaluated once
  at construction `:253` — `cm.overrideExpiry.evaluate(time.Now())`.
- Hosting, run and stop: `service/component_manager_lifecycle.go:219` — `loops.Go(func() { cm.overrideExpiry.run(ctx) })`
  inside `supervise` (`:213-225`), under the manager's one Start-created run context (fields
  `supervisorCancel`/`supervisorDone`, `component_manager.go:89-95`); `supervise` closes `done` only after both loops
  return (`:224` `loops.Wait()`), and Stop waits on `done`.

### "Always reported, even when empty": the mechanism

- "Always": the host is always composed. `service/service_manager.go:155-158` — the loop over
  `"service-manager", componentManagerName` returning `&MandatoryServiceDisabledError{Name: name}` when not enabled;
  `service/service_configs.go:86` — `materializeServiceConfig(resolved, "component-manager", json.RawMessage(`{}`))`
  (present with `{}` when the document names none); a registry without the constructor is refused,
  `service/service_manager.go:200-203` — `no constructor registered for service %s` (spec
  `service-lifecycle/spec.md:244-255`, `TestConfigureRefusesACompositionWithoutAComponentManager`;
  `service/component_manager_integration_test.go:103` `TestServiceManagerMandatoryService`).
- Test gap: no test drives `MandatoryServiceDisabledError` (`git grep -n MandatoryServiceDisabledError -- '*_test.go'`:
  empty) and no spec requirement names it (`git grep -n -i 'cannot be disabled\|MandatoryServiceDisabled' openspec/specs`:
  empty).
- "Even when empty": the gauge is registered at construction whatever is declared (`component_manager.go:245-250`), and
  every declared bridge gets a 0 series at construction (`stream_override_expiry.go:120-122`, `component_manager.go:253`),
  so an alert can be tested before a lapse. With no override declared, the family is registered with no series.
  Measured by the inventory reviewer (client_golang v1.25.0): a registered `GaugeVec` with no children gathers as 0
  families, so `/metrics` shows nothing for the gauge. "Even when empty" therefore means *registered at construction*
  (the name is claimed and a collision refuses construction, `component_manager.go:245-250`), not *visible*. A series
  appears only for a declared bridge.

## 2. Every current spelling of the fact

- Boot-time expiry refusal: `config/stream_bounds.go:554-566` (`parseOverrideExpiry`, `ErrStreamMigrationOverrideExpired`).
- Runtime evaluation: `config/stream_bounds.go:671` `ExpiredMigrationOverrides` (only production reader:
  `service/stream_override_expiry.go:124`).
- Provisioning summary: `config/streams.go:345` — `"migration_overrides", len(exceptions.MigrationOverrides),` (log).
- Seam text: `natsclient/stream_bounds.go:6`, `:113` (prose only).
- A second runtime spelling: every `SafeConfig.Mutate` re-validates the whole draft (`config/config.go:130-143`,
  `draft.Validate()` at `:138`) → `ValidateStreamDeclarations` (`config/config.go:270`) →
  `planStreams(cfg, time.Now(), nil)` (`config/stream_bounds.go:609-611`), which refuses a lapsed override
  (`:561-566`). So on a running process, once a bridge lapses, the expiry fact is also evaluated, as a refusal, on every
  config-manager update (F5, #423).
- One reporter; the evaluator `ExpiredMigrationOverrides` has the one production reader above.

### Findings on today's tree (defects to file, or fix in the move)

- F1. Overrides cannot change while the process runs. The config manager watches only
  `services.*`, `components.*`, `platform`, `nats`, `model_registry` (`config/manager.go:166-170`), and
  `applyConfigKey` has no case for `stream_migration_overrides` (`config/manager.go:741-813`; `default:` returns
  `errNoConfigChange` `:811-813`). Other `Mutate` callers (`:378`, `:1434`, `:1501`) do not set overrides. So the
  reporter's "an operator may extend or remove one without restarting" (`stream_override_expiry.go:44-47`,
  `component_manager.go:234-238`), the `run` comment's "an operator edits one post-boot"
  (`stream_override_expiry.go:91-94`) and spec "live configuration" (`service-lifecycle/spec.md:524`) describe no
  production path. `TestOverrideExpiry_ClearsWhenTheBridgeIsRenewed` (`stream_override_expiry_test.go:93`) and
  `TestOverrideExpiry_RemovedOverrideStopsReporting` (`:114`) mutate the test's own `*config.Config` pointer. The tick is
  still needed: time passes (`TestOverrideExpiry_CrossesTheDeadlineWithoutRestart` `:56`).
- F2. The boot exception report is called "readiness" throughout, though readiness never reads it:
  `config/stream_bounds.go:133-134` ("as readiness reports it"), `:139` — `Expires   time.Time     // when readiness
  starts failing`, `:155` ("readiness's account"), `:164-165` ("a scheduled future readiness failure"), `:601-603`
  ("returning readiness's account"). These contradict "Readiness SHALL not depend on it"
  (`service-lifecycle/spec.md:525`).
- F3. `service/component_manager.go:234` cites ADR-100 for the reporter; `docs/adr/100-*.md` has no override, expiry or
  report-only text (`grep -n -i 'expir\|override'`: one unrelated hit at `:26`).
- F4 (narrow). The scenario at `operator-surface/spec.md:720` checks `semengine_storage_*` names, and
  `TestStorageAlertRules_ReferenceOnlyPublishedMetrics` matches exactly that (`semengine_storage_[a-z_]+`,
  `service/storage_observability_http_test.go:530`). Only its parenthetical, "the migration alert reads
  `semengine_streams_migration_override_expired`, which the component manager publishes", is unproven by that test;
  the name is spelled in a test only at `service/stream_override_expiry_test.go:29`. A move changes "which the
  component manager publishes" in any case.
- F5 (#423, filed by the inventory reviewer, `status:blocked`). Once a bridge lapses on a running process, every
  config-manager update fails validation: the watch path `applyRevision` → `updateConfig` (`config/manager.go:911`)
  logs and drops the change, and `DeleteComponentFromKV` fails at `config/manager.go:894`, after the KV delete, so the
  bucket and memory diverge. The reporter's report-only split ("enforcement stays at boot",
  `stream_override_expiry.go:10-16`) is therefore not what the process does: a lapse also refuses runtime
  configuration writes. Touches `config/`, not the reporter; it constrains option C below.

## 3. Adjacent claims

- Specs: `openspec/specs/service-lifecycle/spec.md:520-538` "A lapsed stream migration override is reported while
  running" (names the component manager; two scenarios); `:244-255` "Every composition has a component manager";
  `openspec/specs/operator-surface/spec.md:720`; `openspec/specs/composition-validation/spec.md:32` (the `now` argument,
  not the reporter).
- Docs: `service/README.md:313-317`; `service/doc.go:274-279`; `config/README.md:311`.
- Reader of the gauge in-tree: `configs/prometheus/rules/storage-pressure.yml:305-306` — alert
  `StorageStreamMigrationOverrideExpired`, `expr: semengine_streams_migration_override_expired == 1`, `for: 1h`. Also
  forwarded: the metrics forwarder gathers the whole registry (`service/metrics_forwarder.go:157`). Outside readers
  (operators' Prometheus) are not measurable.
- Tests: reporter-only `stream_override_expiry_test.go:56,81,93,114`; composed through `NewComponentManager`
  `:173` (registered at construction), `:217` (Start runs it, synctest), `:282` (supervise holds done), `:316` (Stop
  waits), `:350` (failed Start launches none); `component_manager_supervisor_test.go:69`
  `TestOverrideGaugeUsesTheRegisteredCollector`; config side `config/stream_bounds_test.go:977`.
- Ledger: `docs/admission-ledger.yaml:2840` `source_path: service` (PR 3 text `:2850` onward; the reporter is not named
  in `contract`; ADR-098 Decisions 3-4 are cited for PR 5).
- Archived design: `openspec/changes/archive/2026-10-22-setup-04a-03-boot-service-component-manager/design.md:281-302`
  (D4), `:99` (`stream_override_expiry.go` adapt row), `:494-495` (#362 stays; D4 keeps it).
- Owner ruling: #362 comment 2026-10-24 (own PR after #364 and #363; leans toward its own small service; architect
  pass first).
- Open issues (queries `override`, `expiry`, `migration override`, `component manager`, `metrics service`,
  `config manager`, `supervisor`, `stream_override_expiry`, `overrideExpiry`, `stream_bounds`): kept #362. Names the word
  only: #15, #9, #280, #196, #361, #195, #7, #420, #216, #68, #109, #350. #196 (lifecycleguard adoption) and #361
  (semsource StartAll WithoutCancel) touch service lifecycle generally, not these files.
- Open issues added after the first pass: #423 (F5; `config/`, filed by the inventory reviewer).
- Open PRs: #421 (this claim, 0 files); #414 (draft, 31 files: `graph/`, `processor/`, `composition/reserved_test.go`,
  its own openspec change; none under `service/` or `config/`, no `service-lifecycle`/`operator-surface` delta). No
  overlap.

## 4. Who it is for

No new surface is introduced in the inventory phase. The reporter is unexported today; its framework-visible surface is
the gauge name and labels, the Warn line, and the example alert rule.

## 5. The problem shape

"A report-only interval loop over configuration, run as substrate observability." Closest instances:

- `service/heartbeat.go` (199 lines): a small service with an interval loop, its own run context in `start`
  (`:127` — `runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))`), joined in `cleanup` (`:183-194`),
  composed by default through `config/config.go:491-494` (Loader defaults).
- `service/storage_observability.go:1-45`: substrate observability as a service, report-only (ADR-098), opt-in, no
  consumer enables it (ledger `:2861-2864`).
- Default-on spellings: `service/service_configs.go:86-90` (materialized: component-manager, service-manager, metrics;
  metrics keeps an explicit `false`) and `config/config.go:491` (Loader default for heartbeat). Two mechanisms exist.
- Other loops on mandatory hosts: the service manager's health publisher (`service/service_manager.go:565-572`) and its
  startup collector on `deps.MetricsRegistry` (`:489-509`).

## 6. Decision records

No package carried from the pin. Records governing the behaviour: ADR-098 (`docs/adr/098-observability-splits-by-subject.md:12-13`,
Decision 3 `:80-81`: substrate facts leave as logs and metrics; the framework enforces nothing); ADR-113 (Start's
context bounds Start; one run context made in Start, ended in Stop). ADR-100 is cited in code but does not govern the
reporter (F3).

## Collision table (runtime-coordination primitive: an interval loop)

| Dimension | Evidence |
| --- | --- |
| Semantic class | report a lapsed bridge while running; enforcement at boot (`stream_bounds.go:554-566`) and, unintended, on every `Mutate` while running (`config.go:138` → `stream_bounds.go:609-611`; F5, #423) |
| Owners | component manager hosts (`component_manager.go:79`); `config` owns the evaluator (`stream_bounds.go:671`) |
| Catalogs | the configuration document's `stream_migration_overrides` (`config/config.go:60`) |
| Status | the gauge; Warn line; readiness unaffected (`service-lifecycle/spec.md:525`) |
| Lifecycle | construction evaluation; Start-to-Stop loop; Stop joins (`component_manager_lifecycle.go:213-225`) |
| Ownership | per process; two managers on one registry share one gauge (`RegisterOrGet`) |
| Readers | example alert rule; metrics forwarder; tests above |
| Writers | the gauge: the reporter only. The refusal: `SafeConfig.Mutate` validation (F5) |
| Recovery | none needed; reset each tick (`:119`) |

## Adopter seam inventory

- Operators: today they declare nothing; the reporter runs whenever overrides exist. A move that keeps that must be
  default-composed; a move to an opt-in host makes the default path silent (the alert never fires, rank "nowhere").
- Consumer composition roots (semsource, semteams, semboids call `RegisterAll`; ledger `:2850-2851`): a new constructor
  registered by `RegisterAll` reaches them with no edit; a constructor registered by hand would hit
  `service_manager.go:200-203` at boot (loud). A new `config.Manager` option would need every root to pass a registry,
  otherwise no gauge (the Warn line still logs; the gauge and its alert go silent).
- Alert authors: the gauge name and labels must not change (issue text).

## Options (pre-review framing; for the design review)

How each option "fires with no operator config" depends on the mechanism it uses. There are three:
(M1) the mandatory list, `service/service_manager.go:155-158`: cannot be disabled; an explicit `enabled: false` is a boot
error (`MandatoryServiceDisabledError`). (M2) materialized by default, `service/service_configs.go:86-90`: filled in
when the document does not name the service, but an explicit `enabled: false` is kept and silences it with no error
(the metrics shape, `:88-89`). (M3) a Loader default, `config/config.go:491-494` (the heartbeat shape): covers only
documents loaded through `config.Loader`. Whether a document that names a `services` block keeps the Loader default
is not measured.

| Option | What moves | Fires with no operator config? | Lifecycle owner (ADR-113) | Exported-surface change | Rough size | Risks |
|---|---|---|---|---|---|---|
| Today: component manager | nothing | yes: M1 plus `{}` filled in (`service_configs.go:86`) | component manager's run context (`component_manager_lifecycle.go:213-225`) | none | 0 | the issue's complaint: hosted for convenience |
| A. Own small service | reporter plus a heartbeat-shaped shell (`heartbeat.go:120-135`, `:183-194`); one line in `RegisterAll` (`register.go:20-28`); one line in M1 or M2; about 35 lines leave the component manager (`component_manager.go:73-79`, `:234-253`, `component_manager_lifecycle.go:219-224`) | with M1: yes, cannot be disabled (one more mandatory name). With M2: yes, but `enabled: false` silences it with no error. M3 not recommended (Loader-only) | the service: run context made in Start, ended in Stop | none needed: `RegisterAll` is in-package, so the constructor can stay unexported; exporting it (as other services do) is a choice | about 80-120 new lines (unmeasured estimate); 5 composed tests rewritten; 4 reporter tests unchanged; `service-lifecycle` requirement rewritten, `:244-255` extended if M1; README, doc.go, operator-surface `:720` | a root that registers constructors without `RegisterAll` gets a loud boot refusal (`service_manager.go:200-203`); all three consumers call `RegisterAll` (ledger `:2850-2851`). M1 widens the mandatory list, which has no test of its own today |
| B. Metrics service | loop into `Metrics` (`metrics.go:113-125`), which gains a run context | yes via M2, but `metrics: {enabled: false}` drops the gauge and the Warn line, while the metrics forwarder could still have sent the gauge (`metrics_forwarder.go:157`) | `Metrics` | none | small (unmeasured) | the same convenience shape; ties a config reader to the HTTP exposition service |
| C. Config manager | reporter into `config`, beside the evaluator | no: no `deps.MetricsRegistry` reaches `config.Manager` (`NewConfigManager(cfg, natsClient, logger, opts...)`, `config/manager.go:114`); a new exported `ManagerOption` is needed and every consumer root must pass a registry, otherwise no gauge (Warn line only) | `config.Manager`'s existing run context (`config/manager.go:72`) | new exported option in `config`; new import `config` → `metric` (no cycle: `go list -deps ./metric` has no `config`) | small code plus an edit in every consumer | refutes the issue's premise for this option: by F1 the config manager does not hold *live* overrides, and by F5 it already refuses on a lapse |
| D. Service manager | loop beside its health publisher (`service_manager.go:565-572`), registry already in hand (`:489-509`) | yes: it is M1 | the service manager's run context | none | small | repeats the complaint on the other mandatory service |
| E. Storage-observability service | loop into the report-only substrate service (ADR-098) | no: opt-in, not M1/M2/M3; no consumer enables it (ledger `:2861-2864`) | that service | none | small | silent by default |
| F. Do nothing | comment fixes only (F1, F3) | yes, as today | component manager | none | trivial | #362 stays undone |

Recommendation: A. The deciding reason: it is the only option that gives the reporter a lifecycle owner of its own
and still fires with no operator config, no consumer edit and no new exported Go surface. Its service name is new
operator-facing surface: a key operators can write under `services.<name>`, which under M2 turns it off and under M1
can make boot fail. Whether it may be turned off is the mechanism choice M1 or M2, which is the owner's; the name
rides that question.

Owner questions:

1. M1 (cannot be disabled, as the component manager is today) or M2 (on by default, `enabled: false` silences it
   quietly, as metrics)? Only M1 keeps today's guarantee.
2. F1: correct the "live, without restart" wording and the two renewal tests in this PR (keep the per-tick reset,
   which is harmless), or file it on its own?
3. Exported constructor, as the other built-in services, or unexported?
4. F5 (#423) is in `config/` and stays its own fix; the files do not overlap, so either PR may merge first. Once
   #423 lands, the comment "enforcement stays at boot" becomes true again; until then the move should not repeat it
   as fact.
