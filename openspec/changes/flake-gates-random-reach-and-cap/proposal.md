# Proposal: flake-gates-random-reach-and-cap

Track: design (adds a Requirement to `harness-boundaries`); `design.md` records the choices.

## Why

Two of the twenty flake issues since 2026-10-01 (#466, #497) were property tests that failed after their random run
because a counter said a named case was never reached. Both were written after `docs/testing.md` forbade the shape
(2026-10-02) and both passed review: the sentence was in the long-form doc and in neither role contract. The five
porting PRs of October carried 1.7 to 2.9 times more test lines than production lines, up to 7,141 test lines in one
review stop, and the #159 cap counted only production lines. Owner ruling on #500 (comment 6580240783, items 4 and 5).

Approved-by: Coby Leuschke, 2026-11-02, on #500 (comment 6580240783).

## What Changes

- A contract test, `TestNoReachCheckAfterGeneratedRun`, sibling of `TestNoOwnDeadlineInTests` and built on the same
  parser: in a test function that calls `rapid.Check`, a failing call after that call returns is refused unless the
  line directly above carries `// After check: <reason>`. Witness cases that run before the check, and logging after
  it, pass untouched (the shape PR #498 landed). Nothing to sweep: no test on `main` has the shape today.
- The sentence "Do not require a random run to reach a rare state a minimum number of times; that makes the test
  flaky" joins both role contracts as a `BLOCKING` finding, with its `AGENTS.md` and rules-index rows.
- The #159 porting-PR cap (about 3,000 pin lines) counts every Go line, production and test, in `AGENTS.md` section 4,
  the rules index, and the architect contract that defines the count.

## Capabilities

- Modified `harness-boundaries` (one requirement added).

## Impact

- `internal/harness/contract/testtext_test.go`; `docs/testing.md`; both role contracts and the architect contract;
  `AGENTS.md` sections 3 and 4; `.agents/rules-index.md`. No production Go line changes.
- Closes #500. The one open porting PR (#504, `processor/graph-query`) is measured against the new count by its own
  reviewer; this change does not split it.
