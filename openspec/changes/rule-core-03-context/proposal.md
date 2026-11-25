# Proposal: rule-core-03-context

Approved-by:

Track: `design`. PR 3 of the rule-core split plan (`archive/2026-11-25-design-rule-core-split-plan/`). Adds the
`rule-substitution` and `rule-state` capabilities and edits one `rule-conditions` requirement.

## Why

An action needs what fired it (entity, stored state, message) and a way to fill names such as `$entity.id` into its
text; every later rule-core PR runs on this. Today the filling has three bugs: text it filled in is filled in again, so
a triple holding `$message.secret` leaks the message's field; a name that extends a present one is half-filled
(`P-entity`); and `.length` on a value stamped several times gives an error marker, which two of semteams' live prompts
show today.

## What changes

- Text is filled once, left to right; a name either resolves whole or stays as written with a warning.
- `.length` counts every value of a predicate, the same values `.triples` lists.
- The stored state record and its store come over, keys checked before NATS. Nothing runs this until PR 4.

## Decisions you are making by approving

- **Fable reads this design before any code (recommended).** The split plan skipped PR 3 as a straight carry, but
  this design sets what every product action type receives, changes how semteams builds the state store (two of its
  tests), and decides what the stored record's fields mean.
- **Drop `$caller.*` (recommended).** Meant to carry who made a request, but nothing ever fills it and no rule uses
  it; caller identity can come later with the code that fills it.
- **Drop `$related.*` and two-entity ("pair") state (recommended).** Nothing fills the related entity, no rule uses
  it, and you already dropped the list that would have named it (PR 2). This corrects one line of the approved
  target-shape page: the stored record is one rule's state for one entity, not "one entity or pair". With both drops,
  `$caller.` and `$related.` become plain text in a rule, and a regex holding them is accepted.
- **Keep `$entity.lifecycle.*` (recommended)** though no rule uses it yet: you kept the condition side in PR 1, and
  dropping the text side would make one name work in a condition and not in a message.
- **Keep `$now` (recommended), against the old decision record ADR-047, which said there is no `$now`.** semteams
  uses it in 3 prompts. Whether it reads the clock or the firing's own time is settled in PR 7, which owns the clock.
- **What a product's action type gets (recommended): the context's fields, the text filler, and the typed filler for a
  single name.** The helpers for `for_each` loops are dropped: only `publish_agent` used them, and it leaves the
  engine. semteams' two `for_each` actions can get their list from `.triples`, a JSON array, and fill each item
  themselves. The other way is to keep both helpers as supported API, with a contract and a test.
- **`.length` counts every value (recommended), as ADR-048 says; ADR-048 is carried into this repository.** Today it
  reads only the first triple and gives an error marker for a value stamped once per iteration, which is how semteams
  records finished experiments; its "plateau after N iterations" and final-rollup prompts get the marker instead of N.
  The other way keeps that behaviour.
- **A value is never filled twice, and a name is read whole (recommended).** Text from a triple or message that looks
  like a name stays; a name naming a missing value stays visible with a warning, not half-filled. Every product
  rule is run through both engines before merge, and each difference is listed.
- **The stored copy of a rule's `max_iterations` stays (recommended)**, as the value `$state.max_iterations` shows,
  not a limit, as you approved in PR 2.
- **Three stored fields keep the meaning the operations guide gives them (recommended):** the last transition and
  when it happened survive later checks that change nothing; "last checked" is every check. Today they are
  overwritten each time and nothing reads them. PR 7 writes them. The other way drops them.
- **The state store sits on SemEngine's NATS key-value wrapper (recommended)**, which refuses bad keys and a missing
  context and answers an empty bucket cleanly. semteams' two tests that build the store from their own in-memory bucket
  change. Deleting one record by hand is dropped: nothing calls it.

## Impact

`processor/rule` (extends PR 2's package), `processor/rule/expression` (`SubstitutionPrefixes` loses two names), its
ledger row, `docs/adr/048`, the reviewer contract's substitution clause. Waits for PR #758.
