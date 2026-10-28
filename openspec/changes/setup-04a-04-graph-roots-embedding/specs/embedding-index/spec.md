# embedding-index

## ADDED Requirements

### Requirement: The Worker runs until its context ends

`embedding.Worker.Run(ctx)` SHALL refuse a nil context before starting anything, SHALL process pending records from
one watch of `EMBEDDING_INDEX` until `ctx` ends, and SHALL then return `ctx.Err()` only after every goroutine it
started has exited and the watch's delivery has closed, or with an error when that delivery did not close within 5 s.
A second `Run` on the same Worker while one runs SHALL be refused.

#### Scenario: Started, used and stopped

- **WHEN** `Run` is started inside `synctest.Test`, processes a pending record, and its context is cancelled
- **THEN** `Run` returns `context.Canceled` and the bubble ends with no goroutine left
- **AND** the test that verifies this is `TestWorkerRunLeavesNothingBehind`

#### Scenario: Stopped during a backlogged snapshot

- **WHEN** `Run`'s context is cancelled while nats.go's watch delivery is blocked on a full updates buffer
- **THEN** `Run` returns only after that delivery has ended, and no goroutine is left in a Worker method
- **AND** the test that verifies this is `TestWorkerRunLeavesNoWatchDelivery`

#### Scenario: A nil context

- **WHEN** `Run` is called with a nil context
- **THEN** it returns an invalid-input error and starts nothing
- **AND** the test that verifies this is `TestWorkerRunRefusesNilContext`

#### Scenario: A second Run

- **WHEN** `Run` is called while another `Run` on the same Worker is running
- **THEN** the second call returns an error and the first keeps running
- **AND** the test that verifies this is `TestWorkerSecondRunRefused`

### Requirement: A lost watch is reported

When the watch's update channel closes while the context is still live, `Worker.Run` and `VectorCache.Run` SHALL
end the watch's context, SHALL return a non-nil error that is not the context's error, and SHALL leave no goroutine
running for the watch, nats.go's included.

#### Scenario: The update channel closes

- **WHEN** the `EMBEDDING_INDEX` bucket is deleted while the Worker's `Run` has a live context
- **THEN** `Run` returns an error saying the watch was lost
- **AND** the test that verifies this is `TestWorkerRunReportsLostWatch`

#### Scenario: The cache's update channel closes

- **WHEN** the `EMBEDDING_INDEX` bucket is deleted while `VectorCache.Run` has a live context
- **THEN** `Run` returns an error saying the watch was lost, and `FindSimilar` returns `false`
- **AND** the test that verifies this is `TestVectorCacheRunReportsLostWatch`

### Requirement: The vector cache answers only while authoritative

`VectorCache.Run(ctx)` SHALL refuse a nil context and a second `Run` while one runs, and SHALL stop as `Worker.Run`
does. `VectorCache.FindSimilar` SHALL return `false` as its second result before the watch's initial snapshot is
applied, after any record fails to decode, after the watch is lost, and after `Run` returns. Otherwise it SHALL
score only the entity IDs `keep` accepts, sort by `ScoredEntityLess` before applying the limit, and return no
entities for a limit of 0 or less.

#### Scenario: Not authoritative

- **WHEN** `FindSimilar` is called before the snapshot, after an undecodable record, or after `Run` returned
- **THEN** it returns `false`
- **AND** the test that verifies this is `TestVectorCacheNotAuthoritative`

#### Scenario: Order and limit

- **WHEN** several cached vectors tie on similarity across the limit, and when the limit is 0 or negative
- **THEN** the result is ordered by similarity then entity ID, and a non-positive limit returns no entities
- **AND** the test that verifies this is `TestFindSimilarOrderAndLimit`

#### Scenario: Stopped inside a bubble

- **WHEN** `VectorCache.Run` is started, queried and cancelled inside `synctest.Test`
- **THEN** it returns `context.Canceled` and the bubble ends with no goroutine left
- **AND** the test that verifies this is `TestVectorCacheRunLeavesNothingBehind`

#### Scenario: Stopped with a backlogged watch

- **WHEN** `VectorCache.Run`'s context is cancelled while nats.go's watch delivery is blocked on a full buffer
- **THEN** `Run` returns only after that delivery has ended
- **AND** the test that verifies this is `TestVectorCacheRunLeavesNoWatchDelivery`

#### Scenario: A second cache Run

- **WHEN** `VectorCache.Run` is called while another `Run` on the same cache is running
- **THEN** the second call returns an error and the first keeps running
- **AND** the test that verifies this is `TestVectorCacheSecondRunRefused`

### Requirement: An older source revision never replaces a newer outcome

