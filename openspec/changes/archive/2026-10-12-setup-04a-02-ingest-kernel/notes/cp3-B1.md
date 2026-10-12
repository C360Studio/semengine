# Checkpoint 3 B1 notes: poison records cleared only up to the proven revision

The finding is Codex's checkpoint 3 record, PR #93 comment 6099921473, B1, and its portable probe
`TestReviewStaleAbsenceKeepsNewerPoison`. The task is 4.11. The files are `processor/graph-ingest/poison_inventory.go`,
`entity_writes.go`, and two tests in `poison_scoping_test.go`.

## Choices the finding does not spell out

1. **The guard has no exemption for revision 0.** `dropEntityPoisonRecord` used to clear unconditionally when its
   proof revision was 0, and the two unfenced clears passed 0. The `proofRev != 0 &&` exemption is removed, so every
   clear goes through the one comparison: a record newer than the proof stays. `gopls references` on
   `dropEntityPoisonRecord` lists five callers, all in `poison_inventory.go`; after the change each passes a nonzero
   revision. The absence check passes the revision recorded before its read; the valid-bytes check passes the
   revision it read; the delete passes the revision it removed (`DeleteAtRevision` refuses 0,
   `natsclient/kv.go:537-539`); the commit and the validating read pass the revision they committed or read.
2. **What the absence check reads, and when.** `verifyEntityPoisonRecord` reads the key's record under
   `entityPoisonMu` before its `Get`. An absent key then proves repair up to that revision. If there is no record,
   it returns without the `Get`: the record was already cleared, and any later record is new, so its detection
   (`inventoryEntityPoison`, or the boot sweep in `recordBootSweepPoison`) runs its own check. The valid-bytes branch
   is unchanged; the revision it read was already its bound.
3. **The delete's bound.** `deleteEntity`'s `revision`, the expected revision `DeleteAtRevision` fenced on: a
   successful delete removed that revision. `clearEntityPoisonOnDelete` takes it as `deletedRev`; `deleteEntity`
   (`entity_writes.go:419`) is its only caller.
4. **The tests go through production entry points.** In `TestStaleAbsenceKeepsNewerPoison` the first detection is a
   query (`handleQueryEntityNATS`); its check's `Get` is the mock's second `Get`, which signals and waits. The test
   then re-creates the key with poisoned bytes at revision 3, a second query detects them, and the waiting `Get` is
   released to return not-found. In `TestDeleteCleanupKeepsNewerPoison` the delete is `handleCanonicalDelete` at
   revision 4; the mock's `Delete` hook re-creates the key at revision 6 and runs a query that detects it, so both
   happen after the delete and before `deleteEntity`'s cleanup. The order comes from channels and the hook's place in
   the call, never from timing. Both assert what an operator sees: the record at the newer revision, the gauge at 1,
   and Health unhealthy and degraded. Both pass under `-race`.
5. **Generated checks: none.** The rule is one comparison per clear path (recorded revision against proof revision).
   Its two sides are named examples: newer is kept (the two new tests, and `TestPoisonRevisionGuardConcurrentRepair`
   "older commit proof does not erase newer poison"); equal is cleared (`TestPoisonRepairRecoveryWithoutRestart`,
   delete at the recorded revision 3; `TestVerifyAfterRecordDropsStaleRecordAfterCompletedRepair`, absent key with
   record 2). The interleaving each new test fixes is the one the finding names.

## Fail before

The command below, with the tests on `8c09644`'s code (run before the fix, and again with `go test -overlay` mapping
the two production files to their `8c09644` copies); exit 1.

```sh
go test -race -count=1 -cpu=1 ./processor/graph-ingest \
  -run 'TestStaleAbsenceKeepsNewerPoison|TestDeleteCleanupKeepsNewerPoison'
```

- `TestStaleAbsenceKeepsNewerPoison`, `poison_scoping_test.go:707`: "the newer poison record must survive the older
  clear"; record revision expected 3, actual 0; gauge expected 1, actual 0; "Health must stay degraded while poison
  is resident: {Healthy:true ... ErrorCount:0 ... Status:running}"; status expected "degraded", actual "running".
- `TestDeleteCleanupKeepsNewerPoison`, `:734`: the same messages, record revision expected 6, actual 0.

## Mutation records

`task mutate:check`, three mutant runs each, `-expect-text "the newer poison record must survive the older clear"`.
Mutant copies and logs are in `/tmp/pr93-b1-mutants` (on this machine only); the SHA-256 of each copy is given.

- M1, `verifyEntityPoisonRecord` reads the record after its `Get`, not before
  (`63778f6174b2394db392c4b820825a23ac8914298af663d1f2f98c418ac8cea3`), against `TestStaleAbsenceKeepsNewerPoison`:
  **detection**, three of three at `:707`.
- M4, the absence clear unbounded, `dropEntityPoisonRecord(entityID, ^uint64(0))`
  (`e78864e0fa9bb1942316cb0e4e75679afa94ab8f42c1262af27d9e9ae1ba7b66`), against `TestStaleAbsenceKeepsNewerPoison`:
  **detection**, three of three. Its first copy did not build (`recorded` unused); the verdict was "invalid", and
  the copy was fixed and rerun.
- M3, `entity_writes.go`'s delete clear unbounded, `clearEntityPoisonOnDelete(entityID, ^uint64(0))`
  (`d5c3eab0c96d2d6fdd5f26dad1cbac709e88d68a644d2fb97d3f80023e2762aa`), against `TestDeleteCleanupKeepsNewerPoison`:
  **detection**, three of three at `:734`.
- M5, the `proofRev != 0 &&` exemption restored (`d1aaceaf9a9b704b2e929a1ae0ddabbe39771c776ef22d38bf87ba1b0a816fc2`),
  against `TestStaleAbsenceKeepsNewerPoison`: **survivor**. No caller passes 0 any more (choice 1), so no test can
  reach the exemption; it is removed so that a future caller passing 0 is guarded, not cleared unconditionally.

## Not covered

- The early return in the absence check when no record is present was not run as a mutant: without it the `Get`
  runs and its clears are bounded the same way, so no test can tell the two apart.
- The guard's equal case (`>` against `>=`) was not run as a mutant here; the tests named in choice 5 cover it.
- No integration test drives either interleaving on a real NATS server; the mock bucket orders them.
- Candidate follow-up, not in this commit: none found.
