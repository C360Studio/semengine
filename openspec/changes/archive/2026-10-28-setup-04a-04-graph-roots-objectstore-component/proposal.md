# Proposal: setup-04a-04-graph-roots-objectstore-component

## Why

Change 4 of epic #9 ports the graph roots one package per pull request, in the owner's order (#9 comment 6386856657,
item 4: "`storage/objectstore` (Store, then Component)"; #9 comment 6386556850, item 1: the split "needs none", no
waiver). The Store merged as PR #432 (`b3b9031`). This change is the second half: the objectstore Component, which binds
NATS or JetStream inputs, stores message bodies through the Store, emits a `StoredMessage` carrying the reference, and
offers its Store to the store registry (ADR-063). The owner's bar: "graph is in many ways our secret sauce. it needs to
be solid… i would rather not bring in a pkg that causes instant debt". Tier 0 only (#9 comment 6386603659); the files
have no provider code (inventory 8).

## What Changes

- Port `component.go`, `register.go` and `stored_message.go` (1,497 pin lines; inventory 10) onto the tree's shell
  (`component.Lifecycle`, ADR-108) and natsclient, with the corrections of design D2-D9.
- The Component stores the `body` of `ContentStorable` messages through `StoreContent` and emits a `StoredMessage`
  whose reference opens to that body (ADR-063). Other messages are refused, counted and logged (design D3; owner
  question 1).
- A refusal is never silent: on a JetStream input it is terminated; on a core-NATS input it is dropped; on both it is
  logged and counted in `Health().ErrorCount` (design D4; owner ruling 2).
- Ending the context given to `Start` stops nothing: both lanes keep storing (ADR-113; design D5). The pin lost every
  later write (inventory probe 2).
- Configuration is `bucket_name` and `ports`, decoded strictly; `data_cache` and any other key is refused naming it.
  The only output is `stored`; the `events` port and `Event` are not carried (no reader anywhere).
- The Store stays as merged: `Store.Store`, `DefaultKeyGenerator`, `Config.Ports`, `KeyGenerator`,
  `MetadataExtractor` and `DefaultConfig` are not carried, and `storage.KeyGenerator` and `storage.MetadataExtractor`
  leave `storage` (no reader; design D6).
- The Component stamps and registers `StorageInstance = "objectstore"`, as at the pin; a second Component in one
  process is refused at `Start` (design D7; owner ruling 3).
- Tests through `natsfixture` (#195), no test-only fields on `Component` (design D8), the lifecycle suite plus a
  behaviour check for the Start context (design D5).

## Capabilities

- Modified `object-store` (added requirements for the Component and `StoredMessage`).

## Impact

- `storage/objectstore`: `component.go`, `register.go`, `stored_message.go` and tests; `storage/storage.go` and
  `storage/doc.go` (two interfaces and their examples removed); `internal/harness/contract/consumerpolicy_test.go`
  (allowlist gains the Component's file, #422); ledger rows `storage/objectstore`, `storage`; ADR-063 port note.
- `natsclient`: one exported method, `Client.WaitConsumerHandlers`, by owner ruling (PR #437 comment 6467701324;
  one-package exception for this method only, #159); `transport-client` spec and ADR-112 § 4 gain its contract;
  ledger row `natsclient` (design D11).
- Consumers: semteams and semsource register the Component with `{"bucket_name": …}` and default ports; that config
  still builds. A config carrying `data_cache` or an `events` output now fails at boot naming it; none does
  (inventory 4). No consumer publishes to `storage.objectstore.write` today.
- Retires #33 and #443. Advances #195 and #196 (multi-package trackers). Leaves #280 (item 3) and #281 (in
  `component`) open; #441 adopts the new method afterwards.
