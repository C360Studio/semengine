# Proposal: fix-512-prefix-dedup

Approved-by: OPEN-Q (the owner fills this line)

## Why

Issue #512. Under concurrent overwrites, a nats.go key listing can repeat some keys and omit as many: nats.go ends
the watch-based listing by message count (v1.55.0 `jetstream/kv.go:1322-1330`), so an overwritten key's second
delivery takes the place of a key at the end. nats.go documents the repeats ("duplicate keys may be reported during
listing", `jetstream/kv.go:1431-1432`). The omissions are how nats.go lists keys; no `natsclient` listing can tell a
pushed-out key from an absent one without its own mechanism, and `natsclient` stays a pass-through (ADR-112). So the
omission half is not fixed here; the issue's "Done when" accepts telling callers it is a best effort, which the
closing comment does.

The repeat half is one defect. Every caller of the filtered listings on `main` (59231bb) removes repeats or counts
a set, except graph-ingest's prefix query (`processor/graph-ingest/query.go`, the `sort.Strings(keys)` before the
cursor). There a repeated key is served twice in a page and counted twice in `total`, against the repo rule
"sort/dedup before limit" (`AGENTS.md` § 2). MEDIUM by #185: it needs heavy overwrites during a listing.

Overlap: PR #530 touches nothing under `processor/graph-ingest` and not this requirement (checked 2026-11-07).

Quick track (owner, 2026-11-07): proposal, tasks and a spec delta, no design document. Codex reviews the code. This PR
closes #512; the closing comment records that omissions under concurrent writes are nats.go's listing behaviour.

## What Changes

- The prefix query removes repeats right after sorting (`slices.Compact`), before `total`, the cursor and the limit.
- A unit test drives the prefix handler over the package's `mockKVBucket` with a listing that repeats one key. It
  expects the entity once in the page and `total` counting it once; the oracle is the set of IDs the test stored. It
  fails on the base commit (the entity twice, `total` one too high).
- Examples suffice, no generated checks (`docs/testing.md`): one input shape (a listing with a repeat) and one law
  (sorted, unique, then paged).

## Capabilities

### Modified Capabilities

- `graph-transport-boundary`: "A prefix page names its total" counts distinct keys and serves a repeated key once.
