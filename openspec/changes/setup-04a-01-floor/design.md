# Design: setup-04a-01-floor

Status: architect draft for independent design review, then owner acceptance. The slicing, the harness extension's
shape, the owner definition and the ledger conventions are the foundation design's, accepted on #9; this document
cites it as "foundation D*n*" (`openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md`) and decides only
what the foundation left to the first change. The inventory it rests on is the foundation's `inventory.md` (§n).

## Purpose and admission

Foundation D2 row 1 and D3 fix the scope: the 16 packages of closure(natsclient) ∪ closure(message), their tests, the
harness extension of D4, the failed-start check of D5, the first five adapters of D6, the ledger items of D7 and D9
that need no port, and the five spec deltas of D10.1. Admission gates for the change: `task verify` green, the three
`cover:check` targets at 80%, every owner in the set green under the lifecycle suite including the failed-start
check, the I8 test green, and the ledger rows validated by `task ledger:check`.

## Sources

- Foundation design D1–D12, Premises P2, P9–P16, P20–P26, "Owner rulings"; foundation inventory §1.2, §1.4, §1.5,
  §1.6, §3.1–§3.4, §4.1, §6.
- Base `8e0aabc` (= `main`): `internal/harness/natsfixture/{fixture.go,deps.go,errors.go,fixture_integration_test.go}`,
  `internal/harness/lifecycletest/{lifecycletest.go,refowner_test.go}`, `internal/harness/contract/imports_test.go`,
  `internal/harness/runner/runner_test.go`, `scripts/cover-check.sh`, `docs/admission-ledger.yaml`, `openspec/specs/*`.
- Pin `8b99efe9` (scratch snapshot): the 16 packages; `internal/semantictest/fixtures.go`; `payloadregistry/testing.go`;
  `natsclient/{client.go,kv.go,kvspec.go,delivery_settlement.go,delivery_settlement_integration_test.go,test_client.go}`;
  `metric/handler.go`; `pkg/resource/watcher.go`; `pkg/cache/coalescing_set.go`; `pkg/errs/errs.go`.
- Per-package measurement: `scratchpad/04a/chain4.py` output for C1 and the per-package table reproduced in D1 (lines,
  files, tests, integration-tagged tests, `NewTestClient` sites, context roots, in-set imports, third-party imports).

## Decisions

### D1. The 16 packages, in port order

Port order is import order within the set (leaves first), so each package compiles against already-ported packages
and no temporary stub exists. Lines are non-test lines at the pin; "tests" are files / lines; "roots" are production
`context.Background()`/`TODO()` sites to triage (foundation D9).

| Level | Package | Lines / files | Tests | Roots | In-set imports | Third-party |
|---|---|---|---|---|---|---|
| 0 | `pkg/platform` | 43 / 1 | none at the pin | 0 | — | — |
| 0 | `pkg/resource` | 398 / 2 | 1 / 370 | 0 | — | — |
| 0 | `pkg/retry` | 262 / 2 | 1 / 228 | 0 | — | — |
| 0 | `pkg/security` | 384 / 2 | none at the pin | 0 | — | — |
| 0 | `pkg/timestamp` | 366 / 2 | 2 / 808 | 0 | — | — |
| 1 | `pkg/errs` | 904 / 3 | 3 / 741 | 2 | `pkg/retry` | — |
| 1 | `vocabulary` | 2,673 / 11 | 11 / 2,184 | 0 | `pkg/platform` | — |
| 2 | `pkg/acme` | 543 / 2 | 2 / 215 (1 integration) | 0 | `pkg/errs` | `go-acme/lego/v4` ×6 |
| 2 | `pkg/types` | 751 / 9 | 6 / 736 | 0 | `pkg/errs` | — |
| 3 | `pkg/projection/contract` | 163 / 1 | 1 / 54 | 0 | `pkg/types`, `vocabulary` | — |
| 3 | `pkg/tlsutil` | 533 / 2 | 2 / 1,264 (1 integration) | 0 | `pkg/acme`, `pkg/errs`, `pkg/security` | — |
| 4 | `metric` | 1,218 / 4 | 3 / 1,222 | 2 | `pkg/errs`, `pkg/security`, `pkg/tlsutil` | prometheus ×3 |
| 4 | `payloadregistry` | 508 / 2 | 2 / 831 | 0 | `pkg/errs`, `pkg/projection/contract`, `pkg/types`, `vocabulary` | — |
| 5 | `message` | 2,186 / 16 | 9 / 2,209 | 0 | `payloadregistry`, `pkg/errs`, `pkg/platform`, `pkg/timestamp`, `pkg/types` | `google/uuid` |
| 5 | `pkg/cache` | 2,449 / 11 | 6 / 2,369 | 1 | `metric`, `pkg/errs` | prometheus |
| 6 | `natsclient` | 12,377 / 29 | 78 / 21,656 (29 integration; 69 `NewTestClient`) | 9 | `metric`, `pkg/cache`, `pkg/errs`, `pkg/resource`, `pkg/retry` | nats.go, jetstream, prometheus; testcontainers ×2 and `docker/go-connections/nat` in `test_client.go` only |

