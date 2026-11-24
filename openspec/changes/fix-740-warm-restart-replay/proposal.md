# Proposal: fix-740-warm-restart-replay

Approved-by:

Track: `design`. Capabilities: `composed-stack`, `transport-client`, `entity-state-follower` (each gains a
Requirement). Packages: `natsclient`, `graph/follower`, `output/websocket`, `processor/graph-query`,
`processor/graph-ingest`, `graph/embedding`; docs in `docs/boot.md`.

## Why

A warm restart over a large store fails to start on a NATS server with default settings (#740). Every graph reader
replays its whole bucket at boot over the process's one NATS connection. The server lets each replay run up to about
32 MiB ahead of the client, and closes a connection that has more than 64 MiB (`max_pending`) waiting to be sent.
With two or more replays on one connection, the waiting bytes add up past that limit, the server drops the
connection, and any component `Start` that runs during the drop fails, so the service fails.

Measured on the pinned server image (2.15.0) and nats.go v1.55.0 (probe740, posted on PR #747): five replays of a
230 MB bucket on one connection lost it 10 times in 58 s; with five, failures begin between 17 and 33 MB. The
replays themselves survive each drop and finish, so a retry alone would leave the drops in place. One replay per
connection lost nothing at 230 MB with five or ten replays. SemSource works around it with `max_pending: 536870912`,
which an operator would have to raise as the store grows (C360Studio/semsource#351).

## What changes

- Each watch that replays a whole graph bucket (`ENTITY_STATES`, `EMBEDDING_INDEX`, `COMMUNITY_INDEX`) runs on a NATS
  connection of its own, opened when the watch starts and closed before the code that started it returns.
- `natsclient.Client` gains `NewSibling`: an unconnected Client with the same settings, without the metrics registry
  and without the connection-lost callback.
- The follower opens its connection in `Run`, inside its existing startup wait, and closes it before `Run` returns.
- `docs/boot.md` says how many connections a stack opens; no `max_pending` setting is needed.

## Non-goals

- Retrying a component `Start` that failed on a lost connection (#740 ask 2); with the replays apart, no connection
  is lost at boot. A separate issue if the owner wants it.
- Moving graph-ingest's boot sweep out of `Start` (ADR-113 stays as decided).
- One follower for several producers (ADR-108 Decision 7 stays as decided).
- Request-driven full watches (the message logger's KV watch); they stay on the shared connection.

## Impact

A tier 0 stack holds five more connections while it runs, and one more during graph-ingest's `Start`; tier 1 adds
three more and one during graph-embedding's `Start`. A server or account connection limit must allow them. Products
need no setting. One PR (#747): `Closes #740`; also `Closes #741` (two stale sentences in files this PR edits).
