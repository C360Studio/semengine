# merge-gate Delta

## MODIFIED Requirements

### Requirement: Tool tests and repository guards are separate packages

The tool packages SHALL be `internal/harness/toolcheck`, `internal/harness/mutcheck`, `internal/harness/pindiff` and
`internal/harness/scorecard`, named once, in `scripts/test-pkgs.sh`. A test SHALL live in `toolcheck` when every file
of the real tree it reads is under a trigger path, and in `internal/harness/contract`, a unit package, when it reads
any other file. `task test:tools` SHALL run the tool packages with `go test -race -count=1 -cpu 1`.

#### Scenario: A tool package among the unit packages

- **WHEN** `scripts/test-pkgs.sh unit` lists `internal/harness/mutcheck`
- **THEN** `TestTestPkgsLists` fails naming the package

#### Scenario: The repository guards dropped from the unit packages

- **WHEN** `scripts/test-pkgs.sh unit` does not list `internal/harness/contract`
- **THEN** `TestTestPkgsLists` fails naming the package

#### Scenario: A real-tree test in toolcheck

- **WHEN** a test that runs `scripts/lint-test-ports.sh` over the real tree's test files is placed in `toolcheck`
- **THEN** review refuses it; no command fails, because which files a test reads is review only
