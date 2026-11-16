# Inventory: setup-04b-03-graph-query-lexical

base: 125f68238c935056825702052b68ebca8d384af0 (main 80525179 + empty claim commit; worktree .worktrees/claude-graph-query-lexical)
Track: design (touches graph-query, query-classification and lexical-search specs).

Terms. *Word index*: `graph/lexical`, served by component `graph-lexical` on `graph.lexical.query.search`.
*Substring scorer*: graph-query's own text score, which tests each white-space term as a substring.
*Group*: a community, the entities sharing a 4-, 3- or 2-position entity-ID prefix (level 0, 1, 2).
*Router*: `routeGlobalSearch` and the keyword classifier it calls.

## 1. Open issues naming the territory

Queries (2026-11-14, `gh issue list --state open --search <q>`): graph-query, graph/query, classifier, lexical,
searchGraph, "global search", "local search", community, fallback, "tier 1". I read each hit's body and comments.

| Issue | What it asks | Candidate disposition (the design decides) |
| --- | --- | --- |
| #616 | Router misroutes or refuses plain questions on trigger words; owner direction: every question gets a text search, a hint applies only when it can be verified | in scope: retire candidate |
| #506 | Search over a large group reads only one prefix page; done when a match anywhere in the group is found without rescanning | in scope: retire candidate (plan: "dissolves #506") |
| #563 | searchGraph fallback. Ruling 6876750057: none at tier 0; at tier 1 a labelled embedding fallback that never hides the first refusal | in scope: retire candidate |
| #624 | What `ParseQuery` refuses from people and agents (16-word limit, `*bold*`, `HEAD~`). Ruling 6876750265: settle it in this change | in scope: retire candidate |
| #650 | Read text through `graph/entitytext`; graph-query's half is `processor/graph-query/localsearch.go:95` | graph-query half in scope; issue stays open for the `graph/clustering/terms.go:57` half (change 4) |
| #535 | Classifier drops a second place ("from A to B"); accepted loud refusals listed in its ruling comment | in scope where the router changes; retire or narrow |
| #628 | Smoke search checks still read the old path | change 3 owes "move localSearch and global search checks"; stays open for change 4 (bm25 assertions) |
| #486 | Tier 2 LLM classifier; comment says its strategies join global search's routing table | stays (tier 2); the router shape it joins changes here |
| #487 | Tier 2 answer synthesis over graph-query results | stays (tier 2) |
| #489 | Tier-1 HTTP embedder; owes the test that losing it degrades to tier 0 | stays (04C); meets #563's tier-1 half |
| #11 | Epic 04C: fallback or `Deferred` outcomes that weaken neither tier 0 nor tier 1 | stays (epic); bounds #563 |
| #111 | Silent degrades; item 3 (text-tier half) landed in #544; item 4 is spatial | stays; no graph-query item left except the pin's semantic fallback, which #563 now covers |
| #602 | Replies carry no `ENTITY_STATES` generation beside `indexed_revision` | stays; any new reply field this change adds inherits it |
| #578 | graph-ingest prefix read refuses a whole page for one damaged entity | stays (draft PR #655); affects local and text search while they read prefix pages |
| #651, #620 | Tokenizer digit-capital split; NameWeight re-measure on SemSource data | stay (lexical package); bear on the known-answer measurement |
| #477, #68, #195, #196, #155, #536 | Name graph-query files; its sites are already retired (#477 by #524; no `sync.Once` in graph-query production; `component.Lifecycle` at `processor/graph-query/component.go:34`) or are test or flake watch | stay, untouched |
| #9, #10, #24, #84, #108, #109, #141, #300, #350, #484, #485, #502, #554, #619, #621, #625, #630, #641 | Mention the word, not this territory | n/a |

## 2. Overlapping open PRs

`gh pr list --state open`: #654 (this claim, 0 files), #655 (draft, 0 files, `Closes #578`, graph-ingest prefix
read: returns undamaged entities plus a `Missing` list), #597 and #598 (Dependabot, 2 files each: go.mod and go.sum).

- #655 overlaps: `readCountPage` (`processor/graph-query/counts.go:32`) decodes the prefix reply and refuses a page
  with no `entities` or `total`. A new field changes nothing there, but the reply's meaning for local and text
  search changes if pages stay in this design. The design must state merge order.
- #597 and #598: no overlap.

## 3. Current behaviour

### Local search (`processor/graph-query/localsearch.go`)

