# harness-boundaries Specification

## Purpose

Harness boundaries are the machine-checked rules that keep test machinery out of production code, keep SemEngine's
Docker names and cleanup narrow enough that they never select another repository's resources, keep every reused
SemStreams file traceable in the admission ledger, and hold the machine-checked shape rules over ported production
code, such as the one spelling of a deployment's authority.

## Requirements

### Requirement: Import graph

No non-test Go file outside `internal/harness/` SHALL import `internal/harness/...`, `testcontainers-go`, `testing`, or
(except `vocabulary/pack.go` for declarative YAML vocabulary packs) `gopkg.in/yaml.v3`.

#### Scenario: Production package imports the fixture

- **WHEN** a non-test file outside internal/harness imports internal/harness/natsfixture
- **THEN** the contract test fails naming the file

#### Scenario: A test helper lives in the harness

- **WHEN** a non-test file under `internal/harness/semantictest` imports `testing` and `pkg/types`
- **THEN** the contract test reports no violation

#### Scenario: Declarative vocabulary pack loader imports yaml.v3

- **WHEN** `vocabulary/pack.go` imports `gopkg.in/yaml.v3` and another non-test file outside `internal/harness/`
  imports `gopkg.in/yaml.v3`
- **THEN** the contract test permits `vocabulary/pack.go` and fails naming the other file

### Requirement: Production packages import one component family at most

No production package SHALL import the component factories of more than one component family; the consumer's composition
root is the only place factories and payload registrations are aggregated.

#### Scenario: An aggregator package appears

- **WHEN** a production package imports `Register` from two or more component packages
- **THEN** the contract test fails naming the package

### Requirement: No Go file imports SemStreams

No Go file in the module, test or non-test, SHALL import `github.com/c360studio/semstreams` or any path under it, and
`go.mod` SHALL NOT require it.

#### Scenario: A file imports SemStreams

- **WHEN** any Go file in the module, test or non-test, imports `github.com/c360studio/semstreams/...`, or `go.mod`
  requires it
- **THEN** the contract test fails naming the file and the import

### Requirement: Harness packages import little of this module

`internal/harness/natsfixture` SHALL import no package of this module outside `internal/harness/`; any other
`internal/harness/` package MAY import a package of this module only when that package starts no goroutine and holds no
resource beyond a call, and no test of any package in the helper's dependency closure imports the helper.

#### Scenario: The fixture imports a ported package

- **WHEN** a file under `internal/harness/natsfixture` imports `natsclient`
- **THEN** the contract test fails naming the file

### Requirement: No retained context

No struct type in a non-test file SHALL hold a context.Context directly, embedded, aliased, in a container, as a type
argument of a generic holder (`atomic.Pointer[context.Context]`), or as a provider result; unexported context.CancelFunc
fields are permitted, exported ones are rejected. One type is excepted, as "One type may hold its parent context"
states.

#### Scenario: Fixture struct is checked

- **WHEN** natsfixture.Fixture declares a context.Context field
- **THEN** the contract test fails

#### Scenario: Generic holder is checked

- **WHEN** a non-test struct declares an `atomic.Pointer[context.Context]` field
- **THEN** the contract test fails

### Requirement: One type may hold its parent context

The one exception to "No retained context", listed by exact type name in this requirement and nowhere else, is
`github.com/c360studio/semengine/internal/harness/probe.ObservedContext`, which SHALL be allowed to hold a context: it
is itself a context.Context implementation and holds its parent the way the standard library's derived contexts do.

#### Scenario: The excepted type holds its parent

- **WHEN** `probe.ObservedContext` holds its parent context
- **THEN** the contract test reports no violation for it

### Requirement: The retained-context check is static

The check SHALL be static: a context stored in an untyped holder (`any`, `interface{}`, `atomic.Value`) or captured by a
closure is invisible to it, and those shapes are out of its scope by construction.

#### Scenario: An untyped holder

- **WHEN** a non-test struct holds a context in an `any` field
- **THEN** the contract test reports nothing for it

### Requirement: One image pin

The NATS image SHALL be spelled only in `internal/harness/natsfixture/nats-image` as `nats:<tag>@sha256:<digest>`;
natsfixture SHALL embed that file, SHALL refuse a reference without `@sha256:<digest>` before any Docker call, and
`scripts/doctor.sh` SHALL read that file.

#### Scenario: Digest pin ignores tag movement

- **WHEN** another repository re-pulls the mutable `nats:2.15.0-alpine` tag
- **THEN** natsfixture's containers still run the image the pin's digest names, because the pin is a digest reference
  and natsfixture refuses one without a digest

