# error-classification Specification

## Purpose

`pkg/errs` sorts a failure into one of three classes: transient (retrying may succeed), invalid (the input is wrong;
retrying the same input will fail again) or fatal (do not retry). This capability states what `errs.Classify` returns
for an error nobody marked with a class: fatal, so an unjudged failure is never retried by default.

## Requirements

### Requirement: An unclassified error is classified fatal

`errs.Classify` SHALL return `errs.ErrorFatal` for an unclassified error: a non-nil error whose chain holds no
`*errs.ClassifiedError` and none of the sentinels that `errs.IsTransient`, `errs.IsFatal` or `errs.IsInvalid` list,
whatever the error's text says.

#### Scenario: A bare error

- **WHEN** `errs.Classify` is given `errors.New("boom")`
- **THEN** it returns `errs.ErrorFatal`

#### Scenario: Text that names a class

- **WHEN** an unclassified error's text contains "timeout" and "transient"
- **THEN** `errs.Classify` returns `errs.ErrorFatal`

### Requirement: The class predicates report only a marked class

`errs.IsTransient`, `errs.IsFatal` and `errs.IsInvalid` SHALL each report false for an unclassified error, because each
reports only the class an error is marked with.

#### Scenario: A bare error is none of the three

- **WHEN** `errs.IsTransient`, `errs.IsFatal` and `errs.IsInvalid` are given `errors.New("boom")`
- **THEN** each reports false

### Requirement: A marked error keeps its class

An error whose chain holds a `*errs.ClassifiedError` or a listed sentinel SHALL keep the class that marks it.

#### Scenario: A wrapped classified error

- **WHEN** an error wraps, with `%w`, a `*errs.ClassifiedError` of class transient
- **THEN** `errs.Classify` returns `errs.ErrorTransient`

#### Scenario: A listed sentinel

- **WHEN** an error wraps `context.DeadlineExceeded`
- **THEN** `errs.Classify` returns `errs.ErrorTransient`