Totals: 16 / 25,758; 127 / 34,887; 14 roots; 31 integration-tagged test files. `pkg/platform` and `pkg/security` have
no tests at the pin; none are invented — their rows say so and D10 does not gate them. Ported files keep their pin
paths under the SemEngine module; `test_client.go` and `test_options.go` are not ported (their ledger rows:
`adapt → natsfixture`, `defer-exclude`).

### D2. Harness API shapes (foundation D4, made concrete)

- **`Fixture.Restart(ctx) error`** (`natsfixture`). Takes the one-slot semaphore like `Start`/`Stop`; drains the
  fixture's own connection (dialled with `nats.MaxReconnects(0)`, so it is dead after a restart); runs two new
  phases, `PhaseStopContainer = "stop-container"` (`Container.Stop(ctx, &timeout)`) and `PhaseStartContainer =
  "start-container"` (`Container.Start(ctx)`), on the same container; then re-runs `PhaseMappedPort`, `PhaseConnect`,
  `PhaseJetStream` and replaces `url`, `nc`, `js` and the evidence record's mapped port. Contract: `URL()` and
  `JetStream()` are valid until the next `Restart`; a caller stops its owner before and starts it after with the new
  URL (the re-read-and-reconnect shape, foundation D4-C). A failure in any phase is a `FixtureError` carrying the
  phase; no replacement container is attempted (the one-replacement rule is untouched); `Stop` then terminates as
  today. Two new `deps` hooks (`stopContainer`, `startContainer`) extend the fault matrix; `count("restart")` is
  recorded. Fixture-owned resources across a restart: consumers created by `Consume` run on the drained connection,
  so `Restart` stops each of them first (the same stop path `Stop` uses: cancel, wait for the handler to be idle,
  stop the consume context) and records them ended; it does not re-create them — a test that needs a consumer after
  the restart calls `Consume` again. Streams and buckets stay in the ownership record; `Stop` deletes them as
  today and treats one that no longer exists (a memory-backed stream after a restart) as observed absent, which is
  already its success condition. Before `Start` has succeeded, or once `Stop` has begun, `Restart` returns an
  error and makes no Docker call. Memory-backed streams: `CreateMemoryStream(ctx, name, subjects...)` creates a
  stream the fixture owns with the same `MaxAge`, `MaxBytes` and `DiscardOld` bounds as `CreateStream`
  (`fixture.go:431-446`) and memory storage; a test never creates a stream through `JetStream()` directly, so the
  ownership record and the bounds rule hold for both storage classes. The durability premise (P3) is this change's
  proof, not an assumption.
- **`natsfixture.FaultKV`**. `NewFaultKV(real jetstream.KeyValue) *FaultKV` embeds the real bucket and overrides
  `Put`, `Create`, `Update`, `Delete`. `FailBefore(op KVOp, err error)` returns `err` without calling the real
  method; `FailAfter(op KVOp, err error)` calls the real method, discards its result, and returns `err` — the
  "server applied it, client saw an error" shape of #20. `Calls() map[KVOp]int` reports real calls made. It is typed
  on `jetstream.KeyValue`, never on `natsclient.KVStore`, so `natsfixture` keeps importing no ported package (P4).
  Its consumer in change 2 wraps it in `natsclient.NewKVStore` through an in-package test setter.
