# Design: fix-765-follower-keys-only

## Context

- At boot graph-index's follower watch replays every stored entity with its full value. At 16,000 entities and more,
  the watch's client-side buffer passes nats.go's 64 MiB subscription limit, messages are dropped, and nats.go resets
  the watch without a log line (#750, #765 inventory comment 7196643617).
- graph-index never applies the delivered value: it reads the entity again before it writes
  (`processor/graph-index/index.go:211`). graph-lexical and graph-clustering apply it
  (`processor/graph-lexical/apply.go:156`, `processor/graph-clustering/component.go:281`). graph-embedding re-reads
  current state too (`processor/graph-embedding/hop1.go:26-31`; `:80` passes its own read's value), but counts its
  input from the delivered `Entry.Bytes` (`hop1.go:34`), and its watch still carries values.
- With `jetstream.MetaOnly()` the watch still delivers a delete and a purge with their operation, delivers revisions
  in order, and sends the end-of-replay marker, at nats.go v1.55.0 and the pinned nats-server
  (`notes/2.1.md`, from the follower's integration tests in both modes).

## Decision

**D1. A keys-only field on `follower.Config`, off by default.** The watch belongs to the follower, so the choice of
what it carries is a `Config` field: one bool, no option function and no second constructor (Fable, #765 comment
7197615534). Off by default leaves the three readers that apply values as they are. With it set, `Entry.Value` is nil
and `Entry.Bytes` is 0, and the doc comment says the plug-in reads the entity itself.

**D2. graph-index counts its input at its own read** (owner, 2026-11-26). `component.Lifecycle.CountInput` adds one
message with its bytes, so moving the bytes moves the message count with them. One input becomes one entity read,
with that read's bytes. Not chosen: a bytes-only method on `component.Lifecycle`, which is new framework surface for a
counter, and leaving the count at delivery, where its bytes would read 0 for ever.

**D3. A progress log during the initial build,** at Info on the heartbeat that already publishes the envelope:
entries delivered, plus the watermark's observed and indexed revisions. It stops once the envelope reads the build
complete.

## What stays open

Issue #750 stays open for the three readers still watching values. Steps 2 and 3 of #765 follow: measuring one
entity's reconcile cost, then a design that avoids re-indexing on every boot (#108), read by Fable before code.
