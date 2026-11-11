# Proposal: alpha4-output-websocket

Approved-by: Coby Leuschke (2026-11-11) in the Enjoy session; recorded on #34 comment 6798408792 sha256:60761d427470

Track: `design` (adds a capability, `output-websocket`; modifies `composed-stack`).

## Why

`output/websocket` is the last piece of `v0.1.0-alpha.4` (#34; owner, 2026-11-10). SemSource composes it today to relay
ingest input (`graph.ingest.>`), the pattern 03B D14 replaces: the engine's tier-0 change-observation transport is an
output component over applied state. semboids composes it to fan out simulation frames that never enter the graph.
Nothing in this repository streams applied state to an application today.

## What Changes

- New `output/websocket`, exporting only `Register`, on `component.Lifecycle`. It has one input per instance:
  - **applied state**: a kv-watch of every `ENTITY_STATES` key, read through one shared `pkg/graphview` view. A client
    gets the current entities, a `synced` marker, then changes. It converges to the current state: each entity's latest
    write always arrives, and values in between may be merged away. It is not a history of every revision.
  - **a core NATS subject**: at most once, best effort; each message is sent unchanged.
- One wire format for both: one JSON object per websocket message (`data`, `put`, `delete`, `poisoned`, `synced`).
- A slow client loses only its own data. On a subject, its queue drops its oldest message. On applied state, its
  pending changes merge per key. Other clients are never slowed. Every drop or merge is counted and logged.
- A pong now extends the read deadline (probe P1): at the pin, every client that only listens is dropped every 60 s.
- File-based TLS through `internal/tlsutil.LoadServerTLSConfig`. Metrics through `metric.RegisterOrGet`.
- Removed and refused at construction, by name: the JetStream input, `delivery_mode` and the unimplemented
  at-least-once mode, `ack_timeout`, `passthrough`. Also removed: the test-only hooks. ACME stays unported.
- New key `ping_interval` (default `30s`), which the pin documented and never had.
- New ADR-115: three HTTP surface roles (front door, debug tools, output components as the streaming plane).
- The composed smoke stack adds the websocket: a client receives the entities the stack ingests (`composed-stack`
  delta, written against the text #588 lands).

## Rulings

Owner, 2026-11-11, #34 comments 6797028887 and 6797377552, #9 comment 6797029453: ACME deferred until a consumer needs
it; one PR built in reviewed steps; three HTTP surface roles; 03B D14 governs and pin ADR-090 decision 6 is not
carried; two inputs (applied state, core subject); the JetStream input is dropped; application SSE, if ever wanted, is a
second wire format on this component; GraphQL subscriptions are out of scope.

## Capabilities

- New `output-websocket`. Modified `composed-stack` (the stack adds a websocket client step).

## Impact

- Pin: 2,220 production and 3,659 test lines. This PR is about 1,000 production and 1,450 test lines, plus about 150
  elsewhere (smoke, ledger, docs); hand estimate. One PR, four reviewed steps.
- Consumers migrate: semsource replaces its JetStream input and drops `delivery_mode`; semboids drops
  `delivery_mode`. Both read the new frame format; only applied-state clients keep entity state by key (design D4).
- New dependency `github.com/gorilla/websocket v1.5.3` (BSD-2-Clause), as at the pin. Merges after #588.
