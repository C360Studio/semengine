# Proposal: rule-core-04-graph-actions

Approved-by:

Track: `design`. PR 4 of the rule-core split plan (`archive/2026-11-25-design-rule-core-split-plan/`). Adds the
`rule-actions` capability and changes two `rule-definitions` requirements. Closes #27.

## Why

A rule's actions do not run yet. At SemStreams today, a triple or publish action with nothing to write through
succeeds and does nothing; a remove clears a value written by anyone; an update writes twice and ignores a failed
first half; three settings (a triple's time-to-live, update's properties, remove's object) do nothing; and a name in
a value that cannot be filled is written out as its own text.

## What changes

- An action runs through its type's handler, built-in or a product's, from the one set PR 2 built. The engine's runner
  is built once with its publisher and graph writer.
- Every handler is handed the same outputs: publish a message, add, remove or replace a triple. They refuse the
  reserved reply space and stamp who wrote a triple and when.
- Triple actions write through graph-ingest's typed writes, at most one write per action; a firing writes nothing of
  its own. With graph writes off, a rule with a triple action is refused at load.
- A product's deny result passes through for PR 7 to stop on. The rule package is checked to reach no agentic or
  governance code.

## Decisions you are making by approving

- **Fable reads the design before any code** (task 1.2): it adds what every product action type is written against.
- **A product's action type gets its outputs as one more argument when it runs (recommended).** It is how the
  approved page's "publisher handed to every handler" reaches semteams' agent action, which publishes and writes
  triples. It changes one line of that page (run took three arguments). The other ways: put them on the record an
  action runs with (each path that builds it must fill them, and a forgotten one fails only at fire), or hand nothing
  (products bypass the reserved-space check, and cannot reach the graph writer at all).
- **The engine's action runner stays internal (recommended).** semteams' five contract tests that build one stop
  compiling; they call their own handler with a stand-in instead. Exporting it adds a second way to run an action.
- **Removing a value removes only what rules wrote (recommended).** SemEngine's write rules let a conditional write name
  one writer; semteams' two removes clear values only its rules write. The other way is one write per writer found,
  which lets a rule erase a lifecycle phase or another component's fact.
- **An update replaces the value in one write (recommended).** No product uses it. Today it is two writes with a
  moment in between where the value is missing, and a failed first half is ignored. Other ways: carry it as it is, or
  drop it.
- **Three settings that do nothing are refused at load (recommended):** time-to-live (nothing ever expires a triple),
  update's properties (never written), remove's object (ignored; the whole value goes). No product rule sets them. The
  other way keeps time-to-live, documented as never enforced.
- **An action whose text holds a name that cannot be filled does not run (recommended).** Today a triple's value or a
  message's subject and properties go out with the name's own text; only a triple's subject is refused. semteams has 5
  values its conditions do not guarantee, semboids 12 properties; each would fail with an error naming the field.
- **Rule graph writes are not checked against value constraints yet.** You ruled on #350 that the rule write path must
  check once it is ported; the checker does not exist. This PR writes without it, and #350's change adds the check at
  this writer.

## Impact

`processor/rule` (executor, outputs, four built-ins, `DenyVerdict`; partial until PR 12), `internal/graphmutation`
and `pkg/projection` (one shared reading of a write's result), the agentic import guard, the ledger row,
`processor/rule/docs/actions.md` and `syntax.md`. semboids' publish message keeps its shape. Independent of open PRs;
PR 5 waits for this.
