# Design: flake-gates-random-reach-and-cap

Base: `f7528d9`. Track: `design` (adds a Requirement to `harness-boundaries`). Draft PR: see tasks. Owner ruling: #500
comment 6580240783. Records: #466, #497 (the two firings), #498 (the witness pattern), #501 (the sibling guard).

Terms: a *generated run* is one `rapid.Check` call: rapid draws random inputs and runs the property closure on each.
A *reach check* is a failing call after the run that depends on what the random inputs happened to cover. A
*witness* is a named input the test runs through the same assertions before the generated run, so each case is
reached on every run.

## Context

Both flakes failed on a reach check. The fix in #498 moved each case into a witness that runs before the check and
turned the post-run counts into log lines. On `main` at `f7528d9`, 20 test files call `rapid.Check`; the guard, once
written, found one reach check (`service/component_manager_prop_test.go`, a `t.Errorf` on a zero count inside a loop
after the run) that a line-level census had missed. It is converted to witnesses in this change (D5).

## Goals / Non-Goals

Goals: a `verify` gate refuses a reach check at birth; the rule is in the contracts a reviewer reads; the porting cap
counts what a reviewer reads. Non-goals: judging whether a property's invariant is right (review work); `MakeCheck`
and other rapid entry points (none in the tree; named as outside the check); splitting PR #504.

## Decisions

**D1. The guard.** `TestNoReachCheckAfterGeneratedRun` parses each `*_test.go` file. In every function body (test or
helper) that contains a call whose selector is `rapid.Check`, each statement after that call statement, including
nested blocks and loops but not the closure passed to the check, is a violation when it contains a failing call: the
set `TestNoOwnDeadlineInTests` uses (`Fatal`, `Fatalf`, `Error`, `Errorf`, `Fail`, `FailNow` on any receiver, a bare
`Error()` excepted; `require.`, `assert.`, `Require()`). A failing call inside the closure is the property's own
assertion and is not a violation. No baseline, no allowlist; a scan of no test file fails; a file that does not parse
is a violation.

**D2. The escape.** `// After check: <reason>` on the line directly above the statement, same shape as `// Own
deadline:`. The reason the spec names: an assertion on state the run must leave behind (a goroutine count, a closed
resource), which does not depend on which inputs were drawn. A reach count is never a reason.

**D3. Contract text.** The `docs/testing.md` sentence enters the developer contract's generated-checks bullet and the
reviewer contract's property-test bullet as a `BLOCKING` finding, with a pointer to the witness pattern. `AGENTS.md`
section 3 gains the row at gate `verify+review`; `.agents/rules-index.md` the same row.

**D4. The cap.** `AGENTS.md` section 4 (#159 row), `.agents/rules-index.md` and `semengine-architect.md` ("about 3,000
pin lines ... non-test Go lines") say: every Go line at the pin, production and test, with the design's size table
listing both. The number stays 3,000; what it counts changes.

**D5. The one live site.** `TestPropComponentManagerStopHistory` in `service/component_manager_prop_test.go` gets a
named witness history per branch, run through the same assertions before the generated run; its post-run counts become
log lines, as #498 did.

## Verification

Sensitivity test per scenario; `task verify` on each content commit; `task ateam:doctor` for row parity;
independent review, then the cross-agent record.