### Requirement: The image-pin check covers every Docker file

The check SHALL cover every tracked file that configures or runs Docker (`*.go`, `*.sh`, `Taskfile.yml`,
`*.yaml`/`*.yml`, Dockerfiles, CI workflows), including variable tags such as `nats:${TAG}`; Markdown and other prose
may cite the tag.

#### Scenario: Literal elsewhere

- **WHEN** a tracked file that configures or runs Docker, other than `internal/harness/natsfixture/nats-image`,
  contains a `nats:` image literal
- **THEN** the contract test fails

### Requirement: In Go files the image-pin check reports image literals only

In a `*.go` file the check SHALL report `nats:` only when a digit or a variable (`$`, `${`) follows it, and SHALL report
every `nats@sha256:`; a Go literal `nats:` followed by a letter, such as a component port identifier `nats:<subject>` or
a tag such as `nats:latest`, SHALL NOT be reported, and an image tag that starts with a letter in Go is review only.

#### Scenario: A port identifier in a Go file

- **WHEN** a Go file contains the literal `"nats:in"` or `"nats:sensor.data"`
- **THEN** the contract test reports nothing for it

#### Scenario: An image in a Go file

- **WHEN** a Go file contains the literal `"nats:2.10"`, `"nats:${TAG}"`, or a `nats@sha256:` digest in a constant in a
  file with no imports
- **THEN** the contract test fails naming the file and line

### Requirement: Docker names this repository assigns

Every Docker name a script, task or Compose file in this repository assigns (container `--name`/`container_name`,
volume, network, Compose project via `-p`/`--project-name`/`COMPOSE_PROJECT_NAME`/top-level `name:`) SHALL start with
`semengine-` and use only `[a-z0-9-]`. Compose invocations SHALL always pass an explicit project name. natsfixture
assigns no container name; its containers are found by their label (`nats-fixture`, "Ryuk off, labelled containers
and the sweep").

#### Scenario: Name without the prefix

- **WHEN** a script names a volume `cache`
- **THEN** the contract test fails naming the line

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
context.TODO(). The harness's own cleanup roots (`natsfixture.New`'s cleanup Stop and the removal of a failed start's
container, each under a fresh bounded context) live in non-test files, and a package's `TestMain` that stops a shared
server does so under `context.WithTimeout`, so the check has no allowlist.

#### Scenario: Unbounded defer

- **WHEN** a test adds `defer o.Stop(context.Background())`
- **THEN** cleanup-roots-check fails naming the line

#### Scenario: Unbounded shared-server stop

- **WHEN** a `TestMain` calls `s.Stop(context.Background())`
- **THEN** cleanup-roots-check fails naming the line

### Requirement: Integration tests in a package run one at a time

No Go file carrying `//go:build integration` SHALL call `t.Parallel()`, so that the tests sharing a package's NATS
server never run at the same time. The check SHALL have no allowlist and no inline exemption, and a paired sensitivity
test SHALL show it fails on a planted call. A `Parallel()` call in an untagged helper, or on a receiver not named `t`,
escapes the check and is review only.

#### Scenario: Parallel integration test

- **WHEN** an integration-tagged test file calls `t.Parallel()`
- **THEN** the contract test fails naming the file and line

#### Scenario: Parallel unit test

- **WHEN** a test file with no build tag calls `t.Parallel()`
- **THEN** the contract test reports nothing for it

### Requirement: A test with its own server says why

Every call to `natsfixture.New` in a `*_test.go` file outside `internal/harness/natsfixture` SHALL have, on the line
above it, a comment beginning `// Own server:` followed by a reason. A paired sensitivity test SHALL show the check
fails on a planted call without one. The check reads call sites: a package helper that calls `New` once for many
tests, a renamed import and a function value are review only, as is whether the reason holds (a restart, a stop or a
server option).

#### Scenario: Own server without a reason

- **WHEN** a product test calls `natsfixture.New(t)` with no `// Own server:` comment on the line above
- **THEN** the contract test fails naming the file and line

#### Scenario: Own server with a reason

- **WHEN** the line above the call reads `// Own server: restarts the server mid-test.`
- **THEN** the contract test reports nothing for it

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
fail when it scanned no test file.

#### Scenario: Skip added to a test

- **WHEN** a test file gains `t.Skip("flaky")`
- **THEN** the contract test fails naming the file and line

#### Scenario: Test hidden behind a build tag