- `:252` `func (c *Component) localSearch(entitySubject, prefixSubject string) queryHandler`: reads the group record
  from the community view (`:263`), then one prefix page of `record.ID` at `MaxPrefixQueryLimit` (`:276`
  `c.readCountPage(ctx, prefixSubject, record.ID, "")`), then ranks it with the substring scorer (`:280`).
  `EntitiesTruncated` is `page.nextCursor != ""` (`:300`). There is no record for a group yet: it reads the entity
  and answers `index_not_ready` (`:341-349`). Readiness is the community view's alone (ADR-087 port note `:15-17`).
- `:46-73` `checkedSearchTerms` and `searchTerms`: lowercased, split on white space, distinct, 1 to
  `graph.MaxSearchTerms` (16, `graph/query_local_search_types.go:7`).
- `:96` `func (e searchEntity) score(terms []string) float64`: 1.0 when a term equals the ID's type position, 0.5
  when it is a substring of a predicate or a string value. Match threshold `minLocalSearchScore = 0.3` (`:21`).
  Text is every predicate plus every string value, not `entitytext` (#650).

### Global search and searchGraph (`processor/graph-query/globalsearch.go`)

- `:58` `globalSearch`: decode (`:301`, strict: `query`, `level`, `min_revision`), route (`:322`), dispatch to
  entity, path, text, temporal or spatial (`:75-86`).
- `:322-326` One word: entity reference. Otherwise
  `query.NewClassifierChain(query.NewKeywordClassifier(), nil).ClassifyQuery`. This is the only production caller of
  both constructors (gopls references: `globalsearch.go:326` only). `NewEmbeddingClassifier` has no production
  caller.
- `:399` `chooseRoute`: two or more filters, refused; any aggregation hint, refused (`:417-422`); a text-routed hint
  beside a filter, refused (`:438-440`); a path start, `path`; a time range, `temporal`; an area, `spatial`;
  otherwise `text` with `unhonoured`.
- The temporal and spatial routes send to `graph.temporal.query.range` and `graph.spatial.query.bounds`, and no
  component in `processor/` serves either (ls: graph-clustering, -embedding, -index, -ingest, -lexical, -query).
- `:109-132` `globalEntity`: an unresolved or not-found reference falls to text. `:135` `globalPath`: a start that
  is no entity is `entity_not_found` (the `area-is` case in #616).
- Trigger patterns: `graph/query/classifier.go:48-49` (`in zone|area X`, `zone|area X`), `:66-71` (avg/mean,
  sum/total, min/minimum, max/maximum), `:100` `entityPattern` (any hyphenated word), `:55-56` similarity and path
  words.
- `processor/graph-query/query.go:67-71`: `searchGraph` is the same handler ("with no semantic fallback").

### Text tier (`processor/graph-query/textsearch.go`)

- `:50` `searchText`: reads all records at the level; `revision` is their highest `entity_revision` (`:63-65`).
  `textGroups` keeps up to `maxTextGroups = 5` (`:18`) records scoring above 0 on summary or keywords (`:136`
  `groupScore`: 2.0 per summary hit, 1.5 per keyword hit). Then one prefix page per group, ranked by the substring
  scorer. Communities are the gate: a group whose keywords miss is never read (#506).
- The reply `graph.TextSearchData` (`graph/query_global_search_types.go:44-53`) carries groups, matches (id, label,
  score, entity bytes), `entities_scanned`, `entities_truncated`, `unreadable_groups`, `unhonoured_hints`, `answer`.

### The word index's verb

- Request `graph.LexicalSearchRequest` (`graph/query_lexical_types.go:10-15`): `query`, `limit` (0 means 10, else
  1 to `MaxSimilarityLimit` = 100, `graph/query_search_types.go:21,24`; out of range is refused, never clamped),
  `scope []string` (entity-ID prefixes), `min_revision`.
- Reply `LexicalSearchData` (`:29-33`): `results` holds `entity_id` and `score` only (no entity bytes, no label),
  `matched` is counted before the limit, `bodies_not_indexed` is over the whole index.
- Scope: `processor/graph-lexical/query.go:67-74` checks each prefix with `ValidateEntityIDPrefix`; matching is on
  segment boundaries (`graph/id_prefix.go:45` `id == p || strings.HasPrefix(id, p+".")`), applied before the limit
  (graph-lexical spec `:94-96`). A group ID (`graph/clustering/maintainer.go:419-420`: taxonomy, source or
  deployment prefix) is such a prefix: `ValidateEntityIDPrefix` takes 1 to 6 parts (`pkg/types/entity_id.go:190`),
  so a 2-position group ID is a valid scope.
- Order (`processor/graph-lexical/query.go:57-89`, spec graph-lexical `:55-60`): validate (`invalid_request`), then
  `QueryReady`: transient `index_not_ready` (`graph/follower/follower.go:452`), fatal
  `graph_state_reset_required` (`:414`, `:461`), then `revision_not_reached` with detail.
- `ParseQuery` (`graph/lexical/query.go:57`): `ErrQueryTooLong` past 16 distinct tokens after identifier split
  (`:96-97`); `ErrReservedSyntax` for `name:`/`body:` with more text or a final `*`/`~` after a word rune
  (`:125-138`); `ErrEmptyQuery`; `-word` excludes (`:69-73`). All are refused `invalid_request` by the verb (`:75-78`).
  An exclusion is never refused: `go test -race` searches `go` and `test` and drops every entity holding `race`,
  with nothing in the reply that says so. Today only a caller of `graph.lexical.query.search` meets this; no
  graph-query verb sends to `ParseQuery`.

### Docs that state today's search behaviour

- `processor/graph-query/README.md:31-33` (verb table rows for `localSearch`, `globalSearch`, `searchGraph`) and
  `:129-230` (the local search, global search and searchGraph sections).
- `.agents/skills/query-pattern/SKILL.md:79-134` (how code reads the graph, the search verbs among them).
- `docs/admission-ledger.yaml:4346-4396` (the `processor/graph-query` row's contract text for both searches).
- `processor/graph-query/MIGRATION.md:14-30,64-86` (the consumer notes for searchGraph and localSearch).

### Specs this change would change (quoted)

- graph-query `:411-413`: "`localSearch` SHALL read the record … then one `graph.ingest.query.prefix` page … and SHALL
  make no other request when the record is there."
- graph-query `:453-455`: "A term SHALL score 1.0 when it equals the entity's type position, else 0.5 when it is a
  substring … SHALL match only at 0.3 or above."
- graph-query `:575-578` "Global search answers by its hints"; `:594-596` "A count or other aggregation hint … SHALL
  be refused `invalid_request`"; `:638-642` "a question the classifier refuses (an impossible number, an entity beside
  a zone or area phrase) SHALL be refused".
- graph-query `:761-765`: "It SHALL read up to five records scoring above 0 … each with one
  `graph.ingest.query.prefix` page … and no other entity read."
- graph-query `:860-861`: "`graph.query.searchGraph` SHALL decode and answer every request as `globalSearch` does,
  with the same `data`. It SHALL make no semantic request."
- graph-query `:868-870` "Embedding verbs can be turned off": `embedding_enabled` (`processor/graph-query/config.go:33`)
  is the only tier-1 switch graph-query reads today.
- query-classification `:209-211` "The keyword tier accepts any text" (no panic); the requirements at `:161`
  (count/extremum phrases) and `:181` (negated/quoted) bound what the keyword tier reports.
- lexical-search `:155`, `:170`, `:183` (query forms and refusals: #624 territory); `:276-286` scorecard floor
  (`graph/lexical/scorecard_test.go:20` `scorecardFloor = 0.55`).
- composed-stack `:32-33`: "once the lead's level-0 record's `entity_revision` reaches that revision, `localSearch`
  on the lead answers from that group, and a text question on a word of the fixed names answers that group";
  `:98`: the restart check lists "`localSearch` and graph-lexical's `search`" among the answers that include the
  writes. These are the smoke behaviours #628 moves.

## 4. Callers

- gopls references, without the `integration` tag. `GlobalSearchRequest`: only graph-query (`globalsearch.go:109,
  237,301`, `textsearch.go:50`) plus a wire test. `LocalSearchRequest`: only graph-query. `MaxSearchTerms`:
  `graph/lexical/query.go:96-97` and `processor/graph-query/localsearch.go:51-52,68`, two counts of one limit (white
  space against tokens). `LexicalSearchRequest`: `processor/graph-lexical/query.go:58` only.
- String search for the subjects outside graph-query: `internal/smoke` only. `pkg/fusion/fusionnats/client.go:37-43`
  looks up entity, batch, prefix, byName, relationships and semantic, never searchGraph (ledger `:4840`).
  graph-gateway's root resolvers (`gateway/graph-gateway/resolve.go:56-60`) do not include search.
- Smoke: `internal/smoke/smoke_integration_test.go:788` `checkCommunities` drives localSearch and the text search.
  `:515` `checkSearch` asserts `EmbedderType == "bm25"` (`:544`, change 4). `:667` `checkLexical` already asks
  graph-lexical directly. The stack composes graph-lexical (`internal/smoke/stack_integration_test.go:94,153,184`).
- Consumer, SemSource (local checkout 4ca2072c, pins semengine `v0.1.0-alpha.4.0.20261112145006-7bf638643b98`, go.mod:36):
  - `processor/mcp-gateway/query_tools.go:103-107` sends `{"query":…,"summarize_threshold":1}` to
    `graph.query.searchGraph`. SemEngine refuses the key (`processor/graph-query/searchgraph_integration_test.go:64`;
    `MIGRATION.md:22,86`).
  - It decodes the pin's top-level shape (`graph_matches.go:107-114`), and `disclosure.go:29-32` reads the pin's
    `global_search_empty_semantic_fallback` reason.
  - `cmd/semsource/run.go` registers graph-query and graph-clustering and sets `embedding_enabled` false on one path
    (`:790`). It does not register graph-lexical.
- Other consumers. I ran `git grep` over non-test Go for `searchGraph`, `globalSearch`, `localSearch`,
  `graph.lexical.query` and `graph.query.semantic` at each local main. semconnect (dff1265) and semboids (37dbdb0)
  have no hits. semteams (c8571b34) names `globalSearch` only in
  `scripts/generate-semengine-port-manifest.go:280,323` and `scripts/measure-semengine-port-ledger.go:254`, as a
  verb its trimmed gateway drops. None of the three calls a search verb.

## 5. Every current spelling of "which entities match these words"

| Spelling | Where | Text rule |
| --- | --- | --- |
| Word index (BM25F) | `graph/lexical`, `processor/graph-lexical/query.go:57` | `graph/entitytext` |
| `BM25Embedder` (384 hashed slots, cosine) via `semantic` | `graph/embedding/bm25_embedder.go:61` | `graph/entitytext` |
| Substring scorer (local and text tier) | `processor/graph-query/localsearch.go:96` | its own: every predicate and string value |
| Group score on summary and keywords | `processor/graph-query/textsearch.go:136` | community record fields |
| Community keywords | `graph/clustering/terms.go:57` `entityWords` | its own (#650, change 4) |
| Exact name lookup | `graph.index.query.byName` (`graph/query_verbs.go:91`) | name index |

Problem shape: *route, then read through a declared verb and enrich*. The closest existing instance is graph-query's
own composed verbs that ask one verb and then hydrate. `pathSearch` and `globalEntity` (`globalsearch.go:109-132`)
resolve, then read the entity. Hydration by `graph.ingest.query.batch` exists (fusionnats `client.go:39`). Whether
this design adopts that shape is for the design.

## 6. Governing ADRs

This change carries no package from the pin; it edits already-ported surface.

- ADR-062 (`docs/adr/062-deterministic-graph-fusion.md:97`): fusion's `BM25` mapped to `graph.query.searchGraph`;
  `:168` "Semantic (`globalSearch`/`searchGraph`) covers NL".
- ADR-071: scope on semantic search reaches the responder (`:104`).
- ADR-087 port note `:15-17`: localSearch is ready per request from its own `COMMUNITY_INDEX` view alone.
- ADR-099 port note `:39-40`: how graph-query reads records during a tick is not decided there. Group identity is
  derived from the ID (`:30-31`, consumer breaks go to `MIGRATION.md`).
- ADR-088 and ADR-109: readiness is per producer, and aggregation is the consumer's. That bears on graph-query
  depending on graph-lexical's readiness beside the community view's.
- ADR-060: classified RPC errors (class, code, detail) carried through.
- Setup plan `docs/setup-plan.md:77-80`: lexical BM25 is tier 0; tier 1 "losing the provider degrades to tier 0
  with search intact". `:446-450`: 04B qualifies SemSource's lexical paths without a model service.

## 7. Adopter seam inventory

Who: (a) a SemSource developer calling `graph_search`; (b) a deployment author composing graph-query; (c) a person or
agent typing a question.

1. What they must know today.
   - (a): the strict request keys, the reply nesting (`data.text.matches`), and which source answered. SemSource
     calls that source a "rung" (`disclosure.go:8-17`: entity hits only, community summaries, or an LLM answer).
   - (b): whether tier 1 is on, which graph-query reads from `embedding_enabled` (`config.go:33`). If the design
     makes graph-query send to graph-lexical (not decided), a deployment without graph-lexical would meet a
     no-responders refusal per request, transient `internal`, the pattern in graph-query spec `:875`
     (`TestNoRespondersIsTransient`).
   - (c): local search takes at most 16 distinct white-space terms (`localsearch.go:51-52`); a longer query is
     refused `invalid_request`. It has no reserved syntax and no exclusion; `*`, `~`, `-` and `name:` are plain
     characters in a substring. Global search refuses on the same count in the text tier (`textsearch.go:51-53`) and
     on router hints: an aggregation word, two filters, or a hint beside a filter (`globalsearch.go:399-440`).
     A one-word question is an entity reference (`:323-324`).
2. If they do nothing.
   - (a): fails loudly today (key refused). If it only drops the key, it gets no matches with no error
     (`MIGRATION.md:16-22`).
   - (c): an ordinary question with a trigger word is refused, or routed to a walk that answers `entity_not_found`
     (#616). A query sent straight to `graph.lexical.query.search` with `-word` silently drops entities holding that
     word (section 3), an undeclared degrade.
3. Where they find out.
   - (a): a typed runtime error, then a doc.
   - (b): a typed runtime error per request, not at boot (`git grep graph-lexical processor/graph-query` returns
     nothing).
   - (c): a typed error, except the silent exclusion, which they find out about nowhere.
4. What they should have to know is the design's question. Recorded gap: the caller predicts tier, limit and
   grammar facts the framework holds.

## 8. Open questions for the design, each with its settling measurement

1. Quality bar for graph-query search: no known-answer test exists for local or global search (`git grep -i
   scorecard processor/graph-query internal/smoke` returns nothing). Settle by running the existing scorecard corpus
   (`graph/lexical/testdata/scorecard/`) through local and global search against a plain BM25 reference. Report R@1,
   R@10 and MRR beside the package's numbers. SemSource's 33 questions, if the runner reaches it (#10 plan).
   Baselines to beat, from the #10 plan comment (6833147079) and the archived probes: global text search through
   community records R@1 2.0%, R@10 4.0% (`notes/probes/community-probe.log:10`); the substring rule over the whole
   corpus R@1 9.4%, R@10 22.4% (`notes/probes/retrieval-probe.log:13`); the plain reference R@1 14.0%, R@10 38.8%
   (`:4`).
2. Hydration cost: hits carry only ID and score, but today's replies carry entity bytes and labels. Measure the
   batch read of 10 to 100 hits against `max_payload`, using the existing `fitTextSearch` and `fitLocalSearch`.
3. Global grouping: are hits grouped by computing `clustering.GroupKey` from the ID, or by reading records? Measure
   whether a hit's group can lack a record (before the first tick), and what the reply says then.
4. Limit: 100 hits maximum per lexical request. Measure how often `matched` exceeds 100 on the scorecard corpus for
   global questions, which decides whether grouping sees enough.
5. Router: re-run `router-probe.go.txt` (archived `openspec/changes/archive/2026-11-12-setup-04b-01-lexical-index/
   notes/probes/`) under each candidate rule. List every refusal and every hint left unhonoured for the 27 questions
   plus #535's.
6. #624 bound: measure the token counts of SemSource's questions and of a sample of pasted error lines and stack
   frames, to choose the bound. Count how often `*`, `~` and `name:` appear in agent-sent queries, if SemSource logs
   are available.
7. Tier-1 signal for the #563 fallback. Already settled: with `embedding_enabled: false`, `semantic` and `similar`
   refuse invalid `not_enabled` without sending (spec `:880-884`, `TestNotEnabledRefusesBeforeForwarding`). Still
   open: which signal means "tier 1 is on" for the fallback, `embedding_enabled` or the embedding verb's own answer.
   Measurement: with `embedding_enabled` true and no graph-embedding, what the embedding verb's refusal is (transient
   `internal`, `TestNoRespondersIsTransient`, spec `:875`), and whether a fallback reached that way would show the
   first refusal or the second. Read both from the integration tests named.
8. Readiness: while graph-lexical is not ready but the community view is (or the other way round), which
   refusal does each verb give? Read it from an integration test, not by reasoning.
9. Whether the temporal and spatial routes stay while no responder serves them (#616 last row). Today a time range
   with no responder is refused transient `internal` (`searchgraph_integration_test.go:66`); those responders arrive
   in change 7 (`docs/admission-ledger.yaml:2165`). Measurement: the count of router-probe questions that route
   temporal or spatial. I counted the archived `router-probe.log`: 3 of 27 route temporal ("what failed pumps were
   reported last week", "what happened today", "pump failures today") and none route spatial.

Searches that closed categories empty: no production caller of `NewEmbeddingClassifier`; no graph-query reference to
`graph-lexical` or `graph.lexical.query`; no consumer in this repo of `localSearch`, `globalSearch` or `searchGraph`
outside graph-query and smoke.

Skipped: no gopls run under `-tags integration`, so integration-test callers came from string search. SemSource,
semconnect, semteams and semboids were read at their local main only.
