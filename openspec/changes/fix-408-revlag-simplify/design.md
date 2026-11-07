# Design: fix-408-revlag-simplify

Track: design. Owner ruling #408 comment 6714095087 (all three). Inventory: `inventory.md` (base `6190ce2d`).

## Context

- One caller observes, in delivery order on the watch goroutine: `graph/follower/follower.go:239`, `:264`. A new
  watermark per `Start` (`:176`, `processor/graph-index/component.go:236`). Completion runs on keyed-pool lanes and
  the panic path (`processor/graph-index/index.go:89-104`, `:169-172`). The per-key drain serves the coalescer and
  deletes (`index.go:122-139`). The bootstrap target is read from `Observed` (`follower.go:234`, ADR-084 D2).
  Staleness is reported, not gated (ADR-085 D2 `:105-110`).
- Removed by this change: the walk-back `watermark.go:122-131`, the re-observe branch `:112-115`, and the covered
  estimate `:58-71`, `:174-206`.
- Owner Q1 (PR #407 comment 6388065578) still binds. `Complete` costs time in proportion to the key's own in-flight
  revisions, never all of them. Retained state is in proportion to the in-flight count (archived D5 claims 1 and 2).
- Ascending delivery is ADR-066's claim for the pin (`:221-224`). It is **not promised** by any probe at nats.go
  v1.55.0 / nats-server v2.15.2. That the server's commit times rise with revision is also **not promised**.

## Goals / Non-Goals

Goals: the three ruled items, one definition of the time, a refusal where the watermark used to trust its caller.
Non-goals: graph-ingest's backlog staleness proxy (`processor/graph-ingest/readiness.go:332-345`); `pkg/graphview`'s
`Applied`; renaming `IndexedAt` or `IndexStatusInputs.IndexedAt` (owner question 2); staleness readers outside the repo.

## D1. The time rule

`IndexedAt` returns `(floor, at)` read together:

| State | floor | at |
| --- | --- | --- |
| nothing observed | 0 | zero |
| nothing in flight | `Observed` | the commit time accepted with revision `Observed` |
| something in flight | lowest in-flight − 1 | the commit time accepted with the lowest in-flight revision |

Edge cases. Zero passed to `Observe` for the chosen revision gives zero ("unknown"), even when later in-flight
revisions have times. A completion above an in-flight revision, of any key, changes nothing. A delete is a revision
like any other: its `Complete(key, rev)` drains the key's lower revisions, and `at` then follows the table. A
`Complete` ahead of delivery never yields the unobserved revision's time. Under accepted (ascending) observation the
revision whose time is reported never decreases.

Meaning: `now − at` is how long the oldest delivered write the index lacks has waited. When none is in flight, it is
the age of the newest delivered write, as today. Undelivered writes are all newer than any in-flight one. So the rule is
exact for delivered writes, and with nothing in flight it is an upper bound on the true age. Options: (a) this rule,
which the owner ruled; (b) keep the pin's covered estimate. It costs a second bookkeeping path and a number whose
error depends on completion order. Who must know: readers of `staleness_ms` see a lower number while behind, and a
number from the first delivery at cold start. Proof: D5 and D6 tests.

## D2. `Observe` and a revision at or below `Observed`

Options:

- (a) Ignore it silently. This fails open: an in-flight lower revision is invisible to the floor.
- (b) Panic. Under a nats.go fault this crashes the process for a condition the follower can declare.
- (c) Return an error and change nothing; the follower declares the feed lost (`follower.go:287-292`).
- (d) Keep sorted insertion. This is the walk-back the owner dropped.

**Chosen: (c).** It is fail-closed and declared (AGENTS.md § 1), through the follower's existing latch. Revision 0 is
refused by the same rule, since `Observed` starts at 0. This replaces "ignored". Re-observing an in-flight revision is
refused too, and the revision keeps its key and time.

Follower behaviour: on a refusal `Stuck` names the lost feed, the next envelope is `degraded`, and `QueryReady` returns
`index_not_ready`. The entry is still handed to `Deliver`; its later `Complete` drains only that key's lower revisions.
Adopter seam: a future producer learns the ordering rule from a returned error, not from a doc. Ignoring the error is
not caught by lint (`revive.toml` has no unhandled-error rule); review catches it.

## D3. In-flight bookkeeping

What the code must show (the developer chooses the structure):

1. `Observe` is constant time.
2. `Complete` costs time in proportion to the drained key's in-flight revisions (Q1).
3. A floor read is constant or amortized constant.
4. Retained per-revision state is proportional to the in-flight count, also while one revision is stuck and others
   complete above it.

Options:

- (a) Keep `container/list` but only `PushBack`. With the key map holding each key's list elements, the separate
  `pending` map can go.
- (b) A slice queue in observation order with lazy removal and compaction once dead entries outnumber live ones.
- (c) A plain slice scanned by `Complete`. Rejected: it breaks claim 2 (Q1).

Recommended: (a), the smallest step from today's code. Owner question 1 asks whether "drop the `container/list`
walk-back" also meant dropping the list. A plain queue without compaction fails claim 4, which
`TestWatermark_RetentionTracksInFlight` measures (1 MiB bound over 1,000,000 completions). Claims 2 and 3 are shown
by a benchmark at 10,000 and 40,000 in flight in `notes/2.1.md`, as evidence, not a gate.

## D4. The #408 test

`TestWatermark_ConcurrentReadersAndWriters` keeps its writers (one observer, 8 completing workers, repeating keys) and
a free-running reader. It adds checkpoints. The test asks the reader for a read and receives the value back, so the
read is ordered after the writes it waits on:

- **A:** after revisions 1..1,000 are observed and every `Complete` has returned: `(1000, commitAt(1000))`.
- **B:** after 1,001..2,000 are observed and all but 1,500 and 2,000 complete: `(1499, commitAt(1500))`. The final
  completions are withheld until B is read.
- **Final:** after 1,500 and 2,000 complete and every writer has joined: `(2000, commitAt(2000))`, `Observed` 2,000.

Every read, free or checkpoint, goes through one no-fall comparison whose `last` starts at the first read. A
checkpoint read therefore always runs that comparison against a nonzero earlier floor. No sleep, no read-count
assertion; `reads` is logged only.

Shown able to fail (`task mutate:check`, one test per mutant):

- N4, the floor from the highest in-flight revision: B reads 1,999.
- N5, a floor that `Complete` does not advance: A reads 999.

Gates from #408: `go test -race -count=1 -cpu=1` and `go test -count=5 -cpu=1 -shuffle=on` on `./internal/revlag`.

## D5. Generated checks (`docs/testing.md:436-477`)

Needed: an order-dependent history with stated laws. Kept as a Rapid history, with the test's own record as the
reference model. The record holds each in-flight revision's key and time, plus the highest accepted revision and its
time. It now also predicts `at` by D1's table, and every refusal.

`pinref_test.go` is deleted. Options:

- (a) Delete it. The record already checks the floor independently.
- (b) Keep it for the floor only. That keeps 224 lines of the replaced algorithm to check a rule the record checks.
  It also only works on ascending histories, since its lower/re-observe results now differ.

Chosen: (a).

The `lower`, `reobserve` and zero-revision ops are kept: each must return an error and change no result. The fixed
prefix `Observe(10,A)`, `Observe(11,B)`, `Complete(A,10)` gives `(10, commitAt(11))`, a boundary the two rules disagree
on. Then `Observe(11,C)` is refused, so the refusal assertion runs in every case. The final drain runs the caught-up
row. The floor-never-falls check runs on every step. The PR records the seed and the completed check counts.

## D6. Mutation rows (`notes/2.1.md`)

| Row | Wrong behaviour | Test expected to fail |
| --- | --- | --- |
| N1 | `Observe` accepts a revision ≤ `Observed` | `TestWatermark_ObserveRefusesNonAscending` |
| N2 | `at` from the highest in-flight revision | `TestWatermark_TimeIsOldestInFlight` |
| N3 | `at` from the last drained revision | `TestWatermark_CompletionAboveInFlightKeepsTime` |
| N4 | floor from the highest in-flight revision | `TestWatermark_ConcurrentReadersAndWriters` |
| N5 | `Complete` leaves the floor where it was | `TestWatermark_ConcurrentReadersAndWriters` |
| N6 | a drained revision stays in the order structure | `TestWatermark_RetentionTracksInFlight` |
| F1 | follower ignores `Observe`'s error | `TestFollowerDeclaresLostFeedOnOutOfOrderDelivery` |

## D7. ADR-083 decision 3 amendment

Appended at the end of `docs/adr/083-readiness-as-distributed-state.md`, so ADR-109's line citations (`:192-203`) stay
valid. The pin text is unchanged.

> **SemEngine amendment to decision 3 (2026-11-07, #408).** For SemEngine's revision-lag producers
> (`internal/revlag`, held by `graph/follower` for graph-index), `staleness_ms` is `0` when `Ready`. Otherwise it is
> `now −` the commit time of the oldest delivered write not yet applied (the lowest in-flight revision), or, when
> none is in flight, `now −` the commit time of the newest delivered write. It replaces "the newest fully-covered
> revision". The pin carried that time forward as an estimate that dropped the times of completions above the floor,
> so it read older than its own definition while behind, at the cost of a second bookkeeping path. The oldest
> unapplied write is already held for the floor. Its time is exact for delivered writes and is the quantity the
> backlog projection's `OldestOutstandingAt` names. The time is still the NATS server's commit time; that commit
> times rise with revision is not promised, so the rule is stated by revision. Staleness stays reported, never gating
> (ADR-085 D2). Owner ruling: #408 comment 6714095087.

## D8. Docs, ledger, map

These change: the `revision-watermark` Purpose; the ledger `pkg/revlag` row (contract, dependencies if
`container/list` goes, known_risks: no "caller keeps Observe ascending", no "older, never newer", #408 retired;
proving_tests: no `pinref_test.go`); the ledger ADR-083 row (contract "decision text as at the pin, decision 3 amended
in an appended section, #408"); and `docs/repository-map.md:98`.

Go doc comments change at `graph/readiness/index_status.go:23-26`, `graph/index_status.go:86-88` and `:106-108` (the
FLOOR sentence becomes "only a delivered write moves it"), `graph/follower/follower.go:263`, and the `revlag` package
and method comments. `pkg/graphview/view.go:356-360` ("mirrors") becomes "is the newest applied write's; graph-index
reports its oldest unapplied write's".

## D9. Issues and merge order

Issue #408 is retired (`Closes #408`, tasks 2.1-2.2). #108 stays: the graph-index resume point; it says the watermark
stays, and it does. #110 names the word only. Draft #544 does not overlap the edited lines; whichever lands second
merges `origin/main`.

## Risks / Trade-offs

- With nothing in flight and writes still undelivered, `at` over-reports (upper bound). This is unchanged from today.
- The two producers now differ by at most adjacent writes: graph-ingest uses the newest applied, graph-index the oldest
  unapplied. Both are reported, never gating.
- A nats.go regression in ordering becomes a lost feed (restart) instead of a wrong floor. That is the intended trade.

## Migration Plan

No state migration. `staleness_ms` meaning per D7. The one `Observe` caller changes in task 2.2.

## Open Questions

1. **Owner:** did "drop the `container/list` walk-back" mean the walk-back only (recommended D3 (a)) or also the list
   (D3 (b), with compaction)?
2. **Owner:** keep the names `IndexedAt` (method and readiness input), or rename the input to `OldestUnappliedAt`?
   Recommended: keep. A rename widens the change to `graph-readiness`.
