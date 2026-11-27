# Proposal: fix-765-warm-reindex

Approved-by:

Track: `design`. Step 3 of #765, as the owner ordered it (issue comment 7196901233): a design read by Fable before
code, because it changes what a restart trusts. Edits one capability, `graph-index`, and amends ADR-077.

## Why

Every time graph-index applies an entity, it lists the rows the entity owns in four index buckets, so it can delete
the ones the entity no longer has. That happens on every start, and on every live change. Measured on #765 (comment
7208287082): the four listings are 93 to 98 % of an entity's time, and each costs more as the store grows (0.6 ms at
200 entities, 12 ms at 40,000). A warm restart of 40,000 entities with one lane takes about 38 minutes, all of it
refusing queries, and a live change costs about 48 ms. SemSource also sends its whole seed again on every boot, and
each unchanged entity is stored as a new version, so graph-index re-applies it. The decision record behind the
listing, ADR-077, was accepted on the condition that listing stays bounded as the store grows; the measurement shows
it does not.

## What changes

- graph-index keeps one record per entity of the rows it wrote, in a bucket of its own. Before it writes anything for
  an entity, it marks the record "pending" with the old and the new rows. After the last write, it marks the record
  "clean" with the new rows.
- If the record is clean and the rows would come out the same, the entity is not touched. If they would differ, only
  the rows that changed are written or deleted, with no listing. If the record is pending (a write was cut off),
  every new row is written again and every other row it names is deleted. A pending record keeps every row it has
  ever named until a write finishes, so a second cut during that recovery loses nothing. Each record write checks
  that nobody changed the record since it was read, so two graph-index processes on one store for a moment (during a
  rolling update) cannot lose track of a row.
- With no record (a store indexed before this change, or an entity too large for one record), it lists as today.
- Estimated, not measured: a warm restart or re-sent seed of 40,000 unchanged entities in about 20 s instead of about
  38 minutes, and a live change in about 2 ms instead of about 48 ms.

Ruled by the owner (PR #774 comment 7217731180), not choices here: the per-entity record; amending ADR-077 §2 and §4;
the resume point stays on #108; a host power cut is declared and left to #775.

## Decisions you are making by approving

- **Changes made to the index buckets by hand while graph-index is down.** Today a restart repairs them. After this, a
  bucket deleted and created again is still caught and rebuilt. Rows deleted by hand, a bucket emptied in place, or a
  bucket restored from a backup are not caught. Choose (a, recommended) to declare this, with the reset "delete the
  record bucket while graph-index is stopped" (the next start rebuilds everything), or (b) to have graph-index
  re-check every entity in the background after it is ready, which brings back today's listing load without
  blocking queries.
- **Going back to an older graph-index (recommended: declare a reset).** A version from before this change writes
  rows without updating records. The declared step is: after running an older graph-index on the same store, delete
  the record bucket.
- **Leave the default lane count alone here (recommended).** 16 lanes made a 20,000-entity restart five times faster
  but used seven times the NATS CPU. That is a separate question.
