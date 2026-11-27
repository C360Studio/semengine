# Design: fix-765-warm-reindex

Base: `origin/main` 3d823c82. Inventory: `inventory.md`.

## Context

On every start the follower replays every stored entity (`graph/follower/follower.go:251`) and graph-index applies
each from current state (`processor/graph-index/index.go:215`): four owner listings with leading wildcards
(`owner_reconcile.go:46`) find rows the entity no longer has (ADR-077 §4). Queries are refused until the replay ends.
Measured (#765 comment 7208287082, nats-server 2.15.0, nats.go v1.55.0, one run per size): at 40,000 entities the Get
is 0.24 ms, plan 0.04 ms, writes 0.6 ms, the four listings 48 ms median; one lane runs 17 entities/s, about 38
minutes (inferred from a 300 s window). Nothing persists what was indexed; `lastProjections` is memory only
(`component.go:89`). The nearest stored "restart becomes a cheap skip" record is embedding's
(`graph/embedding/storage.go:251`): source revision plus the producer's identity. The nearest detector of a bucket
created again is the stream creation time used as a generation (`graph/follower/follower.go:546-556`,
`processor/graph-ingest/consume.go:80-92`).

## Goals / Non-Goals

Goals: a warm restart over unchanged entities lists nothing; every correctness property a restart has today holds,
except the declared limits of Decision 3. Non-goals: live-write cost, the 105k plateau (#765), lane defaults, #750.

## Decisions

### Decision 1. What a restart trusts (recommended: E, the mark)

- **A. Do nothing.** About 38 minutes, growing with the store.
- **B. More lanes by default.** 20k: 610 s to 121 s at 16 lanes, NATS CPU about 7x (measured once). Still grows.
- **C. Cheaper listings.** Owner-first keys break every reader's prefix read; no listing spans four buckets.
- **D. Per-index resume revision (#108).** Failed entities complete their revision (`index.go:102`, `:233`), so a
  resume point skips them; the failed set and damage record are memory only; vocabulary changes never reach old
  entities.
- **E. A mark per entity (recommended).** It follows embedding's shape, and adds generations. The mark holds the
  revision indexed, a digest of every row written (keys and values, OUTGOING's value as stored), and the creation
  time of ENTITY_STATES and of each of the five index buckets. It is written once, after every row write succeeded:
  Fable's #768 condition and ADR-077 §6's "baseline only after every projection succeeds". Cost per unchanged entity:
  the entity Get, the plan, a mark Get, so about 0.5 ms, roughly 20 s at 40k (inferred; the mark Get is assumed to
  cost what the entity Get does). Real costs: live updates still list; an equal re-arrival (ADR-110 Q4, new revision,
  same rows) still lists; a vocabulary change lists only the entities whose rows it changes.
- **F. A stored list of each entity's rows.** It finds old rows without listing, so live writes get faster too. But
  it needs two writes per change: first a list naming old and new rows, marked incomplete; then the rows; then the
  list marked complete. A single write at the end leaves stale rows after a cut write followed by A or C. It must
  store each row key (the key holds the predicate) and OUTGOING's value as written (ordered, duplicates kept). A
  restart then trusts it to find stale rows, and that changes ADR-077 §4. The follow-up if live-write cost matters.

E's rules, each a spec scenario:

1. **Skip.** graph-index skips an entity when four things hold: its stored value decodes, its revision equals the
   mark's, the digest of its planned rows equals the mark's, and every generation equals the one read at this start.
   It then does not list or write, removes the entity from the failed set, and completes the revision. DataFlow
   still counts the read.
2. **Never after a cut.** A mark names only a revision whose every write succeeded. A cut write leaves the older mark
   or none. Every later value has a new revision, so it cannot match, whatever it holds: A again, B or C.
3. **Delete.** After a clean delete, the mark records the tombstone's revision. A replayed tombstone at that revision
   is skipped. With no mark, it is listed as today.
4. **Fallback.** A mark that is missing, does not decode, or cannot be read (any error other than "not found") means
   the entity is indexed as today. A mark write that fails is not an index failure, because the rows are right; it
   is counted, and the next start lists that entity.
5. **Recreated bucket.** Every mark carries the generations it was written under. A bucket created again makes every
   older mark mismatch, entity by entity, so a kill during the rebuild loses nothing. No marker key is needed.

The digest is over the rows, so a change to the row layout or to vocabulary changes it on its own; there is no
format version to bump. Adopter seam: no reply, subject or config field changes; the operator facts are Decision 3.

### Decision 2. Where the mark lives

Its own bucket, `GRAPH_INDEX_MARKS`, one key per entity ID (OUTGOING's layout), owner graph-index. Class derived:
losing it costs time, never truth. (Its precedent, `GRAPH_INGEST_APPLIED_SEQ`, is operational, because losing that
one does change behaviour.) Widening OUTGOING_INDEX's value instead is rejected: it is a declared output.

### Decision 3. What a restart no longer repairs (owner)

Today every start re-derives every row. With E, these go undetected under a matching mark:

- rows edited or deleted by hand;
- a bucket emptied in place (its creation time is unchanged);
- a bucket restored from a backup with its creation time (the follower's declared limit);
- rows rewritten by a graph-index from before this change;
- row writes lost to a host power cut. nats-server's file store flushes to disk on a timer: `defaultSyncInterval =
  2 * time.Minute` (`server/filestore.go:340` at v2.15.3, the module this tree pins; the 2.15.0 image was not read,
  and nothing was probed). So a mark can survive while its rows are lost. A store rolled back below the mark's
  revision does mismatch.

The owner chooses between (a), declaring these with one reset (delete the mark bucket), and (b), a background check
after ready.

## Risks / Trade-offs

- `reindex_events_total{result}` now reads the mark, so an unchanged entity after a start counts `unchanged`.
- The first start after upgrading costs today's time plus one mark write per entity.

## Migration Plan

None: rule 4 lists a store without marks once.

## Open Questions

- Owner: E over F and B (Decision 1); Decision 3 (a) or (b), for hand edits and for power loss.
- Fable: is a mark that names a fully written revision, together with the generations it was written under, enough
  for a restart to skip the entity? Or is one of the undetected cases in Decision 3 a real hole that needs (b)?
