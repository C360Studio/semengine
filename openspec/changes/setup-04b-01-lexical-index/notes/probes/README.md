# Probes: how tier 0 search behaves today

The first four probes measured the search code on `main` at commit `2853486d` (2026-11-12), before this change. They
are the evidence the proposal cites. The fifth, `memory-probe.go.txt`, is the inventory's: a program of its own, not
a test, that builds a plain in-memory word index from generated text and prints its heap and build time. Run it
outside the tree as `go run main.go <entities> <tokens per entity> <pos|nopos>`; its numbers are in `inventory.md`
section 8. Every `.go.txt` file is Go source kept as text so that it is not built, and each `.log` file is the
output of the probe it is named for.

A *known-item query* is a question with one known right answer. Here the corpus is this repository's Markdown, cut
into sections; the query is a section's heading and the right answer is the section under it. The heading is not
part of the section's text. `R@1` is how often the right answer came first, `R@10` how often it was in the first ten,
and `MRR` is the average of one over its rank.

| Probe | Copy to | Run | Measures |
| --- | --- | --- | --- |
| `retrieval-probe.go.txt` | `graph/embedding/zz_review_experiment_test.go` | `go test ./graph/embedding -run TestReviewExperiment -v -count=1` (add `SHORT=1` for 20-word texts) | The shipped `BM25Embedder` and graph-query's substring rule against a plain BM25 index; made-up words; ingest order; the stemmer |
| `tokenizer-probe.go.txt` | `graph/embedding/zz_review_tokenizer_test.go`, beside the first | `go test ./graph/embedding -run TestReviewTokenizers -v -count=1` | Stemmer and stopword choices under a plain BM25 index |
| `community-probe.go.txt` | `graph/clustering/zz_review_community_test.go` | `go test ./graph/clustering -run TestReviewCommunities -v -count=1` | Community keywords, and global text search through community records |
| `router-probe.go.txt` | `processor/graph-query/zz_review_route_test.go` | `go test ./processor/graph-query -run TestReviewRoutes -v -count=1` | Where global search routes plain questions |

The design added three more, each with its source and its log:

- `sequence-probe`: the memory program with one more layout, where each entity keeps its word sequence (design D4).
- `tokenizer-probe-2`: the tokenizer probe with three more rows, among them the 30-word stopword list and an earlier
  plural rule (strip `es` after `ss`, `x`, `ch`, `sh` or `z`), not the shipped pair of rules (design D3). Design D3
  and `and-query-probe.log` were written as if it measured the shipped pair; it did not, and the scorecard measures
  the shipped rules. It runs as `tokenizer-probe` does.
- `word-shape-probe`: a Python script over the tree's Go and Markdown files that counts long words before and after
  identifier splitting and plural pairs that fold (design D3). Run it from the repository root with `python3`.
- `tokenizer-law-probe`: runs the tokenizer's rules in the order the spec states and fuzzes the law that tokenizing
  twice changes nothing (design D3).
- `and-query-probe`: how many known-item answers hold every word of their question (design D4b).
- `corpus-size-probe`: the size of 2,000 sections, raw and gzipped, and how many carry issue numbers (design D8).

Limits of these numbers:

- The corpus is English prose from one repository. No consumer's data or questions were used.
- The community probe groups sections by directory, which is the probe's choice and not a deployment's entity IDs;
  one group holds 6,601 of the 8,000 sections. It copies graph-query's group and entity scoring rules into the
  probe, since they are unexported in another package.
- The corpus is the tree's own Markdown, so the numbers reproduce only at commit `2853486d`.
