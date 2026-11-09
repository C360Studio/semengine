# Design: `pkg/fusion` lens path with `fusionvocab`

Base `431dfc4` (claim on main `67a8650`); pin `8b99efe9`. Owner rulings: PR #550 comment 6716972354 (split; item 4,
fusion reads not-ready from the reply). Inventory: `inventory.md`. Pin paths are `pin:pkg/fusion/`. "Unit" pin tests
ran in the owner's probes (PR A inventory § 0: `-race` once, `-count=5 -cpu=1 -shuffle=on`, rc=0); the package has
no integration test. **Goal.** A product composes the lens engine over any `RetrievalClient` and gets an answer that
never hides what it left out. **Non-goals:** the NATS client and the no-embedder proof (PR D); `searchGraph`.

## D1. Readiness rides the reply (owner ruling 4; ADR-109 D1, D3)

`Fuse` asks no status. `RetrievalClient.Status`, `IndexStatus`, `IndexState` and its four constants,
`ErrReadinessUnknown`, `readinessEnvelope`, `unknownReadinessStatus`, the status re-sample in `notReadyEnvelope`, and
`Response.Index` are not carried (F1-F3; pin `engine_lens.go:124-150`, `:305-324`; `contract.go:72-177`). Fusion no
longer imports `graph/readiness`, and has nothing to decode `graph.IndexStatusResponse` into: #110 item 2 asked for
that decode, ruling 4 removes the read. A classified `index_not_ready` from `Resolve`, `Entities` or a relations
`Neighbors` call yields `Deferred: true`, `defer_reason: "index_not_ready"`, no nodes and no miss, as the pin did for
those reads (`:155-165`, `:197-201`; pin test `engine_lens_readiness_test.go:TestEngine_ReadinessTransient_Degrades`,
unit). Every other error, `graph_state_reset_required` included, returns as the error (pin
`TestEngine_GenuineError_Propagates`, unit). All seeds unhydrated stays deferred `all_seeds_unhydrated` with no miss
(`:173-183`). The wire changes, so `ContractVersion` becomes `"2"`.

**Does fusion wait on graph-embedding's degraded status?** No. At the pin it never read it: fusion's only status read
was graph-index's key (`pin:fusionnats/client.go:139` — `readiness.NewWatcher(src, readiness.KeyGraphIndex)`). Under
ruling 4 it reads no key at all. Whether the embedding verbs may answer is decided once, by the responder: since PR B
(F2, `graph/follower/follower.go:75-79`) they withhold `index_not_ready` only on hop-1 failures, which can leave a
stale vector, and answer around Worker failures, counting them in `failed_records`. Fusion defers exactly when that
reply withholds, so one document that fails to embed never stops fusion; D3 carries the count. *Proof:*
`TestDeferredOnIndexNotReady` (resolve, hydrate, relations), `TestFatalReplyReturnsError`,
`TestAllSeedsUnhydratedDefersWithoutMiss`, `TestContractVersionIsTwo`.

## D2. The graph facet's revision span (owner question 1)

`ViewRevision{Start, End}` was the status's `indexed_revision` before and after the facet's reads
(`engine_graph.go:198-210`); its source is gone. **G1** thread a revision out of `Neighbors` (the relationships reply
is enveloped) and report min/max: changes the SPI every fake implements, and still says nothing about the facts,
which come from `entity` and `batch`, bare replies (`graph/query_verbs.go:95-98`). **G2 (recommended)** drop
`view_revision`; the facet's doc says its reads are not one snapshot. No SemSource reader (inventory § 5). *Proof:*
`TestGraphProjectionRoundTrip` (ported from `engine_graph_test.go:585`, without the field).

## D3. What the semantic search could not read (owner question 2)

The search reply carries `unreadable_records` and `failed_records` (`graph/query_search_types.go:58-59`); the pin's
`Resolve` returns `[]Seed` and drops them. **R1 (recommended)** `Resolve` returns `Resolution{Seeds,
UnreadableRecords, FailedRecords}`; `Response` carries both under the same JSON names, omitted when 0 (symbol and
prefix modes always 0). **R0** keep `[]Seed`; the doc says an NL answer can miss failed entities, with nothing to
count them. *Proof:* `TestNLAnswerCarriesSearchCounts`, `TestSymbolAnswerOmitsCounts`.

## D4. Every left-out piece is labelled (owner question 4; SemStreams #621)

- Relations: `Node.RelationsTruncated` is true when a role passed `maxRelationsPerNode` (F8, `engine_lens.go:495`), a
  non-readiness `Neighbors` fault was skipped (F9, `:491`; replaces pin `TestEngine_NeighborGenuineError_Swallowed`),
  or a target entity could not be read (F10, `:508`).
- Paths: `Response.PathsTruncated` is true when more paths existed than `maxPaths` (F11, `engine_facets.go:117`).
  A path whose next node cannot be read stops before it, marked `Truncated`, instead of naming the node by its ID
  (F12, `:198-203`; `Path`'s doc, `:14-15`, says never an ID).
- Impact: a node whose file cannot be read sets `Impact.Truncated` (F13, `:91`, `:206-211`).
- Graph: an incoming counterpart that fails to read (an error, not absent) sets `GraphProjection.Truncated` (F14,
  `engine_graph.go:231-234`); absent keeps the pin's edge without evidence (pin
  `TestGraph_IncomingCounterpartAbsent_EdgeWithoutEvidence`, unit).

