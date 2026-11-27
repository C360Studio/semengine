# Proposal: rule-core-06-expression-rules

Approved-by:

Track: `design`. PR 6 of the rule-core split plan (`archive/2026-11-25-design-rule-core-split-plan/`). Adds the
`rule-matching` capability and edits one requirement each of `rule-definitions` and `rule-conditions`. Leaves out the
in-memory cooldown (#702, PR 7) and the graph-event path (dropped, #9 comment 7160955176).

## Why

Every later rule PR asks one question: do this rule's conditions hold for this entity, or this message? Today the
answer has three faults. A condition that names a missing value (`$entity.triple.x.y.missing`) is compared as the
literal text and can match. On a message rule, `$entity.id` silently becomes empty. And an evaluation that fails is
logged where nobody looks, so a broken rule just never fires.

## What changes

- A rule that `Load` returned answers "does it match?" for an entity or a message; the firing path (PR 7) and a
  product's own check (`Matches`) share that one answer.
- A message's payload is read through the one declared projection (`message.RuleReadable`), and a payload the engine
  cannot read is reported once per rule and payload type.
- No cooldown, no trigger latch, no graph event. Nothing runs a rule until PR 8.

## Decisions you are making by approving

- **`Matches` becomes a method of a loaded rule (recommended).** A product loads its pack as it already must, then asks
  `loaded.Matches(ctx, entity)`, so the rule it asks about passed every load check, including "no rule matches on an
  agent's private predicate". Other ways: keep it on a raw definition (a second, partial load path), or drop it (no
  product calls it today; semteams' contract test would lose its way to ask). This is new exported surface: two
  methods.
- **A required field that is absent means "no match" when firing and an error when asking (recommended).** 84 of the
  91 product rules use `required: true` to mean "only when present"; logging that as an error on every evaluation
  would bury real faults. This adds one exported error value, `expression.ErrRequiredFieldMissing`, so code tells the
  case apart without reading error text. Other way: report every error and let PRs 7 and 9 log each once.
- **A condition naming a missing value fails the match on the firing path too (recommended).** Today the firing path
  compares the unfilled text and can match; `Matches` already refuses. This changes what the old engine promised
  ("the firing path does not change"). No product rule is affected: the 5 templated values all resolve.
- **`Load` refuses a condition its path can never fill (recommended):** `$entity.` in a rule with no entity pattern,
  `$message.` in one with a pattern. None of the 91 product rules does this.
- **Fable reads D1, D2 and D4 before code (recommended).** The split plan said no Fable read, but this design adds new
  exported surface (two methods, one error value). Other way: decline the read, since the methods follow the old
  engine's own contract.

## Impact

`processor/rule` (match, payload projection, one `Load` check), `processor/rule/expression` (one error value), the
ledger row, `processor/rule/docs/syntax.md`. Independent of PR 4; PR 7 waits for both. semteams' contract test
`test/contract/research_loop_budget_test.go` moves from `NewExpressionRule` to `Load` and `Matches`.
