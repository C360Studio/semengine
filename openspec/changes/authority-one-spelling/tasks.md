# Tasks: authority-one-spelling

Each task names the outcome and the gate that proves it. An unchecked task whose first line says `Hold:` is one
`task spec:queue` reports as blocked until what it names exists. "This pull request" is the draft PR that claims
`claude/authority-one-spelling`; evidence is recorded there as a comment. Every outcome below is reached on the
branch, in or before the archive commit. (D) is the developer, (W) the technical writer.

"Written first" means the test is run and seen to fail for the stated reason before the code that makes it pass,
and that output is recorded on this pull request. The two checks' expected failure lines are written out in their
sensitivity tests, from the scenarios of the `harness-boundaries` delta.

What follows the archive is not a task: the check of the archive, marking the pull request ready, the CI run that
starts, Codex's review record, `task merge:check -- <n>` and the merge are recorded on this pull request. This
pull request closes #72 (`Closes #72` in its body).

## 1. Design

- [x] 1.1 The independent review of `inventory.md`: `INVENTORY PASS` in round 2 of at most three (issue #72; the
      round-2 review's findings B1–B4 and F5 are folded into the file carried here).
- [ ] 1.2 The independent pre-owner review of `design.md`, `proposal.md`, this file and the `harness-boundaries`
      delta: `DESIGN REVIEW PASS` in at most three rounds, recorded on this pull request with the reviewed and the
      committed checksums.
- [ ] 1.3 The owner's acceptance of the reviewed design, recorded on #72 or on this pull request. The one
      consequence `design.md` records for change 2 (`HierarchyConfig` meets C-2) is relayed to epic #9 as a note,
      not a ruling.

## 2. C-2: the field check

- [ ] 2.1 (D) `TestNoSecondAuthorityFieldSensitivity`, written first, in a new
      `internal/harness/contract/authority_test.go`: the fixture module of `design.md` D3 (the pin's
      `HierarchyConfig` and `EntityParts` shapes, an anonymous struct, an internal and a `main` package; and, to pass,
      the three owner packages, `types.PlatformMeta`, a `Platform types.PlatformMeta` carrier through the
      `component.PlatformMeta` alias, `processor/rule.CallerContext.Org` beside a failing second type in that
      package, lowercase `org`/`platform`, and a `_test.go` file). It requires each failing field by file, line and
      struct type, silence for every passing one, and the exact count. Seen to fail first because the check does not
      exist. Gate: `task test:unit`.
- [ ] 2.2 (D) `TestNoSecondAuthorityField` over the repository, on the `contextViolations` walk
      (`context_test.go:221-289`) with D3's predicate and exceptions held in one map mirroring the requirement's
      list by exact string (D4). Its failure line is D3's. 2.1 passes; the repository test passes on `main` (design
      P1). Gate: `task test:unit`.
- [ ] 2.3 (D) Shown able to fail (`docs/testing.md`, "Show that the test can fail"): each wrong change in turn —
      drop the exported-only test (lowercase `org` then fails: the sensitivity test must catch the false positive);
      drop the carrier-type exception (the alias case fails); drop the `CallerContext.Org` exception; match `Org`
      only; skip anonymous structs; skip `main` packages — with the baseline, wrong-change and restored runs recorded
      on this pull request. A wrong change the sensitivity test lets through is a survivor, closed with a case or
      listed under what is not covered.
- [ ] 2.4 (D) The `contract` package's time in the `test:unit`, `test:integration` and `test:repeat` steps on
      `main` and on the branch, from the CI logs of two runs, recorded on this pull request and under "Declared
      costs" in `design.md`; over D6's 15 s budget, the two checks share one load before anything is removed.

## 3. C-1: the name check

- [ ] 3.1 Hold: until PR #48 merges — it carries `signatures_test.go`, and at its pushed head `c64ac338` the
      check names seven identifiers #48 is ruled to remove (design P2). (D)
      `TestNoSecondAuthorityNameSensitivity`, written first, in `authority_test.go`: the fixture of
      `design.md` D2 (the seven matching names across a public, an internal and a `main` package; the four
      non-matches: an unexported `federationMeta`, lowercase `federation`, a `_test.go` `TestFederation`, a comment
      and a string literal saying `BuildGlobalID`). Seen to fail first. Then `TestNoSecondAuthorityName` over the
      repository, as a second predicate over the public-signature loader (`signatures_test.go:212-258`): every
      exported object of every loaded module package — package scope, methods of named types, struct fields — whose
      name contains `Federation`, `GlobalID` or `EntityIRI`, with D2's failure line. Gate: `task test:unit`.
- [ ] 3.2 (D) Shown able to fail, as 2.3: drop the internal-package scope; drop methods; drop struct fields; match
      `federation` case-insensitively (the lowercase case must catch it); check test files. Recorded on this pull
      request.

## 4. Documents

- [ ] 4.1 (W) `AGENTS.md`: the rule's row as `design.md` D7 words it, naming both tests under "Enforced by" and what
      stays review only. Gate: `task docs:check`.
- [ ] 4.2 (W) `docs/testing.md`, the list at `:155-161`: the two sensitivity pairs; `docs/repository-map.md:41`: the
      contract package's clause names the deployment-authority spelling. Gate: `task docs:check`.

## 5. Review

- [ ] 5.1 Codex's implementation review of this pull request (a code pull request: Go files under
      `internal/harness/contract/`), recorded on it as `Review record` with the commit it read; a later content
      commit gets its re-review. A disagreement goes to the owner.
- [ ] 5.2 The description has one line `implemented-by:` naming `claude` (with the model) and one line
      `reviewed-by:` naming `codex`, written once Codex's newest review record approves.

## 6. In the archive commit

- [ ] 6.1 The Purpose of `openspec/specs/harness-boundaries/spec.md` says the capability also holds the
      machine-checked shape rules over ported production code, in the same commit as
      `openspec archive authority-one-spelling` and the spec sync. Gate: `task spec:check` on that commit.
