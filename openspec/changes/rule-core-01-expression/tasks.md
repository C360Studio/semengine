# Tasks: rule-core-01-expression

## 1. Design

- [x] 1.1 The inventory review passes and the design review passes; the owner fills `Approved-by:`.

## 2. Port

- [x] 2.1 Copy the pin's `processor/rule/expression` (8 files, 3,527 lines) to `processor/rule/expression`; rewrite
  imports (module path; `internal/harness/semantictest` for `evaluator_test.go:11`); drop `Version` at
  `evaluator_test.go:608` and `:1167`. Add the ledger row `processor/rule/expression` (`adapt`). Check: `task
  ledger:check`.
- [x] 2.2 Write the tests for D2 to D4 before the code. They are failing-first because the package does not build
  without `Compile`; after 2.3, run `task mutate:check` on each refusal and record each result as detected or
  survived. Cases: a compiled literal matches as `regexp.MatchString` does; a slice is compiled and each element
  evaluated; an uncompiled regex condition, and one whose value was edited after `Compile`, return `*EvaluationError`;
  `Compile` refuses a non-string value, `[unclosed`, a pattern over 500 characters, and each of the
  `SubstitutionPrefixes()` (`$now`, `$entity.id`, `^x-$state.iteration`, ...), naming index and field; it accepts
  `[$]now`, `[$a-z]+`, `^\$USD`, an end-anchored pattern and semteams' list pattern; a second `Compile` changes
  nothing; every refusal is `errs.IsInvalid`.
- [x] 2.3 Implement D2 to D4: remove the global cache, `init` and the `internal/cache` import (`regex_cache.go:13-30`,
  `:122-137`); add `Compile` over a slice and `SubstitutionPrefixes()`; the regex operator uses the kept pattern;
  replace `validateRegexComplexity` with the 500-character limit only (D4). Check: 2.2 passes; `git grep -n 'func
  init\|internal/cache' processor/rule/expression` is empty.
- [x] 2.4 Drop the dead surface (D5): `HasTriple`, `GetOutgoing`, `Distance` and helpers (`evaluator.go:840-965`) and
  their tests (`evaluator_test.go:429-697`); the `TypeDetector` interface, `FieldType`, `DetectFieldType`; unexport
  `OperatorFunc`. Zero-use features stay (D6). Check: `task build`.
- [x] 2.5 Empty logic means AND (D7, approved): `evaluator.go:151`, with a test of two conditions, one false.
- [x] 2.6 Rewrite the carried tests to the harness rules: drop the cache tests and `TestRegexCache_NoHang`
  (`regex_cache_test.go:207-331`); add tests for `EnsureStatelessResolvable`, numeric strings, NaN and `json.Number`,
  and the spec's resolution exceptions (list operators, required `transition`); add one rapid property test for the
  design's invariants, recording the seed and completed checks. Replace port-history comments (`#147`, `gh#207`,
  `gh#731`, "Phase 1") with plain reasons. Check: `task test:unit`, `task test:repeat`.
- [x] 2.7 Add `"processor/rule/expression unit"` to `scripts/cover-check.sh`. Check: `task cover:check` at 80% or
  more.

## 3. Docs

- [x] 3.1 Port ADR-041 to `docs/adr/041-unified-condition-evaluator.md` with a port note (D8) and its ledger row.
- [x] 3.2 Adapt `processor/rule/docs/conditions.md` to the code (D8), including the token refusal, `[$]`, the
  500-character limit, and `transition`'s three limits (D6), each checked against the code first (the batching limit
  is marked unverified until PR 10), with its ledger row. Check: `task docs:check`.
- [x] 3.3 In `docs/repository-map.md`: add `processor/rule/expression/`, update the cover-check count, and add
  `rule-conditions` to the spec list (`:59`).

## 4. Land

- [ ] 4.0 One comment on epic #9 recording what later PRs owe this design: PR 2's `Load` compiles once and refuses
  naming rule and file; PR 3's substituter reads `SubstitutionPrefixes()`; PRs 6 and 7 log evaluation errors at Warn
  or above with the rule ID; PR 7's design weighs a general previous-value field for the owner; PR 10 confirms the
  watcher's batching against `transition`'s guide text. Read back after posting.
- [ ] 4.1 `task verify` passes on a clean tree at the PR head; the PR body records `implemented-by:` and
  `reviewed-by:`; it closes no issue.
- [ ] 4.2 Sync `specs/rule-conditions/spec.md` to `openspec/specs/rule-conditions/spec.md`, checked against the code,
  and archive the change as the last content commit. Check: `task spec:check`.
