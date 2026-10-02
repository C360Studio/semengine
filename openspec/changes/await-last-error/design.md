# Await last-observation error clarification

## Scoped inventory

Question: what does Await retain from the final observation, what proof is missing, and which open
changes overlap? Repository: SemEngine only. Base: `4b5a264589c1479012fdc36c12299f71870af7c1`.
No sister repository or new external adopter surface is involved.

- Owner and current spellings: `internal/harness/probe/await.go:29` replaces value and error together;
  lines 37–42 report that error or "(no error)". The ambiguity is in the lifecycle-suite requirement
  "Probes retain observable state", lines 33–42. The existing doc comment describes the final error.
- Missing evidence: `TestAwaitReportsLastObservation` at `probe_test.go:147` checks distinct non-nil
  errors; `TestAwaitReportsLastObservationWithoutError` at `:194` checks always-clean observations.
  Neither covers an earlier error followed by a clean observation. The archived flake-defense change
  records why the old alternating-error test depended on the deadline (flake #49).
- Adjacent claims: an open-PR file listing on 2026-10-02 found #48 (26 files), #54 (6), #58 (2), and
  our claim #59 (0 before content). #48 modifies lifecycle-suite's "Portable floor" and adds "Observe
  adapter contract", without editing probe or current specs. #54 changes carry-check artifacts and
  harness-boundaries; #58 changes testing guidance and the protocol. No PR exceeded the listing limit.
- Present consumers: `gopls workspace_symbol -matcher=fuzzy Await` found one implementation;
  `gopls references internal/harness/probe/await.go:20:6` confines callers to the harness. The existing
  admission-ledger probe row covers the pattern. No new exported symbol or reusable primitive is needed.
- Existing problem shape: the always-error test rejects an obsolete first error; the always-clean
  test requires "(no error)". The new history joins those checks without introducing another owner.

Adopter seam: harness callers already supply their context and observer. They need no additional
setup or prediction; the returned diagnostic already clears an earlier error after a clean observation.
No boundary, capability admission, migration, or adoption sweep is introduced.

## Decision and observable proof

The [owner's ruling](https://github.com/C360Studio/semengine/issues/51#issuecomment-5951370165)
fixes the outcome. Leaving the text alone retains the ambiguity; retaining historical errors would
contradict the ruling. Clarify the existing requirement and add `TestAwaitClearsEarlierObservationError`.

The first observation returns an earlier value and sentinel error. The second returns a distinct
clean value and cancels the test-owned context while the condition remains false. Assert exactly two
observations, the second returned value, a diagnostic containing that value and "(no error)", no
earlier error text or wrapping, and preserved `context.Canceled`. Expected values come from this ruled
history and test-owned constants, not Await's output. Use `t.Context()` and the existing synctest
approach; cancellation from the observer determines the final observation without a polling deadline.

## Why examples suffice

This is an order-dependent history. Examples suffice: the existing always-error test covers replacing
one error with another and rejecting the old error; the always-clean test covers the clean branch;
the new error-then-clean test covers clearing the old error. Longer repetitions add no distinct
error-retention state relevant to this clarification. No new input dimensions or concurrent observer
are introduced. All new assertions run after the fixed two-observation history.

## Mutation and verification

The new test passes initially because Await already conforms. Temporarily update the retained error
only when an observation returns a non-nil error, while always updating the value. The unchanged test
must fail because the earlier error remains wrapped and "(no error)" is absent. A build error, timeout,
or unrelated failure is inconclusive. Back up `await.go` with `cp`, restore that copy, verify matching
checksums, and rerun. Record all three verbose runs and the exact mutation in the PR.

Run focused tests first, then the repository preflight before publication. `task verify` includes the
race-tested unit suite and five shuffled unit runs at one CPU; do not duplicate passing heavy gates.

## Ordering and stop point

Either this PR or #48 may land first. Whichever archives second preserves both requirements and
revalidates with `task spec:check`. #54 and #58 may land in either order; pickup reads current guidance.

The [explicit leg-1 instructions](https://github.com/C360Studio/semengine/issues/51#issuecomment-5951968282)
place independent review, archive/spec sync, undraft and merge in leg 2. Leave this change active and
record a verified stop point and write-ownership release in the draft PR for a fresh Claude session.
