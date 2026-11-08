# Consumer migration: `processor/graph-embedding`

For developers of projects that ran SemStreams' graph-embedding and move to SemEngine's. It says what you change at
your cutover and what you can leave alone. The port breaks old shapes where that gives a better result
(owner, 2026-11-02). Cites to sister repositories are at SemSource `e4febc0d`. Nothing here was run against
your code: it comes from reading it.

graph-embedding is ported in four pull requests. This file covers the first, the write path: the component that
follows `ENTITY_STATES` and keeps one BM25 embedding record per entity with text. The query verbs
(`graph.embedding.query.*`) arrive in the second pull request, and `pkg/fusion` in the third and fourth. Nothing in
this tree answers `graph.embedding.query.search` or `.similar` yet; this file gets a section for their reply shape
when they land.

## Changes that stop boot (loud)

Each of these fails when the component is created, naming the key or value. Fix the configuration before the first
start.

- **`batch_size` is refused.** At the SemStreams pin nothing read it. SemSource's composition sets it under
  graph-embedding. *Do:* delete the line.
- **Any unknown key is refused**, `cache_ttl` included (the pin refused that one by name; the rest it dropped
  silently). *Do:* delete the key. The kept keys are `ports`, `embedder_type`, `workers`, `max_text_len`,
  `text_suffixes`, `startup_attempts`, `startup_interval_ms` and `coalesce_ms`.
- **`embedder_type: "http"` is refused** at tier 0, with an error naming the value and the tier. Only `bm25` works
  (an empty value means `bm25`). The HTTP embedder and the model registry it needed are not ported; issue #489 owns
  tier 1. *Do:* use `bm25` or leave the key out. The registration no longer asks boot for a model registry.
- **Ports are checked.** An input other than a `kv-watch` of every `ENTITY_STATES` key with no history, or a
  `store-read`, is refused naming the port, and one such watch is required. Any declared output is refused: the
  component writes `EMBEDDING_INDEX` and `EMBEDDING_DEDUP` itself. A `ports` block that is present is used as
  written; the pin filled in an empty input list with the entity watch, this does not. *Do:* leave `ports` out to
  take the defaults, or declare the watch yourself.

## Changes that do not stop boot (silent)

Read these. Nothing errors.

- **The status card loses `first_failure_at`.** The readiness record on `GRAPH_STATUS` under `graph-embedding` no
  longer has that field, and `graph.IndexStatusResponse` no longer has the Go field `FirstFailureAt` (owner, PR #550 comment
  6716972354). SemSource's MCP gateway passes this entry through to agents (`processor/mcp-gateway/tools.go:142`),
  so its status card stops showing the time. *Do:* stop reading the field; a decoder that requires it fails, one
  that ignores a missing key does not notice. `failed_count` and `failed_reasons` stay: the reasons are the
  per-reason counts of the entities currently failed, as before.
- **The status verb is gone.** `graph.embedding.query.status` has no responder and is not declared in
  `graph.QueryVerbs`; a request to it gets "no responders". Readiness is the `GRAPH_STATUS` record (read the key
  `graph-embedding`); the second pull request is to carry "not ready" on the embedding replies themselves
  (ADR-083, ADR-109), and is not in the tree yet. *Do:* read `GRAPH_STATUS`, as SemSource's gateway already does.
  Nothing in SemSource's tree
  calls the status verb (grep, 0 hits at `e4febc0d`).
- **When the status reads `degraded`.** The follower now reads degraded when the component is behind and no
  revision has completed for 30 seconds. Before, graph-embedding waited 120 seconds on the same completion rule, and
  graph-index waited 30 seconds for the lowest unfinished revision to move (owner ruling (a1), PR #550 comment
  6721867279). Effects: graph-embedding reaches `degraded` sooner when it stalls; graph-index no longer reads
  `degraded` when one revision is held while later ones complete, only not ready with `staleness_ms` rising. An
  alert on `degraded` for graph-index fires less often; one on graph-embedding fires after 30 seconds, not 120.
  *Do:* check any alert threshold built on the old timing.
- **The error count now rises, and `Healthy` ignores it.** At the pin `error_count` and `error_rate` were always 0:
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
  unhealthy components and restart.

## Not carried

- The HTTP embedder and the vector providers behind it (#32, #489).
- The indexing-profile check: at the pin every path returned true, and the ADR behind it (ADR-054) is not ported.
- The pin's own watch, reset latch, status bucket, heartbeat and `readiness.go`: `graph/follower` does these for
  graph-index and graph-embedding alike.
