# Design: port-status-tracker

base: `ad9fda0` (`main`). Not a SemStreams package port: no pin probe, shape sweep, or pin ADR carry-in applies.
Owner decisions recorded in Enjoy session "Port Status and Issue Tracking Audit" (2026-10-22):

1. Rename the four GitHub milestones in place to `setup`, `slice-04a`, `slice-04b`, and `slice-04c` (keeping the full
   descriptive text in each milestone's `description`), and apply the milestone/label audit across all open issues.
2. Provide `task status` + the `semengine-status` skill + GitHub Project v2 sync support.
3. Close obsolete issue #128 (`scripts/test-integration.sh` removed in #205) and open a claimed PR for `task status`,
   the tracker skill, and `docs/repository-map.md`.

## D1. `scripts/port-status.sh` and `task status`

`scripts/port-status.sh` is a read-only bash + python3 script invoked by `task status -- {{.CLI_ARGS}}`.

### Modes and flags

- Default (`scripts/port-status.sh`):
  1. **Local ledger & tree scoreboard (always runs, needs no network):** Parses `docs/admission-ledger.yaml` against the
     65 Tier-0 packages in the seven Slice 04A changes (`D2` of `2026-10-01-setup-04a-foundation/design.md`) and checks
     the 7 `service` lifecycle-owner files (`base.go`, `service_manager.go`, `component_manager.go`,
     `message_logger.go`, `storage_observability.go`, `metrics_forwarder.go`/`heartbeat.go`, `log_forwarder.go`) in the
     tree. Prints per-change counts (`ported`, `partial`, `defer-exclude`, `remaining`), the `service` PR 1–7 checklist,
     and the overall 65-package total.
  2. **GitHub live state (runs unless `--offline` is passed):** Reads milestones, open PRs, and open issues via `gh`.
     Fails closed with exit 2 if a `gh` read fails or returns invalid JSON (matching `scripts/merge-check.sh`). Prints:
     - Milestone progress (`setup`, `slice-04a`, `slice-04b`, `slice-04c` open/closed counts and any milestone whose
       title contains whitespace).
     - Active PR claims (number, draft state, branch, milestone, title).
     - Active `class:flake` issues (highlighting that any open flake blocks `task merge:check`).
     - Issue queue hygiene: count of open issues missing a milestone or missing labels, listing each offending `#n`.
- `--offline`: runs only part 1 (local ledger + `service` split scoreboard) without calling `gh`.
- `--check`: runs both parts (or local-only with `--offline`) and exits 1 if:
  - any open issue has `milestone == null` or zero labels, or
  - any open milestone title contains whitespace or is not one of `setup`, `slice-04a`, `slice-04b`, `slice-04c`.
- `--sync-project [<project-number>]`: checks `gh project list --owner C360Studio` (creating project
  `SemEngine Port Tracker` if no project exists and none is specified), then ensures every open issue and open PR in
  `C360Studio/semengine` is added to the project via `gh project item-add`. If the `gh` token lacks `read:project` or
  `project` scope, prints the exact `gh auth refresh -s project` command on stderr and exits 2.

## D2. Skill and adapter (`semengine-status`)

- Canonical skill: `.agents/skills/semengine-status/SKILL.md`. Instructs agents how to run `task status`,
  `task status -- --check`, and `task status -- --sync-project`, how to assign milestones (`setup`, `slice-04a`,
  `slice-04b`, `slice-04c`) and labels when filing or triaging issues, and how to interpret the 65-package Slice 04A
  scoreboard.
- Claude adapter: `.claude/skills/semengine-status/SKILL.md` (<= 25 lines, links to
  `../../../.agents/skills/semengine-status/SKILL.md` and instructs to read it fully, no checkboxes).
- Registered in `.agents/README.md` and `sharedSkills` in `internal/harness/contract/agentsetup_test.go`.

## D3. Contract tests (`internal/harness/contract/portstatus_test.go`)

- `TestPortStatusOffline`: runs `scripts/port-status.sh --offline` against the live repo root and verifies exit 0 and
  that all seven Slice 04A changes (`setup-04a-01-floor` through `setup-04a-07-substrate-seam`) and the `service` PR
  split appear in stdout.
- `TestPortStatusCheckWithFakeGH`: runs `scripts/port-status.sh --check` with a fake `gh` binary on `PATH`:
  - clean fixture (milestones `setup`, `slice-04a`, `slice-04b`, `slice-04c`; all open issues have milestone + label):
    exits 0;
  - issue missing milestone: exits 1, naming the issue number;
  - issue missing labels: exits 1, naming the issue number;
  - milestone title with spaces (`Slice 04A`): exits 1, naming the milestone;
  - failing `gh` call: exits 2 (`unavailable`).

## D4. `docs/repository-map.md` updates

- Line 13–17: replace the stale "`setup-04a-03` to `setup-04a-07` are not started" text with the current truth
  (`setup-04a-01-floor` and `setup-04a-02-ingest-kernel` archived; `setup-04a-03-boot` in progress with `health`,
  `config`, `internal/flowgraph`, `internal/logforwarderpolicy`, `pkg/rulepack`, `composition`, and `service` PRs 1–3
  merged; `vocabulary/export` ported ahead of Change 7).
- Lines 117, 129–131, 138: update milestone names to `setup`, `slice-04a`, `slice-04b`, `slice-04c` with their
  descriptions.
- Present table: add `scripts/port-status.sh` (`task status`).
