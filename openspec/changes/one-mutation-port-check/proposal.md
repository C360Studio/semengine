# Proposal: one-mutation-port-check

## Why

Two places decide whether a component port is a graph-mutation port and whether it has the right shape:
`internal/flowgraph` (`validateGraphMutationProviders`), when it builds a graph from admitted declarations, and
graph-ingest (`requireMutationProviderPort`), when its configuration is validated and when it builds its mutation
routes at `Start` (#323). The two copies agree on the shape a valid port has, but not on which ports they look at:
flowgraph only treats a `nats-request` port on `graph.mutation.` as a member, and graph-ingest only a port with
exactly one NATS subject. A `nats` port, or a JetStream port carrying a `graph.mutation.` subject among others,
slips past one copy or the other.

The owner ruled on 2026-10-19 (#323 comment 6279624359):

- **Q1, yes.** A port is a graph-mutation port ("prefix member") when its interface type is
  `semengine.graph.mutation`, or when any one of its NATS subjects starts with `graph.mutation.`. Both callers refuse
  a prefix member that does not have the valid shape.
- **Q2.** Both callers keep their refusal texts unchanged. That graph-ingest's text does not name the port is filed as
  #331 and is out of scope.

## What Changes

- Add `graphmutation.ClassifyPort(facts component.PortFacts, required bool) (member, valid bool)`, the one place
  membership and shape are decided.
- graph-ingest's `requireMutationProviderPort` and flowgraph's `validateGraphMutationProviders` call it; each keeps
  its own direction rule, provider count and refusal texts.
- Flowgraph's `portInfo` carries the port's `component.PortFacts` and loses `Interface`, `kind` and `natsSubjects`.
- Flowgraph newly refuses a `nats` or JetStream port that is a prefix member; graph-ingest newly refuses a
  multi-subject JetStream input with one subject on `graph.mutation.`.
- Pin both callers' refusals with tests, then update the docs, the ledger rows and the spec.

Retires #323. #331 stays open.

## Capabilities

- `graph-transport-boundary`: ADDED "One decision on graph-mutation ports".

## Impact

- New: `internal/graphmutation/port.go` and its test. Changed: `internal/flowgraph/flowgraph.go`,
  `processor/graph-ingest/canonical_mutations.go` and their tests; `docs/admission-ledger.yaml`,
  `docs/repository-map.md`, `internal/flowgraph/README.md`, `internal/flowgraph/doc.go`,
  `processor/graph-ingest/README.md`, `internal/graphmutation/protocol.go`'s package comment.
- `internal/graphmutation` newly imports `component`, so `pkg/lifecycle` and `pkg/projection` newly reach
  `component` and its dependencies. No cycle: `component` reaches no graph-family package
  (`internal/harness/contract/componentdeps_test.go`).
- A declaration with a `nats` or JetStream port on a `graph.mutation.` subject, accepted today by one of the two
  callers, is refused. No consumer declares one: semsource, semconnect and the SemStreams deployment file all use a
  `nats-request` port on `graph.mutation.>`.
