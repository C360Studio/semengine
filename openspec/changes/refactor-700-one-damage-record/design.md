# Design: refactor-700-one-damage-record

Premises are pinned in `inventory.md` (base `c6377db9`).

## Context

Two records of damaged entities exist: graph-ingest's poison inventory (`processor/graph-ingest/poison_inventory.go`)
and the follower's damage record (`graph/follower/damaged.go`). Both key by entity ID, keep the revision and the
refusal, replace an entry with a report at the same or a higher revision, and name the first 10 IDs in sorted order.
They differ at one boundary, for a reason each has: graph-ingest clears on a proof at or above the recorded revision,
because two of its proofs name the poisoned revision itself: a delete passes the caller's expected revision, the one
it removed (`entity_writes.go:469`, `:475`), and a re-check that finds the key absent passes the recorded revision
(`poison_inventory.go:159`). The follower clears only above it (a plug-in reports damage and then completes that same
revision, `damaged.go:65-73`). The owner asked for
one type (#700, comment 7030083210); the done-when is that the sites in scope read one type and each component's
existing damage tests pass unchanged.

## Goals / Non-Goals

- Goals: one record type for the follower and graph-ingest; no change any caller, operator or consumer can see.
- Non-goals: wording, the gauge, the debug JSON, the maintainer's set, `pkg/graphview`, refusal detail.

## D1. Which records share the type (Q1)

- **A (recommended): the follower and graph-ingest.** Same fact, same law up to one boundary, same sample.
- **B: all three.** The maintainer's `refused` set records what `newContribution` refuses (nil state, bad ID, ID
  not its key, `maintainer.go:423-431`), which `graph.UnmarshalEntityState` refuses first in the one production
  caller, so it is empty there. Its revision lives in `members` under the maintainer's one lock; moving the set to
  the shared type keeps a second revision per entity for no reader. graph-clustering's stored damage is already on
  the follower's record (`processor/graph-clustering/component.go:282`), so it reads the shared type under A.
- **C: none.** Two records of about 60 lines each, each tested, stay. The cost is two spellings of one law and of the
  10-ID sample, free to drift.
- `pkg/graphview`'s `poisoned` map is a derived view's (last writer wins in watch order, under the view's lock with
  its data, exposed as public `*PoisonError`); it is out of every option.

## D2. Where the type lives

- **A (recommended): `internal/damage`**, standard library only. Both users already import internal packages
  (`internal/revlag` from the follower). It adds no public surface: neither user exposes the type in an exported
  signature, so the rule "public signatures name no `internal/` type" holds, as does "the graph root imports no
  NATS" (the graph root does not import it) and "`component` reaches no graph-family package" (`component` does
  not import it, and it imports no graph package).
- **B: the `graph` root, exported.** Public API with no product caller: it would need a contract line and a product
  test, for a type only two SemEngine packages use.
- **C: exported from `graph/follower`.** graph-ingest would import the follower package (NATS watch, readiness,
  watermark) for a map type, though graph-ingest is not a follower.

## D3. The type's behaviour

`damage.Record` (zero value ready), an entry `damage.Entry{Key string; Revision uint64; Refusal error}`:

- `Report(key, revision, refusal) bool`: records the entry unless one is recorded at a higher revision; returns true
  only when the key was not recorded before. A nil refusal is the caller's to handle (graph-ingest skips it, the
  follower fills one in, as today).
- `Clear(key, below) bool`: drops the key's entry when it was recorded at a revision below `below`; returns whether
  it dropped one. `below` 0 drops nothing. The follower passes the completed or deleted revision; graph-ingest passes
  its proof revision plus one, because a proof at a revision covers that revision.
- `Len() int`, `Count(keep func(id string) bool) int` (nil keeps all), `Get(key) (Entry, bool)`,
  `Sorted(limit int) []Entry` (key order, all when `limit` is 0 or less).
- Safe for concurrent use.
- Cost note, not a requirement: graph-ingest's commit and read paths check for an empty record today with one atomic
  load (`poison_inventory.go:196`, `:207`, `:218`). The shared type should keep that cost; the reviewer checks it.

Invariant (every sequence of reports and clears): a key is recorded exactly when some report of it is at or above
every later clear's bound; `Len` equals the number of recorded keys; `Sorted(n)` is the first n of them in
key order. Spec homes: `entity-state-follower` "A damaged entity is recorded per key and cleared by its repair"
(follower bounds) and the `graph-ingest-recovery` delta (graph-ingest bounds).

## D4. graph-ingest on the shared record

graph-ingest keeps on top, unchanged in behaviour: the unstampable skip; the log line and cache invalidation when a
report is new; the re-check against the key's current bytes; the boot sweep and its scratch map, which keeps the
`entityPoisonRecord` type (so `TestBootSweepLogCap` and the `poisonInventoryEntry` helper's callers keep it); the
Health message from `Len` and `Sorted(10)`; `DebugStatus` from `Sorted(0)`, reason by `errors.As` on the stored
`*graph.StateContractError`; and the gauge, which equals `Len` once no report or clear is in flight, including
under concurrent reports and clears of different keys. Today the gauge is set while the record's lock is held
(`poison_inventory.go:104`, `:188`). If it is set after the lock is released, an older count can be written last.
That can happen when queries and mutations run concurrently during a mass-poison event, and alerting reads this
gauge. `TestGaugeFollowsConcurrentRecordAndClear` proves it. The developer picks how the gauge follows the record and
records the choice in `notes/4.2.md`. No read
or write decision consults the record (pin ADR-079 decision 1, kept; `TestPoisonInventoryIsObservabilityOnly`).

## D5. The follower on the shared record

`damagedRecord` becomes `damage.Record`. `ReportDamaged`, `DamagedCount`, `DamagedRefusal`, `Stuck`'s sentence and
the envelope's `damaged_entity` count read it; `clearDamaged` calls `Clear(key, revision)`. A completion at revision
0 clears nothing, as today. ADR-108's outcomes and ADR-109 Decision 2's stuck states are kept.

## Proving tests

- New, in `internal/damage`, written first: named cases (report then a clear above it drops; a clear at the
  recorded revision does not; a lower report is ignored and returns false; `below` 0 drops nothing; `Sorted` order
  and limit); `TestPropRecordFollowsLatestEvent`, a generated check of reports and clears over a few keys against a
  reference model, run sequentially, with the seed and completed runs recorded.
- Unchanged and passing: the follower's 8 damage tests and its envelope tests; graph-clustering's
  `TestRefusedEntityMakesTickIncomplete` and the maintainer property tests; graph-ingest's 16 tests in
  `poison_scoping_test.go`, `TestBootSweep*`, and `TestIntegration_BootSweepInventoriesResidentPoisonRealNATS`.
  graph-ingest's 10 size reads (9 `entityPoisonSize.Load()`, 1 `len(c.entityPoison)`) and one helper body change how
  they read the record (Q2), not what they assert.
- New in graph-ingest: `TestHealthNamesTenPoisonedEntities` (11 recorded: Health gives count 11, the first 10 sorted
  and "+1 more", and the debug status lists all 11 in sorted order), which no test covers today.
- New in graph-ingest: `TestGaugeFollowsConcurrentRecordAndClear`, under `-race`. Goroutines report and clear
  distinct keys concurrently and are joined with a `sync.WaitGroup`. After the join, the gauge equals an independent
  count of the keys left recorded.

## Adoption

The sweep found no other site of this fact: graph-embedding's, graph-lexical's and graph-index's failure records
hold write failures; `pkg/graphview` is out (D1). Nothing to file.

## Merge order

One PR on #701; no open PR overlaps.
