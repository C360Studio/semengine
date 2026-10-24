# Delta: graph-transport-boundary

## MODIFIED Requirements

### Requirement: graph-ingest refuses an invalid member input

graph-ingest SHALL refuse a member input that is not valid, in `Config.Validate` and when it builds its mutation routes.
When that input is its first member input, the refusal SHALL name the port; a member input after the first is refused
as ambiguous. Each caller keeps its own refusal texts, direction rule and provider count.

#### Scenario: A JetStream input with one graph.mutation subject

- **WHEN** graph-ingest's configuration declares the valid provider and a JetStream input on stream `EVENTS` whose
  subjects are `events.>` and `graph.mutation.entity.create`
- **THEN** `Config.Validate` refuses it with an invalid-class error containing `graph mutation provider port is
  ambiguous`

#### Scenario: A graph-ingest input with the wrong interface version

- **WHEN** graph-ingest's configuration declares a JetStream input `entity_stream` on `entity.>` and a required
  `nats-request` input `mutations` on `graph.mutation.>` with interface `semengine.graph.mutation` `v2`
- **THEN** `Config.Validate` refuses it with an invalid-class error containing `graph mutation provider port
  "mutations" must be a required semengine.graph.mutation v1 nats-request input on graph.mutation.>`
