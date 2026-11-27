# Design: fix-765-warm-reindex

Base: `origin/main` 3d823c82. Inventory: `inventory.md`.

## Context

On every start the follower replays every stored entity (`graph/follower/follower.go:251`) and graph-index applies
each from current state (`processor/graph-index/index.go:215`): four owner listings with leading wildcards
(`owner_reconcile.go:46`) find rows the entity no longer has (ADR-077 §4). Queries are refused until the replay ends.
Measured (#765 comment 7208287082, nats-server 2.15.0, nats.go v1.55.0, one run per size): at 40,000 entities the Get
is 0.24 ms, plan 0.04 ms, writes 0.6 ms, the four listings 48 ms median; one lane runs 17 entities/s, about 38
minutes (inferred). SemSource re-offers its full seed on every boot (#765 body). graph-ingest's stream lane stores an
equal re-offer as a new revision (`processor/graph-ingest/entity_writes.go:238-254`; ADR-110 Q4). So a skip keyed on
the revision misses that workload. Nearest shapes: embedding's restart skip (`graph/embedding/storage.go:251`), its
length-prefixed digest (`graph/embedding/dedup.go:79-90`), and the creation time used as a generation
(`follower.go:546-556`, `processor/graph-ingest/consume.go:80-92`).

## Goals / Non-Goals

Goals: an entity whose rows would come out unchanged is neither listed nor written, whether on a restart or on a
re-offer; every property a restart has today holds, except Decision 3's declared limits. Non-goals: the cost of a
real change, the 105k plateau (#765), lane defaults, #750.

## Decisions

### Decision 1. What a skip matches (owner)

- **A. Do nothing; B. more lanes** (20k: 610 s to 121 s at 16 lanes, about 7x the NATS CPU, measured once);
  **C. cheaper listings** (owner-first keys break every reader's prefix read); **D. resume revision** (#108: failed
  entities complete their revision, `index.go:102`, `:233`). Rejected as before.
- **E1. Rows digest, mark out first (recommended).** The mark holds a digest of the rows, a format constant and the
  generations; it holds no revision. It skips an unchanged entity and an equal re-offer alike. Its cost: a mark
  delete and a mark put on each apply that writes, about 0.6 ms against 48 ms of listings.
- **E2. Revision match.** Written once, last; no delete. It misses every equal re-offer, so the case that reported
  #765 is helped only if SemSource stops re-offering (its stated churn cut, #765 body). A dependency on a consumer.
- **E3. Measure E2 first** against a re-offering workload. The stream lane's code already answers that question, so
  the measurement costs a probe run and buys a number, not a decision.
- **F. A stored row list.** Rejected: it needs two writes per change, and a restart would trust it to find old rows,
  which changes ADR-077 §4.

E1's rules, each a spec scenario:

1. **Skip.** graph-index skips an entity when four things hold: the value decodes, the digest of its planned rows
   equals the mark's, the format constant equals the mark's, and every generation equals the one read at this start.
   It then does not list or write, clears the entity from the failed set, and completes the revision. DataFlow still
   counts the read.
2. **Mark out first, in last.** Any apply that will write deletes the entity's mark before its first row write. If
   that delete fails, nothing is written and the entity is failed (fail closed; the repair loop retries). After every
   write succeeds, the mark is put. A failed put is a failed write: it gets the existing Warn line,
   `write_failures_total`, a failed-set entry, and a repair. So a mark exists only while the rows equal its digest.
   A cut write leaves no mark, whatever value or rules come next.
3. **Delete.** A tombstone applies as a write of the empty row set: mark out, rows listed and removed as today, then
   the empty-set mark. A replayed tombstone then matches and is skipped. A damaged value's removal deletes the mark
   and writes none. Damage is checked before the mark, so it is reported on every start. A repair pass (revision 0)
   follows the same rules; no mark carries a revision.
4. **Fallback.** A mark that is missing, does not decode, carries another format constant, or cannot be read means
   the entity is applied as today (listing), then marked.
5. **Generations.** A bucket created again makes every older mark mismatch, entity by entity. A kill during the
   rebuild loses nothing.

The digest is sha256 over the planned rows as (bucket, key, value) triples. The triples are sorted, and each field
is length-prefixed as in `dedup.go:79-90`, because NAME and ALIAS share a key layout. The format constant is bumped
by any change to how rows are planned or cleaned, which forces one full re-list. A golden mark fixture, fixed plan
to fixed bytes, fails when the encoding moves without a bump. A cleanup fix that leaves the encoding alone must still
bump the constant; review alone enforces that.

### Decision 2. Where the mark lives

Its own bucket, `GRAPH_INDEX_MARKS`, one key per entity ID, owner graph-index, class derived: losing it costs time,
never truth. Widening OUTGOING_INDEX's value is rejected: it is a declared output.

### Decision 3. What a restart no longer repairs (owner)

Under a matching mark these go undetected:

- rows edited by hand;
- a bucket emptied in place, or restored from a backup with its creation time (the follower's declared limit);
- rows rewritten by a graph-index from before this change;
- writes lost to a host power cut. `defaultSyncInterval = 2 * time.Minute` (`server/filestore.go:340`, a source read
  of v2.15.3, the module this tree pins; the 2.15.0 image was not read, and nothing was probed).

The owner chooses between (a), declaring them with the reset "delete the mark bucket while graph-index is stopped",
and (b), a background check after ready. Deleting the mark bucket while graph-index runs fails every later mark
delete, so readiness is withheld until a restart; this is declared and tested.

## Risks / Trade-offs

- `reindex_events_total{result}` reads the mark, so an unchanged entity after a start counts `unchanged`.
- The first start after upgrading costs today's time plus one mark write per entity.
- Whether SemSource's re-offers land before or after the snapshot marker is not known. Either way E1 skips them; it
  decides only whether they count toward the first build.

## Migration Plan

None: rule 4 lists a store without marks once.

## Open Questions

- Owner: E1, E2 or E3 (Decision 1); Decision 3 (a) or (b).
- Fable: does "mark out before the first row write, in after the last" make an existing mark imply that the stored
  rows equal its digest, for every cut point and every change of value or rules between applies, given Decision 3's
  declared limits?
