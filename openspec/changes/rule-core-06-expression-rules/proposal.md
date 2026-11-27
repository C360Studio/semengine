# Proposal: rule-core-06-expression-rules

Approved-by: cglusky (2026-11-27) <https://github.com/C360Studio/semengine/pull/777#issuecomment-7225903712> sha256:f668551c6d9d

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

- **`Matches` becomes a method of a loaded rule, and the only new one (recommended).** The rule asked about has passed
  every load check, including "no rule matches on an agent's private predicate". The cost: the asker must load the
  rule, so it needs a handler for every action type in it and its predicates declared; semteams' research rules need
  `publish_agent`. It answers "no match" for any rule the engine would never run for this entity: a disabled rule, one
  with no entity pattern (it runs on messages or a schedule), an entity outside the pattern. Then the conditions
  decide. It refuses a condition on an entity's lifecycle, as it has no lifecycle lookup; a version that takes one
  waits until a product asks. Other ways: keep it on a raw definition (a second, partial load path), or drop it (no
  product calls it today). New exported surface: one method.
- **A required field that is absent means "no match", with no error, both when firing and when asking
  (recommended).** 84 of 91 product rules use `required: true` to mean "only when present", so this is the ordinary
  "this rule is not for this entity yet". It is exempt from PR 1's every-error-at-Warn commitment, which would
  otherwise log on every evaluation that does not match. A failed comparison and an unfilled template stay errors on
  both. Adds one exported error value, `expression.ErrRequiredFieldMissing`, so the rule package tells the case apart
  without reading error text. What is lost: an asker cannot tell "absent" from "conditions false"; no caller needs to.
  Other ways: an error from `Matches` only, which every asker must special-case; or report every error and let PRs 7
  and 9 log each once.
- **A condition naming a missing value fails the match on the firing path too (recommended).** Today the firing path
  compares the unfilled text and can match; `Matches` already refuses. This changes what the old engine promised. No
  product rule is affected: its 5 templated values all resolve.
- **`Load` refuses a condition its path can never fill (recommended):** `$entity.` in a rule with no entity pattern,
  `$message.` in one with a pattern. None of the 91 product rules does this.
- **Fable read of the exported shape:** read; comment 7223972395; owner ruled 7223995054. The three rulings are worked
  into the two decisions above; no further read.

## Impact

`processor/rule` (match, payload fields, one `Load` check), `processor/rule/expression` (one error value), the ledger
row, `processor/rule/docs/syntax.md`. Independent of PR 4; PR 7 waits for both. semteams' contract test
`test/contract/research_loop_budget_test.go` moves from `NewExpressionRule` to `Load` and `Matches` and keeps its
assertions of false.
