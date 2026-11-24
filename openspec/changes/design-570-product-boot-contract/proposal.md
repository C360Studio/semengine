# Proposal: design-570-product-boot-contract

Approved-by:

Track: `design`. Capabilities: `service-lifecycle` (added), `config-manager` (added). Packages: `service`; docs.

## Why

A product that writes its own `main` around SemEngine has to know about a dozen facts that no API tells it. Examples:
call `EnsureStreams`, register the vocabulary and the payload types, give the right ports, and read the platform ID
`config.Manager` minted rather than the one in the file. The composed smoke test learned them the hard way, and
SemSource, which already pins `v0.1.0-alpha.7`, carries its own code for several. Most of these facts are already
loud: skipping them gives a boot error. One is silent. A `main` that builds `deps.Platform` from the configuration it
loaded, not from the one `config.Manager` set up, boots with no error, and its entities then carry the
un-minted ID. The first-read examples in `service/doc.go` and `service/README.md` point that way. Owner, 2026-11-12:
"this design read is wanted before the port continues". Inventory: `inventory.md` (INVENTORY PASS, round 2).

## What changes

- A new guide, `docs/boot.md`, "Booting SemEngine from your own main". It gives the steps in order, says which
  configuration copy each step reads, and says what goes wrong if a step is skipped. It is linked from
  `docs/contract.md`, `service/README.md` and `config/README.md`.
- The component manager refuses a `deps.Platform` that names a different org or platform ID than the one
  `config.Manager` set up, naming both. `ConfigureFromServices` returns that error and composes nothing.
- The `config-manager` spec states the shape of the `platform_identity` record, which processes outside SemEngine
  read to learn the platform ID. The code and its test already do this.
- The boot examples in `service/doc.go` and `service/README.md`, and the sketch in `docs/contract.md`, are corrected
  to use the configuration `config.Manager` set up.

## Non-goals

- A kernel-picked port and a reader for the bound address (left with #614, owner ruling 2026-11-23).
- The arbitration defects #646 and #647.
- Restarting a manager in-process (ADR-095).
- Promising a reconnect time, which is nats.go's (ADR-112).
- An exported reader for the identity record.
- A boot helper function.
- Reading the vocabulary lazily.
- Edits to sister repositories.

## Impact

There are no new exports. There is one new boot refusal: a `main` passing the un-minted platform ID now fails at
`ConfigureFromServices`. SemSource already passes the right one (`cmd/semsource/run.go:398`). semboids will meet the
refusal when it moves to SemEngine. Three service test fixtures change. One PR (#739), `Closes #570`.
