# Inventory: mutation-check

- base: `8e594f3cdc1911ea9b23d9095ce8c767f5a014ec` (branch `claude/mutation-check`, claim PR #82). It is one empty
  claim commit over `main` at `2ec3bcf` (`main`'s head when re-checked for this revision), so every pin holds on both.
- revision 2, measured 2026-10-04. Revision 1 (sha256 `0a23bcfb52a5d0a18cc7261c4672915a97301eb885347cbccb5f075bbbb7f0e1`)
  had an independent review, verdict BLOCKING (`inventory-review-r1.md`, sha256
  `ac6dada11f1af232827085eed55634c42dd7fa52d6de5fdc7a7f055a769d1fa3`). This revision answers its findings: B1 in
  2c, 3.2, 5 and Q9-Q10; the three HIGH findings in 3.4, 2c, 6 and Q14; the MEDIUM findings in 2b, 2f, 5, 7 and
  Q2-Q7; the NITs in 2a, 2b and 2d. Section 14 maps each finding to where it is answered.
- issue #79: open, label `enhancement`, no milestone. Since revision 1 it has one comment, the owner's sequencing
  (5981621215, section 3.4). Tools: `gh` 2.97.0, `go1.26.6 darwin/arm64` (`go.mod`: `go 1.26.6`), `task` 3.51.1
  (`.task-version`: 3.51.1), `/bin/bash` 3.2.57, `gopls`.
- repositories read: this one, and SemStreams at the pin `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, read only with
  `gh api ... ?ref=<sha>`:
  - for Q14, code facts (`docs/inventory-scope.md` rule 4 and its row for `semstreams`): the recursive tree
    (`truncated: false`, 5626 entries), `Taskfile.yml`, the six files under `.github/workflows/`, and five evidence
    scripts (section 2g);
  - `docs/contributing/01-testing.md` (728 lines, sha256 `a5ac0c78d96b29033b845ae96d22ab222d7c952bd76e1bcc81b093e193fd7b3a`),
    under the owner's ruling on #22, comment 5981684453 ("yes, read it"). Its scope is that one file, for this
    inventory and design only.
- pins are `` `path:line` — text `` inside `text` blocks, each line verbatim except that a tab is shown as four
  spaces. A pin from SemStreams is labelled `semstreams@8b99efe9:<path>:<line>`.
- inventory phase only: no options, recommendation, target state, artifact delta or task appears below.

## Question

"What in SemEngine already performs, specifies, or constrains any part of the mutation procedure, its
classification, its restore, or the command and documentation surfaces #79 would change?"

## Problem statement (as #79 states it, unverified until sections 9 and 10)

Issue #79 asks for `task mutate:check -- <package> <TestName> <mutant>`, backed by `scripts/mutation-check.sh`, that
runs the procedure in `docs/testing.md` ("Show that the test can fail"): back up the one file the mutant changes, run
the named test on unchanged code, apply the mutant, run it again with the same seed flags, classify the outcome as
detection, survivor or inconclusive, restore from the backup, verify the checksum, run once more, and print the
three runs and the verdict as one block for the pull request. Choosing the mutant stays with the implementer.
Acceptance names a fixture test, a `Taskfile.yml` entry, a change to one `AGENTS.md` rules row, and a
`docs/testing.md` edit. The owner has since sequenced the work (3.4): a trial classifier on recorded mutants runs
first, and the ruling on the `AGENTS.md` row waits for it.

## 1. The claimed gap

The claim: no command performs the mutation procedure or classifies its outcome, and the rule's `AGENTS.md` row
says "review only". Result: **the gap is real in this repository and at the pin.** No task, script, Go symbol or CI
step here performs or classifies a mutation check. The rules row is "review only". At the pin there is none either,
only one-off evidence scripts inside proposals and archived changes (2g, Q14).

Searches that came up empty (run at the base, `git grep -n` over tracked content outside
`openspec/changes/archive/`):

- `mutate:check`: 0 hits. `gremlins`, `go-mutesting`, `mutesting`: 0 hits each.
- `mutation-check`: 1 hit, prose (`.agents/contracts/semengine-developer.md:142`, "mutation-check it like a
  refusal"), not a command.
- `mutate`: 6 hits, none a command: git state or repository scope (`semengine-architect.md:39`,
  `.agents/protocol.md:152`, `AGENTS.md:135`), task truth (`semengine-reviewer.md:39`), a CAS baseline
  (`semengine-reviewer.md:243`), and a comment line inside the manual procedure (`semengine-reviewer.md:53`, see 2a).
- `gopls workspace_symbol -matcher=fuzzy` for `Mutant`, `Survivor`, `Inconclusive`, `Detection`: no symbol in the
  module. `Mutat` matched only `TestMergeCheckUpToDateRule` and `mergeRun.status` (fuzzy noise); `Verdict` matched
  only `finalizeVerdict` in `internal/harness/lifecycletest/refowner_test.go:216`, a lifecycle-suite helper.
- `Taskfile.yml` declares 21 tasks; none runs a mutation check:

```text
9:  default:
14:  doctor:
19:  fmt:
25:  fmt:check:
33:  tidy:check:
38:  build:
43:  vet:
48:  lint:
56:  cleanup-roots:check:
61:  ledger:check:
68:  ledger:diff:
73:  vuln:
78:  test:unit:
83:  test:repeat:
90:  test:integration:
95:  merge:check:
100:  cover:check:
105:  spec:check:
110:  spec:queue:
115:  docs:check:
120:  verify:
```

- `scripts/` holds 12 files: `admission-lock.sh cleanup-roots-check.sh cover-check.sh doctor.sh gopkgs.sh
  lint-test-ports_fixture_test.sh lint-test-ports.sh merge-check.sh openspec-queue.sh test-integration.sh
  tree-state.sh verify.sh`. None backs up a file, applies a change, or classifies a test outcome.

The row itself:

```text
AGENTS.md:99 — | A mutation check reports survivors and inconclusive runs as such, never as detections; fuzz seed replay and exploration are reported apart; the pull request records what was not covered | `docs/testing.md`, "Show that the test can fail" and "What the pull request records"; developer contract § Handoff | review only |
```

What does exist is mutation built into the tests of the repository's own guards: paired sensitivity tests, the
lifecycle failpoint matrix, and the fixed-port fixture test (`docs/testing.md:153-163`). They prove each guard can
fail; they do not run the procedure against an arbitrary test. They are listed under category 5.

## 2. Every current spelling of the fact being modeled

The change models six facts. Each is listed with every home it has today, here and, where #46 carried it from, at
the pin.

### 2a. The procedure

```text
docs/testing.md:112 — ## Show that the test can fail
docs/testing.md:114 — A green test tells you nothing until you have seen it go red for the right reason. A mutation check makes one
docs/testing.md:117 — 1. Run the selected test on the unmodified code. It passes, and it actually ran (use `-v` and check the name).
docs/testing.md:118 — 2. Make one deliberate, plausible wrong change to the implementation, the one the test is meant to catch. Leave the
docs/testing.md:119 —    test, its inputs and its expectations untouched. The change must compile and the test must reach it.
docs/testing.md:120 — 3. Run the test again. The intended assertion fails, for the reason you expected. A timeout, a build error, or a
docs/testing.md:121 —    different assertion failing does not count.
docs/testing.md:122 — 4. Put the original code back and run the test again. It passes.
docs/testing.md:124 — Keep a copy of every file before you change it (`cp`) and restore from that copy. Do not use `git checkout --`,
docs/testing.md:125 — `git restore`, `git stash` or `git reset --hard`; they can discard other work in the tree. Record the change you made,
docs/testing.md:126 — the commands, and the output of all three runs in the pull request.
docs/testing.md:131 —   assertion or scope, and report the survivor until it is resolved. If you change the test as a result, start again
docs/testing.md:132 —   from step 1 with the new test.
```

```text
.agents/contracts/semengine-developer.md:58 —    Step 3's "observe the intended failure" and any mutation check must be done with a `cp` backup you make first, and
.agents/contracts/semengine-developer.md:59 —    restoration verified by checksum:
.agents/contracts/semengine-developer.md:62 —    cp path/to/file.go "${TMPDIR:-/tmp}/file.go.bak" && shasum path/to/file.go   # BEFORE
.agents/contracts/semengine-developer.md:63 —    cp "${TMPDIR:-/tmp}/file.go.bak" path/to/file.go && shasum path/to/file.go   # AFTER; sums MUST match
.agents/contracts/semengine-developer.md:66 —    Do not verify restoration with `git diff --stat`: it reports nothing for untracked files, and new test files are
.agents/contracts/semengine-developer.md:67 —    routinely untracked. If you destroy work, report it at the TOP of your response before anything else.
.agents/contracts/semengine-developer.md:142 —   Write the test that observes the signal and mutation-check it like a refusal (skip the emit, and the test MUST
.agents/contracts/semengine-developer.md:180 — The reasoning behind these rules, and the mutation procedure step by step, is in `docs/testing.md`.
.agents/contracts/semengine-developer.md:197 — - A property-based test encodes an invariant the design cited, never one inferred from the implementation. The
.agents/contracts/semengine-developer.md:198 —   generator must provably reach every boundary the cited clause names, by construction or by a committed shrunk
.agents/contracts/semengine-developer.md:199 —   counterexample from a mutation kill; a wide range that merely strides a bound catches an off-by-one only
```

```text
.agents/contracts/semengine-reviewer.md:47 —    Mutation evidence is encouraged; a read-only reviewer requests it from the implementing agent and verifies the
.agents/contracts/semengine-reviewer.md:48 —    artifact. If the user separately authorizes the reviewer to make changes, **`cp` is the only sanctioned
.agents/contracts/semengine-reviewer.md:49 —    backup/restoration mechanism** for a mutation check:
.agents/contracts/semengine-reviewer.md:52 —    cp path/to/file.go "${TMPDIR:-/tmp}/file.go.bak" && shasum path/to/file.go   # BEFORE mutating; record the sum
.agents/contracts/semengine-reviewer.md:54 —    cp "${TMPDIR:-/tmp}/file.go.bak" path/to/file.go && shasum path/to/file.go   # restore; sum MUST match
.agents/contracts/semengine-reviewer.md:57 —    **Verify restoration with checksums, not `git diff --stat`.** `git diff --stat` reports nothing at all for untracked
.agents/contracts/semengine-reviewer.md:58 —    files, and new test files under review are routinely untracked. Compare the recorded checksum of every file you
.agents/contracts/semengine-reviewer.md:59 —    touched, and additionally confirm `git status --porcelain` has the same number of entries as when you started.
.agents/contracts/semengine-reviewer.md:196 — - Verify controlled mutation evidence where a guarantee rests on a refusal or signal: baseline, valid mutant, intended
.agents/contracts/semengine-reviewer.md:197 —   assertion, and restored baseline. A completion checkbox is not evidence. A survivor or inconclusive run claimed as
.agents/contracts/semengine-reviewer.md:198 —   a detection, a generated failure not replayed with the same input against both versions, or an unresolved survivor
```

Archived changes restate it per change:

```text
openspec/changes/archive/2026-10-02-await-last-error/design.md:52 — ## Mutation and verification
openspec/changes/archive/2026-10-02-await-last-error/design.md:56 — must fail because the earlier error remains wrapped and "(no error)" is absent. A build error, timeout,
openspec/changes/archive/2026-10-02-await-last-error/design.md:57 — or unrelated failure is inconclusive. Back up `await.go` with `cp`, restore that copy, verify matching
openspec/changes/archive/2026-10-02-await-last-error/design.md:58 — checksums, and rerun. Record all three verbose runs and the exact mutation in the PR.
```

The source `docs/testing.md` was carried from (#46; `docs/testing.md:334-335`):

```text
semstreams@8b99efe9:docs/contributing/01-testing.md:180 — Perform a bounded experiment in a disposable copy, or preserve an exact backup of every affected file. In SemStreams,
semstreams@8b99efe9:docs/contributing/01-testing.md:181 — follow the developer/reviewer contracts' `cp` backup and checksum restoration procedure; do not use Git restoration or
semstreams@8b99efe9:docs/contributing/01-testing.md:182 — stash commands. Keep concurrent and pre-existing work intact. During each comparison, change only the implementation
semstreams@8b99efe9:docs/contributing/01-testing.md:183 — under assessment; keep test code, generators, expectations, fixtures, and runner configuration fixed.
semstreams@8b99efe9:docs/contributing/01-testing.md:185 — 1. The unmodified baseline passes the selected checks, and the intended tests actually execute.
semstreams@8b99efe9:docs/contributing/01-testing.md:186 — 2. Apply a named, relevant mutation. The mutant builds and reaches the selected test.
semstreams@8b99efe9:docs/contributing/01-testing.md:187 — 3. The relevant assertion fails because it observes the intended violation.
semstreams@8b99efe9:docs/contributing/01-testing.md:188 — 4. Restore the original bytes, verify checksums, and rerun the selected checks successfully.
semstreams@8b99efe9:docs/contributing/01-testing.md:190 — If checks change after investigating a survivor, establish a new passing baseline and repeat the experiment. Do not
semstreams@8b99efe9:docs/contributing/01-testing.md:191 — restore over concurrent edits; isolate the experiment when exclusive ownership of affected files cannot be maintained.
```

A fourth spelling writes nothing in the tree. PR #59's reviewer (comment 5954707106,
`https://github.com/C360Studio/semengine/pull/59#issuecomment-5954707106`) reproduced the mutation check "with
`go test -overlay` so nothing in the worktree changed, under `-race -cpu 1`", with three mutants, all detected. The
limits of `-overlay`, as `go help build` states them on go1.26.6: "overlays will not appear when binaries and tests
are run through go run and go test respectively, and files beneath GOMODCACHE may not be replaced." Measured in a
scratch module (section 11): an overlaid `.go` file changed what was compiled (the test failed on the mutant), and a
file the test read at run time was read from disk, not from the overlay. The tree's files were unchanged.

The homes disagree on what verifies the restore (2c). The developer contract applies the `cp` rule to the
failing-first step as well as to mutation (`semengine-developer.md:58`). The pin allows "a disposable copy" as well
as a backup (`:180`) and forbids restoring "over concurrent edits" (`:190-191`). The carried page has neither.

### 2b. The outcome classification

Here:

```text
docs/testing.md:128 — Only step 3 going red as expected counts as a detection. Record any other outcome under its own name:
docs/testing.md:130 — - **Survivor:** the wrong change compiled and the test ran, but the test still passed. Look for the missing input,
docs/testing.md:133 — - **Inconclusive:** the run ended without the intended assertion failing: an error, a timeout of the whole run, a
docs/testing.md:134 —   different or unrelated failure, or a `-run` pattern that did not select the test. A failing exit code alone is not a
docs/testing.md:135 —   detection. Fix the cause and run the check again; never skip or weaken a test to get past it.
docs/testing.md:137 — If the test generates its inputs, replay the same input or seed against the wrong change and against the original
docs/testing.md:138 — code. Two different random samples differ for reasons unrelated to the change; if the same input cannot be replayed,
docs/testing.md:139 — the outcome is inconclusive.
```

```text
.agents/contracts/semengine-developer.md:203 — - A mutation check reports only its observed outcome. A survivor or an inconclusive run is recorded as such and is
.agents/contracts/semengine-developer.md:204 —   never a detection; a generated failure is replayed with the same input or seed against the wrong change and the
.agents/contracts/semengine-developer.md:205 —   original (`docs/testing.md`, "Show that the test can fail").
```

```text
.agents/skills/semengine-preflight/SKILL.md:84 — and put the output a reader needs in the PR. Separate failure, skip, no selected tests and compilation-only results.
.agents/skills/semengine-preflight/SKILL.md:86 — failure status when filtering output; do not infer success from a quiet log.
```

At the pin, the source of those lines:

```text
semstreams@8b99efe9:docs/contributing/01-testing.md:171 — Record survivors and invalid or inconclusive outcomes explicitly; none establishes detection. If the experiment
semstreams@8b99efe9:docs/contributing/01-testing.md:174 — An accepted deferral records remaining risk and does not establish detection. Required experiments and their evidence
semstreams@8b99efe9:docs/contributing/01-testing.md:175 — are part of the issue's acceptance and PR review, not a new CI gate. They establish sensitivity at the recorded
semstreams@8b99efe9:docs/contributing/01-testing.md:211 — Record execution outcomes separately from reviewer assessments:
semstreams@8b99efe9:docs/contributing/01-testing.md:213 — - **Detected:** the valid mutant caused the intended assertion failure, with passing baseline and restored checks.
semstreams@8b99efe9:docs/contributing/01-testing.md:214 — - **Survived:** the selected checks did not detect the mutation. Investigate missing inputs, assertions, or scope.
semstreams@8b99efe9:docs/contributing/01-testing.md:215 — - **Invalid:** the mutant could not build or was otherwise ineligible for this experiment.
semstreams@8b99efe9:docs/contributing/01-testing.md:216 — - **Inconclusive:** an error, external timeout, skipped or unselected test, or unrelated failure did not establish
semstreams@8b99efe9:docs/contributing/01-testing.md:217 —   detection by the intended assertion. A bounded assertion that observes a required termination failure can establish
semstreams@8b99efe9:docs/contributing/01-testing.md:218 —   detection; an outer runner timeout alone cannot.
semstreams@8b99efe9:docs/contributing/01-testing.md:220 — Assessments such as equivalent, outside the claimed scope, or deferred do not replace those observations. An
semstreams@8b99efe9:docs/contributing/01-testing.md:221 — equivalence assessment names the applicable contract and input domain, its reasoning, and its reviewer; passing the
semstreams@8b99efe9:docs/contributing/01-testing.md:222 — selected tests is not the argument. Unresolved survivors remain unresolved. A runner's zero exit may mean its
semstreams@8b99efe9:docs/contributing/01-testing.md:223 — evaluation succeeded; inspect the report. A nonzero exit alone does not establish detection of the intended fault.
semstreams@8b99efe9:docs/contributing/01-testing.md:238 — Distinguish measurements from judgments. Output can establish an observed assertion failure; whether it adequately
semstreams@8b99efe9:docs/contributing/01-testing.md:239 — challenges a requirement remains a review assessment. An equivalence argument or a reviewer finding no counterexample
```

What the carry by #46 dropped or changed in "Show that the test can fail" (`docs/testing.md:112-163`; each term was
searched in that range and found zero times: `equivalen`, `disposable`, `concurrent`, `invalid`, `checksum`,
`runner configuration`, `bounded assertion`, `assessment`):

| Pin (`01-testing.md`) | `docs/testing.md` | Change |
| --- | --- | --- |
| Four outcomes: Detected, Survived, **Invalid** ("could not build or was otherwise ineligible"), Inconclusive (`:213-218`) | Three outcomes (`:128-135`); a build error "does not count" (`:120`) and Inconclusive covers "an error" (`:133`) | Invalid dropped; its cases fall under Inconclusive |
| "A bounded assertion that observes a required termination failure can establish detection; an outer runner timeout alone cannot." (`:217-218`) | "A timeout ... does not count" (`:120`); Inconclusive includes "a timeout of the whole run" (`:133`) | The rule that a bounded assertion observing termination can detect was dropped. This bears on Q2 |
| "Assessments such as equivalent, outside the claimed scope, or deferred do not replace those observations. An equivalence assessment names the applicable contract and input domain, its reasoning, and its reviewer" (`:220-222`); "Record execution outcomes separately from reviewer assessments" (`:211`) | absent | Dropped. This bears on Q3 |
| "A runner's zero exit may mean its evaluation succeeded; inspect the report. A nonzero exit alone does not establish detection" (`:222-223`) | "A failing exit code alone is not a detection." (`:134-135`) | Half kept |
| "Distinguish measurements from judgments. Output can establish an observed assertion failure; whether it adequately challenges a requirement remains a review assessment." (`:238-239`) | absent | Dropped. #79 keeps "whether the mutant is plausible" as review only |
| "Required experiments and their evidence are part of the issue's acceptance and PR review, not a new CI gate." (`:174-175`); "This discipline does not select a mutation runner" (`:71-72`) | absent | Dropped. These are the pin's stated reasons for having no runner |
| Restore: "Restore the original bytes, verify checksums" (`:188`) | "Put the original code back" (`:122`) | Checksum dropped from the page; it is kept in both contracts (2c) |
| Disposable copy, and "Do not restore over concurrent edits; isolate the experiment ..." (`:180`, `:190-191`) | `cp` backup only (`:124`) | Dropped |
| Keep "test code, generators, expectations, fixtures, and runner configuration fixed" (`:182-183`) | "Leave the test, its inputs and its expectations untouched" (`:118-119`) | Runner configuration dropped |

Spellings in the record. On PR #48 (open draft), each item either uses a category neither page names, counts as
detected what both pages call inconclusive, or contradicts another PR #48 comment. Comment IDs are on
`https://github.com/C360Studio/semengine/pull/48`. All are posted from one login, `cglusky`, for the owner and both
agents (`AGENTS.md:92`).

- PR #48 body, "Not covered": "Two mutants survived. One is `Close`'s timer cancel, also stopped by the watchdog's
  `closed` check. The other is equivalent: the `[]byte` skip." An **equivalent** mutant, recorded with no contract,
  domain or reviewer named (compare pin `:220-222`).
- Comment 5970724192: "Mutants: 13 detected and 1 equivalent survivor (removing the `[]byte` skip). The cycle
  guard's mutant counts as detected only because unbounded recursion killed the process; no assertion fired." A
  **process kill counted as a detection.**
- Comment 5972450374: "14 mutants, one per test, were all detected. The `LatestNeverWaitsOnCollection` mutant (a lock
  held across the walk) is caught only by the `go test` timeout". A **whole-run timeout inside an "all detected"
  count.** The same comment: "the cycle-guard mutant was caught by a bounded child process overflowing its stack", a
  **stack overflow counted as a detection.** The PR body lists the timeout mutant under "Not covered" without the
  word "inconclusive".
- Comment 5968396676: "M1 Shutdown waits on done only -> UnderABlockedCallback: deadlock panic, detected", the panic
  being "panic: deadlock: all goroutines in bubble are blocked" (a `synctest` bubble). A **runtime panic counted as a
  detection.** The same comment records a correctly classified first run: "(first run, before the test had a failure
  path: inconclusive, test timed out after 10m0s)".
