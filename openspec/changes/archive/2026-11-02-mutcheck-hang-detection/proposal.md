# Proposal: mutcheck-hang-detection

Track: design (changes a Requirement of `mutation-check`); `design.md` records the choices.

## Why

After the #500 sweep (PR #501), a wrong implementation under a swept test no longer fails on the test's own timer: it
hangs until `go test -timeout`. That hang is the rejection the test is built to produce (`docs/testing.md`,
"Concurrency and cleanup"). The `mutation-check` spec ("A run is bounded") reads every mutant run that ends by the
timeout as inconclusive, so for every swept test `task mutate:check` can never report a detection. Seen on PR #501:
Codex's mutant on `natsclient/client.go:882` was a survivor before the fix and a timeout panic after it, and the tool's
verdict stayed inconclusive. Owner ruling on #503 (comment 6587993650).

Approved-by: Coby Leuschke, 2026-11-02, on #503 (comment 6587993650).

## What Changes

- A mutant run that ends by `go test`'s own timeout (`panic: test timed out after`) reads as a detection when every
  baseline run and the after-run passed well inside the timeout; the report says the detection came by hang. A run the
  program stops at twice the timeout stays inconclusive. The margin is fixed in `design.md`.
- `openspec/specs/mutation-check/spec.md` gains the clause; `internal/harness/mutcheck` (`classify.go`, `check.go`,
  `report.go`) applies it, with a replayed event log for the new reading.

## Capabilities

- Modified `mutation-check` (one requirement changed, one added).

## Impact

- `internal/harness/mutcheck`, its tests and `testdata/events`; `openspec/specs/mutation-check/spec.md`;
  `docs/testing.md` where it describes `mutate:check` verdicts. No production Go line changes.
- Closes #503.
