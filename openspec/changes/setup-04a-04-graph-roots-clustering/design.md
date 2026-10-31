# Design: setup-04a-04-graph-roots-clustering

Base: `f3b48f6`. Inventory: `inventory.md` (round 1; this design waits on its `INVENTORY PASS`). Change 4, PR 9 of 10
(#9 comment 6386856657, item 8). Tier 0 only (#9 comment 6386603659). Draft PR #483. Governing records: ADR-085
(ported, kept), ADR-099 (SemStreams owner ruling at the pin, unimplemented there; question 1), ADR-087 and ADR-086
(not ported, their mechanisms excluded), the pin spec `graph-clustering` (inventory 6). Probes: inventory, Pin probe.

Coined terms: a *partition* is one community per entity, written to `COMMUNITY_INDEX`; *explicit edges* are the
relationship edges a `graph.Provider` returns; *virtual edges* are edges the pin synthesizes from entity-ID positions
(sibling, system peer) or embedding similarity (semantic).

## Options

- **A. Port the pin's tier-0 half whole** (4,231 pin lines; waiver needed). Carries what the pin's own ADR-099 rules
  out: virtual-edge synthesis with a cache that never refreshes (P1), semantic edges, and levels 1-2 that are not a
  hierarchy (P2). Instant debt by the owner's bar.
- **B. Explicit-edge clustering, the half both rulings keep (recommended).** 2,503 pin lines, no waiver. LPA over the
  provider's explicit edges, one level, the statistical summary, write-then-prune storage. Matches 03B Q11 ("clustering
  on explicit edges"), ADR-099's removals and its overlay rule (explicit edges in, a fixture that can fail: semboids'
  two-clique test), and semboids' live configuration (virtual edges off, `enable_llm` false; inventory 12).
- **C. ADR-099's derived partition** (community = ID prefix, members not stored). New design, not a port: no pin code.
  It gives semboids one community for the whole flock (every boid shares `c360.semboids.sim.flock`), changes the
  `COMMUNITY_INDEX` record graph-query and semboids read, and adds a third spelling of ID-prefix grouping beside
  `types.EntityID` prefixes and `graph/inference` containers (inventory 2).
- **D. Do nothing until change 7.** Item 9 (`processor/graph-query`) cannot build its community path.

## D1. Files, edges, exclusions

Carried into `graph/clustering/`: `types.go`, `storage.go`, `lpa.go`, `summarizer.go` `:1-498`, `pagerank.go`, a new
`doc.go` and `README.md` (the product contract line). Import edges: `graph`, `natsclient`, `pkg/errs`, `pkg/types`;
no `graph/llm`, `metric`, Prometheus or `go-openai`. Ledger: one `adapt` row for the package and `defer-exclude` rows:

- tier 2, naming #32: `summarizer.go:499-863` (`LLMSummarizer`, `ProgressiveSummarizer`), `enhancement_worker.go`,
  `summary_store.go`, `metrics.go`, and `MembershipHash` (`storage.go:42-48`, the `COMMUNITY_SUMMARIES` key);
- ADR-099, naming question 1: `entityid_provider.go`, `semantic_edge_provider.go`;
- dead: `provider.go` (no reader; `QueryManagerProvider.GetAllEntityIDs` always fails, P4).

