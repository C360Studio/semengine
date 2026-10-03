# harness-boundaries

## ADDED Requirements

### Requirement: One spelling of the deployment authority

A deployment's authority (`org`, `platform`) SHALL be established once, by `config.Manager.Start` under ADR-104,
carried to components once as `deps.Platform`, and SHALL become positions 1–2 of an identity only through
`FrameworkIdentityFamily.EntityID`. No other package SHALL compute, parse, re-join or carry it, and no message,
envelope or metadata SHALL hold a copy. Two contract tests in `internal/harness/contract`, both run by
`task test:unit`, enforce the parts of this a static check can see; each has a sensitivity test that plants the
violation in a temporary module and requires the check to name it.

**Names.** The public-signature contract's walk over the module's packages SHALL also fail on an exported identifier
— a package-level type, function, variable or constant, an exported method, or an exported struct field — declared
in a non-test file of any package of this module, public, internal or `main`, whose name contains `Federation`,
`GlobalID` or `EntityIRI` (case-sensitive substrings). The failure SHALL name the file, the line, the identifier and
this requirement. Unexported identifiers, test files, comments and string literals are not checked.

**Fields.** No exported struct field named `Org` or `Platform` SHALL be declared in a non-test file of a package of
this module, in a named or an anonymous struct, except: any field in the packages `pkg/types`, `pkg/platform` and
`config` (the source and its validators); the fields of `types.PlatformMeta` and any field whose type is
`types.PlatformMeta` (the carrier, under whatever field name); and `processor/rule.CallerContext.Org` (a caller's
organization claim, which is not the deployment authority). The exceptions are listed by exact package path, exact
type name and exact field name in this requirement and nowhere else; a new exception is a change to this
requirement. The failure SHALL name the file, the line, the struct type, the field and this requirement. The check is
static and by name: a field named otherwise (`OrgID`, `PlatformID`, `org`, `platform`), a copy held in an untyped
holder, and a value re-joined inside a function body are outside its scope by construction, and remain review only.

#### Scenario: A second spelling by name fails

- **WHEN** a non-test file of any package of a fixture module declares an exported type `FederationMeta`, a function
  `WithFederation`, a function `BuildGlobalID`, a function `EntityIRI`, a method `EntityIRI` on an exported type, a
  constant `FederationLane`, or an exported struct field `GlobalID`, in a public, an internal or a `main` package
- **THEN** the contract test fails naming each identifier with its file and line

#### Scenario: Names the rule does not cover pass

- **WHEN** a fixture module declares an unexported `federationMeta`, an identifier containing `federation` in lower
  case only, a test-file identifier `TestFederation`, or a comment and a string literal that say `BuildGlobalID`
- **THEN** the contract test reports nothing for them

#### Scenario: An authority field outside the owners fails

- **WHEN** a non-test file of a fixture module declares, outside the excepted packages, an exported struct field
  `Org` or `Platform` of type `string` on a named struct (the shape of `graph/inference.HierarchyConfig` and
  `graph/llm.EntityParts` at the pin), on an anonymous struct, or on a struct in an internal or a `main` package
- **THEN** the contract test fails naming each field with its file, line and struct type

#### Scenario: The owners, the carrier and the caller's claim pass

- **WHEN** a fixture module declares `Org` and `Platform` fields in its `pkg/types`, `pkg/platform` and `config`
  packages; a type `types.PlatformMeta` with fields `Org` and `Platform`; a field `Platform` of type
  `types.PlatformMeta` on a struct in any other package; a field `Org` on `processor/rule.CallerContext`; and
  unexported fields `org` and `platform` of type `string` anywhere
- **THEN** the contract test reports nothing for any of them

#### Scenario: A test file is not checked

- **WHEN** a `_test.go` file declares an exported struct field `Org` or an exported function `EntityIRI`
- **THEN** neither contract test reports it

#### Scenario: This module passes

- **WHEN** both contract tests run over this module
- **THEN** they pass; on `main` at `2ec3bcf` the only Go is the harness, which declares no such name and no such
  field, and after PR #48 the floor's packages declare `Org` and `Platform` only in `pkg/types` and `pkg/platform`
  and no identifier the name check matches
