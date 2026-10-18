# Inventory: lifecycleguard-tidy

base: `4c6d95f71a491fe274f6e066e2c6f25535e45b8c` (origin/main `572c0ee790818b7663b8806740d094ced96f4b44` plus the claim
commit, which adds only `openspec/changes/lifecycleguard-tidy/`; draft PR #308). Every pin is at the base.

Status: drafted together with the design at the orchestrator's direction; its `INVENTORY PASS` is the reviewer's
(architect contract, Required workflow step 3). Round 2 revises it after the `INVENTORY FAIL` at `9c772d5`: section 2
now lists every stop method's handling of an ended context, and section 1e records the panicking-cleanup finding.

Question: how `internal/lifecycleguard` refuses a nil or ended context, how its cleanup reaches `Start` and `Stop`,
who reads either, and what tests the concurrent-`Stop` refusal. Scope: #196, comment of 2026-10-16 (moved from #153
item 6); owner, 2026-10-18: "Claim PR 1, the lifecycleguard tidy from #196, as a draft and write its design". Not in
scope: PR 2 of the order on #282 (the `Start`-context change, ADR-113), #291, #301, and #196's adoption rows.

No package is carried from the pin. `internal/lifecycleguard` is SemEngine's own code: design D13 of
`setup-04a-02-ingest-kernel` (`openspec/changes/archive/2026-10-12-setup-04a-02-ingest-kernel/design.md:721`,
`### D13. One owner-lifecycle guard (SS#1411)`); `docs/repository-map.md:74`, "New in `setup-04a-02-ingest-kernel`".
So no pin probe, shape sweep or pin ADR listing is owed.

Repositories read (`docs/inventory-scope.md`): this tree; the semsparql working tree at
`51ac5357ebd89faa5364ab215032abd3c7ab45b9`, read without git commands, for one question: does a consumer read the
errors of graph-ingest's `Start` or `Stop`. Its row names graph-ingest's query verbs, and its test fixtures start
graph-ingest to serve them.

## 1. The claimed gaps (#196): each confirmed

The four claims are #153 item 6 (style-bar review of PR #93 at `88d0d3b`, 2026-10-09), moved to #196 on 2026-10-16.
That item closes with "Plain shape: one refusal helper, and the cleanup given once."

### 1a. A nil context: two sentinels, two texts

- `internal/lifecycleguard/guard.go:43`:
  `return errs.WrapInvalid(errs.ErrInvalidConfig, "lifecycleguard", "Start", "context cannot be nil")`
- `internal/lifecycleguard/guard.go:87`:
  `return errs.WrapInvalid(errs.ErrInvalidData, "lifecycleguard", "Stop", "nil context")`
- Both are class invalid, because the wrapper decides (`pkg/errs/errs.go:203`, `return ce.Class == ErrorInvalid`). The
  Start sentinel is one that `errs.IsFatal` lists (`pkg/errs/errs.go:186`, `return errors.Is(err, ErrInvalidConfig) ||`);
  the Stop sentinel is one that `errs.IsInvalid` lists (`:207`, `if errors.Is(err, ErrInvalidData) ||`).

### 1b. An ended context: wrapped in Start, bare in Stop

- `guard.go:45-46`: `if err := ctx.Err(); err != nil {` / `return errs.WrapInvalid(err, "lifecycleguard", "Start",
  "context already ended")`: class invalid.
- `guard.go:89-90`: `if err := ctx.Err(); err != nil {` / `return err`: a bare `context.Canceled` or
  `context.DeadlineExceeded`, which `errs.IsTransient` lists (`pkg/errs/errs.go:168-169`), so `errs.Classify` returns
  transient.
- Not a refusal at entry, but the same context ending, also returned bare:
  - `guard.go:108-109`, `case <-ctx.Done():` / `return ctx.Err()`: Stop's context ends while Stop waits for a Start that
    is still running;
  - inside the cleanup the guard's Stop runs for graph-ingest: `processor/graph-ingest/component.go:1136-1137`,
    `case <-ctx.Done():` / `cleanupErr = errors.Join(cleanupErr, ctx.Err())`, and `:1143`,
    `return errors.Join(cleanupErr, ctx.Err())`.

