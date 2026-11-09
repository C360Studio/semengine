# Final review fixes (Codex, PR #550 comment 6727056233)

Base `697904d`. F1 retires #555; F2 and F3 are document corrections.

## F1: an absent or unreadable read completes only what was delivered before it

`reconcileEntity` now takes the highest revision the follower has handed `deliver` (a new unexported
`Component.delivered`, stored first thing in `deliver`, zeroed in `start`) before its `ENTITY_STATES` read, and
`readFailed` completes the key at that revision instead of the pin's `^uint64(0)`. A revision delivered after the read
began is either an update, which the coalescer re-adds and a later batch reconciles to its own outcome, or a
tombstone, which completes itself, so it stays outstanding until then. On the inline lane the follower is blocked in
`Deliver` for the read, so the bound is the revision being reconciled. The failed map is unchanged: its stranded mark
names no source revision and is cleared only by a hop-1 convergence, which the re-creation's own batch is.

`TestRecreatedRevisionWaitsForWorker` (Codex's reproduction) runs `reconcileEntity` with its read held, through a test
reader, once it has found the entity absent, re-creates the entity, waits for its delivery, then releases the read.
Failing first at `697904d` (Codex's hand-replayed form): `hop1_integration_test.go:574: recreated revision 3 is still
pending, but indexed revision is 3`.

Wrong changes, each a copy outside the repository swapped in by `-overlay` as in `notes/2.5.md`:

| Diff | Test | Failure |
| --- | --- | --- |
| `final-mutants/r1_absent_drains_all.diff` (the pin's completion) | `TestRecreatedRevisionWaitsForWorker` | `:583: recreated revision 3 is still pending, but indexed revision is 3` |
| `final-mutants/r2_delivered_after_read.diff` (bound taken after the read) | `TestRecreatedRevisionWaitsForWorker` | `:583: recreated revision 3 is still pending, but indexed revision is 3` |

Uncovered: the unreadable-read branch (a `Get` error other than not-found) is not driven against a re-creation; a
wrong change to its completion alone would survive this test.
