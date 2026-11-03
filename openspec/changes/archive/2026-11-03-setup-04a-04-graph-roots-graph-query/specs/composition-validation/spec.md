# composition-validation

## MODIFIED Requirements

### Requirement: Finding types are a closed list

`type` SHALL be one of the exported constants `config_invalid`, `unknown_component`, `component_type_mismatch`,
`component_config_invalid`, `port_declaration_error`, `exclusive_resource_conflict`, `connection_pattern_error`,
`stream_requirement`, `disconnected_node`, `orphaned_port`, `interface_mismatch`, `missing_interface`,
`empty_composition`, `reserved_subject_capture`, `malformed_subject` and `request_subject_conflict`.

#### Scenario: The vocabulary is closed

- **WHEN** compositions exhibiting each condition are validated
- **THEN** every emitted `type` is one of the sixteen constants and each constant is emitted by at least one case
  (`TestValidateFindingsVocabularyIsClosed`)

## ADDED Requirements

### Requirement: A second responder on a declared verb is an error finding

A NATS-request input port whose subject overlaps a subject in `graph.QueryVerbs()` SHALL yield one
`request_subject_conflict` error when its component's factory identity is not that verb's responder, or, when two
instances of the responder declare it, on the instance later in instance-name order. The finding SHALL name the verb's
subject. A port overlapping no table subject SHALL yield none. The check reads declared ports only: a component that
subscribes without declaring a port is not seen.

#### Scenario: A consumer claims a framework verb

- **WHEN** a component of another factory declares a NATS-request input on `graph.query.entity` or `graph.query.*`
- **THEN** the result carries one `request_subject_conflict` naming `graph.query.entity`
  (`TestAnalyzeReportsSecondResponder`)

#### Scenario: A consumer verb beside the family

- **WHEN** graph-query declares `graph.query.*` and another component declares `graph.query.sources`
- **THEN** the result carries no `request_subject_conflict` (`TestAnalyzeReportsSecondResponder`)

#### Scenario: Boot refuses

- **WHEN** the component manager boots a composition with this finding
- **THEN** boot fails naming the subject (`TestBootRefusesSecondResponder`)
