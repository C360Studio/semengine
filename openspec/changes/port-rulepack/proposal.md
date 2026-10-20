# Proposal: port-rulepack

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). The owner set the
order on 2026-10-19 (#9 comment 6270875286): `health`, `internal/logforwarderpolicy`, `component/flowgraph`, `config`,
`pkg/rulepack`, `composition`, then `service`. The first four are merged (`config` as part 1, #322; part 2 is
draft #329). This change ports the fifth, `pkg/rulepack` (143 pin lines at SemStreams `8b99efe9`: `identity.go`
and `identity_test.go`), the rule-pack identity contract admitted by the SETUP 03B design (E5, carry).

## What Changes

- Port `pkg/rulepack` from the pin with its admission-ledger row. The design decides which of its three exported
  functions have a reader in the port set and whether the package keeps its `config` import (inventory, then design).

## Capabilities

- None changed. The contract line is the ledger row; the requirement lands with the first importer.

## Impact

- New package `pkg/rulepack`, its ledger row and a `docs/repository-map.md` row.
