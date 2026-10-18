# graph-transport-boundary

## MODIFIED Requirements

### Requirement: Mutation replies carry four outcomes

A mutation reply's `outcome` SHALL be one of `applied`, `unchanged`, `entity_not_found` or `failed`. Every other
failure SHALL travel as a classified error, never as an outcome. graph-ingest SHALL count each canonical mutation
reply once on `graph_mutation_outcomes_total`, and an append once per subject result. The `outcome` label SHALL be
the reply's outcome, or the failure's classified code for a `failed` subject or a refused request, or `internal` when
the error carries no code (ruling L, #91 comment 6062681355). A refused request SHALL also count once on
`mutation_rejections_total` and log one warning line; a refusal that carries detail logs that detail, and an
authority refusal logs no entity identity.

#### Scenario: A create with an unregistered message type

- **WHEN** a create request is refused with code `message_type_unregistered`
- **THEN** `graph_mutation_outcomes_total{operation="entity.create",outcome="message_type_unregistered"}` increases
  by one

#### Scenario: A create refused as an invalid request

- **WHEN** a create request with no entity is refused with code `invalid_request`
- **THEN** `graph_mutation_outcomes_total{operation="entity.create",outcome="invalid_request"}` increases by one

#### Scenario: A revision mismatch

- **WHEN** a reconcile request names an expected revision the entity no longer has
- **THEN** graph-ingest logs exactly one warning line, which carries the expected revision, and
  `graph_mutation_outcomes_total{operation="entity.reconcile",outcome="revision_mismatch"}` increases by one

#### Scenario: An append to three subjects

- **WHEN** an append applies to one subject, finds a second absent, and leaves a third unchanged
- **THEN** `graph_mutation_outcomes_total{operation="triple.append"}` increases by one under each of `applied`,
  `entity_not_found` and `unchanged`

## ADDED Requirements

### Requirement: Query refusals carry a code

Every refusal graph-ingest returns on `graph.ingest.query.entity`, `graph.ingest.query.batch` and
`graph.ingest.query.prefix` SHALL carry a classified code: `invalid_request` for a malformed request, entity ID or
cursor; `entity_not_found`; the `pkg/types` entity-ID code for a malformed prefix or pattern; `response_too_large` when
no page fits the server's maximum payload; `graph_state_reset_required` for a refused stored value; and `internal`
for any other failure. Giving a refusal a code SHALL NOT change its class. A batch request with no IDs SHALL be
answered with an empty `graph.EntityBatchResponse`.

#### Scenario: A malformed batch request

- **WHEN** a batch request is not valid JSON
- **THEN** graph-ingest refuses it with class invalid and code `invalid_request`

#### Scenario: A failed key listing

- **WHEN** listing keys for a prefix query fails
- **THEN** graph-ingest refuses it with class transient and code `internal`
