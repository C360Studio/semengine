# fusion Specification

## Purpose
The fusion engine (`pkg/fusion`) turns a query into one response of nodes, relations, paths, impact and a graph
projection, keyed by what a human reads, and says what it left out. This capability states how it reads readiness from
a reply, what fails the call and what is skipped, what each label and counter means, and how the engine is built.

## Requirements

### Requirement: Fusion reads readiness from the reply

`Engine.Fuse` SHALL read no readiness status. A classified `index_not_ready` error from `Resolve`, `Entities`, or a
relations `Neighbors` call SHALL produce a response with `deferred` true, `defer_reason` `index_not_ready`, no nodes
and no miss. Any other error from `Resolve` or `Entities` SHALL be returned as the error.

#### Scenario: Resolve is not ready

- **WHEN** the retrieval client's `Resolve` returns a classified `index_not_ready` error
- **THEN** `Fuse` returns no error and a response with `deferred` true, `defer_reason` `index_not_ready`, no nodes and
  no misses (`TestDeferredOnIndexNotReady`)

### Requirement: A fatal seed read fails the call

When `Resolve` or `Entities` returns an error `errs.IsFatal` reports, such as `graph_state_reset_required` from a
prefix resolve, `Fuse` SHALL return that error and no response. `Fuse` SHALL refuse a nil context. A poisoned seed
does not reach `Fuse` as an error from `fusionnats`, which lists it as unhydrated (requirement "A poisoned seed is
unhydrated, not a failed read").

#### Scenario: A reset while hydrating seeds

- **WHEN** `Resolve` answers and `Entities` returns `graph_state_reset_required`
- **THEN** `Fuse` returns that error (`TestFatalReplyReturnsError`)

### Requirement: A failed read under an ended context returns the context's error

When a read fails, or every resolved seed is unhydrated, and the request's context has ended, `Fuse` SHALL return the
context's error and no response. A read that completed before the context ended is kept: the engine checks the
context only after a failed read, not after a successful one.

#### Scenario: The caller gives up

- **WHEN** the context is cancelled before `Entities` returns, and the read fails or every seed is unhydrated
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

### Requirement: A walk read that fails is skipped

A skipped read SHALL be any failed relation-target, paths, impact or graph read, and any failed relations `Neighbors`
read except `index_not_ready`, which defers. A fatal error, `graph_state_reset_required` included, SHALL be skipped
there like any other; only an ended context SHALL instead fail the call.

#### Scenario: A poisoned relation target

- **WHEN** `Resolve` and `Entities` answer and a relation target's read returns `graph_state_reset_required`
- **THEN** `Fuse` answers, that ref is left out and the node's `relations_truncated` is true
  (`TestFatalWalkReadSkipped`)

### Requirement: A node says when relations were left out

A node's `relations_truncated` SHALL be true when a role had more relations than its cap, a relations `Neighbors`
read was skipped, or a relation's target read was skipped or read as absent. Otherwise the key SHALL be omitted.

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

A path SHALL hold only names a lens produced. When its next node's read is skipped or reads as absent, the path SHALL
end at its last readable node with `truncated` true, and a path that would hold the seed alone SHALL not be emitted. The
response's `paths_truncated` SHALL be true when more paths existed than the cap or a path was not emitted for an unread
node.

#### Scenario: The paths cap

- **WHEN** a seed has 11 leaf paths and the cap is 10
- **THEN** the response has 10 paths and `paths_truncated` true; with exactly 10 it is omitted (`TestPathsCapLabelled`)

#### Scenario: An unreadable node on a path

- **WHEN** the third node of a path cannot be read, or reads as absent
- **THEN** the path holds the first two names and is marked `truncated`; when the second node cannot be read, no path
  is emitted and `paths_truncated` is true; no path holds an entity ID (`TestPathStopsAtUnreadNode`)

### Requirement: Impact and the graph facet say when a read was skipped

`impact.truncated` SHALL be true when the node cap was reached, a walk read was skipped, or an affected node's read
for its file was skipped. `graph.truncated` SHALL be true when an incoming counterpart's read was skipped; a
counterpart that reads as absent SHALL keep its edge without evidence and SHALL not set it.

#### Scenario: An unread file

- **WHEN** an affected node's entity read fails during the impact walk
- **THEN** `impact.truncated` is true (`TestImpactUnreadFileTruncates`)

#### Scenario: A counterpart fault

- **WHEN** an incoming counterpart's entity read fails
- **THEN** `graph.truncated` is true; when it reads as absent, the edge has no evidence and `graph.truncated` stays
  false (`TestGraphCounterpartFaultTruncates`)

### Requirement: A skipped read is logged and counted

Each skipped read SHALL increment `semengine_fusion_skipped_reads_total` with label `facet` (`relations`, `paths`, `impact`,
`graph`) when a metrics registry was given. Each `Fuse` call SHALL log one Warn line per facet that skipped any read,
naming the facet, the number skipped and the first error.

#### Scenario: One skipped relations read

- **WHEN** three relations reads are skipped in one `Fuse` call on an engine with a registry
- **THEN** one Warn line names `relations` with count 3 and the series for `relations` reads 3
  (`TestSkippedReadLoggedAndCounted`)

### Requirement: Ranking is a stable permutation

`Fuse` SHALL order hydrated entities by its score and SHALL keep resolve order among entities with equal scores; the
ordered list SHALL hold each hydrated entity exactly once.

#### Scenario: Generated inputs

- **WHEN** entities, names and queries are generated (seed recorded), with groups whose non-position signals are equal,
  built by giving their entities one shared name that matches no query and no signals
- **THEN** every ranking is the input's multiset and each such group keeps resolve order; an exact score tie keeps
  resolve order too (`TestRankIsStablePermutation`, its exact-tie case)

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

### Requirement: The NATS client refuses bad wiring

`fusionnats.New` SHALL refuse a nil requester and a negative timeout, returning an error and no client, and SHALL
treat a zero timeout as natsclient's default request timeout.

#### Scenario: Bad wiring

- **WHEN** `New` is given a nil requester or a negative timeout
- **THEN** it returns an error and no client (`TestNewRefusesNilRequester`, `TestNewRefusesNegativeTimeout`)

#### Scenario: Zero timeout

- **WHEN** `New` is given a zero timeout
- **THEN** each request passes a zero timeout to the requester, which applies its default
  (`TestNewZeroTimeoutUsesDefault`)

### Requirement: The NATS client starts nothing

The client SHALL hold no goroutine, watch or context, and SHALL read no readiness status; readiness reaches the
engine only as a reply's `index_not_ready`.

#### Scenario: No readiness import

- **WHEN** the import closure of `pkg/fusion/fusionnats` is listed
- **THEN** it holds no `graph/readiness` (`TestFusionNATSImportsNoEmbedding`), and `TestNoRetainedContext` (the
  verify guard on retained contexts) passes over the package; that it starts no goroutine or watch is held by review

### Requirement: Subjects come from the verb table

Each request SHALL go to the subject of its graph-query row in `graph.QueryVerbs()`. When the table lacks a row the
client uses, construction SHALL be refused with an error naming the verb.

#### Scenario: Subjects from the table

- **WHEN** each `RetrievalClient` method is called on a recording requester
- **THEN** each request's subject is its graph-query row's subject (`TestSubjectsComeFromVerbTable`)

#### Scenario: A missing row

- **WHEN** the verb lookup is given a table without the `batch` row
- **THEN** it refuses, naming `batch` (`TestVerbLookupNamesMissingRow`)

### Requirement: Replies are decoded as their row declares

A reply on an enveloped row SHALL be decoded as a `graph.QueryResponse` of the row's type and refused when it names
no `producer`. A reply on a bare row SHALL be decoded as the row's type. The shape SHALL never be decided from the
reply's content.

#### Scenario: A bare body on an enveloped row

- **WHEN** an enveloped row's reply is a bare body or `{}`
- **THEN** the call returns an error (`TestEnvelopedRowNeedsProducer`)

### Requirement: The batch read accounts for every requested ID

`Entities` SHALL return every distinct requested ID exactly once, in `Entities` in request order or in `Unhydrated`
with a reason, and SHALL send no request for zero IDs.

#### Scenario: Duplicates and under-reporting

- **WHEN** generated requests (duplicates included) meet generated replies that list an ID as missing, as both
  found and missing, or not at all
- **THEN** each distinct ID appears once, found ones in request order, unreported ones `unknown` (`TestReconcileTotal`)

#### Scenario: Zero IDs

- **WHEN** `Entities` is called with no IDs
- **THEN** it returns an empty `Hydration` and sends no request (`TestEntities_EmptyShortCircuits`)

### Requirement: A word search passes on what it could not read

An NL `Resolve` SHALL copy the search reply's `unreadable_records` and `failed_records` into the `Resolution`; a
symbol or prefix `Resolve` SHALL leave both 0.

#### Scenario: Word-search counts

- **WHEN** the semantic reply reports 2 unreadable and 1 failed record
- **THEN** the `Resolution` carries 2 and 1, and a symbol or prefix resolve carries 0 and 0
  (`TestSemanticCountsReachResolution`)

### Requirement: A poisoned seed is unhydrated, not a failed read

When the batch request's `RequestClassified` call returns an error with code `graph_state_reset_required`, `Entities`
SHALL send one `entity` request per distinct non-empty requested ID, list an ID whose request is refused with that
code as unhydrated `error` and one not found as unhydrated `not_found`. It SHALL never decide from an error's text.

#### Scenario: One poisoned seed

- **WHEN** the batch is refused for poison and the second of three seeds is poisoned
- **THEN** the first and third hydrate in request order and the second is unhydrated `error`
  (`TestPoisonedSeedUnhydrated`, `TestPoisonedSeedOverRealResponders`)

#### Scenario: A poisoned seed requested twice

- **WHEN** the batch is refused for poison and the poisoned ID is requested twice
- **THEN** it is read once and listed once (`TestPoisonedDuplicateReadOnce`)

### Requirement: Reading seeds alone fails on any other fault

After a poison refusal of the batch, an empty requested ID SHALL be listed as unhydrated `error` with no request,
and any other error from an `entity` request, an ended context included, SHALL be returned by `Entities`.

#### Scenario: An empty ID

- **WHEN** the batch is refused for poison and the request holds an empty ID
- **THEN** the empty ID is unhydrated `error` and no `entity` request carries it (`TestPoisonFallbackEmptyID`)

#### Scenario: Another fault while reading alone

- **WHEN** a seed read alone fails transient, or the context ends before a further request or a refused reply
- **THEN** `Entities` returns that error (`TestPoisonFallbackReturnsOtherError`, `TestPoisonFallbackEndedContext`)

### Requirement: The client's own decode refusal is returned, never retried

A refusal from the client's own check of a reply (`graph.ValidateDecodedEntityStates` on the batch reply,
`graph.UnmarshalEntityState` on an `entity` reply) SHALL be returned by `Entities`, and a refused batch reply SHALL
start no `entity` request.

#### Scenario: The client's own validation refuses

- **WHEN** the batch reply, or a per-ID reply, fails the client's own stored-value check
- **THEN** `Entities` returns that refusal, and after a refused batch reply sends no `entity` request
  (`TestOwnValidationNotRetried`)
