# Checkpoint 3 H1 notes: a birth stores each statement once on every lane

The finding is Codex's checkpoint 3 record, PR #93 comment 6099921473, H1, and its portable probe
`TestReviewBirthDeduplicatesExactStatements`. The task is 4.12. The files are `processor/graph-ingest/entity_writes.go`
(the shared step and the stream birth), `canonical_mutations.go` (the mutation create), `component.go` (the in-process
create and one doc comment), and three test files.

## Choices the finding does not spell out

1. **Which existing deduplication.** graph-ingest had two whole-write ones with the same meaning:
   `graph.ReplaceBySource`, which keeps a statement repeated equal in every field once in each set it applies
   (`keyOfStatement`, `graph/helpers.go:123`), and the conditional replace's `dedupeReconcileTriples`
   (`canonical_mutations.go:666`), a pairwise scan over `sameReconcileTriple` (`:654`). Both compare
   `message.AppendIdentityKey`, `Confidence`, and `Timestamp` and `ExpiresAt` as instants, and keep the first in
   order. The shared step, `distinctStatements` (`entity_writes.go:101`), is
   `graph.ReplaceBySource(nil, triples).Triples`: over no stored statement every set applies, and the function's doc
   comment states that an applied set keeps such a repeat once, the first in arrival order. It is the stream lane's
   rule for an existing entity, so a birth and a later replace of the same statements store the same ones, and the
   design puts the write rules in `graph` (D15, "The write modes"). The pairwise scan was not chosen because of its
   cost on the stream lane, where every new entity is a birth (local benchmark below).
2. **Where on each lane.** Last before the encode, on everything the birth stores, hierarchy and profile statements
   included: `canonical_mutations.go:278`, `component.go:2264`, `entity_writes.go:178`. Each lane checks its caller's
   statements first, so a refusal names the caller's index. The mutation create checks subjects (`:261`), metadata
   (`:265`) and the contract (`:275`). The in-process create checks the contract (`component.go:2238`) and metadata
   (`:2241`). On the stream lane, `ingestEntity` runs the contract check (`component.go:1784`) before it calls
   `mergeEntityOnLane` (`:1734`, the only production caller), whose metadata check is at `:2121`. The stream birth's
   `MarshalEntityState` inside the compare-and-set now indexes the candidate without its repeats, which is what the
   merge branch's gate already indexed (the `ReplaceBySource` result). It is a second line behind `ingestEntity`'s
   check.
3. **The class of births.** `gopls references` on `createEntity` lists two callers, `canonical_mutations.go:283` and
   `component.go:2275`. The third birth is `replaceEntity`'s branch for an absent key. The bucket's other writes in
   `entity_writes.go` cannot birth an entity: the append refuses an absent key, and the conditional replace writes at
   an existing revision. `TestEntityWritesHaveOneSeam` keeps every other non-test file from writing the bucket.
4. **What the caller gets back.** The mutation create's reply entity and the in-process create's receipt carry the
   statements without the repeat, which is what was stored. The in-process create already wrote into the caller's
   entity (the hierarchy statements and the profile filter); assigning the new slice to `entity.Triples` is the same.
5. **A test that planted duplicates through a create.**
   `TestAddTriple_PreExistingStoredDuplicatesAreKeptAndStillSuppress` (`add_lane_dedup_test.go`) used `CreateEntity`
   to store two equal statements, which a create no longer does. It now writes the encoded value straight into the
   bucket. What it checks is unchanged.
6. **Generated checks.** Named examples are enough here. The rule is `graph.ReplaceBySource`, which has a property
   test against a model (`TestPropReplaceBySourceMatchesModel`). Its stored draw has length 0 to 4
   (`graph/write_rules_prop_test.go:26`), so an empty stored entity is in range. No recorded run shows that case
   being reached. What this change adds is three call sites, and each has a named subtest and a mutation check.

## Evidence

- `Fail before:` on `19d021f`,
  `go test -race -count=1 -run TestBirthCountsEqualStatementsOnce ./processor/graph-ingest/`: all three subtests
  fail, at `write_modes_test.go:256`, `:265` and `:270`. Each stored five statements (`v/source-a/1/1/-` twice) where
  four are wanted. The stream re-send control (`:272`) already passed, as Codex
  observed. The three new rows of `TestWriteRefusesStatementWithoutSourceOrTimestamp` pass on `19d021f`. They guard
  the order of validation and the step, and were not written to fail first.
- Mutation checks, `task mutate:check`, three mutant runs and one after-run each. All six verdicts are `detection`.
  1. `canonical_mutations.go:278` removed: `-test TestBirthCountsEqualStatementsOnce/mutation`,
     `-expect write_modes_test.go:256`.
  2. `component.go:2264` removed: `-test TestBirthCountsEqualStatementsOnce/in-process`,
     `-expect write_modes_test.go:265`.
  3. `entity_writes.go:178` removed: `-test TestBirthCountsEqualStatementsOnce/stream`,
     `-expect write_modes_test.go:270`.
  4. `request.Triples = distinctStatements(request.Triples)` added before the mutation create builds its entity:
     `-test TestWriteRefusesStatementWithoutSourceOrTimestamp`, `-expect-text 'does not name "triple[2]"'`.
     The expected line was `statement_metadata_writes_test.go:153: refusal "triple[1] has no source" does not name
     "triple[2]"`.
  5. The step added before the in-process create's contract check: same test, same text, same line.
  6. The step added before `mergeEntityOnLane`'s metadata check: same test, same text, same line.
- Local only, not in the tree: a benchmark loaded with `go test -overlay` from `/tmp`, over n distinct statements of
  one subject. `dedupeReconcileTriples` took 17 µs, 1.4 ms and 150 ms at n = 10, 100 and 1000.
  `graph.ReplaceBySource(nil, ·)` took 6 µs, 45 µs and 0.57 ms. Each is twenty iterations, from one run of
  both on one machine.

## Not covered

- A hierarchy or profile statement equal in every field to one the caller gave is not tested. The stream lane refuses
  the hierarchy's reserved source, and `reconcileIndexingProfile` keeps one profile statement.
- An import-lane birth (`importLane` true) runs the same `mergeEntityOnLane` and `replaceEntity` code and has no test
  of its own here.

## Candidate follow-up (not in this commit)

- The conditional replace keeps its own pairwise deduplication, `dedupeReconcileTriples` (`canonical_mutations.go:666`),
  and its own equality, `sameReconcileTriple` (`:654`). That equality is a second spelling of `graph`'s
  `keyOfStatement`, and the scan is quadratic (see the benchmark). `reconcileCandidate` (`entity_writes.go`) could call
  `distinctStatements` instead. `selectedPredicatesEqual` would still need the equality for its counts. This is
  outside H1 and is not changed here.
