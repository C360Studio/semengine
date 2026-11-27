# rule-conditions Specification

## ADDED Requirements

### Requirement: An absent required field is told apart

When a condition marked `required` reads a field the entity, the state or the message does not hold, the evaluator's
`*EvaluationError` SHALL wrap `ErrRequiredFieldMissing`, so a caller tells this case apart with `errors.Is`. No other
evaluation error SHALL wrap it.

#### Scenario: A required message field is absent

- **WHEN** a required `$message.x` condition is evaluated on message fields without `x`
- **THEN** the error is an `*EvaluationError` and `errors.Is(err, ErrRequiredFieldMissing)` holds

#### Scenario: An operator failure is not a missing field

- **WHEN** a required `length_eq "abc"` condition is evaluated on a field holding a list
- **THEN** the error does not satisfy `errors.Is(err, ErrRequiredFieldMissing)`
