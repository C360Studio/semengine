# Proposal: one-exclusive-address-check

## Why

Two places check that no two ports claim one exclusive network address: `component.Registry`, when it admits a
component, and `internal/flowgraph`, when it builds a graph from the admitted declarations (#324). An exclusive
address is one only one port may hold, such as a TCP listen address; today only `NetworkPort` is exclusive. At the
SemStreams pin, flowgraph never sees two components on one address, because the registry refuses the second one
first. The registry's check has no test, and it lets one component claim one address on two of its own ports, which
flowgraph refuses. The owner ruled on 2026-10-19 that this case is refused (#324 comment 6278113073).

## What Changes

- Test the registry's check: a second component on a taken address is refused, naming the holder.
- Refuse one component claiming one exclusive address on two of its ports, through both `Registry.Declare` and
  `Registry.CreateComponent`. The text is
  `resource conflict: tcp:0.0.0.0:8080 already used by another port of component 'gateway'`.
- Delete flowgraph's copy of the check (`validateNetworkPorts`, `exclusivePortRefs`, `portInfo.exclusive`) and its
  tests.
- Delete what the new refusal makes dead in `component`: `compactStrings` and the registry check's skip of the
  component's own name.
- Update the docs and both ledger rows to match.

Retires #324. The protocol-name defect found in the design (#328) is out of scope.

## Capabilities

- `component-registration`: ADDED "Admission refuses a second claim on an exclusive resource".

## Impact

- `component/registry.go` and its tests; `internal/flowgraph` and its tests; `component/README.md`,
  `internal/flowgraph/doc.go`, `internal/flowgraph/README.md`, `docs/repository-map.md`,
  `docs/admission-ledger.yaml`. No exported signature changes.
- A component that declares one exclusive address on two ports, accepted by the registry today, is refused. No
  production declaration at the pin or in the consumers does this.
