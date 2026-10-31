# Proposal: setup-04a-04-graph-roots-clustering

Approved-by:

## Why

Change 4 of epic #9 ports the graph roots one package per pull request in the owner's order (#9 comment 6386856657);
item 8 is "`graph/clustering` without the LLM summarizer", and item 9, `processor/graph-query`, imports it. The
package finds communities of related entities and writes them to `COMMUNITY_INDEX`; semboids composes it today to
show flocks. SETUP 03B admits "clustering on explicit edges" at tier 0. The owner's bar: "graph is in many ways our
secret sauce. it needs to be solid… i would rather not bring in a pkg that causes instant debt". Tier 0 only (#9
comment 6386603659).

## What Changes

- Port the half of `graph/clustering` that both 03B and the pin's ADR-099 keep: label propagation over the graph's
  explicit edges, the statistical community summary, and write-then-prune storage (2,503 of 5,516 pin lines).
- One community level. The pin's levels 1 and 2 re-run the same algorithm over the same entities and can come out
  finer than level 1; nothing links a community to a parent.
- Fixes in carried code: a summary is the same for the same members (the pin's keyword order varies between runs);
  storage stops skipping unreadable entries and stops reporting success on a cancelled context; a test-only in-memory
  store and unread triple output leave production code; empty-bucket handling stops matching error text.
- Not carried, each with a ledger row: the LLM lane (`LLMSummarizer`, the enhancement worker, the summary store and
  its metrics, `MembershipHash`), #32; entity-ID and similarity "virtual edges", which the pin's ADR-099 removes;
  `provider.go` and other unread surface (inventory 4).
- ADR-099 ported with a port note saying what SemEngine applies; ADR-085's note gains the package; ledger, README,
  repository map, 80 % coverage target, one skill line.

## Capabilities

- New `community-detection`.

## Impact

- New `graph/clustering`; edges to `graph`, `natsclient`, `pkg/errs`, `pkg/types`. No new subject, bucket, payload,
  metric or config key; `COMMUNITY_INDEX` keys and record bytes keep their shape (four unread fields dropped).
- `processor/graph-clustering` (change 7) loses its entity-ID and semantic edge config and its LLM worker wiring;
  `processor/graph-query` (item 9) decides its summary view and level-1/2 requests.
- Retires nothing. Stays open: #32, #68, #141, #477. Addresses #9.

## Questions for the owner

1. **Which clustering is tier 0?** Your SemStreams ruling ADR-099 (2026-08-23) removed the entity-ID edge machinery
   and the semantic edges, kept clustering only over explicit edges, and made the base partition the entity-ID prefix.
   SemStreams never built it. This draft carries clustering over explicit edges (what semboids runs) and applies the
   removals, but does not build the prefix partition. Accept, or do you want the prefix partition instead?
2. **The LLM lane leaves whole.** Your ruling kept the enhancement worker holding an interface. With no tier-2
   summarizer the worker has nothing to run, so this draft leaves out the worker, the summary store, its metrics and
   the membership hash, under #32. OK?
3. **One level.** Levels 1 and 2 are not a hierarchy today. Keep only level 0 (graph-query then refuses or ignores
   higher levels, decided at item 9)?
