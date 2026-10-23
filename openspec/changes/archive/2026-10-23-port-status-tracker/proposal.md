# Proposal: port-status-tracker

## Why

The Tier-0 extraction (Slice 04A, epic #9) spans 65 SemStreams packages across seven dependency-closed changes (`D2` of
`openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md`), and Change 3 (`setup-04a-03-boot`) splits into
per-package and per-lifecycle-owner pull requests (#159). Without a single command or skill, checking how many packages
have landed in `docs/admission-ledger.yaml`, which `service` owner PRs remain, which draft PRs claim active work,
whether a `class:flake` issue blocks merges, or whether open issues have drifted without milestones or labels requires
manual cross-referencing across `docs/admission-ledger.yaml`, `openspec/`, and GitHub. In addition, the four GitHub
milestones carried spaces, colons, parentheses, and em-dashes that broke unquoted search filters, 33 open issues had no
milestone, 12 had no labels, and `docs/repository-map.md` still stated that `setup-04a-03` was not started.

## What Changes

- **Command (`task status` / `scripts/port-status.sh`).** Add a read-only status and audit script (`scripts/port-status.sh`,
  wired as `task status` in `Taskfile.yml` and listed in `AGENTS.md`) that:
  1. Reads `docs/admission-ledger.yaml` and prints the Slice 04A 65-package scoreboard across Changes 1–7 (`ported`,
     `partial`, `deferred/excluded`, and `unstarted`), including the `service` 7-PR lifecycle-owner breakdown.
  2. Queries GitHub (`gh`, when `--offline` is not set) for milestone counts (`setup`, `slice-04a`, `slice-04b`,
     `slice-04c`), open PR claims, open `class:flake` merge blockers, and issue hygiene drift (any open issue missing a
     milestone or label).
  3. Supports `--check` (exits 1 when any open issue lacks a milestone or label, or when a milestone title contains
     whitespace) and `--sync-project [<project-number>]` (adds any open repository issues or PRs missing from the
     GitHub Project v2 board once the token carries `project` scope).
- **Skill (`semengine-status`).** Add `.agents/skills/semengine-status/SKILL.md` and its Claude adapter
  `.claude/skills/semengine-status/SKILL.md`, registered in `.agents/README.md` and
  `internal/harness/contract/agentsetup_test.go`, so any session can run `/semengine-status` or `task status` to
  report port progress, audit issue/milestone hygiene, and sync the GitHub Project tracker.
- **Repository map (`docs/repository-map.md`).** Update the Slice 04A summary bullet (`:13-17`), the milestone slug list
  (`:117`, `:129-131`), and add the `scripts/port-status.sh` row so the map matches current truth.

## Capabilities

- Added: `port-status`.

## Impact

- `scripts/port-status.sh`, `Taskfile.yml`, `AGENTS.md`, `.agents/README.md`,
  `.agents/skills/semengine-status/SKILL.md`, `.claude/skills/semengine-status/SKILL.md`,
  `internal/harness/contract/agentsetup_test.go`, `internal/harness/contract/portstatus_test.go`, and
  `docs/repository-map.md`.
- Code pull request (`Taskfile.yml`, `scripts/port-status.sh`, `internal/harness/contract/*`): requires cross-agent
  review before merge.
