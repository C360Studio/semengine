# Proposal: port-composition

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). The owner set the
order on 2026-10-19 (#9 comment 6270875286): `health`, `internal/logforwarderpolicy`, `component/flowgraph`, `config`,
`pkg/rulepack`, `composition`, then `service`. The first five are merged. This change ports the sixth, `composition`
(914 pin lines at SemStreams `8b99efe9`). The component manager in `service` imports it, so it lands before that PR.

## What Changes

- Port `composition` from the pin with its admission-ledger row. Its exclusive-resource check adopts the component
  registry's clash rule (`claimsClash` in `component/registry.go`) instead of comparing `ExclusiveResources` text
  (#338); how the rule is exposed to `composition` is this change's design (inventory, then design).

## Capabilities

- To be named by the design.

## Impact

- New package `composition`, its ledger row and a `docs/repository-map.md` row; possibly a new exported surface in
  `component` for the clash rule.
