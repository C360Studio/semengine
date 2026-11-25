# Proposal: rule-core-02-definitions

Approved-by: cglusky (2026-11-25) <https://github.com/C360Studio/semengine/pull/758#issuecomment-7168133081> sha256:de1b16366a1f

Track: `design`. PR 2 of the rule-core split plan (`archive/2026-11-25-design-rule-core-split-plan/`). Adds the
`rule-definitions` capability and edits two `harness-boundaries` requirements.

## Why

Every later rule-core PR needs two things: a rule as written, and one step that turns it into a rule ready to run or
refuses it with a reason. Today a rule with an unknown action type loads and then fails every time it fires, with only
a log line; a bad rule is skipped at boot with a log line; a typo in a key is ignored.

## What changes

- One decode for a rule's JSON, used by every source. Unknown keys are refused.
- One load step that checks conditions, patterns and every action, and refuses a rule naming the rule, where it came
  from, the position and the cause. As you ruled on the epic, a refused rule stops the load and is never skipped: the
  processor will not start (PR 8), and a live change is rejected whole (PR 12).
- An action keeps its settings exactly as written, so a product's action type reads them untouched (closes #26). A
  rule that is stored and read back keeps every value but not its spacing, since storing rewrites the JSON compactly.
- Each built-in action type refuses settings it does not use.

## Decisions you are making by approving

- **Disabled rules are checked too (recommended), and a refusal names its source** (rule file, inline position, or
  stored rule and revision). A broken rule cannot wait, switched off; the old engine checked only part of one.
- **An unknown key in a rule is refused wherever the rule comes from (recommended).** No checked-in product rule has
  one. Refusing only rules written inside the processor's configuration would leave a typo in a rule file silent.
- **What a missing key means (recommended).** A rule with no "enabled" runs. A condition with no "required" treats a
  missing field as no match, as rule files do today; all of semboids' conditions rely on this.
- **Author notes stay; the unused pair-rule list goes (recommended).** semteams writes notes into 85 rules; the engine
  never reads them, and the docs will say so. The rule-wide iteration number stays as a value conditions read, not a
  limit (7 semteams rules use it). Other ways: enforce it as a limit (changes 7 rules) or drop notes (refuses 85).
- **A rule's own list of buckets to watch is dropped (you ruled this on PR #758).** No product rule sets it, and a
  rule naming it is refused like any unknown key. The processor's own bucket setting, which semteams uses, stays.
- **Restart-recovery settings stay, and an opt-in with nothing to re-run is refused (recommended).** Unused yet,
  but durable execution relies on them; today that mistake is accepted silently.
- **Until PR 11 brings the timer, an enabled scheduled (cron) rule is refused; a disabled one is not (recommended).**
  semteams keeps its one scheduled rule, switched off, in both of its flows; refusing every scheduled rule would stop
  its rule processor from PR 8 to PR 11. Running a scheduled rule that silently does nothing is not offered.
- **Counting firings: consequences of your earlier ruling.** Any edit to an action restarts its count (a limit raised
  from 3 to 5 allows 5 fresh firings, not 2 more); SemStreams' counts are not carried over. A reconcile write's name
  changes with an edit, never within a firing, so a retry is recognised. Setting an action's id keeps its count.
- **An action is made only by reading its JSON (recommended).** One written directly in Go, with its type set but no
  JSON behind it, is refused at load, saying to decode it instead. semteams' tests build three actions that way; they
  become JSON. Accepting both leaves two copies of one setting that can disagree, with nothing saying which one runs.
- **Product action types arrive in two steps (recommended).** In this PR a product's type can check its settings at
  load; running them comes in PR 4, after PR 3 brings what an action runs with. One function joins the built-in types
  and a product's, refusing a product type that reuses a built-in name; what it reads from configuration grows in PRs
  4 and 5. The other ways are a placeholder that promises what it does not hold, or a second list of type names.
- **Reading rule files, the settings schema and status move to PR 8 (recommended)**: they read the processor's
  settings, which PR 8 brings; this PR brings the decode the loader calls. The custom-rule-type guide goes, as ruled.
- **Remove a guard exception that would name nothing (recommended).** The deployment-identity guard lets one
  rule-engine field hold the platform; that field is not ported, so the exception goes.

## Impact

`processor/rule` (new, partial until PR 12), its ledger row, the repository map and the coverage list; the
deployment-identity contract test and its spec. No consumer runs this code before PR 8. Waits for PR #753.
