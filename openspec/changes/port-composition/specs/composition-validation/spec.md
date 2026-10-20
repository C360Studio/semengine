# composition-validation

## ADDED Requirements

### Requirement: Validation is a pure function with a closed vocabulary

`composition.Validate(catalog, cfg)` SHALL be a deterministic function of a `*component.Registry` and a `*config.Config`
that performs no I/O, opens no connection and constructs no component; it SHALL return an error only for a nil
argument, and every judgment SHALL be a finding. `composition.Analyze(declarations, streams)` SHALL be the graph-level
half of the same function, over declarations and the configuration's explicit `streams`. The result SHALL carry
`status` (`errors` when any error finding exists, else `warnings` when any warning exists, else `valid`), `errors`,
`warnings` and `graph`, with every array non-nil. Each finding SHALL carry `type`, `severity`, a non-empty `component`,
an optional `port`, a non-empty `message` and a non-nil `suggestions`.

`type` SHALL be one of the exported constants `config_invalid`, `unknown_component`, `component_type_mismatch`,
`component_config_invalid`, `port_declaration_error`, `exclusive_resource_conflict`, `connection_pattern_error`,
`stream_requirement`, `disconnected_node`, `orphaned_port`, `interface_mismatch`, `missing_interface`,
`empty_composition` and `reserved_subject_capture`. Severity SHALL be warning for `disconnected_node`,
`missing_interface`, `empty_composition` and for every `orphaned_port` except a required stream input with no publisher.
Severity SHALL be error for every other type.

Two enabled components whose exclusive claims clash under `component.Declaration.ConflictWith` SHALL yield one
`exclusive_resource_conflict` error on the component later in instance-name order, with that method's message. A
component whose own claims clash SHALL yield `port_declaration_error`. A JetStream input SHALL NOT be a
`stream_requirement` finding when an explicit stream that it binds to by name has subjects that cover each of the
input's subjects. An input declared `external` SHALL raise no `orphaned_port` finding for a missing publisher.
Components SHALL be visited in instance-name order and edges emitted in a stable order, so equal inputs marshal to
byte-equal JSON.

#### Scenario: The vocabulary is closed

- **WHEN** compositions exhibiting each condition are validated
- **THEN** every emitted `type` is one of the fourteen constants and each constant is emitted by at least one case
  (`TestValidateFindingsVocabularyIsClosed`)

#### Scenario: Offline clash judgment matches admission

- **WHEN** two components declare `http:0.0.0.0:8080` and `websocket:0.0.0.0:8080`, or `0.0.0.0` and `::` on one
  port, or `UDP` and `udp` on one port, or a wildcard and `127.0.0.1` on one port
- **THEN** `Validate` reports one `exclusive_resource_conflict` whose message equals the text
  `Registry.CreateComponent` refuses with for the same pair, and two claims on different transports are not reported
  (`TestValidateExclusiveClashMatchesAdmission`)

#### Scenario: A component clashing with itself

- **WHEN** one component declares `tcp:0.0.0.0:8080` on an input and an output
- **THEN** the result carries one `port_declaration_error` for that component and no `exclusive_resource_conflict`
  (`TestValidateSelfClashIsPortDeclarationError`)

#### Scenario: Findings, not error returns

- **WHEN** a configuration names an unknown factory, a required stream input with no publisher, an interface
  mismatch, a JetStream input fed only by core NATS, or two writers to one KV bucket
- **THEN** each is reported with its type and severity, other components are still analyzed, and `Validate` returns
  no error (`TestValidateReportsUnknownComponent`, `TestValidateReportsRequiredStreamInputWithoutPublisher`,
  `TestValidateReportsInterfaceMismatch`, `TestValidateReportsStreamRequirement`,
  `TestValidateReportsConnectionPatternConflict`)

#### Scenario: Explicit streams and external inputs

- **WHEN** a JetStream input's named explicit stream covers its subjects, or another-named or merely overlapping
  stream is declared, or a required input is marked `external`
- **THEN** only the covering stream of the bound name suppresses `stream_requirement`, and the marked input raises no
  `orphaned_port` (`TestValidateStreamRequirementSatisfiedByExplicitStream`,
  `TestValidateStreamRequirementNeedsTheNamedStream`, `TestValidateStreamRequirementNeedsCoverNotOverlap`,
  `TestValidateSuppressesOrphanOnlyForExternallyFedInput`, `TestValidateImportLaneIsExternallyFedWithoutASecondKnob`)

#### Scenario: Validation is deterministic

- **WHEN** validation runs twice on the same catalog and configuration
- **THEN** the results marshal to byte-equal JSON (`TestValidateIsDeterministic`)

### Requirement: The catalog and the projection are read-only views

`composition.Catalog(registry)` SHALL list every registered factory sorted by ID. Each entry SHALL carry its metadata,
its schema, and either the ports its declarer resolves for the empty configuration `{}` or `ports_require_config`
with the declarer's error text. `composition.Mermaid(graph)` SHALL render the projection as a `flowchart LR` with
nodes and edges in a stable order. Neither SHALL store anything.

#### Scenario: Default ports or the reason they need configuration

- **WHEN** one factory declares ports for `{}` and another refuses `{}`
- **THEN** the first entry carries `default_ports` and the second `ports_require_config` with the refusal text
  (`TestCatalogCarriesDefaultPortsOrRequiresConfig`)

#### Scenario: The diagram is stable

- **WHEN** the same projection is rendered twice, with nodes and edges supplied in different orders
- **THEN** the two renderings are equal (`TestMermaidIsDeterministic`)

### Requirement: The example configuration carries no error finding

`config/example_config.json` SHALL validate with no error finding against a registry holding the factories it names,
and a unit test SHALL assert it.

#### Scenario: The example validates clean

- **WHEN** the example configuration is loaded and validated against a registry built by those factories' `Register`
  functions
- **THEN** the result has no error finding (`TestValidateExampleConfigHasNoErrorFindings`)
