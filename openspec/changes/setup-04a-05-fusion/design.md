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
4 removes the read. Every read `Fuse` makes ends in one of these outcomes, checked in this order:

| Error from a read | `Resolve`, `Entities` | relations `Neighbors` | relation targets, paths, impact, graph walks |
| --- | --- | --- | --- |
| the context ended | `Fuse` returns it | same | same |
| `index_not_ready` | deferred, no nodes, no miss | deferred | skipped and declared (F23) |
| fatal class (`errs.IsFatal`) | returned | skipped and declared | skipped and declared |
| any other | returned | skipped and declared | skipped and declared |

A fatal error from a walk read is skipped, not returned: graph-ingest refuses a poisoned entity as fatal
`graph_state_reset_required` (`processor/graph-ingest/query.go:634-649`), and ADR-109 D1 says "poison refuses only the
poisoned entities" (`docs/adr/109-ready-after-boot.md:77`); fusion cannot tell that refusal from a graph-index reset
over the reply, and the pin skipped these reads too (`engine_lens.go:491`, `:508`; `engine_facets.go:188-211`). A
fatal `Resolve` or `Entities` fails the call: graph-ingest's batch refuses the whole batch when any entity in it is
poisoned (`:617-619`), so one poisoned seed fails the answer; PR D decides how fusionnats, which owns the batch
call, handles it (never by parsing error text, #146). At the pin a reset deferred `hard_stop` through the gate (pin
`engine_lens.go:147-150`), now gone. `defer_reason` keeps only `index_not_ready` and `all_seeds_unhydrated`; the gate's
`hard_stop`, `status_unknown`, `bootstrap_incomplete` and `unrecognized_state` leave the wire, and
`Response.DeferReason`'s doc (pin `contract.go:288-289`) is rewritten. All seeds unhydrated stays deferred with no
miss (`:173-183`). `ContractVersion` becomes `"2"`. Pin evidence: `engine_lens_readiness_test.go`
`TestEngine_ReadinessTransient_Degrades`, `TestEngine_GenuineError_Propagates` (unit).

Fusion does not wait on graph-embedding's degraded status: it reads no status key, the pin read only graph-index's,
and the embedding verbs decide for themselves when to withhold (inventory § 6). *Proof:* task 2.1.

## D2. The graph facet's revision span (decided)

