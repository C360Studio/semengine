# Design: setup-04c-02-hybrid-search

## Context

Premises are pinned in `inventory.md`. Owner rulings: merge in searchGraph first, floor on the merged row (about 60%)
after (#668 comment 6920821032, #666 comment 6920823901). The scorecard takes any embedder; the same run that sets
tier 1's floor is how another model is shown to hold it, and the result names its model (#666). It runs as a Task
target before each tier 1 release, with no real provider in verify (archived setup-04c-01 design, Open Question 2).
At tier 1 the merge replaces the fallback-only behaviour #563 ruled. Size: PR A ~150 production + ~350 test lines;
PR B ~400 + ~150.

## Options considered

**Where the merge lives.** (1) searchGraph (chosen: the one component that already asks both,
`processor/graph-query/query.go:70-73`). (2) An index component: one index would call the other. (3) The consumer:
the floor would guard no product's ranking. (4) Nothing: the same (#668 ruling).

**How the reply labels the source.** (a) Ranks per match in `text.matches`, plus one `merge` entry (chosen). (b) A
third list beside `text`: each entity's bytes sent twice. (c) A source tag per match: less than ranks carry.

**Where the scorecard runs.** (i) A harness program run by a Task target (chosen, D6). (ii) A pure-function scorecard
with no NATS: misses the reply searchGraph builds (depth, batch read, missing entities, payload fit). A verify test
cannot hold it: verify must not reach a provider, and no test may skip.

## Decisions

**D1. The merge** (PR A). With `embedding_enabled` true, searchGraph:

1. Sends the word search and the embedding search, each with limit `graph.MaxSimilarityLimit` (100), whatever the
   request's limit. Fixed depth makes the ranking at limit n the first n of the ranking at 100, so the scorecard's
   R@10 at 100 is the ranking a limit-10 caller gets.
2. Scores each entity Σ 1 / (60 + place), place from 1, over the lists holding it. Sorts by score, highest first,
   entity ID ascending on ties. Keeps the first `limit` (0 means 10).
3. Reads those with one batch request (`matches.go:81`). Missing ones are left out and counted.

k = 60 is a constant: the value measured, and no caller asks to tune it. Invariants (spec "searchGraph merges words
and meaning", "A merged match says which lists held it"): every entity once; order by score then ID; ranks equal each
list's place, absent when not in it; the ranking at n is a prefix of the ranking at m > n; an empty embedding list
gives the word order. `text.matches` keeps the prefix only when no hit is missing and the reply fits. Proof:
`TestMergeRanks` (a hand table plus a generated check against a map-sum-and-sort model in the test, `rapid`, seed
recorded), `TestSearchGraphMergesWordsAndMeaning`, `TestSearchGraphLimitIsAPrefix`,
`TestSearchGraphMeaningAloneWhenWordsMatchNothing`.

**D2. The reply** (PR A).

- `Fallback` becomes `Merge` (JSON `merge`), the same `GlobalSearchHint`: `hint` `semantic`, `detail` `merged` or
  `index_not_ready`, `honoured`, `reason`, `code`.
- `semantic` is unchanged (it carries `embedder_type` and `failed_records`).
- Matches gain `word_rank` and `vector_rank`, 1-based, omitted when absent. In a merged list `score` is the merged
  score.
- `matched` stays the word index's count.
- The answer adds `, and {extra} more by meaning` before the full stop, where `extra` is the number of matches sent
  with no `word_rank`.
- `truncated` is true exactly when the two lists together held more entities than `limit`, `matched` is above the
  word hits read, a hit was missing, or matches were left out to fit. At limits under 100 that is nearly always:
  the embedding list has no score threshold (`graph/query_search_types.go:33-38`), so there is nearly always more.
- `indexed_revision` stays the word index's.

**D3. When one side fails** (PR A).

| Case | Reply | Proving test |
| --- | --- | --- |
| `embedding_enabled` false | As globalSearch; no embedding request | `TestSearchGraphTierZeroAsksNoEmbedding` |
| Embedding search refused or broken | Word list in word order, `word_rank` only; `merge` not honoured, `code`, `reason` | `TestSearchGraphNamesAFailedMerge` |
| Embedding search silent past `query_timeout_ms` | Same, `code` `internal` | `TestSearchGraphMergeTimeoutKeepsWordAnswer` |
| Word search refused `index_not_ready`, embedding answers | `text` null, `semantic`, `merge` honoured, detail `index_not_ready`, revision 0 (`textsearch.go:120-128`) | `TestSearchGraphIndexNotReadyLeavesTextOut` |
| Both fail | The `index_not_ready` refusal as it arrived | same |
| Any other word refusal | Returned at once; the embedding request is cancelled, its reply not awaited | `TestSearchGraphOtherRefusalIsReturned` |
| Provider closed | Found by words; `merge` not honoured, `code` set | `TestLosingTheProviderKeepsWordSearch` |

**D4. The embedding wait is not added to the word wait** (PR A). Both searches are sent before either reply is
awaited, so the embedding search adds at most what it waits beyond the word search, never a second
`query_timeout_ms` after it. The batch read and any hint reads still follow. Proof: `TestSearchGraphAsksBothAtOnce`:
each fake answers only after it has seen the other's request. The embedding search is not a hint: "The word search
comes before any hint" gains that line.

**D5. globalSearch and localSearch stay words only.** globalSearch is the scorecard's words row and tier 0's search.

**D6. The scorecard program** (PR B). The program is `internal/harness/scorecard`, run by `task scorecard:tier1 --
-url <base> -model <name> [-query-prefix <s>] [-api-key-env <VAR>] [-provider <note>]`. Like `pindiff` and
`mutcheck` (`Taskfile.yml:87-95`), it is a harness program Task runs, not a test. Its doc comment names its reason:
no committed way to measure tier 1 search on a real provider (#666). It:

- starts NATS with `natsfixture` from the one image pin. `New` takes `natsfixture.T` (`Helper`, `Errorf`, `Logf`,
  `Cleanup`, the four it uses); the program's `T` runs cleanups on exit and turns an `Errorf` into exit 2;
- composes graph-ingest, graph-index, graph-lexical (defaults), graph-embedding (`RegisterWithEmbedder` over
  `httpembedder.New`, `text_suffixes: [".content"]`) and graph-query (`embedding_enabled: true`);
- writes the 2,000 sections with `TestSearchScorecard`'s predicates. It waits for words by `min_revision`. It waits
  for vectors by watching `EMBEDDING_INDEX` until each section has a record from this embedder; a failed record ends
  the run naming its reason. Every wait is on an event;
- asks each question through `globalSearch`, the `semantic` verb and `searchGraph`, at limit 100, and scores them as
  `TestSearchScorecard` does;
- prints the Go version, the commit and whether the tree is clean (from Task), the provider URL and note, the
  configured model, the model the reply names (one plain request; "none" if unnamed), dimensions, the quoted query
  prefix, the embedded text (`.content`), section and question times, the three rows, and the verdict;
- exits 1 below the floor (D8), 2 on any other failure.

It scores searchGraph's own reply, not a copy. Its unit tests (flag refusals, tally arithmetic) join the tool
packages (`scripts/test-pkgs.sh:19`).

**D7. Sections are embedded by body only.** In this test set a section's `name` is its file's title, so every section
of a file starts with the same words, and their vectors look more alike than the sections are. With the title,
vectors find the answer in the first ten 47.6% of the time; without it, 52.0% (archived probe note). A real entity's
name is its own; #683 measures real entity text. Words keep name and body, as the word scorecard and the 62.4% row do.
Nothing is cut: the longest body is under 4,000 bytes and graph-embedding's cap is 4,000 characters
(`processor/graph-embedding/config.go:20`).

**D8. The floor** (PR B; a proposal until measured). One floor for tier 1, held by every model. It is kept as
constants in the program, each citing the probe note it was measured from. The first run on
`Snowflake/snowflake-arctic-embed-s` through semembed sets it. That run merges 100 deep, not the full lists behind
62.4%, so its row is measured, not assumed. Proposed: merged R@10 at least 60%, MRR at least 0.40, and merged R@10 not
below either single row (#668's "done when"). Output goes in `notes/probes/scorecard-<model>-<commit>.md` and the
release evidence.

**D9. Two PRs, A then B,** each with its own draft claim. PR A: `Addresses #668`. PR B: `Closes #668` (its "done
when" is the merged row) and `Closes #666`. Dependabot #597 (testcontainers) should land before PR B, which edits
natsfixture. #598 touches only `go.mod`/`go.sum`. Whichever lands second merges `origin/main`.

## Risks / Trade-offs

- semembed cuts text at 512 tokens (semembed#6), so the vectors row understates the model. Rerun when semembed
  changes. If semembed adds the query prefix itself (semembed#5), pass `-query-prefix ""`; a doubled prefix shows in
  the printed one.
- Every tier 1 searchGraph embeds the question, including questions the word index refuses (that request is
  cancelled, but the provider may already be working). A down provider costs at most `query_timeout_ms`, never the
  answer.
- Reading 100 deep on each side costs more than reading `limit`; the batch read stays at `limit`.

## Migration Plan

In alpha.5, searchGraph with `embedding_enabled` true answered as globalSearch (word order, BM25 `score`). Now its
order is the merge's and `score` the merged score (at most 2/61). A caller sorting by `score` keeps the right order. A
caller comparing `score` with a BM25 value must read `word_rank` instead. Read `merge` for whether vectors were used.
Rewrite `processor/graph-query/MIGRATION.md` (`:30-36`, `:181`, `:329-335`).

## Open Questions (owner only)

1. **Narrow `natsfixture.New` (D6).** It would take a four-method interface, so a Task-run program can start NATS
   from the one image pin. Recommended yes. Otherwise the program starts its own container with testcontainers: a
   second start path beside natsfixture's.
2. **The floor (D8).** Recommended: one tier 1 floor held by every model, of merged R@10 at least 60%, MRR at least
   0.40, and merged not below either single row, set from the first engine run.
3. **The reply (D2).** Recommended: rename `fallback` to `merge`; add `word_rank`/`vector_rank`; `score` is the merged
   score. Two answers come from vectors alone, and stay different:
   - Words matched nothing: `text.matches` is the vector list, read back.
   - The word index was not ready: `text` is null and `semantic` holds the raw reply, since no word index was read.
