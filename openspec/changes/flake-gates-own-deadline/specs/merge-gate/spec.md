## MODIFIED Requirements

### Requirement: Varied and repeated unit runs

`task test:unit` SHALL run `go test -race -count=1 -cpu 1 -timeout 5m` over the unit packages: every package
`go list ./...` names but the tool packages. `task test:repeat` SHALL run `go test -count=5 -cpu 1 -shuffle=on
-timeout 5m` over the packages after `--`, else the repeat packages: the unit packages but
`internal/harness/contract`. The five-minute timeout is the one deadline a unit package's tests have: four times the
slowest package's measured time (`internal/harness/contract`, 72 s on CI run 41442032443) and inside CI's 15-minute
job, so a hang still prints every goroutine's stack there; no test sets a shorter deadline of its own
(`harness-boundaries`, "No own deadlines in tests"). A pinning test SHALL fail when a command line or the
`scripts/verify.sh` step list differs from this or from "Verify runs both unit steps, the repeat step last".

#### Scenario: Count lowered

- **WHEN** the `test:repeat` command in `Taskfile.yml` carries `-count=1`
- **THEN** the pinning test fails naming the task and the required command

#### Scenario: CPU setting dropped

- **WHEN** the `test:unit` command in `Taskfile.yml` has no `-cpu 1`
- **THEN** the pinning test fails naming the task and the required command

#### Scenario: Race detector added to the repeat step

- **WHEN** the `test:repeat` command in `Taskfile.yml` carries `-race`
- **THEN** the pinning test fails naming the task and the required command

#### Scenario: Repeat over every package

- **WHEN** the `test:repeat` command in `Taskfile.yml` defaults to `./...` instead of the repeat packages
- **THEN** the pinning test fails naming the task and the required command

#### Scenario: Timeout dropped or widened

- **WHEN** the `test:unit` or `test:repeat` command in `Taskfile.yml` has no `-timeout 5m`
- **THEN** the pinning test fails naming the task and the required command
