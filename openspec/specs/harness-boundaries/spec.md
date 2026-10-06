# harness-boundaries Specification

## Purpose
Harness boundaries are the machine-checked rules that keep test machinery out of production code, keep SemEngine's
Docker footprint invisible to SemStreams' substring cleanup, keep every reused SemStreams file traceable in the
admission ledger, and hold the machine-checked shape rules over ported production code, such as the one spelling of
a deployment's authority.

## Requirements

### Requirement: Import graph

No non-test Go file outside `internal/harness/` SHALL import `internal/harness/...`, `testcontainers-go`, `testing`,
or `gopkg.in/yaml.v3`. No production package SHALL import the component factories of more than one component family;
the consumer's composition root is the only place factories and payload registrations are aggregated. No Go file in
the module, test or non-test, SHALL import `github.com/c360studio/semstreams` or any path under it, and `go.mod`
SHALL NOT require it. `internal/harness/natsfixture` SHALL import no package of this module outside
`internal/harness/`; any other `internal/harness/` package MAY import a package of this module only when that package
starts no goroutine and holds no resource beyond a call, and no test of any package in the helper's dependency
closure imports the helper.

#### Scenario: Production package imports the fixture

- **WHEN** a non-test file outside internal/harness imports internal/harness/natsfixture
- **THEN** the contract test fails naming the file

#### Scenario: An aggregator package appears

- **WHEN** a production package imports `Register` from two or more component packages
- **THEN** the contract test fails naming the package

#### Scenario: A file imports SemStreams

- **WHEN** any Go file in the module, test or non-test, imports `github.com/c360studio/semstreams/...`, or `go.mod`
  requires it
- **THEN** the contract test fails naming the file and the import

#### Scenario: A test helper lives in the harness

- **WHEN** a non-test file under `internal/harness/semantictest` imports `testing` and `pkg/types`
- **THEN** the contract test reports no violation

#### Scenario: The fixture imports a ported package

- **WHEN** a file under `internal/harness/natsfixture` imports `natsclient`
- **THEN** the contract test fails naming the file

### Requirement: No retained context

No struct type in a non-test file SHALL hold a context.Context directly, embedded, aliased, in a container, as a
type argument of a generic holder (`atomic.Pointer[context.Context]`), or as a provider result; unexported
context.CancelFunc fields are permitted, exported ones are rejected. The one exception, listed by exact type name in
this requirement and nowhere else, is `github.com/c360studio/semengine/internal/harness/probe.ObservedContext`: it is
itself a context.Context implementation and holds its parent the way the standard library's derived contexts do.
The check is static: a context stored in an untyped holder (`any`, `interface{}`, `atomic.Value`) or captured by a
closure is invisible to it, and those shapes are out of its scope by construction.

#### Scenario: Fixture struct is checked

- **WHEN** natsfixture.Fixture declares a context.Context field
- **THEN** the contract test fails

#### Scenario: Generic holder is checked

- **WHEN** a non-test struct declares an `atomic.Pointer[context.Context]` field
- **THEN** the contract test fails

### Requirement: One image pin

The NATS image SHALL be spelled only in `.nats-image` as `nats:<tag>@sha256:<digest>`; scripts and Go SHALL receive it
through SEMENGINE_NATS_IMAGE. The check covers every tracked file that configures or runs Docker (`*.go`, `*.sh`,
`Taskfile.yml`, `*.yaml`/`*.yml`, Dockerfiles, CI workflows), including variable tags such as `nats:${TAG}`; Markdown
and other prose may cite the tag.

#### Scenario: Literal elsewhere

- **WHEN** a tracked file that configures or runs Docker, other than .nats-image, contains a `nats:` image literal
- **THEN** the contract test fails

### Requirement: SemEngine-assigned names

Every Docker name assigned by this repository (container `--name`/`container_name`, volume, network, Compose project
via `-p`/`--project-name`/`COMPOSE_PROJECT_NAME`/top-level `name:`) SHALL start with `semengine-`, use only
`[a-z0-9-]`, and contain none of `semstreams`, `nats-semstreams`, `semembed`, `agentic`, `crud-tools`,
`deep-research`, `ops`, `research-graph` anywhere in the name, including inside a longer word; generated names SHALL
satisfy the same rule by construction. Compose invocations SHALL always pass an explicit project name.

