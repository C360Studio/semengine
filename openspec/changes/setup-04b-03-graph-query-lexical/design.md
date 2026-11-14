# Design: setup-04b-03-graph-query-lexical

## Context

Facts: `inventory.md` (INVENTORY PASS at `ef4313c3`, PR #654 comment 6876934851), cited by section, and two probes in
`notes/probes/`: `search-quality-probe` and `query-length-probe`. Local search ranks one prefix page of the group with
a substring rule (§3); global search lets community keywords choose five groups and reads one page of each (§3); the
router refuses or misroutes plain questions (§3, #616); the word index answers scoped, limited, ranked searches with a
pre-limit count (§3) and nothing in graph-query calls it (§4). Measured on the frozen scorecard corpus (500 questions;
R@10, keyword phrasing, `search-quality-probe.log`):

| Search | Global R@10 | Local R@10 |
| --- | --- | --- |
| Plain BM25 reference | 56.0% | 60.0% |
| Word index (global unscoped; local scoped to the group) | 56.0% | 59.8% |
| Today's paths (five keyword-chosen groups, one page each; one page of the group) | 7.0% | 32.6% |

Asked as "what is the ...?": word index 53.6% and 57.4%, today 3.6% and 18.0% (8,000 sections, #10: 4.0%, 22.4%).

## Goals / Non-Goals

Goals: plan item 3, "Local search is the index scoped to the group's ID prefix; global search is the index over
everything, with hits then grouped by community for the answer. Communities stop being the gate ... The router gives
every question a text search and applies a hint only when it can be verified; a trigger word never refuses"; what
search accepts (#624, ruling 6876750265); #563's tier 0 half (ruling 6876750057); a required known-answer test against
a plain reference. Non-goals: change 4 (`BM25Embedder`, `EmbeddingClassifier`, community keywords, graph-clustering's
half of #650); time-range and area answers (no responder until change 7, ledger `:2165`).

## Decisions

**D1. Where words are searched: graph-query asks `graph.lexical.query.search`** (chosen): one index, tokenizer and
ranking, with the readiness graph-lexical already proves. Rejected: a second index in graph-query (a second follower
and copy of every entity's text; two rankings to keep equal); consumers call the verb themselves (each rebuilds
grouping and hydration). Forecloses: search in a deployment without graph-lexical. Seam: without it each search is
refused transient `internal` naming the subject (spec `:875`); README and MIGRATION say so.

**D2. Local search: the word index scoped to the group** (chosen): the group ID of `entity_id` at `level`
(`clustering.GroupKey`, a valid scope, inventory §3), with the group's record attached when the view holds one, never
as a gate. Rejected: the record as gate (graph-clustering is opt-in in SemSource, `cmd/semsource/run.go:894`); the
prefix page, re-ranked (it holds the group's first 1,000 IDs, #506). Forecloses `entities_scanned`. Seam:
`record_state` (`read`, `none`, `not_ready`, `unreadable`) makes a missing community index visible, not an error.

**D3. Global search: one unscoped word search, hits grouped by community** (chosen). Matches keep the word index's
order; `groups` lists each group holding a match by its best match, with count and record. The answer's group is
among the first ten hits' groups for 98.0% of questions, against about 88% by chance alone, since one group holds
1,758 of 2,000 sections. Rejected: scoped searches over groups the records choose (communities stay the gate; 7.0%);
a flat list only (no community answer). Forecloses the five-group cap.

**D4. Matches are the hits read back by one batch request** (chosen), in hit order: `id`, `label` (spec `:791`),
`score`, and `entity` as the batch sent it, which graph-query does not re-encode (the batch decodes and re-encodes
each entity, `processor/graph-ingest/query.go:148`). A hit the batch names missing is dropped and counted. Ten hits
are 20.6 KB median, 100 hits 267 KB at most; today's fit rule stays. Both verbs gain `limit` (0 means 10, 1 to 100).
Rejected: IDs and scores only (every caller batches; SemSource builds matches from entities, `graph_matches.go:107`).
A batch refused because one hit is damaged (#656): (a) the refusal reaches the caller, fatal, naming the entity, so one
damaged hit refuses the search until #656 is fixed in graph-ingest, after which graph-query's `missing` path takes it
unchanged (chosen); (b) on that refusal read each hit on its own, as fusionnats does (`openspec/specs/fusion/spec.md`
`:323-339`): up to 100 more reads on a rare path, and a second copy of a workaround whose home is graph-ingest. PR B
does not wait for #656.

**D5. The router: every question gets a word search; hints are reported, applied only when verified** (chosen). The
word search runs first; its refusal (D6's rules: `pump*`, `the of and`) is the reply and no hint is read. Then the
keyword classifier's hints are listed, each honoured or not with a reason and code; an entity reference or path start
the shared resolver finds adds `entity` or `path` beside `text`. Time range, area, aggregation, similarity and
ranking are never honoured at tier 0; a classifier refusal (two places, an impossible number) is a not-honoured hint.
No hint's trigger word refuses. Rejected: exclusive strategies falling through to text (a verified start drops the
text answer); no classifier (#486's tier 2 classifier joins here). Forecloses: `strategy`, and `temporal`/`spatial`
answers until change 7 adds a verifier per hint kind (the extension point, not built). Retires #616; #535 narrows to
"from A to B" (the classifier drops B; the word search still reads it).

**D6. What search accepts (#624)**: `graph.MaxSearchTerms` 16 to 32, counted once, by the word index (graph-query's
white-space count goes). Measured: SemSource's 46 questions hold at most 10 words; three of ten hand-written pasted
error lines hold 19 to 21, refused at 16 and taken at 32; at 50,000 entities the worst case costs 19.6 ms at 16 words
and 24.5 ms at 32. A term starting and ending in `*` is emphasis (`*bold*` passes; `pump*` stays reserved). Excluded
words are declared in `excluded` (`go test -race` removes `race` silently today). A final `~` (`HEAD~`, the ruling's
example): owner question 2. Rejected: keep 16 (pasted lines refused); drop every reserved form (a later prefix form
would change answers given today); strip and retry (hides the refusal, which the ruling forbids).

**D7. searchGraph and the tier 1 fallback (#563)**. Tier 0, ruled: searchGraph answers as globalSearch and asks no
embedding verb (spec `:858-861`, unchanged). Tier 1 shape, written here: on an empty answer or `index_not_ready`,
with tier 1 on, ask the embedding `search`, answer it under `fallback` with its reason, and return the first answer
or refusal if that fails too. When to build it: owner question 1.

**D8. The quality bar is a required test** (chosen): `TestSearchScorecard` (integration: graph-ingest, graph-lexical,
graph-query) asks the 500 questions through both verbs and fails when R@10 is more than one point below a plain BM25
computed in the test, or below 55% global and 58% local. Rejected: the package scorecard alone (it proves the ranking,
not graph-query's scope, grouping and hydration).

**D9. Two pull requests, A before B** (chosen). A: the query rules (D6; `graph/lexical`, graph-lexical, `graph`). B:
graph-query on the index (D1-D5, D8), smoke (#628 item 2), docs, archive; it reads A's `excluded`. Rejected: one pull
request (one review spanning three packages and two capabilities). #655 changes the prefix read, which search stops
reading: no dependency either way; the second to land merges `origin/main`.

Exported `graph` changes: `LocalSearchRequest` and `GlobalSearchRequest` gain `limit`; `LocalSearchData` and
`TextSearchData` lose `entities_scanned` and `entities_truncated` and gain `matched`, `excluded`, `missing` and
`truncated`, with `record_state` on local search and on each group; `GlobalSearchData` loses `strategy`, `temporal`
and `spatial` and gains `hints`; `TextSearchData` loses `unhonoured_hints`; `LexicalSearchData` gains `excluded`;
`MaxSearchTerms` becomes 32, its comment naming the word index.

## Risks / Trade-offs

- One group holds 1,758 of 2,000 sections, so local's figure shows the scope is right, not what it buys (#620).
- One damaged hit refuses a search until #656 lands (D4a). 99.4% of questions match more than 100 entities and the
  answer is within the first 100 for 78.8%; grouping sees only the returned hits.

## Migration Plan

Consumers migrate (MIGRATION.md): SemSource's `graph_search` drops `summarize_threshold`, reads `data.text.matches`
and `data.hints`, and composes graph-lexical; `localSearch` callers read `matches` and `record_state`.

## Open Questions

1. **Owner question:** build D7's tier 1 fallback (a) with #489's provider and degrade test in 04C, this design being
   the extension point (recommended); (b) now, on `embedding_enabled`, which until change 4 is also true on tier 0
   stacks running `BM25Embedder`, so replies with `embedder_type` `bm25` must be discarded, a guard that is debt from
   the start; or (c) now, on a new `search_fallback` key, one more fact for adopters? With (a), #563 stays open.
2. **Owner question:** your example `HEAD~` (#624). (a) It stays refused, with advice to quote it: a person pasting
   `git reset HEAD~` must retype, and `word~` stays free for a fuzzy form. (b) A final `~` separates words, so `HEAD~`
   searches `head`: a later fuzzy form would change answers given today, and the plan lists typo tolerance as not
   planned. Recommended: (b). The deltas are written for (b); with (a), the reserved-syntax delta keeps `~`.
