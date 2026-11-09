# embedding-index

## MODIFIED Requirements

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

`VectorCache.Run(ctx)` SHALL refuse a nil context and a second concurrent `Run`, and keep a new `graphview.View` of
`EMBEDDING_INDEX` per `Run`. `FindSimilar` SHALL return an error matching `graphview.ErrNotReady` before the snapshot,
after a lost watch and after `Run` returns. Otherwise it SHALL rank by `ScoredEntityLess` before the limit and return
the count of records it could not decode. Such a record SHALL NOT withdraw the cache; a later valid write is scored.

#### Scenario: Not authoritative

- **WHEN** `FindSimilar` is called before the snapshot, after a lost watch, or after `Run` returned
- **THEN** it returns an error matching `graphview.ErrNotReady`
- **AND** the test that verifies this is `TestVectorCacheNotAuthoritative`

#### Scenario: An unreadable record heals

- **WHEN** one record fails to decode, and later a valid generated record is written under the same key
- **THEN** the first `FindSimilar` scores the other vectors and counts 1, and after the write it scores that key and
  counts 0
- **AND** the test that verifies this is `TestVectorCachePoisonCountedAndHeals`

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
