# Design: fix-765-warm-reindex

Base: `origin/main` 3d823c82. Inventory: `inventory.md`. Ruling: PR #774 comment 7217731180.

## Context

Each apply runs four owner listings with leading wildcards (`processor/graph-index/owner_reconcile.go:46`). They find
rows the entity no longer has (ADR-077 §4), on a restart's replay (`graph/follower/follower.go:251`) and on every
live change. Measured (#765 comment 7208287082, nats-server 2.15.0, nats.go v1.55.0, one run per size): at 40,000
entities the Get is 0.24 ms, plan 0.04 ms, writes 0.6 ms (two puts), the four listings 48 ms median; one lane does
17 entities/s, a warm restart about 38 minutes (inferred). One listing costs 0.6 ms at 200 entities and 12 ms at
40,000.

ADR-077 approved owner listing "provided that ... real-NATS evidence proves the leading-wildcard shape remains
bounded under the production workload" (`docs/adr/077-bounded-owner-discovery-and-incoming-ownership.md:40-42`). The
measurement refutes that condition, so this change amends §2 and §4. SemSource re-offers its seed every boot, and an
equal re-offer is a new revision (`processor/graph-ingest/entity_writes.go:238-254`), so no revision can key a skip.

## Goals / Non-Goals

Goals, each an estimate from the figures above with a record Get assumed to cost an entity Get (0.24 ms) and a record
put half the measured writes (0.3 ms):

- an unchanged entity, on a restart or a re-offer, about 0.5 ms (40k on one lane: about 20 s, not about 38 minutes);
- a live change about 1.7 ms, not about 49 ms;
- no listing outside the fallback; every property a restart has today, except Decision 3's declared limits.

Non-goals: a first seed (no records yet, so it lists as today), the resume point (#108), lane defaults, #750.

## Decisions

### Decision 1. The per-entity record (ruled)

One record per entity ID in `GRAPH_INDEX_RECORDS`, written only by that entity's lane, repair included. It holds:

- a state, pending or clean;
- the rows, as sorted (bucket, key, sha256 of value) triples, length-prefixed as in `graph/embedding/dedup.go:79-90`
  (NAME and ALIAS share a key layout);
- a format constant;
- the generations, meaning the creation times of ENTITY_STATES and the five index buckets, read as
  `follower.go:546-556` and `processor/graph-ingest/consume.go:80-92` do.

A record is usable when it decodes and its format constant and generations match this start's. Rules, each a spec
scenario:

1. **Skip.** The value decodes (damage is checked first), and a usable clean record's rows equal the planned rows.
   Nothing is listed or written, the entity leaves the failed set, and its revision completes. DataFlow counts the
   read. This covers a restart and an equal re-offer.
2. **Pending first, clean last.** An apply that writes first puts a pending record holding the record's rows (or the
   listed rows) together with the planned rows. If that put fails, nothing is written and the entity is failed
   (fail closed; repair retries). After the last row write it puts the clean record with the planned rows. A failed
   record put or delete is a failed index write: Warn line, `write_failures_total`, failed set, repair. So whenever
   a record exists, it names every row the entity can own.
3. **Change.** For a usable clean record that differs from the plan: delete the rows it names that the plan drops,
   and put the rows that are added or have a changed value. There is no listing.
4. **Pending.** For a usable pending record: put every planned row, and delete every other row the record names.
5. **Fallback.** No record, or one that is unusable or cannot be read: list as today, then rules 2 and 3 with the
   listed rows. A generation mismatch therefore rebuilds entity by entity, and a kill mid-rebuild loses nothing.
6. **Delete.** A tombstone plans no rows: its rows are deleted, then a clean empty record is written, which a replay
   skips. A damaged value's removal ends with no record.
7. **Too large.** If the encoded pending or clean record exceeds the payload limit the server reports
   (`natsclient/client.go:358`), the record is deleted before any row write (fail closed, as in rule 2) and the
   entity is applied by listing, with no record. Each later apply of it lists: a declared cost for very large
   entities.

Listing stays, as the fallback and as the test oracle. After a generated history of changes, cuts and re-offers, each
record's rows equal what a listing of the four buckets plus OUTGOING finds. The format constant is bumped by any
change to how rows are planned, keyed or cleaned. A golden record fixture (a fixed plan gives fixed bytes) fails when
the encoding moves without a bump; a cleanup fix that leaves the encoding alone is caught only in review. The record
replaces `lastProjections` and `computeIndexProjection` (`index.go:498-508`, `:521`).

### Decision 2. ADR-077 amendment (ruled)

- §2: leading-wildcard owner discovery becomes the fallback and the test oracle, because its acceptance condition
  failed.
- §4: replacement diffs the plan against the owner's record, and lists only under rule 5.
- §1's ownership matrix and §5's source-owned INCOMING stand.
- The record is per entity with one writer, which is not the "central reverse manifest" of the Context (`:40`).

### Decision 3. What a restart no longer repairs (owner, except the power cut)

Under a usable clean record these are not caught:

- rows edited by hand;
- a bucket emptied in place, or restored with its creation time;
- rows written by a graph-index from before this change;
- writes lost to a host power cut. That last one is declared, with no choice offered here: it is the whole store's
  question, #775.

The owner chooses between (a), declaring the rest with the reset "delete the record bucket while graph-index is
stopped", and (b), a background check after ready. Deleting the record bucket while graph-index runs fails the next
pending put for every changed entity, so readiness is withheld until a restart: declared and tested.

## Risks / Trade-offs

- An incoming row is owned by its source, so a record holds one triple per relationship; that is what makes rule 7
  necessary.
- `reindex_events_total{result}` reads the record, so an unchanged entity after a start counts `unchanged`.
- Whether SemSource's re-offers arrive before or after the snapshot marker is unknown; either way rule 1 skips them.

## Migration Plan

Rule 5: a store without records is listed once per entity and recorded.

## Open Questions

- Owner: Decision 3 (a) or (b); the older-graph-index reset; the lane count (proposal).
- Fable: with one writer per entity, do rules 2 to 5 keep "a record names every row the entity can own" true at every
  cut point? That includes a cut after a pending put whose outcome is unknown, on the change, pending and fallback
  paths alike. And do rule 7 and the tombstone record leave any row no later apply can reach?
