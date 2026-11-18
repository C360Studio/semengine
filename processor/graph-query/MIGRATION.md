# Consumer migration: `processor/graph-query`

For developers of projects moving onto SemEngine's graph-query. It says what keeps working and what you change at your
cutover. The port breaks old shapes where that gives a better result. Cites to
sister repositories are at SemSource `e4febc0d` and SemConnect `d0d06e00`. Nothing here was run against your code: it
comes from reading it.

`processor/graph-query` is whole in the tree: the lookups (`entity`, `batch`, `prefix`, `byName`, `entityByAlias`,
`relationships`), path search and counts (`pathSearch`, `hierarchyStats`, `summary`), the forwards (`semantic`,
`similar`, `spatial`, `temporal`), `localSearch`, `globalSearch` and `searchGraph`, which find entities by the words of
their
text.

## The changes that fail silently

Read these first. Nothing errors.

- **SemSource `searchGraph`.** Search replies gain a wrapper: `data`, `indexed_revision`, `producer`,
  `timestamp`. `graphSearch` (`processor/mcp-gateway/query_tools.go:113`) decodes into `graphSearchBody`
  (`graph_matches.go:107`), which has no `data` field. It ignores the unexpected wrapper instead of failing, so you get
  no error, no matches and no answer. Inside `data`, none of the keys `graphSearchBody` reads is sent: a text answer
  is `data.text`, with `matches` (`id`, `label`, `score`, `entity`), `groups` and `answer`, and `data.hints` says what
  else the
  question asked for; `entity_digests`,
  `entities`, `entity_ids`, `community_summaries` and `count` are gone (the `globalSearch` rows below). Today your
  first call fails loudly before this: its `summarize_threshold` key is refused (SemSource section).
- **SemSource `deriveDisclosure` (with `searchGraph`).** It reads the same bytes for top-level fields
  (`disclosure.go:86`), so it reports no answer rather than a failure.
- **SemSource `searchGraph` on a deployment with embeddings but no matching words.** The old version answered
  from the embedding index when global search was empty or refused `index_not_ready`: strategy `semantic_fallback`, up
  to 8 entities, marked degraded, which `deriveDisclosure` read as `strategy` and
  `degraded_reason == "global_search_empty_semantic_fallback"` (`disclosure.go:31`). Neither key is sent now. With
  `embedding_enabled` true, every question is also asked of the embedding search, and `data.text.matches` is the merge
  of both lists (each match has `word_rank` and `vector_rank`). `data.merge` says whether vectors were used: `hint`
  `semantic`, `detail` `merged` or `index_not_ready`, `honoured`, and `reason` and `code` when it failed. The
  embedding search's own reply is in `data.semantic` (`results`, up to 100). Read `merge` and `semantic` instead of
  `strategy` and `degraded_reason`, and read a `null` `data.text` as "the word index was not read", never as "no word
  matches". Search no longer waits for graph-clustering,
  so a deployment without clustering gets matches with `record_state` `not_ready`, not a refusal.
- **SemSource `searchGraph` for a time range, an area or a count.** The old version answered "what changed in the last
  2 hours" and "what is within 5km" from the time and place verbs, and refused a count. Every question is now answered
  by its words, and what the words did not carry is listed in `data.hints` with `honoured` false and a `reason`. A
  caller that took the old refusal or the old `temporal` and `spatial` answers as a signal must read `hints`.
- **Fix for both:** unwrap once before reading, and assert it on a real reply. No framework mechanism does this for
  you.
- **A copy of the GraphQL gateway.** It nests every wrapped answer one
  level too deep; see its section below.
- **A `relationships` decoder that keeps `edge_type` (the `relationships` verb).** After unwrapping, the field
  reads empty. With a predicate filter, the old fusion client drops every edge (SemStreams `client.go:526`). SemEngine's
  fusion does not carry this.
- **A `pathSearch` decoder that reads `paths` (the `pathSearch` verb).** After unwrapping, `paths` is gone, so
  the field reads empty and every entity looks unreachable. Read `edges`, where `edges[i]` first reached
  `entities[i+1]`, and follow them back for a path.
