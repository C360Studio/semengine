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

Phrased as "what is the ...?", the word index holds (53.6% and 57.4%) and today's paths fall to 3.6% and 18.0%. The
archived 8,000-section baselines (#10 comment 6833147079) were 4.0% global and 22.4% for the substring rule.

## Goals / Non-Goals

Goals: plan item 3, "Local search is the index scoped to the group's ID prefix; global search is the index over
everything, with hits then grouped by community for the answer. Communities stop being the gate ... The router gives
every question a text search and applies a hint only when it can be verified; a trigger word never refuses"; what
search accepts from people and agents (#624, ruling 6876750265); #563's tier 0 half (ruling 6876750057); a required
known-answer test against a plain reference. Non-goals: change 4 (`BM25Embedder`, `EmbeddingClassifier`, community
keywords, graph-clustering's half of #650); time-range and area answers (no responder until change 7, ledger `:2165`).

## Decisions

**D1. Where words are searched: graph-query asks `graph.lexical.query.search`** (chosen): one index, one tokenizer,
one ranking, the readiness graph-lexical already proves. Rejected: a second index inside graph-query (a second
follower and copy of every entity's text; two rankings to keep equal); no graph-query search, consumers call the verb
(drops the community answer; every consumer re-builds grouping and hydration). Cost: a deployment that serves local or
global search composes graph-lexical. Seam: without it, each search is refused transient `internal` naming
`graph.lexical.query.search` (the no-responders rule, graph-query spec `:875`); documented in README and MIGRATION.

**D2. Local search: the word index scoped to the group** (chosen). The scope is the group ID of `entity_id` at `level`
(`clustering.GroupKey`; a 2- to 4-part prefix, valid scope, inventory §3). The group's record, when the view holds one,
is attached; it never gates. Rejected: keep the record as a gate (graph-clustering is opt-in in SemSource,
`cmd/semsource/run.go:894`, so word search would need a component it does not otherwise want); keep the prefix page
and re-rank it (the page holds the group's first 1,000 IDs, #506). Forecloses: `entities_scanned` (nothing is scanned).
Seam: `record` says `read`, `none`, `not_ready` or `unreadable`, so a missing community index is visible, not an error.

**D3. Global search: one unscoped word search, hits grouped by community** (chosen). The flat match list keeps the word
index's order; `groups` lists each level-`level` group among the matches in order of its best match, with its match
count and record when read. Measured: the answer's group is among the first ten hits' groups for 98.0% of questions.
Rejected: scoped searches over groups the records choose (communities stay the gate; 7.0%); a flat list only (drops
the plan's community answer). Forecloses: the five-group cap and `unhonoured_hints` (moved to D5's `hints`).

**D4. Matches are the hits read back as stored** (chosen): one `graph.ingest.query.batch` request for the hits, in hit
order, each match `id`, `label` (unchanged rule, spec `:791`), `score` (the word index's) and `entity` (the stored
bytes); a hit the batch reports missing (deleted since) is dropped and counted in `missing`. Measured: ten hits are
20.6 KB median, 100 hits 214 KB median, 267 KB max, under 1 MiB; today's fit rule stays, setting `truncated`. Both
verbs gain `limit` (0 means 10, 1 to 100, the word index's bounds). Rejected: IDs and scores only (every caller
then batches; SemSource builds its matches from the entities, `graph_matches.go:107-130`). A batch refusal reaches
the caller intact: one damaged hit refuses the search, as the batch verb does today
(`processor/graph-ingest/query.go:130-137`); filed as a defect beside #578, not fixed here.

**D5. The router: every question gets a word search; hints are reported, applied only when verified** (chosen). The
keyword classifier runs as today; its result becomes `hints`, each `hint`, `detail`, `honoured` and, when not,
`reason` and the refusal `code`. Verified: a one-word reference or a path start that the shared resolver finds
(`entity` or `path` is added beside `text`). Never honoured at tier 0: time range and area (no responder), aggregation,
similarity, ranking. A classifier refusal (two places, an impossible number) becomes a not-honoured `classification`
hint. No question is refused for its words; only a broken request, an ended context or unreadable hints are.
Rejected: exclusive strategies that fall through to text (a verified start would drop the text answer); no classifier
(#486's tier 2 classifier needs the table). Extension point, not built: a hint verifier per hint kind, where change 7's
time range joins. Retires #616; #535 narrows to "from A to B" (the classifier drops B; the text still searches it).

**D6. What search accepts (#624)** (chosen): `graph.MaxSearchTerms` 16 to 32, one count, the word index's (graph-query's
white-space count goes). Measured: SemSource's 46 questions hold at most 10 words; three of ten pasted error lines hold
19 to 21 and are refused at 16, none at 32; at 50,000 entities the worst case costs 19.6 ms at 16 words, 24.5 ms at 32.
A term both starting and ending in `*` is emphasis, not a prefix (`*bold*` passes; `pump*` stays refused); `~` after a
word stays refused, its message saying to quote it. An exclusion is declared: the reply's `excluded` lists the words
removed (`go test -race` excludes `race` silently today). Rejected: keep 16 and every rule (pasted lines refused);
drop the reserved forms (a later prefix or fuzzy form would change answers given today); strip and retry (hides the
refusal, which the ruling forbids).

**D7. searchGraph and the tier-1 fallback (#563)**. Tier 0, ruled: searchGraph answers as globalSearch and asks no
embedding verb (spec `:858-861`, unchanged). Tier 1 shape, written here: on an empty text answer or `index_not_ready`,
with tier 1 on, ask the embedding `search`, answer it under `fallback` with its reason, and return the first answer or
refusal if that fails too. Options for when to build it: (a) with #489's provider and degrade test in 04C, this design
being the extension point (recommended); (b) now, on `embedding_enabled`: until change 4 removes `BM25Embedder`, a
tier 0 stack with graph-embedding has `embedding_enabled` true and would fall back to hashed slots unless a reply with
`embedder_type` `bm25` is discarded, a guard that is debt from day one; (c) now, on a new `search_fallback` key (one
more fact for adopters). **Owner question 1.**

**D8. The quality bar is a required test** (chosen): `TestSearchScorecard` (integration, graph-ingest, graph-lexical,
graph-query) loads the frozen corpus, asks the 500 questions through `localSearch` and `globalSearch`, and fails when
R@10 is more than one point below a plain BM25 computed in the test, or below 55% global and 58% local. Rejected:
the package scorecard alone (proves the ranking, not graph-query's scope, grouping and hydration).

**D9. Two pull requests.** A: the query rules (D6; `graph/lexical`, `processor/graph-lexical`, `graph`). B: graph-query
on the index (D1-D5, D8), smoke (#628 item 2), README, MIGRATION, skill text, ledger, archive. #655 touches
graph-ingest's prefix read, which search stops reading; neither depends on the other, and the second to land merges
`origin/main`.

## Risks / Trade-offs

- The scorecard corpus has one group of 1,758 of 2,000 sections, so local's figure shows scoping is right, not what it
  buys; SemSource's corpus is the real test (#620).
- One damaged hit refuses a search (D4) until the batch defect is fixed.
- Matches above 100 are never seen: 99.4% of questions match more than 100 entities; the answer is within the first
  100 for 78.8%. The limit is the word index's; grouping sees only the returned hits.

## Migration Plan

Greenfield; consumers migrate (MIGRATION.md). SemSource's `graph_search`: drop `summarize_threshold`, read
`data.text.matches` and `data.hints`, compose graph-lexical. `localSearch` callers read `matches` and `record`.

## Open Questions

1. **Owner question:** D7, build the tier-1 fallback now (b or c) or with #489 (a)? With (a), #563 stays open.
2. **Owner question:** ADR-087 Decision 5's port note says local search is ready from its own community view alone;
   with D2 it is ready from graph-lexical's answer and the view only decorates. The note changes in PR B.
