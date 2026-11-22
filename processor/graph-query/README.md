# graph-query

The component that answers the graph's read requests on `graph.query.*`.

## Overview

`graph-query` sits in front of graph-ingest, which holds entities, and graph-index, which holds the relationship and
name indexes. The search verbs also ask graph-lexical, which holds the word index. It writes
nothing. Its one piece of state is a read-only view of the `COMMUNITY_INDEX` bucket, which
holds a record per group of entities and which the search verbs attach to their replies (see below).

`graph.QueryVerbs()` (`graph/query_verbs.go`) decides what it serves: at start it subscribes a handler to each verb
declared for `graph-query`, and fails to start if a declared verb has no handler. Any other `graph.query.*` subject is
left alone, so a consumer can serve its own verb there.

The table below is what graph-query serves. The semantic search (`semantic`, `similar`) is answered by
graph-embedding when the `embedding_enabled` option is true, and refused `not_enabled` otherwise, which is what a
tier 0 composition gets. The time-range and area searches wait for their responders, which are not in the tree.
`MIGRATION.md` lists what a consumer changes at its cutover.

## Verbs

| Verb on `graph.query.` | Answers from | Reply |
| --- | --- | --- |
| `entity`, `batch`, `prefix`, `prefixIDs` | `graph.ingest.query.` same name | bare, as graph-ingest sends it |
| `byName`, `predicate` | `graph.index.query.` same name | enveloped, as graph-index sends it |
| `entityByAlias` | `graph.index.query.alias`, then `graph.ingest.query.entity` | enveloped by `graph-query`, `data` a `graph.ExactEntity` |
| `relationships` | `graph.index.query.outgoing` or `incoming` | enveloped by `graph-query`, `data` a list of `graph.RelationshipEdge` |
| `pathSearch` | `graph.ingest.query.entity` for the start, then `graph.index.query.outgoing` and `incoming` | enveloped by `graph-query`, `data` a `graph.PathSearchData` |
| `hierarchyStats` | `graph.ingest.query.prefix`, one page | bare, a `graph.HierarchyStatsData` |
| `summary` | `graph.index.query.predicateList`, then `graph.ingest.query.prefix`, one page | enveloped by `graph-query`, `data` a `graph.SummaryData` |
| `localSearch` | `graph.lexical.query.search` scoped to one group, then `graph.ingest.query.batch`; the `COMMUNITY_INDEX` view for the group's record | enveloped by `graph-query`, `data` a `graph.LocalSearchData` |
| `globalSearch` | `graph.lexical.query.search` over every entity, then `graph.ingest.query.batch`; the `COMMUNITY_INDEX` view for the groups' records; the alias, entity and edge verbs to verify an `entity` or `path` hint | enveloped by `graph-query`, `data` a `graph.GlobalSearchData` |
| `searchGraph` | the same handler as `globalSearch`; when `embedding_enabled` is true, also `graph.embedding.query.search`, sent with the word search | the same, `data` equal to `globalSearch`'s when `embedding_enabled` is false; when true, one list merging words and meaning |
| `mergedSearch` | `graph.lexical.query.search`; when `embedding_enabled` is true, also `graph.embedding.query.search`, sent with the word search; no entity read | enveloped by `graph-query`, `data` a `graph.MergedSearchData`: ranked entity IDs and scores only |
| `semantic`, `similar` | `graph.embedding.query.search`, `similar` | enveloped by the responder |
| `spatial`, `temporal` | `graph.spatial.query.bounds`, `graph.temporal.query.range` | enveloped by the responder |

The first five, and `semantic`, `similar`, `spatial` and `temporal`, are forwards: the request and reply bytes pass
unchanged, neither decoded nor checked, so `min_revision` reaches graph-index on `byName` and `predicate`, and an
authority read on `entity`, `batch` and `prefix` ignores it. The embedding responder is in this tree (graph-embedding);
the spatial and temporal ones are not in the tree. Until a responder exists, the verb forwarding to it is refused
transient `internal`, naming the subject.

