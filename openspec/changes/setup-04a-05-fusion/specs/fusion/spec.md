# fusion

## ADDED Requirements

### Requirement: Fusion reads readiness from the reply

`Engine.Fuse` SHALL read no readiness status. A classified `index_not_ready` error from `Resolve`, `Entities`, or a
relations `Neighbors` call SHALL produce a response with `deferred` true, `defer_reason` `index_not_ready`, no nodes
and no miss. Any other error from those calls SHALL be returned as the error.

#### Scenario: Resolve is not ready

- **WHEN** the retrieval client's `Resolve` returns a classified `index_not_ready` error
- **THEN** `Fuse` returns no error and a response with `deferred` true, `defer_reason` `index_not_ready`, no nodes and
  no misses (`TestDeferredOnIndexNotReady`)

#### Scenario: A fatal reply

- **WHEN** `Entities` returns a classified `graph_state_reset_required` error
- **THEN** `Fuse` returns that error (`TestFatalReplyReturnsError`)

### Requirement: An answer that read nothing is deferred, never a miss

When seeds resolve and none of them hydrates, the response SHALL have `deferred` true, `defer_reason`
`all_seeds_unhydrated`, every unhydrated seed listed in `unhydrated`, and no miss.

#### Scenario: All seeds unread

- **WHEN** `Resolve` returns two seeds and `Entities` returns both as unhydrated `not_found`
- **THEN** the response is deferred `all_seeds_unhydrated`, lists both handles and has no misses
  (`TestAllSeedsUnhydratedDefersWithoutMiss`)

### Requirement: The wire names its version

Every response SHALL carry `contract_version` `"2"` and no `index` key.

#### Scenario: An answered response

- **WHEN** a response is marshalled to JSON
- **THEN** `contract_version` is `"2"` and the object has no `index` key (`TestContractVersionIsTwo`)

### Requirement: A word-search answer reports what it could not read

For a natural-language resolve, the response SHALL carry the search reply's `unreadable_records` and
`failed_records`; for symbol and prefix resolves both SHALL be omitted.

#### Scenario: A failed embedding

- **WHEN** an NL resolve returns seeds with `failed_records` 1
- **THEN** the response carries `failed_records` 1 beside its nodes (`TestNLAnswerCarriesSearchCounts`)

### Requirement: Every left-out piece of an answer is labelled

Each cut-short part SHALL be labelled: a node's `relations_truncated` when a relation role passed its cap or a
neighbour or target read failed; the response's `paths_truncated` when more paths existed than its cap; a path,
marked `truncated`, ending before a node that cannot be read (a path holds only names a lens produced);
`impact.truncated` when a walk or affected-node read failed; `graph.truncated` when an incoming counterpart's read
failed.

#### Scenario: The relations cap

- **WHEN** a seed has 13 outgoing edges for one role and the cap is 12
- **THEN** its node lists 12 refs for that role and `relations_truncated` is true; with exactly 12 edges it is false
  (`TestRelationsCapLabelled`)

#### Scenario: The paths cap

- **WHEN** a seed has 11 leaf paths and the cap is 10
- **THEN** the response has 10 paths and `paths_truncated` true; with exactly 10 it is false (`TestPathsCapLabelled`)

#### Scenario: An unreadable node on a path

- **WHEN** the second node of a path cannot be read
- **THEN** the path ends at the first node, is marked `truncated`, and holds no entity ID (`TestPathStopsAtUnreadNode`)

### Requirement: Ranking is a stable permutation

`Fuse` SHALL order hydrated entities by its score and SHALL keep resolve order among entities with equal scores; the
ordered list SHALL hold each hydrated entity exactly once.

#### Scenario: Generated inputs

- **WHEN** entities, names, queries and signals are generated (seed recorded)
- **THEN** every ranking is a permutation of its input and equal scores keep resolve order
  (`TestRankIsStablePermutation`)

### Requirement: The engine is built once and registers metrics only where told

`NewEngine` SHALL refuse a nil retrieval client. Its body-failure counter SHALL be registered only on the metrics
registry passed as an option, through `metric.RegisterOrGet`, and a registration refusal SHALL be returned by
`NewEngine`. Without a registry no collector SHALL be registered anywhere. `Fuse` and `BodyResolver.ResolveBody`
SHALL refuse a nil context.

#### Scenario: No registry

- **WHEN** an engine built without a registry fails to load a body
- **THEN** the node carries its `body_reason` and the Prometheus default gatherer holds no fusion series
  (`TestCounterOnlyOnInjectedRegistry`)

#### Scenario: Two engines, one registry

- **WHEN** two engines are built on one registry and each fails one body
- **THEN** the registry's body-failure series reads 2 (`TestEnginesShareOneSeries`)

### Requirement: The store registry resolves bodies

`*storeregistry.Registry` SHALL satisfy `fusion.StoreResolver`, checked at compile time.

#### Scenario: Build

- **WHEN** `pkg/fusion`'s tests build
- **THEN** the assertion compiles, and a body stored under a registered instance resolves byte for byte
  (`TestBodyResolver_LocationIndependence`)

### Requirement: fusionvocab ranks by the parents it is given

`fusionvocab.Signals.ClassSpecificity` SHALL be the number of IRIs the `parents` function given to `New` returns for
the class, and 0 for an empty class or a nil function. `PredicateSalience` SHALL be the registered predicate's
`Weight`, 0 when unregistered. `pkg/fusion` and `pkg/fusion/fusionvocab` SHALL import no ontology package.

#### Scenario: A nil parents function

- **WHEN** `New(nil).ClassSpecificity` is called with any IRI
- **THEN** it returns 0 (`TestClassSpecificityNilParentsIsZero`)
