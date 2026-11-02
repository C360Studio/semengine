# Proposal: flake-gates-own-deadline

Track: quick.

## Why

Two of the three latest flakes (#202, #416, #499) are tests that set their own wall-clock deadline for work that
must finish and fail themselves when load makes the work slower. There is no rule against it, and the ordering rule
from #392 permits it ("a real-clock timer may still bound how long the wait runs"). On `main` at `f8e0ac5` there
are 148 such sites in 14 packages. Every flake class that got a `task verify` gate has stopped; every class left to
review has recurred (#500, table). The owner: "this has to stop or we do not complete this project."

Approved-by: Coby Leuschke, 2026-11-02, on #500 (comment 6577539980): items 1 through 3 with the 5-minute cap.

## What Changes

- Every `select` arm `case <-time.After(...)` whose body fails the test is removed; the test waits directly on the
  event that orders its read (`sync.WaitGroup.Wait`, a receive, `<-done`). A timer arm that passes (a negative check:
  nothing arrived) is untouched. A deadline a test must own says why with `// Own deadline: <reason>` on the line
  above; none is expected.
- A contract test, `TestNoOwnDeadlineInTests`, sibling of `TestNoSleepsInTests`, refuses the shape, with a paired
  sensitivity test.
- `test:unit` and `test:repeat` pin `-timeout 5m`: the one deadline per package, with four times headroom for the
  slowest package (`internal/harness/contract`, 72 s) and twelve times or more for every other one (#500 comment
  6577478437). A real hang fits inside CI's 15-minute job and prints every goroutine's stack.
- `docs/testing.md`, both role contracts, `AGENTS.md` section 3 and `.agents/rules-index.md`: the sentence that
  permits a test's own timer becomes "a test sets no deadline of its own for work that must finish; `go test
  -timeout` is the one deadline", and the new guard's row is added.

## Capabilities

- Modified `harness-boundaries` (one requirement added).
- Modified `merge-gate` ("Varied and repeated unit runs" gains the timeout).

## Impact

- Test files only in 14 packages (148 sites), `internal/harness/contract/testtext_test.go`, `Taskfile.yml`, the
  toolcheck pin test, two specs, four rule documents. No production Go line changes; no pin line changes, so #159's
  cap does not bind, and the sweep lands with the guard because the guard cannot pass on `main` before it.
- Closes #499. Retires items 1 through 3 of #500; items 4 and 5 stay open there.
