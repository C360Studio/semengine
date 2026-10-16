# graph-entity-writes

## ADDED Requirements

### Requirement: One rule decodes a stored entity

`graph.UnmarshalEntityState(key, data, &entity)` SHALL refuse stored bytes in this order:

1. bytes that do not decode as an entity (`unreadable_entity_state`);
2. a decoded entity that violates the canonical contract (`noncanonical_predicate` for a predicate, otherwise
   `noncanonical_entity_id`);
3. an entity whose ID is not `key` (`noncanonical_entity_id`);
4. a statement with no `Source` or no `Timestamp` (`missing_statement_metadata`).

Each refusal SHALL be exactly one classified error, with class fatal and code `graph_state_reset_required`, from which
`errors.As` yields a `*graph.StateContractError` whose `Reason` is the refusal's reason and whose `EntityID` is `key`.

graph-ingest's reads of a stored entity, `pkg/lifecycle`'s `Watch` and `WatchEvents`, and `graph.ExactEntityReader`
SHALL apply this rule. A watch SHALL return the refusal as its error and SHALL NOT deliver the entity.

#### Scenario: A watched value without a source

- **WHEN** a lifecycle `Watch` receives a value under key K whose statement has no `Source`
- **THEN** the watch returns an error with code `graph_state_reset_required`, reason `missing_statement_metadata` and
  entity K, and its callback is not called for that value

#### Scenario: A value stored under another key

- **WHEN** the bytes under key K hold entity J, and every statement has a `Source` and a `Timestamp`
- **THEN** `UnmarshalEntityState(K, …)` refuses them with reason `noncanonical_entity_id` and entity K

#### Scenario: The canonical contract is checked before the key

- **WHEN** the bytes under key K hold entity J with a noncanonical predicate
- **THEN** the refusal's reason is `noncanonical_predicate`

#### Scenario: One classification

- **WHEN** any refusal is returned
- **THEN** unwrapping it yields one `*errs.ClassifiedError`, not two
