---
name: semengine-preflight
description: >-
  Select and run the existing SemEngine verification gates for a concrete diff and record their evidence.
  Use before implementation pushes or when assessing local readiness.
---

# Verify a SemEngine change

The [shared protocol](../../protocol.md) owns claims and landing. Read it first. This skill selects existing checks and
records evidence; it grants no merge, tag or cleanup authority.

## Establish the diff and its owner

Work in the claim's worktree. Verify its branch, HEAD, dirty/untracked files, upstream and PR target before
choosing checks. Compare the committed diff against the actual PR base, including a non-default target in a
stack; also inspect the staged and unstaged work. Do not assume the discovery checkout's local `main` is current.
Before every commit or push, verify the branch still matches the claimed PR head.

## Select the existing gates

One Task entrypoint serves local work and CI. `task --list` shows what exists; inspect the `Taskfile.yml` and the
workflows under `.github/` when exact coverage matters. A task name or old successful log does not prove parity
with current CI.

| Task | What it does |
| --- | --- |
| `task fmt` | Formats Go sources; the only task here that changes files |
| `task fmt:check` | Fails if any Go source needs formatting; writes nothing |
| `task doctor` | Reports tool versions and Docker reachability against their pins; starts no workloads |
| `task spec:check` | Validates all OpenSpec changes and specs strictly |
| `task spec:queue` | Shows each in-flight OpenSpec change with its holds and blocking conditions |
| `task docs:check` | Lints Markdown with `markdownlint-cli2` |
| `task tidy:check` | Fails if `go.mod` or `go.sum` are not tidy |
| `task build` | Builds all packages |
| `task vet` | `go vet` |
| `task lint` | Pinned `revive` |
| `task vuln` | Pinned `govulncheck`; review findings against reachable behavior |
| `task test:unit` | Unit tests with the race detector |
| `task verify` | The checks above except `doctor`, `fmt`, `spec:queue`, cheapest first; fails if tracked files changed |

CI has two jobs: `verify` runs `task verify`, and `required` fails if `verify` failed, is missing, or was skipped or
cancelled. Integration and consumer lanes join `task verify` when their packages and workload exist; until then no
task for them exists, and a missing lane is not a passing one.

Select by what the diff changes:

- **Documentation or skill instructions only:** `task fmt:check`, `task docs:check`, and `git diff --check`; add
  `task spec:check` when `openspec/` changed.
- **An OpenSpec change:** `task spec:check`, then read `task spec:queue` in this worktree.
- **Go behavior:** `task test:unit`, `task vet`, `task lint`, `task build`; focused tests first during iteration.
- **Dependencies or `go.mod`:** `task tidy:check` and `task vuln` in addition to the Go gates.
- **Before any implementation push:** `task verify`.

Do not hand-run a narrower command in place of a gate CI runs: the task owns flags and pins.

## Protect shared test infrastructure

Serialize heavy work on a shared host and do not bypass a lock a task reports. Docker inspection is diagnostic:
container age, a timeout signature, or a name pattern does not establish that a resource is abandoned or that a
failure is infrastructure-only. Preserve other sessions' resources. Cleanup is limited to an identified completed or
abandoned run this session is authorized to clean up; host-wide pruning and stopping an unknown port holder are not
preflight steps.

## Report evidence and remaining gates

Record the tested HEAD (or dirty snapshot), exact command, exit status, assertions/tests actually exercised,
and log/artifact location. Separate failure, skip, no selected tests and compilation-only results. When the
implementation changes, older green results remain evidence for the older revision. Preserve the command's
failure status when filtering output; do not infer success from a quiet log.

Resolve failures from evidence; an isolated successful rerun is not a fix for a known required-job flake.
Apply the protocol's fix-or-recorded-waiver rule. Long or paid runs need the role contract's active progress
checks and bounded stopping behavior.

After local verification, assess the PR's current-head hosted checks and the protocol's review/archive gates.
Local green is not merge authorization. Follow existing user authorization without asking for it again, while
preserving all outstanding required gates.
