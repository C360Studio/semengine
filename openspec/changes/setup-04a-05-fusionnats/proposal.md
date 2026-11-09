# Proposal: setup-04a-05-fusionnats

Approved-by:

Track: `design` (adds requirements to two capabilities, `fusion` and `composed-stack`).

## Why

Change 5 of epic #9 makes SemSource's composition build against SemEngine alone. This is PR D, the last of four
(split row D, accepted on PR #550 comment 6716972354): `pkg/fusion/fusionnats`, the NATS `RetrievalClient` the fusion
engine of PR C (#565) runs over, and the plan's proof that the tier-0 stack works with no embedder
(`docs/setup-plan.md:439-444`). v0.1.0-alpha.3 is tagged after it merges (#9 comment 6727586862).

## What Changes

- `fusionnats.New(requester, timeout)` returns an error, refusing a nil requester or a negative timeout. The client
  starts nothing: no readiness watch, no `Status`, no `Close` (ruling 4; ADR-109). Readiness arrives on each reply as
  `index_not_ready`, which the engine already reads (D1, D4).
- It speaks this tree's wire: subjects and reply shapes from `graph.QueryVerbs()`, the request types `graph` declares,
  `predicate` on relationship rows, and the word search's two "could not read" counts passed to fusion (D2).
- A damaged (poisoned) entity among the seeds no longer fails the whole answer: when graph-ingest refuses the group
  read, each seed is read alone and the damaged ones are listed as not loaded, reason `error` (D3, owner question 1).
- `graph/readiness.Reading.Raw` goes: its one reader at the pin was fusionnats' status read (D4).
- No-embedder proof: a smoke test boots graph-ingest, graph-index and graph-query with no graph-embedding, and fusion
  answers name and prefix questions; a word question is refused, never answered empty (how: owner question 2). A
  contract test keeps `fusionnats` free of embedding imports (D5).

## Capabilities

Modified: `fusion` (the NATS client), `composed-stack` (the no-embedder stack).

## Impact

- Pin lines 658 production, 1,403 test; this PR about 450 + 1,500 (hand estimate). No subject, bucket or goroutine.
- Consumers migrate (`pkg/fusion/MIGRATION.md`): SemSource's `fusionnats.New(client, 0)` (code-context and 11 tests)
  takes the error; one test's `Status` call goes. On the wire nothing changes for SemSource except that a poisoned
  seed is listed instead of failing.
- Stale text rewritten: port notes on ADR-062, ADR-066 and ADR-083 (the watch is not carried), ADR-071, ADR-084; the
  `pkg/fusion` and ADR-062 ledger rows; the `fusion` spec's fatal-seed-read example (MODIFIED). Pin ADR-079 D3 is kept
  under the recommendation.
- Merge order: no open PR overlaps; under question 2's recommendation a small graph-query PR merges before this
  one's proof task. Retires none; #195, #110, #563 stay. Files the graph-ingest prefix-read issue, and under question
  2's alternative a graph-query issue.

## Owner questions

1. **A damaged entity among the results.** Today graph-ingest refuses a group read if any one entity in it is
   damaged, so one bad record would fail every fusion answer it shows up in. Recommended: fusionnats then reads each
   result alone and lists the damaged ones as "could not load". It costs extra requests only while damage exists and
   changes no other package. The alternative is to change graph-ingest so its group read lists damaged entities
   itself: one request, but it reverses a SemStreams decision (ADR-079) and touches a second package.
2. **A word question with no embedder.** The plan asks for "explicitly admitted graph-only interfaces, not
   unsupported NL verbs" (`docs/setup-plan.md:439-440`). Today graph-query still offers word search with no embedder
   and answers it with "temporary internal error", which tells a program to retry something that can never succeed.
   Recommended: a small graph-query change first, so a question nobody in the composition can answer gets a "will not
   succeed, do not retry" refusal with its own error code (you name it), and the proof checks that. The alternative:
   keep today's refusal for alpha.3 and file it as an issue.
