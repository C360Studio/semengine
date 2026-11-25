# Proposal: rule-core-01-expression

Approved-by:

Track: `design`. PR 1 of the rule-core split plan (`openspec/changes/archive/2026-11-25-design-rule-core-split-plan/`).
Adds the `rule-conditions` capability.

## Why

Every rule condition and every action guard is checked by one small package. This PR brings it over from SemStreams
whole, so the rule engine's later PRs have something to build on. It carries your regex ruling: a pattern is checked
once when its rule loads, and a pattern built from a template is refused.

## What changes

- The package lands at `processor/rule/expression`, with its tests rewritten to this repository's test rules.
- Regex patterns are prepared when a rule loads. A broken or templated pattern is refused then, not when a message
  arrives. Nothing is kept in a shared cache.
- Four helpers that nothing calls are removed.
- The conditions guide comes along, corrected where it says something the code does not do. ADR-041, the decision
  behind the single evaluator, comes too.

## Decisions you are making by approving

- **What counts as a template in a regex.** Recommended: a pattern is refused when a dollar sign is followed by a
  letter anywhere in it, because that is how every template looks and no real pattern can match it. A rule that
  wants a literal dollar sign before a letter writes `[$]`. The other choices either miss templates in the middle of
  a pattern, or refuse every pattern that ends with `$`, including the one regex your products use.
- **How a pattern is prepared.** Recommended: each condition prepares itself once when its rule loads and keeps the
  result. Code that skips that step gets an error every time the condition is checked, never a silent answer. The
  stricter option, which catches the mistake while compiling, reshapes three later PRs.
- **Twelve condition types no product uses yet: keep (recommended) or drop.** Less-than, at-least, between,
  contains, starts-with, ends-with, not-in, three list-length checks, list-contains, and "changed from X to Y". They
  are small and already tested. Dropping "changed from X to Y" would also remove the previous-value tracking in a
  later PR.
- **"Any of these" matching, previous-value fields, workflow-phase fields, and deeper paths into a message: keep
  (recommended) or drop.** No product uses them today.
- **Remove the helpers nothing calls** (has-triple, outgoing links, distance between two entities, a type
  detector): recommended remove. No code here, in SemStreams or in any product reads them.
- **Optional: rules with no `logic` match all of their conditions.** Today the checker treats a missing value as
  "any of them", although everything else, and the guides, say "all". Recommended yes: no product is affected, and
  it removes a trap for anyone building conditions in code. If you say no, it stays as it is.
- **Bring ADR-041 and the conditions guide now** (recommended). The rule-syntax guide waits for PR 6, the PR that
  ports what it describes.

## Impact

One new package, one ADR, one guide, three admission-ledger rows, one coverage target. No product changes anything
until PR 2 loads rules.
