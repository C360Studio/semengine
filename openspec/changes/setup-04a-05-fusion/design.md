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
longer imports `graph/readiness` and decodes no `graph.IndexStatusResponse`: #110 item 2 asked for that decode, ruling
4 removes the read. Every read `Fuse` makes is sorted into one of four outcomes, in this order:

| Error from a read | `Resolve`, `Entities` | relations, refs, paths, impact, graph walks |
| --- | --- | --- |
| the context ended | `Fuse` returns the context's error | same |
| fatal class (`errs.IsFatal`; e.g. `graph_state_reset_required`) | returned | returned, the whole answer fails |
| `index_not_ready` | deferred `index_not_ready`, no nodes, no miss | relations: deferred; walks: that facet truncated (F23) |
| any other | returned | skipped and declared (D4) |

A reset therefore fails the call instead of serving relations all marked truncated; at the pin the status gate deferred
it `hard_stop` (pin `engine_lens.go:147-150`), which is gone with the gate. `defer_reason` keeps only `index_not_ready`
and `all_seeds_unhydrated`; the gate's `hard_stop`, `status_unknown`, `bootstrap_incomplete` and `unrecognized_state`
leave the wire, and `Response.DeferReason`'s doc (pin `contract.go:288-289`) is rewritten. All seeds unhydrated stays
deferred `all_seeds_unhydrated` with no miss (`:173-183`). `ContractVersion` becomes `"2"`. Pin evidence:
`engine_lens_readiness_test.go:TestEngine_ReadinessTransient_Degrades`, `TestEngine_GenuineError_Propagates` (unit).

**Does fusion wait on graph-embedding's degraded status?** No. At the pin it never read it: fusion's only status read
was graph-index's key (`pin:fusionnats/client.go:139` — `readiness.NewWatcher(src, readiness.KeyGraphIndex)`). Under
ruling 4 it reads no key at all. Whether the embedding verbs may answer is decided once, by the responder: since PR B
(F2, `graph/follower/follower.go:75-79`) they withhold `index_not_ready` only on hop-1 failures, which can leave a
stale vector, and answer around Worker failures, counting them in `failed_records`. Fusion defers exactly when that
reply withholds, so one document that fails to embed never stops fusion; D3 carries the count. *Proof:*
`TestDeferredOnIndexNotReady` (resolve, hydrate, relations), `TestFatalReplyReturnsError` (from `Entities` and from a
walk), `TestEndedContextReturnsError`, `TestAllSeedsUnhydratedDefersWithoutMiss`, `TestContractVersionIsTwo`.

## D2. The graph facet's revision span (decided: dropped)

`ViewRevision{Start, End}` was the status's `indexed_revision` before and after the facet's reads
(`engine_graph.go:198-210`). Ruling 4 removes its source, and under the owner's 2026-11-02 ruling (PR #504) a consumer
break is a migration note. A replacement threaded out of `Neighbors` would still say nothing about the facts, which
come from `entity` and `batch`, bare replies (`graph/query_verbs.go:95-98`). `view_revision` goes; the facet's doc says
its reads are not one snapshot. No SemSource reader (inventory § 5). *Proof:* `TestGraphProjectionRoundTrip` (ported
from `engine_graph_test.go:585`, without the field).

## D3. What the semantic search could not read (owner question 1)

The search reply carries `unreadable_records` and `failed_records` (`graph/query_search_types.go:58-59`); the pin's
`Resolve` returns `[]Seed` and drops them. **R1 (recommended)** `Resolve` returns `Resolution{Seeds,
UnreadableRecords, FailedRecords}`. Every response built after a natural-language resolve answered (nodes, a miss, or
a later deferral) carries both under the same JSON names; each is omitted when 0, and both always are for symbol and
prefix resolves. A miss matters most: "nothing found" while 3 records were unreadable is not "nothing there". **R0**
keep `[]Seed`; the doc says an NL answer can miss failed entities, with nothing to count them. *Proof:*
`TestNLAnswerCarriesSearchCounts`, `TestNLMissCarriesSearchCounts`, `TestSymbolAnswerOmitsCounts`.

## D4. Every left-out piece is declared (owner question 3)

AGENTS.md § 1 (any skip, drop, or degrade is declared); SemStreams #621. Each `Neighbors` result is sorted by predicate
then target and deduplicated before any cap (F24; AGENTS.md § 2 sort before limit), so which edges a cap keeps does not
depend on reply order. Labels:

- Relations: `Node.RelationsTruncated` when a role passed `maxRelationsPerNode` (F8, `engine_lens.go:495`), a
  relations `Neighbors` read was skipped (F9, `:491`; replaces pin `TestEngine_NeighborGenuineError_Swallowed`), or a
  target entity read was skipped (F10, `:508`). A target that reads as absent is a dangling edge, also left out and
  labelled.
- Paths: `Response.PathsTruncated` when more paths existed than `maxPaths` (F11, `engine_facets.go:117`), or a path was
  dropped. A path whose next node cannot be read ends at its last readable node, marked `Truncated`, never naming the
  node by its ID (F12, `:198-203`; `Path`'s doc, `:14-15`). The pin's rule that a seed alone is no path stays
  (`:164-167`): when the unreadable node is the seed's first hop, nothing is emitted and `PathsTruncated` is set.
- Impact: `Impact.Truncated`, as at the pin for the node cap (`:86-88`) and a walk fault (`:70-72`), and now also when
  an affected node's read for its file was skipped (F13, `:91`, `:206-211`).
- Graph: `GraphProjection.Truncated` when an incoming counterpart's read was skipped (F14, `engine_graph.go:231-234`).
  A counterpart that reads as absent keeps the pin's edge without evidence, not truncated (pin
  `TestGraph_IncomingCounterpartAbsent_EdgeWithoutEvidence`, unit).