- **A `hierarchyStats` decoder that reads `totalEntities` (the `hierarchyStats` verb).** The key is now
  `total_entities`, so the old field reads 0. It also means something new: the whole group as listed, where the old
  value was the rows on one page, stopping at 1,000. Children count one page only, so on a prefix with children they add
  up to `entities_scanned`, not `total_entities`; a prefix that is itself a full six-part entity ID has no children, and
  on the first page of an ID that exists `entities_scanned` is 1 (`TestHierarchyStatsOfAnExactID`), for an absent ID, or
  a cursor past it, 0. `truncated` false says only that this is the last page;
  send `next_cursor` as `cursor` for the next page. The counts are live and as listed, not a snapshot, so a group that
  is written to while you page can miss an entity. A child's `name` reads empty, since the key is
  gone.
- **A `summary` decoder that reads `entity_sample_truncated` (the `summary` verb).** The key is now `truncated`,
  so an old decoder reads false, which says "complete", on an answer that is a sample.
- **A `summary` decoder that reads `total_entities` as the rows scanned.** It is now the whole group as listed,
  before the cursor and any limit, so it can exceed the type counts' sum. The rows read are `entities_scanned`. Type
  counts and examples cover one page: on a first page with `truncated` true, the lexically smallest IDs. `truncated`
  false says only that this is the last page.
- **A `summary` decoder that reads `predicate_total`.** The key is gone, so it reads 0. Use the length of
  `predicates`, which is `null` when you excluded them and `[]` when there are none.
- **A `summary` caller that compares `indexed_revision` with predicates excluded.** That answer reads no index
  and reports 0, which claims nothing. Against a write's `kv_revision` it never catches up, so a retry loop would spin
  for ever. Do not compare on that answer.
- **A bare `entityByAlias` decoder (the `entityByAlias` verb).** It reads no entity from the wrapper and gets nil.
- **A `kvRevision` decoder with no zero check (the `entity` verb).** It gets revision 0. SemConnect's has the
  check, so its failure is loud; see its section.
- **A bare `localSearch` decoder (the `localSearch` verb).** The reply is now wrapped: the answer is in `data`.
  An old decoder reads no entities, no community and no answer, and no error.
- **A `localSearch` decoder that reads `communityId`, `entities` or a count.** The key is now `community_id`, so the old
  field reads empty. The matches are `data.matches` (was `entities`), each with `id`, `label`, `score` and `entity`;
  `matched` is the count in the group before `limit`. `member_count`, `community_summary` and `keywords` moved into
  `data.record`, which is absent unless `record_state` is `read`. `entities_scanned`, `entities_truncated`, `count`,
  `durationMs`, `entity_digests`, `answer_model`, `degraded` and `degraded_reason` are gone and read as zero values;
  `truncated` replaces `entities_truncated`.
- **A `localSearch` or `searchGraph` caller that matched by substring.** The old version scored a term that was a
  substring of a predicate name or a string value. Matching is now by word (BM25), so `pum` no longer finds `pump`;
  send the whole word. A trailing `*` is refused by the word index, not a prefix match.
- **A `localSearch` caller that sends `min_revision`.** It now goes to the word index: the reply's `indexed_revision`
  is the word index's, and a revision from a write to any entity is reached once the word index has applied it. A
  write outside the searched group no longer leaves the request refused for good.

## SemSource

