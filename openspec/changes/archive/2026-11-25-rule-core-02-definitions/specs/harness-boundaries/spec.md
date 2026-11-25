# harness-boundaries Specification

## MODIFIED Requirements

### Requirement: No second spelling of deployment authority

A deployment's authority (`org`, `platform`) SHALL be established once, by `config.Manager.Start` under ADR-104, carried
to components once as `deps.Platform`, and SHALL become positions 1–2 of an identity only through
`FrameworkIdentityFamily.EntityID` (#72 ruling A as extended, comment 5969505488). No other package SHALL compute,
parse, re-join or carry it, and no message, envelope or metadata SHALL hold a copy.

#### Scenario: A deployment-authority name is exported

- **WHEN** a fixture module declares an exported name containing `Federation`, `GlobalID` or `EntityIRI` in a
  public, an internal or a `main` package: a type, function, variable or constant, a method (of an unexported type
  too), a struct field or an interface method, or a type alias, or a member of the struct or interface an alias
  stands for
- **THEN** the contract test fails naming each one once, with its file, line and qualified identifier and the rule

#### Scenario: The words appear where the rule does not apply

- **WHEN** the same words appear only in unexported names, in an exported name that holds one of them in lower case
  only (`Confederation`), in a name such as `Federated` that does not contain one of them, in a comment or a string
  literal, or in a test file
- **THEN** the contract test reports nothing for them

#### Scenario: An authority field outside the owners fails

- **WHEN** a non-test file of a fixture module declares, outside the excepted packages, an exported struct field
  `Org` or `Platform` of type `string` on a named struct (the shape of `graph/inference.HierarchyConfig` and
  `graph/llm.EntityParts` at the pin), on an anonymous struct, or on a struct in an internal or a `main` package;
  or a field `Platform` of type `types.PlatformMeta` on a struct other than the two the exception names
- **THEN** the contract test fails naming each field with its file, line and struct type

#### Scenario: The owners, the carrier and the caller's claim pass

- **WHEN** a fixture module declares `Org` and `Platform` fields in its `pkg/types`, `pkg/platform` and `config`
  packages; a type `PlatformMeta` in its top-level `types` package with fields `Org` and `Platform`; the carrier
  field `Platform` of that type on `component.Dependencies` and `service.Dependencies`; a field `Org` on
  `processor/rule.CallerContext`; and unexported fields `org` and `platform` of type `string` anywhere
- **THEN** the contract test reports nothing for any of them

#### Scenario: A test file is not checked

- **WHEN** a `_test.go` file declares an exported struct field `Org` or an exported function `EntityIRI`
- **THEN** neither contract test reports it

#### Scenario: This module passes

- **WHEN** both contract tests run over this module
- **THEN** they pass, each reporting the number of packages it checked

### Requirement: The caller's claim and the carrier may declare Org and Platform

`processor/rule.CallerContext.Org` (a caller's organization claim, which is not the deployment authority) SHALL be
excepted, and so SHALL the carrier `deps.Platform`, by exact field: `component.Dependencies.Platform` and
`service.Dependencies.Platform`. `processor/rule` declares no `Dependencies` type, so it has no carrier exception.

#### Scenario: The carrier fields

- **WHEN** a fixture module declares `Platform` of type `types.PlatformMeta` on `component.Dependencies` and
  `service.Dependencies`, and `Org` on `processor/rule.CallerContext`
- **THEN** the contract test reports nothing for them

#### Scenario: A rule-engine Dependencies field is not excepted

- **WHEN** a fixture module declares `Platform` of type `types.PlatformMeta` on `processor/rule.Dependencies`
- **THEN** the contract test fails naming the file, line, struct type and field
