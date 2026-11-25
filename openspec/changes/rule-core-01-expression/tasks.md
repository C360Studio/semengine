# Tasks: rule-core-01-expression

## 1. Design

- [ ] 1.1 The inventory review passes and the design review passes; the owner fills `Approved-by:`.
  Hold: OPEN-Q proposal awaits owner approval

## 2. Port

- [ ] 2.1 Copy the pin's `processor/rule/expression` (8 files, 3,527 lines) to `processor/rule/expression`; rewrite
  imports (module path; `internal/harness/semantictest` for `evaluator_test.go:11`); drop `Version` at
  `evaluator_test.go:608` and `:1167`. Add the ledger row `processor/rule/expression` (`adapt`). Check:
  `task ledger:check`.
- [ ] 2.2 Write the failing tests for D2 and D3 first: a compiled literal pattern matches as `regexp.MatchString`
  does; an uncompiled regex condition returns `*EvaluationError` on each evaluation; `Compile` refuses a non-string
  value, `[unclosed`, a pattern over each limit, `$entity.id`, `^x-$now`, `\$now`, naming the field; it accepts
  `[$]now`, `\x24now`, an end-anchored pattern, and semteams' list pattern; every refusal is `errs.IsInvalid`.
  Show each test fails on the pin code.
- [ ] 2.3 Implement D2 to D4: remove the global cache, `init` and the `internal/cache` import (`regex_cache.go:13-30`,
  `:122-137`); add `Compile`; the regex operator uses the kept pattern; keep `validateRegexComplexity` and reword
  its comment (linear-time regexp). Check: 2.2 passes; `git grep -n 'func init\|internal/cache'
  processor/rule/expression` is empty.
- [ ] 2.4 Drop the dead surface (D5): `HasTriple`, `GetOutgoing`, `Distance` and helpers (`evaluator.go:840-965`)
  and their tests (`evaluator_test.go:429-697`); `TypeDetector`, `FieldType`, `DetectFieldType`; unexport
  `OperatorFunc`. Apply whatever the owner decided on zero-use features (D6). Check: `task build`.
- [ ] 2.5 Only if the owner chose D7: empty logic means AND (`evaluator.go:151`), with a test of two conditions, one
  false. Otherwise tick with "not chosen".
- [ ] 2.6 Rewrite the carried tests to the harness rules: drop the cache tests and `TestRegexCache_NoHang`
  (`regex_cache_test.go:207-331`); add tests for `EnsureStatelessResolvable`, numeric strings, NaN and `json.Number`;
  add one rapid property test for the design's invariants, recording the seed and completed checks. Replace the
  port-history comments (`#147`, `gh#207`, `gh#731`, "Phase 1") with plain reasons. Check: `task test:unit`,
  `task test:repeat`.
- [ ] 2.7 Add `"processor/rule/expression unit"` to `scripts/cover-check.sh`. Check: `task cover:check` shows it
  at 80% or more.

## 3. Docs

- [ ] 3.1 Port ADR-041 to `docs/adr/041-unified-condition-evaluator.md` with a port note (D8) and its ledger row.
- [ ] 3.2 Adapt `processor/rule/docs/conditions.md` to the code (D8) with its ledger row. Check: `task docs:check`.
- [ ] 3.3 Add `processor/rule/expression/` to `docs/repository-map.md` and update the cover-check count.

## 4. Land

- [ ] 4.1 `task verify` passes on a clean tree at the PR head; the PR body records `implemented-by:` and the
  reviewer's `reviewed-by:`; it closes no issue.
- [ ] 4.2 Sync `specs/rule-conditions/spec.md` to `openspec/specs/rule-conditions/spec.md`, checked against the code,
  and archive the change as the last content commit. Check: `task spec:check`.
