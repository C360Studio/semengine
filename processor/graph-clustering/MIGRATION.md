# Consumer migration: `processor/graph-clustering`

For developers of projects that ran SemStreams' graph-clustering and move to SemEngine's. It says what you change at
your cutover and what you can leave alone. The port breaks old shapes where that gives a better result
(owner, 2026-11-02). Nothing here was run against your code: it comes from reading it and from the consumers'
configurations as inventoried for this change.

The biggest change is what a community is. The pin grouped entities by label propagation over relationships and
needed graph-index to be caught up first. SemEngine groups entities by the first parts of their IDs (ADR-099): a group
is every entity sharing the first 4, 3 or 2 parts, and each group gets one record in `COMMUNITY_INDEX`. The records
are kept from `ENTITY_STATES` changes, never by rescanning, and nothing else is read.

## Changes that stop boot (loud)

Each of these fails when the component is created, naming the key or value, with a line saying what replaces it. Fix
the configuration before the first start.

- **`batch_size` is refused.** At the pin nothing read it. *Do:* delete the line. semboids sets it.
- **`enable_llm` and `enhancement_workers` are refused.** Language-model community summaries are tier 2 (#485) and
  not present; a record carries a one-line summary built from its counts. semsource always sends `enable_llm`.
  *Do:* delete the line.
- **`min_community_size` and `max_iterations` are refused.** There is no label propagation, so nothing iterates and
  every group is kept whatever its size. semboids sets `min_community_size`. *Do:* delete the line.
- **`entity_id_edges` and `semantic_edges` are refused.** No edges are synthesized: entities that share a prefix
  share a group. semsource may send `entity_id_edges`, and semboids sets it. *Do:* delete the line.
- **`allow_ungated_reads` is refused.** The component reads no other index, so there is no readiness gate to bypass.
  *Do:* delete the line.
- **`enable_anomaly_detection` and `anomaly_config` are refused.** Anomaly detection is not ported and will not be
  (owner ruling, PR #582 comment 6768832111). *Do:* delete the lines.
- **Any unknown key is refused**, naming it. The kept keys are `ports`, `detection_interval`, `startup_attempts` and
  `startup_interval_ms`. A key in the wrong case (`BATCH_SIZE`) is refused as unknown, without the guidance line.
- **A bad `detection_interval` is refused.** The pin fell back to 30 seconds silently when it could not parse the
  value. Here `soon`, `30` (no unit), `0s` and `-1s` fail at creation, naming the key. *Do:* write a Go duration
  above zero, such as `30s` or `5m`, or leave the key out for `30s`.
- **A negative `startup_attempts` or `startup_interval_ms` is refused.** The pin accepted them. Zero still means the
  default (30 tries, 500 ms apart).
- **Ports are checked.** The one input must be a `kv-watch` of every `ENTITY_STATES` key with no history; a second
  input, a kv-read, a filtered or history watch, or no input is refused naming the port. The pin's default also read
  `OUTGOING_INDEX` and `INCOMING_INDEX`; they are no longer read. Any declared output is refused: the component writes
  `COMMUNITY_INDEX` itself. *Do:* leave `ports` out to take the default, or declare the one watch yourself. semboids
  declares input and output ports, so its output ports must go.

## Changes that do not stop boot (silent)

Read these. Nothing errors.

- **Communities are prefix groups.** A group is the entities that share their ID's first 4 (level 0), 3 (level 1) or
  2 (level 2) parts. Two entities with no relationship but the same source prefix are in one community; two linked
  entities under different prefixes are not. Records are keyed `{level}.{group ID}`, and a record holds counts,
  keywords and a few sample IDs, never a member list (see `graph/clustering/README.md`).
- **semboids' two-community test will see one group per source.** Its spike builds two clusters from explicit edges
  and asserts two label-propagation communities (`internal/boidgraph/clustering_spike_integration_test.go`). Under
  prefix grouping the edges do not count: the test sees one group per source prefix, so its expectation changes
  (owner accepted the record break, 2026-10-31). *Do:* assert on the groups of the IDs you wrote.
- **The readiness key counts what is held, not what is published.** `GRAPH_STATUS` under `graph-clustering` reports a
  revision indexed once the Maintainer has applied it, which is before the next tick writes it. A reader that needs a
  group's published state reads the record's `entity_revision`. *Do:* do not gate a read on `indexed_revision` for
  freshness; send `min_revision` to graph-query, which compares it with `entity_revision`.
- **Nothing is published before the replay is applied, and a lost feed stops publication.** The pin ran a timer that
  rescanned the bucket. Here a tick starts only while the follower says the graph state is ready; after a lost feed or
  a latched reset no tick starts, and `Health` says so until the process restarts.
- **No dependency on graph-index or graph-embedding.** The pin deferred a tick until graph-index was ready; it
  watched graph-embedding's key only when semantic edges were on, and a cold embedding index only narrowed the cycle.
  Here the component follows `ENTITY_STATES` alone, so communities wait for neither index.
- **One bad stored value no longer blocks everything.** The pin latched its reset on any bad value it read and
  stopped until restart. Here only a bad value in the startup replay latches the reset. A bad value written live is
  counted as `refused_entity`, left out of its groups, and the other groups keep publishing.
- **Failures are visible.** A tick that fails to write reads `degraded` on the readiness record and unhealthy in
  `Health` (reason `publication_failed`) until a tick succeeds, and counts on
  `semengine_graph_clustering_publication_failures_total`. A stored entity the Maintainer refuses is left out of its
  groups, reported as `refused_entity`, and the other groups still publish.
- **Records can be absent until the first tick.** The first tick runs one `detection_interval` after `Start`, and only
  after the replay is applied. Until then graph-query's `localSearch` and text questions refuse transient
  `index_not_ready`. A smaller `detection_interval` shortens the wait.

## Not carried

- The four `graph.clustering.query.*` verbs. Nothing in the consumers called them. Read `COMMUNITY_INDEX` through
  graph-query.
- The readiness gate on graph-index and graph-embedding (ADR-082, ADR-085).
- The enhancement and review workers, the summary store and language-model summaries (tier 2, #485, #488).
- Label propagation and the edge providers (ADR-099).
- Anomaly detection and its storage and handlers: never ported.
- The indexing-profile check: ADR-054 is not ported.
- The pin's Prometheus globals; the one collector here registers on the injected registry.
