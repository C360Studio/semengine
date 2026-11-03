# Fable rulings for the graph-query port

The owner runs each question below as its own short Fable session (one session, one question), started in this
branch's worktree: `.worktrees/claude-port-processor-graph-query`, or any checkout of
`claude/port-processor-graph-query`. Each session reads `inventory.md` in this change, section 13 for the options and
section 14 for the question and evidence.

## Owner rulings already made (2026-11-02, Enjoy)

- Agents may read how SemSource and SemConnect call these requests, for this port. So the SemConnect evidence in
  question 1 stands.
- "i am okay with us breaking the pr's up in a way that makes the most sense from a task/dev perspective - line coount
  is a guide". The inventory's split is a starting point, so no answer needs to fit a step's line count.
- All three questions go to Fable.

## What every session does

- Answer the one question. Read-only: no edits, no commits, at most 20 tool calls. If the evidence does not settle it,
  say what would.
- Treat the frozen SemStreams version as evidence of what was learned, not the target. The owner: "graph-query is
  essential to our success - many lessons learned in semstreams gets us pretty close but does not mean we can not
  improve from ground up."
- Sister repositories and the pin are read with
  `gh api -H 'Accept: application/vnd.github.raw' 'repos/C360Studio/<repo>/contents/<path>?ref=<sha>'`; the pin is
  semstreams at `8b99efe9`.
- The inventory's sections 1 to 13 had two review rounds; the third round's corrections were not re-reviewed. Check a
  fact before leaning on it.
- Finish by posting one comment on pull request 504, opening with `Fable ruling <n>`: the answer, why, the observable
  behaviour and a test that would prove it, what it depends on that nobody has measured, and any accepted decision it
  would change. It is advice to the owner, who decides.

## Question 1: the reply format (inventory F2, Q4; before step 1)

What is the one reply contract for `graph.query.*`: which verbs carry the envelope, what does `indexed_revision` mean on
an authority read, and how is an unmet `min_revision` answered? The generation marker of #103 is out of this question.

## Question 2: reaching a community's members (inventory F3, Q2; before step 3)

Given that every page of a prefix read rescans the whole group, how should a search reach a group's entities? Real sizes
at community levels 0 and 1 are unknown; say what the answer depends on. Measured runs: `probes/`. This may change the
sentence in ADR-099 that members are read page by page through prefix queries; if so, say so.

## Question 3: reads during a community rewrite (inventory F1, Q1; before step 3)

While a tick is writing group records, what may one request observe of `COMMUNITY_INDEX`, and what must it refuse?
Enumeration cost belongs to question 2.