- **WHEN** a test file begins with `//go:build flaky`
- **THEN** the contract test fails naming the file and the constraint

### Requirement: A ported test file meets the skip check before it lands

A test file ported from SemStreams SHALL meet the check before it lands; there is no list of accepted files. A test that
needs a real broker MAY be moved into a file tagged `integration`, and it still calls no skip.

#### Scenario: Skipped test moved behind the integration tag

- **WHEN** a test that was skipped for want of a real broker is moved into a file whose first line is
  `//go:build integration`, and its skip call is removed
- **THEN** the contract test passes

### Requirement: The skip check matches text

The check SHALL match text: a skip reached through a helper and a platform suffix in a file name are outside its scope
by construction.

#### Scenario: A skip through a helper

- **WHEN** a test calls a helper that calls `t.Skip`
- **THEN** the contract test reports nothing for the helper call

### Requirement: Comparison with the pin

A ledger entry SHALL be compared with SemStreams by one rule, used by both `task ledger:check` and `task ledger:diff`.
The pin side is `source_path` at `source_sha`, fetched from the SemStreams repository. The tree side is the entry's
destination path: the leading run of letters, digits, `/`, `.`, `_` and `-` in `destination`; anything after it is a
note.

#### Scenario: Note after the destination path

- **WHEN** an entry's destination is `scripts/lint-test-ports.sh (adapted by change flake-defense)`
- **THEN** the tree side is the file `scripts/lint-test-ports.sh`

### Requirement: A destination path stays inside the repository

A destination path SHALL be relative to the repository root and inside the repository: one that starts with `/`, has a
`..` segment, or resolves outside the repository is treated as a path that does not exist.

#### Scenario: Destination outside the repository

- **WHEN** an entry's destination is `../elsewhere/pkg` or `/tmp/pkg`, and a directory exists there
- **THEN** the entry is reported as not compared, because its destination is not a path in this repository

### Requirement: A file entry covers one file

When `source_path` is a file at the pin, the entry covers that file and the destination path SHALL be a file.

#### Scenario: A file entry

- **WHEN** `source_path` is a file at the pin and the destination path is a file
- **THEN** only those two files are compared

### Requirement: A directory entry covers its Go files and test data

When `source_path` is a directory, the entry SHALL cover the `.go` files directly in it, test files included, and every
file under its `testdata` directory, and the same selection SHALL be made in the destination directory.

#### Scenario: Test data is covered

- **WHEN** a file under the package's `testdata` directory differs from the pin
- **THEN** the file is reported as different

### Requirement: A directory entry leaves other files uncovered

Other files directly in the directory, such as `README.md`, and other subdirectories, a new sub-package included, SHALL
NOT be covered.

#### Scenario: README is not covered

- **WHEN** `README.md` in a package directory differs from the pin and every `.go` file is equal
- **THEN** no difference is reported for the entry

### Requirement: The pin side is rewritten to this module's paths

Before comparing, the pin side of a `.go` file outside `testdata` SHALL be rewritten, in import paths and in comments
only: `github.com/c360studio/semstreams` becomes `github.com/c360studio/semengine`. Where it is followed by `/` and a
package path that is exactly the `source_path` of a moved package, the package path becomes that package's destination
path.

#### Scenario: Module path in an import and a comment

- **WHEN** a pin file has `github.com/c360studio/semstreams/pkg/errs` in an import and in a doc comment, and the tree
  file has `github.com/c360studio/semengine/pkg/errs` in the same two places and is otherwise identical
- **THEN** the files are equal

#### Scenario: Import of a package the ledger moved

- **WHEN** the `carry` entry for `internal/semantictest` has destination `internal/harness/semantictest`, a pin file
  imports `github.com/c360studio/semstreams/internal/semantictest`, and the tree file imports
  `github.com/c360studio/semengine/internal/harness/semantictest`
- **THEN** the files are equal

### Requirement: Where a package path ends

A package path SHALL end before the first character that is not a letter, a digit, `/`, `_`, `-` or `.`, and does not
include a trailing `.` or `/`.

#### Scenario: Package path in a doc link

- **WHEN** a pin comment holds `[github.com/c360studio/semstreams/pkg/tlsutil]:` and `pkg/tlsutil` is a moved package
- **THEN** the package path rewritten is `pkg/tlsutil`, and `]:` is left as it is

### Requirement: Only carry and adapt entries move a package

A moved package is a `carry` or `adapt` entry with the same `source_sha` as the file, whose `source_path` is a directory
at the pin and whose destination path is a directory in the tree and differs from `source_path`; an entry with any other
disposition SHALL move nothing.

