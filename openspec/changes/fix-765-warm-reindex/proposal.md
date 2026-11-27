# Proposal: fix-765-warm-reindex

Approved-by:

Track: `design`. Step 3 of #765, as the owner ordered it (issue comment 7196901233): a design read by Fable before
code, because it changes what a restart trusts. Edits one capability, `graph-index`.

## Why

Every start re-indexes every stored entity. For each one graph-index lists the rows the entity already owns in four
index buckets, so it can delete the ones the entity no longer has. Measured on #765 (comment 7208287082): those four
listings are 93 to 98 % of an entity's time, each costs more as the store grows (0.6 ms at 200 entities, 12 ms at
40,000), and a warm restart of 40,000 entities with one lane takes about 38 minutes, all of it refusing queries.
SemSource also sends its whole seed again on every boot. Each of those writes is stored as a new version even when
nothing in it changed, so graph-index re-indexes those entities again too.

## What changes

- After every row write for an entity has succeeded, graph-index stores a small mark for it: a fingerprint of the
  rows it wrote, and which copy of each bucket it wrote them in. Before any later write for that entity, it removes
  the mark first.
- An entity whose rows would come out the same as its mark says is neither listed nor written, on a restart or when
  a producer sends the same entity again. Anything else is indexed exactly as today.
- A write cut off part-way leaves no mark, so the next apply lists that entity as today.
- A store with no marks (indexed before this change) is listed once and marked.
- Expected effect, estimated and not yet measured: a warm restart of 40,000 unchanged entities, and SemSource's
  re-sent seed, take tens of seconds instead of tens of minutes.

## Decisions you are making by approving

- **Skip when the rows would come out the same (recommended).** A cheaper version would skip only when the entity's
  version number is unchanged. But SemSource's re-sent seed gives every entity a new version number with the same
  content, so that version would not help the case that reported this issue unless SemSource stops re-sending. A
  third choice is to measure first. The recommended version costs one extra small write per changed entity.
- **Skip by a mark, not by a stored list of rows (recommended).** A stored list of each entity's rows would also make
  real changes faster, but a restart would then trust it to find stale rows, and it changes ADR-077's decision. With
  the mark, listing stays how old rows are found; ADR-077 gets a note only.
- **Changes made to the index buckets by hand while graph-index is down.** Today a restart repairs them. After this, a
  bucket deleted and created again is still caught and rebuilt. Rows deleted by hand, a bucket emptied in place, or a
  bucket restored from a backup are not caught. Choose (a, recommended) to declare this, with the reset "delete the
  mark bucket while graph-index is stopped" (the next start rebuilds everything), or (b) to have graph-index re-check
  every entity in the background after it is ready, which brings back today's listing load without blocking queries.
- **Power loss on the NATS host.** NATS writes to disk on a timer, not on every write. So after a power cut, a mark
  can survive while the rows it vouches for are lost. Today the next start repairs this; after this change it does
  not. Choose (a, recommended) to declare it and say "after a host crash, delete the mark bucket", (b) to rely on the
  background check above, or (c) to tell deployments that need it to make NATS write every change to disk.
- **Going back to an older graph-index.** A version from before this change rewrites rows without removing marks.
  The declared step (recommended) is: after running an older graph-index on the same store, delete the mark bucket.
- **Leave the default lane count alone here (recommended).** 16 lanes made a 20,000-entity restart five times faster
  but used seven times the NATS CPU. That is a separate question.
