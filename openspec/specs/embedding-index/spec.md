# embedding-index Specification

## Purpose
`graph/embedding` turns entity text into vectors with an embedder the caller supplies and keeps them in two NATS
KV buckets: EMBEDDING_INDEX, one record per entity, and EMBEDDING_DEDUP, one vector per content key. This spec states
how `Storage` orders and validates its writes, how the `Worker` and `VectorCache` run and stop, and the per-entity
order of the Worker's callbacks. The component that wires them into a flow is a later change.

## Requirements

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
end the watch's context, SHALL return a non-nil error that is not the context's error, and SHALL return only after
the watch's delivery has ended. nats.go's unsubscribe goroutine for the watch is released by that context, not joined.

#### Scenario: The update channel closes

- **WHEN** the `EMBEDDING_INDEX` bucket is deleted while the Worker's `Run` has a live context
- **THEN** `Run` returns an error saying the watch was lost, no delivery for the watch is running, and the
  unsubscribe goroutine exits afterwards
- **AND** the test that verifies this is `TestWorkerRunReportsLostWatch`

#### Scenario: The cache's update channel closes

- **WHEN** the `EMBEDDING_INDEX` bucket is deleted while `VectorCache.Run` has a live context
- **THEN** `Run` returns an error matching `graphview.ErrWatcherLost`, and `FindSimilar` returns an error matching
  `graphview.ErrNotReady`
- **AND** the test that verifies this is `TestVectorCacheRunReportsLostWatch`

### Requirement: The vector cache answers only while authoritative

`VectorCache.Run(ctx)` SHALL refuse a nil context and a second `Run` while one runs, SHALL keep the cache on a new
`graphview.View` of `EMBEDDING_INDEX` for each `Run`, and SHALL stop as `Worker.Run` does. `FindSimilar` SHALL return
an error matching `graphview.ErrNotReady` before the watch's initial snapshot is applied, after the watch is lost, and
after `Run` returns.

#### Scenario: Not authoritative

- **WHEN** `FindSimilar` is called before the snapshot, after a lost watch, or after `Run` returned
- **THEN** it returns an error matching `graphview.ErrNotReady`
- **AND** the test that verifies this is `TestVectorCacheNotAuthoritative`

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

### Requirement: The vector cache scores and counts one moment

`VectorCache.FindSimilar` SHALL score only the generated vectors whose entity IDs `keep` accepts, sort by
`ScoredEntityLess` before applying the limit, and return no entities for a limit of 0 or less. It SHALL return, read at
the moment it scored, the number of records it could not decode whose keys `keep` accepts (all of them when `keep` is
nil). Such a record SHALL NOT be scored and SHALL NOT withdraw the cache; a later valid write of that key SHALL be
scored and no longer counted.

#### Scenario: An unreadable record heals

- **WHEN** one record fails to decode, and later a valid generated record is written under the same key
- **THEN** the first `FindSimilar` scores the other vectors and counts 1, and after the write it scores that key and
  counts 0
- **AND** the test that verifies this is `TestVectorCachePoisonCountedAndHeals`

#### Scenario: Order and limit

- **WHEN** several cached vectors tie on similarity across the limit, and when the limit is 0 or negative
- **THEN** the result is ordered by similarity then entity ID, and a non-positive limit returns no entities
- **AND** the test that verifies this is `TestFindSimilarOrderAndLimit`

#### Scenario: One moment

- **WHEN** a delete and an undecodable write are applied after `FindSimilar` read the view and before it returns
- **THEN** its answer and count do not include them, and the next call does
- **AND** the test that verifies this is `TestFindSimilarReadsOneMoment`

#### Scenario: Only generated vectors under `keep`

- **WHEN** `keep` accepts some entity IDs and a pending or failed record exists under an accepted ID
- **THEN** only the generated vectors of accepted IDs are scored, and an accepted lower score beats a rejected higher
  one at limit 1
- **AND** the test that verifies this is `TestFindSimilarFromCache_ScopeFilter`

#### Scenario: The count follows keep

- **WHEN** two records fail to decode, one under a key `keep` accepts and one outside it
- **THEN** `FindSimilar` with that `keep` counts 1, and with a nil `keep` counts 2
- **AND** the test that verifies this is `TestFindSimilarCountFollowsKeep`

