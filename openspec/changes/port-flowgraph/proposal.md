# Proposal: port-flowgraph

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). The owner set the
order on 2026-10-19 (#9 comment 6270875286): `health`, `internal/logforwarderpolicy`, `component/flowgraph`, `config`,
`pkg/rulepack`, `composition`, then `service`. This change ports the third, `component/flowgraph` (about 1,324 pin
lines at SemStreams `8b99efe9`). `health` is another session's draft (#318); `internal/logforwarderpolicy` merged
in #319.

## What Changes

Claim skeleton. The design fills this section once the inventory (shape sweep, pin ADR list, open-issue listing)
passes review.

## Capabilities

- To be decided by the design.

## Impact

- New package `component/flowgraph` and its admission-ledger row.