A cap is the answer's designed bound, as the pin's budget `truncated` is: the wire label is its declaration. A skipped
read is a degrade, and `.agents/contracts/semengine-developer.md:197` asks for a log line and a metric as well: each
skip logs one Warn line naming the facet and the read, and increments `fusion_skipped_reads_total{facet}` (`relations`,
`paths`, `impact`, `graph`) on the injected registry (D5). The caps keep their pin values. *Proof:*
`TestEdgesSortedBeforeCap`, `TestRelationsCapLabelled` (12 edges false, 13 true), `TestRelationsFaultLabelled`,
`TestRelationsUnreadTargetLabelled`, `TestPathsCapLabelled` (10 false, 11 true), `TestPathStopsAtUnreadNode` (second and
first hop), `TestImpactUnreadFileTruncates`, `TestGraphCounterpartFaultTruncates`, `TestSkippedReadLoggedAndCounted`; a
mutant per label and for the log and the counter.

## D5. Construction, metrics, logging, nil context

`NewEngine(graph RetrievalClient, body *BodyResolver, logger *slog.Logger, opts ...Option) (*Engine, error)`;
`WithSignals` and `WithMetrics` become options, so nothing changes an engine after it is built (F7,
`engine_lens.go:85-110`). A nil client or logger is refused, as `readiness.NewWatcher` refuses a nil logger (ledger
`graph/readiness`, `:2169-2170`). Both counters, body failures and skipped reads, register through
`metric.RegisterOrGet` on the injected registry, and a refusal is returned (F5, F6: pin `metrics.go:41` falls back to
`prometheus.DefaultRegisterer`, `:46-64` hand-rolls register-or-get and logs a refusal it swallows). With no registry
there are no counters; the log line and the wire labels remain. A body failure also logs. `Fuse` and `ResolveBody`
refuse a nil context. Rejected: keeping the builders, since `WithMetrics(...) *Engine` has nowhere to return a
refusal. *Proof:* `TestNewEngineRefusesNilClient`, `TestNewEngineRefusesNilLogger`, `TestCounterOnlyOnInjectedRegistry`
(default gatherer unchanged), `TestEnginesShareOneSeries`, `TestRegistrationRefusalReturned`,
`TestFuseRefusesNilContext`, `TestResolveBodyRefusesNilContext`.

## D6. `fusionvocab` and the ontology (owner question 2)

`ClassSpecificity` is the number of ancestors from `cco.Parents` (`fusionvocab/signals.go:40`), not two IRI strings as
the ledger says (`docs/admission-ledger.yaml:2723-2725`). `vocabulary/cco` is in semvocab (#234), and semvocab's
`go.mod` requires `github.com/c360studio/semengine`, so SemEngine cannot import it back. **V1 (recommended)**
`New(parents func(iri string) []string) Signals`; SemSource passes semvocab's `cco.Parents`
(`semvocab:catalog/cco/hierarchy.go:64`); nil means `ClassSpecificity` is 0, ranking then as with no ontology. **V2**
count `vocabulary.EntityTypeMetadata.Superclasses` (`vocabulary/entity_type.go:33`): only registered types, and only the
superclasses registered, so a different number under the same name. **V3** a subclass registry in `vocabulary` that
semvocab fills: new surface, its own PR (split.md). **V4** drop the ontology signal. *Proof:*
`TestClassSpecificityCountsParents`, `TestClassSpecificityNilParentsIsZero`, `TestPredicateSalience` (ported). SemSource
needs semvocab under V1 anyway: it imports `vocabulary/cco` itself today (`semsource:source/ontology/ontology.go:24`).

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
permutation of the input, by multiset; its equal-score oracle is built, not computed: inputs whose entities share one
name that matches no query and carry no signals score equal by construction, so the expected order is resolve order.
The pin's signal tests stay as examples.

## D9. ADRs, issues, guidance, order

Ported: ADR-062 with a port note (lens path only; `lensregistry` and the `Fuse` path not ported; increment 5's ontology
helper in semvocab; its honesty envelope replaced under ruling 4). Applied: ADR-063, ADR-071 (`Scope` to `Resolve`),
ADR-084 D3 (no absence claim), D4 (what `index_not_ready` means), D6 (omission reported), ADR-109 D1, D3, ADR-112 (no
fixed timeout). Changed by ruling 4: ADR-066 (pin :320-322 and :372, tree copy :343-344 and :394: the mirror and the
per-request gate), pin ADR-082 :65 (not ported; exact consumers gate on `Ready`), ADR-085 D1 (tree copy :58, fusion as a
gate reader), the status-unknown defer (pin `contract.go:144-153`, `engine_lens.go:125-137`;
ADR-084 D7's hard stops already bind through the reply, ADR-109 table). Issues: inventory § 9. Guidance: SemStreams'
"Exported-surface" and "Guarantee, signal, and revision" sections and "Storage and retention" are in the tree's
contracts already (`.agents/contracts/semengine-developer.md:163`, `:177`, `:270`). `pkg/fusion` joins
`scripts/cover-check.sh` at 80% (03B D10 critical list); `fusionvocab` does not. Pin lines 5,654 against the ~3,000
guide: the owner's split put `fusionvocab` here so tests have a real signal. Order: no overlapping PR; PR D after.
