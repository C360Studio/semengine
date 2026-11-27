# Design: fix-765-warm-reindex

Base: `origin/main` 3d823c82. Inventory: `inventory.md` (pins, overlaps, ADRs, collision table, adopter seam).

## Context

On every start the follower replays every stored entity (`graph/follower/follower.go:251`) and graph-index applies
each one from current state (`processor/graph-index/index.go:215`). Applying lists the entity's rows in INCOMING,
PREDICATE, ALIAS and NAME by an owner filter whose leading tokens are wildcards (`owner_reconcile.go:46`, filters at
`incoming_index.go:30`, `predicate_index.go:20`, `name_index.go:40`), so rows the entity no longer has can be deleted
(ADR-077 §4). Queries are refused until the replay ends (graph-index spec, "Startup-replay queries get a retryable
refusal"). Measured (#765 comment 7208287082, nats-server 2.15.0, nats.go v1.55.0, one run per size): at 40,000
entities the Get is 0.24 ms, plan 0.04 ms, writes 0.6 ms, the four listings 48 ms median; a one-lane warm restart runs
17 entities/s, about 38 minutes (inferred from a 300 s window). Each listing creates and deletes one consumer.

Nothing persists what was indexed for an entity. The only baseline is in memory, `lastProjections`
(`component.go:89`), used for a metric and lost at every stop (`index.go:498-508`).

## Goals / Non-Goals

Goals: a warm restart costs about one read per stored entity more than today's Get, not a listing; a live update
needs no listing; every correctness property a restart has today holds, except one declared limit (Decision 3).
Non-goals: the 105k watermark plateau (#765, open), the default lane count, other followers (#750), first-start cost.

## Decisions

### Decision 1. What a restart trusts: a per-entity record (recommended)

Each option: warm 40k restart with one lane; what it trusts across a restart; what breaks that trust; what it
forecloses.

- **A. Do nothing.** About 38 minutes, growing with the store. Trusts nothing. Forecloses nothing.
- **B. More lanes by default.** 20k went from 610 s to 121 s at 16 lanes, NATS CPU about 7x (measured once). Trusts
  nothing; still grows with the store. Complements any option.
- **C. Cheaper listings.** Unknown gain, not probed. Removing the leading wildcards needs owner-first keys, which
  breaks every reader's prefix read (incoming by target, predicate by predicate, name by hash); one listing across
  four buckets does not exist. Rejected.
- **D. Per-index resume revision (#108).** Re-reads only what changed since the stop. Trusts the floor, but failed
  entities complete their revision (`index.go:102`, `:233`), so a resume point skips them; the damage record and
  failed set are in memory; a vocabulary or layout change is never applied to old entities. Needs both persisted.
- **E. "Indexed at revision R" mark.** About one Get per entity. Breaks on a product registering a new alias or
  name predicate (`index.go:87-88`), a row-layout change, a recreated index bucket. Live updates still list.
- **F. Per-entity record of the plan (recommended).** About two Gets per entity, roughly 20 s at 40k (inferred:
  0.24 ms Get, 0.04 ms plan, a record Get assumed equal to the entity Get). Breaks on a recreated index bucket
  (detected) and rows edited by hand (declared, Decision 3). Live updates stop listing too. Forecloses nothing: the
  listing stays as the fallback.

F stores, per entity, the plan it last indexed (outgoing targets, predicates, incoming targets, aliases, names with
their priority) and a format version, in a graph-index bucket of its own. The rows are a pure function of plan and
format version, so equal plans mean equal rows by construction; no proof of a separate projection is needed, which
`computeIndexProjection` would need (it leaves out name priority, and keeps an alias `TrimSpace` refuses,
`index.go:383` against `:540`). The record replaces `lastProjections` and `computeIndexProjection` (one spelling).

Rules a caller can observe, each a spec scenario:

1. **Superset.** At every moment, every row an entity owns is named by its record, or the entity has no record.
2. **Complete last.** A record reads complete only after every row it names is stored with its value and every row
   it no longer names is deleted. A cut-off write (crash, kill, lost connection) leaves it incomplete, and the next
   apply deletes what it names and the plan lacks, without a listing. This is Fable's #768 condition.
3. **Skip.** An entity whose value decodes, plans, and equals a complete record is neither listed nor written; its
   revision completes. A damaged value is checked before the record, so damage is still reported each start.
4. **Fallback.** No record, or a format version this code does not know: list as today, then write the record.
5. **Delete.** A delete removes the rows the record names, then the record.
6. **Recreated bucket.** An index bucket created after the records were written makes every record untrusted for
   that start; that start lists every entity.

Adopter seam: no consumer surface changes (no reply, subject or config field). The one new fact is for whoever next
edits graph-index: a change to any row key or value bumps the format version. A golden test fails loudly when key
output changes and the version does not. An operator who hand-edits an index bucket while graph-index is down must
delete the record bucket; otherwise nothing tells them (Decision 3).

### Decision 2. Where the record lives (recommended: its own bucket)

Own bucket, `GRAPH_INDEX_RECORDS`, catalog class derived, owner graph-index, one key per entity ID (the OUTGOING
layout). Alternative: widen the OUTGOING_INDEX value; rejected, it is a declared output and its value shape would
change for every reader. The record is about the size of the entity's index-relevant triples; a record the bucket
refuses fails the apply and marks the entity failed (fail closed, readiness withheld).

### Decision 3. Hand edits while down (owner question)

Today a restart re-derives every row, so a row deleted or added by hand is repaired. With F, a recreated bucket is
detected (rule 6) but a hand-deleted row under a complete record is not. Options: (a, recommended) declare it, with
"delete the record bucket" as the documented full rebuild; (b) after the first build, audit every entity by listing
at a low rate behind readiness, repairing mismatches; costs today's listing load, off the query path.

### Decision 4. ADR-077 §4 changes

§4 says the lane "lists the stored owner rows". After F it reads the owner's record and lists only on fallback.
ADR-077's Context rejected "a central reverse manifest"; F's record is per entity with one writer (the entity's lane,
ADR-065's invariant), not a shared list. Recommended: an amendment under ADR-077's port note. ADR-066, ADR-108 D4
(read current truth at apply), ADR-109 D1 and D4 are kept unchanged.

## Risks / Trade-offs

- Not promised by NATS (not probed): that an acknowledged record Put implies the row Puts acknowledged before it
  survive a server crash or power loss; nats-server's file-store sync timing is the open point. Fable question.
- `reindex_events_total{result}` now survives a restart: an unchanged entity after a start counts `unchanged`.
- The first start after upgrading costs today's time plus one record write per entity.

## Migration Plan

None beyond rule 4: a store without records is listed once and recorded. No wipe.

## Open Questions

- Owner: F over E, D and B (Decision 1).
- Owner: Decision 3, (a) or (b).
- Owner: the form of the ADR-077 amendment (Decision 4).
- Fable: are rules 1 to 6 enough for a restart to trust a complete record, and is the cross-bucket durability point
  under Risks a real hole that needs Decision 3 (b)?
