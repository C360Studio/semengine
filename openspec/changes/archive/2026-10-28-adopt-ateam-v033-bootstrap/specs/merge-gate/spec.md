# merge-gate

## MODIFIED Requirements

### Requirement: Tool tests run when their files change

`task test:tools:changed` SHALL run `task test:tools` when a changed file is under `scripts/`, `internal/harness/`,
`.github/`, `.agents/`, `.claude/`, `.codex/` or `.gemini/`, or is `Taskfile.yml`, `go.mod`, `go.sum`,
`docs/admission-ledger.yaml`, `service/base.go`, `service/service_manager.go`, `service/component_manager.go` or
`.mcp.json`. Otherwise it SHALL exit zero after printing that the tool tests were skipped and that no changed file
matched.

#### Scenario: A script changed

- **WHEN** the changed files include `scripts/merge-check.sh`
- **THEN** `task test:tools` runs

#### Scenario: An ateam template changed

- **WHEN** the changed files include `.agents/contracts/reviewer.md`
- **THEN** `task test:tools` runs, so `TestAteamTemplatesUnchanged` checks it

#### Scenario: Only product code and docs changed

- **WHEN** the changed files are `graph/query_verbs.go` and `docs/testing.md`
- **THEN** `task test:tools` does not run and the output says the tool tests were skipped

#### Scenario: An untracked file under a trigger path

- **WHEN** the only change is an untracked file under `internal/harness/`
- **THEN** `task test:tools` runs
