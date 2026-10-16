# Proposal: simplify-04a-13-graph

## Why

This is PR 8c of epic #223 (owner, 2026-10-16, #223 comment 6216623583: "**8c — `graph`**: #132, plus #276 (export
graph-ingest's stored-value check; lifecycle's Watch adopts it)").

The `graph` root spells several facts twice. The rule for a stored entity is spelled five times, and the lifecycle
watch applies only part of it, so it can deliver an entity that graph-ingest refuses (#276). A graph-state refusal is
wrapped in two or three classified layers. Mutation failures have two vocabularies (#132 item 1, ruling L). The
revision key is spelled `kvRevision` on one reply and `kv_revision` everywhere else. The verb table carries strings
nothing reads. The exact reader is an interface with one implementation. Seven package comments compete in `go doc`.

## What Changes

- **One stored-value rule.** `UnmarshalEntityState(key, data, &entity)` refuses what graph-ingest refuses: a value that
  does not decode, a noncanonical value, a value naming another entity, or a statement with no `Source` or no
  `Timestamp`. The refusal carries one classification. graph-ingest, the lifecycle watch, the lifecycle test doubles
  and the exact reader all call it. `ValidateStatementMetadata` is the write lanes' half of the rule.
  `ClassifyStateContractError` goes, and graph-ingest's mutation replies stop adding a second classification. The
  write seam keeps its trusted read (ADR-110 Decision 6).
- **The exact reader is a struct.** `NewExactEntityReader` refuses a nil requester. The reader returns the entity it
  decoded without copying it.
- **Mutation outcomes are four:** applied, unchanged, entity_not_found, failed. `graph_mutation_outcomes_total`'s
  `outcome` label becomes the classified code. `revision_mismatch` and `index_not_ready` are each spelled once, in
  `pkg/errs`.
- **Wire changes.** `ExactEntity` carries `kv_revision`. `QueryVerb` is `{Name, Subject, Responder}`. The entity and
  batch requests are named types in `graph`.
- **Exports used by one package move into it.** The cursor codec and the trusted decoder move into graph-ingest. One
  prefix limit constant replaces two. The create and reconcile replies share one type. `ValidateEntityPredicates` is
  unexported.
- **Typed index state.** `IndexState` is a named type, and `IndexStates()` returns a copy.
- **Docs.** One package comment. Citations that resolve only in SemStreams say so. The README, the `query-pattern`
  skill and the ledger row are updated.

## Capabilities

- Modified: `graph-entity-writes` (added: one rule decodes a stored entity).
- Modified: `graph-transport-boundary` (the verb table's fields; the reader's `kv_revision`; added: mutation replies
  carry four outcomes).

## Impact

- Files: `graph` (most root files); `processor/graph-ingest` (`mutation_runtime.go`, `entity_writes.go`,
  `component.go:1296-1300`, `query.go`, `canonical_mutations.go`, tests); `pkg/lifecycle` (`manager_query.go:267`,
  `manager.go:45`, test doubles); `pkg/projection/mutation_client.go:31`; `internal/graphmutation/client.go`;
  `pkg/errs/errs.go:340-348`; `natsclient/kvspec.go:136-142`, `:350`; `graph/readiness/gauges.go:144`,
  `readiness_gate.go:168`; one assertion in `graph/kvcatalog/kvcatalog_test.go:165`; `graph/README.md`;
  `.agents/skills/query-pattern/SKILL.md` and its adapter; `docs/admission-ledger.yaml` (`graph` row).
- Wire changes: `kvRevision` becomes `kv_revision`. Metric label values change: `invalid` becomes
  `invalid_request`, and four codes that were counted as `internal` are named. The mutation reply
  JSON is unchanged. No deployment migrates (deployments wipe and start fresh); no consumer runs the ported
  graph-ingest.
- Behaviour change: the lifecycle `Watch` / `WatchEvents` return an error for a stored value that graph-ingest refuses,
  instead of delivering it.
- Closes #132, #276. Addresses #223.
