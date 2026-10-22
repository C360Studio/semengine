# composition-validation

## ADDED Requirements

### Requirement: One subject grammar decides validity, cover and overlap

`internal/flowgraph` SHALL own one subject grammar, and `ValidSubject`, `SubjectCovers` and `SubjectsOverlap` SHALL all
read it: a subject is one or more `.`-separated tokens, none empty; `*` and `>` appear only as a whole token and `>`
only last; and no token holds a space, tab, line feed, carriage return or form feed. A subject that fails the grammar
SHALL be invalid, SHALL cover nothing, SHALL be covered by nothing and SHALL overlap nothing, so a stream filter holding
whitespace is not treated as covering a subject or as capturing a reserved one. The grammar is stricter than NATS on a
wildcard inside a token and on form feed. The message logger's `Start` SHALL read `ValidSubject`; the grammar SHALL have
no second copy.

#### Scenario: Whitespace in a token

- **WHEN** `SubjectCovers("a.>", "a.foo bar")` and `SubjectsOverlap("a.>", "a.foo bar")` are asked, with each of the
  five whitespace characters
- **THEN** both return false
- **AND** the test that verifies this is `TestSubjectHelpersRefuseWhitespace`

#### Scenario: ValidSubject accepts exactly the grammar

- **WHEN** `ValidSubject` is asked of the valid and malformed subjects in the tests, and of generated strings
- **THEN** it accepts exactly the subjects the grammar admits, and a subject covers itself exactly when it is valid
- **AND** the tests that verify this are `TestValidSubject` and `FuzzValidSubject`
