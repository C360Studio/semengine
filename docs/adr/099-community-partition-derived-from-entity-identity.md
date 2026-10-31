# ADR-099: Community Partition Is Derived from Entity Identity; Detection Is Removed

> **Ported from SemStreams.** This record is `docs/adr/099-community-partition-derived-from-entity-identity.md` at
> SemStreams commit `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`. It is carried for
> `setup-04a-04-graph-roots-clustering` (the `graph/clustering` port, draft PR #483). Its admission-ledger row is
> `docs/adr/099-community-partition-derived-from-entity-identity.md` in `docs/admission-ledger.yaml`. The decision
> text below is unchanged. Read it with these facts about this repository:
>
> - Issue numbers in the text (`gh#606`, `gh#590`, `gh#820`, `#465`, `#672` and the others) and the files
>   `docs/proposals/gh606-*` are SemStreams'. The pin never carried this decision out: its `graph-clustering` still
>   ran label propagation. `graph/clustering` is the first code that does.
> - Applied: the base partition is the entity-ID prefix, never detected. Membership is a pure function of the ID and
>   is not stored; a record holds a member count, counts by type, keywords and a few sample IDs, bounded by
>   construction (`Community`, `MaxRecordBytes`). The `Maintainer` keeps the groups current from entity changes
>   (`Upsert`, `Delete`) and rewrites only the groups that changed on each `Tick`, instead of the pin's interval
>   loop that reads every entity. Per the owner's ruling of 2026-10-31 on PR #483 the base partition is kept from
>   changes, never by rescanning.
> - Levels as amended by ADR-102 (`docs/adr/102-entity-id-segment-semantics.md`): level 0 is
>   `org.platform.system.domain`, level 1 is the source `org.platform.system`, level 2 is the deployment
>   `org.platform`. This record's phrase "level 0 = system (4 parts), 1 = domain (3), 2 = platform (2)" is the
>   phrasing ADR-102 replaces.
> - Not carried: label propagation, the entity-ID and semantic-edge providers that fed it, and PageRank (their
>   ledger rows name this record). The detection overlay this record allows stays a later change with its own gate
>   (a fixture that can fail, off by default, never a readiness input).
> - Tier 2 is not here. The LLM summaries and the content-addressed `COMMUNITY_SUMMARIES` store (ADR-087, not
>   ported) belong to issue #485. The owner dropped `CommunitySummarizer` and `StatisticalSummarizer` from tier 0
>   (2026-10-31); a record's `statistical_summary` is a one-line template over its counts.
> - Not decided here: how `graph/query` reads these records during a tick, and the watch wiring in
>   `processor/graph-clustering` (change 7). The statement "zero shipped deployments consume the community index" is
>   the pin's measurement, not this repository's.

## Status

**Accepted (2026-08-23).** Owner ruling recorded on gh#606 (triage docket) with the eight
follow-on rulings in `docs/proposals/gh606-derived-communities-design.md` §4. Mechanics belong
to the `graph-clustering` and `graph-query` capability specs via the forthcoming
`gh606-derived-communities` OpenSpec change; this page records only the decision.

## Context

LPA community detection approximated a value the framework already holds. The component
synthesized sibling/system-peer edges *from the 6-part entity ID* and fed them to LPA — while
shipped tiers additionally materialized prefix-derived hierarchy edges (`enable_hierarchy`),
so detection re-derived the ID structure from two directions at once. Field measurement
(semsource, three corpora, recorded on gh#606): largest community 47–82% of the graph — the
system filter yields the useful partition without detection; zero shipped deployments consume
the community index; "better clusters are not better answers." The hierarchy levels above 0
were re-runs over the full entity set, not a hierarchy (gh#606 Finding 2). The e2e coherence
check could not exceed 1 of 3 and was warn-only. ADR-086's semantic-edge tier measured an
honest negative and ships default-off.

## Decision

The community base partition is **derived, not detected**: `community(entity, level)` is the
entity ID prefix at that level — level 0 = system (4 parts), 1 = domain (3), 2 = platform (2).
Community identity is the prefix itself; levels are structurally distinct; membership is a pure
function of the ID and is **never stored** — group records carry bounded metadata only, and
member enumeration rides the existing paginated prefix-query lane. Derivation runs in
`graph-clustering`'s existing interval loop reading ENTITY_STATES only; it does not re-enter
the graph-ingest hot path (the 2026-01-05 monolith-breakup boundary stands), takes no
graph-index readiness dependency, and writes on change only. The 5-part type prefix is not a
community level; type grouping remains served by `hierarchyStats`.

LPA and its supporting machinery — edge synthesis and its weights/caps configuration, the
semantic-edge provider, detection determinism — **leave the tree**. Detection may return only
as an overlay under a re-entry contract: explicit relationship edges in, a namespace the
serving path does not read out, a fixture that can fail as the shipping gate, and never a
readiness dependency of graph-query. Recovery of the removed mechanism is by git history plus
that contract (ADR-061 pattern).

Unchanged and binding: the ADR-087 content-addressed COMMUNITY_SUMMARIES ownership split; the
gh#820 operation-local readiness contract; single-writer topology with no ownership system
(ADR-091 posture). GraphRAG and PathRAG advertised surfaces hold shape-for-shape; the one
advertising change is honest — communities are declared organization (the ID hierarchy), and
"emergent structure" becomes the overlay's claim, unshippable until measurable. LLM summary
enhancement gates to level 0 by default. Removed configuration keys fail at load with
replacement guidance.

## Consequences

The partition is deterministic, restart-stable, and O(1) at entity birth; cold-start
communities exist as soon as entities do. Bounded records end the payload-growth class for
this store. The gh#606 dependent cluster resolves per the design's disposition table
(#465/#672 dissolve; #661 largely dissolves; #608/#618/#701/#829/#588 re-scope or re-judge; #839's
community half dissolves). Breaking, pre-v1 fresh-state; `task e2e:statistical` and
`task e2e:semantic` must be green before the breaking commit lands.

## References

- Owner ruling + measurements: gh#606 (2026-08-23 comments)
- Inventory / design (mechanics source for the spec deltas):
  `docs/proposals/gh606-derived-communities-inventory.md`,
  `docs/proposals/gh606-derived-communities-design.md`
- [ADR-061](061-community-semantic-virtual-edges.md) (removal + recoverability pattern),
  [ADR-085](085-gate-on-health-report-freshness.md) (serve-stale-over-empty posture, retained),
  [ADR-086](086-semantic-colocation-edges-community-detection.md),
  [ADR-087](087-community-summary-store-ownership.md), [ADR-090](090-authoritative-current-state-and-materialized-views.md),
  [ADR-091](091-graph-mutation-authority-without-semantic-ownership.md)
