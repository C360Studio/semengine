# Proposal: ateam-bin-gates

Track: design (two capabilities, `harness-boundaries` and `spec-queue`; four requirements modified, none added).

## Why

PR #545 added `task ateam:bin`, which builds the ateam release `ci.yml` pins into `.bin/ateam`, and moved
`task doctor` and `task merge:check` onto it. `task ateam:doctor` (the first step of `task verify`), `task spec:check`,
`task spec:queue` and `TestAteamTemplatesUnchanged` still run the `ateam` first on `PATH`. On a machine whose `PATH`
holds a development build or an older release, `task verify` fails or checks against the wrong version unless the
developer reorders `PATH`, which is why agents kept installing ad-hoc copies under `/tmp`.

Approved-by: Coby Leuschke (2026-11-07), in the Enjoy conversation "ateam install location" ("Write the spec change
so ateam:doctor, spec:check and spec:queue also use .bin/ateam").

## What Changes

- `task ateam:doctor`, `task spec:check` and `task spec:queue` depend on `ateam:bin` and run `.bin/ateam`.
- `TestAteamTemplatesUnchanged` runs `.bin/ateam`, failing and naming `task ateam:bin` when it is missing.
- `TestSpecTaskWiring` accepts `deps: [ateam:bin]` on the two spec tasks and nothing else besides `desc`, `summary`
  and `cmds`.
- `docs/testing.md` drops the `PATH` workaround.

## Capabilities

- Modified `harness-boundaries`: "ateam doctor gates verify", "ateam template files are the pin's".
- Modified `spec-queue`: "The spec queue is ateam state spec-queue at the pin", "spec:check runs validate then ateam
  state spec-check".

## Impact

- `Taskfile.yml`, `internal/harness/toolcheck/{mergegate,specwiring,ateamtemplates}_test.go`, `docs/testing.md`,
  the two specs. CI already runs `task doctor`, which builds `.bin/ateam`, before `task verify`.
- No issue; follows PR #545.
