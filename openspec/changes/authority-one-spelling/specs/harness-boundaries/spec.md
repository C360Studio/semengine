# harness-boundaries

## MODIFIED Requirements

### Requirement: No second spelling of deployment authority

A deployment's authority (`org`, `platform`) SHALL be established once, by `config.Manager.Start` under ADR-104,
carried to components once as `deps.Platform`, and SHALL become positions 1–2 of an identity only through
`FrameworkIdentityFamily.EntityID` (#72 ruling A as extended, comment 5969505488). No other package SHALL compute,
parse, re-join or carry it, and no message, envelope or metadata SHALL hold a copy. Two contract tests in
`internal/harness/contract`, both run by `task test:unit`, enforce the parts of this a static check can see; each has
a sensitivity test that plants the violation in a temporary module and requires the check to name it.

**Names.** No exported name in a non-test package of this module, internal and `main` packages included, SHALL
contain `Federation`, `GlobalID` or `EntityIRI` (case-sensitive substrings). This covers package-level identifiers,
and the exported methods, struct fields and interface methods of package-level types, whether the type is exported
or not. A type alias is a package-level type: its own name is checked, and so are the members of the type it stands
for; members of a type this module declares are reported once, at that declaration. A failure names the file, line,
qualified identifier and this requirement. Unexported names, test files, comments and string literals are not
checked.

**Fields.** No exported struct field named `Org` or `Platform` SHALL be declared in a non-test file of a package of
this module, in a named or an anonymous struct, except: any field in the packages `pkg/types`, `pkg/platform` and
`config` (the source and its validators); the two fields of the type `PlatformMeta` in the top-level package `types`
(import path `<module>/types` — not `pkg/types`, and not Go's `go/types`); `processor/rule.CallerContext.Org` (a
caller's organization claim, which is not the deployment authority); and the carrier `deps.Platform`, by exact field:
`component.Dependencies.Platform`, `service.Dependencies.Platform` and `processor/rule.Dependencies.Platform`. A
field's type is not an exception: a further field of type `types.PlatformMeta` named `Org` or `Platform` fails
until this list names it. The exceptions are listed by exact package path, exact
type name and exact field name in this requirement and nowhere else; a new exception is a change to this
requirement. The failure SHALL name the file, the line, the struct type, the field and this requirement. The check is
static and by name: a field named otherwise (`OrgID`, `PlatformID`, `org`, `platform`), a copy held in an untyped
holder, and a value re-joined inside a function body are outside its scope by construction, and remain review only.

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
  or a field `Platform` of type `types.PlatformMeta` on a struct other than the three the exception names
- **THEN** the contract test fails naming each field with its file, line and struct type

#### Scenario: The owners, the carrier and the caller's claim pass

- **WHEN** a fixture module declares `Org` and `Platform` fields in its `pkg/types`, `pkg/platform` and `config`
  packages; a type `PlatformMeta` in its top-level `types` package with fields `Org` and `Platform`; the carrier
  field `Platform` of that type on `component.Dependencies`, `service.Dependencies` and
  `processor/rule.Dependencies`; a field `Org` on `processor/rule.CallerContext`; and unexported fields `org` and
  `platform` of type `string` anywhere
- **THEN** the contract test reports nothing for any of them

#### Scenario: A test file is not checked

- **WHEN** a `_test.go` file declares an exported struct field `Org` or an exported function `EntityIRI`
- **THEN** neither contract test reports it

#### Scenario: This module passes

- **WHEN** both contract tests run over this module
- **THEN** they pass, each reporting the number of packages it checked
