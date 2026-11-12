# Consumer migration: `processor/graph-embedding`

For developers of projects that ran SemStreams' graph-embedding and move to SemEngine's. It says what you change at
your cutover and what you can leave alone. The port breaks old shapes where that gives a better result
(owner, 2026-11-02). Cites to sister repositories are at SemSource `e4febc0d`. Nothing here was run against
your code: it comes from reading it.

graph-embedding is ported in four pull requests. This file covers the first two: the write path (the component that
follows `ENTITY_STATES` and keeps one BM25 embedding record per entity with text) and the query verbs
(`graph.embedding.query.search` and `.similar`). `pkg/fusion` arrives in the third and fourth.

## Changes that stop boot (loud)

Each of these fails when the component is created, naming the key or value. Fix the configuration before the first
start.

- **`batch_size` is refused.** In SemStreams nothing read it. SemSource's composition sets it under
  graph-embedding. *Do:* delete the line.
- **Any unknown key is refused**, `cache_ttl` included (SemStreams refused that one by name; the rest it dropped
  silently). *Do:* delete the key. The kept keys are `ports`, `embedder_type`, `workers`, `max_text_len`,
  `text_suffixes`, `startup_attempts`, `startup_interval_ms` and `coalesce_ms`.
- **`embedder_type: "http"` is refused** at tier 0, with an error naming the value and the tier. Only `bm25` works
  (an empty value means `bm25`). The HTTP embedder and the model registry it needed are not ported; they belong
  to tier 1. *Do:* use `bm25` or leave the key out. The registration no longer asks boot for a model registry.
- **Ports are checked.** An input other than a `kv-watch` of every `ENTITY_STATES` key with no history, or a
  `store-read`, is refused naming the port, and one such watch is required. Any declared output is refused: the
  component writes `EMBEDDING_INDEX` and `EMBEDDING_DEDUP` itself. A `ports` block that is present is used as
  written; SemStreams filled in an empty input list with the entity watch, this does not. *Do:* leave `ports` out to
  take the defaults, or declare the watch yourself.

## Changes that do not stop boot (silent)

Read these. Nothing errors.

- - **The status card loses `first_failure_at`.** The readiness record on `GRAPH_STATUS` under `graph-embedding` no
  longer has that field, and `graph.IndexStatusResponse` no longer has the Go field `FirstFailureAt` (owner ruling).
  SemSource's MCP gateway passes this entry through to agents (`processor/mcp-gateway/tools.go:142`), so its status
  card stops showing the time. *Do:* stop reading the field; a decoder that requires it fails, one that ignores a
  missing key does not notice. `failed_count` and `failed_reasons` stay: the reasons are the per-reason counts of the
  entities currently failed, as before.
- **The status verb is gone.** `graph.embedding.query.status` has no responder and is not declared in
  `graph.QueryVerbs`; a request to it gets "no responders". Readiness is the `GRAPH_STATUS` record (read the key
  `graph-embedding`); the second pull request is to carry "not ready" on the embedding replies themselves
  (ADR-083, ADR-109), and the query verbs now do. *Do:* read `GRAPH_STATUS`, as SemSource's gateway already does.
  Nothing in SemSource's tree
  calls the status verb (grep, 0 hits at `e4febc0d`).
- - **When the status reads `degraded`.** The follower now reads degraded when the component is behind and no revision
  has completed for 30 seconds. Before, graph-embedding waited 120 seconds on the same completion rule, and
  graph-index waited 30 seconds for the lowest unfinished revision to move (owner ruling). Effects: graph-embedding
  reaches `degraded` sooner when it stalls; graph-index no longer reads `degraded` when one revision is held while
  later ones complete, only not ready with `staleness_ms` rising. An alert on `degraded` for graph-index fires less
  often; one on graph-embedding fires after 30 seconds, not 120. *Do:* check any alert threshold built on the old
  timing.
- **The error count now rises, and `Healthy` ignores it.** In SemStreams `error_count` and `error_rate` were always 0:
  nothing wrote the count, though `Healthy` would have dropped had it risen. Here the count (`ErrorCount`) rises by one
  when an entity becomes failed (it was not failed before) and by one per latched reset (a stored `ENTITY_STATES`
  value the component cannot decode; `Health` then reads "graph state reset required" until the process restarts);
  a repeated failure of an entity already failed adds nothing.
  `Healthy` is false only while a stuck state holds: a latched reset, unresolved failed entities, a lost
  `ENTITY_STATES` feed, or a lost embedding watch (ADR-109 Decision 2). An alert on `error_count > 0` or
  `error_rate > 0` never fired before and will fire now. *Do:* read the failed count from the readiness record, not
  from the error count, and review any alert on the error count.
