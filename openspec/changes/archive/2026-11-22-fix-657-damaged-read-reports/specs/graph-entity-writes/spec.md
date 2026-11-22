# graph-entity-writes Delta

## MODIFIED Requirements

### Requirement: A decode refusal carries one classified error

Each refusal `graph.UnmarshalEntityState(key, …)` returns SHALL carry exactly one classified error above its
`*graph.StateContractError`, with class fatal and code `graph_state_reset_required`; `errors.As` yields that
`*graph.StateContractError`, whose `Reason` is the refusal's reason and whose `EntityID` is `key`. The classified
error's `Detail` SHALL hold `entity_ids`, a list holding `key` alone, which `graph.StateRefusalEntityIDs` returns. The
cause beneath it its own classification, if it has one (an invalid entity ID keeps `entity_id_invalid`).

#### Scenario: One classification

- **WHEN** any refusal is returned
- **THEN** unwrapping it down to its `*graph.StateContractError` yields one `*errs.ClassifiedError`, not two

#### Scenario: The refusal names its entity in detail

- **WHEN** the bytes under key K do not decode
- **THEN** the refusal's `Detail["entity_ids"]` is [K], and `graph.StateRefusalEntityIDs` returns [K]
- **AND** the test that verifies this is `TestUnmarshalEntityStateRefusesWhatGraphIngestRefuses`, with
  `TestStateRefusalEntityIDs` for the helper