`semantic` and `similar` forward only when `embedding_enabled` is true, which it is not when the key is left out: a
tier 0 composition runs no graph-embedding and needs no setting. A tier 1 product that runs graph-embedding sets it to
`true`. Until it does, each request is refused invalid with code `not_enabled` and a message naming the verb, before
anything is sent. The refusal is the same for every request and
retrying does not help. It counts as one input, not an error, because it describes the composition rather than a
failure of `graph-query`.

A refusal from any verb behind these reaches the caller with its class, code, detail and message. A request that gets
no answer is refused by `graph-query` itself, class transient, code `internal`, naming the subject: nothing responding,
`query_timeout_ms` passing, the handler's context ending (`natsclient`'s limit on a request handler, or the client
closing), or a lost connection.

An enveloped reply can carry `damaged_entities`, the number of damaged entities (stored values
`graph.UnmarshalEntityState` refuses) its producer left out of the answer; the key is absent at 0. Every reply
`graph-query` builds itself (`entityByAlias`, `relationships`, `pathSearch`, `summary`, `localSearch`, `globalSearch`,
`searchGraph`) carries the largest `damaged_entities` among the index replies it used to build the answer, and no such
key when none carried one. A reply it read but set aside, such as a hint it did not honour or an embedding list it could
not merge, adds nothing. Counts are not added, since two reads can count the same entity. A forward passes the count in
the bytes it passes unchanged. A count above 0 means rows may be missing from `data`; the answer about every other
entity stands.

### `entityByAlias`

The request is `{"alias": "...", "min_revision": ...}`, and its bytes go to the alias verb unchanged. An alias with a
holder reads that entity. An alias nobody holds reads the input as an entity ID when it is a valid one, so an ID works
here too. Input that is neither is refused, class invalid, code `entity_not_found`, with no second request.

`data` is the entity verb's JSON as it came, not re-decoded into a struct, so a field `graph-query` does not know
stays. An entity reply that is not JSON is refused transient `internal`. `indexed_revision` is the alias reply's, which
is not the entity's `kv_revision`: it says how far the index had read when it answered.

### `relationships`

The request is `{"entity_id": "...", "direction": "...", "min_revision": ...}`, and its bytes go to the verb for that
direction unchanged. `direction` is required and is `outgoing` or `incoming`; anything else is `invalid_request`, and
no request goes out. An entity with no links in that direction answers `[]`. Each row in `data` has `predicate`,
`from_entity_id` and `to_entity_id`, whichever way it was asked for, and `indexed_revision` is the index reply's.

### `pathSearch`

The request is `{"start_entity": "...", "direction": "...", "max_depth": 3, "max_nodes": 100, "predicates": [...],
"min_revision": ...}`. `start_entity`, `direction` and `max_depth` are required. `direction` is `outgoing`, `incoming`
or `both`. `max_depth` is 1 to 10, and `max_nodes`, the most entities in the reply with the start counted, is 1 to 500
(0 or absent means 100). A value outside a limit is refused `invalid_request`, naming the bound, before any request
goes out; nothing is clamped. `predicates`, when present, keeps only edges holding one of them. An unknown key is
refused.

The walk reads the start through the entity verb, so a start with no record is that verb's `entity_not_found`. It then
asks the index for each entity's edges, nearest first. `data` has `entities` (each with its `depth`, the start at 0),
`edges` and `truncated`. Every entity and edge appears once, and `edges[i]` is the edge that first reached
`entities[i+1]`, named as the graph holds it (`from_entity_id` to `to_entity_id`), whichever way the walk crossed it, so
a path is the edges followed back. `truncated` is true exactly when the walk found an entity it had no room for; false
means `entities` is everything in reach. `indexed_revision` is the smallest of the revisions the hops answered at.

A hop that fails ends the walk with no partial answer; the refusal keeps its class, code and detail, and its message
names the entity being read. That includes a hub: an entity whose edge list in one direction is too large for one
reply is refused `response_too_large`, class invalid, naming that entity, so a caller can find it.
Narrowing `predicates` does not shrink that reply: a hop sends only `entity_id` and `min_revision`, and `predicates`
filters after the hop. Narrowing can only avoid reaching the hub in the first place, and changing `direction` helps
only when it avoids the oversized direction.

