# graph-clustering Delta

## ADDED Requirements

### Requirement: Text is read through the shared text rule

graph-clustering SHALL read an entity's words only from `graph/entitytext`'s `Values` under its `text_suffixes`, which
SHALL select values as graph-lexical's key does, empty meaning the default list. Label predicates and an offloaded
body, which graph-lexical also indexes, SHALL add no words.

#### Scenario: The values the text rule selects

- **WHEN** an entity holds values under configured and unconfigured suffixes, and a relationship
- **THEN** the text its keywords are counted from equals `Values` for it under the same suffixes, the text
  graph-embedding reads (`TestKeywordTextIsTheRuleValues`)

#### Scenario: The key is read

- **WHEN** `text_suffixes` adds `.comment`
- **THEN** words of a `code.doc.comment` value count, and without the key they do not (`TestTextSuffixesIsRead`)