The caps keep their pin values. *Proof:* `TestRelationsCapLabelled` (12 edges false, 13 true),
`TestRelationsFaultLabelled`, `TestRelationsUnreadTargetLabelled`, `TestPathsCapLabelled` (10 false, 11 true),
`TestPathStopsAtUnreadNode`, `TestImpactUnreadFileTruncates`, `TestGraphCounterpartFaultTruncates`; a mutant per label.

## D5. Construction, metrics, nil context

`NewEngine(graph RetrievalClient, body *BodyResolver, opts ...Option) (*Engine, error)`; `WithSignals` and
`WithMetrics` become options, so nothing changes an engine after it is built (F7, `engine_lens.go:85-110`). A nil
client is refused. The body-failure counter registers through `metric.RegisterOrGet` on the injected registry and a
refusal is returned (F5, F6: pin `metrics.go:41` falls back to `prometheus.DefaultRegisterer`, `:46-64` hand-rolls
register-or-get and logs a refusal it swallows). With no registry there is no counter; `body_reason` still marks the
node. `Fuse` and `ResolveBody` refuse a nil context. Rejected: keeping the builders, since `WithMetrics(...) *Engine`
has nowhere to return a refusal. *Proof:* `TestNewEngineRefusesNilClient`, `TestCounterOnlyOnInjectedRegistry`
(default gatherer unchanged), `TestEnginesShareOneSeries`, `TestRegistrationRefusalReturned`,
`TestFuseRefusesNilContext`, `TestResolveBodyRefusesNilContext`.

## D6. `fusionvocab` and the ontology (owner question 3)

`ClassSpecificity` is the number of ancestors from `cco.Parents` (`fusionvocab/signals.go:40`), not two IRI strings
as the ledger says (`docs/admission-ledger.yaml:2723-2725`). `vocabulary/cco` is in semvocab (#234), and semvocab's
`go.mod` requires `github.com/c360studio/semengine`, so SemEngine cannot import it back. **V1 (recommended)**
`New(parents func(iri string) []string) Signals`; SemSource passes semvocab's `cco.Parents`
(`semvocab:catalog/cco/hierarchy.go:64`); nil means `ClassSpecificity` is 0, ranking then as with no ontology.
**V2** count `vocabulary.EntityTypeMetadata.Superclasses` (`vocabulary/entity_type.go:33`): only registered types,
and only the superclasses registered, so a different number under the same name. **V3** a subclass registry in
`vocabulary` that semvocab fills: new surface, its own PR (split.md). **V4** drop the ontology signal. *Proof:*
`TestClassSpecificityCountsParents`, `TestClassSpecificityNilParentsIsZero`, `TestPredicateSalience` (ported).

## D7. Hydration (ADR-063)

`BodyResolver`, `StoreResolver`, `MapStoreResolver` as at the pin. `hydrate_test.go` asserts at compile time that
`*storeregistry.Registry` is a `StoreResolver` (ledger `storage/storeregistry`, `:2271-2272`). *Proof:* the pin's
`hydrate_test.go` tests, ported.

## D8. Kept as at the pin, and not carried

Kept, optional findings (inventory § 8): the swallowed `Names` error in a miss (F16, a suggestion list), the
unmarshalable-fact skip (F15, no trigger: objects come from decoded JSON), `entity.ontology.class` in the engine
(F17), the deferred answer's provenance (F22), paths and impact reading `index_not_ready` as a walk fault (F23),
`RankSignals` with one implementation (keeps fusion free of `vocabulary`). Ranking, budget, scores, facets, `Lens`,
`Want`, `Facet`, `Unhydrated` and `BodyReason` carry; `UnhydratedReason`'s doc stops claiming to mirror
`graph.MissingReason`, which dropped `unknown` (F19). Not carried: `engine.go`, `subquery.go`, `evidence.go`,
`client.go` and their tests (03B D5); `lensregistry` (pin callers: its own test and `storeregistry_test.go`).
Ranking has interacting inputs: `TestRankIsStablePermutation` (generated, seed recorded) checks the output is a
permutation and keeps resolve order among equal scores; the pin's signal tests stay as examples.

## D9. ADRs, issues, guidance, order

Ported: ADR-062 with a port note (lens path only; `lensregistry` and the `Fuse` path not ported; increment 5's ontology
helper in semvocab; its honesty envelope replaced under ruling 4). Applied: ADR-063, ADR-071 (`Scope` to `Resolve`),
ADR-084 D3 (no absence claim), D4 (what `index_not_ready` means), D6 (omission reported), ADR-109 D1, D3, ADR-112 (no
fixed timeout). Changed by ruling 4: ADR-066 :338-344 and :394 (the mirror and the per-request gate), ADR-082 :65 (not
ported; exact consumers gate on `Ready`), the status-unknown defer (pin `contract.go:144-153`, `engine_lens.go:125-137`;
ADR-084 D7's hard stops already bind through the reply, ADR-109 table). Issues: inventory § 9. Guidance: SemStreams'
"Exported-surface" and "Guarantee, signal, and revision" sections and "Storage and retention" are in the tree's
contracts already (`.agents/contracts/semengine-developer.md:163`, `:177`, `:270`). `pkg/fusion` joins
`scripts/cover-check.sh` at 80% (03B D10 critical list); `fusionvocab` does not. Pin lines 5,654 against the ~3,000
guide: the owner's split put `fusionvocab` here so tests have a real signal. Order: no overlapping PR; PR D after.
