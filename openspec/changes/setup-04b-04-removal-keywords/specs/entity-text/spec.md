# entity-text Delta

## MODIFIED Requirements

### Requirement: One rule says which values are an entity's text

`graph/entitytext` SHALL offer two views of one rule. `Values` SHALL select an entity's string values whose
predicate ends in a configured suffix, relationships skipped, in triple order; graph-embedding and graph-clustering
SHALL read their text only through it. Empty `text_suffixes` SHALL mean the default list.

#### Scenario: graph-embedding's bytes do not change

- **WHEN** the fixed entities of graph-embedding's tests are read through `Values` and joined as graph-embedding
  joins them
- **THEN** the text equals what graph-embedding embedded before the rule existed (`TestEmbeddingTextUnchanged`)

#### Scenario: graph-clustering reads the same values

- **WHEN** an entity is read by graph-clustering and through `Values` under the same suffixes
- **THEN** the values are equal (`TestKeywordTextMatchesLexical`)