Cost is counted in requests: at most one plus `max_nodes`, and twice that for `both`. Nothing bounds the elapsed time of
a walk: the handler's context ends a wait for a reply, but not a wait to write behind a broker that has stopped reading
(ADR-112, "Time").

### `hierarchyStats` and `summary`

Both read one page of entity IDs under a prefix from graph-ingest, so each request costs one scan of that group's keys
and a caller who wants more pays per page. A count covers that page and no more.

- `total_entities` is the whole group as listed, before the cursor and any limit. It is live and as listed, not a
  snapshot: under concurrent overwrites a listing can omit a key. A repeated key counts once.
- `entities_scanned` is the rows on this page. `truncated` is true exactly when `next_cursor` is set, so false says only
  that this is the last page, not that the counts are the whole group. Send `next_cursor` as `cursor` for the next page.
- On a first page with `truncated` true, the type counts, children and examples are of the lexically smallest IDs, a
  sample of the group. A page read with a cursor covers that page.

`hierarchyStats` takes `{"prefix": "...", "cursor": "..."}`; an empty prefix counts the whole graph. Its reply is bare.
`children` are `{prefix, count}` for the prefix one segment longer, by count then prefix. A prefix that is itself a
full entity ID has no children; on the first page of an ID that exists, `entities_scanned` is 1, and for an absent ID,
or a cursor past it, 0. `min_revision` is accepted and ignored, since no index is
read.

`summary` takes `prefix`, `cursor`, `examples_per_type` (default 2; negative is refused), `exclude_predicates` and
`min_revision`. It lists the predicate counts first, then reads the page, so the two are not one snapshot. Predicate
counts cover the whole graph, so `prefix` is refused unless `exclude_predicates` is set. A predicate index that is down
or behind refuses the whole request with its own class, code and detail; `exclude_predicates: true` asks for the
entities alone, and `min_revision` is then ignored. An empty body is refused; send `{}`. `predicates` is `null` when
excluded and `[]` when there are none.

With predicates excluded, `summary` reads no index and reports `indexed_revision` 0, which claims nothing. Do not
compare it with a write's `kv_revision`: it never catches up.

A page graph-query cannot count is refused transient `internal`, naming the subject: one with no `entities` or `total`
key, or a row whose ID does not parse or is not under the prefix. An empty group is an answer, not that refusal.

### Search verbs: `localSearch`, `globalSearch` and `searchGraph`

These three find entities by the words in their text. They ask graph-lexical's word index (`graph.lexical.query.search`,
described in `processor/graph-lexical/README.md`) which entities match, ask graph-ingest's `batch` verb for those
entities, and send them back best match first with the entity bytes exactly as graph-ingest sent them. The query
syntax, the ranking (BM25) and the limits are the word index's: plain words, quoted phrases and `-` exclusions.
graph-query does not rank or score anything itself.

Word search helps only entities that have text, such as documents, notes, descriptions and names. An entity whose facts
are numbers, such as sensor or drone telemetry, has no words to match and gains nothing from these verbs. Reach such
entities through the graph's structure instead: `prefix`, `predicate`, `byName`, `entityByAlias` and `relationships`.

A search that needs a word index to answer needs graph-lexical running. With no responder the verb is refused transient
`internal`, naming the subject. A refusal from graph-lexical (such as `index_not_ready` while it rebuilds, or
`revision_not_reached`) or from the `batch` read reaches the caller with its class, code and detail as it arrived.
Anything the request gets wrong, such as an unknown key, a blank `query`, a `level` outside 0 to 2 or a `limit` over
100,
is `invalid_request` naming the key, before any request is made.

**A damaged hit is named, not fatal.** When the `batch` read names a hit `damaged` (its stored value is refused), the
search leaves that hit out, answers the other hits, and lists the damaged ID in the reply's `damaged` field, in hit
order. A damaged hit is not counted in `missing`, which keeps its meaning (the hit was gone when read back), and it
sets `truncated`.

