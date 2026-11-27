# Proposal: fix-765-warm-reindex

Approved-by:

Track: `design`. Step 3 of #765, as the owner ordered it (issue comment 7196901233): a design read by Fable before
code, because it changes what a restart trusts. Edits one capability, `graph-index`.

## Why

Every start re-indexes every stored entity. For each one graph-index lists the rows the entity already owns in four
index buckets, so it can delete the ones the entity no longer has. Measured on #765 (comment 7208287082): those four
listings are 93 to 98 % of an entity's time, each costs more as the store grows (0.6 ms at 200 entities, 12 ms at
40,000), and a warm restart of 40,000 entities with one lane takes about 38 minutes, all of it refusing queries.

## What changes

- After every row write for an entity has succeeded, graph-index stores a small mark for it: the entity's revision,
  a fingerprint of the rows it wrote, and which copy of each bucket it wrote them in.
- On a later start, an entity whose revision, rows and buckets all still match its mark is neither listed nor
  written. Anything else is indexed exactly as today.
- A write cut off part-way never leaves a matching mark, so the next start lists that entity as today.
- A store with no marks (indexed before this change) is listed once and marked.
- Expected effect, estimated and not yet measured: a warm restart of 40,000 unchanged entities takes tens of seconds
  instead of about 38 minutes. Live writes keep their current cost.

## Decisions you are making by approving

- **Skip by a mark, not by a stored list of rows (recommended).** The alternative is a stored list of each entity's
  rows, used to find old rows without listing at all. It would also make live writes faster, but a restart would
  then trust it to find stale rows, it needs two writes per change, and it changes ADR-077's decision. With the
  mark, listing stays how old rows are found. The stored list can follow if live write speed is measured as a problem.
- **Changes made to the index buckets by hand while graph-index is down.** Today a restart repairs them. After this, a
  bucket deleted and created again is still caught and rebuilt. Rows deleted by hand, a bucket emptied in place, or a
  bucket restored from a backup are not caught. Choose (a, recommended) to declare this, with the reset "delete the
  mark bucket" (the next start rebuilds everything), or (b) to have graph-index re-check every entity in the
  background after it is ready, which brings back today's listing load without blocking queries.
- **Power loss on the NATS host.** NATS writes to disk on a timer, not on every write. So after a power cut, a mark
  can survive while the rows it vouches for are lost. Today the next start repairs this; after this change it does
  not. Choose (a, recommended) to declare it and say "after a host crash, delete the mark bucket", (b) to rely on the
  background check above, or (c) to tell deployments that need it to make NATS write every change to disk.
- **Going back to an older graph-index.** A version from before this change rewrites rows without updating marks. The
  declared step (recommended) is: after running an older graph-index on the same store, delete the mark bucket.
- **ADR-077 keeps its decision (recommended).** Listing stays how an entity's old rows are found. A note records that
  a matching mark skips the listing.
- **Leave the default lane count alone here (recommended).** 16 lanes made a 20,000-entity restart five times faster
  but used seven times the NATS CPU. That is a separate question.
