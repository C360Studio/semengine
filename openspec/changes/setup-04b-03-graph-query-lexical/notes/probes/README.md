# Probes: graph-query search on the word index

Two probes measured the design's questions on a copy of the tree at `ef4313c3` (2026-11-14, go1.26.11, darwin/arm64).
Each `.go.txt` file is Go test source kept as text so that it is not built; each `.log` file is its output. Copy the
tree outside the repository (`rsync -a --exclude .git`), put the source in the named package, and run the command at
the top of its log.

A *known-item question* has one right answer. The corpus is the word index's frozen scorecard corpus
(`graph/lexical/testdata/scorecard/`: 2,000 sections of this repository's Markdown, 500 questions, each a section's
heading; the answer is the section under it). `R@1` is how often the answer came first, `R@10` how often it was in the
first ten, `MRR` the average of one over its rank. A *plain reference* is a textbook BM25 (Lucene's idf, k1 1.2,
b 0.75) over one field, written in the probe, sharing nothing with `graph/lexical` but its tokenizer.

| Probe | Package | Measures |
| --- | --- | --- |
| `search-quality-probe` | `graph/clustering` (it calls the unexported community builders to rebuild today's records) | Local and global search four ways: the plain reference; the word index (global unscoped, local scoped to the answer's level-0 group); today's substring rule; and today's paths (local: one page of 1,000 IDs; global: five groups chosen by keywords, one page each). Two phrasings: the heading, and "what is the HEADING?", HEADING being the question. Also how often more than 100 entities match, whether the answer's group is among the first ten hits' groups, and the size of hits read back as stored entities. |
| `query-length-probe` | `graph/lexical` | How many distinct words SemSource's 46 scorecard questions and ten pasted error lines hold after tokenizing, and which `ParseQuery` refuses, at bounds 16 and 32; search time against query length at 2,000 and 50,000 entities (the corpus put 25 times under new IDs), with `graph.MaxSearchTerms` raised to 64 in the copy. |

`query-length-probe.go.txt` is three test files in one: split it at the `// ---- second file` and `// ---- third file`
lines into `zz_gq_parse_test.go`, `zz_gq_cost_test.go` and `zz_gq_scale_test.go`. The cost and scale tests need
`graph.MaxSearchTerms` raised to 64 in the copy; the parse test runs at 16 and at 32.

The parse probe reads SemSource's questions from `/tmp/gq/questions.json` and `/tmp/gq/questions-osh.json`: the
`args.query` of each entry of `scripts/scorecard/questions.json` and `questions-osh.json` in SemSource at `4ca2072c`.
Build them from a SemSource checkout with:

```bash
mkdir -p /tmp/gq && for f in questions questions-osh; do
  jq '[.questions[].args.query]' "scripts/scorecard/$f.json" > "/tmp/gq/$f.json"
done
```

The ten pasted lines are hand-written examples of what a person or agent pastes (a panic, a test failure, a log line,
a stack frame, a command line), not a sample of real traffic.

Limits. The corpus's level-0 groups are its top directories: 21 groups, one of them (`openspec`) holding 1,758 of the
2,000 sections, so local search here is close to global search and shows scoping is right more than what scoping buys.
SemSource's corpus has not been measured (#620). Times are one idle laptop.
