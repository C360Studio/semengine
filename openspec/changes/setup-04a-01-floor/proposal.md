# Proposal: setup-04a-01-floor

## Why

Slice 04A ports SemEngine's tier-0 set — 65 packages, 140,842 lines at the frozen SemStreams pin `8b99efe9` — as a
chain of seven OpenSpec changes, accepted by the owner on #9 (archived design
`openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md`, "Owner rulings"). This is the first change.
It ports the **floor**: the 16 packages that every later change builds on (the NATS client, the message model, the
metrics server, the payload registry, the vocabulary, and the small `pkg/*` libraries they import), and it lands the
harness extension that the later changes' proving tests need (#9 item 6). Nothing boots in this change; its value is
that the substrate compiles and tests against the harness, and that the harness can now restart a broker, inject a
key-value fault after a real write, host and kill a process, and demand a failing start from every owner.

Terms used below: a **package row** is one entry in `docs/admission-ledger.yaml` per ported package directory; an
**owner** is a type that starts a goroutine or holds a resource beyond the call that created it and exposes a method
that ends it; the **lifecycle suite** (`internal/harness/lifecycletest`) is the set of checks every owner is run
through; **T-B1** is the import-graph test that keeps test libraries out of production files.

## What Changes

- Port 16 packages (25,758 non-test lines) with their 127 test files (34,887 lines) as they are at the pin, in
  import order (design D1). Tests travel with their packages; `stretchr/testify` becomes a direct test dependency
  (ruling b). `natsclient`'s 69 `NewTestClient` call sites move to `natsfixture`; the two T-B1 collisions in this set
  (`natsclient/test_client.go`, `payloadregistry/testing.go`) are adapted, not ported.
- Two cross-package test helpers typed on `testing.TB` are rehomed into the harness, the only place T-B1 allows them:
  `internal/semantictest` → `internal/harness/semantictest`; `payloadregistry/testing.go` →
  `internal/harness/payloadfixture` (foundation D4 "Fixture homes").
- Harness extension, as the first task (#9 item 6): `natsfixture.Fixture.Restart` (stop and start the same container,
  re-read the mapped port, reconnect); a fault-injecting `jetstream.KeyValue` double that fails a write *after* the
  real call completed; `internal/harness/prochost`, a process host that re-executes the test binary as a helper
  process and can signal, pause and kill it; the no-SemStreams-import test (I8).
- `lifecycletest.Run` takes a second, required factory whose owner's `Start` must fail; the check proves a failed
  start holds nothing (#38). Both existing callers are updated and the fixture gains a failing factory of its own.
- Five owners in this set run under the suite with test-side `Observe` adapters: `metric.Server` (#38's first real
  target: a listener port already bound), `natsclient.Client`, `natsclient.TemporalResolver`,
  `pkg/cache.CoalescingSet`, `pkg/resource.Watcher`. Before any port, the suite's existing checks are run against
  each of the five at the pin and every failure becomes a named, failing-first `adapt` item on the owner's ledger
  row. One is known already: `natsclient.Client.Close` and `Connect` do not refuse a nil context (a nil context
  panics in `Close` today); they will, as changed behaviour recorded on the row. The last two owners stop with an
  unbounded wait today and gain a context-bounded ender under "SS#1415-class" `adapt` rows — the shape 03B ruled
  for `config.Manager.Stop(timeout)` in SemStreams issue #1415: a stop that takes a timeout or nothing becomes a
  stop that takes a context (ruling h). Three of the five owners have no start that can fail; whether the required
  failing factory is satisfied by a typed "no fallible start" value, by adding a fallible start, or by exclusion is
  an owner question (design D7) and a hold.
- Ledger: 16 package rows at the full pin SHA (foundation D9), eight rows for the packages D4 separated and never
  carries, the Tier-1 cross-check re-measured on the ruled set (#9 item 1), and the triage of this set's 14
  production `context.Background()`/`TODO()` sites (foundation D9).
- Gates: `task cover:check` targets for `natsclient`, `message`, `payloadregistry` (the D10 critical-list members in
  this set); the package-doc lint already on in `revive.toml` now covers the eight packages this change makes
  public (D16: `natsclient`, `metric`, `payloadregistry`, `message`, `vocabulary`, `pkg/types`, `pkg/retry`,
  `pkg/errs` — the ones SemSource imports directly); `AGENTS.md`'s verify list brought back in line with
  `scripts/verify.sh`.

## Capabilities

- `harness-boundaries` (MODIFIED): the import-graph rule gains the no-SemStreams clause (I8) and the aggregator
  clause (T-B8), and states what harness packages may import.
- `nats-fixture` (MODIFIED): `Restart` and what it does to fixture-owned consumers, streams and buckets; a
  memory-backed owned stream; the fault-injecting key-value double; the fixture under the lifecycle floor including
  failed start.
- `lifecycle-suite` (MODIFIED): the required failing factory and the failed-start check; the `Observe` adapter
  contract.
- `process-host` (ADDED): a helper-process host for process-kill proofs.
- `transport-client` (ADDED): what the ported `natsclient` keeps at tier 0 — acknowledged is not durable on a memory
  stream; settlement follows the decision; `Drain` refuses a nil context and `Close` is bounded by the caller's
  deadline — and one thing it gains: `Close` and `Connect` refuse a nil context.

## Impact

- New Go code: 16 ported packages under their pin paths; five harness additions under `internal/harness/`; adapters
  as `_test.go` files inside the ported packages.
- `go.mod`: `prometheus/client_golang`, `google/uuid`, `go-acme/lego/v4`, `stretchr/testify` become direct; nats.go
  stays at v1.54.0 (the pin used v1.52.0 — a behavioural pin difference recorded in the `natsclient` row).
- `docs/admission-ledger.yaml`: 24 new rows; `scripts/cover-check.sh`: three new targets; no lint configuration
  change (`revive.toml:22` already enables `package-comments`).
- No consumer can compose anything from this change alone; SemSource's integration branch waits for change 5.
- Out of scope: every repair row's proof except the two halves this change's own code can produce (settlement's
  `natsclient` half; SS#1218's `pkg/errs` sentinel); every package outside the 16; any port refactor (#25–#36).
