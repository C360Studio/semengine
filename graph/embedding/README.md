# graph/embedding

Embeds the text of graph entities as vectors, stores them in NATS KV, and finds the entities
nearest to a query vector. It runs without any external embedding provider.

## How embedding runs

Embedding takes two steps, called hops:

1. **Hop 1** writes a *pending* record to the `EMBEDDING_INDEX` bucket for each new revision of an
   entity. The component that does this, `processor/graph-embedding`, is not part of this
   package. It calls `Storage.SavePendingGuarded`.
2. **Hop 2** is the `Worker` in this package. It watches `EMBEDDING_INDEX`, resolves each pending
   record's text, calls an `Embedder`, and stores the result as a *generated* record, or as a
   *failed* record with a short reason.

A *source revision* is the `ENTITY_STATES` revision a record was made from. Storage uses it to
tell older work from newer work.

## What is here

| Name | What it does |
| --- | --- |
| `Embedder` | Interface: `Generate` for documents, `GenerateQuery` for search queries |
| `BM25Embedder` | The tier-0 embedder. Hashes terms into `Dimensions` slots (default 384) and weights them with BM25. It matches shared words, not meaning |
| `Storage` | Reads and writes `EMBEDDING_INDEX` (one record per entity) and `EMBEDDING_DEDUP` (one vector per content key) |
| `VectorCache` | In-memory copy of the generated vectors, kept current by watching `EMBEDDING_INDEX`; `FindSimilar` ranks by cosine similarity |
| `Worker` | Hop 2: turns pending records into generated or failed ones, and calls `GeneratedCallback` and `TerminalCallback` |
| `DedupKey`, `EmbedderIdentity` | The content key under which a vector is shared, which includes the embedder's type, model, width and text cap |

## Writes

Every read-modify-write in `Storage` is a compare-and-set through
`natsclient.KVStore.UpdateWithRetryRead`: it decides from the bytes and revision it read, and
retries when another writer got there first. When the retries run out the error matches
`natsclient.ErrKVMaxRetriesExceeded` and nothing was written. The Worker leaves such a record
pending and reports no terminal outcome.

Saves are ordered by source revision:

- `SavePendingGuarded` writes unless the stored record is generated at the same or a newer
  revision. It then writes nothing and returns `saved == false`. An older pending write may
  replace a newer pending or failed record; only a generated record blocks it.
- `SaveGenerated` and `SaveFailed` return `ErrRecordGone` if the key is absent and
  `ErrSupersededRevision` if the stored record is for a newer revision. `SaveFailed` also returns
  `ErrSupersededRevision` over a generated record at the same revision. Neither error is a
  failure of the entity, and neither fires a generated callback.
- When a record's text resolves to nothing, the Worker removes the record only if its key is
  still at the KV revision the Worker read. If newer work has moved the key, nothing is removed
  and the outcome is `OutcomeDeleted`. If the removal itself fails, the record stays pending and
  no terminal callback fires.

## Running the Worker and the cache

Each `Run` blocks until `ctx` ends or its watch is lost, so a caller that wants both runs each on
its own goroutine and waits for both to return:

```go
worker := embedding.NewWorker(storage, embedder, logger).WithWorkers(5)
cache := embedding.NewVectorCache(storage)

errc := make(chan error, 2)
go func() { errc <- worker.Run(ctx) }()
go func() { errc <- cache.Run(ctx) }()
err := errors.Join(<-errc, <-errc) // both have returned
```

A lost watch ends only the `Run` that lost it; the other keeps running until `ctx` ends, so a caller
that stops on the first error cancels `ctx` before waiting for the second.

`Run` returns `ctx.Err()` when `ctx` ended, and an error saying the watch was lost otherwise. The
caller owns `ctx` and ends it to stop. `Run` refuses a nil context and a second `Run` while one
is running; after it returns it can be called again. A lost watch is an error to act on: report
it or run again, but do not drop it.

Each `Run` of the Worker retries failed records once, because a failed record is processed when
it arrives in the watch's initial contents but not when it is written later.

`FindSimilar` returns `(nil, false)`, and the caller reads `EMBEDDING_INDEX` instead, before the
cache has read the bucket's initial contents, after a record failed to decode, after the watch
was lost, and after `Run` returned. Results are sorted by `ScoredEntityLess` (score, then entity
ID) before the limit applies, and `keep`, when set, filters entity IDs before scoring.

## Ordering

The Worker hands each watch entry to a keyed pool (`internal/dispatch.KeyedPool`) under the entity
ID. For one entity, entries are processed one at a time in the order the watch delivered them,
from reading the record through the store write to both callbacks. Different entities run in
parallel on the `WithWorkers` lanes. Callbacks for different entities are unordered, and nothing
here promises the order in which the watch delivers entries; the revision checks above are what
keep an older save from replacing a newer generated record.

The revision checks order what is stored, not what a caller does with the callbacks. A caller
that keeps its own record of failures must not put back a failure older than the entity's latest
terminal outcome.

## Stopping

On `Run`'s shutdown the Worker stops reading, ends the context it derived from `ctx`, lets the
pool finish the entries it already accepted (under `ctx`, ended or not), joins the pool's lanes,
and stops the watch through `internal/kvwatch.Stop`. A callback that never returns holds `Run`;
the join has no bound of its own. How nats.go times the watch's shutdown is not promised; the
probe behind this package's order is on PR #458, comment 6472856197, at nats.go v1.54.0.

## Not here

- No HTTP embedder, and no import of an embedding provider; a product supplies its own `Embedder`.
- No metrics library. `WorkerMetrics` is an interface; the component registers the collectors.
- The package creates neither bucket.

The admission ledger row for `graph/embedding` lists what changed from SemStreams.