#### Groups and records

A *group* is every entity sharing the first 4 (level 0), 3 (level 1) or 2 (level 2) parts of an ID, such as everything
under `acme.ops.src.docs`. `graph/clustering` keeps one *record* per group in `COMMUNITY_INDEX`: its member count, a
summary built from statistics, and keywords. `graph-query` keeps a read-only view of that bucket. A record only
describes a group, so it never decides whether a search answers: each reply says in `record_state` what happened when
it looked.

| `record_state` | Meaning |
| --- | --- |
| `read` | The record was read and is in `record`: `member_count`, `community_summary` (omitted when empty), `keywords` |
| `none` | The view holds no record for the group yet |
| `not_ready` | There is no view: the bucket does not exist, or the view is catching up, lost or stopped |
| `unreadable` | The view holds the record but it cannot be decoded |

`record` is present exactly when `record_state` is `read`. `processor/graph-clustering` writes `COMMUNITY_INDEX`, so a
deployment that does not run it answers every search with `record_state` `not_ready` and the same matches. An
`unreadable` record is a declared degrade: the search answers, counts one error in `Health`, and logs one `Warn`.

#### `localSearch`

Searches one group. The request is `{"entity_id": "...", "query": "...", "level": 0, "limit": 10, "min_revision": ...}`.
`entity_id` is a literal six-part ID that names the group: the group searched is the one it belongs to at `level` (0 to
2, absent means 0). The entity does not have to exist. `limit` is 0 (the default, 10) or 1 to 100. An unknown key is
refused.

`data` (a `graph.LocalSearchData`) holds:

