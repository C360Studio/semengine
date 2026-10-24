# Proposal: fix-331-mutation-port-refusal

## Why

When one of graph-ingest's inputs is a graph-mutation port with the wrong shape, `requireMutationProviderPort`
(`processor/graph-ingest/canonical_mutations.go:124-128`) refuses it with `graph mutation provider must be a required
semengine.graph.mutation v1 nats-request input on graph.mutation.>` (#331). The refusal names no port, so an operator
with several inputs has to work out which one is wrong. Flowgraph's refusal for the same shape
(`internal/flowgraph/flowgraph.go:213-215`) names the component and the port. The refusal is correct and stops the
component at `Config.Validate` and at Start; only the message is less useful than it could be (LOW per #185).

## What Changes

- graph-ingest's refusal of a malformed graph-mutation input names the port when that input is graph-ingest's first
  graph-mutation input.
- `TestConfigValidateMutationProviderRefusals` pins the new text, with the port name written as a literal.

## Capabilities

### Modified Capabilities

- `graph-transport-boundary`: the requirement "graph-ingest refuses an invalid member input" (split out of "One
  decision on graph-mutation ports" by #398) says graph-ingest's refusal of its
  first member input that is not valid names the port.

## Impact

- Code: `processor/graph-ingest/canonical_mutations.go`; `processor/graph-ingest/mutation_port_contract_test.go`.
- Docs: the `processor/graph-ingest` row of `docs/admission-ledger.yaml`.
- Behaviour: only the refusal text changes. What is refused, its error class, and where it is refused stay the same.
