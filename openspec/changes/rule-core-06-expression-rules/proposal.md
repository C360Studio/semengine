# Proposal: rule-core-06-expression-rules

Approved-by: cglusky (2026-11-27) <https://github.com/C360Studio/semengine/pull/777#issuecomment-7221771032> sha256:7768b09ad32b

Track: `design`. PR 6 of the rule-core split plan (`archive/2026-11-25-design-rule-core-split-plan/`). Adds the
`rule-matching` capability and one requirement each to `rule-definitions` and `rule-conditions`. Leaves out the
in-memory cooldown (#702, PR 7) and the graph-event path (dropped, #9 comment 7160955176).

## Why

Every later rule PR asks one question: do this rule's conditions hold for this entity, or this message? Today the
answer has three faults. A condition that names a missing value (`$entity.triple.x.y.missing`) is compared as the
literal text and can match. On a message rule, `$entity.id` silently becomes empty. And an evaluation that fails is
logged where nobody looks, so a broken rule just never fires.

## What changes

- A rule that `Load` returned answers "does it match?" for an entity or a message; the firing path (PR 7) and a
  product's own check (`Matches`) share that one answer.
- A message is read through the fields its payload declares for rules (`message.RuleReadable`); a payload the engine
  cannot read is reported once per rule and payload type.
- No cooldown, no trigger latch, no graph event. Nothing runs a rule until PR 8.

## Decisions you are making by approving

- **`Matches` becomes a method of a loaded rule (recommended).** The rule asked about has passed every load check,
  including "no rule matches on an agent's private predicate". The cost: the asker must load the rule, so it needs a
  handler for every action type in it and its predicates declared; semteams' research rules need `publish_agent`. It
  answers in a fixed order: a disabled rule is "no match"; an enabled rule with no entity pattern is refused (it runs on
  messages or a schedule); an entity outside the rule's pattern is "no match"; then the conditions decide. Other ways:
  keep it on a raw definition (a second, partial load path), or drop it (no product calls it today). New exported
  surface: two methods.
- **A required field that is absent means "no match" when firing and an error when asking (recommended).** 84 of 91
  product rules use `required: true` to mean "only when present". This exempts an absent required field from PR 1's
  every-error-at-Warn commitment, which would otherwise log on every evaluation that does not match. Adds one exported
  error value, `expression.ErrRequiredFieldMissing`, so code tells the case apart without reading error text. Other
  way: report every error and let PRs 7 and 9 log each once.
- **A condition naming a missing value fails the match on the firing path too (recommended).** Today the firing path
  compares the unfilled text and can match; `Matches` already refuses. This changes what the old engine promised. No
  product rule is affected: its 5 templated values all resolve.
- **`Load` refuses a condition its path can never fill (recommended):** `$entity.` in a rule with no entity pattern,
  `$message.` in one with a pattern. None of the 91 product rules does this.
- **Fable reads one question before code (recommended):** "Is `Matches` as a method of a loaded rule (with the path and
  pattern refusals) plus the `ErrRequiredFieldMissing` sentinel the right exported shape?" The split plan said no read,
  but this adds exported surface. Other way: decline, since the methods follow the old engine's own contract.

## Impact

`processor/rule` (match, payload fields, one `Load` check), `processor/rule/expression` (one error value), the ledger
row, `processor/rule/docs/syntax.md`. Independent of PR 4; PR 7 waits for both. semteams' contract test
`test/contract/research_loop_budget_test.go` moves from `NewExpressionRule` to `Load` and `Matches`.
