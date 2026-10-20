# graph-transport-boundary

## ADDED Requirements

### Requirement: No stream captures a reserved request subject

`composition.Analyze` SHALL report one `reserved_subject_capture` error for each stream subject filter that a concrete
subject matches together with a reserved request subject. The filters checked are each subject of an explicit
`streams` entry and each stream subject of a JetStream output port. The reserved subjects are the subjects of
`graph.QueryVerbs()` and the mutation subject family `graph.mutation.>`, read from their declarations. The finding
SHALL name the stream, the filter and the first reserved subject captured (query verbs in table order, then the
mutation family). Its component SHALL be `config` for an explicit stream, or the instance with its port for an output
port. A filter that shares no concrete subject with a reserved subject SHALL NOT be reported.

#### Scenario: A wide or exact filter captures a request subject

- **WHEN** an explicit stream declares `graph.>`, or `graph.ingest.query.entity`, or a JetStream output declares
  `*.mutation.create`
- **THEN** each filter yields one `reserved_subject_capture` error naming its stream, the filter, and
  `graph.ingest.query.entity` or `graph.mutation.>` (`TestValidateRefusesStreamCapturingReservedSubject`)

#### Scenario: Neighbouring namespaces are not refused

- **WHEN** streams declare `graph.ingest.events.>`, `graph.ingestion.>`, `graph.mutations.created` and
  `graph.ingest.query`
- **THEN** no `reserved_subject_capture` finding is reported

#### Scenario: The overlap judgment matches enumeration

- **WHEN** every filter of one to four tokens over a fixed token set with `*` and `>` is checked against each reserved
  subject
- **THEN** a filter is reported exactly when a concrete subject enumerated from the reserved subject matches it
  (`TestReservedSubjectOverlapMatchesEnumeration`)
