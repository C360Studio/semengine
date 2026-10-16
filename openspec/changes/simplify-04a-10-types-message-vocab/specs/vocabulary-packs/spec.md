# vocabulary-packs

## ADDED Requirements

### Requirement: One predicate parser, with no shared state

`vocabulary.ParsePredicate` SHALL accept a predicate exactly when it has three segments separated by `.`, each segment
matches `[a-z][a-z0-9]*(-[a-z0-9]+)*` and is at most `MaxPredicateSegmentBytes` (64) bytes, and the whole is at most
`MaxPredicateBytes` (194) bytes (pin ADR-074). It SHALL return the three segments as `PredicateParts`. A refusal SHALL
be a `*PredicateValidationError` naming the first fault in this order: empty, length, arity, then each segment from the
first, checked for empty, length, first byte, then each later byte (character or hyphen). It SHALL keep no state between
calls and SHALL NOT allocate when it accepts a predicate. Every other check in `vocabulary` of predicate syntax SHALL
call it.

#### Scenario: A predicate never seen before

- **WHEN** a valid predicate not parsed before in the process is parsed
- **THEN** the parse returns its three parts and makes no heap allocation

#### Scenario: Agreement with the grammar

- **WHEN** any string is parsed
- **THEN** it is accepted exactly when it matches the grammar above, with the same parts; and a refusal carries the
  reason and segment of the first fault in the order above
