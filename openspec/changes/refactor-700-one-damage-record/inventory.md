# Inventory: refactor-700-one-damage-record

base: c6377db9e472458fd1a0175aff98b5baf04f6763 (main 7cd05b86 + empty claim commit; no code change)

Question: which per-entity damage records exist, how each records, clears, samples and is read, and which can share
one type (#700; owner decisions 2026-11-20, comment 7030083210). Repositories read: this one; consumer working trees
semsource, semconnect, semteams, semboids (grep only, no git command run in them). Readers were found with
`gopls references`; string literals with `git grep -n`.

A *damaged entity* is an `ENTITY_STATES` value `graph.UnmarshalEntityState` refuses
(`graph/entity_predicate_contract.go:287-303`). graph-ingest calls the same thing *poison*.

## 1. The claimed gap

The issue says three records exist, one per site. Measured:

- graph-ingest: `processor/graph-ingest/poison_inventory.go:52` — `type entityPoisonRecord struct {` (fields
  `contractErr *graph.StateContractError`, `revision uint64`); held at `component.go:123-125` — `entityPoisonMu
  sync.Mutex` / `entityPoison map[string]entityPoisonRecord` / `entityPoisonSize atomic.Int64`.
- follower: `graph/follower/damaged.go:23-33` — `type damagedEntry struct {` (`revision uint64`, `refusal error`) /
  `type damagedRecord struct {` (`mu sync.Mutex`, `entries map[string]damagedEntry`); held at `follower.go:116` —
  `damaged damagedRecord`.
- graph-clustering's maintainer: `graph/clustering/maintainer.go:60` — `refused map[string]struct{}`.
- **The premise "graph-clustering keeps its own damage record" does not hold for stored damage.** The component
  decodes first and reports to the follower: `processor/graph-clustering/component.go:281-282` — `if refusal :=
  graph.UnmarshalEntityState(e.Key, e.Value, &state); refusal != nil {` / `if f.ReportDamaged(e.Key, e.Revision,
  refusal) {`, then applies a delete (`:287`). The maintainer's set holds what `newContribution` refuses
  (`maintainer.go:423-431`: nil state, `types.ParseEntityID` failure, `state.ID != entityID`). `UnmarshalEntityState`
  already refuses each before `Upsert` (`entity_predicate_contract.go:140` — `if err :=
  semtypes.ValidateEntityID(entity.ID); err != nil {`, `:296` — `if entity.ID != key {`; `ParseEntityID` is
  `ValidateEntityID` then a split, `pkg/types/entity_id.go:132-136`). The one production caller of `NewMaintainer` is
  `processor/graph-clustering/component.go:217` (`git grep -n 'NewMaintainer(' -- '*.go' ':!*_test.go'`). So in the
  composed component `refused` stays empty.

## 2. Every current spelling of the fact

| | graph-ingest inventory | follower record | maintainer `refused` | `pkg/graphview` poisoned |
|---|---|---|---|---|
| Key, fields | entity ID; typed refusal + revision | key; `error` refusal + revision | ID only; revision in `members` | key; `*PoisonError{Key, Revision, Err}` |
| Record | `:97` — `if revision >= existing.revision {` replaces; new = absent before (`:95-99`) | `damaged.go:55` — `if recorded && revision < current.revision {` keeps; new = absent before (`:62`) | `maintainer.go:144` — `m.refused[entityID] = struct{}{}` on an applied refused Upsert | `view.go:650` — `v.poisoned[d.Key] = p` |
| Nil refusal | skipped, `:85-87` — `return false // unstampable` | filled in, fail closed, `damaged.go:45-50` | n/a | n/a |
| Clear | `:182` — `if rec.revision > proofRev {` keeps, so a proof at or above clears; paths: delete `entity_writes.go:475`, commit `:484`, valid read `mutation_runtime.go:163`, re-check `poison_inventory.go:146-169` | `damaged.go:70` — `recorded && revision > current.revision` clears; paths: delete delivered `follower.go:299`, `Complete` `:350` | valid applied Upsert `:146`, applied Delete `:170` | any later put or delete, watch order (`view.go:640`, `:643`) |
| Sample cap | `:43` — `const entityPoisonSampleCap = 10`; log cap `:48` — `const bootSweepErrorLogCap = 100` | `damaged.go:16` — `const damagedSample = 10` | `:34` — `const maxNamedRefusals = 10` | none |
| Lock | own mutex + size counter mirror (`:103`, `:187`) | own mutex | `m.mu`, shared with members, groups, words | the view's `mu` |
| Wording | `:249-250`: "`graph_state_reset_required`: N poisoned entities (per-entity refusal; repair = delete + recreate)"; "+N more; enumerate via debug status" | `damaged.go:122-126`: "N damaged entities: … and N more; write a valid value for each key or delete it" | `IncompleteError.Error`, `maintainer.go:36-43` | `PoisonError.Error`, `errors.go:50-51` |

Records of other facts, not this one: graph-embedding `failedMap` (`processor/graph-embedding/failed.go:35-38`, write
failures), graph-lexical and graph-index `failed()` (write failures), `laneGuard.refused`
(`processor/graph-ingest/keyed_ingest.go:106`, ingest lanes), graph-ingest's boot sweep map (`poison_inventory.go:315`
— `sweep := make(map[string]entityPoisonRecord)`, last revision wins, scratch before recording). Search:
`git grep -n -E 'map\[string\]' -- '*.go' ':!*_test.go' | grep -i -E 'poison|damag|refus|unreadable|failed|corrupt'`.

