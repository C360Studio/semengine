# fusion

## ADDED Requirements

### Requirement: Fusion reads readiness from the reply

`Engine.Fuse` SHALL read no readiness status. A classified `index_not_ready` error from `Resolve`, `Entities`, or a
relations `Neighbors` call SHALL produce a response with `deferred` true, `defer_reason` `index_not_ready`, no nodes
and no miss. Any other error from `Resolve` or `Entities` SHALL be returned as the error.

#### Scenario: Resolve is not ready

- **WHEN** the retrieval client's `Resolve` returns a classified `index_not_ready` error
- **THEN** `Fuse` returns no error and a response with `deferred` true, `defer_reason` `index_not_ready`, no nodes and
  no misses (`TestDeferredOnIndexNotReady`)

### Requirement: A fatal reply or an ended context fails the call

When any read `Fuse` makes returns an error `errs.IsFatal` reports, such as `graph_state_reset_required`, or when the
request's context has ended, `Fuse` SHALL return that error and no response, whichever facet made the read. `Fuse`
SHALL refuse a nil context.

#### Scenario: A reset during the relations walk

- **WHEN** `Resolve` and `Entities` answer and a relations `Neighbors` call returns `graph_state_reset_required`
- **THEN** `Fuse` returns that error (`TestFatalReplyReturnsError`)

#### Scenario: The caller gives up

- **WHEN** the context is cancelled before `Entities` returns
- **THEN** `Fuse` returns the context's error (`TestEndedContextReturnsError`); a nil context is refused
  (`TestFuseRefusesNilContext`)

### Requirement: An answer that read nothing is deferred, never a miss

When seeds resolve and none of them hydrates, the response SHALL have `deferred` true, `defer_reason`
`all_seeds_unhydrated`, every unhydrated seed listed in `unhydrated`, and no miss.

#### Scenario: All seeds unread

- **WHEN** `Resolve` returns two seeds and `Entities` returns both as unhydrated `not_found`
- **THEN** the response is deferred `all_seeds_unhydrated`, lists both handles and has no misses
  (`TestAllSeedsUnhydratedDefersWithoutMiss`)

### Requirement: The wire names its version

Every response SHALL carry `contract_version` `"2"` and no `index` key, and `defer_reason` SHALL be empty,
`index_not_ready` or `all_seeds_unhydrated`.

#### Scenario: An answered response

- **WHEN** a response is marshalled to JSON
- **THEN** `contract_version` is `"2"` and the object has no `index` key (`TestContractVersionIsTwo`)

### Requirement: A word-search answer reports what it could not read

Every response built after a natural-language `Resolve` answered SHALL carry that reply's `unreadable_records` and
`failed_records`, each omitted when 0. Responses after a symbol or prefix resolve SHALL omit both.

#### Scenario: A failed embedding beside the nodes

- **WHEN** an NL resolve returns seeds with `failed_records` 1
- **THEN** the response carries `failed_records` 1 beside its nodes (`TestNLAnswerCarriesSearchCounts`)

#### Scenario: A miss while records were unreadable

- **WHEN** an NL resolve returns no seeds with `unreadable_records` 3
- **THEN** the miss response carries `unreadable_records` 3 (`TestNLMissCarriesSearchCounts`)

#### Scenario: A symbol resolve

- **WHEN** the lens picks symbol mode
- **THEN** the response has neither key (`TestSymbolAnswerOmitsCounts`)

### Requirement: Edges are ordered before any cap

Every `Neighbors` result SHALL be sorted by predicate, then target, and deduplicated before a relations, paths, impact
or graph cap is applied, so the same graph gives the same cut whatever order a reply listed its edges in.

#### Scenario: Two reply orders

- **WHEN** one client lists 13 edges in one order and another lists them reversed
- **THEN** the two nodes hold the same 12 refs (`TestEdgesSortedBeforeCap`)

### Requirement: A node says when relations were left out

A node's `relations_truncated` SHALL be true when a role had more relations than its cap, a relations `Neighbors`
read was skipped, or a relation's target could not be read or read as absent. A skipped read is one whose error is
neither fatal, `index_not_ready`, nor an ended context. Otherwise the key SHALL be omitted.

#### Scenario: The relations cap

- **WHEN** a seed has 13 outgoing edges for one role and the cap is 12
- **THEN** its node lists 12 refs for that role and `relations_truncated` is true; with exactly 12 edges the key is
  omitted (`TestRelationsCapLabelled`)

#### Scenario: A skipped neighbour read

- **WHEN** a relations `Neighbors` call returns an unclassified transport error
- **THEN** the node ships with `relations_truncated` true (`TestRelationsFaultLabelled`)

#### Scenario: An unreadable target

- **WHEN** a relation's target entity read fails or finds nothing
- **THEN** that ref is left out and `relations_truncated` is true (`TestRelationsUnreadTargetLabelled`)

### Requirement: Paths hold only names and say when one was cut

