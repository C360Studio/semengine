# fusion

## ADDED Requirements

### Requirement: The NATS client starts nothing and follows the verb table

`fusionnats.New` SHALL refuse a nil requester and a negative timeout, SHALL treat a zero timeout as natsclient's
default, and SHALL refuse, naming the verb, when `graph.QueryVerbs()` lacks a graph-query row it uses. The client
SHALL hold no goroutine, watch or context, and SHALL read no readiness status. Each request SHALL go to its row's
subject. A reply on an enveloped row SHALL be decoded as a `graph.QueryResponse` of the row's type and refused when it
names no `producer`; a reply on a bare row SHALL be decoded as the row's type.

#### Scenario: Bad wiring

- **WHEN** `New` is given a nil requester or a negative timeout
- **THEN** it returns an error and no client (`TestNewRefusesNilRequester`, `TestNewRefusesNegativeTimeout`)

#### Scenario: Subjects from the table

- **WHEN** each `RetrievalClient` method is called on a recording requester
- **THEN** each request's subject is its graph-query row's subject (`TestSubjectsComeFromVerbTable`), and a bare body
  or `{}` on an enveloped row is refused (`TestEnvelopedRowNeedsProducer`)

#### Scenario: A missing row

- **WHEN** the verb lookup is given a table without the `batch` row
- **THEN** it refuses, naming `batch` (`TestVerbLookupNamesMissingRow`)

### Requirement: The batch read accounts for every requested ID

`Entities` SHALL return every distinct requested ID exactly once, in `Entities` in request order or in `Unhydrated`
with a reason, and SHALL send no request for zero IDs. An NL `Resolve` SHALL copy the search reply's
`unreadable_records` and `failed_records` into the `Resolution`.

#### Scenario: Duplicates and under-reporting

- **WHEN** generated requests (duplicates included) meet generated replies that list an ID as missing, as both
  found and missing, or not at all
- **THEN** each distinct ID appears once, found ones in request order, unreported ones `unknown` (`TestReconcileTotal`)

#### Scenario: Zero IDs

- **WHEN** `Entities` is called with no IDs
- **THEN** it returns an empty `Hydration` and sends no request (`TestEntities_EmptyShortCircuits`)

#### Scenario: Word-search counts

- **WHEN** the semantic reply reports 2 unreadable and 1 failed record
- **THEN** the `Resolution` carries 2 and 1, and a symbol or prefix resolve carries 0 and 0
  (`TestSemanticCountsReachResolution`)

### Requirement: A poisoned seed is unhydrated, not a failed read

When the batch request's `RequestClassified` call returns an error with code `graph_state_reset_required`, `Entities`
SHALL send one `entity` request per distinct non-empty requested ID, list an ID whose request is refused with that
code as unhydrated `error`, one not found as unhydrated `not_found` and an empty ID as unhydrated `error` with no
request, and return any other error, the client's own decode refusal of a reply included. A decode refusal of the
batch reply SHALL be returned and start no `entity` request. It SHALL never decide from an error's text.

#### Scenario: One poisoned seed

- **WHEN** the batch is refused for poison and one of three seeds is poisoned
- **THEN** two entities hydrate and the third is unhydrated `error` (`TestPoisonedSeedUnhydrated`,
  `TestPoisonedSeedOverRealResponders`)

#### Scenario: Another fault while reading alone

- **WHEN** a seed read alone fails transient
- **THEN** `Entities` returns that error (`TestPoisonFallbackReturnsOtherError`)

#### Scenario: The client's own validation refuses

- **WHEN** the batch reply, or a per-ID reply, fails the client's own stored-value check
- **THEN** `Entities` returns that refusal, and after a refused batch reply sends no `entity` request
  (`TestOwnValidationNotRetried`)

## MODIFIED Requirements

### Requirement: A fatal seed read or an ended context fails the call

When `Resolve` or `Entities` returns an error `errs.IsFatal` reports, such as `graph_state_reset_required` from a
prefix resolve, or when the request's context has ended during any read, `Fuse` SHALL return that error and no
response. `Fuse` SHALL refuse a nil context. A poisoned seed does not reach `Fuse` as an error from `fusionnats`,
which lists it as unhydrated (requirement "A poisoned seed is unhydrated, not a failed read").

#### Scenario: A reset while hydrating seeds

- **WHEN** `Resolve` answers and `Entities` returns `graph_state_reset_required`
- **THEN** `Fuse` returns that error (`TestFatalReplyReturnsError`)

#### Scenario: The caller gives up

- **WHEN** the context is cancelled before `Entities` returns
- **THEN** `Fuse` returns the context's error (`TestEndedContextReturnsError`); a nil context is refused
  (`TestFuseRefusesNilContext`)
