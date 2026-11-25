# Design: rule-core-01-expression

Base `245c59c4`. Pin `8b99efe9`. Inventory: `inventory.md` (INVENTORY PASS, round 1). Plan: PR 1 of
`archive/2026-11-25-design-rule-core-split-plan`. Binding: owner regex ruling (PR #752 comment 7162536253), shape
review items 5 and 9 (7162146299). Ledger row: `processor/rule/expression`, disposition `adapt`.

## Context

The package is the one condition evaluator for rule conditions and action `when` guards (ADR-041): 1,417 production
and 2,110 test lines at the pin, no context, goroutine or configuration, and it builds on main after three fixes
(inventory, "Compile drift"). After this design it is about 1,235 production and 2,015 test lines (estimates). One
PR, one package. It retires no issue.

Merge order: independent of #754 (target-shape page) and #755 (deadline-timer probe). PR 1 does not wait for the
shape page (ruling 7162536253); the page takes `ConditionExpression` and `Compile` as given.

## Decisions

**D1. Destination `processor/rule/expression`.** It keeps the pin's path, the rule core lands beside it, and semteams
expects it (`scripts/measure-semengine-port-ledger.go:226`). A top-level `rule/expression` gains nothing;
`internal/` cannot work, because semteams tests and the rule definition's exported fields name the types.

**D2. A regex pattern is compiled where a rule is kept, never during evaluation (owner ruling).**

- **A (recommended).** `ConditionExpression.Compile() error` compiles a `regex` pattern and keeps it on the condition.
  It refuses a non-string value, a templated pattern (D3), a pattern over the limits (D4), or one `regexp` rejects.
  It does nothing on other operators, or when the pattern is already compiled from the current value. Evaluation
  refuses a `regex` condition with no compiled pattern, or whose value no longer matches it, with an
  `*EvaluationError`; it never compiles. The global cache, `init` and `internal/cache` go. A copied condition keeps the
  pattern; a JSON or map copy does not, which matters because the pin copies definitions several times after
  loading. So `Compile` must run on the copy that fires (Seam), and since the pin's fire paths swallow evaluation
  errors (`expression_factory.go:200-205`, `:224-230` at Debug; `stateful_evaluator.go:380-386` at Warn), a missed
  compile would make a rule silently never fire. The two obligations below close that.
- B. A compiled type (`Compile(expr) (Compiled, error)`) that the evaluator takes. Forgetting to compile is a build
  error. Not recommended: the evaluator's signature and the substitution layer (PRs 3, 6, 7) must carry the
  compiled form through every per-evaluation substitution, and the keepers still must call it, so this moves the
  same obligation into three more designs.
- C. Compiling inside `UnmarshalJSON`. Not recommended: a decoder's unknown-key refusal stops at a custom
  unmarshaler (probe), and the pin's map path builds conditions without decoding (`config_validation.go:607`).
- D. A cache, or compiling on first use: refused by the ruling.

**D3. A regex is templated when it contains a substitution token.** The tokens are the seven strings the pin's
substituter replaces anywhere in a value: `$now`, `$entity.`, `$related.`, `$state.`, `$schedule.`, `$caller.`,
`$message.` (inventory). Any other `$` stays literal: `[$a-z]`, `^\$USD` and `\Q$x\E` are accepted. A pattern that
must match a token's text writes its `$` as `[$]` (`[$]now`), which the substituter does not touch. This is exactly
the ruling's refusal. Cost: a second spelling of the token list, held in step by a PR 3 test that `Compile` refuses
every token the substituter replaces. Not taken: "starts with `$`" misses mid-pattern tokens; "contains `$`" refuses
every end-anchored pattern, including semteams' one regex.

**D4. Pattern limits are kept as authoring limits (owner: keep or drop).** Length 500, 20 groups, depth 5, the
nested-repeat fragment list, repeats of 1000 and up, run inside `Compile`. The regexp package promises time linear in
the input and nothing about pattern size, so the limits bound what an author can hand the engine; the ReDoS claim
is removed. No product pattern comes near them. Recommended keep: loosening later breaks nobody.

**D5. Dead surface is dropped (surface audit (a); shape items 3, 4).** `HasTriple`, `GetOutgoing`, `Distance` and their
helpers; the `TypeDetector` interface and `FieldType` with `DetectFieldType` (the two lookups the evaluator calls
become unexported functions); `OperatorFunc` is unexported.

**D6. Zero-use features are carried (item 9; owner bullets).** Twelve operators, `or`, `from`, `$prev.*`,
`$entity.lifecycle.*` and deeper `$message.a.b` paths have no product user. Each has a code reader and a test; the
package is admitted whole; dropping `transition` also drops PR 7's previous-value tracking.

**D7 (optional, owner's choice). Empty logic means AND.** Today `""` means OR (`evaluator.go:151`); the rule layer
and both guides spell AND, no production caller passes `""`, and the pin's unit tests pass with the change (probe).

**D8. Docs and ADR.** ADR-041 is ported to `docs/adr/` with a port note (deny/governance examples not carried;
`$entity.lifecycle.*` was added later, ADR-047, PR 5; issue numbers are SemStreams'). `processor/rule/docs/conditions.md`
is corrected to the code (inventory, advertised-absent) and gains the regex rules and limits. `syntax.md` waits for PR
6, which ports what it describes.

## Seam for PRs 2, 6, 7, 11, 12

- PR 2 owns one preparation step for a definition (validate, then `Compile` every condition and every action `when`
  condition) that every keeper calls on the copy it keeps, after the last JSON or map copy: the boot store
  (`rule_loader.go:215`, `:243`), `CreateRuleFromDefinition`, cron (PR 11), runtime and KV apply (PR 12).
  `Compile` is deterministic, so a definition that passed once passes again.
- A refusal fails the load, never a log-and-skip. It names the source: the file (run the step at the per-file decode,
  `rule_loader.go:72`, before the merge), `inline_rules[i]` with the component name, or the KV key and revision, where
  it is an ADR-094 `rejected` activation. PR 2 wraps `Compile`'s error, which is classified invalid and names field,
  operator and reason, with the rule ID, the source and the position (`conditions[2]`, `on_enter[1].when[0]`).
- PRs 6 and 7 report an evaluation error at Warn or above with the rule ID, never Debug, so a missed compile shows.
- PR 2 settles the map path's `required` default of true (`config_validation.go:611`) against JSON's false; semboids
  omits `required` on all 14 of its conditions.

## Invariants (spec: `specs/rule-conditions/spec.md`)

- `and` is all of the per-condition results, `or` any, and an empty list passes ("Conditions combine").
- `not_in` negates `in`; `between [lo, hi]` is `gte lo` and `lte hi` ("List operators").
- A compiled regex's result equals `regexp.MatchString` on the field's string form; evaluation never compiles ("A
  regex pattern is compiled before evaluation").
- Generated checks: one rapid property test for these, recording the seed and completed checks. Examples cover the
  field-resolution order.

## Tests

The carried tests keep their hand-written table oracles. Dropped: the dead-surface tests (`evaluator_test.go:429-697`),
the cache tests and benchmark, and `TestRegexCache_NoHang` (own 100 ms deadline). Fixed: the `semantictest` path and
the two `Version` lines. Added: `Compile` refusals and acceptance, the uncompiled and edited-value refusals,
`EnsureStatelessResolvable` (0% in-package at the pin), and numeric strings, NaN and `json.Number` (`toFloat64` at
28.6%). `processor/rule/expression` joins `scripts/cover-check.sh` at 80% (03B D10).

## Risks

A keeper that skips the step leaves a rule that never fires, visible only through PRs 6 and 7's Warn line (HIGH
until those PRs land; nothing fires before PR 8). Later tests comparing conditions with `assert.Equal` see the
compiled pattern (MEDIUM, PRs 2, 6).
