# Proposal: setup-04b-01-lexical-index

Approved-by:

Track: `design`. Capability added: `lexical-search`.

## Why

Tier 0 search is what the offline and edge claims rest on, and it is weak. Measured on `main` at `2853486d`
(`notes/probes/`, summary on #10 comment 6833147079): the shipped `BM25Embedder` hashes words into 384 slots and ranks
by cosine. It puts the right answer in the first ten for 16% of known-answer queries where a plain BM25 index reaches
39%; it returns hits for made-up words in 200 of 200 tries; and the same corpus loaded in another order changes the
top hit for one query in five. The owner ruled on 2026-11-12 to fix this before the port continues (#10 comment
6833147079).

This is the first of four changes. The others, each its own change and pull request: (2) a processor that follows
`ENTITY_STATES`, rebuilds the index on every start and serves the search verb; (3) graph-query's local and global
search and the question router on the index; (4) removal of `BM25Embedder`, the substring scorers and
`EmbeddingClassifier`, and community keywords that are distinctive to their group.

## What Changes

- New package `graph/lexical`, with no NATS import: one tokenizer; an in-memory index from each word to the entities
  that hold it; BM25 scoring; a query of plain words, `"exact phrases"` and `-excluded` words (SemSource's R1 to R3 on
  #10). Words in an entity's name weigh more than words in its body.
- Corpus statistics (entity count, how many entities hold each word, average length) are read from the index when a
  query runs and never stored. A ranking then depends only on which entities the index holds, so ingest order and a
  rebuild cannot change it.
- A *scorecard*: a required test that asks a fixed set of questions with known answers of a fixed corpus and fails
  below a floor, beside generated checks of the laws in the spec (a word in no entity matches nothing; any order of
  writes that ends in the same entities ranks the same; a singular finds its plural).
- Nothing reads the package in production until change 2; the scorecard is its product test.

## Room to extend (owner condition, 2026-11-12)

The first version keeps no stored index and no per-entity token record. The design SHALL show that each of these can
be added later without changing the package's callers or the scorecard:

- a per-entity token record, so a restart need not refetch bodies kept in the object store: the index is fed entity
  by entity and never reads storage, so what feeds it can change;
- word positions in the index, if checking phrases against each entity's word sequence proves too slow;
- typo tolerance and further query forms: the query is a parsed value, so a form adds a field;
- tier 1 vectors beside the index, combined by rank: the index returns an ordered list, which is all that needs.

## Non-goals

No search library, stored index or typo tolerance. No change to graph-embedding, graph-query, fusion or any verb
(changes 2 to 4). No tier 1 work.

## Impact

`graph/lexical` (new) and its test data; a dependency guard in `internal/harness/contract`; one line in
`scripts/cover-check.sh`; `docs/repository-map.md`. Overlap: the comment clean-up (#606) will reach
files changes 3 and 4 delete (`bm25_embedder.go`, `classifier_embedding.go`, the scorers) and should skip them.

## Rulings

Covered (owner, 2026-11-12, #10 comment 6833147079): fix tier 0 search now, on this shape; start without a token
record provided the base leaves room to add one. This change breaks no consumer.
