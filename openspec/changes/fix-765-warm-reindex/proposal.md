# Proposal: fix-765-warm-reindex

Approved-by:

Track: `design`. Step 3 of #765, as the owner ordered it (issue comment 7196901233): a design read by Fable before
code, because it changes what a restart trusts. Edits one capability, `graph-index`.

## Why

Every start re-indexes every stored entity. For each one graph-index lists the rows the entity already owns in four
index buckets, so it can delete the ones the entity no longer has. Measured on #765 (comment 7208287082): those four
listings are 93 to 98 % of an entity's time, each costs more as the store grows (0.6 ms at 200 entities, 12 ms at
40,000), and a warm restart of 40,000 entities with one lane takes about 38 minutes, all of it refusing queries. Live
writes pay the same 48 ms per entity.

## What changes

- graph-index keeps, per entity, a record of what it last indexed for it, in a bucket of its own. The record names
  every row the entity may own, and says the rows are complete only once every write has finished.
- An entity whose record is complete and matches what its current value asks for is not listed and not written.
  A changed entity finds its old rows from the record, not by listing.
- An entity with no record (a store indexed before this change, or a wiped record bucket) is listed as today, once.
- An index bucket that was deleted and created again while graph-index was down makes every record untrusted, and
  that start rebuilds everything as today.
- Expected effect, inferred and not yet measured: a warm restart of 40,000 unchanged entities in tens of seconds
  instead of about 38 minutes; the first start after upgrading still takes the full time once.

## Decisions you are making by approving

- **Keep a per-entity record instead of a "done at revision R" mark (recommended).** A revision mark misses changes
  that do not change the entity: a product that registers a new alias or name predicate, or a graph-index upgrade
  that changes its row layout. The record compares what the value asks for now, so those are re-indexed.
- **Change the old decision record ADR-077 (recommended: an amendment under its port note).** ADR-077 chose to find
  an entity's rows by listing, without a stored list. After this, listing is only the fallback; the record is how a
  restart and a live update find old rows.
- **Hand edits to an index bucket while graph-index is down become a declared limit (recommended).** Today a restart
  repairs them. After this, a bucket deleted and recreated is still detected and rebuilt, but a row deleted by hand
  is not; the documented reset is to delete the record bucket, which forces a full rebuild on the next start.
- **Keep the listing for stores that have no records (recommended)** rather than asking deployments to wipe: the
  code exists today, and the first start after the upgrade is the only slow one.
- **Leave the default lane count alone here (recommended).** 16 lanes made a 20,000-entity restart five times faster
  but used seven times the NATS CPU; that is its own question.
