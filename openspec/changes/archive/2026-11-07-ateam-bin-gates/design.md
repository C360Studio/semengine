# Design: ateam-bin-gates

## Observable behaviour

Every local gate that runs ateam runs `.bin/ateam`, the release pinned in `.github/workflows/ci.yml`, whatever
`ateam` is first on `PATH`. A task that needs it depends on `ateam:bin`, which builds it when it is missing or does
not report the pin and does nothing otherwise. The CI install step into `$(go env GOPATH)/bin` stays: the workflows
remain the one place the pin is written, and `scripts/doctor.sh` and `ateamPins` read it there.

## Decisions

- **A task dependency, not a wrapper script.** `deps: [ateam:bin]` is Task's own mechanism; a failed build fails the
  task before its command runs. The wiring test allows exactly that key and value, so a dependency cannot be used to
  slip in a command that skips or discards one.
- **The tool test reads `.bin/ateam` and does not build it.** A test that installs a binary would fetch modules from
  inside `go test`. It fails naming `task ateam:bin` instead; `task verify` has built it by then (`ateam:doctor` runs
  first).
- **`.bin/` per worktree.** Each claim worktree builds once from the warm module cache. A shared location would bring
  back the question of which copy a gate ran.

## Proving tests

- `TestUnitInvocationsPinned`: `ateam:doctor` runs `.bin/ateam doctor --check` with the four session variables unset.
- `TestSpecTaskWiring`: the new command lines; a fixture with `deps: [ateam:bin, cleanup]` fails naming `deps`.
- `TestAteamTemplatesUnchanged`: with `.bin/` moved away it fails naming `task ateam:bin`.
