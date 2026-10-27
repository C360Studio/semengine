# Proposal: refactor-363-flow-paths-composition

## Why

Port-refactor #363. `ComponentManager.GetFlowPaths` and its helpers `isInputNode` and `reachableFrom`
(`service/component_manager.go:603-664`, the pin's `service/component_manager.go:1405-1466`) work out which components
are reachable over the composition graph, and they do it inside `service`. ADR-100 decision 3
(`docs/adr/100-compositions-are-validated-diagrams-are-projections.md:62-66`) makes the `composition` library the one
home for reading the analysis: "the library is the one home". The live spec says `/components/paths` "SHALL compute
nothing" (`openspec/specs/operator-surface/spec.md:273-274`), and three code and doc comments repeat that claim. All
four are untrue today. This is the second of three serial `service` port-refactors: #364 merged as #401, then this
one, then #362 (owner ruling on #363, comment 6384691294).

## What Changes

- `composition` gains `FlowPaths(graph Graph) map[string][]string`, a read-only function over the projection, shaped
  like `Mermaid(graph)`. The origin rule and the depth-first walk move into it unchanged.
- `GetFlowPaths` stays exported. Its signature and its refusal when no result is kept do not change. Its body becomes
  `composition.FlowPaths(result.Graph)`, and `isInputNode` and `reachableFrom` leave `service`.
- The `/components/paths` route keeps its body shape: `paths`, `statistics` (four counts of that answer) and
  `timestamp`. The four "computes nothing" statements are reworded so they say what the route actually does.
- Two stale "not yet ported" lines are corrected: `composition/doc.go:19` and
  `openspec/specs/composition-validation/spec.md:8`.
- A new test asserts the `/components/paths` body. Nothing asserts it today.

Observable behaviour is unchanged: for every composition, `GetFlowPaths` and the route return the same paths, in the
same order, with the same counts.

## Capabilities

- `composition-validation`: one ADDED requirement (design D1), saying what `FlowPaths` answers.
- `operator-surface`: one MODIFIED requirement (design D2), "The composition projections serve the result kept at
  construction", which now states what `paths` serves.

## Non-goals

- Merging the three spellings of "this input is fed from outside" (design D3).
- Changing which nodes count as origins.
- Removing `GetFlowPaths`.
- Moving the route's counts into `composition`.
- #362, which follows this PR.
