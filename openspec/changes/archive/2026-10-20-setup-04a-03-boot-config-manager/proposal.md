# Proposal: setup-04a-03-boot-config-manager

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). The owner ruled
on 2026-10-19 (#9 comment 6274501134) that `config` ports as two pull requests. Part 1, the `Config` type, loading,
validation and the stream declarations, merged as #322. This change is part 2: the KV-backed `config.Manager`, the
lifecycle owner that publishes the configuration to a NATS KV bucket and watches it for changes, with its key
families and bucket naming (about 1,700 pin lines at SemStreams `8b99efe9`, under the ~3,000-line cap). `service`
needs it in the tree before its component manager ports.

## What Changes

- Port `manager.go`, `key_family.go` and `bucket_name.go` from the pin into `config`, with their tests and
  admission-ledger rows. Dispositions are set by the design after the inventory.
- Bring `config` under the 80% coverage gate (`scripts/cover-check.sh`), as part 1's design recorded.

## Capabilities

- New: `config-manager` (`specs/config-manager/spec.md`): Start/Stop under ADR-113, the bucket name, platform
  identity, arbitration, fail-closed Start, component writes and key families.

## Impact

- `config` gains `Manager`; ledger rows and the `docs/repository-map.md` row are updated.
