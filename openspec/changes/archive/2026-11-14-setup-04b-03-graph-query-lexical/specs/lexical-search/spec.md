# lexical-search Delta

## MODIFIED Requirements

### Requirement: A query with nothing to search for is refused

A phrase or excluded term left with no word after tokenizing SHALL be ignored. A query left with no plain word and no
phrase SHALL be refused with an error matching `ErrEmptyQuery`, and one with more than `graph.MaxSearchTerms` (32)
distinct words, counting plain, phrase and excluded words after tokenizing, with an error matching `ErrQueryTooLong`.

#### Scenario: Forms and refusals

- **WHEN** the queries are `retry "to be" -the`, `the of and`, `"to be"`, `-test`, `""`, 32 distinct words and 33
  distinct words
- **THEN** the first parses to the word `retry` alone, the next four are refused with `ErrEmptyQuery`, the 32 words
  parse, and the 33 are refused with `ErrQueryTooLong`
- **AND** the test that verifies this is `TestParseQuery`

### Requirement: Syntax kept for later query forms is refused

`ParseQuery` SHALL refuse, with an error matching `ErrReservedSyntax`, a term outside quotes that starts with `name:`
or `body:` followed by more text, or that ends in a letter or digit followed by `*`, unless the term also starts with
`*`, which marks emphasis. The refusal's message SHALL name the term and say that quoting it searches it as text.
Every other character outside letters and digits, `~` and a lone `*` among them, SHALL separate words, as in
documents.

#### Scenario: Reserved and ordinary punctuation

- **WHEN** the queries are `name:retry`, `retr*`, `void*`, `pkg.Func() name: retry`, `SELECT * FROM users`,
  `java.util.*`, `**bold**`, `*bold* text`, `git reset HEAD~`, `retry~`, `cd ~` and `"name:retry void*"`
- **THEN** the first three are refused with `ErrReservedSyntax`, each message naming its term and saying to quote it;
  the next eight parse to the words `pkg func name retry`, `select user`, `java util`, `bold`, `bold text`,
  `git reset head`, `retry` and `cd`; and the last parses to the phrase `name retry void`
- **AND** the test that verifies this is `TestParseQueryReservesForms`

## ADDED Requirements

### Requirement: A query names the words it excludes

`Query.Excluded` SHALL return the query's excluded terms, each as its words in order, in the order the query gave
them, so a caller can say which words removed entities.

#### Scenario: A command line

- **WHEN** the query is `go test -race ./pkg -run TestX`
- **THEN** `Excluded` returns `[[race] [run]]` (`TestParseQueryExcluded`)
