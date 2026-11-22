# Proposal: chore-716-serial-milestones-status

Approved-by: cglusky (2026-11-22) <https://github.com/C360Studio/semengine/issues/716#issuecomment-7080997682> sha256:76f3f3d3b74c

Track: `quick`. Modifies one capability (`port-status`) to update its scenario example and Project v2 stage mapping for
the serial release milestones; adds no new requirement.

## Why

GitHub milestones moved from the provisional slice slugs (`slice-04a`, `slice-04b`, `slice-04c`) and
`rules-durable-execution` to serial pre-release milestones (`v0.1.0-alpha.7`, `v0.1.0-alpha.8`, `v0.1.0-alpha.9`,
`v0.1.0-alpha.10`) alongside `tier-2` and `backlog`. `docs/repository-map.md`, the `semengine-status` skill,
`openspec/specs/port-status/spec.md`, and `scripts/port-status.sh`'s GitHub Project v2 `Stage` mapping still name the
retired slice slugs (`docs/repository-map.md` also stops its tag list at `v0.1.0-alpha.4`).

## What Changes

- **`docs/repository-map.md`:** Update the pre-release tag range (`v0.1.0-alpha.1` to `v0.1.0-alpha.6`), `Where state
  lives`, and `Releases and milestones` to name the serial `v0.1.0-alpha.<N>` release milestones, `tier-2`, and
  `backlog`.
- **`semengine-status` skill:** Update `.agents/skills/semengine-status/SKILL.md` and
  `.claude/skills/semengine-status/SKILL.md` so milestone guidance and `gh issue edit` examples use the serial
  `v0.1.0-alpha.<N>`, `tier-2`, and `backlog` slugs.
- **`scripts/port-status.sh` and `portstatus_test.go`:** Map open issue milestones onto the Project v2 `Stage` slots
  (`v0.1.0-alpha.7` -> 2, `v0.1.0-alpha.8` -> 3, `v0.1.0-alpha.9` -> 4, `v0.1.0-alpha.10` -> 5,
  `tier-2`/`backlog`/other -> 6; active PRs and claimed issues remain 1), and verify both clean serial milestones and
  the `Stage` option selection in `internal/harness/toolcheck/portstatus_test.go`.
- **`port-status` spec delta:** Update the clean-milestone scenario example in `openspec/specs/port-status/spec.md` to
  use serial milestone slugs.

## Non-goals

`docs/admission-ledger.yaml`, the 65-package offline scoreboard, and `docs/setup-plan.md` remain unchanged.

## Capabilities

- Modified: `port-status`.

## Impact

`docs/repository-map.md`, `.agents/skills/semengine-status/SKILL.md`, `.claude/skills/semengine-status/SKILL.md`,
`scripts/port-status.sh`, `internal/harness/toolcheck/portstatus_test.go`, `openspec/specs/port-status/spec.md`. One PR,
`Closes #716`.
