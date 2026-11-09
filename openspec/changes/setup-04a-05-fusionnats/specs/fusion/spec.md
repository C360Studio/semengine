# fusion

## ADDED Requirements

### Requirement: The NATS client starts nothing and follows the verb table

`fusionnats.New` SHALL refuse a nil requester and a negative timeout, SHALL treat a zero timeout as natsclient's
default, and SHALL refuse, naming the verb, when `graph.QueryVerbs()` lacks a graph-query row it uses. The client
SHALL hold no goroutine, watch or context, and SHALL read no readiness status. Each request SHALL go to its row's
subject, and each reply SHALL be decoded as that row declares, enveloped or bare.

#### Scenario: Bad wiring

- **WHEN** `New` is given a nil requester or a negative timeout
- **THEN** it returns an error and no client (`TestNewRefusesNilRequester`, `TestNewRefusesNegativeTimeout`)

#### Scenario: Subjects from the table

- **WHEN** each `RetrievalClient` method is called on a recording requester
- **THEN** each request's subject is its graph-query row's subject (`TestSubjectsComeFromVerbTable`), and a reply in
  the other shape is an error (`TestReplyShapeDecidedByVerb`)

### Requirement: The batch read accounts for every requested ID

`Entities` SHALL return every distinct requested ID exactly once, in `Entities` in request order or in `Unhydrated`
with a reason, and SHALL send no request for zero IDs. An NL `Resolve` SHALL copy the search reply's
`unreadable_records` and `failed_records` into the `Resolution`.

#### Scenario: Duplicates and under-reporting

- **WHEN** generated requests (duplicates included) meet generated replies that list an ID as missing, as both
  found and missing, or not at all
- **THEN** each distinct ID appears once, found ones in request order, unreported ones `unknown` (`TestReconcileTotal`)

### Requirement: A poisoned seed is unhydrated, not a failed read

When the batch request is refused with code `graph_state_reset_required`, `Entities` SHALL read each distinct
requested ID once alone, list an ID refused with that code as unhydrated `error` and one not found as unhydrated
`not_found`, and return any other error. It SHALL never decide this from an error's text.

#### Scenario: One poisoned seed

- **WHEN** the batch is refused for poison and one of three seeds is poisoned
- **THEN** two entities hydrate and the third is unhydrated `error` (`TestPoisonedSeedUnhydrated`,
  `TestPoisonedSeedOverRealResponders`)

#### Scenario: Another fault while reading alone

- **WHEN** a seed read alone fails transient
- **THEN** `Entities` returns that error (`TestPoisonFallbackReturnsOtherError`)
