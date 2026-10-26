# Tasks: refactor-363-flow-paths-composition

Port-refactor #363. Behaviour is preserved, so the oracle is that the named tests pass on the base and after each task.

- A task's new tests that compile on the base must pass on the base before its production change; record it in
  `notes/<task>.md`.
- A test that cannot compile on the base records the compile failure instead.
- Each task ends only when `task verify` passes on a clean tree at its commit (#171).
- Review stops (Codex) come after the design (1.1) and after the code (2.4).

## 1. Design

- [ ] 1.1 Inventory and design pass review and are accepted by the owner. Hold: Codex design review (APPROVE on the
  design commit) and the owner's acceptance, recorded on PR #413. Gate: `task spec:check` and `task docs:check`.

## 2. Refactor

- [ ] 2.1 Tests first, the route body (design D2): add `TestFlowPathsRouteServesTheLibraryAnswer` to
  `service/component_manager_paths_test.go`. Use a composition whose origins reach paths of different lengths, and
  assert `paths`, the four `statistics` values (hand-written; a non-integer average) and an RFC 3339 `timestamp`. It
  passes on the base with no production change (`notes/2.1.md`). Gate: `go test -race ./service/` and `task verify`.
- [ ] 2.2 `composition.FlowPaths` (design D1), in `composition/flowpaths_test.go` and `composition/flowpaths.go`.
  - Tests first: `TestFlowPathsKeysEachOrigin`, `TestFlowPathsWalksDepthFirstInEdgeOrder`,
    `TestFlowPathsVisitsEachNodeOnce` and `TestFlowPathsOverAnEmptyGraph`, over hand-built `Graph` values.
  - Record the base compile failure in `notes/2.2.md`.
  - Then `FlowPaths`, its origin rule and its walk, moved from `service/component_manager.go:626-664` with their doc
    comments adapted.
  - Gate: `go test -race ./composition/` and `task verify`.
- [ ] 2.3 `service` projects it (design D2), in `service/component_manager.go` and `service/component_manager_http.go`.
  - `GetFlowPaths` returns `composition.FlowPaths(result.Graph)`, keeping its nil-result refusal and doc (reworded).
  - `isInputNode` and `reachableFrom` are deleted, and the `types` import goes if nothing else reads it.
  - Reword the field comment `:53-56` and the handler comment `service/component_manager_http.go:623-625`, so neither
    says the route recomputes nothing: it serves `composition`'s answer, counted.
  - Every test named in D1 and D2 passes unchanged.
  - Gate: `go test -race ./service/ ./composition/` and `task verify`.
- [ ] 2.4 Sensitivity (`notes/2.4.md`): report each mutant as detected (naming the test) or as a survivor.
  - Remove each of the three origin arms in turn (`TestFlowPathsKeysEachOrigin`, `TestFlowPathsTraverseTheRetainedGraph`).
  - Append a node after its children instead of before (`TestFlowPathsWalksDepthFirstInEdgeOrder`).
  - Return nil for an empty graph (`TestFlowPathsOverAnEmptyGraph`, `TestComponentManagerCompositionProjection`).
  - `GetFlowPaths` returns an empty map (`TestFlowPathsTraverseTheRetainedGraph`).
  - The handler reports `max_path_length` as the average (`TestFlowPathsRouteServesTheLibraryAnswer`).
  - Use `task mutate:check`.
  - Hold: Codex code review (APPROVE on the head after 2.4, citing that commit and a local `task verify`).

## 3. Docs, ledger and archive

- [ ] 3.1 Docs and ledger (technical writer):
  - `service/README.md:337-338`: `flowgraph` and `validate` serve the kept result; `paths` serves
    `composition.FlowPaths` of its graph, with counts of that answer; `GetFlowPaths` is the same answer as a call.
  - `composition/doc.go`: line 19's "not yet ported" sentence states that the component manager runs `Analyze` at
    boot, and the doc names `FlowPaths` beside `Mermaid`.
  - `docs/admission-ledger.yaml`: the three edits in design "Ledger".
  - Gate: `task docs:check`, `task ledger:check`.
- [ ] 3.2 Sync the two spec deltas into `openspec/specs/`.
  - Correct the `composition-validation` Purpose (`openspec/specs/composition-validation/spec.md:8`): the component
    manager runs the judgment at boot; name `FlowPaths` beside `Mermaid`.
  - Archive as the last content commit.
  - Gate: `task spec:check`, `task docs:check`.
