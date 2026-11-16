# Probe: tier 1 scorecard, arctic-embed-s via semembed, SemEngine 6f69a4e2

The first run of `task scorecard:tier1`. It asks the word scorecard's 500 known-item questions (a section's own
heading; the one right answer is the section under it) of its 2,000 sections through the composed engine:
globalSearch (words), graph-query's `semantic` verb (vectors) and searchGraph (the two lists merged by reciprocal
rank fusion, k = 60, each list 100 deep). Each verb is asked 100 deep; a rank past 100 counts as not found. Only
section bodies are embedded (`text_suffixes: [".content"]`); words index title and body. It sets tier 1's floor
(`internal/harness/scorecard/floor.go`).

- **SemEngine:** commit `6f69a4e21211064aaa65b99fabe8d7945aca2647`, tree clean (printed by the run).
- **Provider:** semembed at commit `c4cfb3090264984cecc9a309efbc1b7d60022442` (the image's
  `org.opencontainers.image.revision` label), image `ghcr.io/c360studio/semembed:latest`, digest
  `ghcr.io/c360studio/semembed@sha256:d2b43eb58dd46e4c18c6d8293908bc75e8ec2ff7e71c5847e05e78b603f2707a`, container
  `semengine-semembed` at `http://127.0.0.1:18081`. `/health` answered
  `{"status":"healthy","model":"Snowflake/snowflake-arctic-embed-s","max_tokens":512}`: texts past 512 tokens are cut.
- **Query prefix:** `"Represent this sentence for searching relevant passages: "`, the model's query instruction,
  put before each question by the HTTP embedder's `QueryPrefix`; sections are embedded with none.
- **Run on:** 2026-11-16, this Mac (12 cores, Docker Desktop), while a `task verify` ran in another worktree; the
  timings are one run's wall clock under that load.

## Results

| Row | R@1 | R@10 | MRR |
| --- | --- | --- | --- |
| Words, globalSearch | 24.6% | 56.0% | 0.357 |
| Vectors, `semantic` | 28.2% | 52.0% | 0.361 |
| Merged, searchGraph | 31.2% | 61.6% (308 of 500) | 0.410 |

The words row is `TestSearchScorecard`'s globalSearch row. The vectors row is the body-only row an earlier
throwaway probe measured outside the engine (52.0%, 0.362; setup-04c-01's `notes/probes/scorecard-semembed-7ceb5281.md`).
Merging 100 deep, as searchGraph does, gives 61.6% and 0.410 against 62.4% and 0.412 for that probe's full lists.

## The floor set from it

Tier 1's floor is the owner-approved proposal, which this run meets: merged R@10 at least 60% (300 of 500), merged MRR
at least 0.40, and merged R@10 not below either single row. The merged row holds it by 8 answers on R@10 and 0.010
on MRR. 500 questions resolve about 2 points, so a model or a change that costs two points of R@10 here fails it.

## Command

From the worktree root, semembed running:

```bash
task scorecard:tier1 -- -url http://127.0.0.1:18081/v1 -model Snowflake/snowflake-arctic-embed-s \
  -query-prefix 'Represent this sentence for searching relevant passages: ' \
  -provider 'semembed c4cfb309, ghcr.io/c360studio/semembed:latest'
```

Exit status 0. The shell's `time`: `14.06s user 4.72s system 16% cpu 1:50.67 total`.

## Output (standard output, verbatim)

```text
go: go1.26.11 darwin/arm64
commit: 6f69a4e21211064aaa65b99fabe8d7945aca2647 (tree clean)
provider: http://127.0.0.1:18081/v1 (semembed c4cfb309, ghcr.io/c360studio/semembed:latest)
model asked for: "Snowflake/snowflake-arctic-embed-s"
model the reply names: "Snowflake/snowflake-arctic-embed-s"
query prefix: "Represent this sentence for searching relevant passages: "
embedded text: values of predicates ending ".content" (section bodies)
corpus: 2000 sections, 500 questions, each asked 100 deep
sections: written and in the word index in 11.658s, in the vector index in 1m28.647s; 384 dimensions
sections the semantic verb answered only when asked by their own body: 0
questions: 500 in 18.831s, each through all three verbs
words, globalSearch        R@1  24.6%  R@10  56.0%  MRR 0.357  over 500 questions
vectors, semantic          R@1  28.2%  R@10  52.0%  MRR 0.361  over 500 questions
merged, searchGraph        R@1  31.2%  R@10  61.6%  MRR 0.410  over 500 questions
floor: merged R@10 at least 60%, MRR at least 0.400, R@10 not below either single row
verdict: holds tier 1's floor
```

Standard error held Task's command line, testcontainers' Docker lines, the components' INFO lines and two
warnings, neither a failure: graph-query's `community view unavailable` (no graph-clustering in this composition, so
replies carry no group records; scoring reads matches only) and, at Stop, graph-embedding's
`dispatch: Shutdown's context ended before accepted work drained`, which its Worker's Stop path logs by design
(`graph/embedding/worker.go`, "Shutdown only begins the drain").