### 1c. The concurrent-Stop refusal has no test

- `guard.go:112-114`: `if g.stopping {` / `g.mu.Unlock()` / `return errs.WrapTransient(errors.New("stop already in
  progress"), "lifecycleguard", "Stop", "concurrent Stop is unsupported")`.
- The tests (`guard_test.go`): `TestGuardOwnerPassesTheLifecycleSuite` `:105`,
  `TestGuardFailedRollbackLeavesCleanupForStop` `:116`, `TestGuardFailedStopLeavesCleanupForNextStop` `:150`,
  `TestGuardStopWaitsForAnInFlightStart` `:187`, `TestGuardRollsBackAPanickingStart` `:229`,
  `TestGuardStopBeforeStartRefusesLaterStart` `:253`. No test has one Stop inside its cleanup while a second Stop is
  called. `TestGuardStopWaitsForAnInFlightStart` calls two Stops, but the first returns, cancelled at `:211-214`, before
  the second begins (`:216`). `git grep -n "stop already in progress\|concurrent Stop"` over the tests finds nothing.
  The suite does not reach it: `component/lifecycle.go:63`, "Concurrent lifecycle calls are not part of the contract."

### 1d. The cleanup is passed twice, and callers pass different values

- Signatures: `guard.go:41`, `func (g *Guard) Start(ctx context.Context, start, cleanup func(context.Context) error)
  (startErr error) {`; `guard.go:85`, `func (g *Guard) Stop(ctx context.Context, cleanup func(context.Context) error)
  error {`. Start's cleanup is used only for the rollback (`:61`,
  `rollbackErr = lifecyclecleanup.RollbackFailedStart(ctx, cleanup)`); Stop's at `:119`, `err := cleanup(ctx)`.
- graph-ingest: `processor/graph-ingest/component.go:967`, `return c.lifecycle.Start(ctx, c.start, c.release)`;
  `:1074-1080`, `return c.lifecycle.Stop(ctx, func(ctx context.Context) error {` ... `c.logger.Info("component stopped
  gracefully", slog.String("component", "graph-ingest"))` (`:1078`). So a Stop logs and a rollback does not.
- Found beyond the brief: two test helpers in graph-ingest depend on Start's rollback differing from Stop's cleanup.
  - `processor/graph-ingest/lifecycle_owner_test.go:136-138`: `err := component.lifecycle.Start(t.Context(),` /
    `func(context.Context) error { return errStart },` / `func(context.Context) error { return
    component.release(expired) })`. The later `owner.Stop(t.Context())` then runs the component's own closure. This
    test is named in the `lifecycle-suite` scenario "graph-ingest's cleanup fails after a failed start"
    (`openspec/specs/lifecycle-suite/spec.md:137-142`) and in the ledger's `proving_tests`
    (`docs/admission-ledger.yaml:2319-2321`).
  - `processor/graph-ingest/test_owner_support_test.go:93-95` (`leaveGuardCleanupPending`):
    `err := c.lifecycle.Start(tb.Context(),` / `func(context.Context) error { return errStart },` /
    `func(context.Context) error { return errRollback })`. Its one caller,
    `test_owner_test.go:65` (`TestGraphIngestTestOwnerFailedAttemptIsNotRetried`), expects the next Stop to run the
    component's drain (`:66-72`, `drain.called == 1`).
- In the guard's tests, Start's and Stop's cleanups also differ: `guard_test.go:194-198` (Start's cleanup returns nil)
  against `:204` and `:216` (counting closures); `:238-247`; `:255-264`.
- The other graph-ingest test callers pass `c.release` to Start: `test_owner_support_test.go:80`
  (`startGuardAcquiringNothing`) and `:107` (`assertGuardUnused`). Their next Stop runs `release` under either shape.
- Every caller (`gopls references`): `Guard.Start` (`guard.go:41`): `component.go:967`; tests `guard_test.go:36`,
  `:194`, `:238`, `:261`; `lifecycle_owner_test.go:136`; `test_owner_support_test.go:80`, `:93`, `:107`.
  `Guard.Stop` (`guard.go:85`): `component.go:1074`; tests `guard_test.go:40`, `:204`, `:216`, `:244`, `:255`.

