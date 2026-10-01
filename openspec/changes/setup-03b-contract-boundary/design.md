# Design: setup-03b-contract-boundary

This design is the architect's draft for SETUP 03B (epic #8, PR #21) against the inventory in `inventory.md` (sections
A1–A13). The independent inventory review passed on re-check. The owner ruled Q1–Q13 on 2026-10-01
([#8 comment 5929656835](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929656835); the questions are
[comment 5929442167](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929442167)); each decision below
states its ruling; the word for a capability level was ruled on #4 (D3). SemStreams paths are at the pin
`8b99efe9`; the inventory holds every measurement cited here. Each decision keeps its options and their costs as
the record of what was weighed.

## Context

The port set at the pin is 65 packages / 126,926 non-test lines (A1), reproduced exactly from Codex's measurement.
The 63→65 change is five entrants and three departures (A1.1): `engine`, `flowstore`, and `internal/lifecyclejoin`
no longer exist at the pin, so two of the six packages the epic asks about are resolved by the pin itself. The set
still reaches an agentic group of 10,264 lines through two files (`component/dependencies.go:7`,
`service/milestone_service.go:10`) and one import in graph-query (`graphrag.go:22`), a higher-tier graph group of
14,954 lines through `processor/graph-query` and `processor/graph-ingest`, and `gateway/graph-gateway` (2,995 lines)
that SemSource composes but the 03A workload never addresses (A3.3). SemConnect has become a second consumer at the
pin (PR semconnect#74) with a two-package, 2,137-line delta (A10).

## Decisions

### D1. Registration cut (#3)

Options:

- **(a) Explicit registration in the consumer's composition root.** Each consumer calls the per-package `Register`
  and `RegisterPayloads` functions it composes (A2.1 lists them; SemSource already does this for its own 14 factories
  at `run.go:301-320` and chains `RegisterPayloads` at `:289-296`). Cost: SemSource replaces two calls
  (`run.go:273,289`) with about eight component and two or three payload registrations; SemConnect builds the
  composition root PR #74 already requires. Benefit: SemEngine ships no aggregator; the compile closure of a consumer
  is exactly what it names; ADR-103's single type authority is unchanged (the registry object is still the one
  authority; only the caller moves).
- **(b) A narrowed engine-side registry** (`Register` admitting only graph components and payloads). Cost: a new
  aggregator that must choose a component set for every consumer at once; each later component widens every
  consumer's closure (the shape that produced 94- and 41-package pulls, A12.1); SemConnect, which composes no
  SemStreams component today, would import components it does not run.
- **(c) Keep importing `componentregistry`/`payloadbuiltins`.** Not available: both pull packages that will not exist
  in SemEngine (agentic, rule, research, governance, gated-dag; A2.1).

**Ruled (Q1, #3): (a)** — explicit per-package registration in each consumer's composition root; no aggregator, no
engine-side registry. The owner's quality bar: "simple yet detailed is always the preference but it better be solid,
idiomatic go." Measured consequence: the production port set is unchanged (the 28 roots already exclude
both aggregators, A1), and two components SemSource composes but that are outside the 65 become explicit decisions
rather than silent imports: `output/websocket` (composed at `run.go:1060`, no 03A assertion) and
`processor/graph-clustering` (composed only under `enable_clustering`, `run.go:932`). **Ruled (Q11):** both are
**defer-exclude** at tier 0 (D4); `websocket`, if SemSource's UI needs it, is a consumer-owned adapter.
Payload owners SemSource needs under (a): `message.RegisterPayloads`,
`storage/objectstore.RegisterPayloads`, plus `graph/inference.RegisterPayloads` only if `enable_hierarchy` is set
(construction already refuses otherwise, `component.go:735-741`). Not needed: `agentic`, `pkg/lifecycle`,
`gated-dag`, `governance`. A SemEngine contract test (T-B8, see Invariants) forbids any production package whose
import set spans more than one component family — the machine form of "no aggregator".

### D2. Relationship to ADR-106 (#2)

Options:

- **(a) Run alongside.** SemStreams proceeds to 1.0 under ADR-106 unchanged. SemEngine is a separate module whose
  exported surface is not ADR-106's Tier 1 by construction: 26 of the 65 are not Tier 1 and seven are `internal/`
  packages SemEngine must re-home (A7). SemSource and SemConnect leave the sister-import set when they migrate.
  Cost to SemStreams, not decided here: semsource is one of ADR-106's three canaries (ruling 6, `:90-98`); its
  departure removes one "active sister tracking a tag" from the RC-4 window.
- **(b) SemEngine supersedes.** The 39 overlapping Tier 1 packages leave SemStreams' frozen surface. Cost: a
  SemStreams-side ruling; the other seven sisters import those 39 and would be pointed at an unqualified SemEngine;
  it contradicts plan ruling 3 (frozen pin, no sync in either direction).
- **(c) Shared freeze.** SemEngine promises ADR-106 Tier 1 compatibility for the 39. Cost: SemEngine inherits a
  compatibility promise on types it is about to repair (#15–#17) and on a module path that differs; ADR-106 itself
  measures compatibility by `apidiff` against SemStreams tags.

**Ruled (Q2, #2): (a)**, recorded as: SemEngine is a fork at the pin of 39 Tier 1 and 26 non-Tier-1 packages; it
makes no compatibility promise to SemStreams' Tier 1; a SemStreams change after the pin is its own ledger row
(ruling 3). "Tier 1" here is SemStreams' *package* tier (`release/tier1-packages.txt`, the ADR-106 sister-import
surface), not a SemEngine capability level (D3). The mechanical check is adopted: a SemEngine boundary test that no
file imports `github.com/c360studio/semstreams` (I8). The canary question (Q2b) is moot; the owner's direction:
"semstreams gets retired if we do this right", so SemStreams' RC window is not a SemEngine constraint.

### D3. The word for a capability level (#4)

Candidates measured against existing spellings: **tier** collides with ADR-106 tiers (`106-…md:52-57`), SemSource
`configs/tiers`, and the plan's own crosswalk; **slice** is already "extraction slice" throughout the plan and
`semengine_slices` in the 03A JSON; **profile** collides with `payloadregistry.Registration.IndexingProfile`
(ADR-103 decision 2, a per-type floor) and matches the 03A evidence's `profile: bm25|neural`
(`pinned-results.json`), which is the thing being named.

The architect proposed "profile" ("capability profile 0/1/2"). The owner, on Q3: "semstreams was tiered because we
have graceful fallback. we need to keep that concept to support offline first and edge deployments with shit comms.
problem was we never standardized what the tier names should be or represent. profile is a good name but it loses
some of the semantics around fallback."

**Ruled (#4, [comment 5929716018](https://github.com/C360Studio/semengine/issues/4#issuecomment-5929716018)):** "the
word is "tier". Tier N = a capability level with named guarantees; a deployment runs at the highest tier whose
providers are available and degrades to tier N−1 when one is lost; every tier's fallback behavior is a
retained-contract-matrix row with a proving test; tier 0 needs no external provider (offline-first, edge). The
standard, not the word, was what SemStreams lacked. Milestones and epics #9–#11 keep their names. "Profile" stays
only as the 03A evidence's run label (`profile: bm25|neural`), which names a configuration, not a level."

**Glossary note.** In this change, "tier N" is a SemEngine capability level as ruled above. The crosswalk table in
the matrix carries SemSource's config file names verbatim (`tier0-statistical.json`, `tier1-semantic.json`), which
do not line up one-to-one with SemEngine tiers. "Tier 1" in D2 and A7 is SemStreams' *package* tier
(`release/tier1-packages.txt`), a different grammar; so is `payloadregistry.Registration.IndexingProfile` (a
registered type's floor). "profile" appears only as the quoted 03A run label (`profile: bm25|neural`) and for a
coverage profile (A13); `semengine_slices` in SemSource's JSON is not renamed.

### D4. Port set: admission or separation of each questioned package

| Package (lines) | Reach (inventory) | Options | Ruling (Q4, Q11) and measured consequence |
| --- | --- | --- | --- |
| `engine`, `flowstore` | absent at the pin (A1.1) | — | **No row**: resolved by the pin; the matrix records "retired upstream by #1116". |
| `agentic` (6,005) + `internal/looptoken` (42) | `component/dependencies.go:7,56-57` (`ToolRegistryReader`); `gateway/graph-gateway/component.go:40` | carry; adapt `component` | **Separate (adapt `component`)**: drop `Dependencies.ToolRegistry` and the `ToolRegistryReader` type (zero consumer in the retained workload; the only requirer is SemStreams' agentic-tools per the comment at `:76`). With D4's graph-gateway row this removes `agentic` and `internal/looptoken` from the closure. |
| `agentic/agentrun` (1,505) + `internal/deliverylane` (248) + `internal/agentterminal` (188) | `service/milestone_service.go:10`, constructed only by `internal/boot/run.go:573` | carry; adapt `service` | **Separate (adapt `service`)**: do not port `milestone_service.go`. SemSource's `service.RegisterAll` does not register it (`service/register.go:8-22`). |
| `pkg/rulepack` (143) | `service/rule_pack_bind.go:8`, callers only `internal/boot` | carry; adapt | **Separate (adapt `service`)**: do not port `rule_pack_bind.go`. |
| `vocabulary/agentic` (2,133) | `processor/graph-query/graphrag.go:1718-1720` (three label predicates) | carry; adapt | **Separate (adapt `graph-query`)**: drop the three agentic predicates from `labelPredicates`. Finding: entity display label has two homes — `fusion.Lens.Label` (consumer, `lens.go`) and `graphquery.labelPredicates` (framework, hard-coded). Consolidation is deferred; the agentic spellings go now. |
| `pkg/lifecycle` (3,739) | `component/dependencies.go:99`, `service/component_manager.go:49,199,996-1111`, `service/dependencies.go:36` | carry (dormant); separate | **Defer-exclude at tier 0 (adapt `component`, `service`)**: remove the `LifecycleManager` field and its Stop/Initialize calls; no retained consumer uses it (SemSource and SemConnect import it nowhere). Re-admission needs a consumer and its own row. |
| `composition` (914) + `component/flowgraph` (1,324) | `service/component_manager.go:402` boot path | carry | **Admit (carry)**: ADR-100 boot validation; the home for #16's refusal (D11). |
| `pkg/projection/contract` (163), `internal/lifecyclecleanup` (38) | ADR-103 and every processor's one-shot lifecycle | carry | **Admit (carry)**; `internal/*` re-homed under SemEngine's `internal/`. |
| `graph/clustering` (5,516), `graph/llm` (842), `pkg/graphview` (1,193), `model/wire` (1,182) | `processor/graph-query` GraphRAG/community/LLM answer (A1.2) | (i) carry dormant; (ii) adapt graph-query behind an optional-capability seam; (iii) defer `searchGraph` | **(ii) Adapt `processor/graph-query` and `graph/query`**: community and LLM-answer paths (and `graph/query`'s `classifier_llm_adapter.go:9`) move behind an interface whose tier-2 implementation lives in a separately admitted package, so tier 0/1 compile without the four. `searchGraph` keeps its already-declared degraded contract ("semantic fallback … reports requested unavailable community enrichment", `query.go:63`). Cost: the largest adapt (graph-query is 6,745 lines); 14,954 lines leave the tier-0 closure. **Ordered by Q4:** (i) first, then (ii): the four are carried dormant through the first green tier-0 extraction (the plan's dormant bridge, `setup-plan.md:412-414`, with its exit condition below), and the seam is the last task of 04A. |
| `graph/inference` (5,338) + `graph/structural` (883) | `processor/graph-ingest/component.go:19` (hierarchy, default off, construction-guarded `:735-741`); graph-gateway | same three | **(ii) Adapt `processor/graph-ingest`**: `initHierarchyInference` (`:1416-1442`) behind the same optional-capability seam. Hierarchy is not in the retained workload (`enable_hierarchy` unset by SemSource). |
| `gateway/graph-gateway` (2,676) + `gateway` (319) | composed by SemSource (`run.go:852`), not exercised (A3.3); requires `agentic_queries` with no responder; drags `agentic`, `graph/inference` | carry; adapt (drop the agentic port and trajectory decode — a port-contract break); defer-exclude | **Defer-exclude at tier 0**: layer 4 of D5 is consumer-owned (SemSource's `mcp-gateway`, SemConnect's `cs-api`). Re-admission requires a consumer assertion through its HTTP surface. Measured: 27 roots → 63 packages; `agentic` then remains only via the two file-level edits above. |
| `pkg/tlsutil` (533) + `pkg/acme` (543) | `metric/handler.go:20` | carry; adapt metric | **Carry** (metrics TLS; small). |
| `health`, `internal/logforwarderpolicy`, `internal/componentadmission` | `service` | carry | **Carry.** |
| `output/websocket`, `processor/graph-clustering` | outside the 65; composed by SemSource via the aggregator (A2.2) | admit; defer-exclude | **Defer-exclude at tier 0** (D1). |

Resulting tier-0 closure under the Q4 ruling: 65 − {`gateway`, `gateway/graph-gateway`, `agentic`,
`agentic/agentrun`, `internal/agentterminal`, `internal/deliverylane`, `internal/looptoken`, `pkg/rulepack`,
`vocabulary/agentic`, `pkg/lifecycle`, `graph/clustering`, `graph/llm`, `pkg/graphview`, `model/wire`,
`graph/inference`, `graph/structural`} = **49 packages, about 95,000 lines** (126,926 − 2,995 − 10,264 − 3,739 −
14,954 = 94,974; file-level adapts change package line counts slightly). This is the number the developer re-measures
after the adapts land; it is a target, not a measurement. Separation rows for the sixteen packages are `defer-exclude`
with "tier-2 or agentic consumer" as the re-admission condition.

**Ruled (Q4): all sixteen separations, ordered.** The agentic, `pkg/lifecycle`, and gateway cuts land first. The six
higher-tier graph libraries (`graph/clustering`, `graph/llm`, `pkg/graphview`, `model/wire`, `graph/inference`,
`graph/structural`; 14,954 lines) are carried dormant through the first green tier-0 extraction, so the 03A
attribution of that extraction is not muddied by a refactor made blind at the pin. The capability seam in
`processor/graph-query`, `graph/query`, and `processor/graph-ingest` is the **last task of 04A**, with exit condition
"tier 0 compiles without the six libraries". The dormant bridge is bounded to that one change, not deferred to 04B.

#### D4a. Port refactors are a tracked risk class

The owner, on Q4: "ensure we track this type of move as i think this is our big class of risk in this migration."
**Ruled:** restructuring ported code away from the pin's shape (a seam, a file-level adapt, a dropped field or
method) is the named risk class `class:port-refactor`. Every such move is:

- an admission-ledger row with disposition `adapt` whose `contract` field cites its retained-contract-matrix row and
  whose `proving_tests` field names the test that proves the retained behavior; and
- carried by an issue labeled `class:port-refactor`, so attribution against the 03A baseline stays queryable.

The ledger schema is closed: T-B7 (`internal/harness/contract/ledger_test.go`, `ledgerFields`) rejects any key
outside the ten fields, and `harness-boundaries` › "Admission ledger" states the same schema. The class is therefore
recorded in existing fields (matrix row in `contract`, test in `proving_tests`, `class:port-refactor` in
`known_risks`) and the ledger header documents that convention (task 2.5). A dedicated field would need a T-B7 code
change and a `harness-boundaries` spec delta; it is not part of this change. The label does not exist in the
repository at this change's base (`gh label list`, 2026-10-01); task 2.5 creates it.

Known port refactors at tier 0: the `component` adapt (drop `Dependencies.ToolRegistry`,
`ToolRegistryReader`, and the `LifecycleManager` field), the `service` adapts (`milestone_service.go`,
`rule_pack_bind.go`, `LifecycleManager` calls), the `graph-query` label-predicate drop, and the capability seam in
`processor/graph-query`, `graph/query`, and `processor/graph-ingest`. The repair-before-port rows (D11) also change
code, under their own disposition; whether their issues carry `class:port-refactor` as well is not ruled here.

### D5. Fusion and graph-tool boundary

The four layers, mapped to code at the pin:

1. **Graph substrate** (SemEngine): `graph`, `graph/readiness`, `graph/embedding`, `graph/query` (classifier; its
   `classifier_llm_adapter.go:9` import of `graph/llm` moves behind the D4 seam), the four processors,
   `storage/objectstore`, `storage/storeregistry`, `pkg/projection`, `internal/graphmutation`.
2. **Generic deterministic fusion** (SemEngine): `pkg/fusion` lens path — `Engine.Fuse`, facets, budget, hydrate,
   provenance, partial results (`engine_lens.go`, `engine_facets.go`, `engine_graph.go`, `hydrate.go`,
   `contract.go`), `fusionnats`, `fusionvocab`. **Keep.** The package-level `fusion.Fuse` / `SubQuery` /
   `GraphQueryClient` / `Evidence` path (`engine.go:96`, `subquery.go`, `client.go`, `evidence.go`) has zero
   consumer in SemSource or SemConnect and its named caller is `research-graph-execute` (`doc.go:7-8`): **defer
   (not ported)**.
3. **SemSource lenses and policy** (consumer): `source/fusion/lens/{code,docs}`, supersession, symbol policy. Not
   ported.
4. **Consumer-owned public adapters**: SemSource `mcp-gateway` and `code-context` HTTP; SemConnect `cs-api`.
   SemStreams' `graph-gateway` is not the SemEngine adapter (D4).

`graph.query.searchGraph`: **keep at tier 0/1 under its degraded contract** (exact or semantic fallback, community
enrichment reported unavailable); **change** by the D4 seam; the 03A observation `graph_search_public_query` is its
proving test at both tiers. `find/anchor/ask` as cases, not a mode API: *find* = `ResolveModeSymbol`/`Prefix` +
`Names` (`lens.go:86-94`, `retrieval.go`; observed `cold_duplicate_name_anchors`); *anchor* = `Entity`/`Entities`
by ID (hydration; observed `*_exact_relationship`, `*_exact_content`); *ask* = `ResolveModeNL` and `searchGraph`
(observed `neural_paraphrase`, `graph_search_public_query`). No new mode enters the contract. Known fusion defects:
SS#621 (open, `class:unobserved-skip`) and SS#603 (open, post-v1) are matrix rows marked "reproduce at the pin before
carrying as a current defect" per the plan; neither was reproduced in 03A.

### D6. Applied-input barrier (#18)

Options: (i) a barrier primitive owned by graph-ingest (per-producer applied census, incarnation-aware); (ii) a
per-producer applied census bucket; (iii) **durable applied facts on the owning entity** — the restart-safety L4a
precedent in the pin (SS PR #1361, `c4a79fd5`: agentic-loop recovers redelivered inputs from facts stamped on the
`LoopEntity` itself, SS#1147 hierarchy item 2, no new bucket), here a per-source applied marker on the source's own
entity; (iv) **defer** with the consumer consequence recorded. Evidence: no primitive
exists (A11 row 4); SS#1147's binding hierarchy admits additional component authority "only when a named failpoint
proves the first three insufficient" and lists a generic outcome/checkpoint bucket as an anti-goal; the consumer need
(semsource#215 retired-source completion) is not in the 03A retained workload (`source_removed` markers are absent
at both pins and recorded as pre-existing, `compatibility.md:244-251`). **Ruled (Q5, #18): (iv) defer**, with two
records: the consequence (SemSource has no supported completion signal for retired sources and keeps
`applied_tail_unproven` as a declared limit), and a design constraint on D11 #15: the stream-generation identity
the #15 repair introduces is the one home a later barrier would key on — it must be a named, readable fact, not a
private encoding, so #18 does not later add a second spelling of "which stream incarnation". The ruling names
(iii), durable applied facts on the owning entity (L4a), as the recorded precedent if a barrier is ever needed,
keyed on the generation fact from #15; the consequence is recorded on #18 and semsource#215.

### D7. Conditional reconcile at a caller-observed revision (#19)

Options: (i) add `ExpectedRevision uint64` to `projection.ReconcileMutation`, honoured by `MutationClient.Reconcile`
(zero keeps today's owner-read behavior); (ii) a separate `ReconcileAt` operation; (iii) defer. Evidence: two of two
consumers need the fence — SemConnect already sends `graph.ReconcilePredicatesRequest{ExpectedRevision:
exact.KVRevision}` over the raw subject (`graph_mutations.go:214-215`), the path `internal/graphmutation/protocol.go:2`
says applications should not take; SemSource cannot through the typed client (`mutation_client.go:191,199`). The wire
already carries the field (`graph/mutation_requests.go:29`); the owner already fences on it
(`canonical_mutations.go:349-355`). Against SS#1147's hierarchy this is item 2 (stable identity plus deterministic
reconciliation) with no new durable state: the fence is the revision the owner already keeps, so it is the cheapest
rung, not additional authority. The consumer design it serves is SemSource PR #213 (merged `3604a9ce`): consumer-private
source-lifecycle machinery that today reports `conditional_reconcile_unavailable` and leaves matching entities stale.
**Ruled (Q6, #19): (i)** — `ExpectedRevision` on `projection.ReconcileMutation`, surfaced through the typed client,
as the one typed home; SemConnect's raw `graph.mutation.>` send moves to the typed client as consumer work, recorded
on SemConnect's ledger row (closing the parallel raw-wire channel). The zero-value fallback is a predict-vs-observe
seam only if a caller forgets the field; the compensating rule is a `MutationError` kind `revision-conflict` that
names the observed and current revisions so a wrong fence is loud, never a silent re-read. Proving tests: read R /
concurrent write R+1 / reconcile at R → conflict, no clear; unchanged R → applied; both consumers' fixtures.

### D8. Commit ambiguity and unknown outcomes (#20)

Options: (i) carry `CommitUnknown` end to end plus a terminal operation-status lookup by request ID; (ii) an
owner-enforced generation fence; (iii) defer. Evidence narrows (i): the client already has `CommitUnknown` and
`unknownMutation` (`mutation_types.go:74`, `mutation_client.go:404-417`); the defect is one server conversion
(`canonical_mutations.go:360` → `rejectFromError` `mutation_runtime.go:218` → `rejectInternal` `:214-216`) that turns a
backend write error of uncertain effect into a classified error, and one client predicate (`isDefiniteFailure`,
`:419-422`) that treats every classified error as definite. **Ruled (Q7, #20): (i-narrow) only** — a repair-before-port
row spanning `processor/graph-ingest` and `pkg/projection`: the owner distinguishes rejected-before-effect (invalid,
conflict, not-found) from effect-uncertain (a new coded outcome, class transient-unknown), and the client maps only
the former to `CommitNotCommitted`. The repair has a consumer half: SemConnect bypasses `pkg/projection` and
classifies replies itself (`graph_mutations.go:113-117`; `systems_post.go:465-469` maps `ErrorCodeInternal` to a
500), so as a first-wave consumer (D9) it must map the new effect-uncertain code to its existing
`commitUnknownError` path — consumer work recorded on its row, not a framework task. **Defer** the
status-lookup-by-request-ID and the generation fence (a same-class collision with SS#1147's "generic outcome bucket"
anti-goal; the graph already offers the observable alternative: re-read the authoritative entity and compare).
Consumer consequence recorded honestly: **no consumer resolves an unknown outcome today** — SemConnect returns HTTP
503 "verify resource state before retrying" to its caller (`systems.go:1034-1042`) and SemSource (PR #213) keeps a
durable fence and refuses re-admission; after the repair both at least receive the truthful classification, and
resolution remains re-read plus the consumer's own fence. Proving test (ruled): an injected KV `Update` timeout in
the harness, and the caller observes `CommitUnknown`.

### D9. SemConnect's status for SemEngine

Options: **(a)** reference fixtures only, port set 65; **(b)** first-wave consumer, port set widened by
`graph/geo/geojson` (1,030) and `vocabulary/export` (1,107) → **67 packages / 129,063 lines** (A10), both leaves
(`export` imports only `message` and `vocabulary`); **(c)** SemConnect copies the two packages. Evidence: the plan
places "generic registration and export mechanisms" in the engine (`setup-plan.md:103`); `vocabulary/cco` and `bfo`
are already in; (c) duplicates 2,137 generic lines in a consumer. **Ruled (Q8): (b)**, SemConnect is a first-wave
(tier 0) consumer, with the ceiling re-measured to 67 / 129,063 and SemConnect's 03A before/after (once PR #74
records it) entering the matrix as a second observed column. The owner: "semconnect touches a few different parts we
should get right at t0." The seams it exercises are tier-0 matrix rows with harness proving tests: entity
identity and authority (ADR-102 d5 import-lane rule and read-only mirrors; ADR-104 unique `platform.id`), the typed
mutation client, projection, artifact/`StorageReference`, and generic vocabulary/export. The federated-create
expectation in semconnect#74 (`TestSetup03AFederatedDatastreamCreate`) is SemConnect's pre-ADR-102 contract; its
resolution (import lane as mirror, or mint locally and cite via `@id`) is SemConnect work, not an engine change.
Its outcome is now recorded on semconnect#74 (PR body, head `dff12657`, 2026-10-01): HTTP 201 at beta.160 and
HTTP 400 `authority_foreign` at the pin, the d5 rejection this matrix keeps.
SemConnect's cutover is not an MVP acceptance gate (the plan's acceptance is SemSource); its composition root is
SemConnect-owned work PR #74 already lists.

### D10. Critical package list for the 80% coverage gate

Selection rule: packages that hold an admission gate (integrity, silent loss, context ownership, durability,
authority/readiness, metadata/content preservation). **Ruled list (Q9):** `processor/graph-ingest`,
`processor/graph-index`, `processor/graph-embedding`, `processor/graph-query`, `graph`, `graph/readiness`,
`graph/embedding`, `pkg/projection`, `internal/graphmutation`, `pkg/fusion`, `pkg/fusion/fusionnats`, `natsclient`,
`service`, `config`, `composition`, `component`, `storage/objectstore`, `storage/storeregistry`, `message`,
`payloadregistry`. Not gated at 80% (tested, not critical): `pkg/errs`, `pkg/types`, `types`, `metric`, `health`,
`model`, the vocabularies, `pkg/{buffer,cache,retry,dispatch,worker,resource,revlag,timestamp,security,platform,
tlsutil,acme}`, `fusionvocab`. Baseline coverage at the pin is **not established** (A13); the gate is new (plan
`:227-229`), so the first measurement is taken when each package lands and recorded on its PR. The ruling asks to
confirm that the package enforcing ADR-102 d5 is on the list; the inventory does not yet name that package, so the
confirmation is task 2.4's outcome, not a claim made here.

### D11. Repair-before-port rows and the tier-0 (#9) admission gate

| Row | Package(s) | Matrix row | Repair shape (design constraint, not implementation) | Gate evidence before admission |
| --- | --- | --- | --- | --- |
| #15 (SS#1442) | `processor/graph-ingest` | acknowledged writes → recovery after transport recreation | the applied-sequence guard key carries a stream-generation identity; equal generation keeps `seq <= last`; a new generation never suppresses current source state | failing-first real-NATS test: retained guard, recreated stream with lower sequences, re-ingest applies; the two 03A failures turn green on SemEngine |
| #16 (SS#1143) | `graph` (declaration), `processor/graph-ingest`, `processor/graph-query`, `composition` | RPC plane / stream-filter safety | one reserved-subject declaration (one home for the 4 port-set literal sites, A4); `composition.Analyze` refuses any stream whose subjects cover a reserved request subject; error names stream, filter, subject | failing-first real-NATS PubAck collision with an unsafe composition; refusal before readiness; SemSource's explicit list starts and answers |
| #17 (SS#1443) | `config` | config crosswalk → desired-state deletion | `syncFromKV` applies component deletion symmetrically with its Services reset (tombstone or reset-and-overlay); a deleted file-declared component does not boot | SemSource's reproduction ported into `internal/harness`; remove → restart → absent; re-add → live |
| #20 (SS#1446) | `processor/graph-ingest`, `pkg/projection` | acknowledged writes → commit classification | D8 | unit + real-NATS: injected KV timeout → `CommitUnknown`; conflict/not-found → `CommitNotCommitted` |
| #19 (SS#1445) | `pkg/projection` | update semantics → conditional reconcile | D7 | the two D7 cases |
| SS#1411 | `component` (ruled at SETUP 02) | component/service seams → owner lifecycle | one owner-lifecycle guard composed by the 7 port-set copies (A5) | lifecycle-suite over each ported component |
| SS#1415 | `config` | component/service seams → Stop authority | `Manager.Stop(ctx)` | lifecycle-suite; no timeout parameter remains |
| SS#1218 | `service`, `pkg/errs` | component/service seams → shutdown sentinel | one `ErrAlreadyStopped` | property/unit |
| SS#1220 | `service` | component/service seams → registry after failed start | spec statement of the mode-independent clear (option 1 or 2 of the issue) | spec scenario |
| SS#1417 | ported tests in the 8 packages with 105 entries (68 in the tier-0 set, binding per Q12) | test debt | ported tests must pass `scripts/cleanup-roots-check.sh` (already a `task verify` step) | the guard |
| SS#1145/#1147 | all | restart behavior | recorded as the lifecycle-suite floor plus per-package restart promises; no new recovery subsystem | lifecycle-suite `Promise{Restart}` per component |

Each is a `repair-before-port` ledger row; none is waivable by budget or coverage. **Ruled (Q12):** the mapping is
approved. The tier-0 gate (#9) is: every row above whose package is in the tier-0 set has its gate evidence
green on SemEngine before the package is admitted. The binding figures are the tier-0 ones: 68 cleanup-baseline
entries and 6 `lifecycleUsed` copies (105 and 7 across the full 65; `gateway/graph-gateway`'s 32 entries leave with
it).

**Ruled (Q13):** SemStreams PR #1437 (open, after the pin) is a ledger-row candidate for its `processor/graph-ingest`
half only, taken up when the graph-ingest repair rows are designed; the row cites the PR head SHA. Its
`agentic/loop_execution_entity.go` half is separated by Q4.

## Retained-contract matrix skeleton

Columns are fixed so a second (or third) consumer adds a column, never a restructure: **Area · Behavior · Observed
(SemSource 03A @ pin) · Observed (SemConnect @ pin, semconnect#74) · Intended SemEngine · Keep/Change/Defer · Owner ·
Proving test**. Rows are keyed by area and behavior; "Observed" cells cite an observation name from
`pinned-results.json` (A9) or "not observed". SemConnect cells still marked `pending #74` are filled from
semconnect#74's recorded qualification evidence (head `dff12657`) by task 3.3.

| Area | Behavior | Observed (SemSource) | Observed (SemConnect) | Intended SemEngine | K/C/D | Owner | Proving test |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Tiers | 0 foundation = structural ops under `tier0-statistical.json`, embedding still composed | slices [0,1] in bm25 run | pending #74 | true no-embedder composition exists and is admitted | Change | architect | 04A no-embedder boot + structural observations |
| Tiers | 1 lexical = BM25 under `tier0-statistical.json` | bm25 61/63 | n/a | same, scope-before-limit retained | Keep | SemSource owner | `{cold,warm}_*_scope_before_limit`, `*_duplicate_name_anchors` |
| Tiers | 2 neural = semembed under `tier1-semantic.json` | neural 65/67; provider identity held | n/a | same; identity pinned per `pins.json` fields | Keep | SemSource owner | `neural_paraphrase`, `provider_metrics_*`, fault supplement 6/6 |
| Tiers | fallback ladder (#4): run at the highest tier whose providers are available; lose one, degrade to tier N−1; tier 0 needs no external provider | not observed | SemConnect has no tiers | each tier names its guarantees, and its fallback is its own row of this matrix with a proving test | Change | architect | per-tier fallback test, added by the change that admits each tier above 0 |
| Crosswalk | SemEngine tier N ↔ SemSource config file | `compatibility.md:21-30` table | SemConnect has no tiers | table carried verbatim | Keep | writer | doc check |
| Identity | ADR-102 segment grammar; ADR-104 minted platform authority | `governed_identity_authority`, `*_persisted_authority` | pending #74 (`entityid.Authority` bounds) | Keep | Keep | architect | same observations on SemEngine |
| Identity | ADR-102 d5: every graph boundary enforces the deployment's own authority; a foreign `org.platform` enters only on an operator-declared import lane, as a read-only mirror (O-12) (Q8) | not observed | `TestSetup03AFederatedDatastreamCreate` expects a local create under a foreign authority (pre-ADR-102 contract); #74 records HTTP 201 at beta.160, HTTP 400 `authority_foreign` at the pin | foreign create rejected with the coded error; import-lane mirror accepted; `@id` citation of a foreign identity allowed | Keep | architect | harness: those three cases (04A identity change adds them) |
| Identity | ADR-104: minted `platform.id` is unique (Q8) | not observed | pending #74 | Keep | Keep | architect | harness test of the minted `platform.id` (04A identity change adds it) |
| Graphable / vocabulary | typed vocabulary (ADR-107), CCO/BFO, export edge | datatype changes attributed | `vocabulary/export` JSON-LD, `csapi` registration | `vocabulary/export` admitted (D9) | Keep | architect | SemConnect `register_test.go`; export round-trip |
| Statement metadata | source, time, confidence, correlation preserved through ingest/mutation/storage/query | `*_provenance` | pending #74 | Keep | Keep | architect | provenance observations; a metadata round-trip property |
| Content | exact bytes via `StorageReference`; unresolved instance excludes body, never degrades | `*_exact_content`, `*_doc_exact_passage_content` | immutable artifact bytes + `StorageReference` | Keep | Keep | architect | content observations; SemConnect `schema_artifacts_test.go` |
| Mutation | typed create/reconcile/delete under revision fence | `edited_*`, `recreated_*` | root-only create/reconcile/delete with exact revisions, absent target | Keep + D7 | Change | architect | SemConnect `beta160_structural_integration_test.go`; D7 cases |
| Mutation | conditional reconcile at caller-observed revision (#19) | not available via typed client | raw-wire today | typed `ExpectedRevision` | Change | owner #19 | D7 |
| Mutation | commit ambiguity preserved (#20) | not induced | own classifier; `ErrorCodeInternal` → not-committed | `CommitUnknown` end to end; lookup deferred | Change | owner #20 | D8 |
| Deletion | watched-file deletion retains stale history; recreation restores identity | `deleted_*`, `recreated_same_identity_not_stale` | n/a | Keep | Keep | SemSource owner | same |
| Deletion | desired-source removal: file-declared component stays out of next boot (#17) | removal supplement 9/10; workaround | n/a | framework-correct | Change | owner #17 | D11 #17 |
| Deletion | retired-source completion signal (#18) | `source_removed` markers absent, pre-existing | n/a | deferred; consequence recorded | Defer | owner #18 | consumer's `applied_tail_unproven` stays a declared limit |
| Deletion | physical purge (semsource#210) | not in workload | n/a | not admitted | Defer | SemSource owner | — |
| Query | exact entity, byName, prefix, status | `initial_*`, `cold/warm_exact_absence` | spatial queries (`graph/geo/geojson`) | Keep | Keep | architect | same; SemConnect spatial case |
| Query | `searchGraph` degraded contract | `graph_search_public_query` | n/a | keep; community/LLM behind seam (D4) | Change | architect | same observation at tiers 0/1/2 |
| Fusion | `Engine.Fuse` lens path: resolve, expand, hydrate, rank, budget, provenance, partial results | fusion HTTP observations | n/a | Keep | Keep | architect | 03A fusion observations; #621 reproduce-first row |
| Fusion | package-level `fusion.Fuse`/SubQuery | no consumer | no consumer | not ported | Defer | architect | T-B8 import guard |
| Fusion | impact facet names (SS#603) | not observed | n/a | reproduce first | Defer | architect | — |
| Readiness | honest readiness; miss is absence only when ready (ADR-066/084) | `ingestion_ready`; fault supplement "error is not absence" | `index-readiness` reader | Keep | Keep | architect | same |
| Durability | memory GRAPH; PubAck is not durability | `memory_transport_not_durable`, `accepted_unindexed_before_broker_restart` | pending #74 (restart workload) | recorded as the contract: acknowledged = accepted by transport; durable = KV authority + content; recovery = re-ingestion from source | Keep (as stated) | owner | same observations |
| Durability | recovery after stream recreation (#15) | **fails** ×2 at both pins | pending | repaired | Change | owner #15 | D11 #15 |
| Durability | capacity: OSH 32,720 under 256 MiB; historical 77,802 open | guarded pass | n/a | tier-gated workload evidence, not transport success | Defer | SemSource owner | capacity probe at the admitted tier |
| Transport | reserved RPC subjects vs stream filters (#16) | `rpc_wildcard_collision_reproduced` | pending | refusal at boot | Change | owner #16 | D11 #16 |
| Component/service seams | declared ports, typed ops, one-shot lifecycle, failed-Start cleanup (ADR-094/095/096/100) | application restart observations; join tests | pending | Keep; `Dependencies` narrowed (D4) | Change | architect | lifecycle-suite per component |
| Component/service seams | `config.Manager.Stop(timeout)` (SS#1415) | n/a | n/a | `Stop(ctx)` | Change | owner | D11 |
| Component/service seams | two `ErrAlreadyStopped` (SS#1218) | n/a | n/a | one | Change | owner | D11 |
| Registration | aggregator imports | both imported | none in production | explicit per-package (D1) | Change | owner #3 | T-B8; consumer boots |
| Provider/model identity | image, build, model, artifact, ONNX, dims, prefixes held constant | `pins.json` | n/a | same fields required per tier-2 run | Keep | SemSource owner | pins check in the qualification runner |
| Context/stop/join | contexts enter as arguments; no retained context; bounded terminal cleanup | 0 retained fields; 34 roots (A8) | — | each root triaged in the ledger | Keep/Change per root | developer | `TestNoRetainedContext`, cleanup-roots guard |
| Level name | "tier" vs "profile" | 03A uses `profile` for runs (a configuration label) | — | "tier" (ruled on #4, D3); `profile` stays only as the 03A run label | Keep | owner #4 | doc check |
| Boundary | no SemEngine file imports `github.com/c360studio/semstreams` (Q2) | n/a | n/a | fork at the pin; no Tier-1 compatibility promise | Keep | architect | SemEngine boundary test (I8) |

## Premises (each with its measurement)

1. The port set at the pin is 65 / 126,926 — A1, `go list` + line count, identical to Codex.
2. `engine` and `flowstore` do not exist at the pin — A1.1, `ls` and `git log --diff-filter=D`.
3. `agentic` is reached only through `component/dependencies.go:7` and `gateway/graph-gateway/component.go:40` (and
   `agentrun` only through `service/milestone_service.go:10`) — A1.2 reverse import graph; dropping graph-gateway as
   a root alone leaves 63 (inventory measurement in D4 row).
4. SemSource's retained workload does not exercise graph-gateway — A3.3, `grep gateway test/setup03a` → one config line.
5. The two Fuse entry points are distinct and only the lens path has a consumer — A3.1.
6. SemConnect already sends `ExpectedRevision` by raw wire — A6 #19, `graph_mutations.go:214-215`.
7. `CommitUnknown` exists at the pin and is defeated by the server-side conversion — A6 #20.
8. SemConnect's delta is two leaf packages — A10.
9. No coverage baseline exists at the pin — A13.

## Invariants and their spec homes (new spec deltas this change must add)

- **I1 (registration):** no SemEngine production package imports more than one component family's factories; the
  consumer's composition root is the only aggregator. Home: `harness-boundaries` spec, new requirement; guard T-B8.
- **I2 (reserved subjects):** for every stream config admitted at boot, no subject filter covers a reserved
  request subject; violation is a boot refusal naming stream, filter, subject. Home: new capability spec
  `graph-transport-boundary` (#16).
- **I3 (generation-aware replay guard):** within one stream generation, a redelivered sequence ≤ last applied is
  stale; across generations, the comparison never suppresses current source state. Home: new capability spec
  `graph-ingest-recovery` (#15).
- **I4 (deletion durability):** a component deleted from desired state is absent from the next boot's concrete set
  regardless of its declaring source. Home: new capability spec `config-desired-state` (#17).
- **I5 (commit classification):** `CommitNotCommitted` is returned only for rejections proven before any storage
  effect; every other failure is `CommitUnknown`. Home: new capability spec `projection-mutation` (#20).
- **I6 (conditional reconcile):** a reconcile carrying revision R applies only if R is current; otherwise it returns
  `revision-conflict` with both revisions and makes no change. Home: `projection-mutation` (#19).
- **I7 (acknowledged-write contract):** accepted ≠ durable; durable = KV authority + content; recovery = re-ingestion.
  Home: `graph-ingest-recovery`.
- **I8 (fork boundary, Q2):** no SemEngine file imports `github.com/c360studio/semstreams` or any path under it; this
  is ADR-106's sister-import freeze enforced from SemEngine's side. Home: `harness-boundaries` spec, the same
  modified requirement as I1; guard: a SemEngine boundary test (task 2.6).

## Adopter seam findings (the gaps are the design work)

From A12: seams 3 (stream subjects), 4 (projection client), and 5 (desired-config removal) are at "found out
nowhere" or "next boot" today; D11 #16, D7/D8, and D11 #17 are their fixes, each converting a predicted value into an
observed one (boot refusal; owner-side fence and classification; owner-side tombstone). Seam 1 (registration) moves
from compile-time-silent to compile-time-explicit under D1. Seam 2 (Lens SPI) and seam 6 (durability) stay as
documented contracts with their invariants in spec (I7).

## Open questions

- Unruled, from the step-back review on #8
  ([comment 5929756463](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929756463)): **Q14** (are
  rules and business workflows engine-owned capabilities, at which tier, qualified by what workload) holds tasks 2.1
  and 2.3; **Q15** (SemConnect's delta re-measured at the semconnect#74 head) holds tasks 1.7 and 2.1.

## Declared costs

- D4(ii) is a refactor of two 5–7K-line processors to introduce an optional-capability seam; it is the single
  largest adapt. Q4 orders it last in 04A, so the first green tier-0 extraction carries 14,954 dormant lines
  through lint, vuln, and coverage gates until the seam's exit condition holds.
- Port refactors (D4a) are the owner's named largest risk class. Their tracking rests on existing ledger fields and
  an issue label, not on a machine-checked ledger field; a reviewer, not T-B7, catches a missing matrix citation.
- SemConnect's second matrix column is partly filled; the cells marked `pending #74` wait on task 3.3, which reads
  semconnect#74's recorded qualification evidence. That PR is a draft whose qualification is itself open on the
  federated-create assertion, so its evidence can still change.
- The tier-0 closure (~49 / ~95K) is a target computed from file-level adapts and is re-measured by the developer.
- Coverage baseline is unknown; the 80% gate may fail on first port of a critical package and that is a finding, not
  a reason to drop the package from the list.
- SemStreams PR #1437 (open, after the pin) changes two port-set packages, one of them `processor/graph-ingest`,
  which already carries three repair rows; under ruling 3 it enters only as its own ledger row with source commit and
  proving test. Q13 ruled it a candidate for the graph-ingest half only (D11).

## Spec deltas drafted for the SETUP 04A changes

This change sets `skip_specs: true`: `openspec/specs/` is current truth verified against code, and no code lands here.
The 04A extraction change for each package carries the delta below as its `specs/<capability>/spec.md`.

### graph-transport-boundary (new; #16)

```markdown
## ADDED Requirements

### Requirement: Reserved request subjects have one declaration
The graph request/reply subject families served by graph-ingest, graph-index, graph-embedding, and graph-query
SHALL be declared once, in the `graph` package, and every server and client SHALL reference that declaration.

#### Scenario: A literal spelling is added
- **WHEN** a non-test file outside the declaration spells a reserved subject as a string literal
- **THEN** the contract test fails naming the file

### Requirement: Stream filters never cover a reserved subject
Composition analysis SHALL refuse to start when any configured stream's subject filter covers a reserved request
subject, with an error naming the stream, the filter, and the subject.

#### Scenario: Wildcard ingest stream
- **WHEN** a stream declares `graph.ingest.>` and graph-ingest serves `graph.ingest.query.entity`
- **THEN** startup fails before readiness with the stream, filter, and subject in the error

#### Scenario: Explicit subjects
- **WHEN** a stream declares exactly `graph.ingest.entity`, `graph.ingest.batch`, `graph.ingest.manifest`,
  `graph.ingest.status`, `graph.ingest.predicates`
- **THEN** startup succeeds and entity, batch, prefix, and suffix queries return correct answers
```

### graph-ingest-recovery (new; #15 and the acknowledged-write contract)

```markdown
## ADDED Requirements

### Requirement: Acknowledged is not durable
A transport acknowledgement SHALL mean accepted by the transport; durability SHALL mean the entity authority and
content are committed to their stores; recovery after transport loss SHALL be re-ingestion from the current source.

#### Scenario: Memory transport lost at broker restart
- **WHEN** an accepted but unindexed message is lost with a memory stream at broker restart
- **THEN** the authority and content committed before the restart survive, and the lost message is not reported as
  applied

### Requirement: Replay protection is generation-aware
The applied-sequence guard SHALL key on the stream generation; within one generation a sequence not newer than the
last applied SHALL be stale; across generations the guard SHALL never suppress current source state.

#### Scenario: Stream recreated with lower sequences
- **WHEN** the stream is recreated and re-ingestion publishes the current source at a sequence lower than the
  retained applied sequence
- **THEN** the current relationship and content are applied and queries return them

#### Scenario: Redelivery within one generation
- **WHEN** a message already applied in the current generation is redelivered
- **THEN** it is acknowledged without changing current state
```

### config-desired-state (new; #17)

```markdown
## ADDED Requirements

### Requirement: Deleted components stay deleted across boot
A component removed from desired state SHALL be absent from the next boot's concrete component set regardless of
whether the file configuration declared it.

#### Scenario: File-declared component deleted then restart
- **WHEN** a file-declared component is deleted through the desired-state API and the process restarts with the same
  file and retained bucket
- **THEN** the component is not created and not started

#### Scenario: Re-add after deletion
- **WHEN** the same component name is written to desired state again and the process restarts
- **THEN** the component is created and started
```

### projection-mutation (new; #19, #20)

```markdown
## ADDED Requirements

### Requirement: Conditional reconcile at a caller-observed revision
`ReconcileMutation` SHALL accept an expected revision; when set, the owner SHALL apply the mutation only if that
revision is current and otherwise return a revision-conflict error naming the expected and current revisions and
making no change.

#### Scenario: Concurrent update between read and reconcile
- **WHEN** the caller read revision R, another writer committed R+1, and the caller reconciles at R
- **THEN** the result is revision-conflict and the entity is unchanged at R+1

#### Scenario: Unchanged revision
- **WHEN** the caller reconciles at the current revision R
- **THEN** the mutation is applied and the receipt reports verified with the new revision

### Requirement: Commit ambiguity is preserved
The owner SHALL classify a failure as not-committed only when it is proven to precede any storage effect (invalid
request, revision conflict, entity not found); every other failure SHALL be reported as commit-unknown through the
canonical server and the public client.

#### Scenario: Backend write timeout
- **WHEN** the authority store update times out or its acknowledgement is lost
- **THEN** the receipt reports commit-unknown, never not-committed

#### Scenario: Revision conflict
- **WHEN** the authority store rejects the update for a revision mismatch
- **THEN** the receipt reports not-committed with the conflict classification
```

### harness-boundaries (modified; T-B8)

```markdown
## MODIFIED Requirements

### Requirement: Import graph
No non-test Go file outside `internal/harness/` SHALL import `internal/harness/...`, `testcontainers-go`, `testing`,
or `gopkg.in/yaml.v3`. No production package SHALL import the component factories of more than one component family;
the consumer's composition root is the only place factories and payload registrations are aggregated. No Go file in
the module SHALL import `github.com/c360studio/semstreams` or any path under it.

#### Scenario: Production package imports the fixture
- **WHEN** a non-test file outside internal/harness imports internal/harness/natsfixture
- **THEN** the contract test fails naming the file

#### Scenario: An aggregator package appears
- **WHEN** a production package imports `Register` from two or more component packages
- **THEN** the contract test fails naming the package

#### Scenario: A file imports SemStreams
- **WHEN** any Go file in the module, test or non-test, imports `github.com/c360studio/semstreams/...`
- **THEN** the contract test fails naming the file and the import
```
