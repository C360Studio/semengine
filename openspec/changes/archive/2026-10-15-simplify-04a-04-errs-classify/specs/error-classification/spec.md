# error-classification

## ADDED Requirements

### Requirement: An unclassified error is classified fatal

`errs.Classify` SHALL return `errs.ErrorFatal` for an unclassified error: a non-nil error whose chain holds no
`*errs.ClassifiedError` and none of the sentinels that `errs.IsTransient`, `errs.IsFatal` or `errs.IsInvalid` list,
whatever the error's text says. `errs.IsTransient`, `errs.IsFatal` and `errs.IsInvalid` SHALL each report false for an
unclassified error, because each reports only the class an error is marked with. An error whose chain holds a
`*errs.ClassifiedError` or a listed sentinel SHALL keep the class that marks it.

#### Scenario: A bare error

- **WHEN** `errs.Classify` is given `errors.New("boom")`
- **THEN** it returns `errs.ErrorFatal`, and `errs.IsTransient`, `errs.IsFatal` and `errs.IsInvalid` each report false

#### Scenario: Text that names a class

- **WHEN** an unclassified error's text contains "timeout" and "transient"
- **THEN** `errs.Classify` returns `errs.ErrorFatal`

#### Scenario: A wrapped classified error

- **WHEN** an error wraps, with `%w`, a `*errs.ClassifiedError` of class transient
- **THEN** `errs.Classify` returns `errs.ErrorTransient`

#### Scenario: A listed sentinel

- **WHEN** an error wraps `context.DeadlineExceeded`
- **THEN** `errs.Classify` returns `errs.ErrorTransient`
