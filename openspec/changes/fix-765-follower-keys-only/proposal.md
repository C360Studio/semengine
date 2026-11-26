# Proposal: fix-765-follower-keys-only

Approved-by: cglusky (2026-11-26) <https://github.com/C360Studio/semengine/pull/772#issuecomment-7197671217> sha256:da8362268350

Track: `quick`. Capability: `entity-state-follower` (one Requirement modified: "An entry carries the delivered
value"). Packages: `graph/follower`, `processor/graph-index`.

## Why

After a warm restart over a large store, graph-index's follower watch replays every stored entity with its full
value. The watch's client-side buffer passes nats.go's 64 MiB subscription limit, messages are dropped, and nats.go
resets the watch, with nothing logged (#750, #765). graph-index never uses the delivered value: it reads each entity
again when it applies it (`processor/graph-index/index.go:211`). The other three users of `graph/follower` do apply
the value (`processor/graph-lexical/apply.go:156`, `processor/graph-embedding/hop1.go:80`,
`processor/graph-clustering/component.go:281`).

This is step 1 of the order the owner ruled on #765 (comment 7196901233): small fix, then measure, then design. Its
shape was confirmed in a Fable read (comment 7197615534).

## What changes

- `follower.Config` gains a keys-only field, off by default. With it set, the watch delivers keys and revisions only
  (`jetstream.MetaOnly()`). `Entry.Value` is nil and `Entry.Bytes` is 0 for every entry, and the owner reads the
  entity itself. The field's doc comment says so.
- graph-index sets it. Its input-bytes count moves from the delivered entry to the bytes of its own entity read, so
  the count keeps meaning what graph-index took in.
- While the initial build runs, the follower logs its progress at Info: entries delivered, the watermark's observed
  and indexed revisions. Today that stretch is silent.

## Decisions you are making by approving

- graph-index's follower stops receiving entity values at boot. Only graph-index changes; the other three readers keep
  receiving values.

## Non-goals

- #750 stays open, narrowed to graph-lexical, graph-embedding and graph-clustering, whose watches still carry values.
- Steps 2 and 3 of #765: measuring one entity's reconcile cost, and a design that avoids re-indexing on every boot
  (#108).
- Why the watermark sat near 105k and then jumped.

## Impact

graph-index's boot replay no longer fills the subscription buffer with entity values. One PR: `Addresses #765`,
since steps 2 and 3 stay open.