- Comment 5954727965 counts "nil check removed from `Run` ... detected: nil-pointer panic in the test (no refusal)".
  Comment 5957969940 classifies the same kind of failure the other way: "nil guard removed → `panic: runtime error:
  invalid memory address or nil pointer dereference` ... That is a loud failure, not a named test failing." **Two
  readings of a nil-pointer panic** on one pull request.
- Comment 5968396676, "M3b TTL Close skips <-c.done -> 100/100 fail once the exited check moved inside the stop
  goroutine (12/20 before)", and `openspec/changes/archive/2026-10-01-flake-defense/inventory.md:480-482` ("failed 60
  of 60 ... and passed 60 of 60"). **Rate-based detections**, a record shape the protocol also asks of a flake fix:

```text
.agents/protocol.md:130 —   (`Closes #n` for each). No comment or label on a pull request exempts it. A pull request that closes a flake shows
.agents/protocol.md:131 —   the reproduction: the command, and how often it failed before the fix and after. A flake that comes back after its
```

- Comments 5954727965 and 5973849025 classify a hang as inconclusive, matching both pages.
- Comments 5968489295 and 5974002766 (Codex reviews): the implementer's mutants "remain implementer-reported, not
  independently artifact-verified or replayed" and "Historical failing-first and mutation outcomes remain
  UNVERIFIED implementer reports".

### 2c. The backup, restore and restore check

```text
docs/testing.md:124 — Keep a copy of every file before you change it (`cp`) and restore from that copy. Do not use `git checkout --`,
docs/testing.md:125 — `git restore`, `git stash` or `git reset --hard`; they can discard other work in the tree. Record the change you made,
```

```text
.agents/contracts/semengine-developer.md:59 —    restoration verified by checksum:
.agents/contracts/semengine-developer.md:62 —    cp path/to/file.go "${TMPDIR:-/tmp}/file.go.bak" && shasum path/to/file.go   # BEFORE
.agents/contracts/semengine-developer.md:63 —    cp "${TMPDIR:-/tmp}/file.go.bak" path/to/file.go && shasum path/to/file.go   # AFTER; sums MUST match
.agents/contracts/semengine-developer.md:66 —    Do not verify restoration with `git diff --stat`: it reports nothing for untracked files, and new test files are
```

```text
.agents/contracts/semengine-reviewer.md:52 —    cp path/to/file.go "${TMPDIR:-/tmp}/file.go.bak" && shasum path/to/file.go   # BEFORE mutating; record the sum
.agents/contracts/semengine-reviewer.md:54 —    cp "${TMPDIR:-/tmp}/file.go.bak" path/to/file.go && shasum path/to/file.go   # restore; sum MUST match
.agents/contracts/semengine-reviewer.md:57 —    **Verify restoration with checksums, not `git diff --stat`.** `git diff --stat` reports nothing at all for untracked
.agents/contracts/semengine-reviewer.md:58 —    files, and new test files under review are routinely untracked. Compare the recorded checksum of every file you
.agents/contracts/semengine-reviewer.md:59 —    touched, and additionally confirm `git status --porcelain` has the same number of entries as when you started.
```

Four spellings of the restore check:

1. `docs/testing.md:124-125`: copy with `cp`, restore from the copy; **no checksum** is required.
2. Both contracts: `shasum` before and after, sums must match. Bare `shasum` is **SHA-1** (measured: `shasum --help`
   prints `-a, --algorithm   1 (default), 224, 256, ...`). The reviewer contract adds a `git status --porcelain`
   entry count (`semengine-reviewer.md:59`).
3. #79 and PR #59: **SHA-256** (PR #59 body: "runtime and backup SHA-256 both matched before and after:
   `a65eb205...c6a2`").
4. The pin's evidence scripts: **MD5** via `md5 -q`, a macOS command (2g).

Two existing proofs that a command left files as they were, both SHA-256:

```text
internal/harness/pindiff/nowrite_test.go:13 — // harness-boundaries › "Carried entries match the pin" and "Pin difference command": the program
internal/harness/pindiff/nowrite_test.go:14 — // writes nothing inside the repository. Each run happens with every directory of the tree made
internal/harness/pindiff/nowrite_test.go:15 — // read-only, so a write the program would clean up itself (a fetch directory removed on return)
internal/harness/pindiff/nowrite_test.go:16 — // still fails the run, and the tree's listing with content hashes is identical afterwards.
internal/harness/pindiff/nowrite_test.go:17 — func TestWritesNothingInTree(t *testing.T) {
internal/harness/pindiff/nowrite_test.go:40 —             before := snapshot(t, root)
internal/harness/pindiff/nowrite_test.go:41 —             readOnly(t, root)
internal/harness/pindiff/nowrite_test.go:46 —             after := snapshot(t, root)
internal/harness/pindiff/nowrite_test.go:47 —             if !maps.Equal(before, after) {
internal/harness/pindiff/nowrite_test.go:71 —         files[rel] = fmt.Sprintf("%x", sha256.Sum256(data))
```

```text
docs/provenance.md:45 — task ledger:diff -- <source_path>... | shasum -a 256   # the hash for the verdict
docs/provenance.md:53 — is not committed; the verdict comment records the commit, the command line, the per-entry lines, and the SHA-256 of
docs/provenance.md:54 — standard output.
```

Homes of "the tree is as it was", for whole trees rather than one file:

```text
scripts/verify.sh:18 — tracked_state() { git status --porcelain --untracked-files=no; git diff HEAD | shasum; }
scripts/verify.sh:34 — if [ "$(tracked_state)" != "$before" ]; then
scripts/verify.sh:39 — untracked=$(git status --porcelain | grep '^??' || true)
scripts/verify.sh:40 — [ -z "$untracked" ] || { echo "untracked files (reported, allowed):"; echo "$untracked" | sed 's/^/  /'; }
```

```text
scripts/tree-state.sh:2 — # Print a short fingerprint of the working tree: HEAD, the status listing, the
scripts/tree-state.sh:3 — # tracked diff, and the content of every untracked, unignored file (the status
scripts/tree-state.sh:4 — # listing names them but does not see an edit to one). The integration runner
scripts/tree-state.sh:5 — # records it with its evidence and cover-check.sh compares it, so coverage is never
scripts/tree-state.sh:13 —   git ls-files --others --exclude-standard | git hash-object --stdin-paths
```

```text
scripts/cover-check.sh:35 —   if ! grep -qx "tree_state=$(scripts/tree-state.sh)" "$run/runner.env"; then
```

`tree-state.sh` hashes untracked, unignored content, so a backup left inside the tree changes the fingerprint that
`cover-check.sh:35` compares with the last integration run.

**Restore when the run is interrupted or killed.** A restore driven from the script that runs `go test` depends on
how that script ends. The tree already records how that goes wrong:

```text
scripts/test-integration.sh:7 — #   - go test runs in its own process group and INT/TERM reach every process in
scripts/test-integration.sh:8 — #     it (SemStreams' traps never forward, so test binaries outlive an interrupt);
scripts/test-integration.sh:13 — # Must stay bash 3.2 compatible: it is macOS's /bin/bash.
scripts/test-integration.sh:15 — # Interrupting a run: Ctrl-C at a terminal, or SIGTERM from a script. A script that
scripts/test-integration.sh:16 — # starts the runner as an `&` job of a non-interactive shell starts it with SIGINT
scripts/test-integration.sh:17 — # ignored, and no shell can trap a signal ignored on entry; such a runner warns,
scripts/test-integration.sh:18 — # records int_ignored_on_entry=yes, and answers only to SIGTERM.
scripts/test-integration.sh:295 — trap 'on_signal INT' INT
scripts/test-integration.sh:296 — trap 'on_signal TERM' TERM
scripts/test-integration.sh:297 — # A signal ignored on entry cannot be trapped (POSIX). How the trap table then looks
scripts/test-integration.sh:298 — # differs between bash 3.2 and 5, so the disposition itself is probed: a child shell
scripts/test-integration.sh:299 — # inherits an ignored SIGINT (an exec keeps SIG_IGN), cannot trap it, and so survives
scripts/test-integration.sh:300 — # signalling itself; with SIGINT deliverable its trap fires and it exits 42.
scripts/test-integration.sh:317 — trap finish EXIT
```

```text
internal/harness/natsfixture/admission.go:71 — // ownerLive requires the recorded owner to be a running process on this host. A runner killed
internal/harness/natsfixture/admission.go:72 — // with SIGKILL never runs its EXIT trap, so its owner file, token included, outlives it; the lock
internal/harness/natsfixture/admission.go:73 — // that file records is then stale, and the next runner will quarantine it. The runner and its
```

```text
scripts/merge-check.sh:62 — trap 'rm -f "$errfile"' EXIT
```

```text
internal/harness/pindiff/main.go:34 —     ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
internal/harness/pindiff/main.go:35 —     code := run(ctx, ".", os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
```

```text
internal/harness/pindiff/pin.go:92 —     cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
```

```text
openspec/specs/harness-boundaries/spec.md:289 — exit 2 with a message that says the pin could not be read and that no entry was checked. When a fetch fails or is cut
openspec/specs/harness-boundaries/spec.md:290 — off, by the fetch bound or by an interrupt, no process the program started for it SHALL still be running when the
openspec/specs/harness-boundaries/spec.md:291 — program exits.
```

Measured with macOS's `/bin/bash` 3.2.57 (section 11). A script backs up a file, writes a mutant, restores in an
`EXIT` trap, and runs a 4-second foreground child in place of `go test`:

- **SIGTERM to the script alone:** the script exited 143 and the file was restored, but **4 seconds later**: the
  restore ran only after the foreground child finished, and the child was not signalled.
- **SIGKILL:** the script exited 137, **no restore ran, and the mutant stayed in the file.**

Every pin evidence script restores in an `EXIT` trap or a Python `finally` (2g), so a SIGKILL leaves its mutant in
place. The pin's text addresses ownership, not interruption: "Do not restore over concurrent edits; isolate the
experiment when exclusive ownership of affected files cannot be maintained" (`01-testing.md:190-191`).

### 2d. The test actually ran: an empty selection is not a pass

```text
docs/testing.md:117 — 1. Run the selected test on the unmodified code. It passes, and it actually ran (use `-v` and check the name).
docs/testing.md:134 —   different or unrelated failure, or a `-run` pattern that did not select the test. A failing exit code alone is not a
docs/testing.md:224 — A zero exit, or the number of checks you asked for, does not show that the checks ran. Run with `-count=1 -v` and
docs/testing.md:243 — fresh one. With `-v`, Rapid prints a summary of the checks it actually completed (`[rapid] OK, passed 100 tests`);
```

```text
.agents/skills/semengine-preflight/SKILL.md:84 — and put the output a reader needs in the PR. Separate failure, skip, no selected tests and compilation-only results.
.agents/skills/semengine-preflight/SKILL.md:86 — failure status when filtering output; do not infer success from a quiet log.
```

```text
openspec/specs/harness-boundaries/spec.md:139 — No `*_test.go` file and no Go file under `internal/harness/` SHALL contain `time.Sleep`. The check SHALL have no
openspec/specs/harness-boundaries/spec.md:140 — baseline, no allowlist and no inline exemption, and SHALL fail when it scanned no test file. A test file ported from
openspec/specs/harness-boundaries/spec.md:162 — No `*_test.go` file SHALL call `Skip`, `Skipf` or `SkipNow`, and no `*_test.go` file SHALL carry a build constraint
```

```text
scripts/gopkgs.sh:3 — # An empty module is stated explicitly ("0 package(s) found ... nothing to run")
scripts/gopkgs.sh:4 — # instead of letting go vet/test/govulncheck fail on an unmatched ./... pattern
scripts/gopkgs.sh:10 — if [ "$n" -eq 0 ]; then
scripts/gopkgs.sh:11 —   echo "go: nothing to run for: $*"
```

```text
scripts/cover-check.sh:5 — # profile. A package missing from its profile fails: no statements is not coverage.
```

```text
semstreams@8b99efe9:docs/contributing/01-testing.md:185 — 1. The unmodified baseline passes the selected checks, and the intended tests actually execute.
semstreams@8b99efe9:docs/contributing/01-testing.md:216 — - **Inconclusive:** an error, external timeout, skipped or unselected test, or unrelated failure did not establish
```

Measured (section 11): with go1.26.6 a `-run` pattern that selects nothing exits **0** and prints
`testing: warning: no tests to run` and `ok ... [no tests to run]`, so by exit status it reads as a pass.

### 2e. Replay with the same input

```text
docs/testing.md:137 — If the test generates its inputs, replay the same input or seed against the wrong change and against the original
docs/testing.md:138 — code. Two different random samples differ for reasons unrelated to the change; if the same input cannot be replayed,
docs/testing.md:139 — the outcome is inconclusive.
docs/testing.md:183 — yet: it enters with the first test that imports it, and no test in this repository uses it today. A native fuzz
docs/testing.md:239 — go test ./pkg/example -run '^TestPropName$' -count=1 -race -v -rapid.checks=100 -rapid.seed=1320
docs/testing.md:243 — fresh one. With `-v`, Rapid prints a summary of the checks it actually completed (`[rapid] OK, passed 100 tests`);
docs/testing.md:259 — green run is not proof the failure was replayed; read the `-v` output. Pass `-rapid.nofailfile` during a mutation
docs/testing.md:260 — check so the wrong change leaves no file behind.
docs/testing.md:262 — A `.fail` file is untracked and is not covered by `.gitignore`. `task verify` reports untracked files but does not fail
```

```text
docs/provenance.md:33 —    and name the behavior that changed. The check reads the working tree, untracked files included, so a stray file
docs/provenance.md:34 —    inside a carried package fails it as `carry entry <source_path>: <path>: only in the tree`. The likely one is a
docs/provenance.md:35 —    `.fail` file Rapid writes under the package's `testdata/rapid/` after a failing property test (`docs/testing.md`,
docs/provenance.md:36 —    "Running and replaying a Rapid test"): delete it, or move the case into a named test.
```

```text
semstreams@8b99efe9:docs/contributing/01-testing.md:203 — For a generated failure, replay the same input or operation sequence (or a stable seed) against the mutant and the
semstreams@8b99efe9:docs/contributing/01-testing.md:204 — original implementation before recording detection. If that comparison cannot be reproduced, record it as
semstreams@8b99efe9:docs/contributing/01-testing.md:205 — inconclusive rather than attributing a difference between random samples to the mutation.
```

Native fuzz targets replay only their seeds under plain `go test` (`docs/testing.md:170`, "`go test` and
`task test:unit` replay only the seeds"), so the same input is replayed without a flag. A seed flag matters only for
Rapid, and no package on `main` imports Rapid (`docs/testing.md:183`). PR #48 adds the first ones
(`pkg/types/entity_id_prop_test.go`, per its `docs/testing.md` hunk). #79's step 4 does not mention
`-rapid.nofailfile` (`docs/testing.md:259-260`). The pin's `run_mutations.sh` passes a fixed seed and
`-rapid.nofailfile` on every run (2g). A stray Rapid `.fail` file inside a carried package fails `task ledger:check`
(`docs/provenance.md:33-36`).

### 2f. The record a pull request carries

```text
docs/testing.md:126 — the commands, and the output of all three runs in the pull request.
docs/testing.md:312 — Mutation:     the wrong change, and the baseline, wrong-change and restored runs with their outcome
docs/testing.md:313 — Not covered:  rules not exercised, unresolved survivors, deferred checks and the reason for each
docs/testing.md:321 — - If the change met one of the criteria above, where are the baseline, wrong-change, and restored runs? Is any
docs/testing.md:322 —   survivor or inconclusive run reported as such, not as a detection?
```

```text
.agents/contracts/semengine-developer.md:217 — Summarize the implemented task slice, semantic blast radius, tests and exact results (the record in `docs/testing.md`,
.agents/contracts/semengine-developer.md:218 — "What the pull request records", including what was not covered and unresolved survivors), unresolved gates, and any
```

```text
docs/provenance.md:40 — The reviewer of a pull request that ports packages runs `task ledger:diff` at the commit under review, naming the
docs/provenance.md:41 — change's source paths:
docs/provenance.md:44 — task ledger:diff -- <source_path>...                   # read the differences
docs/provenance.md:45 — task ledger:diff -- <source_path>... | shasum -a 256   # the hash for the verdict
docs/provenance.md:53 — is not committed; the verdict comment records the commit, the command line, the per-entry lines, and the SHA-256 of
docs/provenance.md:54 — standard output.
```

```text
semstreams@8b99efe9:docs/contributing/01-testing.md:193 — Record the source revision and the experiment snapshot, including relevant uncommitted and untracked files; identify
semstreams@8b99efe9:docs/contributing/01-testing.md:194 — their contents with a retained patch/files and checksums or an equivalent reproducible artifact. Record the mutation
semstreams@8b99efe9:docs/contributing/01-testing.md:195 — location and operation, selected tests and commands, runner version/configuration, relevant environment, observed
semstreams@8b99efe9:docs/contributing/01-testing.md:196 — results, and replay seed or shrunk counterexample when available. Capture enough output to distinguish the intended
semstreams@8b99efe9:docs/contributing/01-testing.md:197 — assertion failure from a broken runner. Baseline, mutant, and restored runs must refer to the same recorded checks.
semstreams@8b99efe9:docs/contributing/01-testing.md:199 — Retain small experiment snapshots inline in the PR record; attach larger snapshots, including patches, relevant files,
semstreams@8b99efe9:docs/contributing/01-testing.md:200 — and checksums, to the PR and link them from the handoff. Session scratchpads and private agent memory are not durable
semstreams@8b99efe9:docs/contributing/01-testing.md:201 — evidence homes.
```

`docs/provenance.md:40-54` is the one existing home of "an on-demand command's output recorded in a pull request":
the verdict comment records the commit, the command line, the per-entry lines, and the SHA-256 of standard output.
PR #59 is the worked mutation record: the command, the exact wrong change, three fenced outputs with exit codes, the
SHA-256, and local copies under `.evidence/await-last-error/`, a path `.gitignore:16` ignores, so the pasted outputs
are what another machine can open (`AGENTS.md:95`; preflight `SKILL.md:82-84`).

### 2g. The single-test command line, and the pin's evidence scripts

Here, four spellings:

```text
docs/testing.md:54 — go test -race -count=1 -run '^TestAwait' ./internal/harness/probe/
docs/testing.md:239 — go test ./pkg/example -run '^TestPropName$' -count=1 -race -v -rapid.checks=100 -rapid.seed=1320
```

```text
Taskfile.yml:81 —       - scripts/gopkgs.sh go test -race -count=1 -cpu 1 ./...
```

- PR #59 and #79 step 3: `go test -race -count=1 -cpu 1 -run '^<TestName>$' -v <package>`.
- `docs/testing.md:54` has no `-cpu 1` and no `-v`; `:239` adds Rapid flags; `test:unit` runs `./...` without `-run`.
- None sets `-timeout`, so go's default applies: "The default is 10 minutes (10m)" (`go help testflag`).

Integration tests cannot be run this way:

```text
openspec/specs/nats-fixture/spec.md:9 — `natsfixture.Start` SHALL refuse with ErrNotAdmitted, naming `task test:integration`, unless
openspec/specs/nats-fixture/spec.md:10 — SEMENGINE_DOCKER_ADMISSION_TOKEN is present in the live lock owner file at SEMENGINE_DOCKER_ADMISSION_LOCK_DIR; the
```

```text
openspec/specs/integration-test-runner/spec.md:76 — The runner SHALL run `go test -race -failfast -tags=integration -count=1 -p 2 -timeout 10m
openspec/specs/integration-test-runner/spec.md:77 — -coverprofile=<evidence dir>/integration.coverprofile <packages>` (the profile is what the 80% coverage gate on
```

```text
.agents/skills/semengine-preflight/SKILL.md:60 — - **Docker-backed behavior (`internal/harness/natsfixture`, `scripts/test-integration.sh`):**
.agents/skills/semengine-preflight/SKILL.md:61 —   `task test:integration -- <pkgs>` to focus, then `task cover:check`; never `go test` directly, since the fixture
.agents/skills/semengine-preflight/SKILL.md:62 —   refuses to start without the runner's admission token. `SEMENGINE_NATS_IMAGE` is accepted only with a non-empty
```

```text
scripts/test-integration.sh:55 — packages=("$@")
scripts/test-integration.sh:56 — ((${#packages[@]} > 0)) || packages=(./...)
scripts/test-integration.sh:402 — argv=(test -race -failfast -tags=integration -count=1 -p 2 -timeout 10m
scripts/test-integration.sh:403 —   "-coverprofile=$evidence_dir/integration.coverprofile" "${packages[@]}")
scripts/test-integration.sh:528 —   printf '%s\n' "$evidence_dir" > "$root/.evidence/last-run"
```

The runner's specified interface is packages only, and each run rewrites `.evidence/last-run`, which
`task cover:check` reads.

At the pin, five evidence scripts run mutation checks. Each is a one-off kept as evidence inside an archived change or
a proposal; none is a command, task or CI step (Q14). Paths are at `8b99efe9`; line counts and sha256 of the fetched
copies:

| Path at the pin | Lines | sha256 of the fetched copy |
| --- | --- | --- |
| `openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py` | 46 | `d6dbacee716174228e0b1d96cc9e6919a023cac83f515bb88c5fb5dac2092599` |
| `openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/mutations.sh.txt` | 46 | `9a1472e83d75badc488d24ede50afe2bf614cbdaecf51383d13fd84145c12b5e` |
| `openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/missing-start-mutant.sh.txt` | 34 | `65a0229d8b6f80662197609f704f8bd6a33ab59256f43afb948a3ffe7f30490e` |
| `openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/accepted-start-order-mutant.sh.txt` | 38 | `dcdca56002420dbb6cfd7e08effff78360fe9543029b8ce91e8c94ee1386e362` |
| `docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh` | 124 | `5395183198f3673129fdbba4685cc651fbfc099a6d1c7536bdcfba0ca8ca9ce0` |

What they share: a backup outside the tree (`/private/tmp/...`, or a disposable copy of the tree), the mutant applied
as a string replacement that must match exactly once, a restore checked with `md5 -q` in an `EXIT` trap or a
`finally`, and **detection decided by exit status plus a caller-named expected-output fragment**:

```text
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py:8 — backup = Path("/private/tmp/gh1423-test-owner-support.bak")
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py:11 — before = subprocess.check_output(["md5", "-q", str(source)], text=True).strip()
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py:13 —     ("provisional_removed", "if !o.transferred {\n\t\to.finish(t, operationCtx)\n\t}", "if !o.transferred {\n\t\t// Mutation: abandon provisional ownership.\n\t}", "^TestGraphIngestOwnerAssertionExitAndCleanupError$/^setup-exit$", "child missing witness"),
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py:22 —         assert original.count(old) == 1, f"mutation anchor count {name}: {original.count(old)}"
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py:26 —             command = ["go", "test", "-race", "-count=1", "./processor/graph-ingest", "-run", selector]
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py:27 —             run = subprocess.run(command, cwd=root, capture_output=True, text=True, timeout=60)
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py:30 —         except subprocess.TimeoutExpired as exc:
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py:31 —             output = str(exc)
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py:32 —             exit_code = 124
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py:37 —                         "oracle": oracle, "oracle_reached": oracle in output, "log": str(log)})
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py:39 —         assert subprocess.check_output(["md5", "-q", str(source)], text=True).strip() == before
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py:41 —     subprocess.run(["cp", str(backup), str(source)], check=True)
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py:42 —     assert subprocess.check_output(["md5", "-q", str(source)], text=True).strip() == before
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-graph-ingest-test-cleanup/review/evidence/mutations.py:45 — if any(item["exit_code"] in (0, 124) or not item["oracle_reached"] for item in results):
```

```text
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/mutations.sh.txt:4 — backup=/private/tmp/gh1418-lifecycle-suite-mutation-backup.go
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/mutations.sh.txt:6 — original=$(md5 -q "$source_file")
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/mutations.sh.txt:10 —   if [[ "$actual" != "$original" ]]; then
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/mutations.sh.txt:11 —     echo "RESTORATION FAILED: $actual != $original" >&2
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/mutations.sh.txt:15 — trap restore EXIT
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/mutations.sh.txt:22 — if original.count(old) != 1: raise SystemExit(f'mutation match count {original.count(old)} != 1')
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/mutations.sh.txt:25 —   if go test -count=1 ./component -run "$pattern" > "/private/tmp/gh1418-mutant-${label}.log" 2>&1; then
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/mutations.sh.txt:26 —     echo "VALID MUTANT SURVIVED: $label" >&2
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/mutations.sh.txt:29 —   if ! rg -q "$assertion" "/private/tmp/gh1418-mutant-${label}.log"; then
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/mutations.sh.txt:30 —     echo "mutant failed for unintended reason: $label" >&2
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/mutations.sh.txt:34 —   echo "MUTANT KILLED $label: $assertion"
```

```text
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/missing-start-mutant.sh.txt:22 — if go test -race -count=1 ./component -run '^TestSharedLifecycleParallelReportedFailureKeepsLivePeerOwned$' > /private/tmp/gh1418-mutant-missing-start.log 2>&1; then
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/missing-start-mutant.sh.txt:23 —  echo 'missing-Start mutant survived' >&2; exit 1
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/missing-start-mutant.sh.txt:25 — if ! rg -q 'fixture missing accepted Start or reported peer failure; all workers joined' /private/tmp/gh1418-mutant-missing-start.log; then
semstreams@8b99efe9:openspec/changes/archive/2026-09-29-shared-lifecycle-test-cleanup/review/evidence/missing-start-mutant.sh.txt:27 —  echo 'missing-Start mutant failed for unrelated reason' >&2
```

```text
semstreams@8b99efe9:docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh:4 — evidence=/tmp/semstreams-gh1292-lintfix-evidence.688Kp8
semstreams@8b99efe9:docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh:5 — cd "$evidence/copy"
semstreams@8b99efe9:docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh:15 — trap cleanup EXIT
semstreams@8b99efe9:docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh:18 —   go test ./processor/graph-index \
semstreams@8b99efe9:docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh:19 —     -run '^(TestPropGraphIndexReconciliation|TestGraphIndexModel)' \
semstreams@8b99efe9:docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh:20 —     -count=1 -race -timeout=120s -v \
semstreams@8b99efe9:docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh:21 —     -rapid.checks=100 -rapid.seed=1292 -rapid.shrinktime=3s -rapid.nofailfile
semstreams@8b99efe9:docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh:44 — assert source.count(old) == 1, (path, source.count(old))
semstreams@8b99efe9:docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh:47 —   diff -u "$backup" "$file" > "$evidence/$name.patch" || [[ $? == 1 ]]
semstreams@8b99efe9:docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh:59 —   if ! rg -q '^FAIL|--- FAIL:' "$evidence/$name.mutant.log"; then
semstreams@8b99efe9:docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh:60 —     echo "$name did not produce a test assertion" >&2
semstreams@8b99efe9:docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh:63 —   cp "$backup" "$file"
semstreams@8b99efe9:docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh:64 —   [[ "$(md5 -q "$file")" == "$before" ]]
semstreams@8b99efe9:docs/proposals/graph-index-reference-model/evidence/lint-refactor/run_mutations.sh:117 — shasum -a 256 \
```

Facts from these scripts that bear on #79:

- Caller-named fragment: `mutations.py` names an `oracle` string per case and fails unless `oracle in output`
  (`:13-16`, `:37`, `:45`); the shell scripts require a named `rg` pattern and print "mutant failed for unintended
  reason" otherwise (`mutations.sh.txt:29-30`). `run_mutations.sh` accepts any `^FAIL|--- FAIL:` line (`:59-60`).
- Timeout: `mutations.py` bounds each run at 60 s, maps a timeout to 124, and does not count 0 or 124 as detection
  (`:27`, `:30-32`, `:45`). `run_mutations.sh` passes `-timeout=120s` (`:20`).
- Mutant targets: `mutations.py` mutates `processor/graph-ingest/test_owner_support_test.go`, a `_test.go` helper
  (`:7`). `run_mutations.sh` mutates production files and, labelled "synthetic", a `_test.go` model helper
  (`:90-93`). The two `.sh.txt` scripts mutate `component/lifecycle_test_suite.go`.
- `run_mutations.sh` works in a copy of the tree (`:4-5`, `cd "$evidence/copy"`) and records a patch per mutant with
  `diff -u` (`:47`).

## 3. Adjacent claims on the territory

### 3.1 Open pull requests

Command: `gh pr list --state open --json number,title,changedFiles,files` (three open, all drafts; re-run for this
revision). PR #48 reports `changedFiles` 279, so its full list was read with
`gh api --paginate repos/C360Studio/semengine/pulls/48/files --jq '.[].filename'` (279 names). Overlap was checked
against `Taskfile.yml`, `scripts/`, `AGENTS.md`, `docs/testing.md`, `docs/provenance.md`, `docs/repository-map.md`,
`.agents/`, `.github/`, `internal/harness/contract/`, `internal/harness/pindiff/` and `openspec/`.

| PR | Head, updated | Overlapping files | What the overlap is |
| --- | --- | --- | --- |
| #82 (this claim) | `8e594f3`, 0 files | none | the claim |
| #73 `claude/authority-one-spelling` | `f612ec5`, 2026-10-04T13:01Z | `AGENTS.md`, `docs/testing.md`, `docs/repository-map.md`; delta `openspec/changes/authority-one-spelling/specs/harness-boundaries/spec.md` | `docs/testing.md` hunk `@@ -153,7 +153,8 @@` replaces lines 155-156, **inside "Show that the test can fail"** (the sensitivity-test paragraph). `AGENTS.md` adds one row after line 69. `docs/repository-map.md` edits the `internal/harness/contract/` row (line 41). |
| #48 `claude/setup-04a-01-floor` | `064d38d`, 2026-10-04T15:35Z (revision 1 read `f8fa86d`; the hunk headers below are unchanged) | `AGENTS.md`, `docs/testing.md`, `docs/provenance.md`, `docs/repository-map.md`, `.agents/contracts/semengine-{architect,developer,reviewer}.md`, `internal/harness/contract/{boundaries,imports,signatures,testtext}_test.go`; deltas under `openspec/changes/setup-04a-01-floor/specs/` for `harness-boundaries`, `lifecycle-suite`, `nats-fixture` and new `background-work`, `process-host`, `transport-client` | `AGENTS.md`: eight rows added between lines 69 and 92 and the "What this is for" paragraph changed; row 99 untouched. `docs/testing.md`: hunks at lines 1-8, 102-108, 179-187, 233-243 and after 281; **none inside lines 112-163**. `docs/provenance.md`: one hunk at line 66, clear of `:33-54`. Contract hunks (developer `@@ -162`, `@@ -175`, `@@ -218`; reviewer `@@ -177`, `@@ -269`, `@@ -305`) change no line of the mutation passages in 2a and 2c; developer `:180` and `:218` appear in them only as unchanged context. Adds Rapid to `go.mod` and the first Rapid tests (2e). Adds a "No bare select" requirement that parses every Go file, test or not (it ignores strings and comments). |

No open pull request changes `Taskfile.yml`, `scripts/`, `.github/`, `.agents/skills/`, `internal/harness/pindiff/`
or `openspec/specs/merge-gate/`.

### 3.2 Current specs

**One current spec already owns an on-demand harness command.** `harness-boundaries` binds the program behind
`task ledger:check` and `task ledger:diff` (`internal/harness/pindiff`). `task ledger:diff` is run by hand, outside
`task verify`, and its output is hashed into a pull-request verdict (2f). Three of its statements bear directly on
issue #79's two mechanics, writing a mutant into the tree and reporting a three-way verdict:

```text
openspec/specs/harness-boundaries/spec.md:280 — ### Requirement: Carried entries match the pin
openspec/specs/harness-boundaries/spec.md:289 — exit 2 with a message that says the pin could not be read and that no entry was checked. When a fetch fails or is cut
openspec/specs/harness-boundaries/spec.md:290 — off, by the fetch bound or by an interrupt, no process the program started for it SHALL still be running when the
openspec/specs/harness-boundaries/spec.md:291 — program exits.
openspec/specs/harness-boundaries/spec.md:292 — The program SHALL write nothing inside the repository. `task ledger:check` SHALL fail whenever the program exits
openspec/specs/harness-boundaries/spec.md:293 — non-zero; `task` and `go run` replace the program's exit status with their own, so through `task` only pass or fail and
openspec/specs/harness-boundaries/spec.md:294 — the message are promised.
openspec/specs/harness-boundaries/spec.md:347 — ### Requirement: Pin difference command
openspec/specs/harness-boundaries/spec.md:357 — The program SHALL exit 0 when it ran, whether or not files differ, and 2 when an argument names no entry,
openspec/specs/harness-boundaries/spec.md:358 — when a named entry could not be compared, or when the ledger or the pin could not be read; through `task` only pass
openspec/specs/harness-boundaries/spec.md:359 — or fail and the message are promised. It SHALL write nothing inside the repository.
```

- **It writes nothing inside the repository** (`:292`, `:359`), held by `TestWritesNothingInTree`
  (`internal/harness/pindiff/nowrite_test.go:13-52`). That test makes every directory read-only, so even a write the
  program cleans up itself fails, and compares SHA-256 snapshots before and after (`:54-78`).
- **Through `task`, only pass or fail and the message are promised** (`:293-294`, `:358-359`), because `task` replaces
  the program's exit status. Measured (section 11): `task` 3.51.1 returns **201** for a command that exits 1, 2 or 3;
  `task -x` returns the command's own status. A three-way verdict carried in the exit status does not survive
  `task mutate:check`.
- **A fetch cut off by its bound or by an interrupt leaves no process running** (`:289-291`), held by
  `TestFetchLeavesNoProcess` (`internal/harness/pindiff/process_test.go`).

The program's own shape is in category 5 (i) and (ii).

No spec covers a mutation check. The nearest purposes:

```text
openspec/specs/merge-gate/spec.md:4 — The merge gate is what must hold before a pull request can merge to `main`. This capability covers the parts of it
openspec/specs/merge-gate/spec.md:5 — that defend against flaky tests: how often and under which settings `task verify` runs the unit tests, that a
openspec/specs/merge-gate/spec.md:6 — failing gate names the test, that no merge happens while a known flake is open, and that a pull request's green was
```

```text
openspec/specs/harness-boundaries/spec.md:4 — Harness boundaries are the machine-checked rules that keep test machinery out of production code, keep SemEngine's
```

Other spec requirements that constrain whatever the change adds:

- `harness-boundaries`, "No sleeps in tests" and "No skipped or hidden tests", match **text** in every `*_test.go`
  file (`spec.md:139-141`, `:162`). A Go test that plants test source as a string must split those literals, as
  `internal/harness/contract/testtext_test.go:14` (`var sleepLiteral = "time" + ".Sleep"`) and
  `cleanuproots_test.go:10` ("Split so this file does not contain the shape the guard refuses.") do. The guards list
  files with `git ls-files --cached --others --exclude-standard` (`internal/harness/contract/repo_test.go:39`), so
  untracked files count.
- `merge-gate`, "Varied and repeated unit runs": `TestUnitInvocationsPinned` reads only the `test:unit` and
  `test:repeat` commands and requires `test:repeat` to be the last step of `scripts/verify.sh`
  (`internal/harness/contract/mergegate_test.go:27-37`, `:99-112`). The CI `verify` job's 15-minute limit is pinned:

```text
internal/harness/contract/mergegate_test.go:129 — const verifyTimeoutMinutes = 15
```

```text
.github/workflows/ci.yml:26 —     timeout-minutes: 15
.github/workflows/ci.yml:54 —       - run: task verify
```

The last three CI runs on `main` took 3m16s, 3m05s and 3m25s, created to updated (`gh run list --branch main
--workflow ci.yml --limit 3`; runs 37121703748, 37120876944, 37028259077). For `integration-test-runner` and
`nats-fixture`, see 2g.

### 3.3 OpenSpec changes, ADRs, ledger

- Active changes on `main`: none (`openspec/changes/` holds only `archive/`).
- Archived changes that ran mutation checks by hand: `2026-10-02-await-last-error` (`design.md:52-58`;
  `tasks.md:10`, `:44`, "three mutants, all detected"), `2026-10-02-carry-check` (`tasks.md:65`, "the two mutants that
  survived review are detected"), `2026-10-03-review-gate-check` (`design.md:281`; `tasks.md:110-116`, "all seven
  detected, no survivor among them ... an eighth wrong change that survived"), `2026-10-01-flake-defense`
  (`inventory.md:480-482`).
- ADRs: none on `main` (`ls docs/adr`: no such directory). PR #48 adds ADR-102 and ADR-104, both about entity IDs.
- Admission ledger: the pattern #79 names is an `adapt` row carried from SemStreams:

```text
docs/admission-ledger.yaml:174 — - source_path: scripts/lint-test-ports_fixture_test.sh
docs/admission-ledger.yaml:175 —   source_sha: 5457b3458936f668b71d2fea061f67f8d7d01e67
docs/admission-ledger.yaml:176 —   consumer_purpose: proves lint-test-ports.sh detects and accepts the documented shapes
docs/admission-ledger.yaml:183 —   known_risks: none known
```

  No ledger row is owed for the command itself: the pin has no admitted mutation command (Q14, closed).

### 3.4 Issues and rulings

Commands: `gh issue list --state all --limit 200`, `gh issue view <n>`, then `gh search issues --repo
C360Studio/semengine` for "mutation", "retrospective", "survivor", "Lesson".

- #79, **owner sequencing, comment 5981621215** (2026-10-04, recorded by the claiming session). The owner's words:
  "i think my answer ddepnds on how realistic it is to expect a command to return solid results without a bunch of
  noise. it also sounds like we should wait for 48 and then 73 to merge before we do?", then "yes go that way, run
  the trial first". What was agreed: (1) a throwaway trial classifier runs first, on recorded mutants (PR #59's, and
  PR #48's survivors, its timeout case and other recorded mutants) and on planted controls, and its noise is counted;
  nothing from it goes in the repository; (2) the ruling on the `AGENTS.md` row waits for the trial; (3) the script,
  its fixture test and the `Taskfile.yml` entry proceed now; (4) the `AGENTS.md` row and the `docs/testing.md`
  section wait for #48, then #73, to merge, written as `Hold:` on their tasks; (5) seed replay waits for #48.
- #22, **owner ruling, comment 5981684453**: the pin's `docs/contributing/01-testing.md` may be read for this
  inventory and design only (header).
- **#60**, open, milestone Slice 04A, no comments: "a command that writes a ported package's expected files from the
  pin". It is a second proposed command that writes inside the repository, and its first design question is the
  conflict this change meets: "**It writes inside the repository, and `pindiff` is specified to write nothing
  there** (`harness-boundaries`, 'The program SHALL write nothing inside the repository'; `TestWritesNothingInTree`).
  Decide whether this is a third mode with its own requirement and its own test, or a separate program, so the
  check's guarantee is not weakened." Also: "It must never silently discard a repair."
- #80, open, unclaimed, no milestone: a guard that every test names the requirement it proves. Its owner ruling
  (comment 5981307449, scope option A) places "the convention text in `docs/testing.md`" and a
  `harness-boundaries` requirement in that change, so both changes edit `docs/testing.md` and the `AGENTS.md` rules
  table. #80's body: "#79 (scripted mutation check) is independent and can proceed in parallel."
- #81, open, milestone Slice 04A: requirement citations in PR #48's tests. No file overlap with #79 named.
- #46, closed: carried the survivor and inconclusive rules from the pin's testing page into `docs/testing.md` and the
  contracts; its scope was "Docs and contracts only; no Go code". What the carry dropped is in 2b.
- #37, closed: source of the command-versus-prose finding that `AGENTS.md:60-62` restates ("Of its defect-class
  labels, the one with a command behind it closed ... The ones policed by review prose stayed open").
- #57, closed: a Rapid failure that comes and goes is a known flake (`docs/testing.md:251-254`).
- No issue labelled `class:flake` is open; no other issue mentions a mutation command.

### 3.5 Statements a new command must reconcile with

The binding statements are the spec's (3.2):

```text
openspec/specs/harness-boundaries/spec.md:292 — The program SHALL write nothing inside the repository. `task ledger:check` SHALL fail whenever the program exits
openspec/specs/harness-boundaries/spec.md:293 — non-zero; `task` and `go run` replace the program's exit status with their own, so through `task` only pass or fail and
openspec/specs/harness-boundaries/spec.md:294 — the message are promised.
openspec/specs/harness-boundaries/spec.md:358 — when a named entry could not be compared, or when the ledger or the pin could not be read; through `task` only pass
openspec/specs/harness-boundaries/spec.md:359 — or fail and the message are promised. It SHALL write nothing inside the repository.
```

They govern `pindiff`'s two commands only; no statement makes "writes nothing in the repository" a rule for every
command. Prose statements say the same thing more widely:

```text
AGENTS.md:28 — `task --list` shows every command with its rationale.
AGENTS.md:32 — task fmt          # format Go sources (the only command that writes)
AGENTS.md:60 — The linked file is the rule; this table is only its index. "Review only" means no command fails when the rule is
AGENTS.md:61 — broken. In SemStreams' record those are the rules that drifted: the defect class with a command behind it closed, and
AGENTS.md:65 — indexed by their spec. A change that can turn a "review only" row into a failing command should.
```

```text
.agents/skills/semengine-preflight/SKILL.md:22 — One Task entrypoint serves local work and CI. `task --list` shows what exists; inspect the `Taskfile.yml` and the
.agents/skills/semengine-preflight/SKILL.md:28 — | `task fmt` | Formats Go sources; the only task here that changes files |
.agents/skills/semengine-preflight/SKILL.md:29 — | `task fmt:check` | Fails if any Go source needs formatting; writes nothing |
.agents/skills/semengine-preflight/SKILL.md:46 — | `task verify` | The checks above except `doctor`, `fmt`, `spec:queue`, `merge:check`, cheapest first with `test:repeat` last; prints each step's time; fails on tracked-file change |
```

```text
Taskfile.yml:3 — # One entrypoint for local verification and CI; both run these same commands.
Taskfile.yml:20 —     desc: Format Go sources (changes files)
```

```text
docs/setup-plan.md:240 — - `task fmt` changes formatting; verification checks formatting without changing tracked files.
docs/setup-plan.md:243 —   tests and integration/consumer lanes as they exist. It must leave tracked files unchanged.
```

- `AGENTS.md:32` and the preflight skill (`SKILL.md:28`) say `task fmt` is the only command that writes. A command
  that applies a mutant writes a source file, even if only until it restores it. #60 would be a second such command.
- `AGENTS.md:60-61` defines "review only" as "no command fails when the rule is broken". Two rows already record an
  on-demand command that prints or displays, and fails nothing, as "review only":

```text
AGENTS.md:77 — | A ported package has an admission-ledger row; a `carry` row matches the pin (SemStreams at its `source_sha`) | `.agents/contracts/semengine-architect.md` § Extraction slices; `docs/provenance.md` rule 5; `docs/admission-ledger.yaml` | `task ledger:check` (in `task verify`) for the schema of the rows present and, fetching the pin, for each `carry` row's `.go` and `testdata` files; `TestCheckSensitivity`, `TestCommandExitStatus` and `TestLedgerCheckWiring` hold it. That a ported package has a row, its `README.md`, a new sub-package under a carried destination, and `adapt` rows (printed by `task ledger:diff`) are review only |
AGENTS.md:93 — | Every OpenSpec task can be ticked in or before the archive commit; a hold is written as `Hold:` on the unticked task it stops | `.agents/protocol.md`, "Target state, task truth, holds"; reviewer contract § Contract and task-truth review | review only; `task spec:queue`, run in the claim's worktree, displays a hold written this way and fails nothing |
```

- Every surface that lists commands would gain a line: the `AGENTS.md` "Commands" block (`AGENTS.md:26-52`), the
  preflight skill table (`SKILL.md:26-46`, which today omits `task ledger:diff`), and the `docs/repository-map.md`
  script rows (`:44-46`).

### 3.6 Consumer asks

None in scope. The command's users are agents and the owner working in this repository; no consumer repository was
read.

## 4. The consumer at birth

The surfaces #79 names: `task mutate:check`, `scripts/mutation-check.sh`, a fixture test, and the printed report
block. `task --list` would show the task (`AGENTS.md:28`). **All are new surface**: the pin has no mutation command,
task or CI step (Q14), so none goes through the surface audit.

Present consumers of the procedure, each a place that performs or checks a mutation check today:

- the implementing agent, bound by `semengine-developer.md:58-63`, `:142`, `:203-205` and `:217-218`;
- the reviewer, who verifies the evidence (`semengine-reviewer.md:47-49`, `:196-198`), and who reproduced PR #59's
  mutants with `go test -overlay` (2a);
- the record template line `docs/testing.md:312`;
- pull requests and changes that did it by hand: PR #59 (merged), PR #48 (open; several dozen mutants across its
  comments, 2b), and the four archived changes in 3.3;
- the owner's trial (3.4), which runs a throwaway classifier against PR #59's and PR #48's recorded mutants.

The default path today, for an implementer who skips the procedure or classifies by hand: no command fails. A wrong
classification is caught only if a reviewer reads the outputs. On PR #48 the Codex reviews say the outcomes were
not independently replayed (comments 5968489295, 5974002766), and comments 5972450374, 5970724192 and 5968396676
count a whole-run timeout, a process kill, a stack overflow and a deadlock panic among the detections.

## 5. The problem shape

Four shapes, each with its closest instances. Whether the design adopts each is a design-phase answer.

**(i) Run a child `go test` under a bound and classify its outcome into a closed set, never reading an ambiguous
outcome as a pass.** No instance here classifies a `go test` run. Instances that classify other outcomes the same
way:

```text
internal/harness/pindiff/main.go:1 — // Package main is the program behind `task ledger:check` and `task ledger:diff`. It compares
internal/harness/pindiff/main.go:2 — // admission-ledger entries with SemStreams at each entry's source_sha (harness-boundaries ›
internal/harness/pindiff/main.go:3 — // "Comparison with the pin"), fetching the pin into a temporary directory outside the repository.
internal/harness/pindiff/main.go:60 — // run is the program with its working directory, arguments, environment and output passed in. It
internal/harness/pindiff/main.go:61 — // returns the exit status: 0 when it ran (check: and every carry entry matches), 1 when check
internal/harness/pindiff/main.go:62 — // finds a carry entry that does not match, 2 when it could not do what was asked.
```

```text
scripts/merge-check.sh:25 — # A read that fails, or an answer that is not the shape asked for, exits 2
```

```text
scripts/gopkgs.sh:3 — # An empty module is stated explicitly ("0 package(s) found ... nothing to run")
scripts/gopkgs.sh:4 — # instead of letting go vet/test/govulncheck fail on an unmatched ./... pattern
```

```text
scripts/cover-check.sh:5 — # profile. A package missing from its profile fails: no statements is not coverage.
scripts/cover-check.sh:22 —   # Its output is printed: a test that fails here must be named, not discarded.
scripts/cover-check.sh:23 —   go test -count=1 -coverprofile="$unit" "./internal/harness/lifecycletest/" "./internal/harness/probe/"
scripts/cover-check.sh:30 —   if ! grep -qx 'go_test_status=0' "$run/runner.env" 2>/dev/null; then
```

```text
openspec/specs/merge-gate/spec.md:50 — `scripts/cover-check.sh` SHALL print the output of the `go test` run that produces its unit profile, so that a test
openspec/specs/merge-gate/spec.md:51 — failing in that run is named in the output of `task cover:check`.
```

`pindiff` returns 0, 1 or 2, where 2 means "could not do what was asked" (`main.go:60-62`), and its spec promises
only pass or fail through `task` (3.2). The integration runner records `go test`'s exit status (`go_test_status`,
read at `scripts/cover-check.sh:30`) but does not classify it. The closest instances of classifying a `go test` run
are at the pin: exit status plus a caller-named expected-output fragment (2g).

**(ii) Take temporary ownership of files in a shared tree and prove they end as they began, even when the run is
interrupted.** Instances:

- write nothing at all: `pindiff` works in a temporary directory outside the repository (`main.go:3`), proved by
  `TestWritesNothingInTree` (3.2); PR #59's reviewer reproduced mutants with `go test -overlay`, which writes nothing
  in the tree, within the limits `go help build` states (2a);
- work in a copy: the pin's `run_mutations.sh` (`:4-5`) and the pin's text "a disposable copy" (`01-testing.md:180`);
- back up and restore: the contracts (2c) and four pin scripts, restoring in an `EXIT` trap or `finally`;
- interruption: `scripts/test-integration.sh` gives `go test` its own process group and forwards INT and TERM to it
  (`:7-8`, `:295-296`), probes whether SIGINT was ignored on entry (`:297-300`), and releases in an `EXIT` trap
  (`:317`); a SIGKILL skips that trap and the next runner quarantines the stale lock (`admission.go:71-73`);
  `pindiff` cancels on SIGINT and SIGTERM and kills its fetch's process group (`main.go:34`, `pin.go:92`);
- proving the tree is unchanged: `scripts/verify.sh:18,34`, `scripts/tree-state.sh:13`, `nowrite_test.go:54-78`.

**(iii) A script or command with a test that plants each case.** Five instances, differing in what they check and
whether they run the real tool:

```text
scripts/lint-test-ports_fixture_test.sh:2 — # lint-test-ports_fixture_test.sh — verifies scripts/lint-test-ports.sh
scripts/lint-test-ports_fixture_test.sh:21 —   tmp=$(mktemp -d)
scripts/lint-test-ports_fixture_test.sh:22 —   trap "rm -rf '$tmp'" RETURN
scripts/lint-test-ports_fixture_test.sh:32 —   ( cd "$tmp" && bash "$OLDPWD/scripts/lint-test-ports.sh" >/dev/null 2>&1 )
scripts/lint-test-ports_fixture_test.sh:36 —   if [ "$expect" = "match" ] && [ $rc -ne 1 ]; then
scripts/lint-test-ports_fixture_test.sh:72 — echo "lint-test-ports fixture test: $PASS passed, $FAIL failed"
scripts/lint-test-ports_fixture_test.sh:73 — [ $FAIL -eq 0 ]
```

```text
Taskfile.yml:49 —     desc: Run the pinned revive, then the carried fixed-port guard and its fixture test
Taskfile.yml:53 —       - scripts/lint-test-ports.sh
Taskfile.yml:54 —       - scripts/lint-test-ports_fixture_test.sh
```

- `scripts/lint-test-ports_fixture_test.sh` (the instance #79 names): bash, run by `task lint`. It checks **exit
  status only** and discards the guard's output (`:32`, `>/dev/null 2>&1`). It cannot tell a guard that fired for
  the right reason from one that fired for another.
- Go-driven script tests in `internal/harness/contract/` check **output text** as well as exit status, against fake
  tools:

```text
internal/harness/contract/repo_test.go:102 — // requireViolation fails unless some violation mentions every fragment. Sensitivity tests use it
internal/harness/contract/repo_test.go:103 — // so a check that fires for the wrong reason does not count as detecting the seeded defect.
internal/harness/contract/repo_test.go:129 — // fakeBin writes executable scripts named by tools into a fresh directory and returns an
internal/harness/contract/repo_test.go:130 — // environment whose PATH starts with it, so a script under test runs the fakes instead of the
internal/harness/contract/repo_test.go:131 — // real tools. Tests here never invoke the real go or gh.
internal/harness/contract/repo_test.go:155 — func copyScript(t *testing.T, name string) string {
```

```text
internal/harness/contract/cover_test.go:130 — // TestCoverCheckPrintsFailingTest (merge-gate › "Coverage gate names a failing test"): in its
internal/harness/contract/cover_test.go:131 — // no-argument mode scripts/cover-check.sh runs go test itself, and a test that fails there must
internal/harness/contract/cover_test.go:132 — // be named in its output. A copy of the script runs in a throwaway root against a fake go that
internal/harness/contract/cover_test.go:133 — // prints a --- FAIL line and exits 1.
internal/harness/contract/cover_test.go:136 —     env := fakeBin(t, map[string]string{"go": "#!/bin/sh\necho '--- FAIL: TestPlanted (0.00s)'\necho FAIL\nexit 1\n"})
internal/harness/contract/cover_test.go:145 —         t.Fatalf("cover-check.sh output does not name the failing test (err=%v):\n%s", err, out)
```

```text
internal/harness/contract/cleanuproots_test.go:43 —         if err == nil || !strings.Contains(out, "x/x_test.go:4:") {
```

- `internal/harness/runner/runner_test.go` drives `scripts/test-integration.sh` with a fake `go` that has modes,
  including a planted hang:

```text
internal/harness/runner/runner_test.go:56 — // it can see; writes a testcontainers session id where the fixture would; and in FAKE_GO_MODE=hang
internal/harness/runner/runner_test.go:69 — if [ "${FAKE_GO_MODE:-ok}" = hang ]; then
internal/harness/runner/runner_test.go:118 — exit "${FAKE_GO_STATUS:-0}"
internal/harness/runner/runner_test.go:158 —     for name, body := range map[string]string{"docker": fakeDocker, "go": fakeGo} {
```

- `internal/harness/pindiff` tests its command end to end against the **real** toolchain: `TestCommandExitStatus`
  builds the program with the real `go build` and runs it from a temporary tree; it is named in `AGENTS.md:77` as
  holding the ledger rule. Its fixtures run real `git` against a local repository:

```text
internal/harness/pindiff/command_test.go:10 — // The program run as a separate process from the tree's root, as `task ledger:check` runs it
internal/harness/pindiff/command_test.go:11 — // (`go run ./internal/harness/pindiff check`): a planted carry violation exits non-zero and names
internal/harness/pindiff/command_test.go:15 — func TestCommandExitStatus(t *testing.T) {
internal/harness/pindiff/command_test.go:16 —     bin := filepath.Join(t.TempDir(), "pindiff")
internal/harness/pindiff/command_test.go:17 —     build := exec.CommandContext(t.Context(), "go", "build", "-o", bin, ".")
```

```text
internal/harness/pindiff/fixture_test.go:31 —     cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
internal/harness/pindiff/fixture_test.go:42 — func pinRepo(t *testing.T, snapshots ...map[string]string) (string, []string) {
```

- `TestEachFailpointTripsExactlyItsCheck` switches on one defect at a time and requires exactly the matching check
  to fail (`docs/testing.md:162-163`; `openspec/specs/lifecycle-suite/spec.md:49-50`).

**(iv) Print an evidence block a pull request quotes.** `docs/provenance.md:40-54` (2f): commit, command line,
per-entry lines and the SHA-256 of standard output. PR #59's body is the worked mutation instance (2f).

**Adoption sweep.** No existing instance classifies a test run in this repository, so the design will have to say
whether this change establishes a reusable primitive and, if so, list adopters (architect contract, "The adoption
sweep"). #60 is a second command that writes in the tree (3.4); whether that makes a shared primitive is for the
design.

## 6. Same-class collision table

**Triggered.** Revision 1 ruled it out because "the backup is a temporary local copy held for one run". The
measurements in 2c refute that premise for a killed run. After a SIGKILL the mutant stays in the tree, and the
backup outside it is the only copy of the file's original content, uncommitted edits included. Other readers of the
tree may also run during the window. The semantic class: **temporary ownership of source files in a shared working
tree while a child `go test` runs, and recovery of their original content however the run ends.** The table reports
owners and unknowns; it chooses nothing.

| Dimension | Inventory evidence |
| --- | --- |
| Semantic class | As above. Today only prose owns it (2a, 2c); no code holds a file on behalf of a run |
| Owners | Manual procedure: `docs/testing.md:124-125`, `semengine-developer.md:58-67`, `semengine-reviewer.md:47-59`. Write-nothing owner: `pindiff` under `harness-boundaries` `spec.md:292`, `:359` and `nowrite_test.go:13-52`; `go test -overlay` (PR #59 review). Process-group and signal owner: `scripts/test-integration.sh:7-8`, `:295-317`; `pindiff/main.go:34`, `pin.go:92`. Tree-state owners: `scripts/verify.sh:18,34`, `scripts/tree-state.sh`, `scripts/cover-check.sh:35`. Prior art at the pin: the five evidence scripts (2g) |
| Catalogs | None. No registry records an in-flight mutation or where its backup is. A search for `backup` and `.bak` outside the archive (`git grep -n -i -E`) finds only the two contracts' `${TMPDIR:-/tmp}/file.go.bak` lines. The only on-disk coordination record in the tree is the Docker admission lock path, `/tmp/semstreams-integration.lock` (`scripts/admission-lock.sh:7`), shared with SemStreams and unrelated to source files |
| Status | None for a mutation in progress. Nearest: the runner's `runner.env` keys (`go_test_status`, `signal`, `int_ignored_on_entry`), `.evidence/last-run`, the `tree-state.sh` fingerprint, and `verify.sh`'s "tracked files changed during verification" (`:35`) |
| Lifecycle | Manual: `cp`, edit, run, `cp` back, compare sums. Pin scripts: restore in an `EXIT` trap or `finally`. Runner: INT and TERM forwarded to `go test`'s process group, `finish` in an `EXIT` trap. `pindiff`: `signal.NotifyContext`, a cancelled fetch kills its process group. Measured (2c): a bash `EXIT` trap under SIGTERM runs only after the foreground child exits, and the child is not signalled; under SIGKILL it never runs. A script started as an `&` job of a non-interactive shell has SIGINT ignored and cannot trap it (`test-integration.sh:15-18`). No expiry exists |
| Ownership | One writer per worktree: a claim works "in its own worktree" (`AGENTS.md:120-121`); "The previous writer must stop or release ownership before pickup edits begin" (`.agents/skills/semengine-pickup/SKILL.md:44`); the reviewer "does not write on the branch" (`.agents/protocol.md:102`). No lock covers a source file. The pin: "Do not restore over concurrent edits; isolate the experiment when exclusive ownership of affected files cannot be maintained" (`01-testing.md:190-191`). The Docker lock is host-wide and joint with SemStreams (`admission-lock.sh:2-6`) |
| Readers | Anything that reads the worktree during the window: another `go test` or `task verify` in the same worktree (a reviewer's read-only run included); `gopls`; the text guards, which list untracked files (`repo_test.go:39`); `tree-state.sh` and `cover-check.sh:35`; `verify.sh:18`'s before-and-after compare; `task ledger:check`, which reads untracked files in carried packages (`docs/provenance.md:33-34`). How often these overlap a mutation run is unmeasured |
| Writers | The mutation's own write. `task fmt` is the one declared writer (`AGENTS.md:32`). Rapid writes `.fail` files under `testdata/rapid/` (`docs/testing.md:256-263`). The runner writes `.evidence/` and `cover:check` writes `coverage/`, both ignored. #60 proposes another writer |
| Recovery | Manual: `cp` from the backup and compare sums. After a SIGKILL: the backup is the only copy of the original, nothing records its path, and the procedure forbids `git restore` (`docs/testing.md:124-125`), which could recover only committed content anyway. Runner after SIGKILL: the next run quarantines the stale lock (`admission.go:71-73`). With `-overlay`: nothing to recover |

## 7. Adopter seam inventory

**Run, briefly, as the precedents did.** No consumer repository reaches this surface, but the changes that added
`task ledger:diff` and `task merge:check` answered the four questions for the people in this repository who carry
them (`openspec/changes/archive/2026-10-02-carry-check/inventory.md:128-138`;
`openspec/changes/archive/2026-10-03-review-gate-check/inventory.md:458-486`). The people here: **an implementing
session** (Claude or Codex) that has never opened the script; **the other agent's reviewing session**, which
verifies the evidence; **the owner**, who reads verdicts and rules on the row.

1. **What they must know today**, with the procedure as prose: back up outside the tree; which checksum; never
   `git restore`; run with `-v` and check the name, because an empty selection exits 0; set `-timeout` or wait 10
   minutes; a whole-run timeout, a panic or a crash exits 1 and is not a detection; for Rapid, the same seed and
   `-rapid.nofailfile`; integration tests only through the runner, which cannot select one test; no other process
   may read the worktree during the run; record three runs. That is ten facts. The architect contract counts more
   than two as a design finding.
2. **If they do nothing:** no command fails. A misclassification merges unless a reviewer re-reads the outputs (PR
   #48, 2b). A run killed mid-mutation leaves the mutant in a tracked file. `task verify` flags it only if a test
   fails on it, and then names the test, not the cause; `git diff` shows it.
3. **Where they find out:** a document (`docs/testing.md`, the contracts), the level the architect contract counts
   as a finding for a correctness fact. A misclassification is found only by a reviewer reading.
4. **What they should have to know:** for the design. The gap between 1 and 4 is the design's work.

## 8. Intent check

**Not applicable.** The change does not set or move a boundary: no port set, tier membership, package exclusion or
capability deferral. #79 leaves generators of mutants (`gremlins`, `go-mutesting`) out of scope. That limits
tooling; it does not exclude a product capability.

## 9. Claims in #79, checked

| # | Claim in #79 | Result | Evidence |
| --- | --- | --- | --- |
| H1 | The procedure is `docs/testing.md` "Show that the test can fail", lines 112–146 | **Stale range** | The section runs from line 112 to 163 (the next heading is at 165). Lines 141-151 say when to do it; 153-163 describe the built-in sensitivity tests. `docs/testing.md` last changed in `31f8739` (2026-10-03), before #79 was filed. |
| H2 | "The mutation rule is the one testing rule with no command behind it" | **False** | `AGENTS.md:97` (generated-checks decision) and `:98` (generated-run record) are testing rules also marked "review only"; `:96` is review only outside the structural guards. |
| H3 | Its `AGENTS.md` row is review only | True, with a caveat | `AGENTS.md:99`. The row bundles three rules: mutation outcomes, fuzz seed replay reported apart from exploration, and recording what was not covered. #79's acceptance moves only the first. |
| H4 | SemStreams' record: rules with a command closed, prose-policed ones stayed open (#37) | True | #37 body, paragraph 2; `AGENTS.md:60-62`. |
| H5 | PR #48 reports two survivors and one lock mutant caught only by the `go test` timeout; outputs implementer-reported | True | PR #48 body, "Not covered" (2b). The body lists the timeout mutant as not covered; the counting errors are in comments 5972450374, 5970724192, 5968396676 and 5954727965 (2b). |
| H6 | PR #59 did the `cp`, the SHA-256 before and after, and the three runs by hand and pasted them | True | PR #59 body, "Mutation check: detected". Its reviewer reproduced the mutants with `go test -overlay` instead (comment 5954707106). |
| H7 | `docs/testing.md:120–122` forbids `git checkout --`, `git restore`, `git stash` | **False line citation** | Those lines are steps 3 and 4. The prohibition is at `docs/testing.md:124-125`, on every open branch. |
| H8 | "Lesson 9 of the retrospective (2026-10-04)" names this gap | **Unlocated** | Not in the tree (`git grep -n -i -e 'lesson 9' -e retrospective -- .`: 0 hits) or in any issue or PR searched. #80 comment 5981307449 mentions a "retrospective session" relaying a ruling. #80's body attributes a different point to "lesson 9": the oracle rule. |
| H9 | Detection "only when the named assertion fails" | **Input missing from the signature** | The command takes `<package> <TestName> <mutant>`; nothing names the assertion. Prior art: the pin's scripts take a caller-named output fragment per mutant (2g). |
| H10 | Refuse a mutant spanning more than one file | Partly matches the pages | Both pages ask that the test, its inputs and expectations stay fixed (`docs/testing.md:118-119`; pin `:182-183`), not that the change is one file. #79 does not refuse a `_test.go` or `testdata` target. Two pin scripts mutated `_test.go` helpers, one labelled synthetic (2g). |
| H11 | Same seed flags (`-rapid.seed`, fuzz seed corpus) for generated tests | Partly applies | Fuzz seeds replay without a flag (`docs/testing.md:170`). No Rapid test exists on `main` (`:183`); the first comes with PR #48, and the owner deferred seed replay until then (3.4). `-rapid.nofailfile` is not mentioned; the pin script passes it (2g). |
| H12 | A whole-run timeout is inconclusive | Matches both pages | `docs/testing.md:133`; pin `:216-218`, which adds that "a bounded assertion that observes a required termination failure can establish detection", a sentence the carry dropped (2b). `probe_test.go:26-27` bounds 8 call sites only by the binary `-timeout`. |
| H13 | The fixture test follows `scripts/lint-test-ports_fixture_test.sh` | Pattern exists; it checks exit status only | Section 5 (iii). It is a SemStreams `adapt` ledger row. `pindiff`'s `TestCommandExitStatus` tests a command against the real toolchain. |
| H14 | "Small: one script, one fixture test, three doc edits" | Understated | The procedure is spelled in `docs/testing.md` plus both contracts (2a, 2c). Commands are listed in `AGENTS.md:26-52`, the preflight table and `docs/repository-map.md` (3.5). A spec requirement already governs on-demand harness commands (3.2). |
| H15 | Moving the `AGENTS.md` row to "enforced by `task mutate:check`" | Conflicts with the table's definition; the owner has deferred it | `AGENTS.md:60-61`, the precedents at `:77` and `:93` (3.5). Through `task` only pass or fail survives (3.2; measured 201). The owner's ruling on the row waits for the trial (3.4). |
| H16 | Not in scope: generating mutants automatically | Matches the pin | The pin's page "does not select a mutation runner" and keeps experiments "not a new CI gate" (`:71-72`, `:174-175`). |

## 10. The orchestrator's claims, checked

1. "PR #48 and PR #73 change `AGENTS.md` and `docs/testing.md`": **true**. Also: both change
   `docs/repository-map.md`; #48 changes `docs/provenance.md`, the developer, reviewer and architect contracts and
   four contract tests; #73's `docs/testing.md` hunk is inside "Show that the test can fail" and #48's are not (3.1).
2. "Lesson 9" is unlocated: **confirmed** (H8).
3. The closest pattern is `scripts/lint-test-ports.sh` with its fixture test: **one of five instances** (5 (iii)).
4. The pin probe passed to the inventory review ("the only mutation-tooling path is `run_mutations.sh`"; the `.gz`
   logs are in the same proposal): **partly wrong, corrected here.** The pin has five evidence scripts (2g). The
   files `mutation-baseline.log.gz` and `mutation-restored.log.gz` are under
   `docs/proposals/rule-lifecycle-suite/evidence/`, not `graph-index-reference-model` (pin tree listing). Its
   conclusion, no admitted mutation tooling, stands (Q14).

## 11. Measurements

Go outcomes, measured in a scratch module outside the repository (`go1.26.6`; command
`go test -race -count=1 -cpu 1 -v -run '<pattern>' .`, plus `-timeout 3s` and then `-timeout 2s` for the hang). The
reviewer reproduced all six on go1.26.4:

| Case planted | Exit | What the output shows |
| --- | --- | --- |
| Test passes | 0 | `=== RUN   TestAdd`, `--- PASS: TestAdd`, `ok` |
| `-run` matches no test | **0** | `testing: warning: no tests to run`, `ok  example.invalid/m 1.190s [no tests to run]` |
| Assertion fails (wrong change) | 1 | `m_test.go:7: Add(2,3) = -1, want 5`, `--- FAIL: TestAdd` |
| Test blocks past `-timeout` | 1 | `panic: test timed out after 2s`, `running tests:`; **no `--- FAIL` line** (counted: 0) |
| Test panics (nil map write) | 1 | `--- FAIL: TestPanics` **and** `panic: assignment to entry in nil map` |
| Wrong change does not compile | 1 | `syntax error ...`, `FAIL example.invalid/m [build failed]` |

`task` exit status (scratch `Taskfile.yml` with commands `exit 1`, `exit 2`, `exit 3`; `task` 3.51.1):

| Command exits | `task <name>` | `task -x <name>` |
| --- | --- | --- |
| 1 | 201 | 1 |
| 2 | 201 | 2 |
| 3 | 201 | 3 |

Restore under a signal (scratch; `/bin/bash` 3.2.57; a script that backs up `f.go`, writes `mutant`, restores in
`trap restore EXIT`, has `INT` and `TERM` traps that `exit`, and runs `sleep 4` as the foreground child):

| Signal to the script's pid | Script exit | File afterwards | When the restore ran |
| --- | --- | --- | --- |
| SIGTERM | 143 | `original` | 4 s after the signal, when the child finished; the child was not signalled |
| SIGKILL | 137 | **`mutant`** | never |

`go test -overlay` (scratch; overlay replacing `o.go` with a mutant and `data.txt`, which a test reads with
`os.ReadFile`): `TestAdd` failed on the overlaid source (exit 1, `o_test.go:11: Add(2,3) = -1, want 5`); `TestData`
passed, reading the on-disk `data.txt`, not the overlay; the SHA-256 of both tree files was unchanged.

Other measurements: `shasum --help` (`-a, --algorithm   1 (default), ...`); `go help testflag` ("The default is 10
minutes (10m)."); `go help build` on go1.26.6 (the `-overlay` limits quoted in 2a); CI run times on `main` (3.2).

## 12. Open evidence questions

- **Q1.** How would a command identify "the intended assertion" (H9)? Prior art: the pin's scripts take a
  caller-named output fragment per mutant and reject a failure without it (2g); here, `requireViolation`
  (`repo_test.go:102-103`) and `cleanuproots_test.go:43` require named fragments in output.
- **Q2.** The pin answers it: an outer runner timeout alone is inconclusive, and "a bounded assertion that observes
  a required termination failure can establish detection" (`01-testing.md:216-218`). `docs/testing.md` dropped the
  second half (2b). Open: which text a command classifies against. Tests whose only bound is the binary `-timeout`
  (`probe_test.go:26-27`, 8 call sites) and PR #48's `synctest` deadlock panic, stack overflow and process kill are
  the cases where the two texts give different answers.
- **Q3.** The pin answers it: "equivalent" is a reviewer assessment, not an outcome, and it names "the applicable
  contract and input domain, its reasoning, and its reviewer" (`:220-222`). `docs/testing.md` has no such text (2b).
  Rate-based detections (2b; `.agents/protocol.md:130-131`) are named by neither page.
- **Q3b.** The pin's fourth outcome, Invalid ("could not build or was otherwise ineligible", `:215`), was dropped;
  `docs/testing.md` and #79 put a build error under Inconclusive.
- **Q4.** Integration-tagged tests cannot be run with plain `go test` (2g). Can the runner select one test? Its
  specified interface is packages only. Does a mutation run through it disturb `cover:check`'s `.evidence/last-run`?
- **Q5.** #79's command line sets no `-timeout`, so a hang costs go's default 10 minutes per run. What it costs a
  fixture test inside `task verify` is unmeasured, against CI's pinned 15-minute limit and today's ~3.3-minute runs.
- **Q6.** A fixture test needs real `go test` output (section 11). The contract package's comment "Tests here never
  invoke the real go or gh" (`repo_test.go:131`) is a comment in one package, not a rule. `pindiff`'s
  `TestCommandExitStatus` runs the real `go build` in `task test:unit` (5 (iii)). Unmeasured: how long a real
  `go test` fixture takes.
- **Q7.** Which checksum verifies the restore: none (`docs/testing.md`), SHA-1 (`shasum` in both contracts),
  SHA-256 (#79, PR #59, and the existing precedents `nowrite_test.go:71` and `docs/provenance.md:45`), or MD5 (the
  pin's scripts)? Four spellings of one fact (2c).
- **Q8.** Where the backup lives: the contracts use `${TMPDIR:-/tmp}`, and the pin's scripts `/private/tmp` or a copy
  of the tree. A backup inside the tree changes `tree-state.sh`'s fingerprint and is seen by the text guards (2c,
  3.2). Writing nothing at all (`-overlay`) has the limits in 2a.
- **Q9.** "Writes nothing inside the repository" is binding for `pindiff` (`spec.md:292`, `:359`) and stated in prose
  for every command but `task fmt` (`AGENTS.md:32`, preflight `SKILL.md:28`). #79's command writes and restores a
  source file. #60 meets the same question; neither change has answered it, and the two are unordered.
- **Q10.** Through `task`, only pass or fail survives (`spec.md:293-294`, `:358-359`; measured 201). A three-way verdict
  needs another channel than the exit status. Whether an on-demand command can move a row off "review only" given
  `AGENTS.md:60-61` and the precedents at `:77` and `:93` is the owner's, deferred until the trial (3.4).
- **Q11.** Ordering against PR #73 and PR #48 is ruled for the `AGENTS.md` row and the `docs/testing.md` section:
  after #48, then #73 (3.4). Not ruled: ordering against #60 (Q9) and #80, which also edits `docs/testing.md` and the
  rules table (3.4).
- **Q13.** No spec covers a mutation check (3.2). Whether the requirement extends `harness-boundaries` beside
  "Pin difference command", or lives elsewhere, is a design question; it is listed so the design states it.
- **Q14 (closed).** The pin has no mutation command, task or CI step: its `scripts/` (23 files) has none, and its
  `Taskfile.yml` and six workflow files have no line matching `mutat|mutant|gremlin|survivor|inconclusive`. The
  five mutation scripts at the pin are evidence inside an archived change or a proposal (2g), and its testing page
  "does not select a mutation runner" (`01-testing.md:72`). The command is new surface; no ledger row is owed.
- **Q15.** The "Lesson 9" quotation in #79 has no locatable source (H8).
- **Q16.** A run killed with SIGKILL leaves the mutant in the tree (2c). A SIGTERM to a bash script waits for its
  foreground `go test` before any trap runs. A script started as an `&` job cannot trap SIGINT. Unmeasured: how
  often other readers (6, Readers) overlap a run in one worktree.

## 13. Searches that closed a category or claim empty

- `git grep -n -e 'mutate:check' -e gremlins -e go-mutesting -e mutesting -- . ':!openspec/changes/archive'`: 0.
- `git grep -n -i -E 'mutat|mutant|survivor|inconclusive' -- . ':!openspec/changes/archive'`: hits are the
  passages pinned in 2a-2f, git-sense uses of "mutate", the integration runner's container "survivors"
  (`scripts/test-integration.sh:480-519`, `runner_test.go:29-612`, `integration-test-runner/spec.md:64`), and
  domain uses of "mutation" in `docs/setup-plan.md` and `docs/inventory-scope.md`. None is a command.
- `git grep -n -i -E 'write nothing|writes nothing' -- . ':!openspec/changes/archive'`: `harness-boundaries`
  `spec.md:292`, `:359`; `pindiff/nowrite_test.go:14`; preflight `SKILL.md:29` (`task fmt:check`). Revision 1 did
  not run this search, which is how it missed B1.
- `git grep -n -E '\btrap\b|signal\.Notify|SIGINT|SIGTERM|SIGKILL|bash 3' -- scripts internal`: the homes pinned in
  2c and 5 (ii).
- Revision 1's search for tests that run the real `go`, `exec\.Command(Context)?\((ctx, )?"(go|...)"`, missed
  `exec.CommandContext(t.Context(), "go", ...)` at `pindiff/command_test.go:17`, because the pattern allowed only a
  first argument spelled `ctx`. Corrected in 5 (iii) and Q6.
- `git grep -n -i -E 'shasum|sha256sum|sha-256|backup' -- . ':!openspec/changes/archive'`: the two contracts,
  `docs/provenance.md:45`, `scripts/tree-state.sh:14`, `scripts/verify.sh:18`. It cannot match `sha256.Sum256`
  (`nowrite_test.go:71`), found through B1.
- `git grep -n -e 'test timed out' -e 'no tests to run' -- .`: 0. No file in the tree interprets these `go test`
  outputs.
- `gopls workspace_symbol -matcher=fuzzy` for `Mutant`, `Survivor`, `Inconclusive`, `Detection`: no module symbol;
  `Snapshot`: `pindiff/nowrite_test.go:55` (`snapshot`).
- `ls docs/adr`: no such directory. `ls openspec/changes`: `archive` only.
- Pin: `gh api 'repos/C360Studio/semstreams/git/trees/8b99efe9c66a4faa4fa509f9f62cc6bad8392128?recursive=1'`,
  filtered with `grep -i -E 'mutat|mutant|gremlin|mutesting'`: domain code, archived specs, and the evidence files of
  2g; nothing under `scripts/`, `Taskfile.yml` or `.github/`.
- `gh search issues --repo C360Studio/semengine --include-prs` for "reported by the implementer", "thin results",
  "implementer-reported": no source for #79's quotation beyond #79 and the PR texts already cited.

## 14. Review r1 findings and where each is answered

| Finding | Answered in |
| --- | --- |
| B1 `harness-boundaries` "Pin difference command" and "Carried entries match the pin" | 2c (`nowrite_test.go`), 3.2 (rewritten), 3.5, 5 (i) and (ii), 11 (`task` exit status), Q9, Q10, 13 |
| HIGH issue #60 | 3.4, 3.5, 5 (adoption sweep), 6 (Writers), Q9, Q11 |
| HIGH restore on interrupt or kill | 2c (homes and measurement), 5 (ii), 6 (run; premise withdrawn), 7, 11, Q16 |
| HIGH Q14 resolvable; probe partly wrong | header, 2g (five scripts), 3.3, 4, 10 item 4, Q14 (closed), 13 |
| MEDIUM pin testing page answers Q2, Q3 | header (owner ruling on #22), 2a, 2b (carry table), Q2, Q3, Q3b |
| MEDIUM `pindiff/command_test.go` against Q6 | 5 (iii), Q6, 13 |
| MEDIUM `docs/provenance.md:40-54` | 2c, 2f, 5 (iv), Q7 |
| MEDIUM section 7 cites precedents that ran it | 7 (run) |
| NIT missing spellings | 2a (`docs/testing.md:131-132`, developer `:197-199`), 2b (preflight `:84-86`, protocol `:130-131`), 2d, 2e (`docs/provenance.md:33-36`) |
| NIT comment 5954727965's nil-pointer panic | 2b (with 5957969940 and 5972450374's stack overflow) |
| Coordinator item 8: PR #59 `-overlay` | 2a, 4, 5 (ii), 6 (Owners, Recovery), 11, Q8 |