#### Scenario: Excluded entry moves nothing

- **WHEN** a `defer-exclude` entry for the package `pkg/old` has a destination path that is a directory in the tree,
  a pin file imports `github.com/c360studio/semstreams/pkg/old`, and the tree file imports that directory instead
- **THEN** the file is reported as different

### Requirement: Text outside import paths and comments is not rewritten

Text outside import paths and comments, string literals included, SHALL NOT be rewritten.

#### Scenario: Module path in a string literal

- **WHEN** a pin file holds `github.com/c360studio/semstreams/natsclient` in a string literal and the tree file
  holds `github.com/c360studio/semengine/natsclient` there
- **THEN** the file is reported as different

### Requirement: Both sides are formatted and compared byte for byte

Both sides of the file SHALL then be formatted with gofmt. A `.go` file gofmt cannot parse, and every other covered
file, is compared as it stands. Two files are equal when the results are byte-identical.

#### Scenario: Imports sorted differently after a move

- **WHEN** a moved package's new import path sorts to a different line of the import block in the tree file
- **THEN** the files are equal

### Requirement: An entry that cannot be compared is reported

An entry SHALL be reported as not compared, with the reason, when its destination path does not exist in the tree, when
`source_path` does not exist at `source_sha`, when one side is a file and the other a directory, or when it is a
directory entry that covers no file at the pin and none in the tree.

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
the file, and which of those four it is.

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

### Requirement: A carry failure points to the diff command

The output SHALL say that `task ledger:diff -- <source_path>` prints the lines, and that a package that differs is
`adapt` (`docs/provenance.md` rule 5).

#### Scenario: A differing carried package

- **WHEN** a covered file of a `carry` entry differs from the pin
- **THEN** the output says that `task ledger:diff -- <source_path>` prints the lines and that a package that differs is
  `adapt`

### Requirement: Other dispositions are not compared by the check

Entries with any other disposition SHALL NOT be compared by the check.

#### Scenario: Adapted entry differs

- **WHEN** an `adapt` entry's files differ from the pin and no `carry` entry does
- **THEN** ledger:check passes

### Requirement: No carry entry needs no fetch

With no `carry` entry the program SHALL exit 0 without contacting SemStreams.

#### Scenario: No carried entry

- **WHEN** the ledger has no `carry` entry and SemStreams cannot be reached
- **THEN** ledger:check passes

### Requirement: An unreadable pin exits 2

When SemStreams cannot be read within the fetch bound, which is two minutes for all the fetches of one run together, the
program SHALL exit 2 with a message that says the pin could not be read and that no entry was checked.

#### Scenario: Pin cannot be read

- **WHEN** the ledger has a `carry` entry and SemStreams cannot be reached
- **THEN** the program exits 2 saying the pin could not be read and no entry was checked, and ledger:check fails

### Requirement: A cut-off fetch leaves no process running

When a fetch fails or is cut off, by the fetch bound or by an interrupt, no process the program started for it SHALL
still be running when the program exits.

#### Scenario: Fetch cut off at the bound

- **WHEN** a fetch still waiting on SemStreams is cut off, by the fetch bound or by an interrupt
- **THEN** the program exits 2, and no process it started for the fetch, git's transport helper included, is still
  running when it returns

### Requirement: The carry check writes nothing in the repository

The program SHALL write nothing inside the repository.

#### Scenario: Repository unchanged

- **WHEN** the program runs
- **THEN** no file inside the repository is written

### Requirement: ledger:check fails when the program exits non-zero

`task ledger:check` SHALL fail whenever the program exits non-zero; `task` and `go run` replace the program's exit
status with their own, so through `task` only pass or fail and the message are promised.

#### Scenario: Non-zero exit

- **WHEN** the program exits 1 or 2
- **THEN** `task ledger:check` fails, and through `task` only pass or fail and the message are promised

### Requirement: Pin difference command

`task ledger:diff -- <source_path>...` SHALL run the same program to compare each named entry and, with no argument,
every entry whose disposition is `carry` or `adapt`, in ledger order.

#### Scenario: No argument

- **WHEN** `task ledger:diff` is run with no argument
- **THEN** every `carry` and `adapt` entry is compared, in ledger order

### Requirement: A differing file prints a unified diff

For each covered file that differs it SHALL print on standard output a unified diff from the rewritten pin text,
labelled `pin/` followed by the file's SemStreams path, to the tree file, labelled with its path.

