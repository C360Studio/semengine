# Proposal: setup-04a-04-graph-roots-clustering

Approved-by: owner, 2026-10-31 in Enjoy: design at `4d844e0` accepted, including its new exported surface; rulings
recorded on PR #483 (comments 6543795826, 6543960797)

## Why

Change 4 of epic #9 ports the graph roots one package per pull request in the owner's order (#9 comment 6386856657);
item 8 is "`graph/clustering` without the LLM summarizer", and item 9, `processor/graph-query`, imports it. The owner's
bar: "graph is in many ways our secret sauce. it needs to be solid… i would rather not bring in a pkg that causes
instant debt". Tier 0 only (#9 comment 6386603659). The pin's algorithm, label propagation, was removed by the owner's
own SemStreams ruling ADR-099, which SemStreams never carried out; SemEngine's ported ADR-102 already defines community
levels as entity-ID prefixes.

## What Changes

- `graph/clustering` becomes a library that keeps the community index current from entity changes: a community is
  every entity sharing an ID prefix (level 0 `org.platform.system.domain`, level 1 the source `org.platform.system`,
  level 2 the deployment `org.platform`). Each entity's last revision and contribution are kept, so a replace subtracts
  the old and adds the new, a delete subtracts, and an out-of-date delivery changes nothing.
- A tick writes only the communities changed since the last one, and only when the record changed: member count, type
  counts, keywords from the community's own term counts, a summary, and a few representative IDs; never the member
  list. Emptied communities are removed. After bootstrap a tick reads nothing.
- Failures are returned, never only logged: a tick before bootstrap is done, a refused entity, a failed write or
  removal. A partial update is never reported as complete.
- Storage fixes from the pin: unreadable entries fail loudly, a cancelled context is reported, the test-only store and
  the error-text match leave production code.
- Not carried, each with a ledger row: label propagation and its edge machinery (ADR-099); the LLM lane, the
  membership hash and the statistical summarizer with its interface (#485); PageRank and `provider.go`. The ledger
  rows for the HTTP embedder and the classifier LLM adapter move to their tracker issues (#489, #486).
- ADR-099 ported with a port note; ADR-085's note gains the package; the setup plan's tier-0 line reads "community
  grouping by entity-ID prefix"; ledger, README, repository map, 80 % coverage target, one skill line.

## Capabilities

- New `community-grouping`.

## Impact

- New `graph/clustering`; edges to `graph`, `natsclient`, `pkg/errs`, `pkg/types`. New framework API, for the owner's
  acceptance: `Maintainer` (`NewMaintainer`, `Upsert`, `Delete`, `BootstrapDone`, `Tick`), `IncompleteError`,
  `ErrBootstrapIncomplete`, the `Community` record, `MaxRepEntities`, `MaxKeywords`, `MaxEntityTerms`.
- `COMMUNITY_INDEX` records change shape (fresh state, no migration). semboids, a load generator, reads `members` and
  sees empty communities until it changes.
- Change 7's `processor/graph-clustering` wires the `ENTITY_STATES` watch to the maintainer. Item 9
  (`processor/graph-query`) owns the reader's consistency contract and its own per-page key rescans.
- Retires nothing. Stays open: #485, #32, #68, #141, #477. Addresses #9.

## Rulings (owner, 2026-10-31)

- Community model: the ADR-099 entity-ID prefix partition, maintained incrementally from changes; no label
  propagation, ever ("Yes, as summarized"; PR #483 comment 6543795826).
- Scope: "Library maintainer only; change 7 wires the watch (recommended)".
- The LLM lane and the membership hash leave whole for tier 2 (#485, epic #484).
- `CommunitySummarizer` and `StatisticalSummarizer` leave tier 0: "Drop it; tier 2 (#485) brings back what it needs."
  Supersedes the 2026-10-24 ruling that the interface stays (PR #483 comment 6543960797; #485 comment 6543961011).
- semboids is a load generator, not a customer; its member decoder breaking under bounded records is accepted.
- A detection overlay over explicit relationship predicates is a later change, with its own gates.