- **`internal/harness/prochost`**. `prochost.Helper(name string, fn func())` is called from the test binary's
  `TestHelperProcess`; when the environment marker `SEMENGINE_HELPER=<name>` is set it runs `fn` and exits, else it
  returns immediately so the test is a no-op in a normal run. `prochost.Start(t, name string, env ...string)
  (*Process, error)` runs `os.Args[0] -test.run=^TestHelperProcess$` with the marker, in its own process group, with
  stdout/stderr captured to the evidence directory. `Process` offers `Signal(os.Signal)`, `Pause()`/`Resume()`
  (SIGSTOP/SIGCONT), `Kill()`, `Wait(ctx) (ExitStatus, error)`, `Alive() bool`. `t.Cleanup` kills the group and
  waits under a fresh bounded context, so no helper survives its test. Reused from `runner_test.go`: the
  `exec.CommandContext` start, SIGTERM and `Process.Kill` paths, `Setpgid` and group signalling, `pidAlive`.
- **`lifecycletest.Run(t *testing.T, factory Factory, mustFail StartFailure, promise Promise)`**. `StartFailure` is
  a struct with an unexported `kind` field and an unexported factory field; its only constructors are
  `MustFail(f Factory) StartFailure` (kind "must fail"; `f` returns a fresh owner whose `Start(ctx)` must return a
  non-nil error; `MustFail(nil)` panics at the call site naming the argument rather than deferring to `Run`) and
  the function `NoFallibleStart() StartFailure` (kind "no fallible start", D7; a function, not an exported variable,
  so the value cannot be reassigned). The zero value has no kind, and
  `Run` rejects it at run time with a failure naming the argument, so a forgotten or zero `mustFail` can never read
  as the exemption; a `Factory` cannot be passed where a `StartFailure` is expected, so omission or a bare function
  is a compile error. New check `CheckFailedStartHoldsNothing(ctx, o Owner) error`: `Start` returns non-nil;
  `Observe().Unresolved` is empty; `Stop(ctx)` returns nil; `Observe().Calls` is unchanged by the `Stop`. The two
  callers on the base change: `refowner_test.go:303-304`
  (the `refowner` double gains a `startFails` failpoint) and `natsfixture/fixture_integration_test.go:519`
  (`TestS1_7Restart`; the must-fail factory is a `Fixture` whose `deps.start` hook returns an error, so `Start`
  returns a `FixtureError` and `Observe` reports nothing held). No readiness accessor is added to `Owner` (#38).
- **Rehomed helpers**. `internal/harness/semantictest` keeps `EntityID(…)` and `Predicate(t testing.TB, …)` as at the
  pin (`fixtures.go:20,41`), importing `pkg/types` and `vocabulary`; `internal/harness/payloadfixture` keeps
  `NewForTest`, `NewWithSubset`, `RegisterTestType` (`testing.go:18,32,53`), importing `payloadregistry`. The bound
  for harness imports (foundation D4, N1): `natsfixture` imports no ported package; other harness packages may
  import ported packages that are pure libraries under the owner definition, after a cycle check over the helper's
  full dependency closure (P5).

### D3. The five adapters and their failing factories (foundation D5, D6)

Adapters are `_test.go` files in the owner's package, so they read unexported state; each lists every retained kind
the owner holds, per the `lifecycle-suite` delta's `Observe` contract.

| Owner | Start / end at the pin | `Unresolved` lists | Failing factory |
|---|---|---|---|
| `metric.Server` | `Start(ctx)` `handler.go:59`, `Stop(ctx)` `:192` | the listener, the `http.Server`, the serve goroutine | a server configured on a port the test already holds with `net.Listen`; `Start` binds synchronously (`:55-110`) and returns the bind error |
| `natsclient.Client` | `Connect(ctx)` `client.go:471`, `Close(ctx)` `:578` | the `nats.Conn`, JetStream handle, subscriptions, internal consumer claims, health-monitor and metrics goroutines (`metricsCancel`) | a client whose URL is a refused local port (its own `Connect` returns the dial error) |
| `natsclient.TemporalResolver` | `NewTemporalResolver(ctx, bucket)` `kv_temporal.go:20-41`, `Close()` | the history cache and its ticker goroutine | **none at the pin**: the constructor never reads the bucket and its only error path is `cache.NewTTL` failing on fixed arguments (D7) |
| `pkg/cache.CoalescingSet` | `NewCoalescingSet(ctx, window, cb)` `:27`, `Close()` `:116` → `Close(ctx)` (adapt row) | the ticker and the `run` goroutine | **none at the pin**: the constructor returns no error and refuses nothing (D7) |
| `pkg/resource.Watcher` | `StartBackgroundCheck(ctx)` `watcher.go:141`, `Stop()` `:218` → `Stop(ctx)` (adapt row) | the check goroutine and its cancel | **none at the pin**: `StartBackgroundCheck` returns nothing (D7) |

The suite's floor is not known to pass on the pin's code for any of the five: that is measured before porting
(tasks 2.0), and every failing check becomes a named, failing-first item on the owner's row. One is known now:
`Client.Close(ctx)` has no nil check and calls `ctx.Deadline()` with a live connection (`client.go:578-620,
:686`), so `Close(nil)` panics, and `Connect(ctx)` shows no nil check before its first operation (`:471-492`),
while `Subscription.Drain(nil)` is already refused (`:796-799`). Decision: `Close` and `Connect` refuse a nil
context, as an `adapt` item on the `natsclient` row with failing-first tests. Why: the nil-context check is part
of the lifecycle floor every owner passes (SETUP 02 `lifecycle-suite`, "Portable floor"); `Client` is the first
ported owner and exempting it would make the floor optional at the first opportunity; the pin already holds this
contract for `Drain`; and no caller passes nil legitimately — today it is a crash, not a behaviour anyone relies
on. The `transport-client` delta states the requirement; the row records it as changed behaviour, not carried.

The two adapt rows change `CoalescingSet.Close()` to `Close(ctx) error` and `resource.Watcher.Stop()` to
`Stop(ctx) error`, each bounded by the context instead of waiting forever on `<-c.done` / `wg.Wait()`. The
unbounded wait is reachable only when the callback (`CoalescingSet`) or the check function (`resource.Watcher`,
whose `Stop` cancels the loop's context before `wg.Wait()`, `watcher.go:218-227`) ignores its context, so the
failing-first test plants exactly that: a callback or check function that blocks until the test releases it.
Neither has a
production caller of the changed method inside this change (`StartBackgroundCheck` has no production caller at the
pin at all); the later callers of `CoalescingSet.Close` (`processor/graph-embedding/component.go:875`,
`processor/rule/entity_watcher.go:980`) are port-refactor rows in changes 5 and 6 (ruling h), named now in the
`pkg/cache` row's `known_risks`.

### D4. Repair evidence this change's own code can produce (ruling g)

- **Settlement, `natsclient` half.** `natsclient/delivery_settlement.go` (`DeliveryWork`, `DeliveryDecision`,
  `DeliveryRetryPolicy`, `HeartbeatDeliveryPolicy`, `SettleDelivery`, `ConsumeDeliveryWithHeartbeat`) is carried
  with its 18 unit tests and three integration tests (`delivery_settlement_integration_test.go:17,136,231`: healthy
  heartbeat renewal prevents overlap; stopped renewal backs off; semantic retry produces durable redelivery). They
  run against `natsfixture` in this change and are the row's evidence for this package; the graph-ingest half is
  change 2's. The `transport-client` delta's "Settlement follows the decision" requirement is what they assert.
- **SS#1218, `pkg/errs` home.** `pkg/errs.ErrAlreadyStopped` (`errs.go:47`) is the one sentinel; the duplicate
  `service.ErrAlreadyStopped` (`service/base.go:28`) is removed in change 3, where the proof lives. This change's
  row for `pkg/errs` names the sentinel and the later change.
- **Acknowledged is not durable.** The `transport-client` delta's first requirement is proven here against
  `Fixture.Restart`: a message acknowledged on a memory stream is absent after a restart, one on a file stream is
  present. This is the `natsclient`-side half of #15's row; the graph-ingest scenarios are change 2's.

### D5. Ledger rows (foundation D7, D9, D4a)

- Sixteen package rows, `source_path` = package directory, `source_sha` = `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`,
  `destination` per D16 — public for the eight packages SemSource imports directly at `e4febc0d` (§5.1):
  `natsclient`, `metric`, `payloadregistry` (`run.go`), `message` (28 files), `vocabulary` (20), `pkg/types`
  (5), `pkg/retry` (4), `pkg/errs` (3); internal for the other eight unless SemConnect's or semboids' measured
  sets (pass3 §2.1) say otherwise, recorded per row —
  `proving_tests` naming the carried tests and the suite run, `known_risks` carrying the context-root triage (a
  legitimate root with its reason, or a defect) and the nats.go v1.52→v1.54 pin difference for `natsclient`.
  Dispositions: `adapt` for `natsclient` (`NewTestClient` sites, `test_client.go`), `payloadregistry` (`testing.go`
  rehomed), `pkg/cache` and `pkg/resource` (SS#1415-class enders); `carry` for the other twelve.
- Existing file rows updated: `natsclient/test_client.go` (`adapt`, now with `evidence`), `natsclient/test_options.go`
  (`defer-exclude`, honoured); a new file row for `payloadregistry/testing.go` is not needed — the package row
  records the rehoming (T-B7 keeps `source_path` unique; the package row's path is the directory).
- Eight `defer-exclude` rows for the D4 ten-out packages that are never carried: `agentic`, `agentic/agentrun`,
  `gateway`, `gateway/graph-gateway`, `internal/agentterminal`, `internal/deliverylane`, `internal/looptoken`,
  `vocabulary/agentic`. `graph/llm` and `model/wire` get carried-dormant rows in change 2.
- Port-refactor rows: none are performed here; the two later `CoalescingSet.Close` callers are named in `known_risks`
  and get their own `class:port-refactor` rows in changes 5 and 6.
- The Tier-1 cross-check re-measured on the ruled set (#9 item 1) is an inventory the architect writes and the
  technical writer records next to the ledger; it does not change a row.

### D6. Gates this change adds or extends

`scripts/cover-check.sh` gains targets `natsclient`, `message`, `payloadregistry` at 80% (D10; baseline unmeasured —
the first measurement is taken when the package lands, P8). The I8 test lands in `internal/harness/contract`
alongside the T-B8 aggregator rule with a tree-shape sensitivity test like `TestImportGraphSensitivity`. Package-doc
lint is already on (revive `package-comments`, `revive.toml:22`) and now covers eight public packages; every package
in the set has a package comment at the pin (§3.3), so the task is a sensitivity check, not an enablement. The
compiled example consumer (D16's other gate) composes `service` and is change 3's; it will import at least these
eight. `AGENTS.md:43-44` is corrected to `scripts/verify.sh:10-11`'s list. No context-root guard is added (D9).

### D7. Owner question: the must-fail factory where a start cannot fail

Ruling #38 and the `lifecycle-suite` delta require every `Run` call to pass a factory whose owner's `Start` returns an
error. Three of this change's five owners have no start that can fail from any caller input (D3):
`NewTemporalResolver` never touches the bucket and only `cache.NewTTL` on fixed arguments can fail;
`NewCoalescingSet` returns no error; `StartBackgroundCheck` returns nothing. A failing `Start` manufactured by the
adapter would prove nothing about the owner — the shape #38 exists to prevent. Options, with the assumed answer
marked:

1. **Typed "no fallible start" value (assumed).** `Run` keeps the required parameter; an owner without a fallible
   start passes `lifecycletest.NoFallibleStart()`, a value of the `StartFailure` type (D2) whose other constructor is
   `MustFail(f Factory)` (which panics on a nil factory at the call site): the start-failure subtest records "no
   fallible start" in its output instead of passing
   silently, and the owner's ledger row says the same. A `Factory` is a function value and compares only to nil,
   so a nil sentinel would turn any accidental nil into the exemption — hence the struct type with an unexported
   kind, whose zero value `Run` rejects at run time. Omission is still a compile error; the exemption is visible in
   the test and in the row; the owner is
   still proven by the floor's other checks (Stop before Start holds nothing; controlled Stop joins; repeated Stop
   is a no-op). Cost: a reviewer must confirm the exemption is true for the owner (a checklist item), because a
   misuse on an owner that does have a fallible start is the silent-skip shape.
2. **Give the owner a fallible start under an `adapt` row.** `NewCoalescingSet` returns an error for a window ≤ 0
   (today it silently substitutes a minimum), `StartBackgroundCheck` returns an error for a nil check function or
   a second start, `NewTemporalResolver` validates its bucket. Cost: invented contracts no consumer asked for,
   signature changes whose callers are in later changes (`NewCoalescingSet`: `processor/graph-embedding`,
   `processor/rule`), and port-refactor rows for them.
3. **Exclude the owner from the suite with the reason on its row** (the delta's existing shape for a start with no
   context). Cost: the two owners with unbounded waits lose exactly the join checks that justify their adapt rows.

Owners this touches in later changes, from the 42-owner scan: every other owner has `Start(ctx) error` or a
constructor returning an error, so the question recurs only if a later port finds such a start unreachable by any
input; the ruling is asked once and applied per owner at port time, recorded on the row. Hold: task 1.4.

## Premises (each with its measurement)

- P1. The 16 packages are closure(natsclient) ∪ closure(message), closed under in-set imports, with no out-of-set
  edge. — foundation P2; D1's in-set column (`inset-edges.json`).
- P2. The set has five owners under the foundation's definition and no `Start`/`Stop` component. — foundation P22;
  `owners-scan.txt`; D3.
- P3. The fixture's JetStream store lives in the container's writable layer (`--js`, no `-sd`, no volume,
  `fixture.go:251-254`), which `Stop`/`Start` preserves. — read from the fixture; proven by this change's restart
  test, not assumed.
- P4. `natsfixture` imports no ported package and `natsclient`'s 78 test files are all internal-package, so
  `natsfixture → natsclient` would be a cycle. — foundation P9, P10.
- P5. No test in `pkg/types`, `vocabulary` or `payloadregistry` imports `semantictest` or calls the payload helpers;
  the closures of both helpers contain only those packages and their in-set dependencies. — foundation P24, P26;
  pin grep.
- P6. T-B1 skips `_test.go` files and `internal/harness/`, parses without evaluating build constraints, and forbids
  `testing`, testcontainers, yaml and the harness in every other non-test file. — `imports_test.go:48-72`.
- P7. `lifecycletest.Run` has two call sites on the base. — foundation P23.
- P8. Coverage at the pin for `natsclient`, `message`, `payloadregistry` is not measured. — foundation D10 ("baseline
  coverage at the pin is not established"); measured in task 3.6.
- P9. The mapped host port may change across a container restart. — SemSource `qualification_test.go:328-334`.
- P10. `StartBackgroundCheck` has no production caller at the pin; `CoalescingSet.Close` is called at
  `graph-embedding/component.go:875` and `rule/entity_watcher.go:980`. — pin grep (foundation N3a).
- P11. The settlement surface has 18 unit tests and 3 integration tests at the pin. — pin `grep -c 'func Test'`.
- P12. `metric.Server.Start` binds its listener synchronously. — `metric/handler.go:55-110` (review re-check 1).
- P13. `Client.Close(ctx)` has no nil-context check and reaches `ctx.Deadline()` with a live connection;
  `Connect` shows none before its first operation; `Subscription.Drain` refuses nil. — `natsclient/client.go:578-620,
  :686, :471-492, :796-799` (change review 1b).
- P14. `NewTemporalResolver` never reads its bucket; `NewCoalescingSet` returns no error; `StartBackgroundCheck`
  returns nothing. — `natsclient/kv_temporal.go:20-41`, `pkg/cache/coalescing_set.go:27`, `pkg/resource/watcher.go:141`
  (change review 1a).
- P15. `CreateStream` always creates a file-backed stream with the fixture's bounds. — `natsfixture/fixture.go:431-446`.
- P16. SemSource at `e4febc0d` imports eight of the 16 directly: `natsclient`, `metric`, `payloadregistry`,
  `message` (28 files), `vocabulary` (20), `pkg/types` (5), `pkg/retry` (4), `pkg/errs` (3). — foundation §5.1.
- P17. Whether the pin's five owners pass the suite's seven existing checks is not measured; task 2.0 measures it.

## Declared costs

- Nothing boots. The change's green is substrate, harness and five owners.
- `go-acme/lego/v4` (six paths) enters the module for `pkg/acme` (543 lines); `task vuln` scans it from now on.
- Thirty-one integration-tagged test files (29 in `natsclient`) run only under `task test:integration` and the host
  lock; the unit lane does not prove them.
- The `Run` signature change touches both existing callers and every future owner test; that is the point.
- Five adapters read unexported fields; a reviewer re-checks each against the owner's retained kinds.
- `URL()` changes after `Restart`; a test that forgets to re-dial fails loudly, not silently.
- If a critical package measures below 80% at landing, the change holds for the owner (task 3.6); the design does not
  predict the number.
- The nats.go version differs from the pin (v1.54.0 vs v1.52.0); regression evidence is the carried `natsclient`
  tests passing, nothing more.
- `Close(nil)`/`Connect(nil)` refusal is changed behaviour in `natsclient`; it is recorded as such, not as carried.
- Until the owner rules D7, three of the five owners cannot run `Run` as the delta is drafted; the hold is task 1.4
  and the port tasks for those owners (3.1, 3.6, 3.7) depend on it.
- The pre-port probe (task 2.0) may find more floor failures than P13; each becomes an adapt item and the row grows.