## 3. Readers and writers (gopls references, production code)

- graph-ingest. `recordEntityPoison`: `poison_inventory.go:131`, `:436`. `inventoryEntityPoison`:
  `entity_writes.go:347`, `mutation_runtime.go:159`, `poison_inventory.go:483`. `verifyEntityPoisonRecord`:
  `entity_writes.go:499`, `poison_inventory.go:133`, `:444`. `clearEntityPoisonOnDelete` `entity_writes.go:475`;
  `OnCommit` `:484`; `OnValidRead` `mutation_runtime.go:163`. `entityPoisonHealthMessage`: `component.go:377`
  (behind `:376` — `if c.entityPoisonSize.Load() > 0 {`). `DebugStatus`: `service/component_manager_http.go:513` —
  `status["debug"] = debugProvider.DebugStatus()`. Gauge: `poison_inventory.go:74` —
  `c.poisonedEntities.Set(float64(len(c.entityPoison)))`, from `:104`, `:188`; registered `metrics.go:332`. Cache:
  `:108` — `c.invalidateEntityCacheEntry(id)` on a new record. Size fast path: `entity_writes.go:498`,
  `poison_inventory.go:196`, `:207`, `:218`.
- follower. `ReportDamaged`: graph-clustering `component.go:282`, graph-embedding `hop1.go:201`, graph-index
  `index.go:266`, graph-lexical `component.go:242`. `DamagedCount`: `follower.go:471`, graph-embedding
  `query.go:94,126,165`, graph-index `query.go:99,113`, graph-lexical `query.go:99,121`. `DamagedRefusal`:
  graph-embedding `query.go:131`, graph-index `query.go:163,175`. `clearDamaged`: `follower.go:299,350`.
  `damagedSentence`: `follower.go:367`.
- maintainer. `refused`: `maintainer.go:108,144,146,170,256-258`. `*IncompleteError`: `maintainer.go:262`,
  `processor/graph-clustering/component.go:338` (the count becomes `refused_entity`, `component.go:311`).

## 4. Tests

- follower: `damaged_test.go` — 8 tests (`TestDamagedEntityClearsOnHigherCompletion`, `…ClearsOnDelete`,
  `…CountsOneError`, `TestDamagedCountAndRefusal`, `TestStuckNamesDamagedEntities`, `TestEnvelopeCountsDamagedEntities`,
  `TestDamagedEntityNeverWithholdsQuery`, `TestPropDamagedSetFollowsLatestEvent`), public methods only
  (`git grep -n -F '.damaged.' -- graph/follower`: none in tests).
