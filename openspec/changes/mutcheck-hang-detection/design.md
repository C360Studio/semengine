# Design: mutcheck-hang-detection

Base: `e61ce55`. Track: `design` (adds a Requirement to `mutation-check`). Draft PR #507, `Closes #503`. Owner
ruling: issue #503, comment 6587993650 (2026-11-02). Record: PR #501, Codex's mutant on `natsclient/client.go:882`.

Terms: a *hang* is a mutant run that `go test`'s own `-timeout` alarm ends, printing `panic: test timed out after`.
The *control runs* are the baseline runs and the after-run, on the unchanged code. The *margin* is how far inside the
timeout every control run must have passed. A *detection by hang* is the verdict detection given for a hang under the
owner's two conditions, as opposed to a detection by an expected line (an assertion).

## Context

After #501 no test sets its own deadline for work that must finish (`docs/testing.md:520-527`); a wrong change that
blocks hangs until `go test -timeout`, and `mutate:check` reads that as inconclusive. Codex on PR #501: baselines
passed in about 2.4 s, mutant run 1 printed `panic: test timed out after 45s`, the after-run passed, verdict
`inconclusive: mutant run 1: go test's whole-run timeout ended the run`.

## Inventory

1. **Claimed gap.** The timeout is its own fact: `timedOut` (`classify.go:43`), set from any output line starting
   `timeoutPanic` (`:94`, `:143`); the bound is `bounded` (`:45`, `:65`; `runs.go:131`). `read` makes `timedOut`
   inconclusive for every run kind (`classify.go:182-183`), after `bounded`/`buildFailed`, before `signal`/`!selected`.
   `sequence` stops mutant runs at the first `timedOut || signal || bounded` (`check.go:74`). No duration reaches
   `decide`: wall time (`runs.go:124`) is `runRecord.wall` (`check.go:26`), printed only (`report.go:122`); `event`
   (`classify.go:86-91`) has no `Elapsed`; `checkResult` (`check.go:33-45`) has no timeout. `git grep -n -i
   "elapsed\|margin\|by hang" -- internal/harness/mutcheck openspec/specs/mutation-check`: test prose only.