A path SHALL hold only names a lens produced. When its next node cannot be read it SHALL end at its last readable node
with `truncated` true, and a path that would hold the seed alone SHALL not be emitted. The response's
`paths_truncated` SHALL be true when more paths existed than the cap or a path was not emitted for an unread node.

#### Scenario: The paths cap

- **WHEN** a seed has 11 leaf paths and the cap is 10
- **THEN** the response has 10 paths and `paths_truncated` true; with exactly 10 it is omitted (`TestPathsCapLabelled`)

#### Scenario: An unreadable node on a path

- **WHEN** the third node of a path cannot be read
- **THEN** the path holds the first two names and is marked `truncated`; when the second node cannot be read, no path
  is emitted and `paths_truncated` is true; no path holds an entity ID (`TestPathStopsAtUnreadNode`)

### Requirement: Impact and the graph facet say when a read was skipped

`impact.truncated` SHALL be true when the node cap was reached, a walk read was skipped, or an affected node's read
for its file was skipped. `graph.truncated` SHALL be true when an incoming counterpart's read was skipped; a
counterpart that reads as absent SHALL keep its edge without evidence and SHALL not set it. In the paths, impact and
graph walks an `index_not_ready` error counts as a skipped read.

#### Scenario: An unread file

- **WHEN** an affected node's entity read fails during the impact walk
- **THEN** `impact.truncated` is true (`TestImpactUnreadFileTruncates`)

#### Scenario: A counterpart fault

- **WHEN** an incoming counterpart's entity read fails
- **THEN** `graph.truncated` is true; when it reads as absent, the edge has no evidence and `graph.truncated` stays
  false (`TestGraphCounterpartFaultTruncates`)

### Requirement: A skipped read is logged and counted

Each skipped read SHALL log one Warn line on the engine's logger naming the facet and the read, and SHALL increment
`fusion_skipped_reads_total` with label `facet` (`relations`, `paths`, `impact`, `graph`) when a metrics registry
was given.

#### Scenario: One skipped relations read

- **WHEN** one relations `Neighbors` read is skipped on an engine with a registry
- **THEN** one Warn line names `relations` and the series for `relations` reads 1 (`TestSkippedReadLoggedAndCounted`)

### Requirement: Ranking is a stable permutation

`Fuse` SHALL order hydrated entities by its score and SHALL keep resolve order among entities with equal scores; the
ordered list SHALL hold each hydrated entity exactly once.

#### Scenario: Generated inputs

- **WHEN** entities, names and queries are generated (seed recorded), with equal-score groups built by giving their
  entities one shared name that matches no query and no signals
- **THEN** every ranking is the input's multiset and each equal-score group keeps resolve order
  (`TestRankIsStablePermutation`)

### Requirement: The engine is built once with its dependencies

`NewEngine` SHALL refuse a nil retrieval client and a nil logger. Its counters SHALL be registered only on the
registry passed as an option, through `metric.RegisterOrGet`, and a registration refusal SHALL be returned by
`NewEngine`. Without a registry no collector SHALL be registered anywhere.

#### Scenario: Bad wiring

- **WHEN** `NewEngine` gets a nil client, a nil logger, or a registry already holding a different collector under the
  counter's name
- **THEN** it returns an error and no engine (`TestNewEngineRefusesNilClient`, `TestNewEngineRefusesNilLogger`,
  `TestRegistrationRefusalReturned`)

#### Scenario: No registry

- **WHEN** an engine built without a registry fails to load a body
- **THEN** the node carries its `body_reason` and the Prometheus default gatherer holds no fusion series
  (`TestCounterOnlyOnInjectedRegistry`)

#### Scenario: Two engines, one registry

- **WHEN** two engines are built on one registry and each fails one body
- **THEN** the registry's body-failure series reads 2 (`TestEnginesShareOneSeries`)

### Requirement: The store registry resolves bodies

`*storeregistry.Registry` SHALL satisfy `fusion.StoreResolver`, checked at compile time, and
`BodyResolver.ResolveBody` SHALL refuse a nil context.

#### Scenario: Build

- **WHEN** `pkg/fusion`'s tests build
- **THEN** the assertion compiles, a body stored under a registered instance resolves byte for byte
  (`TestBodyResolver_LocationIndependence`), and a nil context is refused (`TestResolveBodyRefusesNilContext`)

### Requirement: fusionvocab ranks by the parents it is given

`fusionvocab.Signals.ClassSpecificity` SHALL be the number of IRIs the `parents` function given to `New` returns for
the class, and 0 for an empty class or a nil function. `PredicateSalience` SHALL be the registered predicate's
`Weight`, 0 when unregistered. `pkg/fusion` and `pkg/fusion/fusionvocab` SHALL import no ontology package.

#### Scenario: A nil parents function

- **WHEN** `New(nil).ClassSpecificity` is called with any IRI
- **THEN** it returns 0 (`TestClassSpecificityNilParentsIsZero`)
