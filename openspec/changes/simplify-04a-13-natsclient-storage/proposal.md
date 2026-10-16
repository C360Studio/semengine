# Proposal: simplify-04a-13-natsclient-storage

## Why

PR 6a of epic #223. The owner cut PR 6 (`natsclient`) into parts on 2026-10-16 (#223 comment 6225664166): 6a moves the
storage report code out of `natsclient` into its own package and settles #85.

`natsclient` connects to NATS. About a third of its non-test code (3,415 lines in seven `storage_*.go` files)
does something else: it lists the account's streams, derives growth and pressure, publishes a report to the
`STORAGE_REPORT` bucket, and watches that bucket (#222 part a). Separately, an observer of the report cannot learn that
the account row was removed, so a metrics surface keeps the last account forever (#85).

## What Changes

- The seven files and their tests move to `natsclient/storagereport`. Behaviour does not change.
- Nine names lose the package name they now repeat: `Publisher`, `PublisherConfig`, `NewPublisher`, `Consumer`,
  `ConsumerConfig`, `NewConsumer`, `Observer`, `Snapshot`, `ResourceKey`.
- `Client.AccountStreamLister` becomes `storagereport.AccountStreamLister(client)`, which resolves the client's
  JetStream context on each call, as the method value did.
- 24 exported names that nothing outside the package reads become unexported (design, Exports).
- `Observer` gains `ForgetAccount(row AccountReport)`. The consumer calls it when the account row is deleted or
  purged, or a new watch no longer has it. The Warn log that declared the gap goes.

## Capabilities

- New: `storage-report` (the moved requirement, with #85 resolved; the nil-context refusal of `Collect` and
  `Publish`).
- Modified: `transport-client` (the storage report requirement is removed from it).

## Impact

- Files: `natsclient/storage_*.go` and tests → `natsclient/storagereport/`; `natsclient/nil_context_siblings_test.go`;
  `graph/kvcatalog` (two comments, three test files: imports and names only); `scripts/cover-check.sh`,
  `internal/harness/contract/cover_test.go` (one coverage target); `docs/admission-ledger.yaml`,
  `docs/repository-map.md`.
- Breaking at cutover (compiler-found): the import path, nine renamed and 24 unexported names, `AccountStreamLister`,
  and `ForgetAccount` on any observer. No consumer imports any of them (inventory § 4).
- Closes #85. Part a of #222; #222 stays open for 6b and 6c. Addresses #223.