2. **Spellings of "a timeout is inconclusive".** Spec "A run is bounded" (`spec.md:256-269`), "Inconclusive"
   (`:448-489`, clause and scenario "Whole-run timeout"); `classify.go:182-183`; `classify_test.go:130-132`, `:248-249`,
   `:340-342`; `docs/testing.md:276-286` (whose advice to give the test its own time limit #501 already contradicts).
   "Detection needs an expected line": spec "Detection" (`:412-425`, scenario "A failure that names no expected
   location") and "An expected line decides detection" (`:491-511`).
3. **Adjacent claims.** Open PRs: #507 (this) and #504 (graph-query; no file under `internal/harness/mutcheck`, the
   `mutation-check` spec or `docs/testing.md`). Last commits on those paths: `f7528d9`, `5542936`, `5355107`. No other
   active change names `mutation-check`; #502 does not touch verdict rules.
4. **Who it is for.** No exported surface (`package main`). Reader: an agent running `task mutate:check` on a swept
   test (Codex on PR #501).
5. **Problem shape.** Verdict from per-run facts plus a cross-run control; closest instance: `decideRuns` gating every
   verdict on the after-run (`check.go:138-145`). Adopted.
6. **Pin ADRs.** None: `mutcheck` is SemEngine's own harness, no ledger row, no pin package carried.

**Adopter seam.** Not reached from outside the repository. In-repo users must know that a detection by hang is weaker
evidence, and that `-timeout` must be at least three times the test's run time; the second is told at run time by the
slow-control reason (D1). The tool observes the controls instead of asking the caller to predict them.

**`go test -json` timing (recorded, not promised).** `go doc cmd/test2json`: Elapsed "gives the time elapsed for the
specific test or the overall package test that passed or failed". `testdata/events/pass.jsonl`: package `start`
41.089 s, package `pass` 42.336 s, `Elapsed` 1.247, about one second of it after the test passed (the race runtime's
exit pause). `timeout.jsonl` (`-timeout 1s`): `Elapsed` 1.168. How `Elapsed` relates to the alarm's start is not
promised.

## Options

- **A. Do nothing.** Swept tests stay unprovable; contradicts the ruling.
- **B. Let `-expect-text 'panic: test timed out'` count.** No control-run condition (owner condition 2); rejected.
- **C. Owner's rule, margin on wall time.** No parsing, but wall includes the `-race` build; a cold cache reads slow.
- **D. Owner's rule, margin on the package `Elapsed`.** Closest to what `-timeout` bounds; one field. Recommended.

## Decisions

**D1. The margin.** Checked only when mutant run 1 reads `hang` (D2); no other verdict depends on any elapsed time,
and every other verdict is unchanged. Then every baseline run and the after-run must have a package `Elapsed` of at
most a third of `-timeout` (owner's proposal): the hung run took at least three times the slowest control, on the same
machine, moments apart. `Elapsed` is read only from the event with an empty `Test` and action `pass` or `fail`, kept
so that "not reported" differs from zero; `checkResult` carries the timeout. A control over the margin, or with no
`Elapsed`, gives inconclusive; the reason names the run, its time and the margin, so the reader knows to raise
`-timeout`. Consequence: the recorded `-race` run's `Elapsed` is about 1.25 s for a test that takes none, so a
`-timeout` under about 3.75 s can only read inconclusive on a hang (default `2m`, `inputs.go:54`).

**D2. Which runs read `hang`.** A mutant run whose stream has the timeout panic, a `run` event for the named test, no
result for the named test (`result == ""`), no signal, and not stopped at the bound. `timedOut` matches the panic text
on any line, so a test that passed or failed and also printed that text (a logged child `go test`) keeps today's
inconclusive. Unchanged inconclusive: the bound (checked first, even with the panic printed), a signal, a timeout
before the named test ran, a timeout on a control run. An expected line before the panic does not matter; the report
prints it.

**D3. `-runs` greater than 1.** The first timeout still ends the mutant runs (`check.go:74`). Detection by hang needs
the hang to be mutant run 1; a hang after a passing run, or after a run detected by assertion, is "the mutant runs
disagree". The reason says how many of `-runs` mutant runs were made.

**D4. Expected lines and the report.** "Detection" keeps requiring an expected line and gains "no mutant run ended by
the timeout", since "Inconclusive" drops its parenthetical; its no-expected-line scenario is narrowed to runs with no
timeout panic. "Detection by hang" gives the same verdict word (exit zero, four verdicts). "A failing exit status
alone SHALL never give detection" holds: the reading rests on the panic line and the controls. `hang` is a
reading-only outcome like `pass`: a run line reads `hang`; `decide` maps it to detection or inconclusive and never
returns it. The last line reads `verdict: detection: by hang: ...` with the timeout, margin, the slowest baseline's
and the after-run's `Elapsed`, and that a stall hitting only the mutant run would read the same. The rapid-seed
sentence qualifies a survivor's absence claim and does not apply.

**D5. Spec delta.** MODIFIED "A run is bounded", "Detection", "Inconclusive" ("Whole-run timeout" now on the
after-run), "The report describes each run" (per-run `Elapsed`); ADDED "Detection by hang" (seven scenarios) and
"What a detection by hang reports" (the margin's scope). Each requirement under 500 characters.

**D6. Tests.** Observe-level replays prove `Elapsed` parsing from real recordings (`pass.jsonl` 1.247,
`timeout.jsonl` 1.168) and the `hang` reading. In `TestCheckVerdicts` (`classify_test.go:310`)
each planned run may carry its own `Elapsed` (as the planner already sets `end`), and the check's timeout is a row
input; `sequence(..., 3, ...)` (`:395`) stays. Rows: detected by hang (`recTimeout` as mutant 1; the existing timeout
row flips; words `by hang`); a slow baseline 2 and a slow after-run (words name the run, its time, the margin); the
bound (`recTimeout` with `end.bounded = true`); a mutant run before the hang (`-runs 3`: mutant 1 passes or is detected,
mutant 2 hangs; only mutants 1 and 2 made; `disagree`); "Whole-run timeout" (`recTimeout` as the after-run); timeout
before the test ran; and a test that passed and printed the panic text (new recording `logged-timeout.jsonl`, by the
recipe at `classify_test.go:17`: a test that `t.Log`s `panic: test timed out after 1s` and passes;
`panic-after-pass.jsonl` prints no panic line, so it cannot serve). A new `hang.jsonl` gives the Codex shape. A
`report_test.go` row proves "Elapsed time in the report". `TestReadEveryCombination` (`:248`) learns `hang`. Shown
able to fail: `task mutate:check` on `check.go` with the margin comparison inverted, and on `classify.go` with the
`result == ""` condition dropped, each expected at its row. No e2e: a real hang needs a multi-second `-timeout` and
is load-sensitive.

## Merge order and adoption

No other open PR touches these paths (#504 does not); this PR merges on its own. Adoption: none; the harness's own
verdict, not a reusable primitive.

## Risks

A stall that hits only the mutant run reads as a detection (owner-accepted); the reason says so. `Elapsed` is used
only as documented and probed only by the recordings above.
