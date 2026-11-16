# Design: setup-04c-02-hybrid-search

## Context

Premises are pinned in `inventory.md`. The owner's rulings: merge in searchGraph first, floor on the merged row
(about 60%) after (#668 comment 6920821032, #666 comment 6920823901). The scorecard takes any embedder and names the
model it measured (#666). It runs as a Task target before each tier 1 release, with no real provider in verify
(archived `2026-11-16-setup-04c-01-http-embedder/design.md`, Open Question 2). At tier 1 the merge replaces the
fallback-only behaviour #563 ruled. Estimated size: PR A ~150 production + ~350 test lines; PR B ~400 tool lines + ~150
test.

## Options considered

**Where the merge lives.** (1) In searchGraph (chosen: graph-query is the one component that already asks both,
`processor/graph-query/query.go:70-73`). (2) In graph-lexical or graph-embedding: would make one index call the
other, a cross-index dependency neither has. (3) In the consumer: the ranking the floor guards would be no product's.
(4) Do nothing: the floor would guard a ranking nobody gets (#668 ruling).

**How the reply labels the source.** (a) Per-match ranks inside `text.matches` plus one `merge` entry (chosen). (b) A
third list `merged` beside `text` and `semantic`: each entity's bytes would be sent twice, and callers would have to
choose a list. (c) A source tag per match (`["words","vectors"]`): carries less than ranks, and ranks are what the
merge used.

**Where the scorecard runs.** (i) A harness program run by a Task target (chosen, D6). (ii) An integration test in
graph-query that verify runs with a stand-in embedder and the Task target re-runs with `-args`. Its verify half
measures nothing about quality, and a floor that applies only when a flag is set is a skip in another form. (iii) A
pure-function scorecard with no NATS: scores the merge function, but not the reply searchGraph builds (depth, batch
read, missing entities, payload fit).

## Decisions

**D1. The merge** (PR A). With `embedding_enabled` true and the word search answered, searchGraph:

1. Asks the word search and the embedding search, each with limit `graph.MaxSimilarityLimit` (100,
   `graph/query_search_types.go:24`), whatever the request's limit. Fixed depth makes the merged list at limit n
   exactly the first n of the merged list at limit 100. So the scorecard's R@10, measured at 100, is the ranking a
   limit-10 caller gets.
2. Scores each entity in either list Σ 1 / (60 + place), place counted from 1, over the lists holding it. Sorts by
   score, highest first, entity ID ascending on ties. Keeps the first `limit` (0 means 10).
3. Reads those entities with one batch request (as `readMatches` does, `matches.go:81`). Missing ones are left out and
   counted.

k = 60 is a constant, not configuration: it is the value the probe measured, and no caller has asked to tune it.
Invariants, each with a spec scenario in the delta (requirement "searchGraph merges words and meaning"): every
entity once; order by score then ID; `word_rank`/`vector_rank` equal the entity's place in each list, absent when not
in it; limit n is a prefix of limit m > n; an empty embedding list gives the word list's order.

Proof: `TestMergeRanks` (unit; a hand-computed table plus a generated check against a reference model written in the
test: a map sum and a full sort, `pgregory.net/rapid`, seed recorded), `TestSearchGraphMergesWordsAndMeaning`,
`TestSearchGraphLimitIsAPrefix`, `TestSearchGraphMeaningAloneWhenWordsMatchNothing` (integration, fake embedders
giving a known vector order).

**D2. The reply** (PR A). `GlobalSearchData.Fallback` becomes `Merge` (JSON `merge`), the same `GlobalSearchHint`:
`hint` `semantic`, `detail` `merged` or `index_not_ready`, `honoured`, `reason`, `code`. `Semantic` is unchanged:
the embedding reply's data as it came (`embedder_type`, `failed_records`, which the merge does not carry). Each
`TextSearchMatch` gains `word_rank` and `vector_rank` (1-based, omitted when absent). In a merged list `score` is the
merged score; elsewhere it stays the word index's. `matched` stays the word index's count. The answer template adds
`, and {extra} more by meaning` before the full stop when the embedding list added entities the word hits did not
hold. `truncated` is true when the union held more than was sent, `matched` is above the word hits read, a hit was
missing, or the reply was cut to fit. `indexed_revision` stays the word index's: the embedding verbs claim none.

**D3. When one side fails** (PR A). Observable behaviour and proof:

| Case | Reply | Proving test |
| --- | --- | --- |
| `embedding_enabled` false | As globalSearch, no embedding request | `TestSearchGraphTierZeroAsksNoEmbedding` (kept) |
| Embedding search refused or broken | Word answer in word order, `word_rank` only; `merge` not honoured with `code` and `reason` | `TestSearchGraphNamesAFailedMerge` |
| Embedding search does not answer within `query_timeout_ms` | Same, `code` `internal` | `TestSearchGraphMergeTimeoutKeepsWordAnswer` |
| Word search refused `index_not_ready`, embedding answers | `text` null, `semantic` holds the answer, `merge` honoured with detail `index_not_ready`, revision 0 (unchanged path, `textsearch.go:120-128`) | `TestSearchGraphIndexNotReadyLeavesTextOut` (field renamed) |
| Both fail | The `index_not_ready` refusal, as it arrived | same test |
| Any other word refusal | Returned, the embedding answer discarded | `TestSearchGraphOtherRefusalIsReturned` |
| Provider closed mid-run | B found by its words; `merge` not honoured, `code` set; never a refusal | `TestLosingTheProviderKeepsWordSearch` (updated) |

**D4. The embedding wait does not add to the word wait** (PR A). Both searches are sent before either reply is
awaited, so a slow provider costs at most one `query_timeout_ms` over the word search and stays inside natsclient's
30 s handler budget (`natsclient/request.go:363`). Proof: `TestSearchGraphAsksBothAtOnce`. Each fake responder
answers only after it has seen the other's request, so a reply arrives only if both were in flight. No clock.

**D5. globalSearch and localSearch stay words only.** globalSearch is the scorecard's words row and tier 0's search.

**D6. The scorecard program** (PR B). `internal/harness/scorecard`, a `main` like `mutcheck` and `pindiff`,
run by `task scorecard:tier1 -- -url <base> -model <name> [-query-prefix <s>] [-api-key-env <VAR>] [-provider <note>]`.
The Task target's description says: run by hand before each tier 1 release, never by verify or CI. It:

- starts NATS through `natsfixture` from the one image pin (`natsfixture.New` takes an interface of the four methods
  it uses; the program supplies them and runs the cleanups on exit);
- composes graph-ingest, graph-index, graph-lexical (defaults), graph-embedding (registered with
  `RegisterWithEmbedder` over `httpembedder.New`, `text_suffixes: [".content"]`) and graph-query
  (`embedding_enabled: true`), as a product composes them;
- writes the 2,000 sections from `graph/lexical/testdata/scorecard/` with the title and content predicates
  `TestSearchScorecard` uses;
- waits for the word index by `min_revision`. It waits for vectors by watching `EMBEDDING_INDEX` until each section
  has a record generated by this embedder; a failed record ends the run naming its reason. Each wait is on an event,
  never a clock alone;
- asks each question through `globalSearch` (words), graph-query's `semantic` verb (vectors) and `searchGraph`
  (merged), each at limit 100, and scores R@1, R@10, MRR as `TestSearchScorecard` does;
- prints: Go version; SemEngine commit and whether the tree was clean (passed in by the Task target from git); the
  provider URL and the `-provider` note; the configured model and the model the provider's reply names (one plain
  request; "none" when the reply names none); dimensions; the query prefix, quoted; the text embedded (`.content`);
  how many texts graph-embedding cut (its truncation metric, on the registry the program owns); section embed time;
  question time, total and median; the three rows; the floor verdict.
- exits 1 below the floor, 2 on any other failure.

It scores searchGraph's own reply, so it measures the engine's merge, not a copy. It accepts any provider that
speaks the OpenAI embeddings API, which `httpembedder` checks. An in-process model is not a tier 1 option (owner,
2026-11-14). The tool's unit tests (flag refusals, the tally arithmetic against hand-computed ranks) join the tool
packages in `scripts/test-pkgs.sh:19`.

**D7. Why sections are embedded by body only.** In this test set a section's `name` is the title of the file it sits
in, so every section of one file starts with the same words. Embedded with that title, sibling sections look more
alike than they are: vectors find the answer in the first ten 47.6% of the time with it and 52.0% without (archived
probe note, "What the numbers say"). A real entity's name is its own, so this is a property of the test set. #683
measures real entity text. Word search keeps its default rule (name and body), as the committed word scorecard and
the audit's 62.4% row do. No body is cut by graph-embedding: bodies run to 4,000 bytes and its default cap is 4,000
characters (`processor/graph-embedding/config.go:20`).

**D8. The floor** (PR B; a proposal until measured). After PR A lands, the first run on
`Snowflake/snowflake-arctic-embed-s` through semembed sets the values. Proposed: merged R@10 at least 60%, merged MRR
at least 0.40, and merged R@10 not below either single row (#668's "done when"). The floor names the model it was
measured on. Another model must show it holds by its own run. The run's output goes in
`notes/probes/scorecard-<model>-<semembed commit>.md` and the release evidence.

**D9. Two PRs, A then B.** B needs A's merge to score. PR A: `Addresses #668`. PR B: `Closes #668` (its "done when"
is the merged row on the scorecard) and `Closes #666`. Dependabot #597 (testcontainers) and #598 touch only
`go.mod`/`go.sum`; #597 should land before PR B, which edits natsfixture, and whichever lands second merges
`origin/main`.

## Risks / Trade-offs

- semembed cuts every text at 512 tokens (semembed#6), so the vectors row understates the model; the floor is measured
  with the cut and is rerun when semembed changes. If semembed adds the query prefix itself (semembed#5), run with
  `-query-prefix ""`, or each query carries it twice; the printed prefix makes a double one visible.
- Every searchGraph at tier 1 now embeds the question (one provider call). A down provider costs at most
  `query_timeout_ms` per question, never the answer.
- Reading 100 deep on each side costs a word search and a vector scan of 100 hits instead of `limit`; the batch read
  stays at `limit`.

## Migration Plan

searchGraph callers read `merge` instead of `fallback`, and a match's `word_rank`/`vector_rank` for its source.
MIGRATION.md's searchGraph notes (`:30-36`, `:181`, `:329-335`) are rewritten for the merge.

## Open Questions (owner only)

1. **Where the tier 1 scorecard runs (D6).** Recommended: the harness program behind `task scorecard:tier1`, which
   needs `natsfixture.New` to take a four-method interface. Alternative: a graph-query integration test that runs in
   verify with a stand-in embedder and checks quality only under `-args`.
2. **The floor (D8).** Recommended: merged R@10 at least 60%, MRR at least 0.40, merged not below either single
   row, set from the first engine run and named for its model.
3. **The reply (D2).** Recommended: rename the unreleased `fallback` to `merge`, add `word_rank`/`vector_rank`,
   and make `score` the merged score in a merged list.
