# embedding-index Delta

## MODIFIED Requirements

### Requirement: A save older than the stored record is refused

`Storage` SHALL write every read-modify-write through `natsclient.KVStore.UpdateWithRetryRead`. `SaveGenerated` and
`SaveFailed` SHALL return `ErrRecordGone` for an absent key and `ErrSupersededRevision` when the stored record has a
newer source revision, writing nothing; `SaveFailed` SHALL not replace a generated record at the same revision.
`SavePendingGuarded` SHALL not replace a generated record at the same or a newer revision made under the running
embedder's identity (the dedup key's `EmbedderIdentity`: registered type, model, width and text cap, the width
resolved); any other generated record is replaced, so it is regenerated. The vector cache and `similar` SHALL use only
vectors made under the running identity, read when the request is answered, and none while its width is unresolved.

#### Scenario: Any sequence of writes

- **WHEN** a generated sequence of pending, generated, failed and delete calls over two entities runs in order
- **THEN** every call's result and each entity's stored status and revision equal the model's, including an older
  pending record replacing a newer pending one
- **AND** the test that verifies this is `TestRevisionGuardHistoryMatchesModel`

#### Scenario: Retries exhausted

- **WHEN** a save's compare-and-set retries are exhausted
- **THEN** the error matches `natsclient.ErrKVMaxRetriesExceeded`, the record stays pending, and no terminal
  outcome is reported
- **AND** the test that verifies this is `TestCASExhaustionIsNotTerminal`

#### Scenario: A restart under a different embedder

- **WHEN** graph-embedding restarts on the same state with an embedder of another registered type, model, or width,
  including a width the embedder reports only after its first response
- **THEN** every stored vector of an entity in `ENTITY_STATES` is regenerated under the new embedder before readiness,
  a vector the old embedder made is never a search hit, a `similar` source or a `similar` hit, and a restart with the
  same embedder keeps every stored vector when its width is known at startup
- **AND** the tests that verify this are `TestProviderSwitchRestart`, `TestVectorCacheKeepsOnlyItsEmbeddersVectors`
  and `TestSavePendingGuarded_Decisions`
