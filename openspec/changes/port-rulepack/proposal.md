# Proposal: port-rulepack

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). The owner set the
order on 2026-10-19 (#9 comment 6270875286): `health`, `internal/logforwarderpolicy`, `component/flowgraph`, `config`,
`pkg/rulepack`, `composition`, then `service`. The first four are merged (`config` as part 1, #322; part 2 is
draft #329). This change ports the fifth, `pkg/rulepack` (143 pin lines at SemStreams `8b99efe9`: `identity.go`
and `identity_test.go`), the rule-pack identity contract admitted by the SETUP 03B design (E5, carry).

## What Changes

- Port `pkg/rulepack` from the pin as `adapt`, with its admission-ledger row: keep `ValidateID`, `PackIDCharset` and
  `MaxPackIDBytes`; check the alphabet from `PackIDCharset` alone; reword text naming the retired owner key; drop
  `ValidateConfig` and `ValidateRuntimeUpdate`, which nothing in the port set reads (design, owner question Q1).

## Capabilities

- If the owner answers the design's Q2 yes: new capability `rule-pack-identity` (the pack ID grammar and bound).
  Otherwise none; the contract line is the ledger row.

## Impact

- New package `pkg/rulepack`, its ledger row and a `docs/repository-map.md` row.
