# fusion

## ADDED Requirements

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

- **WHEN** a seed read alone fails transient, or the context ends
- **THEN** `Entities` returns that error (`TestPoisonFallbackReturnsOtherError`, `TestPoisonFallbackEndedContext`)

### Requirement: The client's own decode refusal is returned, never retried

A refusal from the client's own check of a reply (`graph.ValidateDecodedEntityStates` on the batch reply,
`graph.UnmarshalEntityState` on an `entity` reply) SHALL be returned by `Entities`, and a refused batch reply SHALL
start no `entity` request.

#### Scenario: The client's own validation refuses

- **WHEN** the batch reply, or a per-ID reply, fails the client's own stored-value check
- **THEN** `Entities` returns that refusal, and after a refused batch reply sends no `entity` request
  (`TestOwnValidationNotRetried`)

## MODIFIED Requirements

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

- **WHEN** the context is cancelled before `Entities` returns
- **THEN** `Fuse` returns the context's error (`TestEndedContextReturnsError`); a nil context is refused
  (`TestFuseRefusesNilContext`)