- `community_id` and `level`: the group searched.
- `record_state` and `record`: see above.
- `matches`: best first, each `id`, `label` (the first non-empty string value of the entity's most salient label
  predicate, omitted when it has none), `score` (the word index's) and `entity` (the entity as graph-ingest sent it).
- `matched`: how many entities in the group match, before `limit`.
- `excluded`: the query's `-` words, omitted when there are none.
- `missing`: hits deleted between the word search and the read, left out of `matches`.
- `bodies_not_indexed`: entities whose offloaded body the word index did not hold, so their body words are not searched
  yet.
- `truncated`: true when `matched` is above the matches returned, a hit was missing or damaged, the word index's reply
  carried `damaged_entities` above 0 (it may have left a match out), or matches were left out so the reply fits the
  connection's `max_payload`. The matches kept are then the best ones.
- `answer`: a sentence such as `Found 3 matching entities in acme.ops.src.docs.`, then `Showing the first 2.` when
  truncated, then the record's summary when one was read.

`indexed_revision` is the word index's: the latest entity write it had applied when it answered. `min_revision` goes to
the word index unchanged, so a request with a revision from any entity write is held to it, not only a write in this
group; a word index behind it is refused transient `revision_not_reached`.

#### `globalSearch` and `searchGraph`

Searches every entity for the words of a question, and lists the groups that hold the matches. `searchGraph` is the same
handler under a second name, the verb SemSource calls. Its `data` equals `globalSearch`'s for the same request, except
for the search by meaning it merges in when `embedding_enabled` is true (below).

The request is `{"query": "...", "level": 0, "limit": 10, "min_revision": ...}`. `query` is required and not blank.
`level` (0 to 2, absent means 0) picks the group size the matches are grouped at. An unknown key is refused by name,
including SemStreams' `max_communities`, `summarize_threshold`, `include_summaries`, `include_relationships` and
`include_sources`.

`data` (a `graph.GlobalSearchData`) holds:

- `text`: a `graph.TextSearchData`, always there except in a `searchGraph` reply whose word index was not ready
  (below), where it is `null`. An empty `text` means the word index was read and matched nothing. It holds: `level`;
  `matches` as in `localSearch`; `groups`, each group holding a match once, in the order of its best match, with
  `community_id`, `matches` (how many of the reply's matches it holds), `record_state` and `record`; `matched`,
  `excluded`, `missing` and `bodies_not_indexed` as in `localSearch`; `truncated` as in `localSearch`, and also true
  when a read behind an honoured hint (the alias read, or the walk of a `path` hint) carried `damaged_entities` above
  0, since its entity or path is in the reply; `unreadable_groups`, how many listed groups have an `unreadable`
  record; and `answer`, such as `Found 12 matching entities in 3 groups.`
- `entity`: the entity verb's reply data as it came, present only beside an honoured `entity` hint.
- `path`: a `graph.PathSearchData`, present only beside an honoured `path` hint.
- `hints`: what the question asked for beyond its words (empty when nothing).
- `semantic` and `merge`: only in a `searchGraph` reply, when `embedding_enabled` is true (below).

Every question is answered by its words. Nothing is refused for what it asks beyond them. The question is read once with
the keyword classifier (no language model), and each thing it seems to ask for is a *hint* in `hints`:

| `hint` | Raised by | Honoured |
| --- | --- | --- |
| `entity` | A question of one word, read as an alias or a six-part ID | When the word names an entity: `data.entity` holds it |
| `path` | Path words ("what is connected to pump-42") | When the question names a start that exists: `data.path` holds the walk from it, both directions, depth 3, 100 entities, all predicates, IDs and edges only |
| `time_range` | A time range ("in the last 2 hours") | Never: global search does not filter by time |
| `area` | An area ("within 5km of 40.7,-74.0") | Never: global search does not filter by place |
| `aggregation` | A count or other aggregation ("how many", "max temperature") | Never: global search computes no aggregation |
| `similarity` | "similar to" | Never: global search matches words, not meaning |
| `ranking` | "top 5" | Never: matches are ordered by how well their words match |
| `classification` | The classifier refused the question (two places, an impossible number) | Never: the question is searched for its words |

A hint carries `hint`, `honoured`, `detail` when the question named something (the reference, the start, the time range,
the area, the operation), and `reason` for why it was not honoured, plus `code` when a refusal caused it. A reference
that
names no entity, and a read of it that fails, leave the hint not honoured with that refusal's message and code; they
never
refuse the search. Path words with no start are a `path` hint not honoured, with the reason "the question names
no start to walk from".
The list starts with the entity or path hint when there is one.

The keyword classifier reads ordinary words as hints ("max retries" is an aggregation hint, "pumps in area 51" a path
hint with the start `area-51`), so a hint says what the classifier saw, not what the question meant; the matches are the
answer. The classifier takes the first start of "from A to B" and ignores B. To read a known entity use `entity` or
`entityByAlias`; to walk from one use `pathSearch`.

`indexed_revision` is the word index's. `min_revision` goes to the word index, and to the alias read a hint makes.

#### `mergedSearch`

The ranked entity IDs of `searchGraph`'s merge, with no entity read back. Fusion's natural-language resolve calls it:
it needs only IDs and scores, and reading up to 100 entities it would then discard is wasted work. It takes the word
search's request, a `graph.LexicalSearchRequest` (`query`, `limit`, `scope`, `min_revision`), decoded strictly. Before
any request it refuses, `invalid_request`, an unknown key, a blank `query`, a `limit` outside 0 or 1 to 100, and a
`scope` member that is not an entity-ID prefix (an empty member means every entity). `scope` reaches both searches;
`min_revision` reaches the word search alone. The word search needs graph-lexical, but not graph-ingest.

The reply `data` is a `graph.MergedSearchData`:

- `results`: at most `limit` entries, best first, entity ID ascending on ties, each with `entity_id`, `score` and
  `vector_rank`. `vector_rank` is the entity's place in the embedding list, counting from 1, and is left out when the
  entity came from the word list alone or nothing was merged. A reply in which no result has a `vector_rank` holds no
  meaning result, even when `merge` is honoured: the embedding list may have been empty, or all its hits may have
  fallen below `limit`.
- `score`: the merged score (the sum of 1 / (60 + place) over the lists holding the entity) when `merge` is honoured,
  otherwise the word index's score. Compare scores only within one reply.
- `excluded` and `bodies_not_indexed`: the word search's, as `localSearch` reports them; absent or 0 when the word
  index was not ready.
- `merge`: there exactly when `embedding_enabled` is true, a hint entry as in `searchGraph`'s reply. With it absent,
  `results` is the word list alone.

With `embedding_enabled` true, both searches are asked together for 100 entities each, and the results are the first
`limit` of the merge, in the order `searchGraph` gives its matches (a hit missing or damaged in `searchGraph`'s
`batch` read is the one difference, since `mergedSearch` reads none). `mergedSearch` waits for the embedding search at
most `query_timeout_ms` or 4 seconds, whichever is less, so a caller on the default 5 second request timeout keeps
about a second for the word search and the reply. The failure cases:

- **The embedding search is refused, broken, or silent past that wait**, or returns a hit whose ID is not a literal
  entity ID: `results` is the word list, `merge` is not honoured with the reason, and one Warn,
  `mergedSearch answered the word list alone: the embedding search failed`, and one count on
  `semengine_graph_query_search_merge_failed_total` follow, once the reply is encoded.
- **The word search is refused `index_not_ready` and the embedding search answers**: `results` is the embedding list,
  `merge` has `detail` `index_not_ready`, and `indexed_revision` is 0. When both fail, the `index_not_ready` refusal
  is returned.
- **Any other word refusal** is returned as it arrived. A word hit whose ID is not a literal entity ID is the word
  index's broken reply, refused transient `internal`.
- **With `embedding_enabled` false**, no request goes to graph-embedding, `merge` is absent and `results` is the word
  list.

`indexed_revision` is the word search's, and `damaged_entities` is the larger count among the replies read.
`searchGraph`'s own wait for the embedding search is not bounded this way (`query_timeout_ms` only).

#### `searchGraph`'s search by meaning

With `embedding_enabled` false, `searchGraph` makes no request to graph-embedding. With it true, it sends the word
search and `graph.embedding.query.search` together, each for 100 entities whatever the request's `limit` (no
`min_revision` on the embedding request: the embedding verbs claim no entity revision), and answers one list. The
embedding wait is not added to the word wait.

Each entity scores the sum of 1 / (60 + place) over the lists holding it, place counting from 1. The list is ordered by
score, highest first, entity ID ascending on ties, and cut to `limit`. Because both lists are always read 100 deep, the
first ten matches of a limit-100 reply are the matches of a limit-10 reply, unless a hit was missing or the reply was
cut to fit `max_payload`.

In the reply:

- each match carries `word_rank` and `vector_rank`, its place in each list, left out when it is not in that list.
  `score` is the merged score, at most 2/61; compare matches by `word_rank` or `vector_rank`, not by `score`, when you
  need a word-index value.
- `merge` is a hint entry with `hint` `semantic` and `detail` `merged`; `honoured` says whether vectors were used,
  and `reason` and `code` say why not. `semantic` holds the embedding search's `data` as it came (`results`,
  `embedder_type`, `unreadable_records`, `failed_records`).
