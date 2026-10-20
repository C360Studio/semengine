# graph-transport-boundary

## ADDED Requirements

### Requirement: No stream captures a reserved request subject

`composition.Analyze` SHALL report one `reserved_subject_capture` error for each stream subject filter that shares
at least one concrete subject with a reserved request subject: overlap, not cover. The filters checked are:

- each subject of an explicit `streams` entry, with finding component `config`, the stream named by its key;
- each stream subject of a JetStream output port, with the instance and port as finding component, the stream named
  by the port's stream name or, when it has none, by `config.DeriveStreamName` of its first subject, as provisioning
  names it.

The reserved subjects are the subjects of `graph.QueryVerbs()` and the mutation subject family `graph.mutation.>`,
read from their declarations. The finding SHALL name the stream, the filter and the first reserved subject captured:
query verbs in table order, then the mutation family. A filter that shares no concrete subject with a reserved
subject SHALL NOT be reported.

#### Scenario: A wide, exact or crossing filter captures a request subject

- **WHEN** an explicit stream declares `graph.>`, or `graph.ingest.query.entity`, or a JetStream output declares
  `*.mutation.create`
- **THEN** each filter yields one `reserved_subject_capture` error naming its stream, the filter, and
  `graph.ingest.query.entity` (the first two) or `graph.mutation.>` (the third)
  (`TestValidateRefusesStreamCapturingReservedSubject`)

#### Scenario: Neighbouring namespaces are not refused

- **WHEN** streams declare `graph.ingest.events.>`, `graph.ingestion.>`, `graph.mutations.created` and
  `graph.ingest.query`
- **THEN** no `reserved_subject_capture` finding is reported (`TestValidateDoesNotRefuseNeighbouringSubjects`)

#### Scenario: The overlap judgment matches enumeration

- **WHEN** every filter of one to four tokens over a fixed token set with `*` and `>` is checked against each reserved
  subject
- **THEN** a filter is reported exactly when a concrete subject enumerated from a reserved subject matches it, naming
  the first such reserved subject in order (`TestReservedSubjectOverlapMatchesEnumeration`)