#### Scenario: Changed line is printed

- **WHEN** one line of a covered file differs from the pin
- **THEN** standard output holds a unified diff with the pin's line and the tree's line, and the program exits 0

### Requirement: A file on one side only is named

For a file on one side only it SHALL print `only at the pin: pin/<path>` or `only in the tree: <path>`.

#### Scenario: A file only at the pin

- **WHEN** a covered file exists at the pin and not in the tree
- **THEN** standard output holds `only at the pin: pin/<path>`

### Requirement: An equal file prints nothing

An equal file SHALL print nothing, so standard output is empty when nothing differs.

#### Scenario: Unchanged package prints nothing

- **WHEN** every covered file of the named entry equals the pin
- **THEN** standard output is empty and the program exits 0

### Requirement: Standard error carries one line per entry

On standard error it SHALL print one line per entry: the `source_path`, the disposition, and either the number of files
compared (covered files present on both sides), differing, only at the pin and only in the tree, or `not compared` with
the reason.

#### Scenario: Entry that cannot be compared, no argument given

- **WHEN** no argument is given and an `adapt` entry has a file at the pin and a directory in the tree
- **THEN** standard error says the entry was not compared and why, and the program exits 0

### Requirement: Pin difference command exit status

The program SHALL exit 0 when it ran, whether or not files differ, and 2 when an argument names no entry, when a named
entry could not be compared, or when the ledger or the pin could not be read; through `task` only pass or fail and the
message are promised.

#### Scenario: Unknown source path

- **WHEN** an argument is not the `source_path` of any entry
- **THEN** the program exits 2 naming the argument

### Requirement: The pin difference command writes nothing in the repository

It SHALL write nothing inside the repository.

#### Scenario: Repository unchanged

- **WHEN** `task ledger:diff` runs
- **THEN** no file inside the repository is written

### Requirement: No bare select

No Go file in the module, test or non-test, `package main` included, SHALL contain a `select` statement with no cases. A
goroutine parked on one has no way out, and in a re-executed test binary, which runs with no test-timeout timer, Go's
deadlock detector kills the process once every goroutine blocks. Code parks on `ctx.Done()`, a channel or
`signal.Notify`; a `main` uses `signal.NotifyContext`.

#### Scenario: A bare select is added

- **WHEN** a test file, a non-test file or a `package main` file contains `select {}`, in any spacing
- **THEN** the contract test fails naming the file and line

### Requirement: The bare-select check reads code only

The check SHALL parse each file, so it matches code only: a comment or a string that mentions the pattern does not trip
it, and every spelling of an empty `select` (spacing, newlines, a comment inside) does. A file that does not parse fails
the check.

#### Scenario: A select with cases or a comment

- **WHEN** a file contains a `select` with at least one case, or mentions `select {}` only in a comment or a string
- **THEN** the contract test reports nothing for that file

### Requirement: The bare-select check has no exemption

The check sits beside "No sleeps in tests" and SHALL have, like it, no baseline, no allowlist and no inline exemption.

#### Scenario: No exemption

- **WHEN** a file adds a comment or list entry meant to exempt a bare `select {}`
- **THEN** the contract test still fails naming the file and line

### Requirement: Public signatures name no internal type

