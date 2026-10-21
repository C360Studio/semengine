# Shape sweep: setup-04a-03-boot-service-component-manager (service PR 3 of 7)

base: `d39fae37f580a3eae989173c8a8eed1256ca9d99` (`main` `7c4dd2c` plus the claim commit; draft PR #359)

The per-PR read the architect contract asks for (§ The shape sweep), of the five files this pull request carries, on
a scratch copy of the SemStreams pin `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` (`gh api .../tarball/8b99efe9`, local
only). It re-uses the whole-package inventory, `INVENTORY PASS` round 2 at `ad946b7`
(`openspec/changes/archive/2026-10-20-setup-04a-03-boot-service-base/inventory.md`, "the inventory" below), wherever
that already pins a line, and adds what it did not pin. Disposition is proposed here and settled in `design.md`.

Files (non-test pin lines, 2,487 in all): `component_manager.go` 1,526, `component_manager_http.go` 673,
`rule_pack_bind.go` 132, `stream_override_expiry.go` 130, `component_manager_config.go` 26.

## 1. Findings the inventory already pins (cited, not repeated)

| Id | Shape | Pin lines in these files | Disposition |
| --- | --- | --- | --- |
| S1 | run context from the Start context | `component_manager.go:464`, `:622` | required (ADR-113; probe § 3); design D2 |
| S3 | copied lifecycle state machine | `component_manager.go:98-110`, `componentRuntime` `:121-131` | required (#196; ADR-108 Decision 2); D2 |
| S8 | health publisher drops failures | `component_manager.go:1476-1489`, `:1518-1520`, `:1524` | required in part; D4 |
| S12 | `"component-manager"` by name | `component_manager_http.go:20`, `component_manager.go:192`, `:896`, `:937`, `rule_pack_bind.go:32` | optional; the tree's `componentManagerName` (`service/service_manager.go:28`) is used |
| S14 | context rules otherwise clean | re-checked below | none |
| § 2.4 | renamed names | `component_manager.go:47`, `:1216`, `:1218`; `:290`, `:384`, `:397`, `:1107`; `:400`, `:697`; `:1210`; `stream_override_expiry.go:67`, `:79`; `rule_pack_bind.go:16` | required (build; #69 for `:67`; owner Q2 for `rule_pack_bind.go:16`) |
| § 4 | where the manager receives, keeps and derives a context | `component_manager.go:463-464`, `:622`, `:636`, `:699`, `:1033-1042` | required (ADR-113); D2, owner question Q-A |
| § 5 (a) | exported, no reader | `component_manager.go:1139`, `:1144`, `:1333`, `:1345`; `component_manager_config.go:14`; unexported `:1200` | required (surface audit): dropped |
| § 5 (b) | config read by no behaviour | `component_manager_config.go:10` (`EnabledComponents`) | required (surface audit): dropped |
| § 5 (c) | documented, not implemented | `component_manager_config.go:10` ("empty=all") | required: goes with the field |

## 2. Findings added by this sweep

Each is `path:line` — the line's text at the pin, then the shape and the proposed disposition.

- **C1, shape 4 (exported, read only by its own constructor; test-only branches).** `component_manager.go:276` —
  `func (cm *ComponentManager) Initialize() error {`, called only at `:267` (pin-wide outside `service`: 0 typed uses;
  the four consumers: 0). Its re-entry guard `:280-283` and the flag behind it (`:94-95` — `initialized atomic.Bool`
  / `initMu sync.Mutex`) matter only for a manager built as a struct literal, which only tests build (they write
  `cm.initialized.Store(true)`); so do the refusals `:455-458` ("component manager not initialized") and `:1152-1154`.
  Optional; recommended in D1 (simple, owner 2026-10-19 intent): `initialize` unexported, the flag and both refusals
  dropped, tests built through the constructor (D7).
- **C2, nil fallbacks for struct-literal managers.** `component_manager.go:208-211` — `// Fallback to creating a new
  registry if not provided` / `registry = component.NewRegistry()`: a composition that passes no
  `ComponentRegistry` gets a private empty registry, so every configured component fails to construct (`createComponent`
  finds no factory) and C3 then boots without them. `:541-543` — `if cm.overrideExpiry != nil {` ("a struct-literal
  manager (unit tests) has no reporter"); `:1231`, `:1303` — `if cm.storeRegistry == nil {`. The constructor always
  sets the reporter and the store registry (`:225`, `:243`). All three consumers pass a registry (semsource
  `cmd/semsource/run.go:383`, semteams `cmd/semteams/main.go:789`, semboids `cmd/semboids/main.go:188`). Optional;
  recommended in D3: refuse a nil registry as `:155-157` refuses a nil config manager, and drop the test-only nil
  branches.
- **C3, a failure path that fails open.** `component_manager.go:328-346` — `cm.logger.Error("Failed to create component
  from config", ...)` then `continue`, with `:334-338` — "Generic components retain the established best-effort cold
  boot posture." An enabled component that fails to construct is left out with one Error line; the process boots, and
  `/readyz` (which reads only admitted components, ADR-109) says `READY`. Only `rule-processor` instances fail closed
  (`:339-344`, `:356-372`). `AGENTS.md` § 1: "Failure paths fail closed; any skip, drop, or degrade is declared." It is
  declared (a log line), not closed. A change to what boot accepts is the owner's call: owner question Q-B.
- **C4, start order within a phase follows map iteration.** `component_manager.go:570-573` — `for name := range
  cm.components {` / `names = append(names, name)`, unsorted, and `:644-645` records `startOrder` in that order, which
  `stopAllComponents` reverses (`:959-964`). Providers still stop after consumers; within a phase the stop order differs
  from run to run. Optional; recommended in D3 (sort the names: one line, and the stop order becomes a stated law the
  model check of § Generated checks can hold).
- **C5, Stop during Start cancels every Start in flight.** `component_manager.go:785-798` — `if startDone != nil {` /
  `cancel := cm.cancel` / `...` / `cancel()`, then waits for `startDone`; the store guard `:1238-1247` ("skip if
  terminal Stop halted this component between Start and registration") exists for that race. Under
  `internal/lifecycleguard` a Stop during Start waits for Start (`go doc ./internal/lifecycleguard Guard.Stop`). Settled
  with Q-A (D2); the store guard is then unreachable and goes with its only test
  (`component_manager_storeregistry_test.go:96`).
- **C6, a failed running Stop is final.** `component_manager.go:807` — `retryable := cm.cleanupPending` (false once
  Start committed), `:819-821` sets `lifecycleTerminal` whatever `stopErr` was, and `:1055-1056` marks the component
  terminal after a failed `Stop`. So after a running Stop fails or runs out of time, a later Stop returns nil and calls
  nothing (pin ADR-095 `095:30`, "A deadline is a failed exit result"). Required: ADR-113 Decision
  (`docs/adr/113-start-context-bounds-start.md:61-62`, "if `ctx` ends first it returns that error and a later `Stop`
  tries again"), as PR 2 recorded for ADR-095 (archived design `2026-10-21-setup-04a-03-boot-service-manager`, Decision
  records). D2.
- **C7, shape 1 (a third copy of the contract clone).** `rule_pack_bind.go:117-132` — `func
  cloneRulePackContracts(contracts []projection.Contract) []projection.Contract {`, the same copy as the method the tree
  consolidated (which differs only in keeping a nil `Groups` nil where the pin makes an empty slice),
  `pkg/projection/contract/contract.go:75` — `func (c Contract) Clone() Contract {` (#138 item 2, closed). Optional
  (inherited); recommended in D6: `contract.Clone()` per element.
- **C8, a fifth concrete component-manager check and an unlocked map read.** `rule_pack_bind.go:32` — `cmService,
  exists := m.services["component-manager"]` outside `m.mu`, and `:36` — `cm, ok := cmService.(*ComponentManager)`.
  PR 2 turned the other four into `componentStartup` (archived design D3). The only consumer caller runs at composition
  time on one goroutine (semteams `cmd/semteams/rule_config_service.go:88-96`), so the race has no real-world trigger
  (#185: MEDIUM at most). Optional; recommended in D6: `GetService(componentManagerName)` and a `componentStartup`
  assertion.
- **C9, an import registers something.** `component_manager_http.go:19-21` — `func init() {` /
  `RegisterOpenAPISpec("component-manager", componentManagerOpenAPISpec())`. The tree's contract line forbids it:
  `service/openapi_registry.go:11-13` — "Call it from the composition root, never from an init function: importing a
  package registers nothing" (and `AGENTS.md` § 1, "imports register nothing"). It is also a second spelling of the spec
  `OpenAPISpec()` returns (`:93-95`). Required; D5 frames the fix.
- **C10, a read lock held across a network write.** `component_manager_http.go:532-554` — `cm.mu.RLock()` / `defer
  cm.mu.RUnlock()` around `json.NewEncoder(w).Encode(config)`. A client that stops reading holds the lock that
  `updateComponentState` (`:1074-1075`, called during Stop) needs. The trigger needs a response larger than the
  socket buffers, which no named configuration reaches (#185: MEDIUM at most). Optional; recommended in D5: copy under
  the lock, encode through `writeJSON` (`:602-613`) after it.
- **C11, health that has not looked says healthy.** `component_manager.go:1173-1177` — `if !cm.mu.TryRLock() {` /
  `// Under write contention - assume healthy to avoid blocking.` / `return nil` (the reason, gh#508 at the pin, is a
  real deadlock); and the same shape at `:1156-1159` — `if !cm.started.Load() {` / `return nil // Still starting up,
  assume healthy`. A positive answer stands in for "unknown", and `Manager`'s readiness reads it through
  `svc.IsHealthy()` (`service/service_manager_http.go:507`). The trigger is Go lock timing (#185: MEDIUM at most).
  Retained as at the pin and filed under protocol "File" (#212): the issue text is in design D8, "To file".
- **C12, documented, not implemented (§ 5 (c) class).** `component_manager.go:1` — `// Package service provides service
  management and HTTP APIs for the SemStreams platform.` (a second package comment); `:31-38` (`Initialize()`, `Stop()`
  without a context); `:133` — `// ComponentManagerOption removed - we now use Dependencies pattern instead`;
  `component_manager_http.go:409` — `// Return flat array (matches OpenAPI contract in
  specs/008-fix-ui-code/contracts/component-types-api.yaml)` (no such path here). In tree, read by this port:
  `component/lifecycle.go:86-90` — `ManagedComponent.Config` "so a per-component runtime config update can restart the
  component ... (gh#520)", a path nothing implements (boot-only composition, ADR-094). Required: rewritten (D8).
- **C13, a route answers JSON-looking text.** `component_manager_http.go:439` — `http.Error(w,
  fmt.Sprintf(`{"error":"Component type %s not found"}`, componentType), http.StatusNotFound)`: served as `text/plain`
  with the request's segment in the body. Optional; retained as at the pin (a change to what a route answers is the
  owner's, PR #318), declared in the README. It breaks no promise: no spec requirement or contract line
  (`docs/contract.md:131-145`, the 03B operator rows) states that route's error body.
- **C14, a test starts an in-process server it never stops.** `stream_override_expiry_test.go:146-170` —
  `sharedOverrideExpiryNATSOnce.Do(func() {` ... `natsserver.NewServer(...)` ... `server.Start()`: a process-wide
  embedded `nats-server`, never shut down, its temporary store never removed. The tree keeps NATS in Docker through
  `natsfixture` (`docs/test-infrastructure.md`). Required (harness rules): not carried as written (D7).
- **C15, a test-only nil check on the startup writer stays.** `component_manager.go:756-761` —
  `recordComponentStartupMetrics` calls `writer.publishComponents()`, which is nil-safe (pin `startup_metrics.go:82-84`,
  `if w == nil {`) because the manager may start before the service manager hands it a writer (a component manager
  started on its own, as the lifecycle suite does). Not a finding: the nil path has a production caller.

Searched and empty in these five files: shape 5 (`grep -n 'err.Error()\|strings.Contains'`: one hit,
`component_manager_http.go:51`, which tests a decoded path segment, not an error); new roots and context values
(`grep -n 'context.Background\|context.TODO\|WithoutCancel\|select {}\|WithValue\|ctx.Value'`: 0 hits); `GenericJSON`:
0 hits; a `context.Context` field: none (the cancels are `component_manager.go:107` and `:123`, inventory § 4).

## 3. Pin probe for the component manager (inventory § 7, "Not run")

Two probes, 2026-10-21, each under a minute of wall clock. Both are local only; their source is on #359 (comment
6324262845, the suite probe's test file) and their output is below verbatim, for the orchestrating session to post.

**Suite probe.** Scratch copy of the pin, go1.26.4 darwin/arm64. The test file (`/tmp/pr3probe-src/zz_cm_probe_test.go`,
pasted on #359) is copied into the pin copy's `service/` beside the earlier probe's `internal/lifecycletest` copy. The
adapter builds a manager as the pin's own tests do (struct literal, `cm.initialized.Store(true)`) with one lifecycle
component whose work runs until its Start context ends (the carried contract); `Observe` reports the base service's
status and whether that work runs. Command, from the pin copy's root:
`go test -race -count=1 -run 'TestProbeComponent' -v ./service/ 2>&1 | grep -v '^=== '`. Output:

```text
    lifecycletest.go:124: after the Start context ended the owner stopped holding [base-status=running component-work]
2026/10/21 14:19:07 ERROR Component failed to start service=component-manager name=probe type=processor error="context deadline exceeded"
--- FAIL: TestProbeComponentManagerSuite (0.10s)
    --- PASS: TestProbeComponentManagerSuite/NilContextsRefused (0.00s)
    --- PASS: TestProbeComponentManagerSuite/PreCancelledStartRefused (0.00s)
    --- PASS: TestProbeComponentManagerSuite/StopBeforeStartSafe (0.00s)
    --- FAIL: TestProbeComponentManagerSuite/StartContextEndStopsNothing (0.10s)
    --- PASS: TestProbeComponentManagerSuite/StopReturnsItsEndedContext (0.00s)
    --- PASS: TestProbeComponentManagerSuite/RepeatedStopIsNoOp (0.00s)
    --- PASS: TestProbeComponentManagerSuite/SecondStartRefusedOrRestartCycle (0.00s)
    --- PASS: TestProbeComponentManagerSuite/FailedStartHoldsNothing (0.00s)
    zz_cm_probe_test.go:87: after Start: component startCtx.Err=<nil>
    zz_cm_probe_test.go:93: after manager Start ctx cancelled: component startCtx.Err=context canceled base-status=running
    zz_cm_probe_test.go:102: live Start ctx, after Stop: err=<nil> component startCtx.Err=context canceled
--- PASS: TestProbeComponentStartContextAfterManagerStart (0.00s)
FAIL
FAIL    github.com/c360studio/semstreams/service    0.675s
FAIL
```

Read: 7 of 8 checks pass; `StartContextEndStopsNothing` fails. A component's Start context is live right after `Start`
returned, has ended once the manager's Start context is cancelled, and has ended after a `Stop` that returned nil with
the manager's Start context still live (pin `:1040-1042`). The ERROR line is the `mustFail` owner's component failing
its `Start` on purpose.

**Build probe: does the pin's `ComponentManager` satisfy PR 2's `componentStartup` as is?** Yes. go1.26.10 darwin/arm64,
in a scratch copy of this tree (`git archive <commit> | tar -x -C <dir>`: `7c4dd2c` for steps 1-2, `d39fae3`, whose code
is the same, for step 3), with the five pin files copied into `service/` and the module path rewritten with
`sed 's#github.com/c360studio/semstreams#github.com/c360studio/semengine#g'`. Step 1, `go build -gcflags=-e ./service/`
(tabs in the outputs and the diff below are shown as four spaces):

```text
service/component_manager.go:19:2: no required module provides package github.com/c360studio/semengine/internal/componentadmission; to add it:
    go get github.com/c360studio/semengine/internal/componentadmission
service/component_manager.go:20:2: no required module provides package github.com/c360studio/semengine/internal/lifecyclecleanup; to add it:
    go get github.com/c360studio/semengine/internal/lifecyclecleanup
```

Step 2, after removing the `internal/componentadmission` import and its `Access{}` arguments and moving
`lifecyclecleanup` to `pkg/` (ledger `:1898`), the same command:

```text
# github.com/c360studio/semengine/service
service/component_manager.go:46:29: undefined: component.ToolRegistryReader
service/component_manager.go:191:69: too many arguments in call to NewBaseServiceWithOptions
    have (string, nil, []Option...)
    want (string, ...Option)
service/component_manager.go:196:29: undefined: component.ToolRegistryReader
service/component_manager.go:202:23: deps.ToolRegistry undefined (type *Dependencies has no field or method ToolRegistry)
service/component_manager.go:204:27: deps.LifecycleManager undefined (type *Dependencies has no field or method LifecycleManager)
service/component_manager.go:399:48: snapshot.Declaration undefined (type component.Declaration has no field or method Declaration)
service/component_manager.go:696:75: mc.Component.Meta undefined (type component.Discoverable has no field or method Meta)
service/component_manager.go:701:39: mc.Component.Meta undefined (type component.Discoverable has no field or method Meta)
service/component_manager.go:713:39: mc.Component.Meta undefined (type component.Discoverable has no field or method Meta)
service/component_manager.go:719:87: mc.Component.Meta undefined (type component.Discoverable has no field or method Meta)
service/component_manager.go:759:9: writer.publishComponents undefined (type *startupMetricWriter has no field or method publishComponents)
service/component_manager.go:895:10: undefined: attributeShutdownError
service/component_manager.go:936:10: undefined: attributeShutdownError
service/component_manager.go:1014:11: undefined: attributeShutdownError
service/component_manager.go:1209:23: undefined: component.PlatformMeta
service/component_manager.go:1215:3: unknown field ToolRegistry in struct literal of type component.Dependencies
service/component_manager.go:1217:3: unknown field LifecycleManager in struct literal of type component.Dependencies
service/rule_pack_bind.go:16:36: undefined: projection.PredicateReconciler
service/stream_override_expiry.go:78:66: undefined: metric.MetricsRegistrar
```

Every one is in the inventory's § 2.4, plus `NewBaseServiceWithOptions`' dropped config argument (PR 1 Q1) and the two
names PR 2 left for this PR (P16): `publishComponents` (pin `startup_metrics.go:82`) and `attributeShutdownError` (pin
`service_manager.go:1490`). Step 3: the hand edits below (the rest of § 2.4, stubs for those two names, an
`interface{}` in place of `projection.PredicateReconciler`) and the assertion `var _ componentStartup =
(*ComponentManager)(nil)`; then `go build -gcflags=-e ./service/` and `go vet ./service/` both print nothing and exit
0. The diff, `diff -U0` of each file against the pin file with only the module path rewritten
(`component_manager_config.go` and `component_manager_http.go` unchanged):

```diff
--- pin-rewritten/service/component_manager.go
+++ scratch/service/component_manager.go
@@ -19,2 +19 @@
-    "github.com/c360studio/semengine/internal/componentadmission"
-    "github.com/c360studio/semengine/internal/lifecyclecleanup"
+    "github.com/c360studio/semengine/pkg/lifecyclecleanup"
@@ -25 +23,0 @@
-    "github.com/c360studio/semengine/pkg/lifecycle"
@@ -47 +44,0 @@
-    toolRegistry     component.ToolRegistryReader           // Shared tool registry plumbed via deps.ToolRegistry to managed components
@@ -49 +45,0 @@
-    lifecycleManager *lifecycle.Manager                     // Shared Lifecycle harness Manager plumbed via deps.LifecycleManager (ADR-047). Nil when no app workflows are registered.
@@ -192 +188 @@
-    baseService := NewBaseServiceWithOptions("component-manager", nil, opts...) // Config is now service-specific
+    baseService := NewBaseServiceWithOptions("component-manager", opts...) // Config is now service-specific
@@ -197 +192,0 @@
-    var toolRegistry component.ToolRegistryReader
@@ -199 +193,0 @@
-    var lifecycleManager *lifecycle.Manager
@@ -203 +196,0 @@
-        toolRegistry = deps.ToolRegistry
@@ -205 +197,0 @@
-        lifecycleManager = deps.LifecycleManager
@@ -217 +208,0 @@
-        toolRegistry:      toolRegistry,
@@ -219 +209,0 @@
-        lifecycleManager:  lifecycleManager,
@@ -290 +280 @@
-        cm.registry.SealComposition(componentadmission.Access{})
+        cm.registry.SealComposition()
@@ -384 +374 @@
-    cm.registry.SealComposition(componentadmission.Access{})
+    cm.registry.SealComposition()
@@ -397 +387 @@
-    snapshots := cm.registry.Snapshots(componentadmission.Access{})
+    snapshots := cm.registry.Snapshots()
@@ -400 +390 @@
-        declarations = append(declarations, snapshot.Declaration())
+        declarations = append(declarations, snapshot)
@@ -697 +687 @@
-    cm.logger.Debug("Starting component", "name", name, "type", mc.Component.Meta().Type)
+    cm.logger.Debug("Starting component", "name", name, "type", mc.Config.Type)
@@ -702 +692 @@
-            "name", name, "type", mc.Component.Meta().Type, "error", err)
+            "name", name, "type", mc.Config.Type, "error", err)
@@ -714 +704 @@
-            "name", name, "type", mc.Component.Meta().Type, "error", err)
+            "name", name, "type", mc.Config.Type, "error", err)
@@ -720 +710 @@
-    cm.logger.Debug("Component started successfully", "name", name, "type", mc.Component.Meta().Type)
+    cm.logger.Debug("Component started successfully", "name", name, "type", mc.Config.Type)
@@ -1108 +1098 @@
-        componentadmission.Access{}, instanceName, cfg, deps,
+        instanceName, cfg, deps,
@@ -1210 +1200 @@
-        Platform: component.PlatformMeta{
+        Platform: types.PlatformMeta{
@@ -1216 +1205,0 @@
-        ToolRegistry:     cm.toolRegistry,
@@ -1218 +1206,0 @@
-        LifecycleManager: cm.lifecycleManager,
@@ -1526,0 +1515,3 @@
+
+func (w *startupMetricWriter) publishComponents() {}
+func attributeShutdownError(owner string, phase errs.ShutdownPhase, err error) error { return err }
--- pin-rewritten/service/stream_override_expiry.go
+++ scratch/service/stream_override_expiry.go
@@ -78,2 +78,2 @@
-func (r *streamOverrideExpiryReporter) register(registrar metric.MetricsRegistrar) error {
-    return registrar.RegisterGaugeVec("streams", "migration_override_expired", r.expired)
+func (r *streamOverrideExpiryReporter) register(registrar *metric.MetricsRegistry) error { _ = registrar; return nil
+
--- pin-rewritten/service/rule_pack_bind.go
+++ scratch/service/rule_pack_bind.go
@@ -16 +16 @@
-    SetPredicateReconciler(projection.PredicateReconciler) error
+    SetPredicateReconciler(interface{}) error
@@ -132,0 +133 @@
+var _ componentStartup = (*ComponentManager)(nil)
```

The six methods: `startupSnapshot` `component_manager.go:724`, `startupStopBegun` `:769`, `setStartupMetricWriter`
`:763`, `withComponents` `:830`, `GetComponentHealth` `:1319`, `registerStartupHTTPHandlers`
`component_manager_http.go:83` (pin lines; the diff's `+` line numbers are the scratch copy's).

**Other runs.** The unit runs (once under `-race`, five times shuffled), the integration run and the test-text guards
are the inventory's § 7, run on the whole package. Their hits in this PR's tests: `attack_test.go:108` (`time.Sleep`),
`config_boot_process_restart_integration_test.go:106` (`t.Skip("subprocess helper")`), and #195's 12
`natsclient.NewTestClient` calls; plus C14, which the guards do not detect.
