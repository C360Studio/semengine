# graph-transport-boundary

## ADDED Requirements

### Requirement: One decision on graph-mutation ports

Whether a component port is a graph-mutation port, and whether it has the valid shape, SHALL be decided in
`internal/graphmutation` (`ClassifyPort`), and `internal/flowgraph` and graph-ingest SHALL both use that decision. A
port SHALL be a member when its interface type is `semengine.graph.mutation`, or when any one of its NATS subjects
starts with `graph.mutation.`. A member SHALL be valid only when it is a required `nats-request` port whose subjects
are exactly `graph.mutation.>` and whose interface is `semengine.graph.mutation` `v1`. Flowgraph SHALL refuse a
member that is not valid, on an input or an output, with an error of the invalid class naming the component and the
port. graph-ingest SHALL refuse a member input that is not valid, in `Config.Validate` and when it builds its mutation
routes. Each caller keeps its own refusal texts, direction rule and provider count.

#### Scenario: A nats port on a graph.mutation subject

- **WHEN** a flow declares a `nats` output on `graph.mutation.entity.create` beside one valid provider
- **THEN** flowgraph refuses it with `component requester port legacy is not a required nats-request
  semengine.graph.mutation v1 port on graph.mutation.>`

#### Scenario: A JetStream input with one graph.mutation subject

- **WHEN** graph-ingest's configuration declares the valid provider and a JetStream input on stream `EVENTS` whose
  subjects are `events.>` and `graph.mutation.entity.create`
- **THEN** `Config.Validate` refuses it with an invalid-class error containing `graph mutation provider port is
  ambiguous`

#### Scenario: The valid provider

- **WHEN** a port is a required `nats-request` port on `graph.mutation.>` with interface `semengine.graph.mutation`
  `v1`
- **THEN** `ClassifyPort` reports it a member and valid, and both callers accept it
