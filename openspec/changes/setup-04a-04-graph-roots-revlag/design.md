# Design: setup-04a-04-graph-roots-revlag

Pin `8b99efe9`. Ledger row: `pkg/revlag` (D10). Inventory: `inventory.md` (not yet reviewed). Pre-owner design review:
not yet run. Owner acceptance: not yet given.

## Options

- A. Carry byte-equal to `internal/revlag`. `task ledger:check` allows the move (inventory, "Placement facts"). Cost:
  lands `Indexed()`, which has no production reader at the pin (inventory sweep item 4); the surface audit does not
  keep dead surface (`.agents/protocol.md:56-58`).
- B. Adapt to `internal/revlag`: drop `Indexed()`, reword the doc lines that name it, add the tests the pin lacks, and
  a spec. Recommended.
- C. B, plus make `Complete` and the floor read cost independent of how many revisions are in flight (D5, Q1).
  Recommended if the owner answers Q1 yes.
- D. Keep `pkg/revlag`. Cost: breaks the standing placement rule (D1) with no consumer or signature to justify it.
- E. Do nothing until graph-index. Cost: departs from the owner's order (#9 comment 6386856657); graph-index's PR, at
  4,527 pin lines with a waiver, would carry this design as well.

## D1. Home: `internal/revlag`

