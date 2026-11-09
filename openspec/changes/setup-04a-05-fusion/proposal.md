# Proposal: setup-04a-05-fusion

Approved-by:

Track: `design` (adds a new capability, `fusion`; adds spec requirements).

## Why

Change 5 of epic #9 makes SemSource's composition build against SemEngine alone (04A D2). This is PR C of four
(`split.md` of the archived `2026-11-08-setup-04a-05-first-consumer-graph-embedding`, accepted on PR #550 comment
6716972354): the lens path of `pkg/fusion`, the deterministic engine behind SemSource's `code_context`,
`code_impact`, `code_search` and `doc_context`, with `pkg/fusion/fusionvocab`, its ranking signals. The retrieval
client over NATS (`fusionnats`) is PR D. The owner ruled (same comment, item 4) that fusion drops its pre-request
status check and reads "not ready" from the reply (ADR-109), which removes fusion's copy of the readiness record
(#110 item 2) and changes pin ADR-066 and ADR-082.

## What Changes

- `Engine.Fuse` asks no status. A reply's `index_not_ready` defers the answer; a fatal seed read or an ended context
  fails the call; a failed walk read, fatal or not, is skipped and declared (ADR-109 D1: poison refuses only the
  poisoned entities). `RetrievalClient.Status`,
  `IndexStatus`, `IndexState`, `ErrReadinessUnknown` and `Response.Index` go; `contract_version` becomes `"2"` (D1).
- Decided, not asked: the graph facet's `view_revision` goes with the status read that filled it (D2; ruling 4 and
  the 2026-11-02 ruling that a consumer break is a migration note).
- A word-search answer reports how many records the search could not read or had failed (D3, question 1).
- Every left-out piece is declared: edges are sorted before any cap; relations, paths, impact and the graph facet are
  labelled when cut or when a read was skipped, and each skipped read is logged and counted; a path never shows an
  entity ID (D4, SemStreams #621). Only the two new label names are the owner's (question 3).
- `NewEngine` takes a logger and options and returns an error; counters register only on the injected registry
  (D5). `fusionvocab.New` takes the ontology's parent function, since `vocabulary/cco` lives in semvocab (D6,
  question 2).
- Not carried: the package-level `Fuse`/`SubQuery`/`GraphQueryClient`/`Evidence` path (03B D5), `lensregistry`.

## Capabilities

New `fusion`.

## Impact

- Pin lines: 2,245 production and 3,306 test for the lens path, 51 and 52 for `fusionvocab` (inventory § 1);
  this PR about 2,000 + 2,800 (hand estimate). No subject, bucket, goroutine or NATS import.
- Consumers migrate (`pkg/fusion/MIGRATION.md`, owner ruling of 2026-11-02 on PR #504): SemSource's `NewEngine`,
  `fusionvocab.New` and its `RetrievalClient` fake fail to compile until changed. On the wire, `index` and
  `view_revision` leave, the gate's four `defer_reason` values go, and a reset that deferred becomes an error.
- ADR-062 ported; ADR-066 (tree :343-344, :394), pin ADR-082 :65, ADR-085 D1 change under the owner's ruling.
- Merge order: no open PR shares a file (#560 touches `internal/smoke` only). PR D follows this one.
- Retires none. Advances #110 (item 2 done; stays for item 3, temporal's port).

## Owner questions (plain words, answer first)

1. **Should a word-search answer say how many records it could not search?** Recommended: yes, two counts copied
   from the search reply (unreadable, failed), on every answer after a word search, a "found nothing" answer most of
   all. Without them a document whose embedding failed silently never shows up in fusion answers (D3).
2. **How does fusion's ranking learn how deep a class sits in the ontology, now that the ontology lives in
   semvocab?** Recommended: SemSource passes semvocab's parent function to `fusionvocab.New`. SemEngine gains no
   ontology code; semvocab already depends on SemEngine, so SemEngine cannot import it back; and SemSource already
   imports the ontology package itself, so it needs semvocab anyway (D6).
3. **Names for the two new "cut short" labels:** `relations_truncated` on a node and `paths_truncated` on the answer.
   Recommended: these names, matching the `truncated` the answer already uses. The labels themselves follow the
   rule that nothing is dropped silently (D4).