**Use cases kept:** free-text search (the `graph_search` tool's `searchGraph` call, answering as `globalSearch`); prefix
paging (`processor/supersession/prefixquery.go:19`, bare reply, which gains `total`); your own verbs on the same family
(`sources`, `sourceSummary`, `status`, `predicates`, `versionDiff`), which the verb table does not reserve.

**What you change:**

- The search decode above, for `graph_search` and `deriveDisclosure`. Read `data.text.matches`, `data.text.groups`,
  `data.text.answer` and `data.hints`; a question for a time range, an area, a count, a similarity or a ranking comes
  back
  with matches for its words and a hint that says it was not applied.
- Drop `summarize_threshold` from your `searchGraph` request (`query_tools.go:103`). graph-query refuses it
  `invalid_request`, naming the key, so every search fails until you do. Nothing replaces it: a text answer always
  carries its groups.
- Search sends words, not meaning. A question over entities whose facts are numbers, such as sensor or drone telemetry,
  matches nothing: those entities have no text, so word search gains nothing for them. Ask for them by prefix,
  predicate, name, alias or relationship.
- `localSearch` callers: SemSource at `e4febc0d` has none (the whole tree checked; the only mentions are
  subject-ownership lists in tests and docs, and its ADR-0010 keeps `localSearch` unexposed), so nothing reads its
  reply. If you add one, decode `data` by the verb's table row, read `community_id` (not `communityId`) and
  `data.matches`, and send `min_revision` from any write you want searched.
- Your collision guard. graph-query's second-responder check sees declared ports only. Your graph requests subscribe
  with no port (`processor/source-manifest/component.go:176`, `InputPorts` empty at `:613`), so your old
  `graph.query.summary` collision (`:34-41`) would still pass it. Change
  `TestNoSemSourceSubjectCollidesWithTheSubstrate` (`cmd/semsource/subject_ownership_test.go`) to compare
  `RequestSubjects()` (`source-manifest/component.go:932`, and supersession's) with `graph.QueryVerbs()`, in place of
  its hand list. Declaring your verbs as ports also works. The framework adds no check at start: it would miss
  anything that starts later.
- Nothing in your graph-query config: `ports` alone is valid.
- A slow `searchGraph`: your 10 s default (`mcp-gateway/component.go:194-199`) can time out first, loudly.
- A damaged hit no longer refuses a search. The reply gains `data.text.damaged` (the IDs left out because their
  stored value could not be read, in hit order, omitted when none). Your mcp-gateway reads only `truncated`
  (`processor/mcp-gateway/graph_matches.go:130`), which a damaged hit sets; read `damaged` to name the hit.

## SemConnect

**Use cases kept:** exact entity reads and batch hydration (`graph.query.entity`, `graph.query.batch`,
`gateway/cs-api/systems.go:80-81`), both still bare; predicate reads, already enveloped (`:993`); `Register`, for your
backend (`cmd/cs-graph-backend/main.go:127`).

**What you change:**

- `gateway/cs-api/systems.go:808` decodes `kvRevision`; the key is `kv_revision`. Change the struct tag, or decode
  `graph.ExactEntity` as your ADR-004 § 3 already says. Until then entity reads fail loudly: `:814` turns revision 0
  into fatal `internal`.
- Delete `request_timeout`, `max_traversal_depth`, `max_traversal_nodes` from `deploy/semstreams.json` and
  `conformance/compose.semstreams.config.json`. The old version ignored them; unknown keys now refuse boot, naming the
  key. `query_timeout_ms` is the deadline on each downstream reply (default 15000).
- If the composition runs graph-embedding, add `"embedding_enabled": true` to the `graph-query` config. The key is
  new and reads false when left out, which is the tier 0 answer: `semantic` and `similar` are refused invalid
  `not_enabled`, naming the verb, instead of the transient `internal` refusal a caller would retry without success.
- Keep the `graph-query` port block in `deploy/semstreams.json`: by reading, not run, it is accepted as written
  (`outputs: []` counts as none). Only the three keys above must go.
- Your 5 s default (`gateway/cs-api/config.go:157`) is under ingest's 10 s budget, so a 5 to 10 s `batch` still times
  out on your side, loudly, while graph-query keeps forwarding. Raise it if that matters.
- A `batch` reply can now list an ID in `missing` with reason `damaged` (its stored value is refused), where it used to
  refuse the whole batch. The `missing` switch at `gateway/cs-api/systems.go:896` must accept `damaged`; until it
  does, it refuses the batch loudly. A caller that reads `entities` alone gets the readable entities.

## Whoever keeps the SPARQL gateway

A `batch` reply can list an ID in `missing` with reason `damaged`. `sparql/executor.go:1564` fails a query on any
reason other than `not_found`, so a query that reads a damaged entity still fails, but with its own unclassified error
in place of fatal `graph_state_reset_required`.

## Whoever keeps the GraphQL gateway

`gateway/graph-gateway` is in this tree; `gateway/graph-gateway/MIGRATION.md` covers moving to it. SemSource composes
the SemStreams gateway (`cmd/semsource/run.go:867`) and documents `/graphql`
(`docs/integration/m5-consumer-integration.md:178`). A project that keeps its own copy of that gateway instead must
change the following.

**What a copy must change:**

- It unwraps by detection (`component.go:2059`), which is not ported, and the old unwrap accepts only `data` and
  `timestamp` (SemStreams `graph/query_contracts.go:89`). The new envelope has four keys, so a copy silently nests
  every wrapped answer one level too deep: `relationships`, `entityByAlias`, `summary` and the other verbs. Decode by
  `graph.QueryVerbs()` instead.
- Its schema advertises `kvRevision` (`:1869`); the key is `kv_revision`.
- Its `relationships` type must read `predicate`, not `edge_type`, and must send `direction`.
- At the old version its alias lookup looks broken: it sends `alias` and the handler read `aliasOrID`, refusing as
  empty (read, not run). The request key is now `alias`, so that mismatch goes.

## Any caller

| Verb | Change |
| --- | --- |
| `entityByAlias` | Request key `alias`, was `aliasOrID`. Reply enveloped, with the entity in `data`. Refusals reach you: `alias_ambiguous`, `index_not_ready`, `internal`, `revision_not_reached`. An unknown alias not shaped like an ID answers `entity_not_found`, not `invalid_request`; this narrows one sentence of ADR-114, which ADR-114 now records |
| `relationships` | Reply enveloped, was a bare list. `direction` is required, `outgoing` or `incoming`. Rows name `predicate`, was `edge_type`. Fusion reads this |
| `pathSearch` | Reply enveloped; `direction` and `max_depth` are now required and a limit over its bound is refused, not clamped. See "Verb details". |
| `hierarchyStats` | Reply still bare, with new counts and a cursor; `totalEntities` is now `total_entities`. See "Verb details". |
| `summary` | Reply still enveloped; `include_predicates` is now `exclude_predicates`, and a down predicate index refuses the request. See "Verb details". |
| `entity` | Bytes forwarded unchanged, with no re-check by graph-query. The revision key is `kv_revision`, was `kvRevision` |
| `batch`, `prefix`, `byName` | Bytes forwarded unchanged, with no re-check by graph-query. `prefix` gains `total`, always present: the length of the key listing, before the cursor and any limit, true as listed, not a snapshot. The key is additive |
| `predicate` | New, additive: forwards to `graph.index.query.predicate`, bytes unchanged, so `min_revision` reaches graph-index. Request `graph.PredicateRequest` (`predicate`, optional `value` and `limit`); reply enveloped by graph-index, `data` a `graph.PredicateData` |
| `semantic` | Forwards to `graph.embedding.query.search`; reply enveloped by the responder, `data` holding `results` and `embedder_type`. See "Verb details". |
| `similar` | Forwards to `graph.embedding.query.similar`, bytes unchanged. Reply enveloped by the responder. `data.similar` is now `data.results`; the echoed `entity_id` and `duration` are gone. Changes of the responder are in `processor/graph-embedding/MIGRATION.md`. Refused transient `internal`, naming the subject, if nothing responds; refused invalid `not_enabled`, naming `similar`, without a request unless `embedding_enabled` is true |
| `spatial` | Forwards to `graph.spatial.query.bounds`, bytes unchanged. Reply enveloped by the responder, was a bare list: `data` holds `results` and `truncated`. Rows name `entity_id`, was `id`, with `lat`, `lon`, `alt`; `type` is gone. Refused transient `internal` until its responder is in the tree |
| `temporal` | Forwards to `graph.temporal.query.range`, bytes unchanged. Reply enveloped by the responder, was a bare list: `data` holds `results` and `truncated`. Rows name `entity_id`, was `id`; `type` is gone. Request keys become `start_time` and `end_time`, were `startTime` and `endTime`, with the responder; until then it is refused transient `internal` |
| `localSearch` | Reply enveloped, was bare; matches come from the word index, scoped to the group, and the group's record is attached beside them; keys and fields change. See "Verb details". |
| `globalSearch` | Reply enveloped, was bare; every question is searched for its words, strategies are gone, and `hints` lists what else the question asked for. See "Verb details". |
| `searchGraph` | New on this table: `globalSearch` under its own name, with the same request and refusals; with `embedding_enabled` true it merges the word and embedding lists and adds `merge` and `semantic`; each match gains `word_rank` and `vector_rank`. In alpha.5 it answered as `globalSearch`; now its order and `score` come from the merge. See "Verb details". |
| Enveloped verbs | May send `min_revision`; an index behind it refuses transient `revision_not_reached`, with the reached revision in detail. Bare verbs ignore it |

## Verb details

The verbs whose change is too long for the table above.

### `pathSearch`

Request keys: `start_entity`, `direction`, `max_depth`, `max_nodes`, `predicates`, `min_revision`. `direction` is
required, `outgoing`, `incoming` or `both`: nothing, or a typo, read as outgoing before. `max_depth` is required, 1 to
10; there is no default. `max_nodes` is 1 to 500, 0 means 100, and it now counts the start: the old limit did not, so
the same number gives one entity fewer. `timeout`, `include_siblings` and `max_paths` are gone, and any unknown key is
refused. Over a limit is `invalid_request` stating the bound, never a clamp. Reply enveloped: `data` holds `entities`,
`edges` and `truncated`.

Rows name `entity_id` and `depth`, were `id`, `type` and `score`; `paths` is gone, see "The changes that fail
silently". An incoming edge now reads as the graph holds it, `from_entity_id` and `to_entity_id`, not backwards.
`truncated` is true only when an entity was left out, not on reaching the limit. Any hop's refusal ends the walk, its
message naming that hop's entity; a hop over `max_payload` is `response_too_large`.

### `hierarchyStats`

Reply still bare. `totalEntities` is now `total_entities`, the length of the key listing, before the cursor and any
limit, true as listed, not a snapshot; see "The changes that fail silently". New: `entities_scanned`, the rows on this
page; `truncated`, true exactly when `next_cursor` is set; `next_cursor`. Children are `{prefix, count}` from this
page; `name` is gone, being the prefix's last segment. Request keys: `prefix`, `cursor`, `min_revision`, which is
ignored since no index is read; any unknown key is refused.

A page with no `entities` or `total` key read as zero; it is now refused transient `internal`, as is a row whose ID
does not parse or is not under the prefix. A refusal by graph-ingest reaches you with its class, code and detail. Each
request, so each page, costs one key scan of the group.

### `summary`

Reply still enveloped. A page with no `entities` or `total` key is refused transient `internal`, as is a row whose ID
does not parse or is not under the prefix. A down or behind predicate index now refuses the whole request, with its
class, code and detail, where it used to answer with no predicates: send `exclude_predicates: true` for entities
alone. `include_predicates` is now `exclude_predicates`; the old key is refused by name, and so are
`entity_sample_limit` and any other unknown key. An empty body is refused: send `{}`. New request keys: `prefix`, only
with `exclude_predicates`, `cursor`, `min_revision`, which reaches the index.

A negative `examples_per_type` is refused. Reply: `entity_sample_truncated` is now `truncated`; new
`entities_scanned`, `next_cursor`; `total_entities` is the group as listed; `predicate_total` is gone; a page with no
`entities` or `total` key, or an ID that does not parse, is refused transient `internal`; see "The changes that fail
silently".

### `semantic`

Forwards to `graph.embedding.query.search`, bytes unchanged, so `scope` reaches the responder (ADR-071). Reply
enveloped by the responder, `data` holding `results` and `embedder_type`; `query` and `duration` are gone. The
responder, graph-embedding, is in the tree: its changes are in `processor/graph-embedding/MIGRATION.md`, among them
`min_revision` above 0 refused, a limit outside 0 to 100 refused, and unknown keys refused. Refused transient
`internal`, naming that subject, if nothing responds; refused invalid `not_enabled`, naming `semantic`, without a
request unless `embedding_enabled` is true. Fusion reads this.

### `localSearch`

Request keys: `entity_id`, `query`, `level`, `limit`, `min_revision`; any unknown key is refused. `entity_id` must be a
full six-part ID and names the group, at `level` 0 (the default), 1 or 2; the entity need not exist. `query` is the word
index's query (words, quoted phrases, `-` exclusions) and must not be blank; `limit` is 0 (the default, 10) or 1 to
100. Each failure is `invalid_request`, before any read. The old version's limit of 32 terms is `graph.MaxSearchTerms`,
counted by the word index, which refuses a longer query.

Reply enveloped, was bare: `data` holds `community_id` (was `communityId`), `level`, `record_state` and `record`
(`member_count`, `community_summary`, `keywords`, present when `record_state` is `read`), `matches`, `matched`,
`excluded`, `missing`, `bodies_not_indexed`, `truncated` and `answer`. `count`, `durationMs`, `entity_digests`,
`answer_model`, `degraded` and `degraded_reason` are gone. `indexed_revision` is the word index's.

The old version read a group's record, then one page of up to 1,000 entities in ID order and scored them by substring.
Now the word index ranks every entity in the group and the reply holds the best `limit`; a match past a page is no
longer missed. Matches pass as graph-ingest sent them. A group's record only describes it: a missing, unreadable or
not-yet-written record no longer refuses the search, and `record_state` says which. SemStreams' fallbacks to a semantic
search when the group had no record, and to the clustering query that logged and swallowed every failure, are not
carried.

Refusals reach you as they arrived from graph-lexical (such as `index_not_ready` while it rebuilds, and
`revision_not_reached`) and from the `batch` read. Nothing responding is transient `internal`, naming the subject. One
damaged hit no longer refuses the search: the hit is left out and named in the reply's `damaged` field. See "The
changes that fail silently".

### `globalSearch`

Request keys: `query` (required, not blank), `level` (0 to 2, omitted 0; the size of the groups listed), `limit` (0 or
1 to 100, as `localSearch`) and `min_revision`; `max_communities`, `summarize_threshold`, `include_summaries`,
`include_relationships` and `include_sources` are refused `invalid_request` by name, and so is any unknown key. Reply
enveloped, was bare: `data` holds `text`, `hints`, and `entity` or `path` beside an honoured hint. `strategy`,
`temporal`, `spatial`, `entities`, `count`, `duration_ms`, `answer`, `community_summaries`, `degraded` and
`degraded_reason` are gone.

Every question is searched for its words: one word search over every entity, then one `batch` read. `data.text` holds
`level`; `matches`, best first, each `id`, `label`, `score` and `entity` as graph-ingest sent it; `groups`, each group
holding a match once, with `community_id`, `matches` (its count), `record_state` and `record`; `matched`, `excluded`,
`missing`, `bodies_not_indexed`, `truncated`, `unreadable_groups` and `answer`. `indexed_revision` is the word index's.
`unhonoured_hints` is gone; `hints` replaces it.

The old version chose a strategy from the question and refused what it could not answer; no question is refused for
what it asks now. The keyword classifier still reads the question once, and `data.hints` lists each thing it found, with
`hint`, `honoured`, `detail` and, when not honoured, `reason` (and `code` when a refusal caused it):

- `entity`: a question of one word, looked up as `entityByAlias` does (alias first, else a six-part ID). Honoured when
  it
  names an entity, and `data.entity` is the entity verb's reply as it came. An ID or alias holding a word such as
  `today`, `zone-a` or `max` is looked up, not read as a time range or a count. A reference that names no entity is a
  hint not honoured, with `entity_not_found`; the word is still searched.
- `path`: path words. With a start that names an entity ("what is connected to pump-42"), honoured, and `data.path` is
  the
  walk from it, both directions, depth 3, 100 entities, all predicates, IDs and edges only (call `batch` for states). A
  start that names no entity, or path words with no start ("http connection pool"), are not honoured. The start is the
  hyphenated word the classifier finds, never a whole ID; name the start by alias. Only one start is read: "from A to B"
  walks from A.
- `time_range`, `area`, `aggregation`, `similarity`, `ranking`: never honoured, so a question for "the last 2 hours" or
  "top 5" returns matches for its words. The old version answered a time range and an area from the time and place
  verbs, and refused an aggregation, a ranking or a similarity beside a start, time range or area; those refusals are
  gone.
- `classification`: the classifier refused the question (two places, an impossible number such as "the last 0 hours"),
  which is the `detail` and the `code` is `invalid_request`. The question is searched for its words anyway.

A hint's read failing (no alias holder, a broken reply, no responder) leaves it not honoured and never refuses the
search. The classifier reads ordinary words as hints ("max retries" is an aggregation, "pumps in area 51" a path from
`area-51`), so a hint says what it saw.

The text search no longer depends on `COMMUNITY_INDEX`: a missing bucket, a level with no record and an unreadable
record
are `record_state` values (`not_ready`, `none`, `unreadable`) on the group, not refusals. An answer past an unreadable
record sets `unreadable_groups`, counts one error in `Health` and logs one Warn, "global search answered past
unreadable community records". The old version's five-group cap, group scoring and one-page reads are gone, along with
the old match order (a score of 2.0 per summary term, 1.5 per keyword): the word index's BM25 order is the order.

A match's `label` is the first non-empty string value of its most salient label predicate, the ones registered with
`vocabulary.WithAlias(vocabulary.AliasTypeLabel, priority)` (lowest priority first, ties by name), read once when
graph-query starts, so register yours before `Start`; it is omitted when the entity has none. The old version tried
`dc.terms.title` and three agentic predicates, then fell back to the first string value or the ID's last part: an
entity with no label predicate now has no `label`.

Refusals reach you as they arrived from graph-lexical and the `batch` read; nothing responding is transient `internal`.
A reply that cannot fit `max_payload` even with no matches is refused transient `internal`, one error; otherwise the
lowest-ranked matches are left out, `truncated` is true, and
`semengine_graph_query_search_truncated_total{verb="globalSearch"}` rises by one.

Quality: the package's scorecard test holds both search verbs to a plain BM25 over 2,000 sections and 500 questions:
recall at 10 within one point of the reference, and mean reciprocal rank at most 0.01 below it.

### `searchGraph`

Enveloped, `data` a `graph.GlobalSearchData`. It is `globalSearch`, under its own name: the same request gets the same
refusal (see `globalSearch` above), so `summarize_threshold`, `max_communities` and the `include_*` keys are refused
`invalid_request`, naming the key. The old version, when `globalSearch` answered empty or refused `index_not_ready`,
asked the semantic search for 8 entities and answered with strategy `semantic_fallback`, marked degraded.

SemEngine merges the two searches and labels the result. In alpha.5, `searchGraph` with `embedding_enabled` true
answered as `globalSearch` (word order, the word index's `score`); now its order and `score` come from the merge. With
`embedding_enabled` true, `searchGraph` asks the word search and the embedding search together, each for 100
entities, and scores each entity the sum of 1 / (60 + place) over the lists holding it. `data.text.matches` is ordered
by that score (entity ID ascending on ties) and cut to `limit`. `score` is the merged score, at most 2/61, so a caller
sorting by `score` keeps the right order, and a caller comparing it with a BM25 value must read `word_rank` instead.
Each match carries `word_rank` and `vector_rank`, left out when the entity is not in that list. `data.merge` names the
step (`hint` `semantic`, `detail` `merged` or `index_not_ready`, `honoured`, `reason`, `code`); `data.semantic` holds
the embedding search's `data` when it answered. A failed embedding search leaves the word list in word order with
`merge` not honoured; when the word index is not ready and the embedding search fails too, it returns the
`index_not_ready` refusal. A word index that was not ready sends `data.text` as `null`. `strategy` and
`degraded_reason` are gone. A truncated answer counts under `searchGraph`, not `globalSearch`. The graph-query README,
"`searchGraph`'s search by meaning", has the cases.

A stream capturing a declared `graph.query.*` subject refuses boot, and so does a declared NATS-request input
port overlapping one, unless it belongs to graph-query. An undeclared subscriber is not seen. Decode by the verb table:
`QueryVerb.Enveloped` and `QueryVerb.Reply`.

**New names.** For lookups, in `graph`: `ResponderGraphQuery`, `QueryVerb.Enveloped`, `QueryVerb.Reply`,
`MinRevisionField`, `ErrorCodeRevisionNotReached`, `RelationshipEdge`. In `composition`: `TypeRequestSubjectConflict`.
In the new `graphquery` package, the old version's names: `Register`, `DeclarePorts`, `CreateGraphQuery`,
`DefaultConfig`, `Config`, `Component`. For path search, counts and forwards, in `graph`: `ResponderGraphEmbedding`,
`ResponderGraphIndexSpatial`, `ResponderGraphIndexTemporal`; for path search `PathSearchRequest`, `PathSearchData`,
`PathEntity`, `MaxPathSearchDepth`, `MaxPathSearchNodes`, `DefaultPathSearchNodes`; for counts
`HierarchyStatsRequest`, `HierarchyStatsData`, `HierarchyChild`, and `PrefixQueryResponse.Total`; for the forwards
`SemanticData`, `SimilarData`, `SimilarityHit`, `SpatialData`, `SpatialHit`, `TemporalData`, `TemporalHit`.
`SummaryRequest` and `SummaryData` change as the `summary` row says. For the search verbs, in `graph`:
`LocalSearchRequest`, `LocalSearchData`, `LocalSearchRecord`, `RecordState` (`RecordRead`, `RecordNone`,
`RecordNotReady`, `RecordUnreadable`), `GlobalSearchRequest`, `GlobalSearchData`, `GlobalSearchHint`, `TextSearchData`,
`TextSearchGroup`, `TextSearchMatch` and `MaxSearchTerms`. For the time and area requests graph-query no longer sends
and a responder will decode, in `graph`: `TemporalRangeRequest` and `SpatialBoundsRequest`. In `graph/clustering`:
`Community.EntityRevision`, `GroupKey` and `DecodeCommunity`.
