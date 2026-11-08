# Design: fix-408-revlag-simplify

Track: design. Owner ruling #408 comment 6714095087 (all three). Inventory: `inventory.md` (base `6190ce2d`).

## Context

- One caller observes, in delivery order on the watch goroutine (`graph/follower/follower.go:239`, `:264`). There is
  a new watermark per `Start` (`:176`). `Complete` runs on keyed-pool lanes and the panic path
  (`processor/graph-index/index.go:89-104`, `:169-172`). `Observed` sets the bootstrap target (`follower.go:234`).
  Staleness is reported, never gated (ADR-085 D2).
- Removed: the walk-back (`watermark.go:122-131`), the re-observe branch (`:112-115`), and the covered estimate
  (`:58-71`, `:174-206`).
- Owner Q1 (PR #407 comment 6388065578) still binds. `Complete` must not scan every in-flight revision, and retained
  state stays in proportion to the revisions in flight.
- Not promised, and not probed at nats.go v1.55.0 / nats-server v2.15.2: ascending delivery (ADR-066 `:221-224`),
  and commit times that rise with revision.

## Goals / Non-Goals

- Goals: the three ruled items; one definition of the time; a refusal where the watermark used to trust its caller.
- Non-goals: graph-ingest's staleness proxy (`processor/graph-ingest/readiness.go:332-345`); `pkg/graphview`
  `Applied`; renaming `IndexedAt` (owner question 2); staleness readers outside the repository; #482.

## D1. The time rule

| State | floor | at |
| --- | --- | --- |
| nothing observed | 0 | zero |
| nothing in flight | `Observed` | commit time accepted with revision `Observed` |
| something in flight | lowest in-flight − 1 | commit time accepted with the lowest in-flight revision |

Edge cases:

- If the chosen revision was observed with the zero time, `at` is zero (unknown).
- A completion that leaves the lowest in-flight revision in flight changes neither floor nor time. A same-key
  completion above it drains it, and the table then applies.
- A delete is a revision like any other. Its `Complete(key, rev)` drains the key's lower revisions.
- `Complete` ahead of delivery never yields the time of a revision that was never observed.
- Under accepted observation, the revision whose time is reported never decreases.

`now − at` is how long the oldest delivered write the index lacks has waited. This is exact for delivered writes.
With nothing in flight it is an upper bound, as it is today.

Options: (a) this rule, as ruled; (b) the pin's covered estimate, which needs a second bookkeeping path and gives a
number whose error depends on completion order.

## D2. `Observe` at or below `Observed`

Options:

- (a) Ignore the revision. Rejected: it fails open.
- (b) Panic. Rejected: a nats.go fault would crash the process over a condition the follower can declare.
- (c) Return an error and change nothing. **Chosen**: it fails closed and is declared.
- (d) Keep sorted insertion. Rejected: that is the walk-back the owner dropped.

Revision 0 falls under the same rule. On a refusal, the follower declares the feed lost (`follower.go:287-292`):

- `Stuck` names it.
- The envelope reads `degraded`.
- `QueryReady` returns `index_not_ready`.
- The entry is still delivered.

Adopter seam: the ordering rule now arrives as an error, not a doc. No lint rule catches an ignored error; review
does.

## D3. In-flight bookkeeping

What the code must show; the developer picks the structure:

1. `Observe` is constant time.
2. `Complete` costs time in proportion to the drained key's in-flight revisions (Q1).
3. A floor read is amortized constant.
4. Retained state stays in proportion to the revisions in flight, also with one revision stuck.

Options:

- (a) `container/list` with `PushBack` only, the key map holding list elements.
- (b) A slice queue with lazy removal and compaction.
- (c) A scanned slice. Rejected: it breaks claim 2.

Recommended: (a). Owner question 1 asks which the ruling meant. Claim 4 is proved by
`TestWatermark_RetentionTracksInFlight`. Claims 2 and 3 are shown by a benchmark at 10,000 and 40,000 in flight
(`notes/2.1.md`), recorded as evidence, not as a gate.

## D4. The #408 test

`TestWatermark_ConcurrentReadersAndWriters` keeps its writers (one observer, 8 completing workers) and its
free-running reader. Keys stay `r%16`, except revisions 1,500 and 2,000: each gets a key no other revision uses, so no
other completion drains them. The test asks the reader for a read and receives the value back:

- **A:** after 1..1,000 are observed and every `Complete` has returned. Expected `(1000, commitAt(1000))`.
- **B:** after 1,001..2,000 are observed and all but 1,500 and 2,000 have completed. Expected
  `(1499, commitAt(1500))`. The completions of 1,500 and 2,000 are withheld until B is read.
- **Final:** after those complete and every writer has joined. Expected `(2000, commitAt(2000))`, `Observed` 2,000.

The exact values at A and B are the #408 proof: a reader separated from writer progress fails them. Every read also
passes a no-fall comparison against the reader's previous read. That comparison is a stress check: no mutant below
targets it. There are no sleeps; `reads` is logged only. Gates: `go test -race -count=1 -cpu=1` and
`go test -count=5 -cpu=1 -shuffle=on` on `./internal/revlag`.

## D5. Generated checks and `pinref_test.go`

Needed: an order-dependent history with stated laws. `TestPropWatermarkHistory` keeps Rapid, and the test's own record
becomes the reference model: in-flight revisions with key and time, plus the highest accepted revision and its time. It
predicts the floor, `at` (D1) and every refusal.

`pinref_test.go` is deleted. Options:

- (a) Delete it. Chosen.
- (b) Keep it for the floor only: 224 lines that check a rule the record already checks, valid only on ascending
  histories.

The `lower`, `reobserve` and zero-revision ops stay; each must return an error and change no result. The prefix
`Observe(10,A)`, `Observe(11,B)`, `Complete(A,10)` expects `(10, commitAt(11))`, then a refused `Observe(11,C)`. The
final drain covers the caught-up row.

Retention: the `len(w.pending)` reads go. `TestWatermark_RetentionTracksInFlight` gains a no-stuck case. Both cases
bound live-heap growth below 1 MiB over 1,000,000 completions, the restated "Retained state tracks in-flight work"
scenarios.

## D6. Mutation rows (`notes/2.1.md`)

| Row | Wrong behaviour | Assertion it trips |
| --- | --- | --- |
| N1 | `Observe` accepts a revision ≤ `Observed` | `TestWatermark_ObserveRefusesNonAscending`: no error returned |
| N2 | `at` from the highest in-flight revision | `TestWatermark_TimeIsOldestInFlight`: time 100's, want 50's |
| N3 | `at` from the last drained revision | `TestWatermark_CompletionAboveInFlightKeepsTime`: 72's, want 50's |
| N4 | floor is the highest in-flight revision − 1 | `ConcurrentReadersAndWriters` checkpoint B: 1,999, want 1,499 |
| N5 | the floor is recomputed only in `Observe`, so `IndexedAt` returns the value from the last `Observe` | checkpoint A: ≤ 999, want 1,000 |
| N6 | a drained revision stays in the order structure | `RetentionTracksInFlight`: heap grows ≥ 1 MiB |
| F1 | the follower ignores `Observe`'s error | `TestFollowerDeclaresLostFeedOnOutOfOrderDelivery`: `Stuck` empty |

## D7. ADR-083 and ADR-109

Appended at the end of `docs/adr/083-readiness-as-distributed-state.md`:

> **SemEngine amendment to decision 3 (2026-11-07, #408).** For SemEngine's revision-lag producers
> (`internal/revlag`, held by `graph/follower` for graph-index), `staleness_ms` is `0` when `Ready` or when the
> commit time is unknown. Otherwise it is `now −` the commit time of the oldest delivered write not yet applied (the
> lowest in-flight revision), or, when none is in flight, `now −` the commit time of the newest delivered write. It
> replaces "the newest fully-covered revision". The pin carried that time forward as an estimate that dropped the
> times of completions above the floor, so while behind it read older than its own definition and needed a second
> bookkeeping path. The oldest unapplied write is already held for the floor; its time is exact for delivered writes
> and is the quantity `OldestOutstandingAt` names for backlog producers. That commit times rise with revision is not
> promised, so the rule is stated by revision. Staleness stays reported, never gating (ADR-085 D2). Owner ruling:
> #408 comment 6714095087.

Edits in place, each keeping its line count, so ADR-109's line citations stay valid:

- ADR-083 `:7`: "The decision text is unchanged" becomes "The decision text is unchanged; decision 3's time is
  amended at the end (#408)".
- ADR-083 `:200`: "keeps D3" becomes "keeps D3 (its time amended at the end, #408)".
- ADR-109 `:200`, the row's verdict: "Kept: reported, never gating; time amended (#408)".

## D8. Docs and ledger

- Go doc comments to change:
  - `graph/readiness/index_status.go:23-26`, `:38`
  - `graph/index_status.go:86-88`, `:94-96` ("nothing covered yet" becomes "nothing observed yet"), `:106-108`
  - `graph/follower/follower.go:263`
  - `pkg/graphview/view.go:356-360` ("mirrors")
  - the `revlag` package and method comments
- Ledger rows `pkg/revlag` and ADR-083.
- `docs/repository-map.md:98`.
- The `revision-watermark` spec's Purpose.

## D9. Issues and merge order

- #408 is retired by tasks 2.1 and 2.2.
- #482 stays. Its work is a docs-only port-note fix; it edits ADR-083 `:12-15`, not the lines D7 edits. If it lands
  first, task 4.1 merges `origin/main` and rechecks D7's lines.
- #108 stays: the watermark is kept.
- PR #544 does not overlap the lines this change edits; whichever lands second merges `origin/main`.

## Risks / Trade-offs

- graph-ingest takes `staleness_ms` from the newest applied write, graph-index from the oldest unapplied one. Both
  are reported, never gating.
- A nats.go ordering regression becomes a lost feed, which needs a restart, instead of a wrong floor.

## Migration Plan

No state migration. `staleness_ms` takes the meaning in D7.

## Open Questions

1. **Owner:** did "drop the `container/list` walk-back" mean the walk-back only (D3 (a), recommended), or the list
   too (D3 (b))?
2. **Owner:** keep the name `IndexedAt` (method and readiness input), or rename the input to `OldestUnappliedAt`?
   Recommended: keep it; a rename widens the change to `graph-readiness`.