- `matched` stays the word index's count. `answer` adds `, and N more by meaning` for the N matches with no `word_rank`.
- `truncated` is true when the lists read held more distinct entities between them than `limit` (the word list alone
  when the embedding search failed), `matched` is above the word hits read, a hit was missing or damaged, a reply the
  answer was built from carried `damaged_entities` above 0, or matches were left out to fit. The embedding list has no
  score threshold, so at a small `limit` this is nearly always true.
- `indexed_revision` is the word index's.

When one side fails:

- **The embedding search is refused, broken, or silent past `query_timeout_ms`.** The word list answers in word order,
  each match with `word_rank` only and the word index's `score`. `merge` is not honoured, with the refusal's `code`
  (`internal` for a timeout) and message as `reason`, and no `semantic`. A failed merge is always named, never an
  unlabelled word-only answer. An embedding reply whose `data` has no `results` list (`{}`, `null`,
  `{"results":null}`) is broken; `{"results":[]}` is an empty list and the merge is honoured. Each failed merge also
  logs one Warn, `searchGraph answered the word list alone: the embedding search failed`, with the `code`, the
  failure as `error`, and why answering is safe as `reason`, and adds one to
  `semengine_graph_query_search_merge_failed_total`, once the reply is encoded.
- **The words match nothing.** `text.matches` is the embedding list, each match with `vector_rank` only.
- **The word search is refused `index_not_ready`, and the embedding search answers.** `text` is `null` (the word index
  was not read, so an empty `text` would wrongly say nothing matched), `semantic` holds the answer, `merge` has `detail`
  `index_not_ready`, and `indexed_revision` is 0. When both fail, the `index_not_ready` refusal is returned.
