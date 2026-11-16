# Proposal: setup-04b-03-graph-query-lexical

Approved-by: cglusky (2026-11-14) <https://github.com/C360Studio/semengine/issues/10#issuecomment-6877374827> sha256:c08b5aee704d

Track: `design`. Modified capabilities: `graph-query`, `lexical-search`, `graph-lexical`, `composed-stack`.
`query-classification` is unchanged: the classifier keeps its rules, and only how graph-query uses its answer changes.

## Why

graph-query's local and global search do not use the word index (inventory §4). On the frozen scorecard corpus they
find the answer in the first ten for 32.6% (local) and 7.0% (global) of questions; the word index, 59.8% and 56.0%; a
plain BM25, 60.0% and 56.0% (`notes/probes/search-quality-probe.log`). Community keywords gate global search, and the
router refuses or misroutes plain questions (#616). This is change 3 of the plan on epic #10 (comment 6833147079).

## What Changes

- `localSearch` is a word search scoped to the group of `entity_id` at `level`; `globalSearch` is one word search over
  every entity, its hits grouped by community. Group records describe the groups and never gate the answer.
- Matches are the hits read back as stored entities, in the word index's order, with score and label; both verbs take
  `limit` (default 10, at most 100) and declare a cut answer, missing entities and excluded words.
- Every question gets a word search. The classifier's hints are reported, each honoured or not with a reason; a path
  start or entity reference that resolves adds its walk or entity. No hint's trigger word refuses; the word index's
  query rules still do (`pump*`, a question of stopwords only).
- What search accepts (#624): up to 32 distinct words; `*bold*` is text; excluded words are listed in the reply;
  `HEAD~` is owner question 2 (recommended: a final `~` separates words).
- searchGraph answers as globalSearch, with no embedding request at tier 0; the tier 1 fallback is owner question 1.
- A required known-answer test runs the 500 questions through both verbs against a plain BM25 computed in the test.
- Smoke checks local and global search on the word index (#628, second item). README, MIGRATION, the query-pattern
  skill and the ledger row say what the verbs do now.

## Non-goals

Change 4 (`BM25Embedder`, the embedding classifier, community keywords, graph-clustering's half of #650); time-range
and area answers (change 7); tokenizer changes (#651); a stored index.

## Impact

Consumers migrate. SemSource's `graph_search` drops `summarize_threshold`, reads `data.text.matches` and `data.hints`,
and composes graph-lexical. A deployment that serves search composes graph-lexical; without it each search is refused
transient `internal` naming the verb. graph-query stops reading prefix pages for search; `summary` and
`hierarchyStats` are unchanged.

## Retires, and in which pull request

PR A, the query rules: `Closes #624`. PR B, graph-query on the index, with the archive: `Closes #616`, `Closes #506`;
`Addresses #650` (graph-query's half; graph-clustering's stays for change 4), `Addresses #628` (second item; change 4
has the rest), and narrows #535 to "from A to B" by comment. #563 stays open unless the owner picks building the tier 1
fallback now. #656 (a batch read refuses every entity when one is damaged) stays open: PR B neither fixes nor works
around it, so one damaged hit refuses a search until it is fixed in graph-ingest.

## Rulings

Covered: plan item 3 (#10 comment 6833147079); #563 at tier 0 (6876750057); #624 settled here (6876750265);
consumers migrate (owner, 2026-11-02). Ruled 2026-11-14: design question 1 (tier 1 fallback built with #489)
and 2 (`HEAD~` searches `head`).

Changed ADR decision, for approval with this proposal: ADR-087 Decision 5 ("Readiness is gated on `COMMUNITY_INDEX`
only", `docs/adr/087-community-summary-store-ownership.md:96`) stops applying to search, which is ready when
graph-lexical answers; the community view only describes groups. Its purpose, never gating on summaries, still holds,
and ADR-088 already makes readiness per producer. PR B updates the ADR's port note.
