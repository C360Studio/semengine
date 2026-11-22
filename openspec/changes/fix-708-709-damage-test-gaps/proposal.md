# Proposal: fix-708-709-damage-test-gaps

Approved-by: cglusky (2026-11-21) <https://github.com/C360Studio/semengine/issues/708#issuecomment-7072568152> sha256:b22723bc9c79

Track: `quick`. No capability changes: test-only, and no spec requirement is added or changed.

## Why

Two tests on the damaged-entity record cannot catch the bugs they exist for.

- #708: `TestHealthCountMatchesNamedIDsWhileChanging` (`processor/graph-ingest/poison_report_test.go`) needs a writer
  and a reader running at the same moment. `task test:unit` and `task test:repeat` run with `-cpu 1`, so in every
  gate `task verify` runs, the two never overlap and the broken code passes.
- #709: nothing tests overlapping reports and clears of the same key on `internal/damage`'s `Record`, the case where
  a query reports a key damaged while a repair clears it. The existing concurrency test uses distinct keys, and the
  generated check runs one call at a time.

## What Changes

- #708: the Health test raises `GOMAXPROCS` to at least 2 for its own run and restores it afterwards, so its writer
  and reader run in parallel even under `-cpu 1`. It stays out of `t.Parallel`. Shown to fail at `-cpu 1` with the
  two-read Health code put back.
- #709: a new test in `internal/damage` runs reports and clears of one key from several goroutines under `-race`,
  raising `GOMAXPROCS` the same way. Every clear uses one bound `B`, and reports use revisions both below and at or
  above `B`. After all of them return, the test checks a rule that holds whatever the order: if any report was at or
  above `B`, the key is recorded at the highest such revision. Shown to fail against a `Report` that does not keep
  the highest revision, and against a `Clear` that drops entries at its bound.

## Non-goals

The gauge-order test (#707) needs a different approach and stays open. No production code changes. The `-cpu 1`
setting of the gates stays.

## Impact

`processor/graph-ingest/poison_report_test.go`, `internal/damage/record_test.go`. One PR, `Closes #708`, `Closes #709`.