#### Scenario: Destroyer substring in a literal

- **WHEN** a script names a volume `semengine-ops-cache`
- **THEN** the contract test fails citing `ops`

#### Scenario: Destroyer substring inside an allow-listed-looking word

- **WHEN** a lane segment is a word such as `stops` or `loops`
- **THEN** the contract test fails citing `ops`, because Docker's `--filter name=ops` matches any part of a name

#### Scenario: SemStreams routine cleanup cannot select SemEngine resources

- **WHEN** `docker volume ls -q --filter name=<each destroyer>` is evaluated while SemEngine resources exist
- **THEN** no SemEngine resource is listed

### Requirement: No fixed addresses in tests

No `*_test.go` file SHALL contain a fixed broker address (`nats://localhost:`, `nats://127.0.0.1:`, `:4222"`,
`:8222"`) or bind a fixed TCP port (scripts/lint-test-ports.sh SHALL pass). `scripts/lint-test-ports.sh` SHALL
honour no inline exemption, and its failure output SHALL name the offending line and tell the author to bind port 0
and hand the listener itself to the code under test, naming no file outside this repository.

#### Scenario: Fixed dial

- **WHEN** a test dials nats://localhost:4222
- **THEN** the contract test fails naming the line

#### Scenario: Marked fixed port

- **WHEN** a test binds a fixed port on a line that ends with the comment `// gh#220:allow-fixed-port`
- **THEN** scripts/lint-test-ports.sh exits 1 naming the line

#### Scenario: Guidance keeps the listener

- **WHEN** scripts/lint-test-ports.sh fails on a fixed port
- **THEN** its output tells the author to bind port 0 and pass the listener on, and names no SemStreams file

### Requirement: Bounded cleanup roots

