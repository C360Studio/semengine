# graph-entity-writes

## ADDED Requirements

### Requirement: One rule decodes a stored entity

`graph.UnmarshalEntityState(key, data, &entity)` SHALL refuse stored bytes in this order:

1. bytes that do not decode as an entity (`unreadable_entity_state`);
2. a decoded entity that violates the canonical contract (`noncanonical_predicate` for a predicate, otherwise
   `noncanonical_entity_id`);
3. an entity whose ID is not `key` (`noncanonical_entity_id`);
4. a statement with no `Source` or no `Timestamp` (`missing_statement_metadata`).

Each refusal SHALL carry exactly one classified error above its `*graph.StateContractError`, with class fatal and code
`graph_state_reset_required`; `errors.As` yields that `*graph.StateContractError`, whose `Reason` is the refusal's reason
and whose `EntityID` is `key`. The cause beneath it keeps its own classification, if it has one (an invalid entity ID
keeps `entity_id_invalid`).

graph-ingest's validated read of an entity (the entity, batch and prefix verbs, reconcile, append and delete), its
boot sweep of `ENTITY_STATES`, and its re-check of a stored value after a failed read-modify-write SHALL apply this
rule; so SHALL `pkg/lifecycle`'s `Watch` and `WatchEvents`, and `graph.ExactEntityReader`. The write seam's own read
of the value it changes is the one exception: it decodes without the canonical contract and checks only the key
(ADR-110 Decision 6; "The write path refuses a stored value it cannot change"), and when it refuses, the refusal
SHALL be the one this rule gives for those bytes. A watch SHALL return the refusal as its error and SHALL NOT deliver
the entity. A mutation reply that refuses for a stored value SHALL carry the refusal without adding a classification.

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
- **THEN** unwrapping it down to its `*graph.StateContractError` yields one `*errs.ClassifiedError`, not two

#### Scenario: A reconcile refused for its stored value

- **WHEN** a reconcile names entity K, whose stored value holds a statement with no `Source`
- **THEN** the error returned holds one `*errs.ClassifiedError` above its `*graph.StateContractError`,
  with code `graph_state_reset_required` and reason
  `missing_statement_metadata`
