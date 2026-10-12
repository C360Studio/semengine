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
