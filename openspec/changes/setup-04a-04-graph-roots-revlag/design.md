# Design: setup-04a-04-graph-roots-revlag

Pin `8b99efe9`. Ledger row: `pkg/revlag` (D10). Inventory: `inventory.md` (not yet reviewed). Pre-owner design review:
round 1 findings addressed (PR #407). Owner acceptance: Q1 answered 2026-10-24 (see Owner rulings); design acceptance
follows review.

## Options

- A. Carry byte-equal to `internal/revlag`. `task ledger:check` allows the move (inventory, "Placement facts"). Cost:
  lands `Indexed()`, which has no production reader at the pin (inventory sweep item 4); the surface audit does not
  keep dead surface (`.agents/protocol.md:56-58`).
- B. Adapt to `internal/revlag`: drop `Indexed()`, reword the doc lines that name it, add the tests the pin lacks, and
  a spec.
- C. B, plus `Complete` and the floor read stop scanning every in-flight revision (D5). Chosen: the owner ruled Q1
  yes.
- D. Keep `pkg/revlag`. Cost: breaks the standing placement rule (D1) with no consumer or signature to justify it.
- E. Do nothing until graph-index. Cost: departs from the owner's order (#9 comment 6386856657); graph-index's PR, at
  4,527 pin lines with a waiver, would carry this design as well.

## D1. Home: `internal/revlag`

The standing rule (#9 comment 5953295358) makes a package public only when a starter consumer imports it or an
exported signature in a public package names its type. Neither holds: no starter consumer's Go code names `revlag`,
and at the pin both readers hold `*revlag.Watermark` in unexported fields (inventory, "Who reads it"). So the package
moves to `internal/revlag`, as `pkg/dispatch` and `pkg/cache` did. The package name stays `revlag`. This is not an
owner question: the rule decides it.

Mentions of `pkg/revlag` in the tree's ADRs: ADR-066 is a pin record, and its port note is edited (D11). ADR-108 is
SemEngine's own accepted record (`docs/adr/108-processor-shell.md:25`, `:130`). It names the package as the pin had it
when the record was accepted, and it is not edited here: the ledger row's `source_path` to `destination` maps the name.

## D2. Disposition: `adapt`

Required: `Indexed()` goes. The architect contract counts a caller in an admitted package that is not ported yet
(`.agents/contracts/semengine-architect.md:71-72`), and `Indexed()` has 30 such callers, all in graph-index and
graph-embedding tests. Tests are not readers. The reviewer contract judges an exported symbol whose only callers are
tests, with no contract line, as dead surface, and dead surface carried from the pin is not protected
(`.agents/contracts/semengine-reviewer.md:413-418`; `.agents/protocol.md:56-58`). `IndexedAt()` returns the same
floor (sweep items 1 and 4). Cost: those 30 call sites, plus 17 in this package's own test, read the floor from
`IndexedAt()` when they port. Because the file changes, the row is `adapt`.

## D3. API as ported

| Identifier | Disposition | Reason |
| --- | --- | --- |
| `Watermark` | kept | readers' fields (pin graph-index `component.go:274`, graph-embedding `:337`) |
| `New` | kept | graph-index `:706`, graph-embedding `:732` |
| `(*Watermark).Observe(r uint64, key string, commitAt time.Time)` | kept | graph-index `:1031`, graph-embedding `:1516` |
| `(*Watermark).Complete(key string, rev uint64)` | kept | graph-index `:1146`, `:1364`; graph-embedding `readiness.go:43` |
| `(*Watermark).Observed() uint64` | kept | graph-index `:1017`, graph-embedding `:1474` (the first-build target) |
| `(*Watermark).IndexedAt() (indexed uint64, commitAt time.Time)` | kept | graph-index `watermark.go:59`, graph-embedding `readiness.go:126` |
| `(*Watermark).Indexed() uint64` | dropped | no production reader; same value as `IndexedAt`'s first result |

Nothing is renamed. Signatures are unchanged.

## D4. Doc comments

- The eleven lines that name `Indexed()` (inventory, sweep) name "the floor" or `IndexedAt` instead. The text of
  `Indexed`'s own doc (pin `:161-163`) moves to `IndexedAt`'s doc. This follows from D2, so it is required.
- Pin `:32-35`, the nats.go and nats-server ordering claim, stays word for word: carried text under Bounded port scope,
  with no probe in this tree. The spec (D6) states ascending order as the caller's precondition instead, so it promises
  nothing about the library (ADR-112; `.agents/contracts/semengine-architect.md:425-431`).
- Comments that describe the pin's scan (pin `:47-50`, `:144-159`, `:202-212`) are rewritten to describe the code as
  ported (D5). They must keep stating the memory and commit-time trade the pin documents (`:51-62`).
- Optional, not done: `gh#590` at pin `:15` is SemStreams' issue. ADR-083's port note already says so.
- The package comment's references to ADR-066 and ADR-083 resolve in this tree.

## D5. Cost of `Complete` and the floor read (ruled: Q1 yes)

At the pin, every `Complete` and every floor read scans all in-flight revisions. Draining N in-flight revisions
therefore costs time quadratic in N (PR #407 probe comment (issuecomment-6388227813): 10,000 took 0.87 s, 40,000 took
14.5 s, all under the
watermark's lock). This bites at graph-embedding's cold start, where every entity waiting for an embedding is in
flight, and on graph-index's opt-in coalesced path.

What the ported code must show. The structure is the developer's choice.

1. `Complete` costs time proportional to the drained key's own in-flight revisions. A floor read costs at most
   amortized logarithmic time in the in-flight count.
2. Retained per-revision state stays proportional to the in-flight count, including while one revision is stuck in
   flight and others complete above it. A structure that keeps a stale entry per completed revision fails this.
3. Every result (the floor, its commit time and `Observed`) equals the pin's for every call sequence: any order,
   revisions observed again, revisions completed ahead of delivery, and zero commit times.

Proof: claim 3 by D8, which compares against the pin's code itself. Claim 2 by `TestWatermark_RetentionTracksInFlight`
(D7). Claim 1 by a benchmark at 10,000 and 40,000 in flight, recorded in `notes/3.2.md` as evidence, not as a gate.
The 2.1 tests pass unchanged before and after task 3.2.

## D6. Spec: new capability `revision-watermark`

Precedent: `internal/dispatch` has its own capability, `keyed-dispatch`. The delta states seven requirements: the
floor (including a revision observed again), key-scoped completion, `Observed`, ascending-order monotonicity, the
commit time, retained state, and concurrent use. Each is under 500 characters. At archive, the Purpose reads:

> `internal/revlag.Watermark` tracks how far a pipeline fed by a NATS KV watch has caught up: the floor at or below
> which every delivered revision has completed, the highest revision delivered, and the commit time of a covered
> revision. graph-index and graph-embedding feed their readiness envelopes from it (pin ADR-066 section 1, ADR-083
> decision 3); the envelope is `graph`'s.

Invariants and their spec homes: the floor (requirement "The floor is the low-water mark of in-flight revisions") and
monotonicity ("The floor never falls under ascending observation"). D8 checks these two against its own record of
what is in flight. Every other result it checks for equality with the pin.

## D7. Proving tests

`internal/revlag/watermark_test.go`, task 2.1:

- The pin's 13 tests, unchanged except that each `Indexed()` read becomes the floor from `IndexedAt()`. They already
  meet `AGENTS.md` section 3: fixed synthetic times, no sleep, skip, port or build tag. Their oracle is hand-computed
  literals over sparse revision sets. `TestWatermark_Concurrent` reads after `wg.Wait()`, the event that orders the
  read after the writes (#392). `TestWatermark_MemoryDoesNotGrowWithCompletions` reads `len(w.pending)`; if 3.2
  renames or splits that state, the test follows it, and `TestWatermark_RetentionTracksInFlight` is the
  structure-independent check.
- New: `TestWatermark_ObservedIsHighestDelivered` (pin `Observed` is 0% covered locally),
  `TestWatermark_DrainAtFloorIsCovered` (`Observe(10,A)`, `Observe(11,B)`, `Complete(A,10)` gives `(10, commit time
  of 10)`: the boundary the pin tests miss), `TestWatermark_CompleteWithNothingInFlight` (a `Complete` on a key with
  nothing in flight changes no result).
- New: `TestWatermark_ConcurrentReadersAndWriters`, the real wiring. One goroutine observes revisions 1 to 2,000 in
  order and hands each to a pool of workers that complete it, while a reader goroutine calls `IndexedAt` and
  `Observed` in a loop. The reader checks that each floor is no lower than the one before. After `wg.Wait()`, the
  event that orders the read after every write, `IndexedAt` returns floor 2,000 and revision 2,000's commit time.
  Run under `-race`.
- New: `TestPropWatermarkHistory` (D8).

Task 3.2 adds `TestWatermark_RetentionTracksInFlight`. One revision stays in flight while 1,000,000 others are observed
and completed. After `runtime.GC()`, the live heap (`runtime.MemStats.HeapAlloc`) has grown by less than 1 MiB. The
test reads no field of the watermark. PR #407 probe comment (issuecomment-6388227813): the pin's code grows the heap by
about 0 bytes (1,864
under `-race`), and one 8-byte entry kept per completion grows it by 8.45 MB.

## D8. Generated check

`docs/testing.md:403-415` asks for this decision, because the watermark is an order-dependent history with a stated
law. Decided: a Rapid history (precedent `internal/cache/coalescing_set_prop_test.go:45`) whose oracle is the pin's
code itself. The pin's `watermark.go` is kept in `internal/revlag/pinref_test.go`, word for word except that the type
and constructor are renamed so it compiles beside the port. It records the behaviour the port must keep (D5 claim 3).

- Operations: observe the next revision (gaps of 1 to 3, so the sets are both dense and sparse) for one of three keys;
  observe a lower revision than the highest; observe again a revision already observed, under any key; complete a key
  at an observed or an unobserved revision; observe with a zero commit time. The same operation goes to both
  watermarks.
- Checked after every step: `IndexedAt` (floor and time) and `Observed` are equal between the port and the reference.
  The floor also equals the test's own record: the highest observed revision when nothing is in flight, else the
  lowest in-flight revision minus 1. On histories that only ascend, the floor never falls.
- Each history ends by completing every key at the highest revision. Both watermarks must then report nothing in
  flight (floor equals `Observed`), and their times must be equal.
- Assertion activation: a fixed prefix (`Observe(10,A)`, `Observe(11,B)`, `Complete(A,10)`) reaches the boundary
  deterministically, and the final drain runs the caught-up path in every case. The PR records the seed, the number of
  checks completed (`-count=1 -v`), and any failure as a named test.
- The model is sequential. Concurrency is covered by the two concurrent tests under `-race`.

## D9. Mutation rows (`task mutate:check`, recorded in `notes/5.1.md`)

Each row names the wrong behaviour. It is applied to the ported code at the line that implements it; the pin line it
came from is given for reference. M1-M5 and S1 were run against the pin's code (PR #407 probe comment
(issuecomment-6388227813)); M6
targets the new code.

| Row | Wrong behaviour (pin origin) | Test expected to fail |
| --- | --- | --- |
| M1 | floor is the lowest in-flight revision, not one less (`:212`) | `TestWatermark_SparseMidBuild_LowWaterOfPending` |
| M2 | completion drains only the exact revision (`:116`) | `TestWatermark_CoalescerCollapse_KeyScopedDrainsLowerRevisions` |
| M3 | `Observed` returns the floor (`:182`) | `TestWatermark_ObservedIsHighestDelivered` |
| M4 | a revision drained exactly at the floor is not covered (`:154`) | `TestWatermark_DrainAtFloorIsCovered`; again with `TestPropWatermarkHistory` |
| M5 | caught-up time from the last drained revision, not the highest observed (`:148`) | `TestWatermark_CompletionIsKeyScoped` |
| M6 | a drained revision is left in the structure that orders in-flight revisions (new code) | `TestWatermark_RetentionTracksInFlight` |
| S1 | the nothing-drained early return deleted (`:120-122`) | predicted survivor: equivalent |

If the ported code has no nothing-drained early return, S1 is dropped and the PR says so.

## D10. Admission-ledger row

Appended at the end of `docs/admission-ledger.yaml` under a `# Change setup-04a-04-graph-roots-revlag` comment:

```yaml
- source_path: pkg/revlag
  source_sha: 8b99efe9c66a4faa4fa509f9f62cc6bad8392128
  consumer_purpose: >-
    the low-water-of-pending caught-up watermark that graph-index (pin component.go:706, :1017, :1031, :1146,
    :1364; watermark.go:59) and graph-embedding (pin component.go:732, :1474, :1516; readiness.go:43, :126) feed
    their readiness envelopes from; both admitted (04A D2, changes 4 and 5) and ported later; internal because no
    starter consumer imports it and no exported signature names its type (#9 comment 5953295358)
  destination: internal/revlag
  contract: >-
    03B retained-contract-matrix row (design.md:703, "Readiness": honest readiness, ADR-066/084): the watermark is
    the revision-lag input to it. openspec/specs/revision-watermark/spec.md: the floor is the highest observed
    revision when none is in flight, else the lowest in-flight revision minus 1; Complete(key, rev) drains that
    key's in-flight revisions at or below rev and no other key's; Observed is the highest revision observed;
    Observe ignores revision 0; under ascending observation the floor never falls, and IndexedAt pairs it with the
    commit time of a completed observed revision at or below it (zero means unknown), exact when none is in flight
    (pin ADR-066 section 1, ADR-083 decision 3)
  dependencies_and_side_effects: sync, time; no goroutine, no I/O; per-revision state only while a revision is in flight
  known_risks: >-
    Indexed() dropped as unread (no production reader at the pin; 30 test call sites in graph-index and
    graph-embedding read the IndexedAt floor when they port; its doc mentions reworded); the in-flight scan in
    Complete and the floor read replaced (owner, Q1, PR #407 comment 6388065578; at the pin, draining 40,000 took
    14.5 s), every result equal to the pin's (TestPropWatermarkHistory against the pin's code); the caller keeps
    Observe ascending and the watermark does not check it: a lower revision observed after catch-up lowers the
    floor and pairs it with a newer commit time, as at the pin; the covered commit time may be older than the newest
    covered revision's, never newer; "gh#590" in the package comment is SemStreams' issue; no class:port-refactor
    issue exists for the package; no importer until graph-index
  proving_tests: >-
    internal/revlag/watermark_test.go (the pin's 13 tests, TestWatermark_ObservedIsHighestDelivered,
    TestWatermark_DrainAtFloorIsCovered, TestWatermark_CompleteWithNothingInFlight,
    TestWatermark_ConcurrentReadersAndWriters, TestWatermark_RetentionTracksInFlight, TestPropWatermarkHistory
    against internal/revlag/pinref_test.go, the pin's code)
  owner: setup-04a-04-graph-roots-revlag developer
  disposition: adapt
```

## D11. Repository map, ADR-066 note, status, coverage

- `docs/repository-map.md`: a row `internal/revlag/`, worded like `internal/logforwarderpolicy/`'s ("Ported from the
  pin's `pkg/revlag` by `setup-04a-04-graph-roots-revlag`, ahead of its importers: ..."). Lines 12-19 are corrected:
  change 3 is complete (all seven `service` PRs merged, the last as PR #393), change 4 has started with this package,
  and changes 5 to 7 have not started (apart from `vocabulary/export`).
- `docs/adr/066-honest-graph-index-readiness.md:15-18`: the port note moves `pkg/revlag` from "not yet ported" to
  "ported as `internal/revlag` (its admission-ledger row)". The decision text is unchanged.
- `scripts/port-status.sh`: no change. It keys on `source_path` and already lists `pkg/revlag` under change 4 (`:96`).
- Coverage: no `scripts/cover-check.sh` target, because 03B D10 rules the package out of the 80% gate. Coverage is
  recorded on the PR (predicted 100%).

## D12. Issues and merge order

- #108 stays: its work (measuring graph-index rebuild-on-restart) is graph-index's PR, per #9 comment 6386556850 item
  2. Its body's claim that the watermark "is right and stays" is unchanged by this PR.
- #9: `Addresses #9`. Nothing is retired; no `Closes`.
- #401 merged (`a8715ef`, 2026-10-24). It touched only the `service` ledger row, so this PR's appended row does not
  conflict; the branch merges `origin/main` before review. Independent of #403 (this PR does not touch
  `port-status.sh`) and #404 (other capabilities). The processor shell (#109) follows this PR.

## D13. Pin ADRs: kept, changed, dropped

- ADR-066 section 1: kept. Floor over delivered revisions, `minPending - 1`, key-scoped completion. Q1 changes how the
  in-flight set is held, not the decision; ADR-066 itself describes pending revisions per key (`:194`).
- ADR-083 decision 3: kept. `IndexedAt` pairs the floor with a commit time; zero means unknown.
- ADR-082's rejection of a bounded-ready predicate in this package: kept (nothing added).
- ADR-068, 073, 077, 081, 084, and in-tree ADR-108 and ADR-109: govern readers, unaffected. ADR-113: no lifecycle
  owner here.
- Changed or dropped: none. `Indexed()` is an accessor that no ADR decides on.
- ADR-066 and ADR-083 are already in the tree. No record is copied.

## Owner rulings

- **Q1, 2026-10-24 (Enjoy, relayed; PR #407 comment 6388065578): yes.** `Complete` and the floor read stop scanning
  every in-flight revision in this PR. Task 3.2 stays with its gate (the 2.1 tests unchanged; benchmark at 10,000 and
  40,000 in flight in `notes/3.2.md`). D9's rows are re-pointed at the new code's lines; D10's `known_risks` uses the
  "Q1 yes" wording. The "If no" branches in D5, D9 and D10 are closed.

## Owner questions

- **Q1 (answered yes; see Owner rulings). Make draining fast in this PR?** At the pin, draining N in-flight
  revisions takes time quadratic in N (40,000 took 14.5 s under the watermark's lock). graph-embedding's cold start
  puts every unembedded entity in flight, and graph-index's coalesced path can too. The history check compares the
  port with the pin's code after every step, so every result, the commit time included, is shown unchanged.