### 1e. Found in review: a cleanup that panics cannot run again

- In Stop: `guard.go:116`, `g.stopping = true`; `:119`, `err := cleanup(ctx)`; `:120-121`, `g.mu.Lock()` /
  `g.stopping, g.terminal = false, err == nil`. A panic in the cleanup skips `:120-122`, so `stopping` stays set, and
  every later Stop is refused as concurrent at `:112-114`. The cleanup never runs again.
- In a failed Start's rollback: the deferred function at `guard.go:58-71` calls
  `lifecyclecleanup.RollbackFailedStart(ctx, cleanup)` at `:61`. That helper calls the rollback with no recover
  (`pkg/lifecyclecleanup/lifecyclecleanup.go:55`, `rollbackErr := rollback(ctx)`). A panic there skips `close(done)` and
  `g.startDone = nil` (`:69-70`), so every later Stop waits at `:103-110` until its own context ends and then returns
  that error. The cleanup never runs again, and a later Start is refused as already used (`:49-52`).
- A panicking `start` is handled: the rollback still runs (`committed` stays false, `:57-75`), as
  `TestGuardRollsBackAPanickingStart` (`guard_test.go:229-250`) shows, with a rollback that succeeds. No test has a
  cleanup that panics.
- Real-world trigger: none today. graph-ingest's `release` (`component.go:1087-1096`) has no panic path in this
  inventory's reading, and every cleanup in the tree is a method value. A nil cleanup would panic in Stop
  (`guard.go:119`). By the severity rule (#185) this is MEDIUM at most.

## 2. Every current spelling of the facts

### A context is refused (nil, or already ended)

- In the guard: `guard.go:42-47` (Start), `:86-91` (Stop); the wait path `:108-109` (1b).
- The other lifecycle owner held to the suite: `metric/handler.go:70-75` (`start`),
  `return errs.WrapInvalid(errs.ErrInvalidData, "Server", operation, "nil context")` /
  `return errs.WrapInvalid(err, "Server", operation, "context already ended")`;
  `:195-197` (`Stop`), `return errs.WrapInvalid(errs.ErrInvalidData, "Server", "Stop", "nil context")`.
- How the tree refuses a nil context in production code (`git grep -n -i "nil context\|context cannot be nil\|ctx == nil"
  -- '*.go' ':!*_test.go' ':!internal/harness'`):
  - `errs.WrapInvalid(errs.ErrInvalidData, ..., "nil context")`: 5 sites. `metric/handler.go:71`, `:196`;
    `graph/inference/hierarchy.go:129`; `processor/graph-ingest/component.go:2053`; `guard.go:87`.
  - `errs.WrapInvalid(errors.New("nil context"), ...)`: 13 construction sites in 4 packages. `natsclient/request.go:28`
    is a shared helper (`nilContextErrorOf`), plus `natsclient/client.go:458`, `:651`, `kvspec.go:239`, `:330`,
    `stream.go:484`, `:753`; `natsclient/storagereport/storage_inventory.go:621`;
    `graph/kvcatalog/owned_bucket_retention.go:41`; `internal/cache/cache.go:98`, `coalescing_set.go:79`, `:198`,
    `config.go:109`.
  - Plain `errors.New`: `graph/readiness/publisher.go:69`, `watcher.go:197`, `:254`; `pkg/retry/retry.go:63`;
    `internal/resource/watcher.go:153`; `pkg/lifecycle/manager_query.go:148`, `:180`;
    `internal/dispatch/keyed_pool.go:286`; `graph/exact_entity.go:69`. The readiness change chose this because "no
    caller tells this error apart" and the package has no classified vocabulary
    (`openspec/changes/archive/2026-10-17-readiness-nil-context/design.md`, D1).
  - `pkg/errs.ErrInvalidConfig`: `guard.go:43` only. (`internal/dispatch/keyed_pool.go:125` uses dispatch's own
    `ErrInvalidConfig`, `internal/dispatch/errors.go:11`.)
- An ended context at the start of work. Start and setup methods refuse it at entry
  (`git grep -n -A1 "if err := ctx.Err(); err != nil {"`): `guard.go:45-46` (`WrapInvalid`); `metric/handler.go:73-74`
  (`start`, `WrapInvalid`, the same words as `guard.go:46`); `natsclient/client.go:469-470` (`Connect`,
  `WrapTransient`); `natsclient/stream.go:487-488` (`WrapInvalid`), `:546-547` (`WrapTransient`). Others wrap it with
  `fmt.Errorf` or return it bare.
- An ended context at a stop. Every stop method in production code
  (`git grep -n -E "\) (Stop|Close|Shutdown)\(ctx context\.Context\) error" -- '*.go' ':!*_test.go'`; the two
  `internal/harness/natsfixture` hits are test fixtures) returns the context's error bare when the context wins. None
  turns it into a refusal class:
  - `metric/handler.go:194-203` (`Stop`: the nil check, then no ended check); `:248-249`,
    `if err := ctx.Err(); err != nil {` / `stopErr = errors.Join(stopErr, err)`.
  - `natsclient/client.go:649-652` (`Close`: the nil check only); `:688-689`, `if err := ctx.Err(); err != nil {` /
    `return err`; `:704-705`, `case <-ctx.Done():` / `return ctx.Err()`.
  - `internal/dispatch/keyed_pool.go:284-287` (`Shutdown`: the nil check only); `:304-307`, `case <-ctx.Done():` ...
    `return ctx.Err()`.
  - `internal/cache/coalescing_set.go:196-199` (`Shutdown`: the nil check only); `:213-214`, `case <-ctx.Done():` /
    `return ctx.Err()`.
  - graph-ingest's `Stop` runs the guard: `guard.go:89-90` checks at entry and returns the error bare, the only stop
    in the tree that checks at entry; `:108-109`; and the cleanup's `component.go:1136-1137`, `:1143` (1b).
  - The written rule: `openspec/specs/background-work/spec.md:20-22`, `Shutdown(ctx) error` "stops the work, waits
    for it, and returns `ctx.Err()` when the context ends first". `docs/adr/113-start-context-bounds-start.md:54-55`:
    `Stop(ctx)` and `Shutdown(ctx)` "make the same promise: stop, wait, return the context's error if the bound wins,
    and let a later call try again". Owner ruling, #282 comment 6249934379 point 2, restated at ADR-113 `:61-62`:
    "`Stop(ctx)` is the only stop: nil only after everything is released; if `ctx` ends first it returns that error and
    a later `Stop` tries again."

### A concurrent Stop is refused

- `guard.go:112-114`; `metric/handler.go:199-202`, `return errs.WrapTransient(errors.New("metrics server stop already
  in progress"),` / `"Server", "Stop", "concurrent Stop is unsupported")`. Its test,
  `metric/handler_test.go:323` (`TestServerConcurrentStopIsTypedTransientAndDoesNotHoldLifecycleLock`), polls the
  private `server.stopping` field and `TryLock`s the mutex (`:368-379`).

### A cleanup or acquisition that panics

- Handled for `start` only: `guard.go:57-75` (1e). Not handled for a cleanup, in Stop (`:116-121`) or in the rollback
  (`:61` before `:69-70`). `pkg/lifecyclecleanup/lifecyclecleanup.go:55` does not recover.

### The cleanup an owner runs

- `guard.go:41` and `:85` (1d); `pkg/lifecyclecleanup/lifecyclecleanup.go:36`, `func RollbackFailedStart(parent
  context.Context, rollback func(context.Context) error) error`, which refuses a nil rollback (`:48-49`).
- An owner records at acquisition what its release reads: `component.go:451-454`, "The handles start records for
  release: release reads and clears them."

## 3. Adjacent claims

- Specs. `git grep -n -i "lifecycleguard\|lifecycle guard\|the guard" -- openspec/specs/` finds nothing.
  - `lifecycle-suite` "Portable floor" (`openspec/specs/lifecycle-suite/spec.md:13-28`): nil Start and Stop refused
    (any error, no panic); a pre-cancelled or pre-expired Start refused with the matching context error; repeated
    Stop a no-op.
  - "Failed start whose own cleanup fails" (`:127-147`) names
    `TestLifecycleOwnerFailedCleanupRetainsExactHandlesForLaterStop`.
  - `error-classification` (`openspec/specs/error-classification/spec.md:11`) has one requirement (unclassified means
    fatal) and nothing on nil contexts.
  - `background-work` (`openspec/specs/background-work/spec.md:20-22`): `Shutdown` returns `ctx.Err()` (section 2).
  - `component-registration` and `harness-boundaries` do not mention the guard.
  - The change already carries `skip_specs: true` (`openspec/changes/lifecycleguard-tidy/.openspec.yaml:3`).
- Suite checks that read these errors: `internal/harness/lifecycletest/lifecycletest.go:136-149`
  `CheckNilContextsRefused` (non-nil, not a panic: `notARefusal`, `:358-361`); `:154-172`
  `CheckPreCancelledStartRefused` (`errors.Is` context error at `:157`, `:162`); `:206-225`
  `CheckAbortStopPreservesCause` (`:220`,
  `case !errors.Is(err, stopCtx.Err()) && !errors.Is(err, context.Cause(stopCtx)):`).
- Who matches on the guard's refusal classes. `git grep -n "ErrInvalidConfig\|ErrInvalidData" -- '*.go'
  ':!pkg/errs'` gives 71 lines. The test hits are `internal/dispatch/keyed_pool_test.go:92`, `:110` (dispatch's own
  sentinel), `message/generic_json_shape_test.go:113` and `message/payload_test.go:41`, `:45`, `:48`; none reads a guard
  or graph-ingest Start/Stop error. `IsInvalid|IsTransient|Classify(|ErrorInvalid` over graph-ingest's tests,
  `internal/lifecycleguard`, `internal/harness` and `component` tests: every graph-ingest hit is an ingest, mutation
  or query error. The tests that read a Start or Stop error read only `err == nil` (`lifecycle_owner_test.go:244`) or
  `ErrorIs(..., context.DeadlineExceeded)` (`component_test.go:937`, `component_integration_test.go:119-120`). No
  production code calls graph-ingest's `Start` or `Stop`; the tree has no `service` package yet.
- ADRs:
  - ADR-108 (`docs/adr/108-processor-shell.md:36`, `:56`): the guard is the lifecycle state machine; behaviour only.
  - ADR-113 (`docs/adr/113-start-context-bounds-start.md:16`): PR 2 comes after this tidy. `:54-55`: one promise for
    Stop and Shutdown. `:61-62`: the Stop rule. `:88-93`: the guard does not hold the run context. `:105-106`: "the
    guard changes nothing, because the cancel order is the owner's". `:97-100`: PR 2 changes two suite checks and the
    guard's test owner.
- Ledger. `internal/lifecycleguard` has no row (its own code). It is named in graph-ingest's row
  (`docs/admission-ledger.yaml:2301`, "(6) owner lifecycle guarded by internal/lifecycleguard with RollbackFailedStart
  (SS#1411, design D13)") and its dependencies (`:2308`), and in the `internal/lifecyclecleanup` row (`:1901`). None
  states the refusal classes or how the cleanup is passed.
- Rules. `AGENTS.md:82` and `.agents/rules-index.md:28` (that row cites "`internal/lifecycleguard/guard.go` package
  doc"); `.agents/contracts/semengine-developer.md:314-317`; `semengine-reviewer.md:333-335`;
  `docs/repository-map.md:74`. None states a signature. `git grep -n -e "Stop(ctx, " -e "Guard.Stop" -e "Guard.Start"
  -- ':!*.go' ':!openspec/changes/archive'` finds nothing.
- `component/lifecycle.go:46-47`: "Start and Stop refuse a nil context, and Start refuses a context that has already
  ended" (the `LifecycleComponent` contract; PR 2 rewrites the Stop rule beside it, `:49-53`).
- Design D13 (`openspec/changes/archive/2026-10-12-setup-04a-02-ingest-kernel/design.md:744-753`) lists what a component
  composing the guard observes. Its spec home is "Portable floor", "whose checks are these behaviors except that retry;
  no delta" (`:757`). The list names no concurrent Stop, no single cleanup and no panicking cleanup.
- Open PRs (`gh pr list --state open --json number,title,changedFiles,files`): only #308 (this claim; three files
  under `openspec/changes/lifecycleguard-tidy/`). PR 2 (#282's order) and PR 3 (#291) are unclaimed.
- Open issues (`gh issue list --state open --search <q>` for `lifecycleguard`, `lifecycle guard`, `guard.go`,
  `concurrent Stop`, `RollbackFailedStart`, `lifecyclecleanup`, `lifecycle`):
  - #196: the adoption tracker for the guard, and this work's source; names the package.
  - #109 (the processor shell, which composes "the lifecycle state machine", ruling comment): adjacent, not the package.
  - #291 (graph-ingest `Start` during a reconnect, PR 3) and #301 (test-only NATS fault fields, PR 2's design
    question): graph-ingest `start`, not the guard.
  - Name the word, not the package: #280 (`pkg/lifecycle`), #80, #81 ("guard" for a test-naming guard), #114, #9,
    #24, #29, #33, #86, #129, #195, #7, #20.
- #282 was closed by #304's squash merge at `2026-10-18T13:19:22Z`: the words "does not close #282" in that PR's body
  were read as a closing keyword. It was reopened by hand on 2026-10-18 so that PR 2 retires it, and #308's body was
  reworded to declare no close (task 3.1 checks `closingIssuesReferences`).

## 4. Who it is for

No new exported symbol, subject, bucket or config field. `Guard.Stop` loses a parameter. Both methods are plumbing:
their production callers are `component.go:967` and `:1074`. `internal/` keeps them out of reach of consumers.
graph-ingest's exported `Start`/`Stop` keep their signatures. Consumer readers of those: semsparql `51ac535`, test
code only. `cmd/semsparql/main_test.go:530` (`if err := ingest.Start(ctx); err != nil {`), `:536`
(`_ = ingest.Stop(stopCtx)`); `reader/nats_test.go:641`, `:647`; `server/diekm_integration_test.go:361`, `:367`. Start's
error is checked only for nil, and Stop's error is discarded. semvocab imports no graph-ingest. The other starter
consumers depend on SemStreams, not SemEngine (`docs/inventory-scope.md` table).

## 5. Problem shape

Two shapes.

- Admit-or-refuse at a seam. Closest instance: `metric.Server.start`, `metric/handler.go:70-75`, the other lifecycle
  owner under the suite. A package-local refusal helper exists in `natsclient/request.go:20-29` (`nilContextError`,
  `nilContextErrorOf`). The stop side's closest instances are the four stop methods in section 2, none of which
  refuses an ended context at entry.
- Record at acquisition what release needs. Closest instance: graph-ingest's own handles, `component.go:451-454`.
- Neither establishes a new primitive. The guard itself was the establishing change (D13), and its adoption sweep is
  #196.

## 6. Pin decision records

None: no package is carried from the pin. The guard's own records are ADR-108 and ADR-113 (category 3).

## Context retention (architect contract, Design discipline)

- `guard.go:23-34`: fields `mu`, `used`, `terminal`, `stopping`, `startDone`; no context.
- graph-ingest keeps only private cancels: `component.go:456` `cancel context.CancelFunc`, `:550-551`.
- `pkg/lifecyclecleanup/lifecyclecleanup.go:52`: the bounded terminal detachment the contract allows.

## Same-class collision table and intent check

Not triggered: no new durable, communication or coordination primitive, and no boundary moves.

## Adopter seam

The guard is internal: its adopters are owners in this module (#196's rows), and graph-ingest is its one adopter
today.

1. What an adopter must know today: pass the same cleanup to `Start` and `Stop`, and never let the cleanup panic.
   Nothing says either or enforces it.
2. If they do nothing: two values are accepted silently. The rollback and a Stop then run different functions (1d:
   graph-ingest's Stop logs and its rollback does not; two test helpers rely on the difference). A cleanup that panics
   leaves every later Stop refused or waiting (1e).
3. Where they find out: nowhere for the first; for the second, a Stop that fails without saying why.
4. What they should have to know: nothing.

For graph-ingest's consumers: semsparql's fixtures check Start's error for nil and discard Stop's error (category 4).
