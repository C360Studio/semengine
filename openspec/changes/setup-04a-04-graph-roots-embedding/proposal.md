# Proposal: setup-04a-04-graph-roots-embedding

## Why

Change 4 (`setup-04a-04-graph-roots`) of epic #9 ports the graph roots one package per pull request, in the order the
owner set on 2026-10-24 (#9 comment 6386856657). Item 5 is "`graph/embedding` without `http_embedder.go` (#35);
second-lane capable", and item 6, `graph/query`, imports this package, so it lands first. The package turns entity
text into vectors (BM25 in process at tier 0), stores them in the `EMBEDDING_INDEX` and `EMBEDDING_DEDUP` buckets,
and runs the worker that generates them; `processor/graph-embedding` (change 5) composes it. The owner's bar: "graph
is in many ways our secret sauce. it needs to be solid… i would rather not bring in a pkg that causes instant debt".
Tier 0 only (#9 comment 6386603659): the provider file `http_embedder.go` stays out.

## What Changes

- Port `graph/embedding` without `http_embedder.go`: the `Embedder` interface, `BM25Embedder`, the dedup key,
  `CosineSimilarity`, `Storage`, the `Worker`, and the failure-reason constants.
- `http_embedder.go` and its two test files are not carried: a `defer-exclude` ledger row naming #32, which keeps the
  tier-1 → tier-0 fallback test for Slice 04C (owner ruling, PR #458 comment 6473224284, item 1).
- The Worker's `Start`/`Stop` become `Run(ctx)`, and the vector cache moves from `Storage.StartVectorCache` to a
  `VectorCache` type with its own `Run(ctx)`, so each stops when its context ends or its watch is lost and returns
  only after every goroutine it started, and nats.go's delivery for its watch, has ended (`background-work` spec;
  ADR-113 for the component that runs them). Both stop the watch through the follower's bounded drain, moved
  unchanged into `internal/kvwatch` so there is one spelling (#454's bound included).
- The Worker runs on `internal/dispatch.KeyedPool`, keyed by entity ID, so one entity's entries, their store writes
  and their callbacks run one at a time in the order the watch delivered them (#107).
- A record whose text is empty is removed only if its key is still at the revision the Worker read, through the
  existing `natsclient.KVStore.DeleteAtRevision`; at the pin an older revision could delete a newer revision's record.
- `Storage` writes through `natsclient.KVStore.UpdateWithRetryRead` instead of its three hand-written retry loops and
  its unconditional dedup overwrite (ADR-110 Decision 2 and 7).
- Dead surface is not carried: `ContentHash`, `SavePending`, `SavePendingWithStorageRef`, `Record.ContentFields`,
  `DedupRecord.EntityIDs`, `WorkerMetrics.SetPending`, the unused vector helpers, and `ErrCASExhausted` (replaced by
  `natsclient.ErrKVMaxRetriesExceeded`).
- The follower test's helpers that see nats.go's watch delivery move into `internal/harness/natsfixture`, shared by
  both packages' tests. One adoption issue lists the other code that stops a KV watch.
- Docs say only what ships: no HTTP embedder, the BM25 `B` default, one text-length default for both lanes.

## Capabilities

- New `embedding-index`.

## Impact

- New `graph/embedding`: 3,082 pin lines considered (3,302 less `http_embedder.go`), under the owner's waiver for this
  package at 3,302 (#9 comment 6386556850, ruling 1); one pull request.
- New import edge `graph/embedding` → `natsclient` (ADR-110). `golang.org/x/sync` becomes a direct dependency.
  `go-openai` and `model` are not imported.
- `graph/follower` calls `internal/kvwatch` instead of its own copy; its behaviour and spec are unchanged.
- No new subject, bucket, stream or config key; both buckets are already in `graph/kvcatalog`. New exported surface
  beyond the pin's: `Worker.Run`, `VectorCache` with `NewVectorCache`, `Run` and `FindSimilar` (design D4, D5), each
  replacing a pin method that `processor/graph-embedding` calls today.
- Consumers: none imports the package (inventory 4). `processor/graph-embedding` (change 5) and `graph/query` (next
  pull request) are edited when they port.
- Retires #35 (owner ruling, PR #458 comment 6473224284, item 1) and #107 by doing its work: the Worker adopts the
  keyed pool, the ruling's fallback (same ruling, item 2), after Codex's review showed the storage guard alone does
  not order the terminal callbacks (PR #458 comment 6474241300). Advances #141 (this package's sites) and #195 (this
  package's site); both stay open for other packages.
  Stays open: #32, #96, #109, #155, #10, #11.