- graph-ingest: `poison_scoping_test.go` (16 tests from `:71` to `:817`), `query_contract_guard_test.go`
  (`TestBootSweep*` `:41-201`, gauge reads `:69,141,190,226,239`, Health text `:83` — `"1 poisoned entity"`),
  `poison_scoping_integration_test.go:95`. **Private-state reads:** `component_fixture_test.go:237-239` (helper
  `poisonInventoryEntry`, 38 call sites, returns `entityPoisonRecord`); `entityPoisonSize.Load()` at
  `keyed_ingest_test.go:388`, `merge_entity_write_gate_test.go:61`, `poison_scoping_test.go:642,700,708,717`,
  `query_contract_guard_test.go:318,353,432`; `len(c.entityPoison)` at `query_prefix_ids_test.go:303-305`. Tests
  assign the gauge as a `prometheus.Gauge` (`poison_scoping_test.go:41`). `TestBootSweepLogCap` builds
  `map[string]entityPoisonRecord` (`:620-627`). No graph-ingest test checks the 10-ID Health sample
  (`git grep -n entityPoisonSampleCap -- 'processor/graph-ingest/*_test.go'`: none).
- maintainer: `maintainer_test.go:287` `TestRefusedEntityMakesTickIncomplete`, `:807-831` table,
  `maintainer_prop_test.go:401,522,577`.

## 5. Adjacent claims

- Specs: `entity-state-follower` "A damaged entity is recorded per key and cleared by its repair" and the four that
  follow it (`spec.md:220-305`); `graph-ingest-recovery` "graph-ingest's health reads its stuck states"
  (`spec.md:223`). The graph-ingest record/clear law has no requirement
  (`git grep -n -i 'inventor' -- openspec/specs`: no graph-ingest hit). `graph-clustering` `spec.md:83`
  (`refused_entity`).
- ADRs: ADR-108 (follower outcomes), ADR-109 Decision 2 (`:96-106`, both records are named stuck states), pin ADR-079
  (not carried; cited at ADR-109 `:102`, `:207`). It decides: decision 1, no inventory gates a decision; decision 3, a
  revision-stamped, self-healing inventory with degraded Health, one gauge, one ERROR per entity, and debug listing.
- Ledger: `processor/graph-ingest` row `:2509` (contract "poison accounting … as at the pin"); `graph/follower` row
  `:3623-3676` (contract names `ReportDamaged`, dependencies list `internal/resource, internal/revlag`);
  `graph/clustering` row `:4139`. No ledger check needs a row for a new internal package
  (`internal/harness/contract/ledger_test.go` checks the schema only; `internal/lifecycleguard` has no row).
- Open PRs (`gh pr list --state open`): #701 (this claim, no files); #695-#698 (Dependabot: `go.mod`, `go.sum`,
  `package*.json`). No overlap.
- Open issues: queries `follower`, `clustering`, `graph-clustering`, `graph-ingest`, `poison`, `damaged`. Naming a
  touched package: #657 (the damaged-read decision; its graph-ingest rows cite query refusals, not the record), #109
  (follower and shell; follower half delivered), #110 (readiness; names poisoned graph-ingest Health), #300
  (graph-ingest optional items, file split done), #502 (one poll in `poison_scoping_integration_test.go`). Names the
  word, not the package: #554, #602, #574, #571, #195, #108, #485, #635, #477, #141, #68, #621, #484, #196, #270, #630,
  #659, #111, #134, #570, #641, #350, #24, #9, #10.

## 6. Problem shape

A keyed, revision-guarded set of refusals with a sorted, capped sample: record if not older, clear by a later proof.
Closest instances: the two records above. No shared one exists (`gopls workspace_symbol -matcher=fuzzy Damage`:
only follower and plug-in symbols). Precedent for an internal package shared by the follower and others:
`internal/revlag` (`graph/follower/follower.go:46`).

## Adopter seam inventory

Reached from outside: follower's `ReportDamaged`, `DamagedCount`, `DamagedRefusal`, `Stuck` (plug-in authors);
graph-ingest's Health message, the gauge `semengine_graph_ingest_poisoned_entities`, and the `DebugStatus` JSON
(`poisoned_entity_count`, `poisoned_entities[].entity_id|reason|revision|error`) served by the component manager.
Consumer reads: semsource, semconnect, semteams none (`git grep -n -i 'poisoned_entit\|DebugStatus\|ReportDamaged\|
DamagedCount\|IncompleteError\|NewMaintainer'`); semboids reads the SemStreams-named gauge in
`docs/migration/shared-pin/performance/summaries/*.json`. What must an adopter know: nothing new if those surfaces
stay as they are; doing nothing changes nothing for them; the gap between items 1 and 4 is empty.