`task verify` SHALL fail when any `*_test.go` file calls Stop, Close, or Terminate with context.Background() or
context.TODO(). The sanctioned harness cleanup roots (`natsfixture.New`'s cleanup Stop under a fresh bounded
context, and the rollback helper's bounded `WithoutCancel` context) live in non-test files, so the check has no
allowlist.

#### Scenario: Unbounded defer

- **WHEN** a test adds `defer o.Stop(context.Background())`
- **THEN** cleanup-roots-check fails naming the line

### Requirement: No broad Docker cleanup

No script or task SHALL invoke `docker … prune`, `docker volume ls … --filter name=`, `docker ps … --filter name=`
(the short `-f` included), `xargs … docker volume rm`, or any Compose subcommand without an explicit `semengine-`
project name. A backslash-continued command is checked as one command.

#### Scenario: Substring removal added

- **WHEN** a script gains `docker volume ls -q --filter name=semengine | xargs docker volume rm`
- **THEN** the contract test fails

### Requirement: Admission ledger

`docs/admission-ledger.yaml` SHALL be a list of entries each with non-empty source_path, source_sha (40 lowercase
hex), consumer_purpose, destination, contract, dependencies_and_side_effects, known_risks, proving_tests, owner, and
disposition ∈ {carry, adapt, repair-before-port, defer-exclude}; source_path SHALL be unique; `task verify` SHALL fail
on any violation.

#### Scenario: Short SHA

- **WHEN** an entry's source_sha is `5457b345`
- **THEN** ledger:check fails naming the entry

#### Scenario: Unknown disposition

- **WHEN** an entry's disposition is `port`
- **THEN** ledger:check fails naming the entry

### Requirement: No sleeps in tests

No `*_test.go` file and no Go file under `internal/harness/` SHALL contain `time.Sleep`. The check SHALL have no
baseline, no allowlist and no inline exemption, and SHALL fail when it scanned no test file. A test file ported from
SemStreams SHALL meet the check before it lands; there is no list of accepted files. The check matches the text
`time.Sleep`: a renamed `time` import and a wait built from `time.After` or a timer are outside its scope by
construction.

#### Scenario: Sleep added to a test

- **WHEN** a test file gains `time.Sleep(10 * time.Millisecond)`
- **THEN** the contract test fails naming the file and line

#### Scenario: Sleep added to harness code

- **WHEN** a Go file under `internal/harness/` that is not a test file gains `time.Sleep(time.Second)`
- **THEN** the contract test fails naming the file and line

#### Scenario: Nothing scanned

- **WHEN** the check is given a tree with no `*_test.go` file
- **THEN** the contract test fails saying it scanned no test file

### Requirement: No skipped or hidden tests

No `*_test.go` file SHALL call `Skip`, `Skipf` or `SkipNow`, and no `*_test.go` file SHALL carry a build constraint
other than `//go:build integration`. The check SHALL have no baseline, no allowlist and no inline exemption, and SHALL
fail when it scanned no test file. A test file ported from SemStreams SHALL meet the check before it lands; there is
no list of accepted files. A test that needs a real broker MAY be moved into a file tagged `integration`, and it still
calls no skip. The check matches text: a skip reached through a helper and a platform suffix in a file name are
outside its scope by construction.

#### Scenario: Skip added to a test

- **WHEN** a test file gains `t.Skip("flaky")`
- **THEN** the contract test fails naming the file and line

#### Scenario: Test hidden behind a build tag

- **WHEN** a test file begins with `//go:build flaky`
- **THEN** the contract test fails naming the file and the constraint

#### Scenario: Skipped test moved behind the integration tag

- **WHEN** a test that was skipped for want of a real broker is moved into a file whose first line is
  `//go:build integration`, and its skip call is removed
- **THEN** the contract test passes

### Requirement: Comparison with the pin

A ledger entry SHALL be compared with SemStreams by one rule, used by both `task ledger:check` and
`task ledger:diff`. The pin side is `source_path` at `source_sha`, fetched from the SemStreams repository. The tree
side is the entry's destination path: the leading run of letters, digits, `/`, `.`, `_` and `-` in `destination`;
anything after it is a note. A destination path SHALL be relative to the repository root and inside the repository:
one that starts with `/`, has a `..` segment, or resolves outside the repository is treated as a path that does not
exist.

When `source_path` is a file at the pin, the entry covers that file and the destination path SHALL be a file. When
it is a directory, the entry covers the `.go` files directly in it, test files included, and every file under its
`testdata` directory, and the same selection is made in the destination directory. Other files directly in the
directory, such as `README.md`, and other subdirectories, a new sub-package included, are not covered.

Before comparing, the pin side of a `.go` file outside `testdata` SHALL be rewritten, in import paths and in
comments only: `github.com/c360studio/semstreams` becomes `github.com/c360studio/semengine`. Where it is followed by
`/` and a package path that is exactly the `source_path` of a moved package, the package path becomes that package's
destination path. A package path ends before the first character that is not a letter, a digit, `/`, `_`, `-` or
`.`, and does not include a trailing `.` or `/`. A moved package is a `carry` or `adapt` entry with the same
`source_sha` as the file, whose `source_path` is a directory at the pin and whose destination path is a directory in
the tree and differs from `source_path`; an entry with any other disposition moves nothing. Text outside import paths
and comments, string literals included, SHALL NOT be rewritten. Both sides of the file SHALL then be formatted with
gofmt. A `.go` file gofmt cannot parse, and every other covered file, is compared as it stands. Two files are equal
when the results are byte-identical.

An entry SHALL be reported as not compared, with the reason, when its destination path does not exist in the tree,
when `source_path` does not exist at `source_sha`, when one side is a file and the other a directory, or when it is
a directory entry that covers no file at the pin and none in the tree.

#### Scenario: Module path in an import and a comment

- **WHEN** a pin file has `github.com/c360studio/semstreams/pkg/errs` in an import and in a doc comment, and the tree
  file has `github.com/c360studio/semengine/pkg/errs` in the same two places and is otherwise identical
- **THEN** the files are equal

#### Scenario: Module path in a string literal

- **WHEN** a pin file holds `github.com/c360studio/semstreams/natsclient` in a string literal and the tree file
  holds `github.com/c360studio/semengine/natsclient` there
- **THEN** the file is reported as different

#### Scenario: Import of a package the ledger moved

- **WHEN** the `carry` entry for `internal/semantictest` has destination `internal/harness/semantictest`, a pin file
  imports `github.com/c360studio/semstreams/internal/semantictest`, and the tree file imports
  `github.com/c360studio/semengine/internal/harness/semantictest`
- **THEN** the files are equal

#### Scenario: Excluded entry moves nothing

- **WHEN** a `defer-exclude` entry for the package `pkg/old` has a destination path that is a directory in the tree,
  a pin file imports `github.com/c360studio/semstreams/pkg/old`, and the tree file imports that directory instead
- **THEN** the file is reported as different

#### Scenario: Imports sorted differently after a move

- **WHEN** a moved package's new import path sorts to a different line of the import block in the tree file
- **THEN** the files are equal

#### Scenario: Package path in a doc link

- **WHEN** a pin comment holds `[github.com/c360studio/semstreams/pkg/tlsutil]:` and `pkg/tlsutil` is a moved package
- **THEN** the package path rewritten is `pkg/tlsutil`, and `]:` is left as it is

#### Scenario: Note after the destination path

- **WHEN** an entry's destination is `scripts/lint-test-ports.sh (adapted by change flake-defense)`
- **THEN** the tree side is the file `scripts/lint-test-ports.sh`

#### Scenario: Destination outside the repository

- **WHEN** an entry's destination is `../elsewhere/pkg` or `/tmp/pkg`, and a directory exists there
- **THEN** the entry is reported as not compared, because its destination is not a path in this repository

#### Scenario: README is not covered

- **WHEN** `README.md` in a package directory differs from the pin and every `.go` file is equal
- **THEN** no difference is reported for the entry

#### Scenario: Test data is covered

- **WHEN** a file under the package's `testdata` directory differs from the pin
- **THEN** the file is reported as different

#### Scenario: File at the pin, directory in the tree

- **WHEN** `source_path` is a file at the pin and the destination path is a directory
- **THEN** the entry is reported as not compared, with that reason

#### Scenario: Directory entry with nothing covered

- **WHEN** `source_path` is a directory at the pin that holds only `README.md` and a sub-directory, and the
  destination directory holds only `README.md`
- **THEN** the entry is reported as not compared, because no file is covered at the pin or in the tree

### Requirement: Carried entries match the pin

After the schema check, `task ledger:check` SHALL run a program that compares every entry whose disposition is `carry`,
test files included. The program SHALL exit 1 when a covered file differs from the pin, exists only at the pin, or
exists only in the tree, or when a `carry` entry cannot be compared. Each failure SHALL name the entry's `source_path`,
the file, and which of those four it is. The output SHALL say that `task ledger:diff -- <source_path>` prints the lines,
and that a package that differs is `adapt` (`docs/provenance.md` rule 5). Entries with any other disposition SHALL NOT
be compared by the check. With no `carry` entry the program SHALL exit 0 without contacting SemStreams. When SemStreams
cannot be read within the fetch bound, which is two minutes for all the fetches of one run together, the program SHALL
exit 2 with a message that says the pin could not be read and that no entry was checked. When a fetch fails or is cut
off, by the fetch bound or by an interrupt, no process the program started for it SHALL still be running when the
program exits.
The program SHALL write nothing inside the repository. `task ledger:check` SHALL fail whenever the program exits
non-zero; `task` and `go run` replace the program's exit status with their own, so through `task` only pass or fail and
the message are promised.

#### Scenario: Carried package unchanged

- **WHEN** every covered file of a `carry` entry equals the pin
- **THEN** ledger:check passes

#### Scenario: One line edited in a carried file

- **WHEN** one line of a non-test `.go` file under a `carry` entry is edited
- **THEN** ledger:check fails naming the entry and the file

#### Scenario: Test repaired in a carried package

- **WHEN** one line of a `_test.go` file under a `carry` entry is edited
- **THEN** ledger:check fails naming the entry and the file

#### Scenario: File left out

- **WHEN** a `.go` file at the pin is absent from the destination directory of a `carry` entry
- **THEN** ledger:check fails naming the entry and the file as only at the pin

#### Scenario: File added

- **WHEN** the destination directory of a `carry` entry holds a `.go` file the pin does not have
- **THEN** ledger:check fails naming the entry and the file as only in the tree

#### Scenario: Destination names no path

- **WHEN** a `carry` entry's destination path does not exist in the tree
- **THEN** ledger:check fails naming the entry and the reason

#### Scenario: Adapted entry differs

- **WHEN** an `adapt` entry's files differ from the pin and no `carry` entry does
- **THEN** ledger:check passes

#### Scenario: No carried entry

- **WHEN** the ledger has no `carry` entry and SemStreams cannot be reached
- **THEN** ledger:check passes

#### Scenario: Fetch cut off at the bound

- **WHEN** a fetch still waiting on SemStreams is cut off, by the fetch bound or by an interrupt
- **THEN** the program exits 2, and no process it started for the fetch, git's transport helper included, is still
  running when it returns

#### Scenario: Pin cannot be read

- **WHEN** the ledger has a `carry` entry and SemStreams cannot be reached
- **THEN** the program exits 2 saying the pin could not be read and no entry was checked, and ledger:check fails

### Requirement: Pin difference command

`task ledger:diff -- <source_path>...` SHALL run the same program to compare each named entry and, with no argument,
every entry whose disposition is `carry` or `adapt`, in ledger order. For each covered file that differs it SHALL
print on standard output a unified diff from the rewritten pin text, labelled `pin/` followed by the file's
SemStreams path, to the tree file, labelled with its path. For a file on one side only it SHALL print
`only at the pin: pin/<path>` or `only in the tree: <path>`. An equal file prints nothing, so standard output is empty
when nothing differs. On standard error it SHALL print one line per entry: the `source_path`, the disposition, and
either the number of files compared (covered files present on both sides), differing, only at the pin and only
in the tree, or `not compared` with the reason.
The program SHALL exit 0 when it ran, whether or not files differ, and 2 when an argument names no entry,
when a named entry could not be compared, or when the ledger or the pin could not be read; through `task` only pass
or fail and the message are promised. It SHALL write nothing inside the repository.

#### Scenario: Unchanged package prints nothing

- **WHEN** every covered file of the named entry equals the pin
- **THEN** standard output is empty and the program exits 0

#### Scenario: Changed line is printed

- **WHEN** one line of a covered file differs from the pin
- **THEN** standard output holds a unified diff with the pin's line and the tree's line, and the program exits 0

#### Scenario: Unknown source path

- **WHEN** an argument is not the `source_path` of any entry
- **THEN** the program exits 2 naming the argument

#### Scenario: Entry that cannot be compared, no argument given

- **WHEN** no argument is given and an `adapt` entry has a file at the pin and a directory in the tree
- **THEN** standard error says the entry was not compared and why, and the program exits 0

### Requirement: No bare select

No Go file in the module, test or non-test, `package main` included, SHALL contain a `select` statement with no
cases. A goroutine parked on one has no way out, and in a re-executed test binary, which runs with no test-timeout
timer, Go's deadlock detector kills the process once every goroutine blocks. Code parks on `ctx.Done()`, a channel or
`signal.Notify`; a `main` uses `signal.NotifyContext`. The check parses each file, so it matches code only: a comment
or a string that mentions the pattern does not trip it, and every spelling of an empty `select` (spacing, newlines,
a comment inside) does. A file that does not parse fails the check. It sits beside "No sleeps in tests" and has, like
it, no baseline, no allowlist and no inline exemption.

#### Scenario: A bare select is added

- **WHEN** a test file, a non-test file or a `package main` file contains `select {}`, in any spacing
- **THEN** the contract test fails naming the file and line

#### Scenario: A select with cases or a comment

- **WHEN** a file contains a `select` with at least one case, or mentions `select {}` only in a comment or a string
- **THEN** the contract test reports nothing for that file

### Requirement: Public signatures name no internal type

A public package is a non-test, non-`main` package of this module whose import path has no `internal` element. No
exported identifier of a public package SHALL name a type declared in a package of this module with an `internal`
path element (Go's rule for what a caller outside the module cannot import), directly or through what a caller
outside the module reaches from it: the exported methods of a type it names, its exported and embedded struct fields,
an interface's method set (embedded interfaces included), type arguments, generic constraints, and alias targets. A
type an exported identifier reaches is followed whether it is exported or not. Function bodies and unexported
identifiers are not checked; a caller outside the module cannot reach them.

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

#### Scenario: The floor passes

- **WHEN** the contract test runs over this module with the floor's eleven public packages ported
- **THEN** it passes: no exported identifier names a type under `internal/` (the pin's one,
  `natsclient.TemporalResolver.GetStats`, is dropped with `TemporalResolver`; owner ruling, #9 comment 5969522395,
  item 1)

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