- **Any other word refusal** is returned as it arrived. The embedding request is cancelled and the call waits for it
  to end, not for its reply.

`globalSearch` never asks graph-embedding. Losing the embedding provider leaves the word searches answering, and
`merge` names the provider's refusal (`TestLosingTheProviderKeepsWordSearch`).

#### Quality bar

The package's scorecard test (`TestSearchScorecard`) holds both verbs to a plain BM25 computed over the same 2,000
sections and 500 questions as graph-lexical's scorecard. Recall at 10 may not be more than one point below the
reference's, and the mean reciprocal rank (over the first 100 matches) may be at most 0.01 below it. Both verbs also
have to clear fixed floors of 55% (global) and 58% (local) recall at 10.

### A reply `graph-query` cannot use

An index reply to either verb that does not decode, or names no `producer` (`{}` decodes), is refused, class transient,
code `internal`, naming the subject, so it never reads as no holder or no rows. So is an entity reply to `entityByAlias`
that is not JSON. Nothing else about the entity reply is checked.

## Configuration

An empty configuration is enough. This is the same configuration written out:

```json
{
  "type": "processor",
  "name": "graph-query",
  "enabled": true,
  "config": {
    "ports": {
      "inputs": [
        {
          "name": "graph_queries",
          "required": true,
          "config": {
            "kind": "nats-request",
            "subject": "graph.query.*",
            "interface": {"type": "graph.query", "version": "v1"}
          }
        }
      ]
    },
    "query_timeout_ms": 15000,
    "embedding_enabled": true
  }
}
```

| Option | Type | Default | Description |
| --- | --- | --- | --- |
| `ports` | object | the port above | A `ports` block that is present is used as written; one that is absent takes the default |
| `embedding_enabled` | bool | false | Whether the composition runs graph-embedding. Left out or `null` reads false. Unless `true`, `semantic` and `similar` are refused invalid `not_enabled`, naming the verb, without a request being sent |
| `query_timeout_ms` | int | 15000 | How long `graph-query` waits for the reply to each request it makes to graph-ingest or graph-index, in milliseconds. `0` means the default. Negative is refused, and so is anything above 9223372036854, which does not fit a Go duration. |

The default is above graph-ingest's 10 second budget on `batch` and `prefix`, so the slowest of those can finish. A
larger value gains little past `natsclient`'s limit on a request handler, 30 seconds by default, which also ends the
wait for a reply.

Both are deadlines on that wait, not on the whole request. `nats.go` takes one lock for every write on a connection, so
behind a broker that has stopped reading a request can be held past either one, for as long as a write ahead of it holds
that lock. ADR-112's "Time" section has the measurements. `entityByAlias` makes two requests, and each gets its own
`query_timeout_ms`.

The configuration is decoded strictly: a key not listed here is refused at construction, naming it. In the SemStreams
version `query_timeout` took a number of nanoseconds, and keys nothing read, such as `max_traversal_depth`, were
accepted and dropped. A configuration that still sets one of them fails at boot until the line is changed or deleted.

## Ports

| Direction | Name | Kind | Subject |
| --- | --- | --- | --- |
| input | `graph_queries` | nats-request | `graph.query.*` |

