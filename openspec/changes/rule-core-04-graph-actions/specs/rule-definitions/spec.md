# rule-definitions Specification

## MODIFIED Requirements

### Requirement: The action handler set is built once

`NewHandlers(product, cfg)` SHALL be the one way to build the handler set `Load` and the executor take: the nine
built-in types plus the product's map, with the settings in `cfg` that the built-in checks read. It SHALL refuse a
product type name that is exactly a built-in name (compared as written), an empty name or a nil handler, classified
invalid and naming the type. `Load` SHALL refuse a handler set that `NewHandlers` did not build. The zero `cfg` SHALL
mean the rule processor writes no triples.

#### Scenario: A product name reuses a built-in one

- **WHEN** `NewHandlers` is given a product handler named `publish`
- **THEN** it fails, classified invalid, naming `publish`

#### Scenario: No product handlers

- **WHEN** `NewHandlers` is given no product handlers
- **THEN** the set holds the nine built-in types and no other

### Requirement: Built-in actions refuse settings they do not read

Each built-in action type SHALL refuse at load a body key that neither the core nor that type reads, naming the key,
the action's position, the rule and the source. `add_triple` SHALL read `subject`, `predicate` and `object`;
`remove_triple` `subject` and `predicate`; `update_triple` `subject`, `predicate` and `object`; `publish` `subject`
and `properties`. `publish` SHALL refuse a declared subject in `user.response.>`; the triple types SHALL refuse a
predicate that is not a declared literal, and SHALL refuse every action when the handler set was built with triple
writes off, saying the rule processor writes no triples; and `max_iterations` SHALL be 0 or more.

#### Scenario: A misspelt setting

- **WHEN** an `add_triple` action carries `predicat`
- **THEN** `Load` fails naming `predicat`, the position, the rule and the source

#### Scenario: A setting nothing acts on

- **WHEN** an `add_triple` action carries `ttl`, an `update_triple` action `properties`, or a `remove_triple` action
  `object`
- **THEN** `Load` fails naming that key, the position, the rule and the source

#### Scenario: Reserved subject and dynamic predicate

- **WHEN** a `publish` action declares `user.response.x`, or an `add_triple` predicate holds `$`
- **THEN** `Load` fails naming the cause

#### Scenario: Triple writes off

- **WHEN** a rule with an `add_triple` action is loaded with a set built with triple writes off
- **THEN** `Load` fails naming the position, the rule and the source, and saying the rule processor writes no triples
