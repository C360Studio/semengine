# Tasks: authority-one-spelling

Each task names the outcome and the gate that proves it. An unchecked task whose first line says `Hold:` is one
`task spec:queue` reports as blocked until what it names exists. "This pull request" is the draft PR that claims
`claude/authority-one-spelling`; evidence is recorded there as a comment. Every outcome below is reached on the
branch, in or before the archive commit. (D) is the developer, (W) the technical writer.

"Written first" means the test is run and seen to fail for the stated reason before the code that makes it pass,
and that output is recorded on this pull request. The two checks' expected failure lines are written out in their
sensitivity tests, from the scenarios of the `harness-boundaries` delta.

What follows the archive is not a task: the check of the archive, the re-review of the archive commit if any,
marking the pull request ready, the CI run that starts, `task merge:check -- <n>` and the merge are recorded on this
pull request. This
pull request closes #72 (`Closes #72` in its body).

## 1. Design

- [x] 1.1 The independent review of `inventory.md`: `INVENTORY PASS` in round 2 of at most three (issue #72; the
      round-2 review's findings B1–B4 and F5 are folded into the file carried here).
- [x] 1.2 The independent pre-owner review of `design.md`, `proposal.md`, this file and the `harness-boundaries`
      delta: `DESIGN REVIEW PASS` in at most three rounds, recorded on this pull request with the reviewed and the
      committed checksums. PASS at `f67f835` in round 3 (record: PR #73 comment 5969786253).
- [x] 1.3 The owner's acceptance of the reviewed design, recorded on this pull request (2026-10-03, after the
      review record at `f67f835`). Q1 (the carrier
      exception) is already ruled, form (ii) by exact field (#72 comment 5969757891), and the delta, D3 and the
      fixture of 2.1 carry it. The one consequence `design.md` records for change 2 (`HierarchyConfig` meets C-2)
      is relayed to epic #9 as a note, not a ruling.

## 2. C-2: the field check

- [x] 2.1 (D) `TestNoSecondAuthorityFieldSensitivity`, written first, in a new
      `internal/harness/contract/authority_test.go`: the fixture module of `design.md` D3 (the pin's
      `HierarchyConfig` and `EntityParts` shapes, an anonymous struct, an internal and a `main` package; and, to pass,
      the three owner packages, `types.PlatformMeta`, a `Platform types.PlatformMeta` carrier through the
      `component.PlatformMeta` alias, `processor/rule.CallerContext.Org` beside a failing second type in that
      package, lowercase `org`/`platform`, and a `_test.go` file). It requires each failing field by file, line and
      struct type, silence for every passing one, and the exact count. Seen to fail first because the check does not
      exist. Gate: `task test:unit`.
- [x] 2.2 (D) `TestNoSecondAuthorityField` over the repository, on the `contextViolations` walk
      (`context_test.go:221-289`) with D3's predicate and exceptions held in one map mirroring the requirement's
      list by exact string (D4). Its failure line is D3's. 2.1 passes; the repository test passes on `main` (design
      P1). Gate: `task test:unit`.
- [x] 2.3 (D) Shown able to fail (`docs/testing.md`, "Show that the test can fail"): each wrong change in turn —
      drop the exported-only test (lowercase `org` then fails: the sensitivity test must catch the false positive);
      drop one exact-field exception (`component.Dependencies.Platform` then fails); except by type instead of by
      field (the fixture's second `Platform types.PlatformMeta` field then passes and the sensitivity test must
      catch it); drop the `CallerContext.Org` exception; match `Org` only; skip anonymous structs; skip `main`
      packages — with the baseline, wrong-change and restored runs recorded
      on this pull request. A wrong change the sensitivity test lets through is a survivor, closed with a case or
      listed under what is not covered. Recorded: PR #73 comment 5969839223 (six listed changes detected, and the
      case-insensitive variant of the first; dropping the exported-only test alone is an equivalent survivor, not
      covered). Two further wrong changes from the review at `beb3c3e` (exceptions matched without the package;
      the whole `types` package excepted) survived the first fixture and are detected after `81ae1ca`: PR #73
      comment recorded with that commit.
- [x] 2.4 (D) The `contract` package's time in the `test:unit`, `test:integration` and `test:repeat` steps on
      `main` and on the branch, from the CI logs of two runs, recorded on this pull request and under "Declared
      costs" in `design.md`; over D6's 15 s budget, the two checks share one load before anything is removed.

## 3. C-1: the name check

- [x] 3.1 (D) Adopted check verified at #48's merged head `deaafd4` against D2 and the delta's name scenarios; at
      `441066e`, `signatures_test.go:408` (`Federation|GlobalID|EntityIRI`), `:621-719` and `:725-760` (every
      module package, file:line lines; inherited interface methods, Codex F3, `30b9e5f`; members gained by
      embedding, `fe3eddc`). The fixture gained `Confederation`,
      comment and literal non-matches (`1b80a11`), a package count (`7084f9e`), the rule name as a literal
      (`28c9ffa`), and both checks fail on zero packages, scenario "This module passes" (`29fbd9b`, each
      sensitivity test's empty-module case).
      Gate: `go test -count=1 -race -run 'TestNoDeploymentAuthorityNames' -v ./internal/harness/contract/` passes.
- [x] 3.2 (D) Shown able to fail: D2's five wrong changes (internal scope, methods, fields, case-insensitive match,
      test files) each fail the sensitivity test after `1b80a11` (the case-insensitive one survived #48's fixture);
      baseline, wrong-change and restored runs, with checksums, in the PR #73 comment recording `1b80a11`; the two
      F3 wrong changes (inherited methods dropped; in-module rule dropped) fail it at `30b9e5f`, and the embedding
      wrong changes (struct promotion dropped; in-module rule for promoted members dropped; value method set for
      pointer; promoted fields dropped) fail it at `fe3eddc`, and the field-selection filter removed and a single
      pass fail it at `441066e` (the single pass by `Wrapper`, not the cross-package case), recorded likewise.

## 4. Documents

- [x] 4.1 (W) `AGENTS.md`: the rule's row as `design.md` D7 words it, naming both tests under "Enforced by" and what
      stays review only. Gate: `task docs:check`.
- [x] 4.2 (W) `docs/testing.md:173-181` names both sensitivity pairs (`f2957e2`); `docs/repository-map.md:40`
      names the deployment-authority spelling. Gate: `task docs:check` passes.

## 5. Review

- [x] 5.1 Codex `Review record` APPROVE at `fd02b82` (PR #73 comment 6019262509; F5 MEDIUM nonblocking filed as #87),
      after CHANGES REQUESTED at `83f4fdb` (comment 6018394100, F3/F4 fixed in `83f4fdb..fd02b82`). Codex's
      implementation review of this pull request (a
      code pull request: Go files under
      `internal/harness/contract/`), recorded on it as `Review record` with the commit it read; a later content
      commit gets its re-review. A disagreement goes to the owner.
- [x] 5.2 Written once comment 6019262509 approved. The description has one
      line `implemented-by:` naming `claude` (with the model) and one line
      `reviewed-by:` naming `codex`, written once Codex's newest review record approves.

## 6. In the archive commit

- [x] 6.1 The Purpose of `openspec/specs/harness-boundaries/spec.md` says the capability also holds the
      machine-checked shape rules over ported production code, in the same commit as
      `openspec archive authority-one-spelling` and the spec sync. Gate: `task spec:check` on that commit.
