# entity-text Delta

## ADDED Requirements

### Requirement: One rule says which values are an entity's text

`graph/entitytext` SHALL select an entity's string values whose predicate ends in a configured suffix,
relationships skipped, in triple order, mark each Name when its predicate is a vocabulary label predicate or ends in
`.name` or `.title` and Body otherwise, and cap the text at `max_text_len` characters (default 4,000), cut at a word.
Empty `text_suffixes` SHALL mean the default list. graph-embedding and graph-lexical SHALL read text only through it.

#### Scenario: graph-embedding's bytes do not change

- **WHEN** the fixed entities of graph-embedding's tests are read through the rule and joined as graph-embedding
  joins them
- **THEN** the text equals what graph-embedding embedded before the rule existed (`TestEmbeddingTextUnchanged`)

#### Scenario: Name and body

- **WHEN** an entity holds `dc.terms.title`, `fleet.identity.name`, `code.doc.comment` and a relationship, with
  `.comment` configured
- **THEN** the first two are Name, the third is Body and the relationship is neither (`TestNameAndBodyRoles`)

### Requirement: An offloaded body is read with a deadline

`graph/entitytext` SHALL read an offloaded body only from the store registered under its `StorageReference`'s
instance, at most enough bytes for the cap, under a context that ends 10 seconds after the read starts, and SHALL
return an error that says the deadline passed when it ends first. A store that does not honour its context is not
bounded by this. It SHALL import no NATS client.

#### Scenario: A store that blocks until its context ends

- **WHEN** the store's read blocks until its context ends
- **THEN** the read returns a deadline error 10 seconds after it began (`TestBodyReadDeadline`, under
  `testing/synctest`)

#### Scenario: Imports

- **WHEN** `go list -deps ./graph/entitytext` runs
- **THEN** no NATS module is listed (`TestEntityTextImportsNoNATS`)
