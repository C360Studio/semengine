# Design: repo-local-ateam

## Observable behaviour

Every hook, MCP server registration, and local Task gate (`doctor`, `ateam:doctor`, `spec:check`, `spec:queue`,
`merge:check`) invokes `scripts/ateam`, which resolves `<main-checkout>/.tools/bin/ateam` first, then
`<worktree>/.tools/bin/ateam`, then `ateam` on `PATH`, and exits 127 naming `task tools:ateam` when none exists.
`task tools:ateam` installs the pinned release into the main checkout's `.tools/bin/ateam` (found via
`git rev-parse --path-format=absolute --git-common-dir`), so every linked worktree shares one install without rebuilding.
`.agents/ateam.json` records `"ateam_version": "v0.4.0"`, so `ateam doctor --check` verifies both the running binary
and the configured `hook_binary` against the pin.

## Decisions

- **Main-checkout `.tools/bin/ateam` over per-worktree `.bin/ateam`.** Linked worktrees under `.worktrees/<branch>`
  have a `.git` file whose first line is `gitdir: <main>/.git/worktrees/<name>`. `scripts/ateam` strips
  `/.git/worktrees/*` using POSIX parameter expansion without forking `git`, staying well within the 10 ms hook budget
  while sharing a single binary across all worktrees.
- **Task dependencies stay on `tools:ateam`.** `doctor`, `ateam:doctor`, `spec:check`, `spec:queue`, and `merge:check`
  keep `deps: [tools:ateam]` so a fresh checkout builds `.tools/bin/ateam` automatically before running any gate, and
  `TestSpecTaskWiring` allows only `deps: [tools:ateam]`.
- **Client and plugin hook/MCP files are adopter-configured.** `ateam init` writes bare `ateam` into `.agents/hooks.json`
  and `.agents/plugins/ateam/mcp_config.json` as well as the five client configs; adding those two to `semengineOwned`
  in `ateamtemplates_test.go` lets them invoke `scripts/ateam` while `ateam doctor --check` validates their hook and
  MCP entries.
- **Single version across workflows, Taskfile, and `.agents/ateam.json`.** `scripts/doctor.sh` and `TestAteamPinParity`
  in `ateamtemplates_test.go` verify that `.github/workflows/{ci,merge-check}.yml`, `Taskfile.yml` (`ATEAM_VERSION`),
  and `.agents/ateam.json` (`ateam_version`) all name the same release.
- **No generated checks needed.** The shim is a 20-line POSIX shell script with four deterministic resolution branches
  (main `.tools/bin`, worktree `.tools/bin`, `PATH`, missing -> exit 127), covered by example and mutation-sensitivity
  subtests in `TestAteamShimResolution`.

## Proving tests

- `TestAteamShimResolution`: exercises primary and worktree resolution without `git` on `PATH`, fallback order, exit 127
  when missing, and two sensitivity mutations (swapped search order and broken primary-checkout `else` branch).
- `TestUnitInvocationsPinned` and `TestSpecTaskWiring`: hold `ateam:doctor`, `spec:check`, and `spec:queue` to
  `scripts/ateam` and `deps: [tools:ateam]`.
- `TestAteamTemplatesUnchanged` and `TestAteamPinParity`: verify `scripts/ateam` runs at the pin and that workflows,
  `Taskfile.yml`, and `.agents/ateam.json` agree.
