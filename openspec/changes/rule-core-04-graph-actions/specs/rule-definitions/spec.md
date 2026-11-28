# rule-definitions Specification

## MODIFIED Requirements

### Requirement: The action handler set is built once

`NewHandlers(product, cfg)` SHALL be the one way to build the handler set `Load` and the executor take: the nine
built-in types, the product's map, and the settings in `cfg`, whose zero value writes no triples. It SHALL refuse,
classified invalid and naming the type, a product type name equal to a built-in name as written, an empty name or a
nil handler. `Load` SHALL refuse a set `NewHandlers` did not build.

#### Scenario: A product name reuses a built-in one

- **WHEN** `NewHandlers` is given a product handler named `publish`
- **THEN** it fails, classified invalid, naming `publish`

#### Scenario: No product handlers

- **WHEN** `NewHandlers` is given no product handlers
- **THEN** the set holds the nine built-in types and no other

### Requirement: Built-in actions refuse settings they do not read

Each built-in type SHALL refuse at load a body key neither the core nor it reads, naming key, position, rule and
source: `add_triple` and `update_triple` read `subject`, `predicate`, `object`; `remove_triple` `subject`,
`predicate`; `publish` `subject`, `properties`. `publish` SHALL refuse a declared `user.response.>` subject; triple
types an undeclared predicate, and any action with triple writes off, saying the rule processor writes no triples;
`max_iterations` SHALL be 0 or more.

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