The standing rule (#9 comment 5953295358) makes a package public only when a starter consumer imports it or an
exported signature in a public package names its type. Neither holds: no starter consumer's Go code names `revlag`,
and at the pin both readers hold `*revlag.Watermark` in unexported fields (inventory, "Who reads it"). So the package
moves to `internal/revlag`, as `pkg/dispatch` and `pkg/cache` did. The package name stays `revlag`. In-tree ADR
mentions of `pkg/revlag` (ADR-066, ADR-108) are pin paths; the ledger row maps them. This is not an owner question:
the rule decides it.

## D2. Disposition: `adapt`

Required, by the surface audit (`.agents/contracts/semengine-architect.md:67-78`; `.agents/protocol.md:56-58`; #190):
`Indexed()` goes. Its only callers are tests (17 in its own test file, 30 in the readers' tests), and `IndexedAt()`
returns the same floor (sweep items 1 and 4). Because the file changes, the row is `adapt`. Everything else is carried.

## D3. API as ported

| Identifier | Disposition | Reason |
| --- | --- | --- |
| `Watermark` | kept | readers' fields (pin graph-index `component.go:274`, graph-embedding `:337`) |
| `New` | kept | graph-index `:706`, graph-embedding `:732` |
| `(*Watermark).Observe(r uint64, key string, commitAt time.Time)` | kept | graph-index `:1031`, graph-embedding `:1516` |
| `(*Watermark).Complete(key string, rev uint64)` | kept | graph-index `:1146`, `:1364`; graph-embedding `readiness.go:43` |
| `(*Watermark).Observed() uint64` | kept | graph-index `:1017`, graph-embedding `:1474` (ADR-109 `:180`: the first-build target) |
| `(*Watermark).IndexedAt() (indexed uint64, commitAt time.Time)` | kept | graph-index `watermark.go:59`, graph-embedding `readiness.go:126` |
| `(*Watermark).Indexed() uint64` | dropped | no production reader; same value as `IndexedAt`'s first result |

Nothing is renamed. Signatures are unchanged.

## D4. Doc comments

- The eleven lines that name `Indexed()` (inventory, sweep) name "the floor" or `IndexedAt` instead. The text of
  `Indexed`'s own doc (pin `:161-163`) moves to `IndexedAt`'s doc. This follows from D2, so it is required.
- Pin `:32-35`, the nats.go and nats-server ordering claim, stays word for word: carried text under Bounded port scope,
  with no probe in this tree. The spec (D6) states ascending order as the caller's precondition instead, so it promises
  nothing about the library (ADR-112; `.agents/contracts/semengine-architect.md:425-431`).
- Optional, not done: `gh#590` at pin `:15` is SemStreams' issue. ADR-083's port note already says so.
- The package comment's references to ADR-066 and ADR-083 resolve in this tree. Nothing else names a SemStreams-only
  concept.

## D5. Cost of `Complete` and the floor read (Q1)

At the pin, every `Complete` and every floor read scans all in-flight revisions. Draining N in-flight revisions
therefore costs time quadratic in N (probe: 10,000 took 0.9 s, 40,000 took 15.4 s, all under the watermark's lock).
This bites at graph-embedding's cold start, where every entity waiting for an embedding is in flight, and on
graph-index's opt-in coalesced path. It is not an admission gate, and no reader in change 4's default path is
affected, so the work is optional under Bounded port scope. Q1 asks the owner to schedule it here.

If yes: `Complete` costs time proportional to the drained key's own in-flight revisions, and a floor read costs at
most logarithmic time in the in-flight count. The floor, `Observed` and each drain stay equal to the pin's for every
call order, ascending or not; the commit time keeps the spec's bound under ascending order. Proof: the D8 check
passes unchanged before and after the change, and a benchmark at 10,000 and 40,000 in flight is recorded in
`notes/3.2.md` as evidence, not as a gate. The structure is the developer's choice. If no: task 3.2 is
removed, the caller files the cost as an issue with these numbers, and graph-embedding's PR (change 5) retires it.

## D6. Spec: new capability `revision-watermark`

Precedent: `internal/dispatch` has its own capability, `keyed-dispatch`. The delta states seven requirements: the
floor, key-scoped completion, `Observed`, ascending-order monotonicity, the commit time, retained memory, and
concurrent use. Each is under 500 characters. At archive, the Purpose reads:

> `internal/revlag.Watermark` tracks how far a pipeline fed by a NATS KV watch has caught up: the floor at or below
> which every delivered revision has completed, the highest revision delivered, and the commit time of a covered
> revision. graph-index and graph-embedding feed their readiness envelopes from it (pin ADR-066 section 1, ADR-083
> decision 3); the envelope is `graph`'s.

Invariants and their spec homes: the floor (requirement "The floor is the low-water mark of in-flight revisions");
monotonicity ("The floor never falls under ascending observation"); the commit-time bound ("IndexedAt pairs the floor
with a covered commit time"). These three are the only source the D8 model may use.

## D7. Proving tests

`internal/revlag/watermark_test.go`:

- The pin's 13 tests, unchanged except that each `Indexed()` read becomes the floor from `IndexedAt()`. They already
  meet `AGENTS.md` section 3: fixed synthetic times, no sleep, skip, port or build tag. Their oracle is hand-computed
  literals over sparse revision sets. `TestWatermark_MemoryDoesNotGrowWithCompletions` reads private state on
  purpose, because retained memory is the behaviour and no exported call shows it. `TestWatermark_Concurrent` reads
  after `wg.Wait()`, the event that orders the read after the writes (#392).
- New: `TestWatermark_ObservedIsHighestDelivered` (pin `Observed` is 0% covered locally),
  `TestWatermark_DrainAtFloorIsCovered` (`Observe(10,A)`, `Observe(11,B)`, `Complete(A,10)` gives `(10, commit time
  of 10)`: the boundary the pin tests miss), `TestWatermark_CompleteWithNothingInFlight` (a `Complete` on a key with
  nothing in flight changes neither result).
- Also new: `TestPropWatermarkHistory` (D8).

## D8. Generated check

`docs/testing.md:403-415` asks for this decision, because the watermark is an order-dependent history with a stated
law. Decided: a Rapid history checked against a reference model, following `internal/cache/coalescing_set_prop_test.go:45`.
Examples alone are not enough: the pin's tests missed the boundary mutant (inventory probe).

- Operations: observe the next revision (gaps of 1 to 3, so the sets are both dense and sparse) for one of three keys;
  complete a key at an observed revision or at an unobserved one; read `IndexedAt` and `Observed`. A separate draw
  observes a lower revision after a higher one. Commit times rise with revision and are never zero (the zero case is
  the pin's named example).
- Model: a list of observed (revision, key, time, done) records the test owns. Floor = highest observed revision when
  none is pending, else the lowest pending revision minus 1. `Observed` = highest observed.
- Checked after every step, for all histories: floor and `Observed` equal the model's; a completion drains exactly the
  model's revisions. For ascending histories only, also: the floor never falls; the time is that of a done observed
  revision at or below the floor, non-zero once any observed revision is at or below the floor, and exact when none is
  pending.
- Assertion activation: a fixed prefix (`Observe(10,A)`, `Observe(11,B)`, `Complete(A,10)`) reaches the boundary
  deterministically, and the done-revision branch runs whenever the floor reaches an observed revision. The PR records
  the seed, the number of checks completed (`-count=1 -v`), and any failure as a named test.
- The model is sequential. Concurrency is covered by `TestWatermark_Concurrent` under `-race`.

## D9. Mutation rows (`task mutate:check`, recorded in `notes/5.1.md`)

| Row | Wrong change (pin line) | Test expected to fail |
| --- | --- | --- |
| M1 | `return minRev - 1` becomes `return minRev` (`:212`) | `TestWatermark_SparseMidBuild_LowWaterOfPending` |
| M2 | `in.key == key && r <= rev` becomes `... && r == rev` (`:116`) | `TestWatermark_CoalescerCollapse_KeyScopedDrainsLowerRevisions` |
| M3 | `Observed` returns the floor (`:182`) | `TestWatermark_ObservedIsHighestDelivered` |
| M4 | `r <= floor` becomes `r < floor` (`:154`) | `TestWatermark_DrainAtFloorIsCovered`; again with `TestPropWatermarkHistory` |
| M5 | caught-up time from the last drained revision, not `observedHighAt` (`:148`) | `TestWatermark_CompletionIsKeyScoped` |
| S1 | delete the nothing-drained return (`:120-122`) | predicted survivor: equivalent (inventory probe) |

All were run against the pin's tests or the new examples in a temp copy (inventory). M3 and M4 survive the pin's
tests. If Q1 is yes, the rows are re-pointed at the new code's lines with the same intent, and the PR records which
rows still apply.

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
    openspec/specs/revision-watermark/spec.md: the floor is the highest observed revision when none is in flight,
    else the lowest in-flight revision minus 1; Complete(key, rev) drains that key's in-flight revisions at or below
    rev and no other key's; Observed is the highest revision observed; Observe ignores revision 0; under ascending
    observation the floor never falls, and IndexedAt pairs it with the commit time of a completed observed revision
    at or below it (zero means unknown), exact when none is in flight (pin ADR-066 section 1, ADR-083 decision 3)
  dependencies_and_side_effects: sync, time; no goroutine, no I/O; per-revision state only while a revision is in flight
  known_risks: >-
    Indexed() dropped as unread (no production reader at the pin; its doc mentions reworded; the readers' ports read
    the IndexedAt floor); the caller keeps Observe ascending and the watermark does not check it: a lower revision
    observed after catch-up lowers the floor and pairs it with a newer commit time (inventory probe); the covered
    commit time may be older than the newest covered revision's, never newer; Complete and the floor read no longer
    scan every in-flight revision (Q1; at the pin, draining 40,000 took 15.4 s); "gh#590" in the package comment is
    SemStreams' issue; no importer until graph-index
  proving_tests: >-
    internal/revlag/watermark_test.go (the pin's 13 tests, TestWatermark_ObservedIsHighestDelivered,
    TestWatermark_DrainAtFloorIsCovered, TestWatermark_CompleteWithNothingInFlight, TestPropWatermarkHistory)
  owner: setup-04a-04-graph-roots-revlag developer
  disposition: adapt
```

If Q1 is no, the `known_risks` cost clause reads: "Complete and the floor read scan every in-flight revision (at the
pin, draining 40,000 took 15.4 s); tracked by #n for graph-embedding's port".

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
- Merge order: independent of #401 (different ledger lines), #403 (this PR does not touch `port-status.sh`) and #404
  (other capabilities). Whichever merges second keeps both ledger edits. The processor shell (#109) follows this PR.

## D13. Pin ADRs: kept, changed, dropped

- ADR-066 section 1: kept, without change. Floor over delivered revisions, `minPending - 1`, key-scoped completion.
  With Q1 yes, the structure changes but not the decision.
- ADR-083 decision 3: kept. `IndexedAt` pairs the floor with a commit time; zero means unknown.
- ADR-082's rejection of a bounded-ready predicate in this package: kept (nothing added).
- ADR-068, 073, 077, 081, 084, and in-tree ADR-108 and ADR-109: govern readers, unaffected. ADR-113: no lifecycle
  owner here.
- Changed or dropped: none. `Indexed()` is an accessor that no ADR decides on.
- ADR-066 and ADR-083 are already in the tree. No record is copied.

## Owner questions

- **Q1. Make draining fast now, in this PR?** Recommended: yes. At the pin, draining N in-flight revisions takes time
  quadratic in N (40,000 took 15.4 s under the watermark's lock). graph-embedding's cold start puts every unembedded
  entity in flight, and graph-index's coalesced path can too. This 213-line package, with the history check written
  in the same PR, is the cheapest place to change it and prove nothing else moved. Cost: about 30 more production
  lines and a second focus for review. Answer no: the pin's algorithm is ported as is (B only), the cost is filed as an
  issue, and graph-embedding's PR (change 5) retires it.
