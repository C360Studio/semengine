# entity-text Delta

## ADDED Requirements

### Requirement: One rule says which values are an entity's text

`graph/entitytext` SHALL offer two views of one rule. `Values` SHALL select an entity's string values whose
predicate ends in a configured suffix, relationships skipped, in triple order; graph-embedding SHALL read its text
only through it. Empty `text_suffixes` SHALL mean the default list.

#### Scenario: graph-embedding's bytes do not change

- **WHEN** the fixed entities of graph-embedding's tests are read through `Values` and joined as graph-embedding
  joins them
- **THEN** the text equals what graph-embedding embedded before the rule existed (`TestEmbeddingTextUnchanged`)

### Requirement: Document splits an entity's text into Name and Body

`Document` SHALL put in Name every value of a vocabulary label predicate, whatever its suffix, and every `Values`
value whose predicate ends in `.name` or `.title`, each once, and in Body every other `Values` value in triple
order, then the offloaded body; graph-lexical SHALL read its text only through it.

#### Scenario: Name and body

- **WHEN** an entity holds `dc.terms.title`, `fleet.identity.name`, `code.doc.comment`, a registered label predicate
  that ends in no configured suffix, and a relationship, with `.comment` configured
- **THEN** `Document` puts the first two and the label predicate's value in Name and the third in Body, and leaves
  out the relationship; `Values` leaves out the label predicate's value (`TestNameAndBodyRoles`)

### Requirement: The text cap cuts the body's tail and says so

`max_text_len` SHALL be the most characters of text taken. For `Document` it SHALL apply to Body only, inline values
first and the offloaded body after them, cut at a word, and SHALL never cut Name. A cut SHALL be reported to the
caller, which counts it. For `Values` it SHALL apply as graph-embedding applies it today.

#### Scenario: A long body

- **WHEN** an entity's Body is longer than the cap
- **THEN** Body ends at a word at or before the cap, Name is whole, and the cut is reported (`TestDocumentCap`)

### Requirement: An offloaded body is read with a deadline

`graph/entitytext` SHALL read an offloaded body only from the store registered under its `StorageReference`'s
instance, at most enough bytes for the cap, under a context derived from the caller's that ends 10 seconds after
the read starts. A store that does not honour its context is not bounded by this.

#### Scenario: A read under its deadline

- **WHEN** a body is read from a store that answers at once
- **THEN** the store saw a context whose deadline is 10 seconds after the read began (`TestBodyReadDeadline`, under
  `testing/synctest`)

### Requirement: The read's own context decides that the deadline passed

When the read's context has ended by the time the store returns, the read SHALL return an error that says the
deadline passed, whatever error the store returned. The NATS object store returns `nats: timeout` at its deadline,
which wraps no context error (`notes/probes/objectstore-watch-probe.log`, A2).

#### Scenario: A store that waits for its context, then returns its own error

- **WHEN** the store's read blocks until its context ends and then returns an error that wraps no context error
- **THEN** the read returns a deadline error 10 seconds after it began (`TestBodyReadDeadline`, under
  `testing/synctest`)

### Requirement: A body read's failures are told apart

A body read SHALL tell apart a body that is missing (`storage.ErrObjectNotFound`), an instance with no registered
store, and any other failure. `graph/entitytext` SHALL import no NATS client.

#### Scenario: Three failures told apart

- **WHEN** a read finds no object, finds no store for the instance, or fails otherwise
- **THEN** each returns an error the caller tells apart with `errors.Is` (`TestBodyReadOutcomes`)

#### Scenario: Imports

- **WHEN** `go list -deps ./graph/entitytext` runs
- **THEN** no NATS module is listed (`TestEntityTextImportsNoNATS`)
