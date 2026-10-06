# harness-boundaries

## MODIFIED Requirements

### Requirement: One image pin

The NATS image SHALL be spelled only in `.nats-image` as `nats:<tag>@sha256:<digest>`; scripts and Go SHALL receive it
through SEMENGINE_NATS_IMAGE. The check covers every tracked file that configures or runs Docker (`*.go`, `*.sh`,
`Taskfile.yml`, `*.yaml`/`*.yml`, Dockerfiles, CI workflows), including variable tags such as `nats:${TAG}`; Markdown
and other prose may cite the tag. In a `*.go` file the check reports `nats:` only when a digit or a variable (`$`,
`${`) follows it, and reports every `nats@sha256:`; a Go literal `nats:` followed by a letter, such as a component port
identifier `nats:<subject>` or a tag such as `nats:latest`, is not reported, and an image tag that starts with a letter
in Go is review only.

#### Scenario: Literal elsewhere

- **WHEN** a tracked file that configures or runs Docker, other than .nats-image, contains a `nats:` image literal
- **THEN** the contract test fails

#### Scenario: A port identifier in a Go file

- **WHEN** a Go file contains the literal `"nats:in"` or `"nats:sensor.data"`
- **THEN** the contract test reports nothing for it

#### Scenario: An image in a Go file

- **WHEN** a Go file contains the literal `"nats:2.10"`, `"nats:${TAG}"`, or a `nats@sha256:` digest in a constant in a
  file with no imports
- **THEN** the contract test fails naming the file and line

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
caller's organization claim, which is not the deployment authority); until the change that removes the package
(`setup-04a-07-substrate-seam`), the two fields of `graph/llm.EntityParts`, which is carried unchanged and unused
(owner question B of `setup-04a-02-ingest-kernel`); and the carrier `deps.Platform`, by exact field:
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
  `Org` or `Platform` of type `string` on a named struct (the shape of `graph/inference.HierarchyConfig` at the
  pin), on an anonymous struct, or on a struct in an internal or a `main` package;
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

### Requirement: Public signatures name no internal type

A public package is a non-test, non-`main` package of this module whose import path has no `internal` element. No
exported identifier of a public package SHALL name a type declared in a package of this module with an `internal`
path element (Go's rule for what a caller outside the module cannot import), directly or through what a caller
outside the module reaches from it: the exported methods of a type it names, its exported and embedded struct fields,
an interface's method set (embedded interfaces included), type arguments, generic constraints, and alias targets. A
type an exported identifier reaches is followed whether it is exported or not. Function bodies and unexported
identifiers are not checked; a caller outside the module cannot reach them.

One exception, listed here and nowhere else: a parameter of type `componentadmission.Access`, declared in
`internal/componentadmission` as a struct with no fields, of the `component.Registry` methods `CreateComponent`,
`SealComposition` and `Snapshots`. It is an access token that keeps those methods to the engine's own component
manager (owner question C of `setup-04a-02-ingest-kernel`). The token type SHALL have no fields and no
methods, and no other exported identifier SHALL name it.

#### Scenario: A planted violation fails

- **WHEN** a fixture module's public package declares an exported identifier that names a type declared in a package
  with an `internal` path element directly (a function result, an exported variable or constant, an exported struct
  field), or only through an exported method, an embedded field, an interface method set, a type argument, a generic
  constraint on a function or a type, an alias, or an unexported type that an exported function returns
- **THEN** the contract test fails naming each identifier and the internal type it reaches

#### Scenario: A clean tree passes

- **WHEN** a fixture module's public package uses a type declared under its `internal/` only in unexported
  identifiers and function bodies
- **THEN** the contract test reports nothing for that package

#### Scenario: The access token is the only exception

- **WHEN** a fixture module's public package has the three `Registry` methods taking the field-less token, and
  another exported function that takes the same token
- **THEN** the contract test reports the other function and not the three methods

#### Scenario: The floor passes

- **WHEN** the contract test runs over this module with the floor's eleven public packages ported
- **THEN** it passes: no exported identifier names a type under `internal/` (the pin's one,
  `natsclient.TemporalResolver.GetStats`, is dropped with `TemporalResolver`; owner ruling, #9 comment 5969522395,
  item 1)
