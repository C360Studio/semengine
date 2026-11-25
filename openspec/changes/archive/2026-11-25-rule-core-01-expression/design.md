# Design: rule-core-01-expression

Base `245c59c4`. Pin `8b99efe9`. Inventory: `inventory.md` (INVENTORY PASS, round 1). Plan: PR 1 of
`archive/2026-11-25-design-rule-core-split-plan`. Binding: owner regex ruling (PR #752 comment 7162536253), shape
review items 5 and 9 (7162146299). Ledger row: `processor/rule/expression`, disposition `adapt`.

## Context

The package is the one condition evaluator for rule conditions and action `when` guards (ADR-041): 1,417 production
and 2,110 test lines at the pin, no context, goroutine or configuration, and it builds on main after three fixes
(inventory, "Compile drift"). After this design it is about 1,235 production and 2,015 test lines (estimates). One PR,
one package. It retires no issue.

Merge order: independent of #754 (target-shape page) and #755 (deadline-timer probe). PR 1 does not wait for the shape
page (ruling 7162536253); the page takes `ConditionExpression` and `Compile` as given.

## Decisions

**D1. Destination `processor/rule/expression`.** It keeps the pin's path, the rule core lands beside it, and semteams
expects it (`scripts/measure-semengine-port-ledger.go:226`). A top-level `rule/expression` gains nothing; `internal/`
cannot work, because semteams tests and the rule definition's exported fields name the types.

**D2. A regex pattern is compiled once, by the one function that loads a rule (owner ruling).**

- **A (recommended).** `expression.Compile(conds []ConditionExpression) error` compiles each `regex` pattern in the
  slice and keeps it on the element, writing by index, so no caller can compile a loop copy and throw it away. It
  refuses a non-string value, a templated pattern (D3), a pattern over the length limit (D4), or one `regexp` rejects,
  naming the element's index. It does nothing on other operators, or when the pattern is already compiled from the
  current value. Evaluation refuses a `regex` condition with no compiled pattern, or whose value no longer matches it,
  with an `*EvaluationError`; it never compiles. The global cache, `init` and `internal/cache` go. A copied condition
  keeps the pattern; a JSON copy does not, and evaluation then refuses it rather than compiling.
- B. A compiled type (`Compile(expr) (Compiled, error)`) that the evaluator takes. Forgetting to compile is a build
  error. Not recommended: the evaluator's signature and the substitution layer (PRs 3, 6, 7) must carry the compiled
  form through every per-evaluation substitution, for no gain once `Load` is the one caller (Seam).
- C. Compiling inside `UnmarshalJSON`. Not recommended: a decoder's unknown-key refusal stops at a custom unmarshaler
  (probe).
- D. A cache, or compiling on first use: refused by the ruling.

**D3. A regex is templated when it contains a substitution token.** The tokens are the seven prefixes the pin's
substituter fills in anywhere in a value: `$now`, `$entity.`, `$related.`, `$state.`, `$schedule.`, `$caller.`,
`$message.` (inventory). Any other `$` stays literal: `[$a-z]`, `^\$USD` and `\Q$x\E` are accepted. A pattern that
must match a token's text writes its `$` as `[$]` (`[$]now`), which the substituter does not touch. Refusing on the
prefix is slightly wider than the substituter (`$state.foo` is refused, though nothing replaces it), which is the safe
reading of "templated". The list is spelled once, here, as `expression.SubstitutionPrefixes()` (a function returning a
fresh slice, so the package keeps no state); PR 3's substituter reads it rather than spelling its own. This package
already owns the `$state.`, `$prev.` and `$message.` field prefixes, and the substituter imports it. Not taken:
"starts with `$`" misses mid-pattern tokens; "contains `$`" refuses every end-anchored pattern, including semteams'
one regex.

**D4. One pattern limit: 500 characters (owner ruling, PR #753 comment 7163712305).** `Compile` refuses a pattern
longer than 500 characters, which bounds what an author hands the engine. The fragment list, the repeat check, the
group count and the depth check are not carried: `regexp.Compile` already refuses a repeat above 1000, the fragment
list refuses only patterns Go runs in linear time, and the counts miscount escaped and class parentheses. The ReDoS
claim goes with them (`go doc regexp`: time linear in the input).

**D5. Dead surface is dropped (surface audit (a); shape items 3, 4).** `HasTriple`, `GetOutgoing`, `Distance` and
their helpers; the `TypeDetector` interface and `FieldType` with `DetectFieldType` (the two lookups the evaluator
calls become unexported functions); `OperatorFunc` is unexported.

**D6. Zero-use features are carried (item 9; owner bullets).** Twelve operators, `or`, `from`, `$prev.*`,
`$entity.lifecycle.*` and deeper `$message.a.b` paths have no product user. Each has a code reader and a test; the
package is admitted whole. `transition`, `from` and `$prev.*` are carried by owner ruling (PR #753 comment
7163777239): it is the one condition that says where a value came from. `conditions.md` states its three limits, each
checked against the code first (task 3.2): entity rules only; "previous" is the value at this rule's last evaluation
of the entity, so batched changes can read A to C (unverified until PR 10 reads the watcher); text and exact match
only.

**D7 (approved). Empty logic means AND.** Today `""` means OR (`evaluator.go:151`); the rule layer and both guides
spell AND, no production caller passes `""`, and the pin's unit tests pass with the change (probe).

**D8. Docs and ADR.** ADR-041 is ported to `docs/adr/` with a port note (deny/governance examples not carried;
`$entity.lifecycle.*` was added later, ADR-047, PR 5; issue numbers are SemStreams').
`processor/rule/docs/conditions.md` is corrected to the code (inventory, advertised-absent) and gains the regex rules
and limits. `syntax.md` waits for PR 6, which ports what it describes.

## Seam for PRs 2, 3, 6, 7

- The one caller is `Load(def, file, handlers) (*Rule, error)` (target-shape page, PR #754, § 1): the one decode feeds
  it, the map round trip is not carried, and nothing can fire a rule `Load` did not return. `Load` calls `Compile`
  once on the rule's conditions and once on each action's `when`, on the `Rule` it returns, and wraps the error with
  rule ID, file and position (`conditions[2]`, `on_enter[1].when[0]`). The error is classified invalid and names
  index, field, operator and reason. A refusal fails the load, never a log-and-skip. PR 2 decides what `file` names
  for a rule from the KV bucket (key and revision), and a hot-reload refusal is ADR-094's `rejected` activation. This
  design names no other caller.
- PR 3's substituter reads `SubstitutionPrefixes()` for its tokens.
- PRs 6 and 7 report an evaluation error at Warn or above with the rule ID, never Debug, so a refused condition shows.
- PR 7's design weighs a general previous-value field (any field a rule names as `$prev.<field>`, not only a
  `transition`'s), carry as is or generalize with the cost of each, for the owner (comment 7163777239).
- PR 2 settles the old map path's `required` default of true (`config_validation.go:611`) against JSON's false, if any
  source still decodes through a map; semboids omits `required` on all 14 of its conditions.

## Invariants (spec: `specs/rule-conditions/spec.md`)

- `and` is all of the per-condition results, `or` any, and an empty list passes ("Conditions combine").
- `not_in` negates `in`; `between [lo, hi]` is `gte lo` and `lte hi` ("List operators").
- A compiled regex's result equals `regexp.MatchString` on the field's string form; evaluation never compiles ("A
  regex pattern is compiled before evaluation").
- Generated checks: one rapid property test for these, recording the seed and completed checks. Examples cover the
  field-resolution order.

## Tests

The carried tests keep their hand-written table oracles. Dropped: the dead-surface tests
(`evaluator_test.go:429-697`), the cache tests and benchmark, and `TestRegexCache_NoHang` (own 100 ms deadline).
Fixed: the `semantictest` path and the two `Version` lines. Added: `Compile` refusals and acceptance, a slice compiled
and each element evaluated, the uncompiled and edited-value refusals, `EnsureStatelessResolvable` (0% in-package at
the pin), and numeric strings, NaN and `json.Number` (`toFloat64` at 28.6%). `processor/rule/expression` joins
`scripts/cover-check.sh` at 80% (03B D10).

## Risks

A JSON copy of a loaded rule loses its compiled patterns and its regex conditions are refused at evaluation; only
`Load` returns a rule that runs (LOW). The seam rests on the target-shape page, reviewed but not yet approved; if it
changes, PR 2's design re-checks the one caller (MEDIUM). Later tests comparing conditions with `assert.Equal` see the
compiled pattern (MEDIUM, PRs 2, 6).
