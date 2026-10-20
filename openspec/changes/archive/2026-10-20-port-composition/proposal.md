# Proposal: port-composition

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). The owner set the
order on 2026-10-19 (#9 comment 6270875286): `health`, `internal/logforwarderpolicy`, `component/flowgraph`, `config`,
`pkg/rulepack`, `composition`, then `service`. The first five are merged. This change ports the sixth, `composition`
(914 pin lines at SemStreams `8b99efe9`). It is the one validator that judges whether a set of components is wired
correctly, run offline over a configuration file and, once `service` lands, at boot (pin ADR-100). The component
manager in `service` imports it, so it lands before that PR.

## What Changes

- Port `composition` from the pin. Its exclusive-resource check uses the component registry's clash rule through one
  new method, `component.Declaration.ConflictWith`, which the registry's admission check also calls (#338).
- Remove `component.Declaration.ExclusiveResources`, which nothing reads once composition stops comparing its text
  (owner ruling 3, #9 comment 6295357983).
- Drop `AssertValid`, the pin's test helper, which imports Go's `testing` package from production code. Products call
  `composition.Validate` and check that the error list is empty (owner ruling 1).
- `composition.Analyze` refuses a stream whose subjects would capture a reserved graph request subject. This is
  composition's share of #16, which rides here as the plan says (owner ruling 2).
- A test validates `config/example_config.json`. The file is rewritten to name factories that exist in SemEngine
  (owner ruling 4).
- Carry pin ADR-100 into `docs/adr`.

## Capabilities

- New capability `composition-validation`: the validator, its closed findings vocabulary, the catalog and projection
  views, and the example-config check.
- Modified `graph-transport-boundary`: no stream captures a reserved request subject.
- Modified `component-registration`: one exported check names a declaration's first clash with another.

## Impact

- New package `composition` (about 900 production lines), its `repair-before-port` ledger row, a
  `docs/repository-map.md` row, a `scripts/cover-check.sh` target, and `docs/adr/100-...md` with its ledger row.
- `component`: one exported method added and one exported field removed; the `component` ledger row is edited.
- `config/example_config.json` and its loader test are rewritten.
