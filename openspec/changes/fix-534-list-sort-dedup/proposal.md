# Proposal: fix-534-list-sort-dedup

Approved-by: OPEN-Q

## Why

Issue #534. `pkg/lifecycle`'s `Manager.List` applies `Offset` and `Limit` (`pkg/lifecycle/manager_query.go`) to
participants in the order the key listing returns them. It neither sorts nor removes repeats. nats.go says its key
listing may report a key more than once on a busy bucket (v1.55.0 `jetstream/kv.go:1431-1432`; #512 measured up to
4 repeats in a 20,000-key listing under concurrent overwrites). So `List` can return a participant twice, and the
repeat pushes another participant out of a `Limit` page. The listing is in stream order, so a write between two calls
reorders it, and `Offset` pages skip or repeat participants.

This breaks the repo-wide rule "sort/dedup before limit" (`AGENTS.md` § 2). `List` has no production caller in this
repository; it is consumer API. MEDIUM by #185: it needs a busy bucket during a listing, and no workload has been
measured.

Overlap: PR #530 (change 4, step 4) touches nothing under `pkg/lifecycle` or `openspec/specs/lifecycle-query`, so
merge order is free (checked 2026-11-07). #533 (callers of nats.go `Keys` take a cut-short listing as complete) does
not reach `List`, which already lists through `natsclient.FilteredKeys` and fails a cut-short listing (#496).

Quick track: proposal and tasks. There is no spec delta, because the code is brought in line with the existing
`AGENTS.md` § 2 rule rather than a changed requirement. The "List returns every match" requirement in
`openspec/specs/lifecycle-query/spec.md` is unchanged. Codex reviews the code.

## What Changes

- `List` sorts the listed keys and removes repeats (`sort.Strings`, `slices.Compact`) before it reads entities.
  So participants come back in entity-ID order, each at most once, and `Offset`/`Limit` page a stable order.
- The `List` and `ListOptions.Offset` doc comments state that order. The comment in
  `query_transition_integration_test.go` that says "List promises no order" is updated to match.
- A unit test in `pkg/lifecycle/manager_test.go` lists over the existing `fakeBucket` with a snapshot that repeats a
  key and is out of key order. With `Limit` it expects the first participants by entity ID, each once. With `Offset`
  it expects the next participants, with none skipped or repeated. The oracle is the sorted set of IDs the test put
  in the bucket. On the base commit the test fails: the repeated participant comes back twice and the last one is cut.
- Examples suffice, no generated checks (`docs/testing.md`): the change has one input shape (a listing with repeats
  in some order) and one law (sorted, unique, then sliced). A fixed snapshot with a repeat and a reversed pair covers
  both the dedup and the order, under `Limit` and under `Offset`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

None. `List`'s order was unpromised. This change makes it entity-ID order, as the `AGENTS.md` § 2 rule requires, and
states it in the doc comment.
