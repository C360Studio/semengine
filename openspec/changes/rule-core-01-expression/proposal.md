# Proposal: rule-core-01-expression

Approved-by:

Track: `design`. PR 1 of the rule-core split plan (`openspec/changes/archive/2026-11-25-design-rule-core-split-plan/`).
Adds the `rule-conditions` capability.

## Why

Every rule condition and every action guard is checked by one small package. This PR brings it over from SemStreams
whole, so the rule engine's later PRs have something to build on. It carries your regex ruling: a pattern is checked
when its rule loads, never while a message is checked, and a pattern built from a template is refused.

## What changes

- The package lands at `processor/rule/expression`, with its tests rewritten to this repository's test rules.
- Regex patterns are prepared when a rule is loaded. A broken or templated pattern is refused then, not when a
  message arrives. Nothing is kept in a shared cache.
- Unused pieces are removed: three helpers (has-triple, outgoing links, distance) and the type detector's interface
  with its never-used type check.
- The conditions guide comes along, corrected where it says something the code does not do. ADR-041, the decision
  behind the single evaluator, comes too.

## Decisions you are making by approving

- **What counts as a template in a regex.** A pattern is refused only when it contains one of the template words the
  rule engine fills in (such as `$entity.` or `$now`). Every other dollar sign is left alone, so a pattern matching
  "$USD" still works. A pattern that must match a template word's text writes the dollar as `[$]`. This is your
  ruling, read on the safe side: a few patterns that start like a template word but that nothing would fill in
  are refused too.
- **Where patterns are prepared.** Recommended: each rule's conditions are prepared at the point the engine keeps the
  rule to run it, and a bad pattern stops that load with an error naming the rule and where it came from. If a later
  part of the engine skipped that step, the rule would not run, so those later PRs must also log evaluation
  errors as warnings. The stricter option, which catches a skipped step when the code is built, reshapes three
  later PRs and still leaves them the same step to do. One difference from your ruling's word "once": a pattern
  may be compiled twice while a rule loads (once to name the file in an error, once on the copy that runs). It is
  still never compiled while a message is checked, and nothing is cached.
- **Keep the pattern limits** (length, groups, nesting, very large repeats). Recommended keep. They bound what a
  rule author can hand the engine; no product pattern comes near them, and loosening them later breaks no one.
- **Twelve condition types no product uses yet: keep (recommended) or drop.** Less-than, at-least, between,
  contains, starts-with, ends-with, not-in, three list-length checks, list-contains, and "changed from X to Y". They
  are small and already tested. Dropping "changed from X to Y" also removes the previous-value tracking in a later
  PR.
- **"Any of these" matching, previous-value fields, workflow-phase fields, and deeper paths into a message: keep
  (recommended) or drop.** No product uses them today.
- **Remove the unused pieces listed above.** Recommended remove: no code here, in SemStreams or in any product
  reads them.
- **Optional: rules with no `logic` match all of their conditions.** Today the checker treats a missing value as
  "any of them", although everything else, and the guides, say "all". Recommended yes: no product is affected. If
  you say no, it stays as it is.
- **Bring ADR-041 and the conditions guide now** (recommended). The rule-syntax guide waits for PR 6, which ports
  what it describes.

## Impact

One new package, one ADR, one guide, three admission-ledger rows, one coverage target. No product changes anything
until rules load (PR 2) and fire (PR 8).
