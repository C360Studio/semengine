# Design: setup-04a-01-floor

Status: accepted on #9 (2026-10-01, task 1.3); amended by the rulings it cites. The slicing, the harness extension's
shape, the owner definition and the ledger conventions are the foundation design's, accepted on #9; this document
cites it as "foundation D*n*" (`openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md`) and decides only
what the foundation left to the first change. The inventory it rests on is the foundation's `inventory.md` (§n).

## Purpose and admission

Foundation D2 row 1 and D3 fix the scope: the 15 packages of closure(natsclient) ∪ closure(message) less `pkg/acme`
(D1), their tests, the
harness extension of D4, the failed-start check of D5, the lifecycle suite on the two services of this set and the
helper tests of D7, the ledger items that need no port, and the five spec deltas of D10.1, the `background-work`
delta that D7 adds (#9 comments 5950234192, 5950482163), and the `metric-registry` and `message-codec` deltas that
state D9's law and the `message` codec laws (#9 comment 5983188211). Admission gates for the
change: `task verify` green, the three `cover:check` targets at 80%, both services green under the lifecycle suite
including the failed-start check, every helper green under its three tests (D7), the I8 test green, and the ledger
rows validated by `task ledger:check`.

## Sources

- Foundation design D1–D12, Premises P2, P9–P16, P20–P26, "Owner rulings"; foundation inventory §1.2, §1.4, §1.5,
  §1.6, §3.1–§3.4, §4.1, §6.
- Base `8e0aabc` (= `main`): `internal/harness/natsfixture/{fixture.go,deps.go,errors.go,fixture_integration_test.go}`,
  `internal/harness/lifecycletest/{lifecycletest.go,refowner_test.go}`, `internal/harness/contract/imports_test.go`,
  `internal/harness/runner/runner_test.go`, `scripts/cover-check.sh`, `docs/admission-ledger.yaml`, `openspec/specs/*`.
- Pin `8b99efe9` (scratch snapshot): the 15 packages and `pkg/acme`; `internal/semantictest/fixtures.go`;
  `payloadregistry/testing.go`;
  `natsclient/{client.go,kv.go,kvspec.go,delivery_settlement.go,delivery_settlement_integration_test.go,test_client.go}`;
  `metric/handler.go`; `pkg/resource/watcher.go`; `pkg/cache/coalescing_set.go`; `pkg/errs/errs.go`.
- Per-package measurement: `scratchpad/04a/chain4.py` output for C1 and the per-package table reproduced in D1 (lines,
  files, tests, integration-tagged tests, `NewTestClient` sites, context roots, in-set imports, third-party imports).
- Overlap: draft PR #73 (`claude/authority-one-spelling`) also changes `harness-boundaries`. This change carries the
  names half of #72 ruling A (requirement "No second spelling of deployment authority",
  `TestNoDeploymentAuthorityNames`; #9 comment 5969776736 item 3). #48 merges first. #73 then MODIFIES that
  requirement to add the field check (ruling C) rather than ADDing a second requirement for the same names.

## Decisions

### D1. The 15 packages, in port order

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
| 2 | `pkg/types` | 751 / 9 | 6 / 736 | 0 | `pkg/errs` | — |
| 3 | `pkg/projection/contract` | 163 / 1 | 1 / 54 | 0 | `pkg/types`, `vocabulary` | — |
| 3 | `pkg/tlsutil` | 533 / 2 at the pin; 180 cut (`tlsutil.go:186-365`) | 2 / 1,264 (1 integration) | 0 | `pkg/errs`, `pkg/security` | — |
| 4 | `metric` | 1,218 / 4 | 3 / 1,222 | 2 | `pkg/errs`, `pkg/security`, `pkg/tlsutil` | prometheus ×3 |
| 4 | `payloadregistry` | 508 / 2 | 2 / 831 | 0 | `pkg/errs`, `pkg/projection/contract`, `pkg/types`, `vocabulary` | — |
| 5 | `message` | 2,186 / 16 | 9 / 2,209 | 0 | `payloadregistry`, `pkg/errs`, `pkg/platform`, `pkg/timestamp`, `pkg/types` | `google/uuid` |
| 5 | `pkg/cache` | 2,449 / 11 | 6 / 2,369 | 1 | `metric`, `pkg/errs` | prometheus |
| 6 | `natsclient` | 12,377 / 29 | 78 / 21,656 at the pin; 70 ported (43 unit, 27 integration) after six exclusions and two files removed with dropped surface; 51 `NewTestClient` sites | 2 live (`client.go:566`; `trace.go:56`, dropped) | `metric`, `pkg/errs`, `pkg/resource`, `pkg/retry` | nats.go, jetstream, prometheus; test-only: `nats-server/v2` (embedded broker, `client_connect_test.go:104`; v2.12.4 at the pin, v2.14.7 here). At the pin also testcontainers and `docker/go-connections/nat` in `test_client.go`, and testcontainers in `client_integration_test.go:14` and `integration_test.go:18`; none after task 3.7b |

Totals: 15 / 25,215 at the pin, of which 180 lines of `pkg/tlsutil` are not ported; 121 / 33,279 at the pin under the
four Q3 exclusions (measured before the rulings of 2026-10-03; the ported count is recorded in task 3.9); 7 live roots
(natsclient's nine re-measured as two live, task 3.7); 30 integration-tagged test files at the pin
(`natsclient` 29, `pkg/tlsutil` 1), 29 under the Q3 exclusions, 28 ported (`natsclient` 27 after
`kv_temporal_integration_test.go` leaves with dropped surface, `pkg/tlsutil` 1), measured over the pin tarball with
`grep -l '^//go:build.*integration' <package>/*_test.go` for each of the 15 package directories (`pkg/acme`, not
ported, has one more). `pkg/acme` and the two ACME
loaders are not ported (owner ruling, #9 comment 5950752741). In the floor only `pkg/tlsutil/tlsutil.go` imports
`pkg/acme` (`:12`), and only `LoadServerTLSConfigWithACME`, `LoadClientTLSConfigWithACME` and their helper
`initACMEClient` use it (`:186-365`); no other floor package imports it or calls them, and no `pkg/tlsutil` test does
(pin grep of the import path over the 16 package directories: `tlsutil.go` only; of `WithACME` outside `pkg/tlsutil`:
`input/websocket/websocket_input.go:821,1088`, `output/websocket/websocket.go:762`, `output/httppost/httppost.go:307`).
Cutting the loaders removes the floor's only edge to `pkg/acme`, so the floor is closure(natsclient) ∪ closure(message)
less `pkg/acme`: 15 packages. `pkg/cache` is in that closure only through `natsclient/kv_temporal.go:8`.
`TemporalResolver` is dropped as dead surface (owner ruling, #9 comment 5969522395, item 1); `internal/cache` stays
in the floor whole because it is admitted: six admitted packages import it at the pin (`component`,
`processor/graph-ingest`, `processor/graph-embedding`, `processor/rule`, `processor/rule/expression`,
`storage/objectstore`). In this change nothing imports it. The loaders, `pkg/acme`, `go-acme/lego/v4` and their two
defects (D7) move to change 5, which ports `output/websocket` (foundation D2 row 5); `input/websocket` and
`output/httppost` call them too and are in no change of the chain, so the change that ports them inherits the row (#9
comment 5950822861). `pkg/platform` and `pkg/security` have no tests at the pin; none are invented — their rows say so
and D10 does not gate them. Ported files land at their row's `destination` (D5). Not ported: `test_client.go`
(`adapt → natsfixture`) and `test_options.go` (`defer-exclude`); by owner ruling (#9, comment 5941920346, Q3), the three
test files that exercise them — `test_client_factory_test.go`, `test_client_integration_test.go` and
`test_client_readiness_test.go`
(`defer-exclude`). `monitoring_consumers_test.go` is also `defer-exclude`, an exclusion forced by Q3 and not an owner
ruling: it walks the SemStreams tree for `NewTestClient(…, WithMonitoring())` callers (`:13-27, :83, :111-115`),
`WithMonitoring` is declared at `test_client.go:448`, which is not ported, and the files it names lie outside the set
(`processor/graph-index/…`), so it has nothing left to check. `test_options_test.go` and `mapped_port_retry_test.go` are
also `defer-exclude`, forced by Q3: they test `test_options.go` and `test_client.go` internals. `typed_test.go`,
`kv_temporal_integration_test.go` and `TestTemporalResolver_ErrorBoundaries` leave with the surface they test (D8).

Three modules become direct test requirements: `stretchr/testify` (ruling b), `pgregory.net/rapid` v1.3.0 (the pin's
`go.mod:25`; owner ruling, PR #48 comment 5951926492), and `github.com/nats-io/nats-server/v2` at v2.14.7, the
`.nats-image` line (the pin requires v2.12.4, `go.mod:11`), imported only by `natsclient/client_connect_test.go`; where
its indirect requirements name a module `go.mod` already lists, the versions match, and `go mod tidy` adds six indirect
requirements `go.mod` did not list: `google/go-tpm` v0.9.8, `minio/highwayhash` v1.0.4, `nats-io/jwt/v2` v2.8.2,
`golang.org/x/time` v0.16.0, `antithesishq/antithesis-sdk-go` v0.8.0-default-no-op and `kylelemons/godebug` v1.1.0, and
drops `kr/text` (9cda760). In the 15 packages only `pkg/types/entity_id_prop_test.go` imports Rapid, and it is ported
with its property test (P23).

### D2. Harness API shapes (foundation D4, made concrete)

- **`Fixture.Restart(ctx) error`** (`natsfixture`). Takes the one-slot semaphore like `Start`/`Stop`; drains the
  fixture's own connection (dialled with `nats.MaxReconnects(0)`, so it is dead after a restart); runs two new
  phases, `PhaseStopContainer = "stop-container"` (`Container.Stop(ctx, nil)`) and `PhaseStartContainer =
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
  - Why the stop passes a nil timeout (`deps.go:68-70`): Docker's default stop grace and kill escalation apply. The
    call returns only after the container has stopped, so the grace period is Docker's wait before it escalates to a
    kill, not a timeout standing in for a join. A fixture constant there would be a hard-coded shutdown timeout,
    which the background-work rule forbids (#9 comment 5950482163).
  - Why readiness after a restart counts lines: testcontainers counts "Server is ready" lines across the container's
    whole log, every boot included (testcontainers `wait/log.go:210`). So `Restart` counts the ready lines before the
    start and waits for one more.
- **`natsfixture.FaultKV`**. `NewFaultKV(real jetstream.KeyValue) *FaultKV` embeds the real bucket and overrides
  `Put`, `Create`, `Update`, `Delete`; the matching `KVOp` values are `KVPut`, `KVCreate`, `KVUpdate` and `KVDelete`.
  `FailBefore(op KVOp, err error)` returns `err` without calling the real
  method; `FailAfter(op KVOp, err error)` calls the real method, discards its result, and returns `err` — the
  "server applied it, client saw an error" shape of #20. A fault is sticky: it applies to every call of that
  operation until cleared by passing a nil error. `PutString` goes through `Put`, so a `KVPut` fault covers it;
  `Purge` is never faulted. Change 2 extends `FaultKV` with a one-shot (or n-shot) fault if it needs "applied once,
  then the retry sees the result" for the `KVStore` compare-and-swap retry loop (pin `natsclient/kv.go:370-394`,
  #20); it is not built in this change. `Calls() map[KVOp]int` reports real calls made. It is typed
  on `jetstream.KeyValue`, never on `natsclient.KVStore`, so `natsfixture` keeps importing no ported package (P4).
  Its consumer in change 2 wraps it in `natsclient.NewKVStore` through an in-package test setter.
- **`internal/harness/prochost`**. `prochost.Helper(name string, fn func())` is called from the test binary's
  `TestHelperProcess`; when the environment marker `SEMENGINE_HELPER=<name>` is set it runs `fn` and exits, else it
  returns immediately so the test is a no-op in a normal run. `prochost.Start(t, name string, env ...string)
  (*Process, error)` runs `os.Args[0] -test.run=^TestHelperProcess$` with the marker, in its own process group, with
  stdout/stderr captured to the evidence directory. `Process` offers `Signal(os.Signal)`, `Pause()`/`Resume()`
  (SIGSTOP/SIGCONT), `Kill()`, `Wait(ctx) (ExitStatus, error)`, `Alive() bool`. `t.Cleanup` kills the group and
  waits under a fresh bounded context, so no helper survives its test. Reused from `runner_test.go`: the
  `exec.Command` start (not `CommandContext`: the helper outlives `Start`), SIGTERM and `Process.Kill` paths,
  `Setpgid` and group signalling. "No pid behind"
  uses the start-identity check of `runner_test.go` (`psStartIdentity` `:255`, `deadPID` `:293`), so a reused pid
  cannot pass as the helper; `pidAlive` (`:327`) alone is not enough. Pause is proven from observed state — `ps`
  reports the process stopped (state `T`) — never by waiting for a checkpoint that does not come; `prochost`'s
  non-test files are under the sleep check (`testtext_test.go:54-56`).

  Surface as built in task 2.4 (detail this text did not record; no behaviour change). `Start` takes
  `t testing.TB`. `Signal`, `Pause`, `Resume` and `Kill` return `error`. Once the helper has been reaped, `Kill`
  returns nil and the other three return an error; none sends a signal, because a reaped helper's process group id
  can be reused by another process. `ExitStatus` is `{Code int; Signal syscall.Signal}`; `Code` is -1 when a signal
  ended the process (and when the wait returned no process state). Output goes to
  `<SEMENGINE_EVIDENCE_DIR or t.TempDir()>/prochost/<test>-<name>-*.stdout` and a matching `.stderr`, with `/` and
  spaces in the test name replaced by `_`. There is no `Pid()` export.
- **`lifecycletest.Run(t *testing.T, factory Factory, mustFail Factory, promise Promise)`**. `mustFail` returns a
  fresh owner whose `Start(ctx)` must return a non-nil error; `Run` fails before any check, naming the argument, when
  `mustFail` is nil. New check `CheckFailedStartHoldsNothing(ctx, o Owner) error`: `Start` returns non-nil;
  `Observe().Unresolved` is empty; `Stop(ctx)` returns nil; `Observe().Calls` is unchanged by the `Stop`. No check
  counts a panic as a refusal: `call` (`lifecycletest.go:291-297`) turns a panic into an error today, so a panicking
  `Start(nil)` passes `CheckNilContextsRefused`; the probe found `Client` and `Watcher` on that path (PR #48 comment
  5942307713, "Suite gap"). `call` marks a recovered panic, and a check that expects a refusal fails on it. The two
  callers on the base change: `refowner_test.go:491-492` (the `refowner` double gains a must-fail construction mode
  and a `startFailsButHolds` failpoint, D8 note) and `natsfixture/fixture_integration_test.go:519` (`TestS1_7Restart`;
  the must-fail factory is a `Fixture` whose `deps.start` hook returns an error, so `Start` returns a `FixtureError`
  and `Observe` reports nothing held). No readiness accessor is added to `Owner` (#38).
- **Rehomed helpers**. `internal/harness/semantictest` keeps `EntityID(…)` and `Predicate(t testing.TB, …)` as at the
  pin (`fixtures.go:20,41`), importing `pkg/types` and `vocabulary`; `internal/harness/payloadfixture` keeps
  `NewForTest`, `NewWithSubset`, `RegisterTestType` (`testing.go:18,32,53`), importing `payloadregistry`. The bound
  for harness imports (foundation D4, N1): `natsfixture` imports no ported package; other harness packages may
  import ported packages that are pure libraries under the owner definition, after a cycle check over the helper's
  full dependency closure (P5).
- **The aggregator rule (T-B8) as implemented** (`harness-boundaries` › "Import graph";
  `internal/harness/contract/boundaries_test.go`, `aggregatorViolations`). Working definition:
  - A *component package* is a package of this module, outside `internal/harness/`, whose non-test files declare a
    top-level `func Register` or `func RegisterPayloads`. A `Register` from outside the module
    (`prometheus.Register`) does not count.
  - A non-main package outside `internal/harness/` whose non-test files refer to the `Register` or
    `RegisterPayloads` of two or more component packages is an *aggregator*, and the test fails naming it. "Refer"
    means a call or a use as a value (`var _ = alpha.Register` counts), not only a call.
  - A `package main` is the *composition root* (the consumer's `main`, where components are wired together) and is
    exempt. Aggregation inside `_test.go` files is allowed.

  Why `RegisterPayloads` counts: the requirement keeps both factories and payload registrations in the composition
  root only. At the pin, `payloadbuiltins.Register` chained `message.RegisterPayloads` and
  `objectstore.RegisterPayloads`, which is the shape the 03B Q1 ruling forbids ("explicit per-package registration in
  each consumer's composition root; no aggregator, no engine-side registry";
  `openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md`, D1, "Ruled (Q1, #3)"). The
  sensitivity test plants that shape and the rule rejects it.

  Open for change 3, to settle before the first component port:
  - (a) The requirement says "component family"; the scenario says "two component packages". The test uses
    packages, the stricter reading: two packages of one family aggregated outside `main` fail.
  - (b) A package that calls `alpha.NewFactory()` and adds the results to a registry, without referring to
    `Register`, is not caught.
  - A dot import (`import . "…/alpha"`) followed by a bare `Register` is not matched by T-B8, but `task lint` fails
    it first: the pinned `revive.toml` enables `dot-imports`.

### D3. The two services and their failing factories (foundation D5, D6)

In the items marked F21–F27 and `natsclient-close-reports-drain-timeout`, a `client.go`, `stream.go` or
`jetstream_metrics.go` cite is to `c69d7ac` unless it says otherwise.

The lifecycle suite applies to services only: types the engine starts, supervises and stops, with a `Start` that can
fail (ruling #9 comment 5950234192, item 1). In this set those are `metric.Server` and `natsclient.Client`. Each runs
the full suite through a test-side adapter in its own package (a `_test.go` file, so it reads unexported state) that
lists every retained kind, per the `lifecycle-suite` delta's `Observe` contract. A service's `Close` returns nil only
after the join; when its context ends first it returns `ctx.Err()` (`lifecycle-suite`, abort cause) and the join stays
pending.

| Service | Start / end at the pin | `Unresolved` lists | Failing factory |
|---|---|---|---|
| `metric.Server` | `Start(ctx)` `handler.go:59`, `Stop(ctx)` `:192` | the listener, the `http.Server`, the serve goroutine, the requests its handler admitted and has not returned from (since aa94acf; `Stop` waits for them within its context). Since a19f393 (Codex, PR #48 comment 5959412053, finding 1) `Stop` closes admission before `Shutdown`; a later request gets 503 with `Connection: close` and its handler never runs, and the closed flag shares the count's mutex, so a nil `Stop` is final (`metric/admission_test.go`) | a server configured on a port the test already holds with `net.Listen`; `Start` binds synchronously (`:55-110`) and returns the bind error |
| `natsclient.Client` | `Connect(ctx)` `client.go:471`, `Close(ctx)` `:578` | the `nats.Conn`, JetStream handle, subscriptions, internal consumer claims, the health monitor, the metrics poller, claim-release goroutines, the connection-loss and circuit-test timers, message handler invocations still running (after `Close` has cleared the connection), and connection event handlers still running | a client whose URL is a refused local port (its own `Connect` returns the dial error) |

The task 2.0 probe (PR #48 comment 5942307713) and a read of every `go` statement in both packages give these
failing-first `adapt` items:

- **`natsclient-nil-context-refused`.** `Close` (`client.go:578`) has no nil check, and on a connected client
  `Close(nil)` panics at `:685`. `Connect(nil)` dials at `:494` and then panics at `:495`, leaking the dialled
  connection. Both refuse a nil context with an error, before any dial or close. The pin already holds this contract
  for `Subscription.Drain` (`:796-799`).
- **`natsclient-connect-refuses-second-start`.** `connectWith` has no already-connected guard. It overwrites `m.conn`
  at `:547` without closing it, and starts a second metrics poller at `:566`. A second `Connect` on a connected
  client returns an error and changes nothing. A `Connect` that passed that check and then lost to a concurrent one,
  by failing, by having its context end, or by dialling successfully second, leaves the winner's status, failure
  count and circuit as they were, and closes what it dialled (`natsclient-status-ownership` below).
- **`natsclient-close-joins-its-goroutines`.** `Close` signals but never joins six sites where the client runs work on
  another goroutine, once task 3.7a has dropped the disconnect, reconnect and health-change callbacks with their launch
  sites: the metrics poller (`jetstream_metrics.go:345`, cancelled at `client.go:595-596`); the health monitor (`:1628`,
  whose `done` is closed at `:1678-1680` with no wait, and which runs `conn.RTT` and sets the status); the claim
  releases that wait on a consumer's `Closed()` (`stream.go:547, :646`); and two `time.AfterFunc` timers, the
  connection-loss watchdog (`client.go:1555-1567`, which runs `onConnectionLost`; `cancelConnectionLossTimer` at
  `:1572-1579` does not wait for a timer that has already fired) and the circuit test (`:289`, never stopped). `Connect`
  also races `Close`: it releases `closeMu` at `:553`, starts the monitor (`:560`) and the poller (`:566`), and writes
  `metricsCancel` with no lock that `Close` reads at `:595`. The fix, in one place:
  - Every site starts through `startBackground`; a subscription and the async publish error handler are counted through
    `admitLocked` and `admitAlways` (`client.go:223, :242, :269` at `c69d7ac`). The helper, under `m.mu`, refuses once a
    `closing` flag is set and otherwise adds one to an owned-work count before the goroutine starts; `Close` waits for
    that count to reach zero. The two timer bodies enter through the same check before doing anything. Nothing can be
    admitted once `Close` has begun waiting, so a callback, disconnect or timer that nats.go or the runtime delivers
    while `Close` runs (`handleDisconnect` arms the watchdog, `:1512`) cannot race the wait. `Connect` admits and starts
    the monitor and poller through the same helper, so a `Close` that has begun refuses them, and then returns
    `nats.ErrConnectionClosed`, not nil.
  - Arming either timer is refused, and logged, once `Close` has begun, so no timer is left pending after it.
    Arming the circuit timer stops a pending one from an earlier round, which nothing else would track or stop.
  - **`jetstream.New`'s error.** It is dropped at `:525`. `Connect` returns it and closes the dialled connection. No
    failing test exists (nats.go v1.54.0 `jetstream.go:471-492`), and the row records that.
  - Once `Close` has begun, the client starts no new work. Work offered after that point (a goroutine start, a timer
    body that fired, arming either timer, a `Connect`, `Subscribe`, `SubscribeForRequests` or consumer setup) is
    refused: background work is dropped, never run inline, and logged at debug level with its kind; a call returns
    `nats.ErrConnectionClosed`. Work admitted before `Close` began may still be running, or may only now enter its
    body, while `Close` waits for it. Message handlers keep running during `Close`'s drain, so messages already
    delivered to the client are handled. The pin dropped the watchdog's callback after close (`:1564`); the drop
    log is the contract's silent-drop rule, and the row records it as changed behaviour.
  - Only the first `Close` performs cleanup (stop the monitor, the timers and the poller; drain and close the
    connection; clear the connection and the JetStream handle), bounded by its own context. The pin's early
    `return nil` on a second call (`:583-585`) goes. See `natsclient-close-honours-each-context` for what every
    other caller observes.
  - A `Close` called from inside one of the client's callbacks waits for owned work that includes its own goroutine
    (an owned-work count, `client.go:146-160` at `c69d7ac`), so
    it returns `ctx.Err()` when its context ends and never returns nil, as `http.Server.Shutdown` does from inside a
    handler. `Close`'s doc comment and the row say so; at the pin such a call returned at once.
- **`natsclient-close-is-final`** (Codex F21, PR #48 comment 5980134911). `Close` returns nil only when everything
  the client owns has finished:
  - the background work of the six sites above (five call sites since 3.7c2: the two claim releases are one,
    `owned_delivery.go:133` at `c69d7ac`);
  - every invocation of a message handler passed to `Subscribe`, `SubscribeForRequests`,
    `ConsumeStreamWithConfig`, `ConsumeStreamWithConfigContexts` and `ConsumeInternalStreamWithConfig`;
  - the client's connection event handlers and its async publish error handler.

  One residual is declared. An invocation of the async publish error handler that starts after the join has completed
  runs unjoined: nats.go can still call it then, from the JetStream reply subscription's delivery goroutine caught
  mid-callback by a forced close, or from `resetPendingAcksOnReconnect` processing a queued status
  (`jetstream/publish.go:607-624`), and that goroutine has no end signal the client could wait for (nats.go v1.54.0).
  It is safe: once `Close` has begun the handler records no failure and changes no client state; it only counts the
  `publish_async` error metric and logs at debug level (`admitAlways`, `asyncPublishErrHandler`).

  This holds on every `Close` path: a drain that completes, a drain error, the drain timeout, the caller's context
  ending, and a connection the native library closed by itself. It does not rest on the native library's end
  signals alone, because nats.go v1.54.0 force-closes without waiting for a running callback (`nats.go:6196-6227`),
  finishes a drain after its timeout with a callback still running (`:6349-6372, :6389-6390`), and its consumer
  `Closed()` closes at once, and closes a channel handed out earlier, when the subscription is already invalid
  (`jetstream/pull.go:822-837`). The client observes the end of each handler invocation itself. A delivery that the
  native library hands over after the client has recorded that consumer's or subscription's end does not run the
  caller's handler. That can happen only when delivery was stopped without a drain (the caller's `Stop`, or a
  forced close of the connection) and a native `Closed()` reported early. The native library is discarding that
  consumer's buffered messages at the same moment (`nats.go:3801-3803, :3817-3828`; `jetstream/pull.go:58-61`).
  The refused message's fate depends on the ack policy. With `AckPolicy` explicit or all, it is left
  unacknowledged and the server redelivers it after its ack wait. With `AckPolicy: "none"` it is lost, like the
  buffered AckNone messages discarded beside it. A core NATS message is lost, as core NATS is at-most-once. At
  most one message per subscription or consumer is refused this way, because the native library delivers on one
  goroutine per subscription. Each refusal is logged at warn level with the subject, the stream and consumer
  where there is one, and the ack policy, and counted on the existing JetStream error metric as
  `recordError("late_delivery_refused")` (`jetstream_metrics.go:252-256`, the path `publish_async` already uses,
  `client.go:1534`) when the client has JetStream metrics configured; without them the warn log is the only
  signal. That label also counts refusals on core subscriptions, although the metric's help text names JetStream
  operations. The owner accepted that AckNone loss
  (question 4, #9 comment 5980769459).

  Work a call starts belongs to that call until it returns and to the client afterwards: a `Connect` that returns an
  error has no event handler of the connection it dialled still running, so a `Connect` still in flight when `Close`
  returns (a valid ordering) does not leave work behind either (owner ruling 2, #9 comment 5980296112). That wait takes
  no context and is still background-work shape 2 (ruling #9 comment 5950482163), not shape 3: the only callbacks on the
  candidate's native dispatcher are the client's own event handlers (`client.go:639-642`), which wait on nothing outside
  the client, and no caller callback is ever queued there (the connection's `ClosedHandler` is the last callback its
  dispatcher runs, nats.go v1.54.0 `nats.go:6154-6160, :6236-6252, :3637-3660`).
- **Connections installed or replaced through `SetConnection`.** `Close` joins the event handlers of exactly one
  connection: the one it drains and closes, when this client dialled it (only a dialled connection carries the
  client's handlers, `buildConnectionOptions`, `client.go:632-656`). A connection installed through `SetConnection` is
  drained and closed like any installed connection, with no handler join. A connection that `SetConnection` replaced,
  dialled by this client or not, belongs to whoever called `SetConnection`: `Close` neither closes it nor waits for
  its `ClosedHandler`, so `Close` cannot hang on it. Reasons: `SetConnection` is a test hook ("for testing",
  `client.go:408`) whose only caller swaps a connection out and restores it in cleanup
  (`request_response_bounds_integration_test.go:167, :189`), so closing the replaced connection would break the caller
  that owns it; and the client's handlers on a replaced connection change nothing, because each state-changing handler
  checks that its connection is the installed one first (`isCurrentConn` at `7ce1940`, `client.go:1593-1597`;
  `ownsStatusLocked`, `client.go:1964`, since 3.7c2); `handleError` only logs. Client-owned subscriptions are different:
  they were created by `Subscribe` or `SubscribeForRequests` and belong to the client wherever their connection went.
  `Close` unsubscribes one left on a replaced connection and joins its running handler invocation (nats.go v1.54.0
  `nats.go:3826-3834`). An error from that `Unsubscribe` (the replaced connection already closed or draining, or the
  subscription already ended) means the subscription is
  already ending: `Close` neither fails nor returns early on it, and still waits for the
  subscription's end, bounded by its own context. The client keeps no catalog of its subscriptions, so the carried
  guard `TestClientHasNoChildLifecycleSurfaceOrCatalog` stands: each client-owned subscription has its own admitted
  watcher, which unsubscribes it when `Close` begins if its connection is not the one `Close` drains. Declared cost:
  messages buffered on that subscription and not yet handed to the handler are discarded by the native library on
  unsubscribe (nats.go v1.54.0 `nats.go:3801-3803, :3817-3828`, `jetstream/pull.go:58-61`), as they are when any core
  subscription is unsubscribed. Once `Close` has begun, `SetConnection` changes nothing and logs at warn level (owner
  ruling 3, #9 comment 5980296112). Client-created consumers are the client's in the same way (Codex F26, PR #48 comment
  5981562076): when `Close` begins, the ownership goroutine of a consumer whose connection
  (the one its JetStream handle was made on, read together with that handle when the setup began) is not the one
  `Close` drains stops it through its native handle, leaving
  that connection open. After `Stop` the native `Closed()` can report the end early, so the claim and the metrics
  observation are released only once the handler count reaches zero. A consumer on the drained connection keeps its
  graceful drain. Declared cost: messages that consumer had buffered are discarded by `Stop`
  (`jetstream/pull.go:58-61`); with acknowledgements they are redelivered, with AckNone they are lost, as the owner
  accepted for a cut-short delivery (question 4).
- **Consumer handle `Closed()`** (owner ruling 1, #9 comment 5980296112). nats.go v1.54.0 closes a consume
  handle's `Closed()` at once for an invalid subscription (`jetstream/pull.go:822-837`), so the three Consume doc
  comments say that a nil `Client.Close`, not `Closed()`, proves no handler runs. The handle is not wrapped; the
  exact-native-handle contract (`stream_handle_test.go:87`) stays. Tracked by #83, with an upstream nats.go report.
- **Consumer setup that meets `Close`.** The boundary is admission, which comes just before native `Consume`. A
  consumer setup not yet admitted when `Close` begins is refused with `nats.ErrConnectionClosed`, its claim released,
  and the native `Consume` is never called. One admitted before `Close` began may still call native `Consume` after
  it; it is owned all the same, and once native delivery has started it stops it and keeps its local claim and its
  metrics observation until every handler invocation has returned; it then returns `nats.ErrConnectionClosed` with no
  handle. If its setup context ends first, it returns that context's error at that point; the claim stays held until
  the handlers return, and `Close` does not return nil before they have. A caller that is refused has nothing to
  drain.
- **`natsclient-close-honours-each-context`** (Codex F22). Every `Close` returns within its own context. A `Close`
  whose context has ended, or ends while another `Close` is draining, returns `ctx.Err()` at once, without waiting
  for that drain. Cleanup runs once, bounded by the first `Close`'s context; a later `Close` with a longer context
  does not extend the drain, and returns nil only once the cleanup and `natsclient-close-is-final` are complete. A
  `Close` that has observed an ended context returns `ctx.Err()` even if the join has also finished (unchanged):
  when both are ready a `select` picks at random, and a nil return under an ended context is what the suite's abort
  check rejects. A `Connect` made while cleanup runs returns `nats.ErrConnectionClosed` without waiting for it.
- **`natsclient-status-ownership`** (Codex F23). `Status()` reports the installed connection and nothing else:
  - While a connection is installed and `Close` has not begun, a `Connect` that does not install its own
    connection changes neither `Status()`, nor `Failures()`, nor the circuit.
  - A `Connect` that fails while no connection is installed records its failure and writes `Disconnected`, or
    opens the circuit, as at the pin.
  - Once `Close` has begun, nothing but `Close` changes `Status()`: not the health monitor, not the connection
    event handlers, not failure accounting from operations still in flight, not a `Connect`. The status is frozen
    at whatever it was when `Close` began, so a handler still running during the drain can publish, read KV and
    settle messages as before (those calls require `Connected`: `client.go:1420, :1481, :1567, :1705, :1751, :1807,
    :1886, :1913`, `stream.go:163, :473, :743, :1020`). `Close` writes `Disconnected` once its cleanup has finished,
    as at the pin (`:578-640`, after the drain; `client.go:989` at `c69d7ac`), and from then on `Status()` stays
    `Disconnected` for the life of the client. Declared cost: if the server connection drops during the drain,
    `Status()` keeps the value it had when `Close` began until the cleanup ends, which is bounded by the drain timeout
    or the first `Close`'s context.
  - Refusing new work once `Close` has begun depends on `Close` having begun, not on `Status()`: `Subscribe`,
    `SubscribeForRequests`, the three consumer APIs and `Connect` return `nats.ErrConnectionClosed` before any
    status gate can return `ErrNotConnected`: the closing checks `stream.go:467, :737` and `client.go:1291`
    (`subscribeOwned`) come ahead of the gates `stream.go:473, :743` and `client.go:1296`.
  - A `Connect` after `Close` refuses before it dials and writes no status.
- **`natsclient-close-reports-drain-timeout`** (review finding F-3). A drain that ran out of time is reported the
  same way whichever timer ran out first, the native drain's or the client's own (they have the same length,
  `client.go:638, :1094-1098`): the first `Close` returns a transient error wrapping `nats.ErrDrainTimeout`. At the pin
  and at `7ce1940` the native timer usually won and `Close` returned nil.
- **`metric-abort-stop-reports-context`.** On an idle server `Shutdown(ctx)` (`handler.go:215`) returns nil even when
  `ctx` has ended, and the `select` at `:222-228` then picks between `serveDone` and `ctx.Done()` at random, so
  `Stop` drops the caller's cause about once in 1,000 runs at `-cpu 1`. `Stop` reports `ctx.Err()` whenever its
  context has ended.
- **`metric-forced-join-without-timer`.** After force-closing the server and listener, `Stop` waits on the serve
  goroutine (`handler.go:180`) under a fixed one-second bound (`forcedServeJoinTimeout`, `:23, :243-249`). `Serve`
  returns once its listener is closed, which `Stop` has just done, so `Stop` waits on `serveDone` with no timer.

The `metric` row becomes `adapt`. Goroutines in these packages that end before the call that started them returns are
not background work: `metric/registry.go:76` (drained by the range at `:79-81`) and
`natsclient/delivery_settlement.go:349` (joined on every exit, `:373, :379, :383`). The `transport-client` delta
states the `Client` nil-context requirement; the row records each item as changed behaviour, not carried.

**Generated checks for the `Client` lifecycle** (`docs/testing.md`, "Decide whether generated checks are needed").
The `Client` lifecycle is a history: what `Close`, `Connect`, `Status()` and a consumer setup report depends on the
order of `Connect`, `Close`, timer firings, native callbacks, consumer setup and context cancellation. It gets named
examples, not a generator, for two reasons.

First, the outcomes that matter depend on how operations interleave, not on which operations run in what order. A
generator draws operation sequences, and its seed replays those; it cannot draw or replay goroutine interleavings
against a real NATS connection, and `synctest` cannot host the embedded server's sockets. A generated test would
therefore sample interleavings by luck and could not replay a failure, which `docs/testing.md` requires of a
generated run. Each interleaving that matters is a point where ownership of work or of the status passes between a
call and the client: admission of background work, the start of native delivery, a handler entering and returning,
a native end signal, a status commit, and the two points of `Close` (it begins; it finishes cleanup). The examples
force each such point against each `Close` point at a named seam (`opHook`, a held logger record, a held handler, a
fake native consumer, or a private hook at the commit), so each ordering runs every time, deterministically. The
mapping (task 3.7c2 tests by name; the 3.7c tests likewise):

| Ownership point | Against `Close` beginning | Against `Close` finishing cleanup |
|---|---|---|
| Background work admitted | `TestClientCloseDropsLateDisconnect`, `TestClientCloseDuringConnectStartsNothing` | `TestClientCloseJoinsConnectionLossCallback` |
| Native delivery started (consumer setup) | `TestClientRefusedConsumerKeepsOwnershipUntilHandlersReturn`, `TestClientRefusedConsumerSetupContextEndsWhileHandlerRuns` | `TestClientLifecycleOperationTable` (closed-state rows) |
| Handler invocation enters and returns | `TestClientCloseJoinsSubscribeHandlerAfterForcedClose`, `TestClientCloseJoinsConsumerHandlerWhenClosedReportsEarly`, `TestClientLifecycleAdapterListsHeldHandler` | `TestClientCloseJoinsSubscribeHandlerAfterForcedClose`, `TestClientCloseJoinsConsumerHandlerWhenClosedReportsEarly` |
| Native end signal (accurate, or early) | `TestClientCloseJoinsSubscribeHandlerAfterForcedClose` (accurate), `TestClientCloseJoinsConsumerHandlerWhenClosedReportsEarly` and `TestClientRefusesLateDeliveryAfterRecordedEnd` (early) | `TestClientRefusesLateDeliveryAfterRecordedEnd` |
| Status commit | `TestClientHealthMonitorCannotOverwriteClosedStatus`, `TestClientFailuresAfterCloseLeaveStatusDisconnected`, `TestClientEventHandlerCannotCommitAfterClose`, `TestClientAsyncPublishErrorAfterCloseRecordsMetricOnly` | `TestClientHealthMonitorCannotOverwriteClosedStatus`, `TestClientConnectAfterCloseRefusesBeforeDial`, `TestClientLifecycleOperationTable` |
| Status commit against a winner's install | `TestClientLosingConnectLeavesWinnerStatus` (four cases) | not applicable |
| Event handler and async publish error handler | `TestClientCloseJoinsConnectionEventHandlers`, `TestClientCloseJoinsRunningAsyncPublishErrorHandler` | `TestClientCloseJoinsConnectionEventHandlers`, `TestClientLosingConnectLeavesNoCandidateHandler`, `TestClientCloseJoinsRunningAsyncPublishErrorHandler` |
| Drain outcome | `TestClientCloseReportsDrainTimeout` | `TestClientCloseReportsDrainTimeout` |
| A second `Close`, or a `Connect`, during the drain | `TestClientCloseHonoursItsContextDuringAnotherDrain`, `TestClientConnectDuringCloseDrainReturnsPromptly`, `TestClientConcurrentClosesEachHonourTheirContext` | `TestClientCloseHonoursItsContextDuringAnotherDrain` |
| Subscription on a replaced connection | `TestClientCloseEndsSubscriptionOnReplacedConnection` | `TestClientCloseEndsSubscriptionOnReplacedConnection` |
| Consumer on a replaced connection (the `transport-client` scenario "A consumer on a replaced connection") | `TestClientCloseEndsConsumerOnReplacedConnection`, `TestClientCloseAttributesConsumerToItsHandlesConnection` | `TestClientCloseEndsConsumerOnReplacedConnection`, `TestClientCloseAttributesConsumerToItsHandlesConnection` |

Second, the sequential part (which operation is allowed in which lifecycle state) is small and finite: four states
(new, connected, closing with a held drain, closed) times eight operations (`Connect`, `Close` with a live context,
`Close` with an ended context, `Subscribe`, `SubscribeForRequests`, `ConsumeStreamWithConfig`,
`ConsumeInternalStreamWithConfig`, `SetConnection`). The third consumer entry point,
`ConsumeStreamWithConfigContexts`, runs the same function as `ConsumeStreamWithConfig`
(`consumePortStreamWithConfigContexts`, `stream.go:431-441, :698-709` at `c69d7ac`) and differs only in taking a
separate handler context, so the table covers it through `ConsumeStreamWithConfig`. `TestClientLifecycleOperationTable`
enumerates all 32 pairs. The expected return and expected `Status()` for each pair are written out in the test from the
guarantees in D3, never derived by calling production code. Exhaustive enumeration covers every pair; a generator
would only sample them.

Repetition: the lifecycle tests run under `go test -race -count=20 -run 'Lifecycle|Close|Connect|Subscribe|Consume'`
and in `task test:repeat`'s shuffled runs. What the examples do not cover: three-way orderings (two `Close` callers
and a `Connect` racing one held drain) beyond the cases listed, and orderings inside nats.go itself, which the nats.go
v1.54.0 cites in this section stand in for.

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

- Fifteen package rows, `source_path` = package directory, `source_sha` = `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`,
  `destination` from the table below, `proving_tests` naming the carried tests and the suite run, `known_risks`
  carrying the context-root triage (a legitimate root with its reason, or a defect) and the nats.go v1.52→v1.54 pin
  difference for `natsclient`.

  Destinations. The standing rule (owner ruling, #9 comment 5953295358, refining 5952661571): a ported package is
  public if a starter consumer imports it, or if an exported signature in a public package names one of its types;
  a public API never names a type that a caller outside the module cannot construct. `TestPublicSignatures`
  enforces it (owner ruling, #9 comment 5953477174; D6, task 2.10). Public packages keep their pin paths. Every
  other package moves from `pkg/<name>` to `internal/<name>`, with no exception. The compiler then forbids imports
  of it from outside the module. No package name changes, so a port rewrites import paths only. A `file:line`
  cite elsewhere in this design is a pin path; the row's `source_path` → `destination` maps it, and #52's command
  reads that map.

  Applied to the floor: eleven public, four internal. The eight SemSource imports directly at `e4febc0d` (§5.1, P16) and
  `pkg/projection/contract`, which SemConnect imports (P21), are public by import; `pkg/security` is public because a
  public signature names its `Config` type (P22), and `pkg/platform` because `config`'s exported `Config` names its
  `Config` (change 3), a caller ported later: the signatures that named it in this change are removed (task 3.6b).
  `pkg/cache` stays internal. The one public symbol that named its type at the pin,
  `natsclient.TemporalResolver.GetStats` (`kv_temporal.go:221`), leaves with `TemporalResolver`, which is dropped as
  dead surface (task 3.7). After that no exported symbol of the eleven names a type from the four (P22).

  | Pin path (`source_path`) | `destination` | Why |
  |---|---|---|
  | `natsclient` | `natsclient` | public: SemSource imports it |
  | `metric` | `metric` | public: SemSource imports it |
  | `payloadregistry` | `payloadregistry` | public: SemSource imports it (`run.go`) |
  | `message` | `message` | public: SemSource imports it (28 files) |
  | `vocabulary` | `vocabulary` | public: SemSource imports it (20 files) |
  | `pkg/types` | `pkg/types` | public: SemSource imports it (5 files) |
  | `pkg/retry` | `pkg/retry` | public: SemSource imports it (4 files) |
  | `pkg/errs` | `pkg/errs` | public: SemSource imports it (3 files) |
  | `pkg/projection/contract` | `pkg/projection/contract` | public: SemConnect imports it (`gateway/cs-api/payloads.go:11`) |
  | `pkg/platform` | `pkg/platform` | public: nothing in this change reads it, since task 3.6b removed the seven `message` federation symbols and `vocabulary.EntityIRI`; its reader is `config` (change 3: `config/config.go:29`, `:49`, `:226-244`; `manager.go:88-101`), surface whose caller is ported later |
  | `pkg/security` | `pkg/security` | public: `security.Config` is named by `metric.NewServer` |
  | `pkg/resource` | `internal/resource` | no consumer import; no public signature names its types |
  | `pkg/timestamp` | `internal/timestamp` | no consumer import; no public signature names its types |
  | `pkg/tlsutil` | `internal/tlsutil` | no consumer import; no public signature names its types |
  | `pkg/cache` | `internal/cache` | no consumer import; no public signature names its types (the pin's one, `TemporalResolver.GetStats`, is dropped with its type) |

  Every importer of the four is inside the module (P22): `internal/resource` ← `natsclient`; `internal/cache` — none in
  this change (its pin importers arrive with change 2 and later); `internal/tlsutil` ← `metric`; `internal/timestamp` ←
  `message`. The harness helpers (D2) import public packages only. Dispositions, by owner ruling (#9, comment
  5941920346, Q1: a repaired test file makes the row `adapt`): `adapt` for `natsclient` (the `NewTestClient` sites,
  `test_client.go`, the D3 items, the D8 repairs, the surface audit's drops, the census rewrite, the D9 gauges),
  `metric` (D3 items), `payloadregistry` (`testing.go` rehomed), `pkg/cache` and `pkg/resource` (the D7 items, the D8
  repairs), `pkg/tlsutil` (the ACME loaders cut, D1) and `pkg/retry` (D8 repair); and, by owner ruling (#9, comment
  5957221949, which replaced comment 5955265930: a ported `README.md` keeps the pin's text except for the edits
  markdownlint requires and edits to passages that describe behavior the ported code no longer has, each behavior edit
  listed by README line as an `adapt` item; and comment 5968665464: import paths and module references rewritten to
  SemEngine, each an `adapt` item), `vocabulary` and `pkg/types`, whose READMEs need lint fixes (358a01e);
  `pkg/platform` (a doc comment corrected, #72 ruling D, task 3.6b); `message` (task 3.6b and its row: README import
  paths and the decode path corrected, the federation family removed as dead surface (#72 comment 5969293525),
  timestamps decoded strictly as integer milliseconds, and invalid UTF-8 refused in the source, the type and a generic
  payload, owner rulings #9 comments 5969522395 and 5969776736 and #9 comment 5970334875); `carry` for the other
  four: `pkg/security`, `pkg/timestamp`, `pkg/errs` and `pkg/projection/contract`.
- Existing file rows updated: `natsclient/test_client.go` (`adapt`, now with its evidence in `proving_tests`),
  `natsclient/test_options.go` (`defer-exclude`, honoured); new `defer-exclude` file rows
  `natsclient/test_options_test.go` and `natsclient/mapped_port_retry_test.go` (forced by Q3); a new file row for
  `payloadregistry/testing.go` is not needed — the package row records the rehoming (T-B7 keeps `source_path` unique;
  the package row's path is the directory).
- Eight `defer-exclude` rows for the D4 ten-out packages that are never carried: `agentic`, `agentic/agentrun`,
  `gateway`, `gateway/graph-gateway`, `internal/agentterminal`, `internal/deliverylane`, `internal/looptoken`,
  `vocabulary/agentic`. `graph/llm` and `model/wire` get carried-dormant rows in change 2.
- Port-refactor rows: none are performed here. Later callers of what this change alters are named in the owning
  row's `known_risks` and get their own `class:port-refactor` rows when their package is ported: `CoalescingSet.Close`
  becomes `Shutdown(ctx)` (`processor/graph-embedding/component.go:875`, `processor/rule/entity_watcher.go:980`);
  `cache.WithEvictionCallback` is deleted (`processor/rule/expression/regex_cache.go:21`, a no-op callback).
  `resource.Watcher`'s `StartBackgroundCheck` and `Stop` have no caller outside
  its own tests at the pin; its `WaitForStartup` callers are unaffected (`natsclient/client.go:1407`,
  `processor/graph-clustering/component.go:1337`, `processor/graph-index/component.go:1083`,
  `processor/rule/entity_watcher.go:119`, `processor/graph-index-temporal/component.go:468`,
  `processor/graph-index-spatial/component.go:456`, `processor/graph-embedding/component.go:1209`).
- The Tier-1 cross-check re-measured on the ruled set (#9 item 1) is an inventory the architect writes and the
  technical writer records next to the ledger; it does not change a row.
- The identity documents ride with `pkg/types`: ADR-102, ADR-104, `docs/concepts/16-federation.md` and
  `docs/specs/entity-id-contract.md` (a reference contract, not `openspec/specs/`; #9 comments 5969510651,
  5970020197; task 3.3a).

### D6. Gates this change adds or extends

`scripts/cover-check.sh` gains targets `natsclient`, `message`, `payloadregistry` at 80% (D10; baseline unmeasured —
the first measurement is taken when the package lands, P8). The I8 test lands in `internal/harness/contract`
alongside the T-B8 aggregator rule with a tree-shape sensitivity test like `TestImportGraphSensitivity`. D5's
public-signature rule is enforced by `TestPublicSignatures` in the same package (task 2.10; `harness-boundaries` ›
"Public signatures name no internal type"), which loads the module with `golang.org/x/tools/go/packages` as
`TestNoRetainedContext` already does, so no dependency is added. Package-doc
lint is already on (revive `package-comments`, `revive.toml:22`) and now covers eleven public packages; every package
in the set has a package comment at the pin (§3.3), so the task is a sensitivity check, not an enablement. The
compiled example consumer (D16's other gate) composes `service` and is change 3's; it will import at least the
eight that SemSource imports. No context-root guard is added (D9). `TestNoBareSelect` (`testtext_test.go:205`)
enforces `harness-boundaries` › 'No bare select', with its sensitivity test (task 2.11).

### D7. Background work outside services takes one of three shapes

Background work is a goroutine that outlives the call that started it, in code that is not a service; services stay
under the lifecycle suite (D3). Background work takes one of the three shapes of the `background-work` delta
(ruling #9 comments 5950234192 and 5950482163): `Run(ctx) error`, preferred; `Close() error`, or a stop function the starting
call returns, that cancels and joins with no timeout, for a goroutine that waits only on what the stop controls; or
`Shutdown(ctx) error`, for shutdown that waits on work it does not control. No shape uses a fixed shutdown timeout.
Each has a `synctest` test proving nothing is left behind, and refuses a nil context at the call: with an error where
the entry returns one, otherwise by a panic at the call before anything starts.

Every `go` statement in the 15 packages at the pin outside the two services (the cut ACME loaders aside, D1):

| Site | At the pin | Shape and item |
|---|---|---|
| `pkg/resource.Watcher` | `StartBackgroundCheck(ctx)` (`watcher.go:141-154`) starts `go w.backgroundLoop(ctx)` (`:153`); `Stop()` waits on `wg.Wait()` with no bound (`:218-227`); the loop calls the caller's check function (`:183`) | Shape 1: `Run(ctx) error` is the loop itself, returns when `ctx` ends, refuses nil with an error, and refuses a second `Run` while one is in progress with an error at the call (carried from the pin's "already running" guard, `watcher.go:145-147`). `StartBackgroundCheck`, `Stop`, and the `cancel` and `wg` fields are removed, with their doc references (`watcher.go:108, :138-140`; `doc.go:21, :57, :66, :85, :149, :151, :159`) |
| TTL cache | `go c.cleanup(ctx)` (`ttl.go:73`); `Close()` (`:249-263`) waits on `c.done` or a fixed `time.After(5 * time.Second)` (`:258-262`); a nil context panics in the goroutine (`:276`) | Shape 2: `Close() error` closes `shutdown` and waits on `c.done`, with the fixed wait removed; `cache.NewTTL` refuses nil with an error |
| Hybrid cache | `go c.cleanup(ctx)` (`hybrid.go:79`); `Close()` (`:276-292`) has the same fixed 5 s wait (`:289`) | Shape 2, as the TTL cache; `cache.NewFromConfig` refuses nil with an error |
| `pkg/cache.CoalescingSet` | `go c.run(ctx)` (`coalescing_set.go:45`); each tick calls `fireBatch` (`:142`), which calls the caller's callback outside the lock (`:163-175`); `Close()` waits on `<-c.done` with no bound (`:116-126`); a nil context panics in the goroutine (`:136`) | Shape 3: `Shutdown(ctx) error` replaces `Close()`; `NewCoalescingSet` panics at the call on nil |

Why these shapes, at the pin:

- **`Cache.Close() error` (`cache.go:48`) does not change.** Shape 2's `Close` already returns `error`; only the fixed
  wait inside the TTL and hybrid implementations goes. That holds only if the cleanup goroutine waits on nothing
  `Close` does not control. At the pin it does: `removeExpired` calls the eviction callback from the cleanup
  goroutine (`ttl.go:302-304`, `hybrid.go:366-369`). The three production eviction callbacks at the pin are empty
  (`natsclient/kv_temporal.go:27,59`; `processor/rule/expression/regex_cache.go:21`), so `WithEvictionCallback`
  (`options.go:44`), the `EvictCallback` type (`cache.go:51-53`) and the eviction plumbing in the four
  implementations that carry it (simple, LRU, TTL, hybrid) are deleted (owner ruling, #9 comment 5950725772), which
  leaves shape 2 true and the interface unchanged.
- **`CoalescingSet` takes shape 3, not a cancelled callback context.** Its goroutine runs the caller's callback, which
  is the user callback the shape-3 rule names. Handing the callback a context that `Close` cancels would change the
  callback's signature for its two later callers and still hang `Close` on a callback that ignores the context;
  `Shutdown(ctx)` bounds the wait by the caller's context whatever the callback does, with one signature change.
- **`Watcher` takes shape 1.** Its loop calls a caller's check function, and its start and stop have no caller to
  keep. `Run(ctx)` removes the goroutine, the cancel and the wait group from the type.

**Generated checks for `pkg/cache` (task 3.6; `docs/testing.md`, "Decide whether generated checks are needed").**
`CoalescingSet` has an order-dependent history: what a window delivers depends on every `Add`, `Remove`,
`RemovePrefix` and `Drain` before it and on whether `Shutdown` came first. Examples cover a few orders, so it gets a
generated check: `TestPropCoalescingSetHistory` (`internal/cache/coalescing_set_prop_test.go`) draws a history with
Rapid and runs it in its own `synctest` bubble (Rapid calls `t.Deadline`, which a bubble refuses), comparing each step
with a test-owned model: each method's answer, `PendingCount`, one batch per window holding exactly the pending keys,
nothing delivered after `Shutdown`. Examples still own a blocked or panicking callback, `Shutdown` under an ended
context, concurrent callers (the model is sequential), the zero window, the nil refusals, and a key holding the prefix
past its start (`TestCoalescingSetRemovePrefixMatchesOnlyAtTheStart`), which the generator reaches only by chance at
100 checks. The TTL and hybrid caches' close is not a history worth generating: `Close` signals one goroutine and
waits for it, and its cases (first close, repeated close, close after the goroutine ended through its context) are
named examples; the carried cache semantics do not change in this change.
`Config.UnmarshalJSON` decodes outside bytes (task 3.6a, Codex F5), so it gets a native fuzz target,
`FuzzConfigUnmarshalJSON` (`internal/cache/config_fuzz_test.go`): no panic; an accepted document holds only `Config`'s
keys; each duration equals what the test derives with the standard library (`time.ParseDuration` of a string,
`time.Duration` of an integer, zero when absent); an accepted `Config` survives `json.Marshal` and a second decode.
Its seeds cover each accepted and refused form, and its JSON `null` seed found a pin panic. Named examples own the
unknown-key and `stats_interval` refusals and the `null` case.

**Generated checks for `message` (task 3.6b, Codex F8).** `Decoder.Decode` decodes outside bytes: a message
envelope (`id`, `type`, `payload`, `meta`) whose payload is decoded by the type the registry holds for the envelope's
`type`, and whose two timestamps are integer milliseconds (owner ruling 7, #9 comment 5969522395). The cases interact
(malformed JSON, a field of the wrong JSON type, an unregistered type, a registered type that is not a `Payload`, a
payload that does not fit its type, loose `meta` values, a timestamp that is not an integer), so it gets native fuzz
targets in `message/decoder_fuzz_test.go`, each run through `NewDecoder` with a registry holding `core.json.v1`
(`RegisterPayloads`), a test payload with a typed field and a schema-less stub (`payloadfixture.RegisterTestType`).
`FuzzDecoderDecode` decodes the same bytes with the standard library into a test-owned copy of the documented envelope
and predicts acceptance in both directions: accepted exactly when the envelope decodes, its type is registered as a
`Payload`, the payload decodes into that type's fields, and each timestamp is absent, `null`, or an integer literal
(RFC 8259's number grammar without fraction or exponent) in `int64`'s range, checked with `math/big`. So a decoder
that refuses everything fails on the accepted seeds, and one that accepts too much fails on the refused ones. On
acceptance it checks the ID, the type, the payload's fields, `meta.source` (empty unless a string) and each timestamp
as exactly that many milliseconds, 0 being the zero time; a message that validates survives `json.Marshal` and a
second `Decode` equal in full (ID, type, payload, source, both timestamps). `FuzzDecoderRoundTrip` builds a message
through `NewBaseMessage` and `NewDefaultMetaWithReceivedAt` from a fuzzed source, two `int64` millisecond timestamps
and a count, and asserts `decode(marshal(m)) == m` in full; its seeds sit on 0, ±1, 10^12 − 1 and 10^12 (the
seconds/milliseconds switch of the pin's `timestamp.Parse`), pre-1970 instants and both ends of `int64`, so every
boundary is reached by construction. `MarshalJSON` refuses a source that is not valid UTF-8 with an invalid-data error,
where `encoding/json` would write each invalid byte as U+FFFD (owner ruling 2, #9 comment 5969776736); the target
asserts that refusal, judged by `unicode/utf8`, and the full equality for every valid source, with no exception. The
owner extended that ruling to every string `message` encodes (#9 comment 5970334875): `Type.Validate` and
`MarshalJSON` refuse a type component that is not valid UTF-8, and `GenericJSONPayload.MarshalJSON` such a string, key
or value at any depth. `GenericJSONPayload.Data` holds JSON-shaped values only (#9 comments 5972117486 and
5972208367): `MarshalJSON` checks `Data` by exact type before encoding and refuses any other value with its type and
path, and a map or list that contains itself. `FuzzGenericJSONShapes` builds nested maps and lists of every ruled kind
and three refused ones, and asserts refusal, judged from what the generator built, or a round trip equal at each
number's literal. `FuzzDecoderStrings` generates the three type components and a generic key and value and asserts,
judged by `unicode/utf8` and `strings`, that `Type.Validate`, the envelope and the payload refuse exactly those inputs,
and that every accepted one survives `NewDecoder` equal in full; a type registered per input carries the generated
components through the registry, and its seeds hold an invalid byte in each of the five positions.
`GenericJSONPayload.UnmarshalJSON` gets `FuzzGenericJSONPayloadUnmarshalJSON`, checked against the standard library's
decode of `{"data": …}`. Named examples own the four ruled instants (`TestBaseMessageTimestampsAreMilliseconds`) and the
refused timestamp forms (`TestBaseMessageRefusesTimestampsThatAreNotMilliseconds`). The entity-ID helpers keep the
qualification of their canonical authority in `pkg/types`.

The `message-codec` delta states these laws (owner ruling, #9 comment 5983188211). Its scenarios map to existing
tests: millisecond timestamps → `TestBaseMessageTimestampsAreMilliseconds` (an instant before 2001) and
`TestBaseMessageRefusesTimestampsThatAreNotMilliseconds` (refused forms; absent, `null` and 0), with
`FuzzDecoderDecode` over the timestamp grammar; UTF-8 → `TestBaseMessageRefusesSourceThatIsNotUTF8`,
`TestBaseMessageRefusesTypeThatIsNotUTF8` and `TestGenericJSONRefusesInvalidUTF8AtDepth`, with `FuzzDecoderStrings`;
JSON shape → `TestGenericJSONRefusesValuesThatAreNotJSONShaped`, `TestGenericJSONRefusesCycles` and
`TestGenericJSONAcceptsJSONShapedValues`, with `FuzzGenericJSONShapes`; round trip → `FuzzDecoderRoundTrip`.

The ACME loaders' renewal goroutines (`tlsutil.go:253, :331`) are not in this change (D1): their stop can wait inside
`legoClient.Certificate.Renew`, which takes no context (`pkg/acme/client.go:352`), and their renewal callback writes
`tlsConfig.Certificates` while the config may be serving handshakes (`tlsutil.go:258, :336`). Both defects travel with
the loaders to change 5 and are recorded on the `pkg/tlsutil` row.

### D8. Ported tests land repaired

Every ported `_test.go` file lands meeting the `harness-boundaries` requirements "No sleeps in tests" and "No skipped
or hidden tests", and the `merge-gate` requirement "Varied and repeated unit runs". There is no list of accepted
files. A row with a repaired test file is `adapt` (Q1 ruling), and the row's `proving_tests` names each repair as
pin `file:line` → SemEngine `file:line`. The repairs at the pin (P18, P19) fall into four classes:

- **R1. Sleep → wait on a signal.** Timers in a repaired file fall into three classes:
  - **R1a. Fake clock.** Code whose timing comes from its own timers (`CoalescingSet`, the `pkg/cache` TTL caches,
    `resource.Watcher`, `retry`, and `natsclient` unit tests that make no network call) runs inside `synctest.Test`,
    as `refowner_test.go:385` does. Inside the bubble, timers are admitted without restriction. With sleeps banned,
    `<-time.After(d)` is how a test moves the fake clock (for example the TTL test at `cache_test.go:286-298`), and
    `synctest.Wait` settles the bubble before an assertion. The 30 `t.Parallel()` calls in `pkg/cache` tests are
    removed, because `t.Parallel` cannot be called inside a bubble.
  - **R1b. Real clock, event exists.** Tests against a real broker wait on a channel or callback, or on
    `probe.Await` (`internal/harness/probe/await.go:20`) over observed state. A real-clock timer is admitted only
    as the failure bound of a `select` or context that waits on that signal. A failure bound is not a pacing
    device. It never decides the outcome of a correct run, and it is never sized tight: it comes from the test's
    context deadline or is at least 10 s, because the lanes run under `-race -cpu 1`, and a sub-second literal
    such as 100 ms is not a failure bound.
  - **R1c. Real clock, behaviour interval.** A real-clock interval that the behaviour under test is defined over
    (AckWait, a TTL, a drain window) is admitted only when its expiry can never fail a correct implementation: a
    slow host can only make the check miss a defect. The interval is written from the configured value, not a
    fresh literal. An R1c timer used as pacing — the 50 ms steps at `delivery_settlement_integration_test.go:102-106`
    — is admissible only because the table below lists it; an unlisted pacing timer is banned.
  - **Ban.** Any other real-clock wait is banned, including a sleep swapped for a `time.After`, `time.NewTimer` or
    `time.Tick` whose expiry stands in for the event. The text check cannot see timers (`testtext_test.go:10-13`),
    so task 3.10 checks each real-clock timer in a repaired file against this table. Those not listed are R1b
    failure bounds.

  | Site at the pin | What it waits for | Disposition |
  |---|---|---|
  | `natsclient/subscription_integration_test.go:115-119` | 200 ms in which a correct `Drain` must not return while its callback runs | R1c, kept: expiry only misses a defect (its own comment, `:113-115`) |
  | `natsclient/client_close_integration_test.go:77` | 250 ms in which a correct `Close` must not return | R1c, kept: expiry only misses a defect (added at task 3.7b; the pin row was missing) |
  | `natsclient/delivery_settlement_integration_test.go:102-106` | 50 ms polling steps across the AckWait renewal window | R1c, kept: the window is AckWait's, and each step ends on `ctx.Done` or a redelivery check, never failing a correct renewal |
  | `pkg/cache/coalescing_set_test.go:92` | 10 ms in which the callback must not fire | R1a: inside the bubble, after `synctest.Wait`, the callback has not fired before the window |
  | `pkg/cache/cache_test.go:294` (TTL, `:286-298`) | 150 ms past a 100 ms TTL | R1a: `<-time.After` inside the bubble moves the fake clock |

  Three pin rows retired with their tests: `integration_test.go:262, :278` with `TestIntegration_HealthMonitoring`
  (owner ruling, #9 comment 5969522395, item 2), and `kv_error_integration_test.go:440` with
  `TestTemporalResolver_ErrorBoundaries` (item 1).

- **R2. Skip.** A skip is removed. Where the skip meant only "needs a broker", the test moves into an
  `//go:build integration` file and runs with no skip call. The one skip at the pin, `TestIntegration_Reconnection`
  (`natsclient/integration_test.go:63`), is skipped because the mapped port changes on restart. By owner ruling (Q2) it
  is rewritten on `natsfixture.Restart`. The loss is observed through `WithConnectionLostCallback` and `Status` through
  `probe.Await` (item 2), and it proves that a client dialled from the new `URL()` reaches the restarted broker
  (re-dial). It does not prove nats.go's automatic reconnect, and its name and comment say so.
- **R3. Build tag.** A `// +build` line is deleted and the `//go:build integration` line stays. None remains in the 15
  packages: the one at the pin (`pkg/acme/integration_test.go:2`) leaves with `pkg/acme` (D1).
- **R4. Repeat failures.** A ported test that fails `task test:repeat` is repaired in the porting pull request. It
  is never deferred and never filed `class:flake`. The failures measured at the pin are intermittent and
  order-dependent (P19), so one green run proves nothing. The repair removes the cause (R1), and the package then
  passes `test:repeat` several times on recorded seeds, task 3.6. Integration-tagged tests are not repeated
  (`merge-gate`, "Varied and repeated unit runs"), so a ported integration test has no repeat evidence. Its only
  evidence is its one run in the integration lane.
- **R5. Fixed address.** A ported test binds or names no fixed address (`harness-boundaries`, "No fixed addresses in
  tests"). In `natsclient`, 55 lines in 12 files match the guard at the pin: 47 in 8 unit files and 8 in 4 integration
  files. `stream_visibility_test.go:82` is an error string the guard does not match. `t.Parallel()` (41 calls in 6
  `natsclient` unit files) stays unless its test moves into a `synctest` bubble. The two tests that pinned NATS
  2.14.4 run on `.nats-image` with the override dropped (#9 comment 5969522395, item 6); the image-pin guard cannot
  see that spelling, review only.

Doc-comment sleeps in ported non-test files (`pkg/errs/doc.go:48,111,288`, `metric/doc.go:386`,
`natsclient/doc.go:209,536,541`) are carried as they are (Q4). They are outside both checks' scope.

A test of a removed feature is deleted with it, and the deletion is recorded on the row like a repair:
`TestEvictCallback` (`pkg/cache/cache_test.go:444-503`, 60 lines, two subtests) goes with `WithEvictionCallback` (D7).
In `natsclient`, the files and tests that tasks 3.7 and 3.7a name as removed with dropped surface.

Note on the `refowner` double (task 2.5): `TestEachFailpointTripsExactlyItsCheck` (`refowner_test.go:382-401`) runs
every check against each failpoint's double. A failpoint that makes Start fail would trip every check that starts
the owner, so "Start fails" is a construction mode of the double, like `restartable`, and not a failpoint.

- In must-fail mode, Start returns its error after `o.startAttempted = true` (`:72`), so `stopBeforeStartPanics`
  does not trip the new check.
- `startFailsButHolds` is a table row whose expected check is `FailedStartHoldsNothing`. It acts only in must-fail
  mode, where Start starts a worker under its context and then fails. The worker is ended through Start authority,
  so `finalize` holds with no exemption (`lifecycle-suite`, "Complete sensitivity matrix").
- In normal mode the row is inert. So `TestAbortStopThenFinishJoinsWorker` (`:414-418`), which takes every row except
  `ControlledStopUnderLiveStartAuthority`'s, runs it as a clean double.
- The `checks` entry for the new check is marked must-fail. Every test that iterates `checks` builds the must-fail
  double for that entry, including `TestChecksPassAgainstCleanDouble` (`:239-249`).

### D9. One canonical collector per metric key

Codex's review of PR #48 (comment 5956732582, finding 4) found that `MetricsRegistry` reports success for a
registration it did not perform. The owner accepted this design on 2026-10-02, after an architect draft and a
reviewer pass in round 2. The architect's pin probe measured three silent losses at the pin:

- Same key, second collector: `RegisterCounter` (`registry.go:136`) returns nil and keeps the first collector, so
  writes to the second are gathered as 0.
- Cross-key alias: the `AlreadyRegisteredError` branch (`:143-146`) stores, under a second key, a candidate that
  Prometheus did not register; `Unregister` of the second key then removes the first key's series.
- `RegisterGaugeVec` (`:246`) calls `RegisterOrGetGaugeVec` and throws away the canonical collector it returns.

The idempotent behaviour entered SemStreams in `3e37d387` ("make metric registration idempotent to prevent nil
component panic").

The registration surface becomes one generic function. Every guard runs under `r.mu`:

```go
func RegisterOrGet[C prometheus.Collector](r *MetricsRegistry, serviceName, metricName string, candidate C) (C, error)
```

- **G1, nil.** The candidate is refused when `reflect.ValueOf(any(candidate))` is invalid, or when its kind is
  Pointer, Interface, Map, Slice, Func or Chan and `IsNil()` is true.
- **G2, same key.** The existing collector is returned only when `existing.(C)` succeeds, when
  `reflect.TypeOf(existing) == reflect.TypeOf(candidate)` (the exact concrete type: `NewCounter`, `NewGauge` and
  `NewHistogram` return interfaces, and a gauge satisfies `prometheus.Counter`), and when `sameCollectorDescriptors`
  (`:66-85`) holds.
- **G3, new key.** `prometheus.Register(candidate)`. Any error, `AlreadyRegisteredError` included, is refused and
  nothing is stored; otherwise the candidate is stored and returned.

`MetricsRegistrar` (`:16-25`) and the six `Register*` methods (`:127-278`) are removed; `RegisterOrGetGaugeVec`
(`:27-60`) is generalised into `RegisterOrGet`. `Unregister` is unchanged.

What a caller observes:

| Case | Result |
|---|---|
| Same key, same concrete type, same descriptors | the canonical collector and nil; its writes are gathered |
| Same key, a type, help or label mismatch | a zero `C` and an `errs` fatal error; the canonical collector is untouched |
| A nil or typed-nil candidate | a zero `C` and a fatal error, no panic |
| A cross-key alias, including a core metric or a collector registered directly through `PrometheusRegistry()` (`:118`) | a fatal error, nothing stored; `Unregister` of the second key returns false |

A caller uses the collector `RegisterOrGet` returns, never its own candidate; this is review only (AGENTS.md rule
index). Eight example tests, each written first, with the oracle each one asserts:

1. Same key over Counter, Gauge, Histogram, CounterVec, GaugeVec and HistogramVec: no error, and two writes through
   the returned handles gathered as 2.
2. A Gauge, then `RegisterOrGet[prometheus.Counter]` with the same name and help: a fatal error, the zero
   collector, and the gauge's gathered value unchanged.
3. Same key with a different help: a fatal error, the zero collector, the first counter's gathered value.
4. A typed nil `*GaugeVec` and a nil `Counter`: a fatal error, the zero collector, no panic, and `Unregister` of
   the key false (nothing in the key map). No gathered value: nothing was registered to gather.
5. A cross-key alias: a fatal error, the zero collector, `Unregister` of the second key false, and one series of
   value 1 after `a.Inc()`.
6. A collision with a core metric: a fatal error, the zero collector, `Unregister` false, the core gauge's
   gathered value.
7. A collision with a collector registered directly: as test 6, for the direct collector.
8. The concurrent test (`registry_test.go:188`) generalised under `-race`: no error, one identity, and the
   workers' writes gathered as their count.

Failing first, implementer-reported (the run was a temporary shim that sent the new signature to the pin's
`Register*` methods): test 1 gathered 1 for all six kinds; tests 2, 3, 5 and 7 were accepted; in test 4 the nil
`Counter` was accepted, while the typed-nil `*GaugeVec` was already refused at the pin (`RegisterOrGetGaugeVec`'s
nil check); test 6's collision was already refused at the pin by `RegisterGaugeVec`, and the test failed only on
the shim returning the candidate instead of the zero collector; test 8 returned different collectors. Guard
mutations, also implementer-reported: removing the type check fails test 2, the descriptor check test 3, and
storing the candidate in the alias branch tests 5, 6 and 7. Removing the nil guard is a different result: no
assertion fails; the run crashes with a nil-pointer panic inside Prometheus' `Register`.

**Generated check (`docs/testing.md`, "Decide whether generated checks are needed").** D9 states an idempotence law
(registering a key again returns the same collector) over an order-dependent history (registration, refusal,
unregistration, re-registration, and writes through handles returned at different times), so examples alone do not
cover it. `TestPropRegisterOrGetHistory` (`metric/registerorget_prop_test.go`) is a Rapid state machine over three
keys, two metric names, two help texts and five candidate kinds (counter; gauge; a gauge registered as
`prometheus.Counter`; a counter vector; a gauge vector with the core `semstreams_service_status` descriptor), with
actions `RegisterOrGet`, `RegisterSame` (a held key's own spec, so the same-key success path runs in most
histories), `RegisterNil` (typed and interface nil), `Unregister` and `Write`. A reference model owned by the test
holds key → (concrete type, descriptor), the names registered on the Prometheus registry including the core
metric, and each name's first help and label names, which Prometheus keeps for the registry's lifetime even after
`Unregister` (client_golang `registry.go`, `Unregister`: "dimHashesByName is left untouched"). After every step the
test asserts, against the model: a same-key success returns the canonical collector; every refusal the model
predicts is a fatal error with the zero collector and nothing stored; `Unregister` returns what the model holds; and
through `Gather`, each held key's series carries exactly the model's writes and no generated name the model does
not hold is gathered. It logs how often each assertion ran. History classes it covers: same-key retries of the same
and of another type or descriptor, cross-key aliases, core-name collisions, unregister then re-register under
the same or another key, inconsistent help after unregister, and writes through handles from earlier and later
registrations. The eight examples still own: the six collector kinds one by one (the generator uses four), a
collector registered directly on `PrometheusRegistry()`, and concurrent callers (the model is sequential).

The `metric-registry` delta states this law (owner ruling, #9 comment 5983188211). Its scenarios map to the example
tests above: same key, same type and descriptors → `TestRegisterOrGetSameKeyReturnsCanonicalCollector`,
`TestRegisterOrGetConcurrentCallersShareOneCollector`; same key, another type, help or label names →
`TestRegisterOrGetRefusesSameKeyOfAnotherType`, `TestRegisterOrGetRefusesSameKeyWithDifferentHelp`,
`TestRegisterOrGetRefusesSameKeyWithDifferentLabelNames`; nil candidate →
`TestRegisterOrGetRefusesNilCandidates`; cross-key alias → `TestRegisterOrGetRefusesCrossKeyAlias`,
`TestRegisterOrGetRefusesCoreMetricCollision`, `TestRegisterOrGetRefusesDirectRegistrationCollision`. A label-name
mismatch on a held key has its own example, because `TestPropRegisterOrGetHistory` cannot reach it: its generator
fixes the label names by kind (`registerorget_prop_test.go:67-76`).

Consumer impact, recorded on the `metric` row: semsource `internal/entitypub/metrics.go:99-106` and semboids
(`internal/boidgraph/metrics.go`, `internal/sim/lifecycle.go`, `internal/api/graphstream_metrics.go`) stop compiling on
their next pin bump, and the semsource comment at `:99-101` has to be rewritten. At run time, a consumer that registers
one descriptor under two keys now gets a fatal error at startup instead of silent success; the pin's
`registry_test.go:254-275` encoded that pattern for "component recreation from stale KV data". The later callers in this
change adopt it in tasks 3.6 (`pkg/cache`), 3.7 (`natsclient` registration, `jetstream_metrics.go:128-161`) and 3.7d. In
`natsclient`, 8 of the 11 collectors orphan when two clients share a registry (`:128-146, :158-160`). Three counters
`Add` the server's current values on every poll (`:305-307`); they become gauges set from server state, with new names
(owner ruling, #9 comment 5969522395, item 4). No client label. The `stream_state` and `forgetConsumer` sharing
defects (`:278`, `:232-243`) are one shared-series ownership issue, #75, tracked, not fixed here.

## Premises (each with its measurement)

- P1. The 15 packages are closure(natsclient) ∪ closure(message) less `pkg/acme`, closed under in-set imports once the
  ACME loaders are cut, with no out-of-set edge. — foundation P2; D1's in-set column (`inset-edges.json`); D1 grep.
  `pkg/cache` enters only through `kv_temporal.go:8` and stays by admission (D1).
- P2. The set has two services (`metric.Server`, `natsclient.Client`) and four sites (`Watcher`, the TTL and hybrid
  caches, `CoalescingSet`; `TemporalResolver` is dropped) of background work outside them, and no `Start`/`Stop`
  component; its other two `go` statements end before their call returns. — foundation P22; `owners-scan.txt`; D3, D7;
  pin grep over the 15 packages of `go` statements (16 sites, after the ACME cut) and of `time.AfterFunc` (2 sites, both
  in `natsclient/client.go`, `:289` and `:1555`).
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
- P13. `Client.Close(ctx)` has no nil-context check and panics on `Close(nil)` at `:685` with a live connection;
  `Connect(nil)` dials at `:494` and panics at `:495`, leaking the connection; `Subscription.Drain` refuses nil. —
  `natsclient/client.go:578, :685, :494-495, :796-799`; probe, PR #48 comment 5942307713.
- P14. `NewCoalescingSet` returns no error; `StartBackgroundCheck` returns nothing. — `pkg/cache/coalescing_set.go:27`,
  `pkg/resource/watcher.go:141`.
- P15. `CreateStream` always creates a file-backed stream with the fixture's bounds. — `natsfixture/fixture.go:431-446`.
- P16. SemSource at `e4febc0d` imports eight of the 15 directly: `natsclient`, `metric`, `payloadregistry`,
  `message` (28 files), `vocabulary` (20), `pkg/types` (5), `pkg/retry` (4), `pkg/errs` (3). — foundation §5.1.
- P17. At the pin, `metric.Server` fails AbortStop once in 1,000 runs and `Client` fails NilContexts and SecondStart;
  every other check passes on both. — task 2.0 probe, PR #48 comment 5942307713.
- P18. Measured before the rulings of 2026-10-03: in the 121 test files ported (pin, after the Q3 exclusions and without
  `pkg/acme`) there are 70 `time.Sleep` calls (`pkg/resource` 6, `pkg/retry` 1, `pkg/cache` 26, `natsclient` 37), one
  skip call and no `// +build` line; 22 files use `time.After(` or `time.NewTimer(`; `pkg/cache` tests call
  `t.Parallel()` 30 times. — text scan of the pin tarball with the contract tests' rules (`testtext_test.go:58,96,180`);
  hit list attached to the pull request. After them, `natsclient` keeps 31 sleeps (8 unit, 23 integration).
- P19. Under `go test -count=5 -cpu 1 -shuffle=on`, `pkg/cache` fails intermittently at the pin:
  `TestCoalescingSet_EntityUpdateScenario`, `TestAttack_ConcurrentAddRemove` and
  `TestCoalescingSet_ContextCancellation` each failed in some runs, and some runs were green. Over four full runs of
  the 14 tested packages, no other package failed. Under `-race -count=1 -cpu 1` all 14 passed once. —
  eleven `pkg/cache` runs with their seeds (architect four, reviewer seven); task 2.0b records them on the pull
  request.
- P20. The CI job `verify` has a 15-minute limit (`merge-gate` "Required needs both jobs";
  `.github/workflows/ci.yml:22`) and runs `task verify`, which runs the integration lane (`ci.yml:48-50`) under
  `-race -count=1 -p 2 -timeout 10m` (`scripts/test-integration.sh:402`). The wall time with the port added is not
  measured; task 3.11 measures it.
- P21. SemConnect at `dff12657` imports `pkg/projection/contract` (`gateway/cs-api/payloads.go:11`) and fills
  `payloadregistry.Registration.Contracts` with it (`:85`); no starter consumer imports `pkg/platform`,
  `pkg/security`, `pkg/resource`, `pkg/timestamp`, `pkg/tlsutil` or `pkg/cache`. — pass3 §2.1 (SemConnect's 26);
  `git grep -lE 'semstreams/pkg/(platform|resource|security|timestamp|projection/contract|tlsutil|cache)"' --
  '*.go'` in semsource `4093d3c`, semconnect `dff1265`, semteams `ce22c961`, semboids `37dbdb0`: one hit,
  semconnect `gateway/cs-api/payloads.go`.
- P22. Public signatures at the pin name `platform.Config` in eight exported symbols and `security.Config` in one. Of
  the eleven public packages, one exported symbol at the pin names a type from the four internal ones,
  `natsclient.TemporalResolver.GetStats`; it is dropped with `TemporalResolver` (owner ruling, #9 comment 5969522395,
  item 1), so in this change none does, and only floor packages import the four. — A `go/parser` walk of the eleven
  packages' 80 non-test files (exported funcs and methods on exported types with their type parameters, exported and
  embedded struct fields, interface methods, other exported type definitions, exported vars and consts with type and
  value) finds one hit for the four: `natsclient/kv_temporal.go:221` (dropped with `TemporalResolver`; afterwards no
  importer of `internal/cache` exists in the module). The same walk with `pkg/platform` and `pkg/security` counted as
  internal finds `metric/handler.go:40`; `message/base_message.go:81, :91`; `message/federation.go:31, :50, :62, :78,
  :95`; `vocabulary/iris.go:85`. Importers of the four: `natsclient/client.go:18`, `natsclient/kv_temporal.go:8`,
  `metric/handler.go:20`, `message/base_message.go:13`, `message/meta_default.go:6`. `GetStats`'s three callers at the
  pin (`natsclient/kv_error_integration_test.go:421, :436, :447`) lie in `TestTemporalResolver_ErrorBoundaries`, which
  leaves with it (D1).
- P23. In the 15 packages at the pin only `pkg/types/entity_id_prop_test.go` imports `pgregory.net/rapid`. — `git
  grep -ln pgregory.net/rapid 8b99efe9` over the 15 directories finds that file and
  `vocabulary/export/datatype_prop_test.go`, which lies in `vocabulary/export`, a package outside the 15.

## Declared costs

- Nothing boots. The change's green is substrate, harness, two services and four sites of background work (P2).
- 29 integration-tagged test files in the ported packages (27 ported and one helper in `natsclient`; one in
  `internal/tlsutil`), and the harness's own `natsfixture` one, run only under `task test:integration` and the host
  lock; the unit lane does not prove them, and `test:repeat` does not repeat them.
- Ported tests are repaired (D8), so carried tests differ from the pin's text; every difference is on a row.
- The 15-minute `verify` limit is spec; if the port pushes the job past it, the change holds for the owner (task
  3.11), and the design does not predict the number.
- The `Run` signature change touches both existing callers and every future service test; that is the point.
- Two adapters read unexported fields; a reviewer re-checks each against the service's retained kinds.
- `CoalescingSet.Close` becomes `Shutdown(ctx)` and `cache.WithEvictionCallback` is deleted; three later callers
  become port-refactor rows (D5).
- `Client.Close` now joins its background work, every running message-handler invocation and the event handlers of
  the connection it closes (D3), and can wait on a caller's callback, bounded by its context. Work offered once it is
  closing is refused. A delivery cut short by `Stop` or a forced close can lose one AckNone or core message (accepted,
  #9 comment 5980769459). `Status()` keeps its value through the drain. A drain timeout is now an error.
- Outward-facing names (`semstreams_*` metrics, the `SEMSTREAMS_` environment prefix) are carried unchanged pending
  #69 (#9 comments 5969776736 item 4, 5968665464).
- `URL()` changes after `Restart`; a test that forgets to re-dial fails loudly, not silently.
- Four packages' import paths differ from the pin (`pkg/<name>` → `internal/<name>`, D5); each row's `source_path` →
  `destination` records the difference. The 3.7a list is dropped, including `WithTLS`, `WithToken` and
  `WithCompression`. `WithCredentials` remains the only auth option. This makes a defect at the pin plain without making
  it worse: `config` parses the NATS `Username`, `Password`, `Token` and `TLS`
  (`config/config.go:175-188, :761-768, :1051-1054`), but boot builds the client from URLs only
  (`internal/boot/run.go:425-436` → `internal/bootstrapobservability/bootstrap.go:123`). So an operator who configures
  TLS, a token or credentials gets a plaintext connection with no auth, and nothing says so. It is tracked by #74
  against the `config`/`boot` port (#9 comment 5968830525, rule 4); that change wires what it admits and refuses what it
  cannot honour. A consumer that needs an option back adds it as new surface with a present consumer. Three JetStream
  consumer metrics change name and type (3.7d).
- If a critical package measures below 80% at landing, the change holds for the owner (task 3.8); the design does not
  predict the number.
- The nats.go version differs from the pin (v1.54.0 vs v1.52.0); regression evidence is the carried `natsclient`
  tests passing, nothing more.
- The unit lane embeds `nats-server` v2.14.7, where the pin used v2.12.4. One broker version across both lanes is review
  only, because T-B3 cannot see an embedded server.
- `Close(nil)`/`Connect(nil)` refusal, the second-`Connect` refusal and `metric.Server`'s abort cause are changed
  behaviour; each is recorded as such on its row, not as carried.
