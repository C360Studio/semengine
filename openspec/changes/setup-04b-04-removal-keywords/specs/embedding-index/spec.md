# embedding-index Delta

## MODIFIED Requirements

### Requirement: A save older than the stored record is refused

`Storage` SHALL write every read-modify-write through `natsclient.KVStore.UpdateWithRetryRead`. `SaveGenerated` and
`SaveFailed` SHALL return `ErrRecordGone` for an absent key and `ErrSupersededRevision` when the stored record has a
newer source revision, writing nothing; `SaveFailed` SHALL not replace a generated record at the same revision.
`SavePendingGuarded` SHALL not replace a generated record at the same or a newer revision that the running embedder
made (same model, and same width when the embedder reports one); a generated record another embedder made is
replaced, so it is regenerated, and the vector cache never scores it.

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

- **WHEN** graph-embedding restarts on the same state with an embedder of another model
- **THEN** every stored vector is regenerated under the new model before readiness, a vector of the old model is
  never a search hit or a `similar` source, and a restart with the same embedder keeps every stored vector
- **AND** the tests that verify this are `TestProviderSwitchRestart`, `TestVectorCacheKeepsOnlyItsEmbeddersVectors`
  and `TestSavePendingGuarded_Decisions`
