# graph/embedding

Embeds the text of graph entities as vectors, stores them in NATS KV, and finds the entities
nearest to a query vector. It has no embedder of its own: a product supplies one.

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
| `TruncationMarker` | Optional interface an `Embedder` also implements when its provider says which texts it cut to fit the model's input window; the Worker then counts each text only the provider cut |
| `Storage` | Reads and writes `EMBEDDING_INDEX` (one record per entity) and `EMBEDDING_DEDUP` (one vector per content key) |
| `VectorCache` | In-memory copy of the generated vectors, kept current by watching `EMBEDDING_INDEX`; `FindSimilar` ranks those made under the running embedder's identity by cosine similarity |
| `Worker` | Hop 2: turns pending records into generated or failed ones, and calls `GeneratedCallback` and `TerminalCallback` |
| `DedupKey`, `EmbedderIdentity` | The content key under which a vector is shared, which includes the embedder's type, model, width and text cap |

## Writes

Every read-modify-write in `Storage` is a compare-and-set through
`natsclient.KVStore.UpdateWithRetryRead`: it decides from the bytes and revision it read, and
retries when another writer got there first. When the retries run out the error matches
`natsclient.ErrKVMaxRetriesExceeded` and nothing was written. The Worker leaves such a record
pending and reports no terminal outcome.

Saves are ordered by source revision:

- `SavePendingGuarded` writes unless the stored record was generated under the running embedder's
  identity (the `EmbedderIdentity` the caller passes: registered type, model, width and text cap, the
  same identity the dedup key folds in; `Record.GeneratedBy`) at the same or a newer revision. It
  then writes nothing and returns `saved == false`. An older pending write may replace a newer
  pending or failed record, and any write replaces a record made under another identity, or met
  while the running width is unresolved, so it is regenerated; only a generated record of the
  running identity blocks it.
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
worker := embedding.NewWorker(storage, embedder, logger).WithEmbedderType("my-embedder").WithWorkers(5)
identity := func() embedding.EmbedderIdentity { return embedding.IdentityOf("my-embedder", embedder, 0) }
cache := embedding.NewVectorCache(storage, identity) // scores only vectors made under identity()

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

Each call to the embedder gets a context that ends 30 seconds after the call begins. A call that
fails once that context has ended fails the record `timeout`, whatever error the embedder returned,
and the lane goes on to its next record. An embedder that ignores its context is not bounded.

The cache is a `graphview.View` of `EMBEDDING_INDEX`, rebuilt by each `Run`. `FindSimilar` returns
an error matching `graphview.ErrNotReady` before the view has read the bucket's initial contents,
after the watch was lost, and after `Run` returned; there is no fallback read of the bucket. A record
that cannot be decoded is not scored and does not make the cache unavailable: `FindSimilar` returns
how many such records it skipped, read at the same moment as the vectors it scored, and a later valid
write of that key is scored again. Results are sorted by `ScoredEntityLess` (score, then entity ID)
before the limit applies, and `keep`, when set, filters entity IDs before scoring.

## Failure reasons

A failed record holds one reason from a closed set; the raw error text is kept beside it but is
never a reason. The Worker files an embedder's error in this order, the first match winning:

| Reason | When |
| --- | --- |
| `timeout` | the call's 30-second context had ended |
| `rate_limited` | the error matches `errs.ErrRateLimited` (package `pkg/errs`) |
| `connection_refused` | the text says "connection refused" |
| `timeout` | the text says "timeout" or "deadline exceeded" |
| `provider_unavailable` | `errs.IsTransient` reports the error |
| `dimension_mismatch` | the text says "dimension" |
| `embedder_error` | anything else |

Two more come from outside the embedder: `content_error` when the record's text cannot be read,
and `internal` when the Worker's own read or write of a bucket fails.

`RetryableReason` reports which reasons name a fault that may clear on its own, so that embedding
the same text later can succeed: `connection_refused`, `timeout`, `rate_limited`,
`provider_unavailable` and `internal`. The others say the request itself was refused, or nothing
says it would not be refused again. graph-embedding's repair lane re-drives the entities that
failed with a retryable reason, so they are embedded once the provider answers again, with no
restart.

A product's own embedder gets the retryable reasons for its outages by returning a `pkg/errs`
error: `errs.ErrRateLimited` (wrapped with `%w` or `errs.WrapTransient`) when the provider asks it
to slow down, and `errs.WrapTransient(err, ...)` when the provider failed in a way it may recover
from. The marks are read from the error, never from its text. An embedder that marks nothing
still has refused connections and timeouts filed as retryable; its other outages are
`embedder_error`. `httpembedder` marks a 429 reply with `errs.ErrRateLimited`, and a 500, 502,
503 or 504 reply or a request that got no reply as transient; a certificate the client does not
trust, any other status and any reply it cannot trust stay unmarked.

The text checks read the whole message, a transient error's included, so an action word given to
`errs.WrapTransient` must not contain "timeout", "connection refused" or "dimension". A 504
"Gateway Timeout" reply is filed `timeout` by its text, which is retryable too.

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
order this package relies on is shown at the module's nats.go version by `TestWorkerRunLeavesNoWatchDelivery`.

## Not here

- No embedder in this package, and no import of an embedding provider; a product supplies the `Embedder`. For a
  provider that serves the OpenAI-shaped embeddings request over HTTP, such as semembed, use
  [`httpembedder`](httpembedder/httpembedder.go), a package of its own.
- No metrics library. `WorkerMetrics` is an interface; the component registers the collectors.
- The package creates neither bucket.

The admission ledger row for `graph/embedding` lists what changed from SemStreams.
