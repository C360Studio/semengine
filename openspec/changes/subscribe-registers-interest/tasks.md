# Tasks: subscribe-registers-interest

Each task names the outcome and the gate that proves it. "This pull request" is PR #156; evidence is recorded there as
a comment. Every outcome below is reached on the branch, in or before the archive commit. (D) is the developer, (W)
the technical writer.

What follows the archive is not a task: the check of the archive, marking the pull request ready, the CI run that
starts, `task merge:check -- 156` and the merge are recorded on this pull request.

## 1. Design

- [x] 1.1 The independent review of `inventory.md`: `INVENTORY PASS`, with its corrections folded in, recorded on this
      pull request with the file's checksum.
- [x] 1.2 The independent pre-owner review of `design.md` revision 3, `proposal.md`, this file and the
      `transport-client` delta: `DESIGN REVIEW PASS`, recorded on this pull request.
- [x] 1.3 The owner's acceptance of the design, revision 3, recorded on #144 or this pull request.
- [x] 1.4 The independent pre-owner review of `design.md` revision 4 (D7, L6, L7, "Files", "Overlaps and order",
      "Owner decisions") and of this file's revision 4 changes: `DESIGN REVIEW PASS`, recorded on this pull request.
- [ ] 1.5 The owner's answers to O1 and O2 (`design.md`, "Owner decisions"), recorded on #144 or this pull request.

## 2. The fix and its tests

- [ ] 2.1 (D) T1 to T8 of `design.md`, D7 (revision 4), written first and run on the base code: T1 fails with "no
      responders" for both calls, and T2 to T8 fail because the call returns a subscription and no error (T4: the
      server reads a `SUB`). Run together in one `go test` run, T2 to T8 each report that failure, and the run ends
      with those failures, not with a `fatal error` or a time-out. The first run (PR #156 comment 6086018979) settled
      T1's miss count and the dial seam. Gate: the run's output recorded on this pull request with the system it ran
      on. Hold: the owner's acceptance of revision 4 (task 1.5).
- [ ] 2.2 (D) The round trip in natsclient's core subscribe, as `design.md`, D1 to D5 state, and the doc comments of
      `Subscribe` and `SubscribeForRequests` saying that the subscription is registered with the server when the call
      returns, what bounds the wait, which errors a failed round trip returns, and that a failed call does not wait
      for a running handler of it, which Close joins. Gate: `task test:unit` and `task test:repeat`, T1 to T8 passing;
      and `go test -race -cpu=1 -count=5 -shuffle=on -v ./natsclient/` run 20 times, all 20 passing, with each run's
      `-test.shuffle` seed recorded on this pull request (`go test` prints a passing package's seed only with `-v`).
      A run that fails is recorded with its seed and output, not replaced by a rerun.
- [ ] 2.3 (D) Shown able to fail: M1 to M10 of `design.md`, D7, one at a time with `task mutate:check`, each report
      pasted on this pull request with the system it ran on, beside the record of `docs/testing.md`, "What the pull
      request records"; and, by hand (`docs/testing.md`, "Run it by hand"), the helper's two refusals in D7: called
      from a subtest, and given a pattern that selects no test, it fails the test. Gate: detection for each by the
      assertion D7 names, a survivor or an inconclusive run reported as such; for each refusal, the command and the
      helper's failure line, pasted on this pull request.
- [ ] 2.4 (D) natsclient's row in `docs/admission-ledger.yaml`: adapt item `(13) natsclient-subscribe-registers-interest`
      at the end of `contract` (what changed, the pin's `natsclient/client.go:834-849` and `request.go:359`, #144 and
      the ruling), and T1 to T8 with the M1 to M10 record at the end of `proving_tests`. Gate: `task ledger:check`.
- [ ] 2.5 (D) The #144 reproduction ("Known flakes" in `.agents/protocol.md`). In a detached worktree at PR #93's head,
      run the #144 command from #144's reproduction comment with `-count=300 -cpu=1`; then merge this pull request's
      head into that worktree, without pushing, and run it again. Gate: both counts of
      `TestGraphIngestServesExactlyTheDeclaredVerbs` and `TestGraphIngestProvisionsNoSuffixIndex` failures, the command,
      and the two input SHAs (PR #93's head and this pull request's head; the local merge's SHA exists on one machine
      only), recorded on this pull request and on #144, with the logs named as local only; the second count is 0 of
      300 for each test. A failure in the second run sends the design back to review.
- [ ] 2.6 (D) `task verify` passes on the commit that carries 2.2 and 2.4, and that commit's CI run passes. Gate: CI
      `Required` on that commit, its run link recorded on this pull request.
- [ ] 2.7 `design.md`, "Conformance", maps each decision to the `file:line` that carries it out at the commit of 2.6,
      and to the test or record that shows it. Gate: every row filled, in a commit that changes no other file.
- [ ] 2.8 (W) `docs/testing.md`, "How reach is judged" (`:295-296`), no longer says that a child process the test
      starts writes no coverage: one given `-test.gocoverdir`, as D7's helper is, writes it, and so does a `prochost`
      helper, through the `GOCOVERDIR` it inherits. Gate: `task docs:check`.

## 3. Review

- [ ] 3.1 Codex's implementation review of this pull request, recorded on it, naming the commit it read. Each finding
      is fixed or answered before 4.1.
- [ ] 3.2 The description has one line `implemented-by:` that names `claude` and one line `reviewed-by:` that names
      `codex`, written once Codex's newest review record approves.

## 4. In the archive commit

- [ ] 4.1 (W) In the same commit as `openspec archive subscribe-registers-interest` and the spec sync: the
      `transport-client` Purpose names the new guarantee; `natsclient/README.md` says, beside the Subscribe example
      (`:85-86`), that a subscription is registered with the server when `Subscribe` returns; `docs/repository-map.md`
      gains a row for the archived change and its specs row says `transport-client` is amended by it. Gate:
      `task spec:check` and `task docs:check` on that commit.
