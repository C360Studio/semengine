# Proposal: port-logforwarderpolicy

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). The owner set the
order on 2026-10-19 (#9 comment 6270875286): `health`, `internal/logforwarderpolicy`, `component/flowgraph`, `config`,
`pkg/rulepack`, `composition`, then `service`. This change ports the second, `internal/logforwarderpolicy` (about 105
pin lines at SemStreams `8b99efe9`). `health` is another session's draft (#318).

## What Changes

- Carry `internal/logforwarderpolicy` (`policy.go`, `policy_test.go`) unchanged from the pin, with its admission-ledger
  row (`disposition: carry`). No production importer until `service`'s log-forwarder PR (design, Decision).

## Capabilities

- None changed. The contract line is the ledger row; the requirement lands with `service`.

## Impact

- New package `internal/logforwarderpolicy` (105 production lines, 54 test lines), its ledger row and a
  `docs/repository-map.md` row.
