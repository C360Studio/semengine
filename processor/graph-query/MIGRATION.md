# Consumer migration: `processor/graph-query`

For developers of projects moving onto SemEngine's graph-query. It says what keeps working and what you change at your
cutover. The port breaks old shapes where that gives a better result (owner, PR #504 comment 6588079803). Cites to
sister repositories are at SemSource `e4febc0d` and SemConnect `d0d06e00`. Nothing here was run against your code: it
comes from reading it.

`processor/graph-query` is ported in five steps, and this file grows with them. Steps 1 to 4 are in the tree today; a
change marked with a later step describes what that step will bring and does not apply yet. Each later step appends
what it changes here.

| Step | What it adds to graph-query |
| --- | --- |
| 1 | The six lookups: `entity`, `batch`, `prefix`, `byName`, `entityByAlias`, `relationships`; `min_revision`; the check for a second responder on a declared subject |
| 2 | Path search and counts (`pathSearch`, `hierarchyStats`, `summary`), and the `semantic`, `similar`, `spatial` and `temporal` forwards |
| 3 | Local search inside one community (`localSearch`), read from a view of `COMMUNITY_INDEX` |
| 4 | The `globalSearch` coordinator: entity, path, time-range and area strategies; every other question is refused until step 5 |
| 5 | The text tier and `searchGraph`; the package is whole |

## The changes that fail silently

Read these first. Nothing errors.

- **SemSource `searchGraph` (not yet in the tree; arrives with step 5, the text tier).** Search replies gain a wrapper:
  `data`, `indexed_revision`, `producer`, `timestamp`. `graphSearch` (`processor/mcp-gateway/query_tools.go:113`)
  decodes into `graphSearchBody` (`graph_matches.go:107`), which has no `data` field. It ignores the unexpected wrapper
  instead of failing, so you get no error, no matches and no answer.
- **SemSource `deriveDisclosure` (with `searchGraph`, step 5).** It reads the same bytes for top-level fields
  (`disclosure.go:86`), so it reports no answer rather than a failure.
- **Fix for both:** unwrap once before reading, and assert it on a real reply. No framework mechanism does this for
  you.
- **A copy of the GraphQL gateway (already at step 1, more at each later step).** It nests every wrapped answer one
  level too deep; see its section below.
- **A `relationships` decoder that keeps `edge_type` (the `relationships` verb, step 1).** After unwrapping, the field
  reads empty. With a predicate filter, the old fusion client drops every edge (SemStreams `client.go:526`). Change 5's
  port of fusion must not carry this.
- **A `pathSearch` decoder that reads `paths` (the `pathSearch` verb, step 2).** After unwrapping, `paths` is gone, so
  the field reads empty and every entity looks unreachable. Read `edges`, where `edges[i]` first reached
  `entities[i+1]`, and follow them back for a path.
- **A `hierarchyStats` decoder that reads `totalEntities` (the `hierarchyStats` verb, step 2).** The key is now
  `total_entities`, so the old field reads 0. It also means something new: the whole group as listed, where the old
  value was the rows on one page, stopping at 1,000. Children count one page only, so on a prefix with children they add
  up to `entities_scanned`, not `total_entities`; a prefix that is itself a full six-part entity ID has no children, and
  on the first page of an ID that exists `entities_scanned` is 1 (`TestHierarchyStatsOfAnExactID`), for an absent ID, or
  a cursor past it, 0. `truncated` false says only that this is the last page;
  send `next_cursor` as `cursor` for the next page. The counts are live and as listed, not a snapshot, so a group that
  is written to while you page can miss an entity (#512). A child's `name` reads empty, since the key is
  gone.
- **A `summary` decoder that reads `entity_sample_truncated` (the `summary` verb, step 2).** The key is now `truncated`,
  so an old decoder reads false, which says "complete", on an answer that is a sample.
- **A `summary` decoder that reads `total_entities` as the rows scanned (step 2).** It is now the whole group as listed,
  before the cursor and any limit, so it can exceed the type counts' sum. The rows read are `entities_scanned`. Type
  counts and examples cover one page: on a first page with `truncated` true, the lexically smallest IDs. `truncated`
  false says only that this is the last page.
- **A `summary` decoder that reads `predicate_total` (step 2).** The key is gone, so it reads 0. Use the length of
  `predicates`, which is `null` when you excluded them and `[]` when there are none.
- **A `summary` caller that compares `indexed_revision` with predicates excluded (step 2).** That answer reads no index
  and reports 0, which claims nothing. Against a write's `kv_revision` it never catches up, so a retry loop would spin
  for ever. Do not compare on that answer.
- **A bare `entityByAlias` decoder (the `entityByAlias` verb, step 1).** It reads no entity from the wrapper and gets nil.
- **A `kvRevision` decoder with no zero check (the `entity` verb, step 1).** It gets revision 0. SemConnect's has the
  check, so its failure is loud; see its section.
- **A bare `localSearch` decoder (the `localSearch` verb, step 3).** The reply is now wrapped: the answer is in `data`.
  An old decoder reads no entities, no community and no answer, and no error.
- **A `localSearch` decoder that reads `communityId` (step 3).** The key is now `community_id`, so the old field reads
  empty. `count`, `durationMs`, `entity_digests`, `answer_model`, `degraded` and `degraded_reason` are gone and read
  as zero values. Use the length of `entities` for the count.
- **A `localSearch` caller that sends `min_revision` from a write outside the group it searches (step 3).** The reply's
  `indexed_revision` is the latest entity write the group's record took in. A write to another group does not move
  it, so the request is refused `revision_not_reached` until something in the searched group changes. A large group
  (level 2, the whole deployment) catches up at its next publish; a quiet level-0 group may never. A retry loop on that
  refusal can spin for ever. Send the `kv_revision` of a write to an entity in the group you search, or no
  `min_revision` at all.

## SemSource

**Use cases kept:** free-text search (`searchGraph`, which is not in the tree until step 5 lands and changes 5 and 7
bring what it needs); prefix paging (`processor/supersession/prefixquery.go:19`, bare reply, which gains `total`); your
own verbs on the same family (`sources`, `sourceSummary`, `status`, `predicates`, `versionDiff`), which the verb table
does not reserve.

**What you change:**

- The search decode above.
- Nothing for `localSearch`: no SemSource code at `e4febc0d` calls it (the whole tree checked; the only mentions are
  subject-ownership lists in tests and docs, and its ADR-0010 keeps `localSearch` unexposed), so nothing reads its
  reply. If you add one, decode `data` by the verb's table row, read `community_id` (not `communityId`), and send a
  `min_revision` only from a write to an entity in the group you search (see "The changes that fail silently").
- Your collision guard. graph-query's second-responder check sees declared ports only. Your graph requests subscribe
  with no port (`processor/source-manifest/component.go:176`, `InputPorts` empty at `:613`), so your old
  `graph.query.summary` collision (`:34-41`) would still pass it. Change
  `TestNoSemSourceSubjectCollidesWithTheSubstrate` (`cmd/semsource/subject_ownership_test.go`) to compare
  `RequestSubjects()` (`source-manifest/component.go:932`, and supersession's) with `graph.QueryVerbs()`, in place of
  its hand list. Declaring your verbs as ports also works. The framework adds no check at start: it would miss
  anything that starts later.
- Nothing in your graph-query config: `ports` alone is valid.
- A slow `searchGraph`, once step 5 lands: your 10 s default (`mcp-gateway/component.go:194-199`) can time out first, loudly.

## SemConnect

**Use cases kept:** exact entity reads and batch hydration (`graph.query.entity`, `graph.query.batch`,
`gateway/cs-api/systems.go:80-81`), both still bare; predicate reads, already enveloped (`:993`); `Register`, for your
backend (`cmd/cs-graph-backend/main.go:127`, on the branch of PR 74).

**What you change:**

- `gateway/cs-api/systems.go:808` decodes `kvRevision`; the key is `kv_revision`. Change the struct tag, or decode
  `graph.ExactEntity` as your ADR-004 § 3 already says. Until then entity reads fail loudly: `:814` turns revision 0
  into fatal `internal`.
- Delete `request_timeout`, `max_traversal_depth`, `max_traversal_nodes` from `deploy/semstreams.json` and
  `conformance/compose.semstreams.config.json`. The old version ignored them; unknown keys now refuse boot, naming the
  key. `query_timeout_ms` is the deadline on each downstream reply (default 15000).
- Keep the `graph-query` port block in `deploy/semstreams.json`: by reading, not run, it is accepted as written
  (`outputs: []` counts as none). Only the three keys above must go.
- Your 5 s default (`gateway/cs-api/config.go:157`) is under ingest's 10 s budget, so a 5 to 10 s `batch` still times
  out on your side, loudly, while graph-query keeps forwarding. Raise it if that matters.

## Whoever keeps the GraphQL gateway

`gateway/graph-gateway` is not ported: the ledger has it as consumer-owned, composed by SemSource but not exercised,
and a copy needs the agentic and `graph/inference` packages. SemSource composes it (`cmd/semsource/run.go:867`) and
documents `/graphql` (`docs/integration/m5-consumer-integration.md:178`); no owner is named after cutover.

**Use case to keep:** GraphQL over the `graph.query.*` verbs, if anyone still wants it.

**What a copy must change:**

- It unwraps by detection (`component.go:2059`), which is not ported, and the old unwrap accepts only `data` and
  `timestamp` (SemStreams `graph/query_contracts.go:89`). The new envelope has four keys, so a copy silently nests
  every wrapped answer one level too deep: `relationships` and `entityByAlias` from step 1, `summary` and the other verbs
  as their steps land. Decode by `graph.QueryVerbs()` instead.
- Its schema advertises `kvRevision` (`:1869`); the key is `kv_revision`.
- Its `relationships` type must read `predicate`, not `edge_type`, and must send `direction`.
- At the old version its alias lookup looks broken: it sends `alias` and the handler read `aliasOrID`, refusing as
  empty (read, not run). The request key is now `alias`, so that mismatch goes.

## Any caller

| Verb | Change |
| --- | --- |
| `entityByAlias` | Request key `alias`, was `aliasOrID`. Reply enveloped, with the entity in `data`. Refusals reach you: `alias_ambiguous`, `index_not_ready`, `internal`, `revision_not_reached`. An unknown alias not shaped like an ID answers `entity_not_found`, not `invalid_request`; this narrows one sentence of ADR-114, which ADR-114 now records |
| `relationships` | Reply enveloped, was a bare list. `direction` is required, `outgoing` or `incoming`. Rows name `predicate`, was `edge_type`. Fusion reads this; change 5 ports fusion here |
| `pathSearch` (step 2) | Request keys: `start_entity`, `direction`, `max_depth`, `max_nodes`, `predicates`, `min_revision`. `direction` is required, `outgoing`, `incoming` or `both`: nothing, or a typo, read as outgoing before. `max_depth` is required, 1 to 10; there is no default. `max_nodes` is 1 to 500, 0 means 100, and it now counts the start: the old limit did not, so the same number gives one entity fewer. `timeout`, `include_siblings` and `max_paths` are gone, and any unknown key is refused. Over a limit is `invalid_request` stating the bound, never a clamp. Reply enveloped: `data` holds `entities`, `edges` and `truncated`. Rows name `entity_id` and `depth`, were `id`, `type` and `score`; `paths` is gone, see "The changes that fail silently". An incoming edge now reads as the graph holds it, `from_entity_id` and `to_entity_id`, not backwards. `truncated` is true only when an entity was left out, not on reaching the limit. Any hop's refusal ends the walk, its message naming that hop's entity; a hop over `max_payload` is `response_too_large` |
| `hierarchyStats` (step 2) | Reply still bare. `totalEntities` is now `total_entities`, the length of the key listing, before the cursor and any limit, true as listed, not a snapshot; see "The changes that fail silently". New: `entities_scanned`, the rows on this page; `truncated`, true exactly when `next_cursor` is set; `next_cursor`. Children are `{prefix, count}` from this page; `name` is gone, being the prefix's last segment. Request keys: `prefix`, `cursor`, `min_revision`, which is ignored since no index is read; any unknown key is refused. A page with no `entities` or `total` key read as zero; it is now refused transient `internal`, as is a row whose ID does not parse or is not under the prefix. A refusal by graph-ingest reaches you with its class, code and detail. Each request, so each page, costs one key scan of the group |
| `summary` (step 2) | Reply still enveloped. A page with no `entities` or `total` key is refused transient `internal`, as is a row whose ID does not parse or is not under the prefix. A down or behind predicate index now refuses the whole request, with its class, code and detail, where it used to answer with no predicates: send `exclude_predicates: true` for entities alone. `include_predicates` is now `exclude_predicates`; the old key is refused by name, and so are `entity_sample_limit` and any other unknown key. An empty body is refused: send `{}`. New request keys: `prefix`, only with `exclude_predicates`, `cursor`, `min_revision`, which reaches the index. A negative `examples_per_type` is refused. Reply: `entity_sample_truncated` is now `truncated`; new `entities_scanned`, `next_cursor`; `total_entities` is the group as listed; `predicate_total` is gone; a page with no `entities` or `total` key, or an ID that does not parse, is refused transient `internal`; see "The changes that fail silently" |
| `entity` | Bytes forwarded unchanged, with no re-check by graph-query. The revision key is `kv_revision`, was `kvRevision` |
| `batch`, `prefix`, `byName` | Bytes forwarded unchanged, with no re-check by graph-query. `prefix` (step 2) gains `total`, always present: the length of the key listing, before the cursor and any limit, true as listed, not a snapshot. The key is additive |
| `semantic` (step 2) | Forwards to `graph.embedding.query.search`, bytes unchanged, so `scope` reaches the responder (ADR-071). Reply enveloped by the responder, `data` holding `results` and `embedder_type`; `query` and `duration` are gone. Refused transient `internal`, naming that subject, until change 5 brings the responder. Fusion reads this; change 5 ports fusion here |
| `similar` (step 2) | Forwards to `graph.embedding.query.similar`, bytes unchanged. Reply enveloped by the responder. `data.similar` is now `data.results`; the echoed `entity_id` and `duration` are gone. Refused transient `internal` until change 5 |
| `spatial` (step 2) | Forwards to `graph.spatial.query.bounds`, bytes unchanged. Reply enveloped by the responder, was a bare list: `data` holds `results` and `truncated`. Rows name `entity_id`, was `id`, with `lat`, `lon`, `alt`; `type` is gone. Refused transient `internal` until change 7 |
| `temporal` (step 2) | Forwards to `graph.temporal.query.range`, bytes unchanged. Reply enveloped by the responder, was a bare list: `data` holds `results` and `truncated`. Rows name `entity_id`, was `id`; `type` is gone. Request keys become `start_time` and `end_time`, were `startTime` and `endTime`, at change 7, with the responder; until then it is refused transient `internal` |
| `localSearch` (step 3) | Request keys: `entity_id`, `query`, `level`, `min_revision`; any unknown key is refused. `entity_id` must be a full six-part ID; `level` is 0 (the default), 1 or 2; `query` is lowercased and split on white space, repeats removed, and must leave 1 to 16 terms. Each failure is `invalid_request`, before any read. Reply enveloped, was bare: `data` holds `community_id` (was `communityId`), `level`, `member_count`, `community_summary`, `keywords`, `entities`, `entities_scanned`, `entities_truncated` and `answer`; `count`, `durationMs`, `entity_digests`, `answer_model`, `degraded` and `degraded_reason` are gone. `indexed_revision` is the group record's latest entity write. Nothing in the framework writes `COMMUNITY_INDEX` yet, so it is refused transient `index_not_ready` until change 7 brings graph-clustering's writer. It reads the group's record, then one page of the group (at most 1,000 entities); `entities_truncated` is true when the group had more than that page, or matches were left out to fit `max_payload`. Matches pass as graph-ingest sent them. `answer` is a plain sentence built from the counts and the record's summary; the pin's text ended `(7 entities, 0% match)`, and the pin's fallbacks to a semantic search when the group had no record, and to the clustering query that logged and swallowed every failure, are not carried. Refusals: transient `index_not_ready` while `COMMUNITY_INDEX` is missing, catching up or lost, or the entity exists but its group has no record yet; the entity verb's own refusal, such as `entity_not_found`, when the entity does not exist; transient `internal` naming the key for an unreadable record, or for a broken page or no responder; transient `revision_not_reached` with `indexed_revision` in detail. See "The changes that fail silently" |
| `globalSearch` (step 4) | Request keys: `query` (required, not blank) and `min_revision`; `level`, `max_communities`, `summarize_threshold`, `include_summaries`, `include_relationships` and `include_sources` are refused `invalid_request` by name until step 5, and so is any unknown key. Reply enveloped, was bare: `data` holds `strategy` and one of `entity`, `path`, `temporal` or `spatial`; `entities`, `count`, `duration_ms`, `answer`, `community_summaries`, `degraded` and `degraded_reason` are gone. Strategy names: `pathrag` is `path`, `entity_lookup` is `entity`; `graphrag` and `semantic` are not answered yet. A one-word question is always an entity reference, read before any other meaning, so an ID or alias holding a word such as `today`, `zone-a`, `link`, `max` or `most` is looked up, not read as a time range, path, count or ranking: `entity` is the entity verb's reply as it came, under the alias reply's `indexed_revision`. A lone `today` or `yesterday` that names no entity is refused `text_search_unsupported`, never as a time range. A path question ("what is connected to pump-42") is answered by the path search from its start, both directions, depth 3, 100 entities, all predicates: `path` is the `pathSearch` reply's `data` for that request, IDs and edges, no entity states (call `batch` for those), under the walk's `indexed_revision`. A partial ID such as `sensor-001` resolves only as an alias: the suffix lookup is gone. A six-part ID is now asked of the alias index first; the old version took it as itself. In a longer path question the start is whatever word with a hyphen the classifier finds, never a whole ID: "what is connected to acme.plant.robotics.gcs.drone-x.001" walks from the alias `drone-x`, and a full ID with no hyphenated segment gives no start and is refused `text_search_unsupported` as below, or, beside a zone or area ("what is connected to c360.test.x.y.z.001 in zone b"), refused `invalid_request` naming both places; name the start by alias. Only one start is read: "from A to B" walks from A, a limit of the keyword classifier tracked in #535. A question naming an entity and a zone or area ("what is connected to sensor-001 in zone-b", "near zone-b", or "show sensor-001 in zone-b", with or without path words), or two different zones or areas however spelled ("what is connected to zone a in zone b"), is refused `invalid_request`, naming both places; the old version answered from one of them and dropped the other. That refusal is loud and can be wrong: an ordinary hyphenated word ("in real-time", "non-zero") or "zone or area" plus a word ("the area with pumps") beside a place can be refused as a second place, a known limit tracked in #535. Refusals reach you as they arrived: from the alias verb (`alias_ambiguous`, `index_not_ready`, `internal`, `revision_not_reached`) and from the entity verb (any but `entity_not_found`); the old version dropped them and answered some other way. A question with two of a path, a time range and an area is `invalid_request`, naming both, as is an impossible number ("the last 0 hours"). A question whose path or time range comes with a count ("how many"), a ranking ("top 5") or a similarity ("similar to") is not answered by that strategy any more (the old version answered the path or time part and dropped the rest). Any other question, and a name or ID that names no entity (no alias holder and no valid ID, or a valid ID with no record), is refused class invalid, code `text_search_unsupported`, until step 5 brings the text search: do not retry it unchanged. That includes a zone or area question whose start names no entity ("the area of the plant", #479), which the old version, once started, passed on to its community search (`graphrag`): its empty `pathrag` answer ran only without a community cache, which a started component always has. That refusal does not count as an error in `Health`. A time-range question ("what changed in the last 2 hours") is answered by `graph.temporal.query.range`, and an area question ("what is within 5km of 40.7,-74.0") by `graph.spatial.query.bounds`: `temporal` or `spatial` is that verb's `data` as it came (`results`, rows naming `entity_id`, with `lat`, `lon` and `alt` for an area, and `truncated`), no entity states (call `batch` for those; the old version loaded each entity), under that reply's `indexed_revision`. The time-range request is `start_time` and `end_time` in RFC 3339, `limit` 100 and `min_revision`, was `startTime`, `endTime` and `limit`; the area request is `north`, `south`, `east`, `west`, `limit` 100 and `min_revision`. A refusal from either verb reaches you as it arrived, its class, code and detail unchanged. A reply missing `results` or `truncated` is refused transient `internal` naming the subject, which counts as an error in `Health`: a missing `truncated` would read as a complete answer. Until change 7 brings their responders both are refused transient `internal` naming the subject, which counts as an error in `Health` |
| `globalSearch` (step 5, routing) | Replaces the step 4 row where they differ. Request keys gain `level`, 0 to 2, omitted 0, read only by the text search; any other value is `invalid_request`. `text_search_unsupported` is never sent. A question with no hint, or whose only hints are path words with no start ("http connection pool", "file path handling"), a similarity ("similar to") or a ranking ("top 5"), is answered by the text search, `strategy` `text`; its reply will name each such hint it did not honour (`path`, `similarity`, `ranking`). A count or other aggregation ("how many", "max retries", "total memory usage"), with or without other hints, is `invalid_request` naming it. Path words with no start, a similarity or a ranking beside a start, a time range or an area ("top 5 sensors connected to pump-42", "what is related to it in the last 2 hours") is `invalid_request` naming both: neither half is dropped. A one-word name or ID that names no entity is searched as text. A path start that names no entity ("what is connected to pump-99", and "the area of the plant", whose start is `area-of`) is `entity_not_found`, class invalid, as `entityByAlias` answers it; a start whose entity is gone passes the entity verb's refusal as it came. Until `COMMUNITY_INDEX` has a writer (change 7) the text search refuses transient `index_not_ready`, which counts no error in `Health` |
| `globalSearch` (step 5, what the text search reads) | Replaces the routing row where they differ. A text question with more than 16 distinct terms (lowercased, repeats removed) is `invalid_request`, before any read. The text search reads the community records at `level` once, and before any page read refuses: transient `index_not_ready` while `COMMUNITY_INDEX` is missing, catching up, lost or stopped, or when the level holds no record (the old version answered an empty level with an empty success); transient `internal` naming the count when every record at the level is unreadable, which counts as an error in `Health`; transient `revision_not_reached`, with `indexed_revision` in detail, when `min_revision` is above the highest `entity_revision` among the level's readable records, the text answer's `indexed_revision`. It then reads at most five groups, only those whose record scores above 0 for the question, best first, ties by key (the old version read the first five even when none matched, ties left to an unstable sort, and `max_communities` raised the count), and one page of at most 1,000 entities from each (the old version loaded every member, up to 10,000), whose entities are matched and ranked as `localSearch` ranks them. Until this step's reply lands, a text question the search answers is refused transient `internal`, which counts no error |
| Enveloped verbs | May send `min_revision`; an index behind it refuses transient `revision_not_reached`, with the reached revision in detail. Bare verbs ignore it |

A stream capturing a declared `graph.query.*` subject refuses boot (#16), and so does a declared NATS-request input
port overlapping one, unless it belongs to graph-query. An undeclared subscriber is not seen. Decode by the verb table:
`QueryVerb.Enveloped` and `QueryVerb.Reply`.

**New names steps 1 to 4 export** (step 5 adds its own after these). Step 1, in `graph`: `ResponderGraphQuery`,
`QueryVerb.Enveloped`, `QueryVerb.Reply`, `MinRevisionField`, `ErrorCodeRevisionNotReached`, `RelationshipEdge`. In
`composition`: `TypeRequestSubjectConflict`. In the new `graphquery` package, the old version's names: `Register`,
`DeclarePorts`, `CreateGraphQuery`, `DefaultConfig`, `Config`, `Component`. Step 2, in `graph`: `ResponderGraphEmbedding`,
`ResponderGraphIndexSpatial`, `ResponderGraphIndexTemporal`; for path search `PathSearchRequest`, `PathSearchData`,
`PathEntity`, `MaxPathSearchDepth`, `MaxPathSearchNodes`, `DefaultPathSearchNodes`; for counts `HierarchyStatsRequest`,
`HierarchyStatsData`, `HierarchyChild`, and `PrefixQueryResponse.Total`; for the forwards `SemanticData`, `SimilarData`,
`SimilarityHit`, `SpatialData`, `SpatialHit`, `TemporalData`, `TemporalHit`. `SummaryRequest` and `SummaryData` change
as the `summary` row says. Package `graphquery` exports nothing new in steps 1 and 2. Step 3, in `graph`:
`LocalSearchRequest`, `LocalSearchData` and `MaxSearchTerms`. In `graph/clustering`: `Community.EntityRevision`,
`GroupKey` and `DecodeCommunity`. Package `graphquery` exports nothing new. Step 4, in `graph`: `GlobalSearchRequest`,
`GlobalSearchData`, `TemporalRangeRequest` and `SpatialBoundsRequest`. Package `graphquery` exports nothing new.
Step 5, in `graph`: `GlobalSearchRequest.Level`.
