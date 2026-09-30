# harness-boundaries

## Purpose

Harness boundaries are the machine-checked rules that keep test machinery out of production code, keep SemEngine's
Docker footprint invisible to SemStreams' substring cleanup, and keep every reused SemStreams file traceable in the
admission ledger.

## ADDED Requirements

### Requirement: Import graph

No non-test Go file outside `internal/harness/` SHALL import `internal/harness/...`, `testcontainers-go`, `testing`,
or `gopkg.in/yaml.v3`.

#### Scenario: Production package imports the fixture

- **WHEN** a non-test file outside internal/harness imports internal/harness/natsfixture
- **THEN** the contract test fails naming the file

### Requirement: No retained context

No struct type in a non-test file SHALL hold a context.Context directly, embedded, aliased, in a container, or as a
provider result; context.CancelFunc fields are permitted.

#### Scenario: Fixture struct is checked

- **WHEN** natsfixture.Fixture declares a context.Context field
- **THEN** the contract test fails

### Requirement: One image pin

The NATS image SHALL be spelled only in `.nats-image` as `nats:<tag>@sha256:<digest>`; scripts and Go SHALL receive it
through SEMENGINE_NATS_IMAGE.

#### Scenario: Literal elsewhere

- **WHEN** any tracked file other than .nats-image contains a `nats:` image literal
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
`:8222"`) or bind a fixed TCP port (scripts/lint-test-ports.sh SHALL pass).

#### Scenario: Fixed dial

- **WHEN** a test dials nats://localhost:4222
- **THEN** the contract test fails naming the line

### Requirement: Bounded cleanup roots

`task verify` SHALL fail when a `*_test.go` file outside the two sanctioned harness cleanup roots calls Stop, Close,
or Terminate with context.Background() or context.TODO().

#### Scenario: Unbounded defer

- **WHEN** a test adds `defer o.Stop(context.Background())`
- **THEN** cleanup-roots-check fails naming the line

### Requirement: No broad Docker cleanup

No script or task SHALL invoke `docker … prune`, `docker volume ls … --filter name=`, `docker ps … --filter name=`,
`xargs … docker volume rm`, or Compose `down` for a project it did not create.

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
