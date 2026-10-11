# Checkpoint 3 H3 notes: a stored statement without source or time

The finding is Codex's checkpoint 3 record, PR #93 comment 6099921473, H3. The owner ruled the design on #91
(comment 6111530424): graph-ingest treats such a value as damaged, with one new exported reason in `graph`, and
graph's encoder unchanged. The design text is the architect's round 3, option (b). The task is 4.15. The files are
`graph/state_contract.go` (the reason), `processor/graph-ingest/entity_writes.go`, `mutation_runtime.go` and
`component.go` (code and doc comments), four test files, and the design and `graph-entity-writes` delta.

## Choices the design does not spell out

1. **One spelling of the rule.** The design adds "the same five lines" to `decodeStoredEntity`. Those lines would have
   been a second spelling of `requireStatementMetadata`'s loop. Instead the loop moved into
   `missingStatementMetadata` (`entity_writes.go:79`), which returns an unclassified error naming the index and the
   field. `requireStatementMetadata` (`:68`) classifies it as `invalid_request`, as before, with the same message
   text. `decodeStoredEntity` (`mutation_runtime.go:231`) wraps it as a `StateContractError` with
   `GraphStateReasonMissingStatementMetadata`. `refuseMissingMetadata`, the old helper, had no other caller and is
   gone. Wrapping `requireStatementMetadata`'s error instead was rejected: the stored-value error would carry an
   `invalid` class inside the `fatal` one.
2. **One helper for the two merges.** The design allows it. `encodeMerge` (`entity_writes.go:112`) runs the metadata
   check and then `MarshalEntityState`, and sends either refusal through `classifyStoredStateRMWError`. The stream
   merge (`:269`) and the append (`:436`) call it. The check is placed before the encoder, as the design asks; the
   refusal's reason comes from `decodeStoredEntity`'s order (decode, key, metadata), not from which gate tripped. So
   `guardTestPoisonBytes`, whose statement has neither field and a noncanonical predicate, is still recorded as
   `noncanonical_predicate`.
3. **Order inside `decodeStoredEntity`.** The metadata check runs after the key check. A value under the wrong key
   that also lacks metadata keeps the reason it had (`noncanonical_entity_id`).
4. **`storedPoison` takes the key.** The read and delete tests use different keys, and a value without metadata must
   name its own key, or it is refused as mis-keyed first. Its two new values are written as JSON by hand
   (`storedStatementWithout`, `poison_scoping_test.go:387`), independent of graph's encoder. The write-seam values
   (`seamStoredWithout`, `entity_writes_test.go:342`) are planted through `graph.MarshalEntityState`, which shows the
   encoder still accepts them.
5. **`bootSweep`.** The mis-keyed sweep test's local `sweep` closure moved to a file-level helper
   (`query_contract_guard_test.go:150`) so the new sweep test shares it. Its body is unchanged.
6. **Both fields in the holding append test.** `TestAppendAddingNothingOverStatementWithoutMetadataIsUnchanged`
   runs over a value without a source and one without a timestamp; the design names one test, not its rows.
7. **Ruling citation.** The design names the owner's acceptance in D23's "Declared cost:" sentence, so the ruling is
   cited there; D15's appended sentence keeps the design's text.

Before adding: the metadata rule's owner is `requireStatementMetadata`, now sharing its finder; the stored-value
rule's owner is `decodeStoredEntity`. The new exported name is the ruled one; its readers at birth are the poison
inventory's Health, debug status and log (`poison_inventory.go`). No knob. Nothing reusable across packages, so no
adoption list. Fuzzing: no new surface parses external bytes; the reason is a constant on an existing decode.
Generated checks: not used. The rule is two independent field checks over a list, with no law or history; named
examples reach each field on each lane, read site and the sweep.

## Evidence

Failing first, on `03aeb05` plus only the new constant (the tests name it), `go test -race -count=1`:

- `TestWriteSeamRecordsStoredPoisonAtTheRevisionRead`, 4 rows: the stream lane returned no error and the append
  answered `applied` at revision 9 (planted 7, the other writer's 8).
- `TestEveryEntityReadRefusesTheSameStoredPoison`, 8 rows: "the stored value must not be served"; the reconcile rows
  answered `applied`.
- `TestCanonicalDeleteRemovesAValueTheReadRefuses`, 2 rows: "the delete's read must record the refusal".
- `TestBootSweepInventoriesStatementWithoutMetadata`, 2 rows: "not inventoried by the boot sweep".

The holding tests passed on that tree and pass after the fix.

Fixtures. `BenchmarkMergeEntityCycle` failed at `merge_entity_bench_test.go:195` ("triple[0] has no source", the
lane's check of its delta, code this commit does not change); with a source on its delta and its resident
statements it passes (`-benchtime 1x`; every benchmark in `graph/...` and graph-ingest ran once, all pass).
`TestResidentUnregisteredStampIsNotPoison` failed with the fix and the old fixture (`:63` inventoried, `:68` the
append refused), and passes with a source on its resident statement, through `task test:integration -- -run`.
Neither failure was run on a pristine `03aeb05`.

`task mutate:check`, each 3 baseline runs, 3 mutant runs and an after-run, every verdict detection:

1. Stream merge without the check, `TestWriteSeamRecordsStoredPoisonAtTheRevisionRead`: the two stream rows commit.
2. Append without the check, same test: the two append rows answer `applied`.
3. `decodeStoredEntity` without the rule: `TestEveryEntityReadRefusesTheSameStoredPoison` (8 lines),
   `TestBootSweepInventoriesStatementWithoutMetadata` (2), and the seam's stream row, whose error falls back to the
   candidate's `invalid_request` ("triple[1] has no source").
4. The timestamp case dropped: `TestWriteRefusesStatementWithoutSourceOrTimestamp/mutation_append_without_timestamp`
   ("the write was accepted").
5. Reason `noncanonical_predicate`: `TestEveryEntityReadRefusesTheSameStoredPoison`, `poison_scoping_test.go:463`.
6. The append's check moved ahead of its nothing-to-add skip:
   `TestAppendAddingNothingOverStatementWithoutMetadataIsUnchanged` (2 rows).
7. The stored statements checked before the stream replace: `TestStreamArrivalReplacesStoredStatementWithoutTimestamp`
   (2 rows, `processIngest` refused).

No survivor and no inconclusive run. No new test is integration-tagged, so no by-hand mutation was needed.

## Not covered

- The broker's redelivery of a refused stream arrival until the consumer's limit, then its drop: the unit tests see
  one Nak; the limit is NATS behaviour (declared cost).
- The cost per read and per merge of the check (not measured; declared cost).
- A reconcile is covered at its read (the reconcile read rows); its write never sees such a value.

## Candidate follow-ups

None found.