- **A lost embedding watch stays lost.** If the Worker's watch on `EMBEDDING_INDEX` ends while the component runs,
  `Health` names it and stays unhealthy until the process restarts. The pin had no such state. *Do:* alert on
  unhealthy components and restart. The same holds for the vector cache's watch, which is new here: `Health` reads
  `vector cache watch lost; restart the process`, and both query verbs are refused `index_not_ready` until you do.
- **Queries are refused while the graph state is not ready.** The pin answered from a bucket scan when its cache was
  cold. Here a verb is refused transient `index_not_ready` before the first build completes, while the
  `ENTITY_STATES` feed is lost, and while a hop-1 failure stands (a failed record delete, pending write or entity
  read); a latched reset is refused fatal `graph_state_reset_required`. A failure in the Worker (for example a
  document whose text cannot be extracted) does not refuse anything: the other vectors answer and `failed_records`
  counts the entity. The published status still reads `degraded` for it. *Do:* retry on `index_not_ready`.

## Query verbs: what changes in a request or reply

Applies to `graph.embedding.query.search` and `.similar`, and so to graph-query's `semantic` and `similar`, which
forward the bytes unchanged. Each change below fails or alters an answer without a boot error.

- **The reply is enveloped.** The pin sent a bare object. Now `data.results` holds the hits inside a
  `graph.QueryResponse`; `search` also sends `data.embedder_type`. *Do:* read `data.results`.
- **`similar`'s hit list is renamed.** `similar` (the field holding the hits) is now `results`, and the echoed
  `entity_id` and the `duration` are gone.
- **Limits are refused, not clamped.** In SemStreams a `limit` of 0 or below meant 10 and one above 100 was cut to 100.
  Now 0 still means 10, but a negative limit or one above 100 is refused `invalid_request`, naming the bound.
  *Do:* send a limit from 0 to 100 (0 means 10).
- **Unknown keys are refused.** The pin ignored them, so a typo or an old key went unnoticed; now the request is
  refused `invalid_request`, naming the key, and so is data after the JSON object. This changes ADR-071's compatibility
  note that an unmigrated server ignores `scope`. *Do:* send only `query`, `limit`, `scope` and `min_revision` to
  `search`, and `entity_id`, `limit` and `min_revision` to `similar`.
- **A bad `scope` or entity ID is `invalid_request`.** The pin sent the inner code `entity_id_prefix_invalid` for a
  bad scope prefix; the code is now `invalid_request`, with the validator's message. *Do:* match on
  `invalid_request`.
- **`min_revision` above 0 is refused.** These verbs claim no `ENTITY_STATES` revision, so they cannot promise your
  write is included, and refuse the request class invalid, `invalid_request`, rather than answer without it. 0 is
  answered, with `indexed_revision` 0. graph-query's `semantic` and `similar` forward `min_revision` to them, so the
  refusal reaches you through graph-query. *Do:* do not ask for a revision on these verbs.
- **A hit with a score of 0 is dropped.** A query of only stopwords used to return a list of unrelated entities; it
  now returns `results: []`.
- **Two counts are always sent.** `unreadable_records` and `failed_records` say how many records could not be
  decoded and how many entities failed to embed. Both 0 does not prove every entity is searchable: an entity whose
  embedding is still pending is counted in neither.
- **No bucket-scan fallback.** The pin scanned `EMBEDDING_INDEX` when its cache was cold, which hid a defect: a
  scan could answer from the bucket while the cache held old data, so the two paths disagreed; the verbs are
  refused `index_not_ready` instead until the cache is ready.

## Not carried

- The HTTP embedder and the vector providers behind it.
- The indexing-profile check: in SemStreams every path returned true, and the ADR behind it (ADR-054) is not ported.
- The pin's own watch, reset latch, status bucket, heartbeat and `readiness.go`: `graph/follower` does these for
  graph-index and graph-embedding alike.
