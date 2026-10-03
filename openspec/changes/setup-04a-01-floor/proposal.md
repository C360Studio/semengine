# Proposal: setup-04a-01-floor

## Why

Slice 04A ports SemEngine's tier-0 set — 65 packages, 140,842 lines at the frozen SemStreams pin `8b99efe9` — as a
chain of seven OpenSpec changes, accepted by the owner on #9 (archived design
`openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md`, "Owner rulings"). This is the first change.
It ports the **floor**: the 15 packages that every later change builds on (the NATS client, the message model, the
metrics server, the payload registry, the vocabulary, and the small `pkg/*` libraries they import), and it lands the
harness extension that the later changes' proving tests need (#9 item 6). Nothing boots in this change; its value is
that the substrate compiles and tests against the harness, and that the harness can now restart a broker, inject a
key-value fault after a real write, host and kill a process, and demand a failing start from every service.

Terms used below: a **package row** is one entry in `docs/admission-ledger.yaml` per ported package directory; an
**owner** is a type that starts a goroutine or holds a resource beyond the call that created it and exposes a method
that ends it; a **service** is an owner the engine starts, supervises and stops, with a `Start` that can fail; the
**lifecycle suite** (`internal/harness/lifecycletest`) is the set of checks every service is run through; **T-B1** is
the import-graph test that keeps test libraries out of production files.

## What Changes

- Port 15 packages (25,215 non-test lines at the pin, less the two ACME loaders in `pkg/tlsutil`) with their 121 test
  files (33,279 lines), repaired where the flake-defense rules require (design D8), in import order (design D1). Eleven
  are public and keep their pin paths; four move from `pkg/<name>` to `internal/<name>` (#9 comments 5952661571,
  5953295358; design D5). Tests travel with their packages; `stretchr/testify` (ruling b) and `pgregory.net/rapid`
  (owner ruling, PR #48 comment 5951926492) become direct test dependencies. `natsclient`'s 51 (56 at the pin, five in
  tests removed with dropped surface) `NewTestClient` call sites move to `natsfixture`; the two T-B1 collisions in this
  set (`natsclient/test_client.go`, `payloadregistry/testing.go`) are adapted, not ported.
- Two cross-package test helpers typed on `testing.TB` are rehomed into the harness, the only place T-B1 allows them:
  `internal/semantictest` → `internal/harness/semantictest`; `payloadregistry/testing.go` →
  `internal/harness/payloadfixture` (foundation D4 "Fixture homes").
- Harness extension, as the first task (#9 item 6): `natsfixture.Fixture.Restart` (stop and start the same container,
  re-read the mapped port, reconnect); a fault-injecting `jetstream.KeyValue` double that fails a write *after* the
  real call completed; `internal/harness/prochost`, a process host that re-executes the test binary as a helper
  process and can signal, pause and kill it; the no-SemStreams-import test (I8).
- `lifecycletest.Run` takes a second, required factory whose owner's `Start` must fail; the check proves a failed
  start holds nothing (#38). Both existing callers are updated and the fixture gains a failing factory of its own.
- The two services in this set run under the suite with test-side `Observe` adapters: `metric.Server` (#38's first real
  target: a listener port already bound) and `natsclient.Client`. The pre-port probe and a read of every `go` statement
  found five defects in them (on `Client`, a nil context, a second `Connect`, and goroutines `Close` never joins; on
  `metric.Server`, a dropped abort cause and a fixed join timeout); each is a failing-first `adapt` item. The helpers
  that run background work — `pkg/resource.Watcher`, the TTL and hybrid caches, `pkg/cache.CoalescingSet` — each take
  one of three shapes, ruled as the engine's standing rule (#9 comments 5950234192, 5950482163): `Run(ctx)`, a `Close()`
  that joins, or `Shutdown(ctx)`, with no fixed shutdown timeout and a `synctest` test proving nothing is left behind.
- Ledger: 15 package rows at the full pin SHA (foundation D9), eight rows for the packages D4 separated and never
  carries, the Tier-1 cross-check re-measured on the ruled set (#9 item 1), and the triage of this set's 14
  production `context.Background()`/`TODO()` sites (foundation D9).
- Gates: `task cover:check` targets for `natsclient`, `message`, `payloadregistry` (the D10 critical-list members in
  this set); the package-doc lint already on in `revive.toml` now covers the eleven packages this change makes
  public (design D5: `natsclient`, `metric`, `payloadregistry`, `message`, `vocabulary`, `pkg/types`, `pkg/retry`,
  `pkg/errs`, which SemSource imports directly; `pkg/projection/contract`, which SemConnect imports; `pkg/security`,
  whose type a public signature names; `pkg/platform`, whose type `config` names, change 3); `AGENTS.md`'s verify
  list brought back in line with `scripts/verify.sh`.

## Capabilities

- `harness-boundaries` (MODIFIED): the import-graph rule gains the no-SemStreams clause (I8) and the aggregator
  clause (T-B8), and states what harness packages may import; a new requirement holds that a public package's
  exported identifiers name no type declared under `internal/` (owner ruling, #9 comment 5953477174).
- `nats-fixture` (MODIFIED): `Restart` and what it does to fixture-owned consumers, streams and buckets; a
  memory-backed owned stream; the fault-injecting key-value double; the fixture under the lifecycle floor including
  failed start.
- `lifecycle-suite` (MODIFIED): the required failing factory and the failed-start check, for services; the `Observe`
  adapter contract.
- `background-work` (ADDED): the three shapes background work takes, no fixed shutdown timeout, nil contexts refused
  at the call, and the `synctest` test that proves nothing is left behind.
- `process-host` (ADDED): a helper-process host for process-kill proofs.
- `transport-client` (ADDED): what the ported `natsclient` keeps at tier 0 — acknowledged is not durable on a memory
  stream; settlement follows the decision; `Drain` refuses a nil context and `Close` is bounded by the caller's
  deadline — and one thing it gains: `Close` and `Connect` refuse a nil context.

## Impact

- New Go code: 15 ported packages, eleven at their pin paths and four under `internal/` (design D5); five harness
  additions under `internal/harness/`; two adapters and the helper tests as `_test.go` files inside the ported
  packages.
- `go.mod`: `prometheus/client_golang`, `google/uuid`, `stretchr/testify`, `pgregory.net/rapid` become direct;
  nats.go stays at v1.54.0 (the pin used v1.52.0 — a behavioural pin difference recorded in the `natsclient` row).
- `docs/admission-ledger.yaml`: 27 new rows (15 package rows, 8 `defer-exclude` package rows, 4 `defer-exclude`
  file rows for the unported `natsclient` test files); `scripts/cover-check.sh`: three new targets; no lint
  configuration
  change (`revive.toml:22` already enables `package-comments`).
- No consumer can compose anything from this change alone; SemSource's integration branch waits for change 5.
- Out of scope: every repair row's proof except the two halves this change's own code can produce (settlement's
  `natsclient` half; SS#1218's `pkg/errs` sentinel); every package outside the 15, including `pkg/acme` and the ACME
  loaders of `pkg/tlsutil` (change 5, owner ruling #9 comment 5950752741); any port refactor (#25–#36).
