# Proposal: rule-core-01-expression

Approved-by:

Track: `design`. PR 1 of the rule-core split plan
(`openspec/changes/archive/2026-11-25-design-rule-core-split-plan/`). Adds the `rule-conditions` capability.

## Why

Every rule condition and every action guard is checked by one small package. This PR brings it over from SemStreams
whole, so the rule engine's later PRs have something to build on. It carries your regex ruling: a pattern is checked
when its rule loads, never while a message is checked, and a pattern built from a template is refused.

## What changes

- The package lands at `processor/rule/expression`, with its tests rewritten to this repository's test rules.
- Regex patterns are prepared when a rule is loaded. A broken or templated pattern is refused then, not when a message
  arrives. Nothing is kept in a shared cache.
- Unused pieces are removed: three helpers (has-triple, outgoing links, distance) and the type detector's interface
  with its never-used type check.
- The conditions guide comes along, corrected where it says something the code does not do. ADR-041, the decision
  behind the single evaluator, comes too.

## Decisions you are making by approving

- **What counts as a template in a regex.** A pattern is refused only when it contains one of the template words the
  rule engine fills in (such as `$entity.` or `$now`). Every other dollar sign is left alone, so a pattern matching
  "$USD" still works. A pattern that must match a template word's text writes the dollar as `[$]`. This is your
  ruling, read on the safe side: a few patterns that start like a template word but that nothing would fill in are
  refused too.
- **Where patterns are prepared.** Recommended: once, by the one function that loads a rule and returns it ready to
  run (the target-shape page's `Load`). A bad pattern stops that load with an error naming the rule and its file.
  Nothing can run a rule that function did not return. Later PRs must still log evaluation errors as warnings, so a
  mistake shows.
- **Keep only the 500-character pattern limit** (your ruling on the PR). Go's regex library already refuses oversized
  repeats and runs every pattern in time linear in its input; some of the old checks refused safe patterns.
- **Twelve condition types no product uses yet: keep (recommended) or drop.** Less-than, at-least, between, contains,
  starts-with, ends-with, not-in, three list-length checks, list-contains, and "changed from X to Y". They are small
  and already tested. "Changed from X to Y" is kept by your ruling, and the guide states its three limits.
- **"Any of these" matching, previous-value fields, workflow-phase fields, and deeper paths into a message: keep
  (recommended) or drop.** No product uses them today.
- **Remove the unused pieces listed above.** Recommended remove: no code here, in SemStreams or in any product reads
  them.
- **Optional: rules with no `logic` match all of their conditions.** Today the checker treats a missing value as "any
  of them", although everything else, and the guides, say "all". Recommended yes: no product is affected. If you say
  no, it stays as it is.
- **Bring ADR-041 and the conditions guide now** (recommended). The rule-syntax guide waits for PR 6, which ports what
  it describes.

## Impact

One new package, one ADR, one guide, three admission-ledger rows, one coverage target. No product changes anything
until rules load (PR 2) and fire (PR 8).
