# harness-boundaries

## MODIFIED Requirements

### Requirement: One image pin

The NATS image SHALL be spelled only in `internal/harness/natsfixture/nats-image` as `nats:<tag>@sha256:<digest>`;
natsfixture SHALL embed that file, SHALL refuse a reference without `@sha256:<digest>` before any Docker call, and
`scripts/doctor.sh` SHALL read that file. The check covers every tracked file that configures or runs Docker
(`*.go`, `*.sh`, `Taskfile.yml`, `*.yaml`/`*.yml`, Dockerfiles, CI workflows), including variable tags such as
`nats:${TAG}`; Markdown and other prose may cite the tag. In a `*.go` file the check reports `nats:` only when a digit
or a variable (`$`, `${`) follows it, and reports every `nats@sha256:`; a Go literal `nats:` followed by a letter,
such as a component port identifier `nats:<subject>` or a tag such as `nats:latest`, is not reported, and an image tag
that starts with a letter in Go is review only.

#### Scenario: Literal elsewhere

- **WHEN** a tracked file that configures or runs Docker, other than `internal/harness/natsfixture/nats-image`,
  contains a `nats:` image literal
- **THEN** the contract test fails

#### Scenario: A port identifier in a Go file

- **WHEN** a Go file contains the literal `"nats:in"` or `"nats:sensor.data"`
- **THEN** the contract test reports nothing for it

#### Scenario: An image in a Go file

- **WHEN** a Go file contains the literal `"nats:2.10"`, `"nats:${TAG}"`, or a `nats@sha256:` digest in a constant in a
  file with no imports
- **THEN** the contract test fails naming the file and line

#### Scenario: Digest pin ignores tag movement

- **WHEN** another repository re-pulls the mutable `nats:2.15.0-alpine` tag
- **THEN** natsfixture's containers still run the image the pin's digest names, because the pin is a digest reference
  and natsfixture refuses one without a digest

### Requirement: Bounded cleanup roots

`task verify` SHALL fail when any `*_test.go` file calls Stop, Close, or Terminate with context.Background() or
context.TODO(). The harness's own cleanup roots (`natsfixture.New`'s cleanup Stop and the removal of a failed start's
container, each under a fresh bounded context) live in non-test files, and a package's `TestMain` stops its shared
server under `context.WithTimeout`, so the check has no allowlist.

#### Scenario: Unbounded defer

- **WHEN** a test adds `defer o.Stop(context.Background())`
- **THEN** cleanup-roots-check fails naming the line

#### Scenario: Unbounded shared-server stop

- **WHEN** a `TestMain` calls `s.Stop(context.Background())`
- **THEN** cleanup-roots-check fails naming the line

## REMOVED Requirements

### Requirement: SemEngine-assigned names

**Reason**: Its substring list guarded against SemStreams' name-substring cleanup, which destroys volumes (setup-02
premise P11); natsfixture creates no volume and now assigns no container name, and SemEngine does not coordinate with
SemStreams (owner ruling, #205 comment 6132580576). Its scenarios on destroyer substrings go with the list. The prefix
rule moves to "Docker names this repository assigns".

**Migration**: None.

## ADDED Requirements

### Requirement: Docker names this repository assigns

Every Docker name a script, task or Compose file in this repository assigns (container `--name`/`container_name`,
volume, network, Compose project via `-p`/`--project-name`/`COMPOSE_PROJECT_NAME`/top-level `name:`) SHALL start with
`semengine-` and use only `[a-z0-9-]`. Compose invocations SHALL always pass an explicit project name. natsfixture
assigns no container name; its containers are found by their label (`nats-fixture`, "Ryuk off, labelled containers
and the sweep").

#### Scenario: Name without the prefix

- **WHEN** a script names a volume `cache`
- **THEN** the contract test fails naming the line

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
