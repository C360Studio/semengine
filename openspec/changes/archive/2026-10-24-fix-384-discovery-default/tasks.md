# Tasks: fix-384-discovery-default

Each task names its outcome and the gate that proves it. Tests come first. Record in `notes/<task>.md` how each test
fails on the base commit. Each code task ends only when `task verify` passes on a clean tree at its commit (#171).

## 1. Design

- [x] 1.1 Inventory (`inventory.md`) passes independent inventory review (`INVENTORY PASS`), recorded on the PR.
  Gate: the reviewer's verdict. Passed with two LOW findings, both absorbed into § 1 and § 3 (PR #394 comment
  6383590713).
- [x] 1.2 Claude design review. A Claude `semengine-reviewer` passes `design.md`, this file and the spec delta; its
  verdict is linked from a PR comment. Passed at 3a276d6 with one MEDIUM (the example's trailing comma) and one LOW
  (an `-expect` line per mutant), both applied.
- [x] 1.3 Codex design review and owner design acceptance on #384 or the PR. Gate: the comments. Codex passed at
  89b17a7 with no findings, and the owner accepted the design on 2026-10-24, both on PR #394.

## 2. Tests and code

- [x] 2.1 Tests first (design "Proving tests"):
  - add `TestLoad_DefaultServicesAreHeartbeatOnly` to `config/load_test.go`;
  - delete `config/config_test.go:114` and `:116` in `TestLoader_Defaults`;
  - replace the discovery lines `:250` and `:252` in `TestLoader_ExampleConfig` with the literal service-key
    assertion.

  Gate: both new assertions fail on the base, recorded in `notes/2.1.md` with the failing output. Done at aae6c29.
- [x] 2.2 The fix:
  - drop the `discovery` entry from `getDefaults` (`config/config.go:491-494`);
  - delete `config/example_config.json:13` and the trailing comma on `:12`;
  - correct the comment at `types/service.go:47-50` (design D3).

  Gate: `task verify` green, plus mutants M1-M3 each reporting `verdict: detection` under `task mutate:check`, with
  the commands and output recorded in `notes/2.2.md`. Done at 90ae0d2: `task verify` exit 0,
  M1-M3 all detected.

## 3. Code review

- [x] 3.1 Claude code review. A Claude `semengine-reviewer` passes the code at a named commit with a local
  `task verify`. Passed at 3d57cfc0ddb4c7bb1906b4e1ab3a812bfa3ee54e with no findings, local `task verify` exit 0;
  recorded on PR #394.
- [x] 3.2 Codex code review of record at a named commit with a local `task verify`; the PR body carries
  `implemented-by: claude (opus)` and `reviewed-by: codex`. Codex approved at
  b600f9e80a0f3c28d7c28b42bc7e17cbe2782a34 with no findings (PR #394 comment 6384232401).

## 4. Docs and archive

- [x] 4.1 `config/README.md:48` says the defaults name one service, `heartbeat` (enabled, every 30 seconds). The
  `config` row of `docs/admission-ledger.yaml` records the change in its "Changed from the pin" list in
  `known_risks` (`:2729-2747`): the pin's dormant `discovery` default, for a service that exists neither at the pin
  nor here, is dropped (#384). Gate: `task docs:check` and `task ledger:check`. Done at a5b37df.
- [x] 4.2 The `config-loading` delta synced into `openspec/specs/config-loading/spec.md`; `docs/repository-map.md`
  names the archive (merging `origin/main` first if #395 has landed); archived as the last content commit. Gate:
  `task spec:check` and `task docs:check`. #395 landed first and made the map describe the archive folder generically
  (`docs/repository-map.md:63`), naming no archived change, so the map needs no row for this one.
