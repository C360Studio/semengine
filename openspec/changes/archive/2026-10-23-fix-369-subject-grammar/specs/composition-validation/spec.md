# composition-validation

## MODIFIED Requirements

### Requirement: Validation is a pure function with a closed vocabulary

`composition.Validate(catalog, cfg)` SHALL be a deterministic function of a `*component.Registry` and a `*config.Config`
that performs no I/O, opens no connection and constructs no component; it SHALL return an error only for a nil
argument, and every judgment SHALL be a finding. It SHALL judge the configuration document with
`config.Config.ValidateDocument`, so its result does not depend on files on the host and it writes nothing to standard
error. The one input it reads from outside its arguments is the current time, used to judge whether a stream migration
override has expired. `composition.Analyze(declarations, streams)` SHALL be the graph-level half of the same function,
over declarations and the configuration's explicit `streams`. The result SHALL carry `status` (`errors` when any error
finding exists, else `warnings` when any warning exists, else `valid`), `errors`, `warnings` and `graph`, with every
array non-nil. Each finding SHALL carry `type`, `severity`, a non-empty `component`, an optional `port`, a non-empty
`message` and a non-nil `suggestions`.

`type` SHALL be one of the exported constants `config_invalid`, `unknown_component`, `component_type_mismatch`,
`component_config_invalid`, `port_declaration_error`, `exclusive_resource_conflict`, `connection_pattern_error`,
`stream_requirement`, `disconnected_node`, `orphaned_port`, `interface_mismatch`, `missing_interface`,
`empty_composition`, `reserved_subject_capture` and `malformed_subject`. Severity SHALL be warning for
`disconnected_node`, `missing_interface`, `empty_composition` and for every `orphaned_port` except a required stream
input with no publisher. Severity SHALL be error for every other type.

Two enabled components whose exclusive claims clash SHALL yield one `exclusive_resource_conflict` error, on the
component later in instance-name order. Its message SHALL be the error text `component.Declaration.ConflictWith`
returns. A component whose own claims clash SHALL yield `port_declaration_error`. A JetStream input SHALL NOT be a
`stream_requirement` finding when an explicit stream that it binds to by name has subjects that cover each of the
input's subjects. An input declared `external` SHALL raise no `orphaned_port` finding for a missing publisher. A port's
interface contract and its connected state SHALL be judged in its own lane (input or output), so a port in one lane
never stands in for a same-named port in the other. Components SHALL be visited in instance-name order and edges
emitted in a stable order, so equal inputs marshal to byte-equal JSON.

#### Scenario: The vocabulary is closed

- **WHEN** compositions exhibiting each condition are validated
- **THEN** every emitted `type` is one of the fifteen constants and each constant is emitted by at least one case
  (`TestValidateFindingsVocabularyIsClosed`)

#### Scenario: Offline clash judgment matches admission

- **WHEN** two components declare one of these pairs: `http:0.0.0.0:8080` and `websocket:0.0.0.0:8080`; `0.0.0.0`
  and `::` on one port; `UDP` and `udp` on one port; a wildcard and `127.0.0.1` on one port
- **THEN** `Validate` reports one `exclusive_resource_conflict` whose message equals the literal expected text, and
  the error `Registry.CreateComponent` refuses the same pair with contains that text
- **AND** two claims on different transports are reported by neither (`TestValidateExclusiveClashMatchesAdmission`)

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

#### Scenario: A same-named output does not hide an input

- **WHEN** a source output providing `a.v1` feeds a processor input `data` requiring `b.v1`, and the processor's output
  is also named `data` and provides `a.v1`
- **THEN** the result carries one `interface_mismatch`, as it does when that output is renamed `out`
  (`TestValidateKeepsSameNamedLanesApart`)
- **AND** a required stream input `data` with no publisher is an `orphaned_port` error even when the same component's
  output `data` is connected (`TestValidateReportsOrphanBesideSameNamedOutput`)

#### Scenario: Explicit streams and external inputs

- **WHEN** a JetStream input's named explicit stream covers its subjects, or another-named or merely overlapping
  stream is declared, or a required input is marked `external`
