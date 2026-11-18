# fusion Delta

## MODIFIED Requirements

### Requirement: A fatal seed read fails the call

When `Resolve` or `Entities` returns an error `errs.IsFatal` reports, such as `graph_state_reset_required` from a
reset, `Fuse` SHALL return that error and no response. `Fuse` SHALL refuse a nil context. A damaged seed does not
reach `Fuse` as an error from `fusionnats`, which lists it as unhydrated (requirement "A damaged seed is unhydrated,
not a failed read"). Nor does a damaged entity under a resolved prefix (requirement "A prefix resolve lists IDs, then
reads them").

#### Scenario: A reset while hydrating seeds

- **WHEN** `Resolve` answers and `Entities` returns `graph_state_reset_required`
- **THEN** `Fuse` returns that error (`TestFatalReplyReturnsError`)

### Requirement: The client's own decode refusal is returned, never retried

A refusal from the client's own check of the batch reply (`graph.ValidateDecodedEntityStates`) SHALL be returned by
`Entities`, which SHALL send no further request.

#### Scenario: The client's own validation refuses

- **WHEN** the batch reply fails the client's own stored-value check
- **THEN** `Entities` returns that refusal and sends no `entity` request (`TestOwnValidationNotRetried`)

## ADDED Requirements

### Requirement: A damaged seed is unhydrated, not a failed read

`Entities` SHALL list a requested ID the batch reply names `damaged` as unhydrated `error`, hydrate the others in
request order, and send one request: the batch. A batch refusal of any class or code SHALL be returned by `Entities`
with no `entity` request.

#### Scenario: One damaged seed

- **WHEN** the batch reply names the second of three seeds `damaged`
- **THEN** the first and third hydrate in request order and the second is unhydrated `error`
  (`TestDamagedSeedUnhydrated`, `TestPoisonedSeedOverRealResponders`)

#### Scenario: A refused batch

- **WHEN** the batch is refused with code `graph_state_reset_required`
- **THEN** `Entities` returns that refusal and sends no `entity` request (`TestBatchRefusalNotReadAlone`)

## REMOVED Requirements

### Requirement: A poisoned seed is unhydrated, not a failed read

**Reason**: graph-ingest's batch no longer refuses for a damaged entity; it names it `damaged`, so fusion has no
refusal to read around. Replaced by "A damaged seed is unhydrated, not a failed read".

**Migration**: None for fusion consumers: a damaged seed is still unhydrated `error`.

### Requirement: Reading seeds alone fails on any other fault

**Reason**: `Entities` no longer reads seeds one at a time.

**Migration**: None.
