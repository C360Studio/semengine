# Proposal: fix-768-graph-index-stop

Approved-by: cglusky (2026-11-26) <https://github.com/C360Studio/semengine/pull/770#issuecomment-7196743368> sha256:fa80ee52bd71

Track: `quick`. Capability: `graph-index` (one Requirement modified: "Stop writes accepted work"). Package: `processor/graph-index`.

## Why

graph-index's `Stop` waits until every queued index item is written (`processor/graph-index/component.go:319`,
`KeyedPool.Shutdown`). The queue holds up to 1,000 entities (`index.go:32`). On a loaded host, a stop during a backlog
uses the caller's whole stop deadline, and every component sharing that deadline fails with it: SemSource exits 1
(#768, C360Studio/semsource#406).

Finishing the queue buys nothing. graph-index keeps no resume point: each boot replays every stored entity
(`graph/follower/follower.go:236`), so the next boot redoes the work anyway.

Ending the work context instead is wrong. Each queued item would then fail its entity read, mark the entity failed,
log `reconcile: authoritative entity read failed; readiness withheld`, and complete its revision as applied
(`index.go:205-225`). That is up to 1,000 false warnings and false "applied" marks, and it tears the write in flight.

## What changes

- graph-index sets a stopping flag just before it shuts the pool down.
- `processWork` checks the flag first. A queued item that reaches a lane after the flag is set returns at once:
  no read, no write, no failure mark, no revision completed, no log line.
- An item already being written when the flag is set finishes whole. A stop under backlog therefore takes at most one
  entity's writes per lane.

The state left behind is the state a crash leaves, which the next boot's replay already repairs. The pool keeps its
contract: every accepted item still reaches `Process` (`keyed-dispatch` spec unchanged).

## Decisions you are making by approving

- On stop, graph-index drops index work that has not started, instead of finishing it. Work that has started
  finishes.
- The `graph-index` spec's "Stop writes accepted work" changes to say so. `TestStopWritesAcceptedWork` and
  `TestStopInterruptedThenRetriedWritesAcceptedWork`, which prove the old wording, are rewritten or removed, and
  their admission-ledger line follows.

## Non-goals

- The roughly 40-minute index plateau after a warm restart (#765), and a resume point that avoids re-indexing on
  every boot (#108).
- graph-embedding's stop path: its pool runs under the run context, which has already ended at stop, so its queued
  items do not wait. Whether they log false failures is checked and filed separately if so.
- Any change to `internal/dispatch`.

## Impact

`Stop` on graph-index stays short whatever the backlog. One PR: `Closes #768`.
