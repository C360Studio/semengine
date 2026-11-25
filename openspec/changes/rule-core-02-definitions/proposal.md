# Proposal: rule-core-02-definitions

Approved-by:

Track: `design`. PR 2 of the rule-core split plan
(`openspec/changes/archive/2026-11-25-design-rule-core-split-plan/`). Adds the `rule-definitions` capability and
edits two `harness-boundaries` requirements.

## Why

Every later rule-core PR needs two things: a rule as written, and one step that turns it into a rule ready to run or
refuses it with a reason. Today a rule with an unknown action type loads and then fails every time it fires, with only
a log line; a bad rule is skipped at boot with a log line; a typo in a key is ignored.

## What changes

- One decode for a rule's JSON, used by every source. Unknown keys are refused.
- One load step that checks conditions, patterns and every action, and refuses a rule naming the rule, where it came
  from, the position and the cause. As you ruled on the epic, a refused rule stops the load and is never skipped: the
  processor will not start (PR 8), and a live change is rejected whole (PR 12).
- An action keeps its own settings exactly as written, so a product's action type reads them untouched (closes #26).
- Each built-in action type refuses settings it does not use.

## Decisions you are making by approving

- **Disabled rules are checked too (recommended), and a refusal names its source** (rule file, inline position, or
  stored rule and revision). A broken rule cannot wait, switched off. The other way is the old engine's: enabled only.
- **An unknown key in a rule is refused wherever the rule comes from (recommended).** No checked-in product rule has
  one. The other way, refusing only rules written inside the processor's configuration, leaves a typo in a rule file
  silent.
- **What a missing key means (recommended).** A rule with no "enabled" runs. A condition with no "required" treats a
  missing field as no match, as rule files do today; all of semboids' conditions rely on this. The other way, a
  missing field becoming an error, would change semboids' behaviour.
- **Author notes stay; the unused pair-rule list goes (recommended).** semteams writes notes into 85 rules; the engine
  never reads them, and the docs will say so. The rule-wide iteration number stays as a value conditions can read,
  documented as not a limit, as 7 semteams rules use it. The pair-rule list has no user and no reader. The other ways:
  enforce the iteration number as a limit (changes 7 rules), or drop the notes (refuses 85 rules).
- **Restart-recovery settings stay, and an opt-in with nothing to re-run is refused (recommended).** Unused yet,
  but durable execution relies on them; today that mistake is accepted silently.
- **Until PR 11 brings the timer, an enabled scheduled (cron) rule is refused; a disabled one is checked like any
  other rule but not refused for being scheduled (recommended).** semteams lists its one scheduled rule, switched
  off, in both of its flows; refusing every scheduled rule would stop semteams' rule processor from PR 8 until PR 11
  unless it edits both flows. Running a scheduled rule silently doing nothing is the third way, not offered.
- **Counting an action's firings: consequences of your earlier ruling.** Any edit to an action restarts its count,
  including raising its limit: a limit raised from 3 to 5 allows 5 fresh firings, where the old engine allowed 2
  more. Counts saved by SemStreams are not carried over. The same key names the write a reconcile action makes, so an
  edit gives it a new name; within one firing the name is stable, so a retried write is still recognised. An author
  who wants a count kept across edits sets the action's id.
- **Product action types arrive in two steps (recommended).** In this PR a product's type can check its settings at
  load; running them is added in PR 4, after PR 3 brings what an action runs with. The other ways are a placeholder
  that promises what it does not hold, or a second list of type names until PR 4.
- **Reading rule files moves to PR 8, and so do the processor's settings schema and status (recommended).** The split
  plan put the file loader here, but it reads the processor's configuration, which PR 8 brings. This PR brings the
  decode the loader calls. The custom-rule-type guide is not carried, as you ruled.
- **Remove a guard exception that would name nothing (recommended).** The deployment-identity guard lets one
  rule-engine field hold the platform; that field is not ported, so the exception goes.

## Impact

`processor/rule` (new, partial until PR 12), its ledger row, the repository map and the coverage list; the
deployment-identity contract test and its spec. No consumer runs this code before PR 8. Waits for PR #753.
