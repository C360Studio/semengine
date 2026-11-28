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
- Every handler is handed the same outputs, to publish a message and add a triple. They refuse the reserved reply
  space and stamp who wrote a triple and when.
- Triple actions write through graph-ingest's typed writes; each commits at most one write, and a firing writes
  nothing of its own. A remove or update that loses a race to another writer reads again and retries, up to five
  times, as the lifecycle manager does. With graph writes off, a rule with a triple action is refused at load.
- A product's deny result passes through for PR 7 to stop on. The rule package is checked to reach no agentic or
  governance code.

## Decisions you are making by approving

- **Fable reads the design before any code** (task 1.2): it adds what every product action type is written against.
- **A product's action type gets its outputs as one more argument when it runs (recommended).** They are the two
  things semteams' agent action does: publish, and add a triple. The built-ins reach remove and replace through the
  same argument, privately. It changes one line of the approved page (run took three arguments). The other ways: put
  them on the record an action runs with, which the runner then writes into on every call; or hand nothing, so a
  product skips the reserved-space check and writes through the public projection client under any writer name.
- **The engine's action runner stays internal (recommended).** semteams has four test files that run rule actions
  without the processor: two build the runner for its agent action; two run built-in actions (a lifecycle move, add,
  remove) over their own stand-ins. All four move to processor tests on NATS once PR 8 lands; a test of a product's
  own action type calls it directly. This replaces the approved page's "those tests need a writer". Exporting the runner
  keeps them a rename away but exports two more interfaces PRs 7 to 10 must keep stable.
- **Removing a value removes only what rules wrote (recommended).** SemEngine's write rules let a conditional write name
  one writer; semteams' two removes clear values only its rules write. The other way is one write per writer found,
  which lets a rule erase a lifecycle phase or another component's fact.
- **An update replaces the rule's value in one write (recommended).** No product uses it. A value another writer
  stored stays beside it; a reader asking for one value gets the newest, so the rule's wins until that writer writes
  again. Today it is two writes that first clear every writer's value, and a failed first half is ignored. Other ways:
  carry it as it is, or drop it.
- **Three settings that do nothing are refused at load (recommended):** time-to-live (nothing ever expires a triple),
  update's properties (never written), remove's object (ignored; the whole value goes). No product rule sets them. The
  other way keeps time-to-live, documented as never enforced.
- **An action whose text holds a name that cannot be filled does not run (recommended).** Today a triple's value or a
  message's subject and properties go out with the name's own text; only a triple's subject is refused. semteams has 5
  values its conditions do not guarantee, semboids 12 properties; each would fail with an error naming the field.
- **Rule graph writes go unchecked against value constraints for now (recommended).** You ruled on #350 that the rule
  write path must check once ported, and that checking is opt-in per projection contract. The checker does not exist
  yet, not even for the projection client, and rule writes carry no projection contract, so there is nothing to opt
  into; the approved split plan already says PR 4's writes do not check. #350's change then decides how a rule write
  opts in. The other way holds the rule core until #350's change lands.

## Impact

`processor/rule` (runner, outputs, four built-ins, `DenyVerdict`; partial until PR 12), `internal/graphmutation`
and `pkg/projection` (one shared reading of a write's result, both ledger rows), the agentic import guard,
`processor/rule/docs/actions.md` and `syntax.md`. semboids' publish message keeps its shape. Independent of open PRs;
PR 5 waits for this.
