# graph-clustering Delta

## ADDED Requirements

### Requirement: Text is read through the shared text rule

graph-clustering SHALL read an entity's words only from `graph/entitytext`'s `Values` under its `text_suffixes`, which
SHALL mean what graph-lexical's key means, empty meaning the default list.

#### Scenario: The same text as the word index

- **WHEN** an entity holds values under configured and unconfigured suffixes, and a relationship
- **THEN** the text its keywords are counted from equals `Values` for it under the same suffixes, as graph-lexical
  reads it (`TestKeywordTextMatchesLexical`)

#### Scenario: The key is read

- **WHEN** `text_suffixes` adds `.comment`
- **THEN** words of a `code.doc.comment` value count, and without the key they do not (`TestTextSuffixesIsRead`)