One line in `.agents/skills/entity-or-bucket/SKILL.md` item 2: community detection reads `ENTITY_STATES` through a
provider (the pin skill's clause, `pin:.agents/skills/entity-or-bucket/SKILL.md:47`).

## D2. The detector (observable behaviour)

- `NewLPADetector(provider, storage)`; `WithLogger`, `WithMaxIterations`, `WithSummarizer`, `SetEntityProvider` as at
  the pin. `DetectCommunities(ctx)` refuses a nil `ctx` and a nil provider or storage with a typed error.
- One level (**changed, question 3**): every community has `Level` 0; `WithLevels` and `detectHierarchicalLevel` go.
  The result keeps its pin type, a map keyed by level, now with key 0 only. The pin's levels 1-2 re-run LPA over
  the same entities (P2) and nothing sets `ParentID`.
- Kept from the pin spec: entity IDs and neighbours are sorted, the shuffle is seeded per run and ties go to the
  smallest label (`:301`); write-then-prune (`:66`, ADR-085): every community is saved before `Prune` runs; any save
  failure returns an error, skips `Prune` and reports no success; a `Prune` failure is logged and not returned.
- `ResetEdgeCache` is called on a provider that has it at the start of each run (the change 7 `kvProvider` does).
- Dropped as unread (inventory 4): `UpdateCommunities`, `GetCommunity`, `GetEntityCommunity`,
  `GetCommunitiesByLevel`, `InferRelationshipsFromCommunities` with `InferenceConfig`, `DefaultInferenceConfig`,
  `InferredTriple` (dormant at the pin, errors swallowed, and it would assert facts with no mutation or provenance
  contract, setup plan `:106-108`), `WithProgressiveSummarization`. `CommunityDetector` keeps `DetectCommunities`
  only; its one implementation is retained as the component's seam (optional finding S8).

## D3. Storage

`NATSCommunityStorage` holds a `*natsclient.KVStore` (the `graph/embedding.Storage` shape) and keeps `SaveCommunity`,
`Prune` and `GetAllCommunities` (read by the anomaly half, change 7). Required: S1 the nil-KV in-memory store goes
(#186; tests use a fake `CommunityStorage`); S2 the member-of triples and `NewNATSCommunityStorageWithConfig` go; S3
key listing uses `KVStore.Keys`, so no error text is searched; S4 an unreadable or undecodable entry fails
`GetAllCommunities` with an error naming the key; S5 a cancelled context returns its error. Kept: a value over the
payload ceiling is a permanent (invalid) error. Dropped as unread: `GetCommunity`, `GetEntityCommunity`,
`GetCommunitiesByLevel`, `DeleteCommunity`, `Clear`, `MaxCommunityLevels`. The key format is unchanged
(`{level}.{id}`, `entity.{level}.{id}`), so item 9 and semboids read the same bytes.

## D4. Summary and the community record

`CommunitySummarizer` and `StatisticalSummarizer` port (ruling 6386603659). Required: S9, keyword, representative
and type orders break ties by term or ID, so one membership always yields the same record (03B row `:218` admits the
summarizer as deterministic; P3 shows it is not). `Community` loses `ParentID`, `SummaryStatus`, `LLMSummary`,
`SummaryTruncated` (no reader, never stored since ADR-087). PageRank's exported names become unexported (S14).
Optional, retained: S12 sentinel choice, S13 the second parser.

## D5. ADRs

ADR-085 kept, port note gains this package. ADR-099 ported with a port note (question 1): SemEngine applies its
removals (virtual edges, semantic edges, levels) and keeps detection over explicit edges as the tier-0 capability 03B
admits, with semboids' clique test as its fixture; the derived base partition is not adopted. ADR-087 stays out with
the tier-2 lane (#32); ADR-086 and ADR-061 are not ported (the mechanism they decide is excluded).

## D5a. Spec delta

`specs/community-detection/spec.md` is written against option B. If the owner rules otherwise on question 1,
it is rewritten before review continues.

## D6. Proving tests

Unit, on fakes: the pin's LPA and storage tests rewritten off the nil-KV store; `TestDetectRefusesNilContext`;
`TestOneLevel`; `TestDetectDisjointCliques`; `TestNoVirtualEdges`; `TestSaveFailureDoesNotPrune`;
`TestPruneFailureIsNotFatal`; `TestSummaryDeterministic` (tied scores, 200 runs, one result; fails on the pin, P3);
`TestDetectionMatchesModel`, seeded, random explicit-edge graphs: every entity in exactly one community, the result
identical across two runs and across shuffled provider order. Integration on `natsfixture`: the pin's storage tests,
`TestGetAllCommunitiesFailsOnCorruptEntry`, `TestPruneReportsCancel`. Mutation checks: tie-break removed, `Prune`
called after a failed save, corrupt-entry skip restored. Coverage floor 80 % (pin 57 %).

## D7. Issues and merge order

Retires none. Stay open: #32 (gains the exclusion rows), #68 (`metrics.go`), #141 (`summary_store.go:179`), #477
(`enhancement_worker.go:239`): each site leaves with the tier-2 lane; a comment on each says so. No open PR overlaps;
if #464 merges first, the integration run repeats at nats.go 1.55.0. Item 9 ports after this and decides its
summary view (tier 2) and what a level-1/2 request returns.
