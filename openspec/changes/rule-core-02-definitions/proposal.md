# Proposal: rule-core-02-definitions

Approved-by:

Track: `design`. PR 2 of the rule-core split plan
(`openspec/changes/archive/2026-11-25-design-rule-core-split-plan/`). Adds the `rule-definitions` capability and
edits two `harness-boundaries` requirements.

## Why

Every later rule-core PR needs two things: a rule as written in a file or the rules bucket, and one step that turns
it into a rule ready to run or refuses it with a reason. Today a rule with an unknown action type loads and then fails
every time it fires, with only a log line. A bad rule is skipped at boot with a log line. A typo in a key is ignored.

## What changes

- One decode for rule files, inline rules and the rules bucket. Unknown keys are refused.
- One load step that checks conditions, patterns and every action, and refuses a rule naming the rule, where it came
  from, the position and the cause. Nothing is skipped.
- An action keeps its own settings exactly as written, so a product's action type reads them untouched (closes #26).
- Each built-in action type refuses settings it does not use.

## Decisions you are making by approving

- **A rule that fails its checks stops the load (recommended); it is never skipped.** At start-up the rule processor
  refuses to start, naming the rule. On a live rule change the whole new rule set is rejected and the running set
  stays. The error names the rule file, the inline rule's position in the configuration, or the stored rule and its
  revision. Disabled rules are checked too. The other way, skip it with a log and a counter, is what your ruling on
  the epic forbids.
- **An unknown key in a rule is refused wherever the rule comes from (recommended).** No checked-in product rule has
  one. The other way, refusing only rules written inside the processor's configuration, leaves a typo in a rule file
  silent.
- **What a missing key means.** A rule with no "enabled" runs. A condition with no "required" treats a missing field
  as no match, as rule files do today; all of semboids' conditions rely on this. The other way, a missing field
  becoming an error, would change semboids' behaviour.
- **Author notes stay; the unused pair-rule list goes (recommended).** semteams writes notes into 85 rules; the engine
  never reads them, and the docs will say so. The rule-wide iteration number stays as a value conditions can read,
  documented as not a limit, because 7 semteams rules read it that way. The pair-rule list has no user and no reader;
  it is dropped. The other ways: enforce the iteration number as a limit (changes 7 rules), or drop the notes
  (refuses 85 rules).
- **Restart-recovery settings stay, and an opt-in with nothing to re-run is refused.** No product uses them yet, but
  the durable-execution decision relies on them. Today that mistake is accepted and does nothing.
- **Scheduled (cron) rules are refused until PR 11 brings the timer (recommended).** The other way, accepting them
  and running nothing, is silent. semteams' one scheduled rule is disabled and would still be refused.
- **Counting an action's firings: the consequences of your earlier ruling.** Any real edit to an action restarts its
  count. Counts saved by SemStreams are not carried over. The same key also names the write a reconcile action makes,
  so an edit gives it a new name; within one firing the name is stable, so a retried write is still recognised.
- **Product action types arrive in two steps.** In this PR a product's type can check its settings at load; running
  them is added in PR 4, after PR 3 brings what an action runs with. The other ways are a placeholder type that
  promises what it does not hold, or a second list of type names until PR 4.
- **The rule processor's start-up settings and status move to PR 8 (recommended)**, next to what they describe. The
  custom-rule-type guide is not carried, as you ruled.
- **Remove a guard exception that would name nothing.** The deployment-identity guard lets one rule-engine field hold
  the platform. That field is not ported, so the exception goes before another field can use it unnoticed.

## Impact

`processor/rule` (new, partial until PR 12), its ledger row, the repository map and the coverage list; the
deployment-identity contract test and its spec. No consumer runs this code before PR 8. Waits for PR #753.