The one input is required and has to be this port: a different name, subject, kind or interface, a `timeout` other
than `1s` (the value it takes when none is written) or `retries`, which nothing would read, a second input, or any
output is refused. Its `description` is yours to set, and so is `external`, which says requests come from outside the
composition. A composition runs one graph-query: a second instance, or another component declaring a NATS-request input
on one of these verbs, is the `request_subject_conflict` error that refuses boot (`composition.Analyze`).

## Health

`Health` is running or stopped, from `component.Lifecycle`. It keeps no state that could leave it degraded, and
`ErrorCount` never lowers `Healthy`.

`DataFlow` counts every request a verb receives, with its bytes, once however many requests it makes downstream; the
replies' bytes are not counted. `ErrorCount`, and `DataFlow`'s `ErrorRate` with it, counts each time `graph-query`
got no usable answer from downstream: a request got no answer, so an outage of graph-ingest or graph-index shows here,
or a reply was broken, so a broken responder shows here too. Each failure counts once. A refused request counts the
failure that refused it, such as a broken or missing reply that `entityByAlias` or `relationships` reads, a broken
word-search or `batch` reply in a search, or a reply that cannot fit `max_payload`. A search that still answers can
count too: one error when it answered past an unreadable group record, and, in `globalSearch`, one when the lookup for
a hint got no usable answer and the hint is reported not honoured. So one request can count more than one: a
`globalSearch` whose hint lookup failed and whose reply then cannot fit `max_payload` counts two. A refusal
the responder sent is an answer and is not counted: "no such entity" is ordinary traffic. Neither is a caller's
`invalid_request`.

## Metrics

Collectors register on the injected `metric.MetricsRegistry`, never on Prometheus' global registry; with none, they
count and register nowhere. A second `graph-query` on one registry counts into the collector the first registered.

| Name | Type | Meaning |
| --- | --- | --- |
| `semengine_graph_query_search_truncated_total` | counter vec | `localSearch`, `globalSearch` and `searchGraph` answers cut short, by `verb` (`localSearch`, `globalSearch`, `searchGraph`) |
| `semengine_graph_query_search_merge_failed_total` | counter | `searchGraph` and `mergedSearch` answers given as the word list alone because the embedding search was refused, broken, or silent past its deadline (`query_timeout_ms` for `searchGraph`; that or 4 s, whichever is less, for `mergedSearch`) |

A search answer is cut short when more entities matched than the `limit` returned, a hit was deleted or damaged
before it was read, a reply it was built from carried `damaged_entities` above 0, or matches were left out to fit
`max_payload`: each sets `truncated` and adds one to the counter. While any entity is damaged, every unscoped search
counts it, so the counter rises with each `globalSearch` and `searchGraph` until the entity is repaired. A refused
request, and an answer that is whole, count nothing.

## Starting and stopping

`Start` needs a NATS client, and its context bounds the subscribing only. It starts one goroutine that keeps a view of
`COMMUNITY_INDEX`, under a run context that `Stop` ends, not `Start`'s context. `Start` neither waits for the bucket nor
fails when it does not exist: until it does, every search answers with `record_state` `not_ready` and every other verb
answers as usual.
While the bucket is missing or its watch is lost, the goroutine tries again every 5 seconds, and the view
reports `not_ready` until a new one has caught up. Each failure is logged at `Warn` when it is the first, differs from
the last, or follows a
view that had opened, and at `Debug` otherwise.

`Stop` drains each verb's subscription, ends the view and returns once every running handler and the view have
returned. If its context ends first, it returns that error and keeps the rest for the next `Stop`. If the view could not
stop its watch cleanly, the `Stop` that waited for it returns that failure, and a later `Stop` returns nil, since there
is nothing left to retry (`TestShutdownFailureKeepsAWatchCleanupFailure`). If a failed `Start` cannot clean up after
itself, `Start` reports both errors and a later `Stop` finishes the job.

## Dependencies

- Registered by a composition root through `Register`; importing the package registers nothing.
- Consumers read `graph.QueryVerbs()` for subjects and spell none as a literal.
