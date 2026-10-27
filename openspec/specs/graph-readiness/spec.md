# graph-readiness Specification

## Purpose

`graph/readiness` writes and reads the readiness envelope a producer such as graph-ingest keeps in the `GRAPH_STATUS`
KV bucket, and holds the gate that judges one envelope. The bucket is information, not a gate (ADR-109 Decision 3):
the gate has one in-process reader today, and the Watcher is kept for the ports that will read it. This capability
states what the gate answers for a reading, and which wiring the publisher and the Watcher refuse.

## Requirements

### Requirement: The readiness gate gives one reason in a fixed order

`readiness.EvaluateReadinessGate` SHALL take one `readiness.Reading` and return one `DeferReason`, checked in this
order: `status_unknown` when the reading is not fresh; `unrecognized_state` when the state is not one of
`graph.IndexStates()`; `hard_stop` when the state is `degraded` or `reset_required`; `bootstrap_incomplete` when the
envelope's `bootstrap_complete` is false; otherwise `DeferNone`, whatever the envelope's `ready`, `lag` or
`staleness_ms`.

#### Scenario: A stale reading of a healthy envelope

- **WHEN** the gate reads a `Reading` with `Fresh` false and a `ready`, bootstrapped envelope
- **THEN** it returns `status_unknown`

#### Scenario: Two conditions hold

- **WHEN** the gate reads a fresh `Reading` whose state is `degraded` and whose `bootstrap_complete` is false
- **THEN** it returns `hard_stop`

### Requirement: The zero reading is status_unknown

The zero `Reading` SHALL give `status_unknown`.

#### Scenario: The zero reading

- **WHEN** the gate reads the zero `Reading`
- **THEN** it returns `status_unknown`

### Requirement: The reason list is a new slice on every call

`readiness.DeferReasons()` SHALL return the four non-empty reasons as a new slice on every call.

#### Scenario: The reason list is a copy

- **WHEN** a caller overwrites an element of the slice `DeferReasons()` returned
- **THEN** the next call returns the four reasons unchanged

### Requirement: Readiness wiring is refused, not ignored

`readiness.NewPublisher` SHALL return an error, and no publisher, for a nil bucket or an empty key. `Watcher.Run`
SHALL return an error before any I/O for a nil context, a nil bucket source, an empty key, a heartbeat of zero or less,
or a nil logger. `Publisher.Publish` SHALL return an error for a nil context without writing, and
`Watcher.WaitForFirst` SHALL return an error for a nil context whether or not an envelope has arrived. None SHALL
substitute a default for a missing value.

#### Scenario: A publisher with no key

- **WHEN** `NewPublisher` is called with a bucket and an empty key
- **THEN** it returns an error and a nil publisher

#### Scenario: A watcher with no heartbeat

- **WHEN** `Run` is called on a watcher built with a heartbeat of zero
- **THEN** it returns an error without opening the bucket

#### Scenario: A publish with a nil context

- **WHEN** `Publish` is called with a nil context
- **THEN** it returns an error and the bucket receives no write

#### Scenario: Waiting for the first envelope with a nil context

- **WHEN** `WaitForFirst` is called with a nil context after an envelope has arrived
- **THEN** it returns an error instead of reporting the envelope

### Requirement: ComputeIndexStatus projects a revision watermark

`readiness.ComputeIndexStatus` SHALL report `ready` when the target is above zero and the indexed floor reaches it,
`lag` as the target less the floor (never below zero), state `degraded` when the failed count is above zero, else
`ready` when ready, else `degraded` when stuck, else `building`; and `staleness_ms` 0 when ready or the commit time is
unknown, else the age at least 1.

#### Scenario: Caught up over failures

- **WHEN** the floor equals a nonzero target and the failed count is 1
- **THEN** `ready` is true and the state is `degraded`

### Requirement: A revision-lag producer exports its revisions

The gauge set built for a revision-lag producer SHALL also export `indexed_revision` and `target_revision`; the set
built for a backlog producer SHALL not.

#### Scenario: graph-index's gauges

- **WHEN** graph-index's follower registers its gauges and publishes an envelope with floor F and target T
- **THEN** `semengine_graph_index_indexed_revision` reads F and `semengine_graph_index_target_revision` reads T
