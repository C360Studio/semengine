# operator-surface delta: refactor-363-flow-paths-composition

## MODIFIED Requirements

### Requirement: The composition projections serve the result kept at construction

`/components/flowgraph`, `/components/validate` and `/components/paths` SHALL project the result the component manager
kept at construction: `validate` the result as is, `flowgraph` its graph as JSON or `composition.Mermaid`, `paths`
`composition.FlowPaths` of its graph with counts of that answer and a timestamp. None SHALL compute a graph,
reachability or severity of its own. The flowgraph answer SHALL be stable: its JSON and Mermaid forms are a tested
contract.

#### Scenario: The flowgraph for a fixture composition

- **WHEN** `/components/flowgraph` is asked for a fixture composition, as JSON and with `format=mermaid`
- **THEN** both answers equal the checked-in files under `service/testdata/`
- **AND** the test that verifies this is `TestFlowGraphMatchesGoldenForFixtureComposition`

#### Scenario: Validation projects the kept result

- **WHEN** `/components/validate` is asked after construction
- **THEN** it returns the findings the construction-time analysis produced
- **AND** the test that verifies this is `TestFlowValidationHandlerProjectsLibraryResult`

#### Scenario: Paths projects the library's answer, counted

- **WHEN** `/components/paths` is asked for a composition whose origins reach paths of different lengths
- **THEN** `paths` and `statistics` equal hand-written values, fixed on the base before `FlowPaths` existed:
  `input_component_count` (the number of origins), `max_path_length`, `total_reachable` (the sum of path lengths)
  and `avg_path_length` (that sum over the origin count)
- **AND** the test that verifies this is `TestFlowPathsRouteServesTheLibraryAnswer`
