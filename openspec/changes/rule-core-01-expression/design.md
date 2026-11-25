# Design: rule-core-01-expression

Base `245c59c4`. Pin `8b99efe9`. Inventory: `inventory.md` (awaiting INVENTORY PASS). Plan: PR 1 of
`archive/2026-11-25-design-rule-core-split-plan`. Binding: owner regex ruling (PR #752 comment 7162536253), shape
review items 5 and 9 (7162146299). Ledger row: `processor/rule/expression`, disposition `adapt`.

## Context

The package is the one condition evaluator for rule conditions and action `when` guards (ADR-041). It is 1,417
production and 2,110 test lines at the pin, has no context, goroutine or configuration, and builds on main after three
fixes (inventory, "Compile drift"). After this design it is about 1,235 production and 2,015 test lines
(estimates). One PR, one package, close to the 3,000-line guide. No issue is retired (none names the package
but #9, an epic).

## Decisions

**D1. Destination `processor/rule/expression`.** It keeps the pin's path, the rule core lands beside it in PR 2, and
semteams already expects it (`scripts/measure-semengine-port-ledger.go:226`). Other options: a top-level
`rule/expression` breaks the path mapping and gains nothing; `processor/rule/internal/expression` cannot work,
because semteams tests and the rule definition's exported fields name its types.

**D2. A regex pattern is compiled once, when its condition is loaded (owner ruling).** Options:

- **A (recommended): `ConditionExpression.Compile() error`, kept on the condition.** It refuses a non-string
  pattern, a templated pattern (D3), a pattern over the limits, or one `regexp` rejects, and otherwise stores the
  compiled pattern on the condition. Running it twice does no harm, and on any other operator it does nothing.
  Evaluating a regex condition that was never compiled returns `*EvaluationError` ("regex condition not compiled")
  every time; evaluation never compiles. The global cache, `init` and the `internal/cache` import go. Cost: one
  exported method. A copied condition keeps the compiled pattern, so the pin's substitution copy
  (`execution_context.go:421-423`) preserves it. A JSON round trip after `Compile` loses it, and that fails loudly.
- B: `Compile(expr) (*Compiled, error)`, and the evaluator takes only compiled expressions. Forgetting to compile
  then fails at compile time, but the evaluator's three callers and the substitution layer (PRs 3, 6, 7) change
  shape, and substitution must rewrap on every evaluation.
- C: keep a cache, or compile on first use: refused by the ruling.

**D3. "Templated" means a `$` followed by an ASCII letter anywhere in the pattern.** The pin substitutes any string
holding `$` and replaces tokens mid-string (`execution_context.go:424-428`, `:288-322`); every token starts with `$`
and a letter (`:35`). A Go regex cannot match `$` followed by a letter (probe), so no working pattern is refused
except a literal dollar before a letter (`\$now`), which the pin would have rewritten. The error says to write
`[$]` or `\x24`. Options not taken: "starts with `$`" (the addendum's reading) misses mid-pattern templates;
"contains `$`" refuses every end-anchored pattern, including semteams' one regex. The rule spells the token shape a
second time, broader than the substituter's grammar, and does not depend on PR 3's namespaces (accepted cost).

**D4. Pattern limits are carried as authoring limits.** `validateRegexComplexity` (length 500, 20 groups, depth 5,
the fragment list, repeats of 1000 and up) runs inside `Compile`, unchanged. Its comment loses the ReDoS claim:
Go's regexp is linear time (`go doc regexp`). Dropping it is optional, so it is not proposed.

**D5. Dead surface is dropped (surface audit (a); shape items 3, 4).** `HasTriple`, `GetOutgoing`, `Distance` and their
helpers; `TypeDetector`, `FieldType`, `DetectFieldType` become two unexported lookup functions; `OperatorFunc` is
unexported. No reader in the pin or in any consumer (inventory).

**D6. Zero-use features are carried (item 9; owner bullets).** Twelve operators, `or`, `from`, `$prev.*`,
`$entity.lifecycle.*` and deeper `$message.a.b` paths have no product user. Each has a code reader and a test, the
package is admitted whole, and dropping `transition` would also drop the previous-value tracking in PR 7.

**D7 (optional, owner's choice). Empty logic means AND.** Today `""` means OR (`evaluator.go:151`). The rule layer
spells AND in four places and in both docs, no production caller passes `""`, and the pin's unit tests pass with
the change (probe). Cost: one line and one test. Later PRs may then delete their own defaults.

**D8. Docs and ADR.** ADR-041 is ported to `docs/adr/` with a port note: deny/governance examples not carried; the
`$entity.lifecycle.*` step was added later (ADR-047, PR 5); issue numbers are SemStreams'.
`processor/rule/docs/conditions.md` is adapted to the code: a missing required field is an error; no operator is
gated by type; operators compare as strings without error; `length_gte`/`length_lte`; regex rules and limits; no
port history. `syntax.md` describes rule definitions and waits for PR 6.

## Seam for PR 2

PR 2's loader calls `Compile` on every rule condition and every action `when` condition, on the definition it keeps,
after its last decode. It wraps the error with the rule ID, the file and the position (`conditions[2]`,
`on_enter[1].when[0]`); the error is classified invalid (`errs.IsInvalid`) and names the field and the reason. Cron
rules (PR 11) and hot reload (PR 12) load through the same path. Decoding strictness for unknown condition keys is
PR 2's.

## Invariants (spec: `specs/rule-conditions/spec.md`)

- `and` is all of the per-condition results, `or` any of them, and an empty list passes ("Conditions combine").
- `not_in` is the negation of `in`; `between [lo, hi]` is `gte lo` and `lte hi` ("Operators").
- A compiled regex condition's result equals `regexp.MatchString` on the field's string form; evaluation never
  compiles ("A regex pattern is compiled when its condition is loaded").

Generated checks: one rapid property test for these, with the seed and completed checks recorded. Examples cover
the field-resolution order.

## Tests

The carried tests keep their oracles (table values written by hand). Dropped: the dead-surface tests
(`evaluator_test.go:429-697`), the cache tests (eviction, concurrency, statistics, the cache benchmark), and
`TestRegexCache_NoHang`, which sets its own 100 ms deadline. Fixed: the `semantictest` path and the two
`Version` lines. Added: `Compile` refusals and acceptance (including `[$]`, `\x24` and semteams' pattern), the
uncompiled refusal, `EnsureStatelessResolvable` (0% in-package at the pin), and numeric strings, NaN and `json.Number`
in comparisons (`toFloat64` at 28.6%). `processor/rule/expression` joins `scripts/cover-check.sh` at 80% (03B D10).

## Risks

Later PRs' tests comparing conditions with `assert.Equal` see the compiled pattern (MEDIUM, PRs 2, 6). A
templated regex in a product rule would stop boot (0 today).
