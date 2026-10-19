# Proposal: setup-04a-03-boot-config

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159), in the order the
owner set on 2026-10-19 (#9 comment 6270875286). `config` is fourth. At 5,193 pin lines it is over the ~3,000-line
cap, so the owner ruled on 2026-10-19 (#9 comment 6274501134) that it ports as two pull requests. This change is the
first: the `Config` type, its loading and validation, and the JetStream stream declarations and provisioning that
`Config.Validate` depends on, waived by count at about 3,150 pin lines. The KV-backed `config.Manager` is the second
pull request.

## What Changes

- Port `config.go`, `security.go`, `streams.go`, `stream_bounds.go`, `stream_drift.go` and `doc.go` from SemStreams
  `8b99efe9`, with their admission-ledger rows. Dispositions are set by the design after the inventory.
- Drop `helpers.go`, `env.go` and `minimal_config.go` as dead surface: none has a non-test caller at the pin.

## Capabilities

- To be named by the design.

## Impact

- New package `config` (without `Manager`), its ledger rows and a `docs/repository-map.md` row.