### Requirement: A save older than the stored record is refused

`Storage` SHALL write every read-modify-write through `natsclient.KVStore.UpdateWithRetryRead`. `SaveGenerated` and
`SaveFailed` SHALL return `ErrRecordGone` for an absent key and `ErrSupersededRevision` when the stored record has a
newer source revision, writing nothing; `SaveFailed` SHALL not replace a generated record at the same revision.

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

### Requirement: Only the running embedder's vectors are used

`SavePendingGuarded` SHALL not replace a generated record at the same or a newer revision made under the running
embedder's identity (the dedup key's `EmbedderIdentity`: registered type, model, width and text cap, the width
resolved); any other generated record is replaced, so it is regenerated. The vector cache and `similar` SHALL use only
vectors made under the running identity, read when the request is answered, and none while its width is unresolved.

#### Scenario: A restart under a different embedder

- **WHEN** graph-embedding restarts on the same state with an embedder of another registered type, model, or width,
  including a width the embedder reports only after its first response
- **THEN** every stored vector of an entity in `ENTITY_STATES` is regenerated under the new embedder before readiness,
  a vector the old embedder made is never a search hit, a `similar` source or a `similar` hit, and a restart with the
  same embedder keeps every stored vector when its width is known at startup
- **AND** the tests that verify this are `TestProviderSwitchRestart`, `TestVectorCacheKeepsOnlyItsEmbeddersVectors`
  and `TestSavePendingGuarded_Decisions`

### Requirement: One entity's work runs in order through its callbacks

The Worker SHALL process one entity's watch entries one at a time, in the order the watch delivered them, and SHALL
finish each entry's store write and its terminal and generated callbacks before it starts the entity's next entry.
Different entities MAY run in parallel.

#### Scenario: A failure followed by a success

- **WHEN** revision N's failure is stored and its terminal callback is held until pending M is in the bucket
- **THEN** the terminal callbacks arrive as Failed(N) then Generated(M), and M's processing starts only after N's
  callback returns
- **AND** the test that verifies this is `TestKeyedCallbacksFollowRevisionOrder`

### Requirement: A save refused for a newer pending record is dropped

When a Worker's save for a source revision is refused because hop 1 stored a newer revision's pending record, the
Worker SHALL store nothing, SHALL NOT call the generated callback for that save, and SHALL report the outcome as
deleted; the newer record SHALL then be processed.

#### Scenario: Hop 1 writes M while N is embedding

- **WHEN** revision N's embedding is held until pending M (M > N) is in the bucket, then released
- **THEN** N's outcome is deleted with no generated callback, and the record ends generated at M
- **AND** the test that verifies this is `TestWorkerSaveRefusedWhenNewerPendingArrives`

### Requirement: Removing a record with no text is fenced

When a record's resolved text is empty, the Worker SHALL remove the record only if its key is still at the KV
revision the Worker read; when the key has moved or is gone it SHALL remove nothing and report the outcome as
deleted; when the removal fails otherwise it SHALL report no terminal outcome and leave the record pending.

#### Scenario: A newer pending record arrived

- **WHEN** N's empty body fetch is held until pending M is in the bucket, then released
- **THEN** nothing is removed, N's outcome is deleted, and the record is still pending at M
- **AND** the test that verifies this is `TestNoTextDeleteKeepsNewerPending`

#### Scenario: A newer generated record arrived

- **WHEN** N's empty body fetch is held until M's generated record is in the bucket, then released
- **THEN** nothing is removed, N's outcome is deleted, and the record is still generated at M with M's vector
- **AND** the test that verifies this is `TestNoTextDeleteKeepsNewerGenerated`

#### Scenario: The removal fails

- **WHEN** the fenced removal fails with an error that is neither a moved key nor a missing one
- **THEN** no terminal callback fires and the record stays pending
- **AND** the test that verifies this is `TestNoTextDeleteFailureIsNotTerminal`

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

### Requirement: The HTTP embedder sends one request per call

The HTTP embedder SHALL send each `Generate` or `GenerateQuery` call as one `POST` to `<base URL>/embeddings` with the
configured model and the texts, sending an `Authorization` header only when an API key is configured, and SHALL send
nothing for no texts. It SHALL import neither the OpenAI SDK nor the module's `model` package.

