# Proposal: fix-729-follower-status-read-health

Approved-by: cglusky (2026-11-23) <https://github.com/C360Studio/semengine/issues/729#issuecomment-7104642470> sha256:9ffa3079f38b

Track: `quick`. Capability modified: `entity-state-follower` ("The envelope": its text gains what `Stuck` says
while a target read fails, and one scenario). One package, `graph/follower`.

## Why

During a NATS outage a follower cannot read `ENTITY_STATES`' last sequence, so its envelope reads `degraded`, but
`/components/health` still lists graph-index, graph-lexical and graph-clustering as healthy (#729, seen in
`TestSmoke_BrokerOutageUnderRunningStack`). ADR-109 Decision 2 makes health a component's own stuck states, binds
every component's `Health()`, and says a transport failure that persists "is stuck while it lasts and still latches
nothing". graph-ingest already follows it for a failed backlog read. The owner ruled the follower's case a defect
(#729, comment 7104642470).

## What Changes

- The follower records whether its latest target read failed. `Stuck` names it ("ENTITY_STATES status read failed")
  while it holds; the next successful read clears it. It never declares the feed lost.
- graph-index, graph-lexical, graph-clustering and graph-embedding report it through their existing `Health()`,
  which already returns `Stuck`; no component code changes.
- Tests: a follower unit test that a failed read is named by `Stuck`, and a successful read on the same follower
  clears it. `TestSmoke_BrokerOutageUnderRunningStack` stops only logging health during the outage and requires
  each follower-based component unhealthy, naming the failed read; its existing after-restart check already requires
  every component healthy again.

## Non-goals

No change to the envelope, the gauges, `QueryReady`, the lost-feed state, graph-ingest, or the 5 s heartbeat. No
debounce: the state follows the latest read, as graph-ingest's does. The `composed-stack` spec is unchanged; the
outage smoke case has no scenario there.

## Impact

`graph/follower/follower.go` and its tests; `internal/smoke/smoke_integration_test.go`; the `entity-state-follower`
spec. One PR (#731), `Closes #729`. Consumers that read `/components/health` or `/readyz` see the indexes unhealthy
during an outage; `/readyz` already read not ready then, through graph-ingest.
