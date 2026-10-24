# Tasks: fix-384-discovery-default

Each task names its outcome and the gate that proves it. Tests come first. Record in `notes/<task>.md` how each test
fails on the base commit. Each code task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory (`inventory.md`) passes independent inventory review (`INVENTORY PASS`), recorded on the PR.
  Gate: the reviewer's verdict. Passed with two LOW findings, both absorbed into § 1 and § 3 (PR #394 comment
  6383590713).
- [ ] 1.2 Claude design review. A Claude `semengine-reviewer` passes `design.md`, this file and the spec delta; its
  verdict is linked from a PR comment.
- [ ] 1.3 Codex design review and owner design acceptance on #384 or the PR. Gate: the comments.

## 2. Tests and code

- [ ] 2.1 Tests first (design "Proving tests"):
  - add `TestLoad_DefaultServicesAreHeartbeatOnly` to `config/load_test.go`;
  - delete `config/config_test.go:114` and `:116` in `TestLoader_Defaults`;
  - replace the discovery lines `:250` and `:252` in `TestLoader_ExampleConfig` with the literal service-key
    assertion.

  Gate: both new assertions fail on the base, recorded in `notes/2.1.md` with the failing output.
- [ ] 2.2 The fix:
  - drop the `discovery` entry from `getDefaults` (`config/config.go:491-494`);
  - delete `config/example_config.json:13`;
  - correct the comment at `types/service.go:47-50` (design D3).

  Gate: `task verify` green, plus mutants M1-M3 each reporting `verdict: detection` under `task mutate:check`, with
  the commands and output recorded in `notes/2.2.md`.

## 3. Code review

- [ ] 3.1 Claude code review. A Claude `semengine-reviewer` passes the code at a named commit with a local
  `task verify`.
- [ ] 3.2 Codex code review of record at a named commit with a local `task verify`; the PR body carries
  `implemented-by: claude (opus)` and `reviewed-by: codex`.

## 4. Docs and archive

- [ ] 4.1 `config/README.md:48` says the defaults name one service, `heartbeat` (enabled, every 30 seconds). The
  `config` row of `docs/admission-ledger.yaml` records the change in its "Changed from the pin" list in
  `known_risks` (`:2729-2747`): the pin's dormant `discovery` default, for a service that exists neither at the pin
  nor here, is dropped (#384). Gate: `task docs:check` and `task ledger:check`.
- [ ] 4.2 The `config-loading` delta synced into `openspec/specs/config-loading/spec.md`; `docs/repository-map.md`
  names the archive (merging `origin/main` first if #395 has landed); archived as the last content commit. Gate:
  `task spec:check` and `task docs:check`.