A public package is a non-test, non-`main` package of this module whose import path has no `internal` element. No
exported identifier of a public package SHALL name a type declared in a package of this module with an `internal` path
element (Go's rule for what a caller outside the module cannot import), directly or through what a caller outside the
module reaches from it.

#### Scenario: A planted violation fails

- **WHEN** a fixture module's public package declares an exported identifier that names a type declared in a package
  with an `internal` path element directly (a function result, an exported variable or constant, an exported struct
  field), or only through an exported method, an embedded field, an interface method set, a type argument, a generic
  constraint on a function or a type, an alias, or an unexported type that an exported function returns
- **THEN** the contract test fails naming each identifier and the internal type it reaches

#### Scenario: The floor passes

- **WHEN** the contract test runs over this module with the floor's eleven public packages ported
- **THEN** it passes: no exported identifier names a type under `internal/` (the pin's one,
  `natsclient.TemporalResolver.GetStats`, is dropped with `TemporalResolver`; owner ruling, #9 comment 5969522395,
  item 1)

### Requirement: What an exported identifier reaches is followed

What a caller outside the module reaches from an exported identifier includes the exported methods of a type it names,
its exported and embedded struct fields, an interface's method set (embedded interfaces included), type arguments,
generic constraints, and alias targets. A type an exported identifier reaches SHALL be followed whether it is exported
or not.

#### Scenario: An unexported type is followed

- **WHEN** an exported function returns an unexported type whose exported field names a type under `internal/`
- **THEN** the contract test fails naming the identifier and the internal type

### Requirement: Function bodies and unexported identifiers are not checked

Function bodies and unexported identifiers SHALL NOT be checked; a caller outside the module cannot reach them.

#### Scenario: A clean tree passes

- **WHEN** a fixture module's public package uses a type declared under its `internal/` only in unexported
  identifiers and function bodies
- **THEN** the contract test reports nothing for that package

### Requirement: No second spelling of deployment authority

A deployment's authority (`org`, `platform`) SHALL be established once, by `config.Manager.Start` under ADR-104, carried
to components once as `deps.Platform`, and SHALL become positions 1–2 of an identity only through
`FrameworkIdentityFamily.EntityID` (#72 ruling A as extended, comment 5969505488). No other package SHALL compute,
parse, re-join or carry it, and no message, envelope or metadata SHALL hold a copy.

#### Scenario: A test file is not checked

- **WHEN** a `_test.go` file declares an exported struct field `Org` or an exported function `EntityIRI`
- **THEN** neither contract test reports it

### Requirement: Two contract tests enforce the static part of deployment authority

Two contract tests in `internal/harness/contract`, both run by `task test:unit`, SHALL enforce the parts of "No second
spelling of deployment authority" a static check can see; each has a sensitivity test that plants the violation in a
temporary module and requires the check to name it.

#### Scenario: This module passes

- **WHEN** both contract tests run over this module
- **THEN** they pass, each reporting the number of packages it checked

### Requirement: No exported name carries Federation, GlobalID or EntityIRI

No exported name in a non-test package of this module, internal and `main` packages included, SHALL contain
`Federation`, `GlobalID` or `EntityIRI` (case-sensitive substrings). This covers package-level identifiers, and the
exported methods, struct fields and interface methods of package-level types, whether the type is exported or not. A
failure names the file, line, qualified identifier and the requirement "No second spelling of deployment authority".

#### Scenario: A deployment-authority name is exported

- **WHEN** a fixture module declares an exported name containing `Federation`, `GlobalID` or `EntityIRI` in a
  public, an internal or a `main` package: a type, function, variable or constant, a method (of an unexported type
  too), a struct field or an interface method, or a type alias, or a member of the struct or interface an alias
  stands for
- **THEN** the contract test fails naming each one once, with its file, line and qualified identifier and the rule

### Requirement: A type alias is checked by name and by members

A type alias is a package-level type: its own name SHALL be checked, and so SHALL the members of the type it stands for;
members of a type this module declares SHALL be reported once, at that declaration.

#### Scenario: Members of an alias target

- **WHEN** a type alias stands for a struct with an exported field whose name contains `GlobalID`
- **THEN** the contract test reports the field once, at the declaration of the struct

### Requirement: Unexported names and test files are not checked for retired words

Unexported names, test files, comments and string literals SHALL NOT be checked by the name check.

#### Scenario: The words appear where the rule does not apply

- **WHEN** the same words appear only in unexported names, in an exported name that holds one of them in lower case
  only (`Confederation`), in a name such as `Federated` that does not contain one of them, in a comment or a string
  literal, or in a test file
- **THEN** the contract test reports nothing for them

### Requirement: No exported Org or Platform field outside the listed exceptions

No exported struct field named `Org` or `Platform` SHALL be declared in a non-test file of a package of this module, in
a named or an anonymous struct, except the fields listed in "Owner packages may declare Org and Platform" and "The
caller's claim and the carrier may declare Org and Platform". The failure SHALL name the file, the line, the struct
type, the field and the requirement "No second spelling of deployment authority".

#### Scenario: An authority field outside the owners fails

- **WHEN** a non-test file of a fixture module declares, outside the excepted packages, an exported struct field
  `Org` or `Platform` of type `string` on a named struct (the shape of `graph/inference.HierarchyConfig` and
  `graph/llm.EntityParts` at the pin), on an anonymous struct, or on a struct in an internal or a `main` package;
  or a field `Platform` of type `types.PlatformMeta` on a struct other than the three the exception names
- **THEN** the contract test fails naming each field with its file, line and struct type

### Requirement: Owner packages may declare Org and Platform

Any field in the packages `pkg/types`, `pkg/platform` and `config` (the source and its validators) SHALL be excepted,
and so SHALL the two fields of the type `PlatformMeta` in the top-level package `types` (import path `<module>/types` —
not `pkg/types`, and not Go's `go/types`).

#### Scenario: The owners, the carrier and the caller's claim pass

- **WHEN** a fixture module declares `Org` and `Platform` fields in its `pkg/types`, `pkg/platform` and `config`
  packages; a type `PlatformMeta` in its top-level `types` package with fields `Org` and `Platform`; the carrier
  field `Platform` of that type on `component.Dependencies`, `service.Dependencies` and
  `processor/rule.Dependencies`; a field `Org` on `processor/rule.CallerContext`; and unexported fields `org` and
  `platform` of type `string` anywhere
- **THEN** the contract test reports nothing for any of them

### Requirement: The caller's claim and the carrier may declare Org and Platform

`processor/rule.CallerContext.Org` (a caller's organization claim, which is not the deployment authority) SHALL be
excepted, and so SHALL the carrier `deps.Platform`, by exact field: `component.Dependencies.Platform`,
`service.Dependencies.Platform` and `processor/rule.Dependencies.Platform`.

#### Scenario: The carrier fields

- **WHEN** a fixture module declares `Platform` of type `types.PlatformMeta` on `component.Dependencies`,
  `service.Dependencies` and `processor/rule.Dependencies`, and `Org` on `processor/rule.CallerContext`
- **THEN** the contract test reports nothing for them

### Requirement: A field's type is not an exception

A field's type is not an exception: a further field of type `types.PlatformMeta` named `Org` or `Platform` SHALL fail
until the exception list names it.

#### Scenario: A further PlatformMeta field

- **WHEN** a struct not named in the exceptions declares a field `Platform` of type `types.PlatformMeta`
- **THEN** the contract test fails naming the file, line, struct type and field

### Requirement: Authority field exceptions are listed once

The exceptions SHALL be listed by exact package path, exact type name and exact field name in "Owner packages may
declare Org and Platform" and "The caller's claim and the carrier may declare Org and Platform" and nowhere else; a new
exception is a change to those requirements.

#### Scenario: A new exception

- **WHEN** a field outside the listed exceptions is to be allowed
- **THEN** the change edits the exception requirements, since no other list exists

### Requirement: The authority field check is static and by name

The check SHALL be static and by name: a field named otherwise (`OrgID`, `PlatformID`, `org`, `platform`), a copy held
in an untyped holder, and a value re-joined inside a function body are outside its scope by construction, and remain
review only.

#### Scenario: Names the check does not see

- **WHEN** a struct declares `OrgID` or an unexported `org` field
- **THEN** the contract test reports nothing for it

### Requirement: Agent setup and rule-index parity

`task test:unit` SHALL run a contract test (`TestAgentSetupParity` and `TestAgentSetupParitySensitivity`) that checks
the tracked agent contracts, platform adapters, skill adapters, and rule index against `.agents/README.md`, in the eight
steps stated in the requirements that follow.

#### Scenario: Repository agent setup passes

- **WHEN** `TestAgentSetupParity` runs on the repository root
- **THEN** it reports no violation

#### Scenario: Planted violation in any of Steps 1 to 8 fails

- **WHEN** a fixture tree omits a canonical contract or adapter, points an adapter at the wrong contract, grants `Edit`
  or `LSP` to a read-only Claude adapter, omits `sandbox_mode = "read-only"` on a read-only Codex adapter or sets
  `sandbox_mode` on a write Codex adapter, adds a rule table to `CLAUDE.md`, grows `AGENTS.md` to 24,000 bytes or more,
  adds a rule row to `AGENTS.md` without `.agents/rules-index.md`, adds a checkbox or exceeds 20 lines in a role
  adapter, breaks a Claude skill adapter link, or changes a pinned `model` or `model_reasoning_effort`
- **THEN** the check fails naming the file and the violated rule

### Requirement: Agent setup step 1: required files exist

Step 1. All four canonical contracts (`.agents/contracts/semengine-{architect,developer,reviewer,technical-writer}.md`),
all eight role adapters (`.claude/agents/semengine-*.md`, `.codex/agents/semengine-*.toml`), `.agents/protocol.md`,
`.agents/rules-index.md`, `.agents/README.md`, `AGENTS.md`, and `CLAUDE.md` SHALL exist.

#### Scenario: Missing file

- **WHEN** a fixture tree omits a canonical contract or an adapter
- **THEN** the check fails naming the file and the violated rule

### Requirement: Agent setup step 2: each role adapter points at its contract

Step 2. Each role adapter SHALL declare `name` equal to its role, reference only its matching
`.agents/contracts/<role>.md` path, and instruct the agent to read that contract fully first.

#### Scenario: Adapter points at the wrong contract

- **WHEN** a role adapter references another role's contract
- **THEN** the check fails naming the file and the violated rule

### Requirement: Agent setup step 3: Claude adapter tools

Step 3. Read-only Claude adapters (`semengine-architect`, `semengine-reviewer`) SHALL list `tools: Read, Bash, Grep,
Glob, Skill` and SHALL NOT list `Edit`, `Write`, `Task`, or `LSP`; write Claude adapters (`semengine-developer`,
`semengine-technical-writer`) SHALL list `tools: Read, Edit, Write, Bash, Grep, Glob, Skill` and SHALL NOT list `Task`
or `LSP`.

#### Scenario: Read-only adapter with Edit

- **WHEN** a fixture grants `Edit` or `LSP` to a read-only Claude adapter
- **THEN** the check fails naming the file and the violated rule

### Requirement: Agent setup step 4: Codex adapter sandbox

Step 4. Read-only Codex adapters (`semengine-architect`, `semengine-reviewer`) SHALL set `sandbox_mode = "read-only"`;
write Codex adapters (`semengine-developer`, `semengine-technical-writer`) SHALL NOT set `sandbox_mode`.

#### Scenario: Sandbox mode wrong

- **WHEN** a fixture omits `sandbox_mode = "read-only"` on a read-only Codex adapter or sets `sandbox_mode` on a write
  Codex adapter
- **THEN** the check fails naming the file and the violated rule

### Requirement: Agent setup step 5: CLAUDE.md, AGENTS.md and the rule index

Step 5. `CLAUDE.md` SHALL contain `@AGENTS.md`, have at most 10 lines, and contain no section heading (`##`) or table;
`AGENTS.md` SHALL be strictly smaller than 24,000 bytes; and `AGENTS.md` and `.agents/rules-index.md` SHALL have the
same numbered rule-domain headings and the same number of rule rows in each domain table.

#### Scenario: AGENTS.md and rule index drift

- **WHEN** a fixture adds a rule table to `CLAUDE.md`, grows `AGENTS.md` to 24,000 bytes or more, or adds a rule row to
  `AGENTS.md` without `.agents/rules-index.md`
- **THEN** the check fails naming the file and the violated rule

### Requirement: Agent setup step 6: role adapter size

Step 6. Every role adapter SHALL have at most 20 lines and SHALL NOT contain a markdown task-list checkbox (`- [ ]` or
`- [x]`).

#### Scenario: Adapter too long or with a checkbox

- **WHEN** a fixture adds a checkbox or exceeds 20 lines in a role adapter
- **THEN** the check fails naming the file and the violated rule

### Requirement: Agent setup step 7: skill adapters

Step 7. Every canonical skill under `.agents/skills/<skill>/SKILL.md` SHALL have its matching Claude adapter under
`.claude/skills/<cmd>/SKILL.md` (`preflight` for `semengine-preflight`, `<skill>` otherwise) that links to
`../../../.agents/skills/<skill>/SKILL.md`, says to read it `fully`, has at most 25 lines, and contains no task-list
checkbox.

#### Scenario: Broken skill adapter link

- **WHEN** a fixture breaks a Claude skill adapter link
- **THEN** the check fails naming the file and the violated rule

### Requirement: Agent setup step 8: Claude model pins

Step 8, Claude. Claude adapters SHALL pin `model: opus` for `semengine-architect`, `semengine-developer`, and
`semengine-reviewer`, and `model: sonnet` for `semengine-technical-writer`.

#### Scenario: Claude model changed

- **WHEN** a fixture changes a pinned Claude `model`
- **THEN** the check fails naming the file and the violated rule

### Requirement: Agent setup step 8: Codex model pins

Step 8, Codex. Codex adapters SHALL pin `model = "gpt-6-astra"` and `model_reasoning_effort = "medium"` for
`semengine-architect`, `model = "gpt-6-sol"` and `model_reasoning_effort = "medium"` for `semengine-developer` and
`semengine-technical-writer`, and `model = "gpt-6-astra"` and `model_reasoning_effort = "high"` for
`semengine-reviewer`.

#### Scenario: Codex model or effort changed

- **WHEN** a fixture changes a pinned Codex `model` or `model_reasoning_effort`
- **THEN** the check fails naming the file and the violated rule
