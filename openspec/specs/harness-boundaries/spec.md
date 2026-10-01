# harness-boundaries Specification

## Purpose
Harness boundaries are the machine-checked rules that keep test machinery out of production code, keep SemEngine's
Docker footprint invisible to SemStreams' substring cleanup, and keep every reused SemStreams file traceable in the
admission ledger.
## Requirements
### Requirement: Import graph

No non-test Go file outside `internal/harness/` SHALL import `internal/harness/...`, `testcontainers-go`, `testing`,
or `gopkg.in/yaml.v3`.

#### Scenario: Production package imports the fixture

- **WHEN** a non-test file outside internal/harness imports internal/harness/natsfixture
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

