# Scorecard data

The scorecard asks fixed questions of an index holding a fixed corpus. `TestScorecardHoldsItsFloor` and
`TestScorecardBehaviour` in `scorecard_test.go` read these files. They are frozen: a change that makes a question
fail is a change in search quality, never a reason to edit the data.

## `corpus.jsonl.gz`

2,000 sections of this repository's Markdown, one JSON object per line, gzipped: `id`, `path`, `name` and `body`.
A section is the text under one Markdown heading, up to the next heading; a body under 30 words, or one already
seen, is left out, and a body is cut at 4,000 bytes on a space. `name` is the first heading of the section's file,
and `body` is the section's text without its own heading. The sections were shuffled and the first 2,000 kept.

The entity ID is `c360.semengine.<top directory>.markdown.section.s<number>`, where the top directory is the
section's first path element (`root` for a file at the top) and the number is the section's place in the
shuffled corpus.

The text is as it was written, including references to issues and pull requests; it is test data, not
documentation.

## `known.json`

500 known-item questions. A *known-item question* has one right answer: here the question is a section's own
heading, and the answer is that section. The heading is not part of the section's text. A heading is used when it
appears once in the corpus, holds at least three distinct words after tokenizing, and is not its file's first
heading (that heading is every section's `name`).

The test asks each question and ranks the answer among every hit. `R@1` is the share of questions whose answer
comes first, `R@10` the share whose answer is in the first ten, and `MRR` the average of one over the answer's
rank (zero when it is not found). The test fails when `R@10` is below `scorecardFloor`.

## `questions.json`

Behaviour questions, in the shape SemSource's scorecard uses: an `id`, a `band`, the query in `args`, the
expectations, and `why` the question exists. A `twin` names the question it is paired with, such as the same
query without its exclusion, which shows the exclusion removes something. Every question must pass.

`args.query` goes through `ParseQuery`. `args.scope` is the search scope. `args.limit` is the most hits: 10 when it
is absent, every entity when it is 0.

| Field | Passes when |
| --- | --- |
| `expect_error` | `ParseQuery` refuses the query: `empty`, `reserved` or `too_long` |
| `expect_no_hits` | Nothing matches |
| `expect_all` | Each string is in some hit's text (name and body), ignoring case |
| `expect_any` | At least one string is in some hit's text |
| `expect_none` | No string is in any hit's text |
| `expect_top_all` | Each string is in the first hit's text |
| `expect_ids` | Each entity is a hit |
| `expect_no_ids` | No entity is a hit |
| `expect_hits_match` | The hits are exactly the sections whose name or body matches this Go regular expression |

`expect_hits_match` is the check for a phrase. The expression is written from the tokenizer's rules, not from its
code: a word is a run of letters and digits, so anything else separates words. Where the corpus holds a phrase's
words inside an identifier (an identifier splits where a lower case letter meets a capital) or as a plural, the
expression allows for that too.