`view_revision` goes with the status read that filled it (ruling 4; a consumer break is a migration note, owner
2026-11-02 on PR #504); the facet's doc says its reads are not one snapshot. Reasoning: inventory § 6.

## D3. What the semantic search could not read (owner question 1)

The search reply carries `unreadable_records` and `failed_records` (`graph/query_search_types.go:58-59`); the pin's
`Resolve` returns `[]Seed` and drops them. **R1 (recommended)** `Resolve` returns `Resolution{Seeds,
UnreadableRecords, FailedRecords}`. Every response built after a natural-language resolve answered (nodes, a miss, or
a later deferral) carries both under the same JSON names; each is omitted when 0, and both always are for symbol and
prefix resolves. A miss matters most: "nothing found" while 3 records were unreadable is not "nothing there". **R0**
keep `[]Seed`; the doc says an NL answer can miss failed entities, with nothing to count them. *Proof:* task 2.2.

## D4. Every left-out piece is declared (owner question 3)

AGENTS.md § 1 (any skip, drop, or degrade is declared); SemStreams #621. Each `Neighbors` result is sorted by predicate
then target and deduplicated before any cap (F24; AGENTS.md § 2 sort before limit), so which edges a cap keeps does not
depend on reply order; a node's refs therefore list by predicate, then target. A skipped read is any walk read D1's
table marks "skipped and declared". Labels:

- Relations: `Node.RelationsTruncated` when a role passed `maxRelationsPerNode` (F8, `engine_lens.go:495`), a
  relations `Neighbors` read was skipped (F9, `:491`; replaces pin `TestEngine_NeighborGenuineError_Swallowed`), or a
  target was skipped or read as absent (F10, `:508`).
- Paths: `Response.PathsTruncated` when more paths existed than `maxPaths` (F11, `engine_facets.go:117`), or a path was
  dropped. A path whose next node is skipped or reads as absent ends at its last readable node, marked `Truncated`,
  never naming the node by its ID (F12, `:198-203`; `Path`'s doc, `:14-15`). The pin's rule that a seed alone is no
  path stays (`:164-167`): when that node is the seed's first hop, nothing is emitted and `PathsTruncated` is set.
- Impact: `Impact.Truncated`, as at the pin for the node cap (`:86-88`) and a walk fault (`:70-72`), and also when an
  affected node's read for its file was skipped (F13, `:91`, `:206-211`).
- Graph: `GraphProjection.Truncated` when an incoming counterpart's read was skipped (F14, `engine_graph.go:231-234`);
  one that reads as absent keeps the pin's edge without evidence, not truncated (pin
  `TestGraph_IncomingCounterpartAbsent_EdgeWithoutEvidence`, unit).

A cap is the answer's designed bound, as the pin's budget `truncated` is: the wire label is its declaration. A skipped
read is a degrade, and `.agents/contracts/semengine-developer.md:197` asks for a log line and a metric as well: each
skip increments `fusion_skipped_reads_total{facet}` (`relations`, `paths`, `impact`, `graph`) on the injected registry,
and each `Fuse` call logs at most one Warn line per facet that skipped, naming the facet, the count and the first
error, so an index outage costs four lines per call, not one per edge. The caps keep their pin values. *Proof:* task
2.2 and 2.3.

## D5. Construction, metrics, logging, nil context

`NewEngine(graph RetrievalClient, body *BodyResolver, logger *slog.Logger, opts ...Option) (*Engine, error)`;
`WithSignals` and `WithMetrics` become options, so nothing changes an engine after it is built (F7,
`engine_lens.go:85-110`). A nil client or logger is refused, as `readiness.NewWatcher` refuses a nil logger (ledger
`graph/readiness`, `:2169-2170`). Both counters, body failures and skipped reads, register through
`metric.RegisterOrGet` on the injected registry, and a refusal is returned (F5, F6: pin `metrics.go:41` falls back to
`prometheus.DefaultRegisterer`, `:46-64` hand-rolls register-or-get and logs a refusal it swallows). With no registry
there are no counters; the log line and the wire labels remain. A body failure also logs. `Fuse` and `ResolveBody`
refuse a nil context. Rejected: keeping the builders, since `WithMetrics(...) *Engine` has nowhere to return a
refusal. *Proof:* task 2.3.

## D6. `fusionvocab` and the ontology (owner question 2)

`ClassSpecificity` is the number of ancestors from `cco.Parents` (`fusionvocab/signals.go:40`), not two IRI strings as
the ledger says (`docs/admission-ledger.yaml:2723-2725`). `vocabulary/cco` is in semvocab (#234), and semvocab's
`go.mod` requires `github.com/c360studio/semengine`, so SemEngine cannot import it back. **V1 (recommended)**
`New(parents func(iri string) []string) Signals`; SemSource passes semvocab's `cco.Parents`
(`semvocab:catalog/cco/hierarchy.go:64`); nil means `ClassSpecificity` is 0, ranking then as with no ontology. **V2**
count `vocabulary.EntityTypeMetadata.Superclasses` (`vocabulary/entity_type.go:33`): only registered types, and only the
superclasses registered, so a different number under the same name. **V3** a subclass registry in `vocabulary` that
semvocab fills: new surface, its own PR (split.md). **V4** drop the ontology signal. SemSource needs semvocab under V1
anyway: it imports `vocabulary/cco` itself today (`semsource:source/ontology/ontology.go:24`). *Proof:* task 2.4.

## D7. Kept, not carried, hydration

Optional findings kept as at the pin, and what is not carried: inventory § 8. Hydration keeps ADR-063's contract;
`hydrate_test.go` asserts at compile time that `*storeregistry.Registry` is a `StoreResolver` (ledger
`storage/storeregistry`, `:2271-2272`). *Proof:* task 2.3.

## D8. ADRs, order, size

ADR-062 is ported with a port note; applied ADRs and guidance: inventory § 7. Changed by ruling 4: ADR-066 (pin
:320-322 and :372, tree copy :343-344 and :394: the mirror and the per-request gate), pin ADR-082 :65 (not ported;
exact consumers gate on `Ready`), ADR-085 D1 (tree copy :58, fusion as a gate reader), and the status-unknown defer
(pin `contract.go:144-153`, `engine_lens.go:125-137`). Pin lines 5,654 against the ~3,000 guide: the owner's split put
`fusionvocab` here so tests have a real signal. Order: no overlapping PR; PR D after.
