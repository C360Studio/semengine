# rule-conditions Specification

## MODIFIED Requirements

### Requirement: A regex pattern is compiled before evaluation

`Compile` SHALL compile each `regex` pattern in a condition slice and keep it on that element. It SHALL refuse a
non-string value, a pattern holding one of `SubstitutionPrefixes()` (`$now`, `$entity.`, `$state.`, `$schedule.`,
`$message.`), a pattern over 500 characters, and one `regexp` rejects; each refusal is classified invalid and names
index and field. Evaluation SHALL never compile, and refuses a regex condition not compiled from its current value.

#### Scenario: A literal pattern matches as the standard library does

- **WHEN** a `regex` condition with pattern `^a.*z$` is compiled and evaluated against a field holding "abcz"
- **THEN** it matches, and its result equals `regexp.MatchString` for the same pattern and string

#### Scenario: A pattern holding a substitution token is refused

- **WHEN** `Compile` receives a `regex` condition whose pattern is `$entity.id` or `^x-$now`
- **THEN** it returns an error classified invalid that names the field and says to write the dollar sign as `[$]`

#### Scenario: Other dollar signs are literal

- **WHEN** `Compile` receives `[$]now`, `[$a-z]+`, `^\$USD`, `$caller.id`, `$related.id`, or an end-anchored
  pattern such as `^[a-z]+$`
- **THEN** it compiles without error

#### Scenario: An uncompiled or edited regex condition is refused at evaluation

- **WHEN** a `regex` condition is evaluated without `Compile`, or after its value was changed since `Compile`
- **THEN** evaluation returns an `*EvaluationError` naming the field, and no pattern is compiled
