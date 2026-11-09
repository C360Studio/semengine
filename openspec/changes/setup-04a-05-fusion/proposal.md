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

- `Engine.Fuse` asks no status. A reply's classified `index_not_ready` defers the answer; other errors return.
  `RetrievalClient.Status`, `IndexStatus`, `IndexState`, `ErrReadinessUnknown` and `Response.Index` go; the wire's
  `contract_version` becomes `"2"` (design D1).
- The graph facet's `view_revision`, filled from the status read, goes with it (D2, owner question 1).
- A natural-language answer reports how many records the semantic search could not read or had failed (D3, question 2).
- Every left-out piece of an answer is labelled: relations past the cap or not read, paths past the cap, an unread
  file in impact, an unread counterpart in the graph facet; a path never shows an entity ID (D4, question 4;
  SemStreams #621).
- `NewEngine` takes options and returns an error; the body-failure counter registers only on the injected registry
  (D5). `fusionvocab.New` takes the ontology's parent function, since `vocabulary/cco` lives in semvocab (D6,
  question 3).
- Not carried: the package-level `Fuse`/`SubQuery`/`GraphQueryClient`/`Evidence` path (03B D5), `lensregistry`.

## Capabilities

New `fusion`.

## Impact

- Pin lines: 2,245 production and 3,306 test for the lens path, 51 and 52 for `fusionvocab` (inventory § 1);
  this PR about 2,000 + 2,800 (hand estimate). No subject, bucket, goroutine or NATS import.
- Consumers migrate (`pkg/fusion/MIGRATION.md`, owner ruling of 2026-11-02 on PR #504): SemSource's `NewEngine`,
  `fusionvocab.New` and its `RetrievalClient` fake fail to compile until changed; the `index` block leaves the wire.
- ADR-062 is ported with a port note; ADR-066 :338-344 and ADR-082 :65 change under the owner's ruling.
- Merge order: no open PR shares a file (#560 touches `internal/smoke` only). PR D follows this one.
- Retires none. Advances #110 (item 2 done; stays for item 3, temporal's port).

## Owner questions (plain words, answer first)

1. **Drop the graph facet's "revision span" along with the status check?** Recommended: yes. It was filled from
   the status read you removed; nothing in SemSource reads it, and the facts it described come from replies that
   carry no revision, so a replacement would overstate what it knows (D2).
2. **Should a word-search answer say how many records it could not search?** Recommended: yes, two counts copied
   from the search reply (unreadable, failed). Without them a document whose embedding failed silently never shows
   up in fusion answers (D3).
3. **How does fusion's ranking learn how deep a class sits in the ontology, now that the ontology lives in
   semvocab?** Recommended: SemSource passes semvocab's parent function to `fusionvocab.New`; SemEngine gains no
   ontology code, and semvocab already depends on SemEngine so it cannot be imported back (D6).
4. **Two new labels for cut-short answers:** `relations_truncated` on a node and `paths_truncated` on the answer.
   Recommended: add both; today the caps cut silently (SemStreams #621) (D4).