#### Scenario: Request shape

- **WHEN** two texts are embedded with no API key configured
- **THEN** one `POST` to `<base URL>/embeddings` carries the model and both texts and no `Authorization` header
  (`TestRequestShape`)

### Requirement: The HTTP embedder trusts only a whole reply

The HTTP embedder SHALL return one vector per text, placed by the reply's `index`. It SHALL return an error and no
vectors when the reply's status is not 2xx, its count differs from the texts', an `index` is out of range or repeated,
a vector is empty or of another width than it was configured with or learned, or the reply names another model.

#### Scenario: Out-of-order reply

- **WHEN** the provider replies with `index` 1 before `index` 0
- **THEN** each vector is returned at its text's position (`TestVectorsPlacedByIndex`)

#### Scenario: An untrusted reply

- **WHEN** the reply has a wrong count, a repeated index, an empty vector, a second width, or another model
- **THEN** the call returns an error and no vectors (`TestRefusesUntrustedReply`)

### Requirement: The HTTP embedder can be given its width

With `Config.Dimensions` set, `Dimensions` SHALL report that width from construction and `New` SHALL send no request.
With it 0, `Dimensions` SHALL be 0 until the first accepted reply and that reply's width after it. `New` SHALL refuse
a negative `Config.Dimensions`.

#### Scenario: A declared width

- **WHEN** the embedder is built with `Config.Dimensions` 3
- **THEN** `Dimensions` is 3 before any request, a reply of width 2 is refused with no vectors, and `Dimensions`
  stays 3 (`TestDeclaredWidth`)

### Requirement: Only query text carries the query prefix

`GenerateQuery` SHALL send `"input_type": "query"` and `Generate` SHALL send `"input_type": "document"` in every
request body, unless `Config.OmitInputType` is true, when neither SHALL send the field. `GenerateQuery` SHALL prepend
the configured query prefix to each text, and `Generate` SHALL never prepend it; with no prefix configured the two
SHALL send the same texts.

#### Scenario: Asymmetric model

- **WHEN** a prefix is configured and the same text goes through both methods
- **THEN** only the query request carries the prefix (`TestHTTPEmbedder_GenerateQuery_PrependsPrefix`)

#### Scenario: The request names its input type

- **WHEN** a text goes through `GenerateQuery` and then `Generate`
- **THEN** the query request's body carries `"input_type": "query"` and the document request's `"input_type":
  "document"`, and with `OmitInputType` true neither body has the field (`TestInputTypeNamesTheText`)

### Requirement: An unavailable provider is named

The Worker SHALL file a generation error as `timeout` when its call's deadline passed; else as `rate_limited` when it
matches `errs.ErrRateLimited`; else `connection_refused` or `timeout` by its text as today; else as
`provider_unavailable` when `errs.IsTransient` reports it; else `dimension_mismatch` or `embedder_error` as today.
`embedding.RetryableReason` SHALL accept exactly `connection_refused`, `timeout`, `rate_limited`,
`provider_unavailable` and `internal`.

#### Scenario: Reasons

- **WHEN** a rate-limited error, a transient error, a refused connection marked transient, a plain error, and each
  stored reason are classified
- **THEN** they are `rate_limited`, `provider_unavailable`, `connection_refused`, `embedder_error`, and only the five
  are retryable (`TestClassifyEmbedErr`, `TestRetryableReason`)

### Requirement: The HTTP embedder marks a busy or failing provider

The HTTP embedder SHALL return, for a 429 reply, an error matching `errs.ErrRateLimited`, without waiting on
`Retry-After` or sending again; and, for a 500, 502, 503 or 504 reply or a request that got no reply, an error
`errs.IsTransient` reports. A TLS certificate failure, any other non-2xx reply and any untrusted reply SHALL NOT be
transient.

#### Scenario: Status classes

- **WHEN** the provider answers 429 with `Retry-After`, 503, 400 or 501, is closed, or presents an untrusted
  certificate
- **THEN** the errors are rate-limited after one request, transient, not transient, not transient, transient, and
  not transient (`TestProviderUnavailableIsMarked`)
