# Proposal: setup-04a-04-graph-roots-clustering

Approved-by:

## Why

Change 4 of epic #9 ports the graph roots one package per pull request in the owner's order (#9 comment 6386856657);
item 8 is "`graph/clustering` without the LLM summarizer", and item 9, `processor/graph-query`, imports it. The owner's
bar: "graph is in many ways our secret sauce. it needs to be solid… i would rather not bring in a pkg that causes
instant debt". Tier 0 only (#9 comment 6386603659). The inventory found that the pin's algorithm, label propagation,
was removed by the owner's own SemStreams ruling ADR-099 (2026-08-23), which SemStreams never carried out, and that
SemEngine's ported ADR-102 already defines community levels as entity-ID prefixes.

## What Changes

- `graph/clustering` arrives as ADR-099 decided: a community is every entity sharing an ID prefix (level 0 four
  positions, level 1 the source, level 2 the deployment); each group record holds counts, a membership hash, keywords,
  a statistical summary and a few representative IDs, never the member list; unchanged records are not rewritten.
- Carried from the pin: the storage (write everything, then remove leftovers), the statistical summarizer, the
  membership hash. Fixed on the way: summaries no longer vary run to run; storage stops skipping unreadable entries and
  stops reporting success on a cancelled context; a test-only store leaves production code; a failed cleanup is
  returned, not only logged.
- Not carried, each with a ledger row: label propagation and its edge machinery (ADR-099); the LLM lane (summarizer,
  enhancement worker, summary store, metrics), #32; PageRank and `provider.go`.
- ADR-099 ported with a port note; ADR-085's note gains the package; ledger, README, repository map, 80 % coverage
  target, one skill line.

## Capabilities

- New `community-detection`.

## Impact

- New `graph/clustering`; edges to `graph`, `natsclient`, `pkg/errs`, `pkg/types`. No new subject, bucket or payload.
  `COMMUNITY_INDEX` records change shape (fresh state, no migration): item 9 reads members by prefix query.
- semboids' flock view becomes one group per flock prefix until a detection overlay is built and measured.
- `processor/graph-clustering` (change 7) calls the new partition step and refuses its old LPA config keys.
- Retires nothing. Stays open: #32, #68, #141, #477. Addresses #9.

## Questions for the owner

1. **Which community model is tier 0?** Your SemStreams ruling ADR-099 (2026-08-23) removed label propagation
   entirely, made communities the entity-ID prefixes, and allowed detection back only as a default-off overlay that
   graph-query never reads, after a test that can fail shows it works (the old check never beat 1 of 3). The tier-0
   admission "clustering on explicit edges" (2026-10-01) came later and did not read that ruling, and SemEngine's own
   ADR-102 already says level 0 is the four-part prefix. Options: (A) port the pin as is; (B) keep label propagation
   over explicit edges as the communities graph-query serves; (B′) label propagation only as a separate, default-off
   overlay; (C) the prefix communities ADR-099 ruled. Recommended: C, and the setup plan's tier-0 line changes from
   "clustering on explicit edges" to "community grouping by entity-ID prefix".
2. **The LLM lane leaves whole.** Your 2026-10-24 ruling kept the enhancement worker holding an interface. With no
   tier-2 summarizer the worker has nothing to run, so this draft leaves out the worker, the summary store and its
   metrics under #32 (the membership hash stays: the new record carries it). OK?
3. **semboids' flocks.** Under C, semboids sees one community per flock prefix and loses its flock clusters until a
   detection overlay is built with a test that can fail. Accept that gap, with the overlay as its own later change?
