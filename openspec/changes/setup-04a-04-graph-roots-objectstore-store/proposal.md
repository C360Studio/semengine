# Proposal: setup-04a-04-graph-roots-objectstore-store

## Why

Change 4 (`setup-04a-04-graph-roots`) of epic #9 ports the graph roots, one package per pull request, in the order
the owner set on 2026-10-24 (#9 comment 6386856657). Item 4 is "`storage/objectstore` (Store, then Component)", and
the same day's ruling (#9 comment 6386556850, item 1) says objectstore "splits into the Store, then the Component, and
needs none" (no size waiver). This change is the first of those two pull requests: the `Store`, the NATS ObjectStore
backend behind `storage.Store` and `storage.StreamableStore`. SemSource builds it directly for its verbatim body
store (four sites at `e4febc0d`), and the Component (pull request 5b) wraps it. The owner's bar: "graph is in many
ways our secret sauce. it needs to be solid… i would rather not bring in a pkg that causes instant debt". The
package has no external-provider code, so the tier-0 ruling (#9 comment 6386603659) removes nothing.

## What Changes

- Port the Store half of `storage/objectstore`: `Config` (`bucket_name`, instance name, logger), `NewStoreWithConfig`,
  `NewStoreWithConfigAndMetrics`, `Put`, `Get`, `Open`, `List`, `Delete`, `StoreContent`, `InstanceName`, the
  no-lifecycle-retention boot guard, and its metrics.
- Make the Store keep the `storage.Store` contract it implements, which the pin breaks: `List` returns sorted keys and
  an empty list on an empty bucket; `Delete` of a missing key succeeds; `Open` reports a missing object as
  `storage.ErrObjectNotFound`, as `Get` does; every exported call refuses a nil context.
- `StoreContent` writes the payload's body as raw bytes and returns a reference that opens to it, as ADR-063's read
  contract requires; the pin wrote and referenced a JSON envelope. It refuses binary fields (no implementer exists),
  and its keys carry a per-call nonce.
- Not carried with it: the envelope types `StoredContent` and `BinaryRef`, which nothing then writes or reads.
- Metrics register through `metric.RegisterOrGet` under `semengine`; the two gauges nothing sets are not carried.
- Not carried (no reader at the pin or in a consumer): `FetchContent`, `FetchBinary`, `GetMetadata`, `SetDecoder`,
  the `StoredContent` role helpers. By owner ruling (PR #432 comment 6446982969): the in-memory data cache
  (`Config.DataCache`, which would expose `internal/cache`) and `Close` are not carried.
- Deferred to pull request 5b with the Component: `Store.Store`, `DefaultKeyGenerator`, `Config.Ports`,
  `KeyGenerator`, `MetadataExtractor`, `DefaultConfig`, `StoredMessage`, `Register`, `RegisterPayloads`.
- Carry SemStreams ADR-063 (store substrate and resolver) into `docs/adr/` with a port note; fix `storage/doc.go` and
  `storage/storage.go` examples (#280 item 1).

## Capabilities

- New `object-store`.

## Impact

- New `storage/objectstore` (Store half): about 1,845 pin lines carried (whole `config.go` and `doc.go` counted
  here), about 1,000 entering the tree. Pull request 5b carries the other about 1,500. Under the one-package bound
  (#159); no waiver.
- No new subject, bucket, stream or config key. New exported surface: none beyond the pin's (design D3).
- Consumers: SemSource compiles unchanged (it uses `Config{BucketName, InstanceName}`, `NewStoreWithConfig`, `Put`,
  and the store as `storage.Store`). Behaviour changes it can see: `List` and `Delete` now follow `storage.Store`'s
  doc; no cache in front of `Get` (it sets none today).
- Retires #321 (ADR-063 carried). Advances #280 (item 1 done; item 3 stays), #195. #33, #196 and #281's objectstore
  instance move to pull request 5b (design D13).
