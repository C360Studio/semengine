# Delta: error-classification

## ADDED Requirements

### Requirement: A classified error built without a class is fatal

`errs.ErrorClass`'s zero value SHALL name no class. A `*errs.ClassifiedError` whose `Class` is unset SHALL be none of
transient, invalid and fatal to `errs.IsTransient`, `errs.IsInvalid` and `errs.IsFatal`, and `errs.Classify` SHALL
return `errs.ErrorFatal` for it, even when it wraps a listed transient sentinel.

#### Scenario: A classified error literal with no class

- **WHEN** `errs.Classify` is given `&errs.ClassifiedError{Err: errors.New("boom")}`
- **THEN** it returns `errs.ErrorFatal`, and `errs.IsTransient` reports false

#### Scenario: An unset class over a transient sentinel

- **WHEN** `errs.Classify` is given `&errs.ClassifiedError{Err: context.DeadlineExceeded}`
- **THEN** it returns `errs.ErrorFatal`
