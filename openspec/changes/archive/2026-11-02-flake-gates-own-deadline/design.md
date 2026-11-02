# Design: flake-gates-own-deadline

Base: `f8e0ac5`. Track: `design` (adds a Requirement to `harness-boundaries`, modifies `merge-gate`). Draft PR #501.
Owner ruling: #500 comment 6577539980 (items 1 through 3, five-minute cap); measurements: #500 body and comment
6577478437. Records: #392 (the ordering rule this design amends), #202, #416, #499 (the three firings of the shape).

Terms: an *own deadline* is a wall-clock bound a test sets for work that must finish, failing itself when the bound
passes. A *fallback* is a shorter timeout inside the awaited call (a 30 s context, a library's connect, drain or
write timeout) that can end the call late and let a wrong implementation pass once the test's own bound is gone.

## Context

Twenty flake issues since 2026-10-01: every class with a `task verify` gate stopped; every review-only class recurred.
The own-deadline shape fired three times, every time a false alarm under local load, catching no hang. On `main` at
`f8e0ac5` the shape had 148 literal sites in 14 packages and more in spellings a text check cannot see.

## Goals / Non-Goals

Goals: no test on `main` fails on its own deadline for work that must finish; a `verify` gate refuses the shape at
birth; the one deadline per package is pinned and fits CI's job; the rule documents say what the code does.
Non-goals: `context.WithTimeout` deadlines and polling bounds (`require.Eventually`) as a gate (#502, #500 item 4
and 5 remain the owner's); any repeat or soak run (ruled out on #392); production code changes.

## Decisions

**D1. The sweep.** Each `select` arm `case <-time.After(...)` or `case <-x.C:` whose body fails the test is removed
and the test waits directly on the event the other arm received. Every assertion that followed the arm still runs. A
timer arm that passes (a negative check: nothing arrived in the window) stays. Deleted helpers whose only job was
the bound are inlined at their callers.

**D2. Fallbacks made unreachable.** Removing a bound is safe only when a wrong implementation hangs. Where the awaited
call has a fallback shorter than the package timeout, the test makes it unreachable (an hour budget: `WithTimeout`,
`WithWriteTimeout`, `drainTimeout`, `HTTPReadTimeout`; or `t.Context()` instead of a 30 s context) so the hang is the
only wrong outcome. Found by review round 1 (two HIGH, natsclient) and applied at six more sites.

**D3. The escape.** `// Own deadline: <reason>` on the line directly above the `case` is the only exemption, for three
reasons: a `synctest` bubble's fake clock (deterministic, not wall clock; 45 arms); a tool-package test whose timer arm
is the only code that kills a helper process (`tail -f /dev/null`) which would outlive a `go test -timeout` panic
(3 arms, `internal/harness/mutcheck`, `pindiff`); a deadline that is itself the behaviour under test (1 arm,
graph-ingest T-G3). No baseline, no allowlist.

**D4. The guard.** `TestNoOwnDeadlineInTests` in `internal/harness/contract/testtext_test.go`, sibling of
`TestNoSleepsInTests`: matches both arm spellings, counts a failing call on any receiver (`Fatal`, `Fatalf`, `Error`,
`Errorf`, `FailNow`, `Fail`, `require.`, `assert.`, `Require()`), with or without arguments except a bare `Error()`,
anywhere in the arm's body, including after multiline literals; fails when it scanned no test file; its sensitivity
test proves every scenario can fail. Spellings it cannot see (a `time.After` stored in a variable, a
`context.WithTimeout` deadline, a polling helper) are named in the spec and are review work.

**D5. The one deadline.** `-timeout 5m` on `test:unit` and `test:repeat`, pinned by toolcheck and the `merge-gate`
spec: four times the slowest unit package on CI (`internal/harness/contract`, 72 s), twelve times or more every
other, inside the 15-minute job (the whole verify took 6.5 minutes), so a hang prints every goroutine's stack. A
hang is deterministic and fixed once; an own deadline's cost is paid on every loaded run.

**D6. Rule text.** The #392 clause "a real-clock timer may still bound how long the wait runs" becomes the one-deadline
rule in `docs/testing.md`, both role contracts, the `AGENTS.md` row (gate `verify+review`) and the rules index;
the hazard in D2 and the three reasons in D3 are written where a developer reads them.

## Verification

`task verify` on each content commit (recorded in the PR body). Per package group: `-race -count=1 -cpu 1`,
`-count=5 -cpu 1 -shuffle=on`, `-tags integration` with Docker. Review round 1 mutants (a Connect that ignores its
context, a Close that waits on another drain) passed at `af0dd81` and hang at `dfadd73`; Codex's drain-timeout mutant
(`client.go:882`, `task mutate:check`) is re-run after its fix and its verdict quoted in the PR.