`Storage` SHALL write every read-modify-write through `natsclient.KVStore.UpdateWithRetryRead`. `SaveGenerated` and
`SaveFailed` SHALL return `ErrRecordGone` for an absent key and `ErrSupersededRevision` when the stored record has a
newer source revision, writing nothing; `SaveFailed` SHALL not replace a generated record at the same revision.
`SavePendingGuarded` SHALL not replace a generated record at the same or a newer revision.

#### Scenario: Any sequence of writes

- **WHEN** a generated sequence of pending, generated, failed and delete calls over two entities runs in order
- **THEN** every call's result and each entity's stored status and revision equal the model's, including an older
  pending record replacing a newer pending one
- **AND** the test that verifies this is `TestRevisionGuardHistoryMatchesModel`

#### Scenario: Concurrent saves for one entity

- **WHEN** twenty goroutines each save a generated or failed outcome for a distinct revision of one pending record
- **THEN** the record ends at the highest revision's outcome
- **AND** the test that verifies this is `TestConcurrentSavesEndAtHighestRevision`

#### Scenario: Retries exhausted

- **WHEN** a save's compare-and-set retries are exhausted
- **THEN** the error matches `natsclient.ErrKVMaxRetriesExceeded`, the record stays pending, and no terminal
  outcome is reported
- **AND** the test that verifies this is `TestCASExhaustionIsNotTerminal`

### Requirement: A superseded revision's vector is dropped

When a Worker's save for a source revision is refused because a newer revision's record is stored, the Worker SHALL
store nothing, SHALL NOT call the generated callback for that save, and SHALL report the outcome as deleted. Generated
callbacks for one entity are otherwise unordered.

#### Scenario: The older revision finishes last

- **WHEN** revision N's embedding is held until revision M's generated record (M > N) is stored, then released
- **THEN** the record stays generated at M with M's vector, the generated callback fired for M only, and N's
  terminal outcome is deleted
- **AND** the test that verifies this is `TestWorkerSupersededRevisionIsDropped`

### Requirement: The dedup record keeps its vector space

`Storage.SaveDedup` SHALL leave a stored record of the same model and dimensions unchanged and SHALL replace a record
of another vector space. `DedupKey` SHALL fold the embedder's type, model, dimensions and text cap into the key and
SHALL return no key while the dimensions are unknown.

#### Scenario: Same space and other space

- **WHEN** `SaveDedup` is called for a key holding a same-space record, then for one holding another model's record
- **THEN** the first stored record is unchanged and the second is replaced
- **AND** the test that verifies this is `TestSaveDedupKeepsSameSpaceReplacesOther`

### Requirement: Each terminal outcome is reported once

The Worker SHALL call its terminal callback exactly once for each record it carries to a terminal outcome, SHALL NOT
call it for an entry whose processing panicked, and SHALL count a failure as shutdown only when its error matches
`context.Canceled` and the Worker's context has ended.

#### Scenario: One callback per outcome

- **WHEN** records reach generated, failed, no-text and deleted outcomes
- **THEN** the terminal callback fires once each, with the record's source revision and outcome
- **AND** the test that verifies this is `TestTerminalCallbackOncePerOutcome`

#### Scenario: A panicking entry

- **WHEN** processing one entry panics
- **THEN** the record stays pending, no terminal callback fires for it, and the next entry is processed
- **AND** the test that verifies this is `TestPanickingEntryStaysPending`

#### Scenario: Failure text that mentions cancellation

- **WHEN** an embedder fails with an error whose text contains "context canceled" while the Worker's context is live
- **THEN** the failure is recorded as a failed outcome and counted
- **AND** the test that verifies this is `TestFailureWithCanceledTextIsAFailure`

### Requirement: Both lanes embed the same bytes

For identical content the Worker SHALL embed the same bytes whether the text is inline or fetched through a
`StorageRef`, with a text cap of 0 meaning 8,000 runes on both lanes.

#### Scenario: Inline and offloaded

- **WHEN** the same over-cap text arrives once inline and once by reference, with the cap unset
- **THEN** the embedder receives identical bytes and both records carry the same dedup key
- **AND** the test that verifies this is `TestBothLanesEmbedIdenticalBytes`

### Requirement: Tier 0 imports no provider code

`graph/embedding` SHALL NOT depend, directly or transitively, on `github.com/sashabaranov/go-openai` or on the
module's `model` package.

#### Scenario: Dependency listing

- **WHEN** `go list -deps ./graph/embedding` runs
- **THEN** neither module path appears
- **AND** the test that verifies this is `TestEmbeddingImportsNoProviderModule`