- **THEN** only the covering stream of the bound name suppresses `stream_requirement`, and the marked input raises no
  `orphaned_port` (`TestValidateStreamRequirementSatisfiedByExplicitStream`,
  `TestValidateStreamRequirementNeedsTheNamedStream`, `TestValidateStreamRequirementNeedsCoverNotOverlap`,
  `TestValidateSuppressesOrphanOnlyForExternallyFedInput`, `TestValidateImportLaneIsExternallyFedWithoutASecondKnob`)

#### Scenario: Files on the host do not change the result

- **WHEN** a configuration names a client CA file, and validation runs with the file absent and again with it present,
  and once more with `insecure_skip_verify` set
- **THEN** the first two results marshal to byte-equal JSON and nothing is written to standard error
  (`TestValidateReadsNoTLSFile`)

#### Scenario: Validation is deterministic

- **WHEN** validation runs repeatedly on the same catalog and configuration, including one with several invalid
  component entries
- **THEN** the results marshal to byte-equal JSON, and the `config_invalid` message names the first invalid entry in
  instance-name order (`TestValidateIsDeterministic`, `TestValidateConfigInvalidIsDeterministic`)

## ADDED Requirements

### Requirement: A malformed port or stream subject is an error finding

`composition.Analyze` SHALL check, with `flowgraph.ValidSubject`, every subject of every NATS, NATS-request and
JetStream port of every declaration, in the input and the output lane, and every subject of every explicit stream in
`streams`. Each subject that fails SHALL yield one `malformed_subject` error.

A port's finding SHALL carry the component's instance name and the port's name. Its message SHALL name the lane, the
field (`subject`, or `subjects[i]` for a JetStream port) and the subject. An explicit stream's finding SHALL carry the
component `config` and no port. Its message SHALL name the stream key, the field `subjects[i]` and the subject. A
stream derived from a JetStream output SHALL NOT be reported again under its stream name.

The check SHALL run before `Analyze` returns early, so it is reported when no component is enabled and when the flow
graph cannot be built. A subject that fails the grammar overlaps nothing ("One subject grammar decides validity, cover
and overlap"), so it SHALL yield no `reserved_subject_capture`.

The grammar is stricter than NATS: a token that mixes a wildcard with other text, such as `a*`, is a NATS literal and
SHALL be a `malformed_subject` here.

#### Scenario: The probe's subjects at every site

- **WHEN** each of `a.foo bar`, `a.foo` + form feed + `bar`, `a.>.b`, `a*`, `a.b*.c`, `a..b`, and a subject holding a
  tab, a line feed or a carriage return is declared on a NATS input, a NATS output, a NATS-request input, a
  NATS-request output, a JetStream input's and a JetStream output's second subject, and an explicit stream's second
  subject; and an empty subject on the explicit stream
- **THEN** each yields exactly one `malformed_subject` error naming the component, port, lane, field and subject, or
  the stream key, index and subject
- **AND** `a.>`, `a.*.c`, `>` and `$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.>` at the same sites yield none
- **AND** the test that verifies this is `TestAnalyzeReportsMalformedSubject`

#### Scenario: Reported when analysis stops early

- **WHEN** an explicit stream holds `a..b` and no component is enabled, and again beside two writers to one KV bucket
- **THEN** the result carries the `malformed_subject` error beside `empty_composition` or `connection_pattern_error`
- **AND** the test that verifies this is `TestAnalyzeReportsMalformedSubjectBeforeEarlyReturns`

#### Scenario: A malformed filter is not a reserved capture

- **WHEN** an explicit stream holds `graph.mutation.>.x`
- **THEN** the result carries one `malformed_subject` and no `reserved_subject_capture`
- **AND** the test that verifies this is `TestAnalyzeMalformedFilterIsNotAReservedCapture`

Not covered by a test in this change: boot refusal on `malformed_subject` in particular. It follows from the error
severity and the component manager's refusal on any error finding (`TestComponentManagerRefusesBootOnErrorFinding`).
