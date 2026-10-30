# Design: adopt-ateam-spec-queue

## Context

- `scripts/openspec-queue.sh:1-304`: reads `openspec list --json` through python3, then each `tasks.md`; labels
  open tasks' first lines, prints each hold from `Hold:` on, `--check` fails on a stray hold (exit 1) and on an
  unreadable file (exit 2), `--strict` exits 1 on any reported line, `--stale-days` is parsed and unused by any
  caller (`git grep` on `main`: `Taskfile.yml` calls the script bare and with `--check` only).
- ateam v0.3.4 `internal/state/openspec.go`: `ParseTasksMarkdown` (lines 150-232) is the script's block and label
  logic ("parses a tasks.md file according to the SemEngine spec-queue specification"); `ScanOpenSpecQueue`
  (304-355) reads the `openspec/changes/*` directories, skipping `archive`, no CLI; `CheckOpenSpec` (443-525) runs
  `openspec validate --all --strict --no-interactive` from `node_modules/.bin` or `PATH` under a 10 s deadline, else
  prints `strict validation skipped` and goes on; then stray holds (exit 1), unreadable files (exit 2), the 60-line
  proposal and 120-line design warnings, `holds: ok (<n> tasks.md read)`. `cmd/ateam/main.go:188-215`:
  `spec-queue` exits 0 or 2, `spec-check` 0, 1 or 2.
- `internal/harness/toolcheck/specqueue_test.go`: five tests drive the script against a stand-in `openspec` and
  planted files; `TestSpecCheckWiring` (lines 469-512) checks the `spec:check` Task entry; its `lines` helper
  (line 100) has no caller outside the file.
- `scripts/doctor.sh:59-62`: the python3 row names the queue script; `scripts/port-status.sh` also runs python3
  (lines 57, 211, 319, 361, 388), and `task status` runs that script.
- semteams after its #375 epic: `spec:queue` is `ateam state spec-queue`; `spec:check` still runs its own script
  beside `ateam state spec-check` (layered, the shape this epic avoids).

## Goals / Non-Goals

Goals: the queue and the hold check are ateam's at the v0.3.4 pin; `task spec:check` stops when no `openspec` is
reachable instead of passing with validation skipped; the Task wiring is guarded by a test shown able to fail.
Non-goals: `ateam test-spec` (deferred until ateam #237, #447 ruling); the hook and MCP surfaces, already ateam's
since PR A; any change to the `tasks.md` grammar.

## Decisions

- **D1 `spec:check` keeps the validate line.** Option A: `ateam state spec-check` alone; one command, but a
  missing CLI degrades to a warning and exit 0 (`openspec.go:467-485`), so a tree with no `node_modules` passes
  strict validation it never ran. Option B (chosen): the validate line first, as #450 says, then the ateam check;
  with no `openspec` reachable `npx` exits 1 and the check never runs (`notes/2.1.md` measures where `npx` looks).
Cost: validation twice, 0.8 s each. The divergence is recorded in the proposal.
- **D2 the python3 row stays.** `task status` runs `scripts/port-status.sh`, five python3 calls; the doctor row's
  comment names it. Removing the row (#450's wording) would leave `task status` failing with no doctor line.
- **D3 one wiring test, `TestSpecTaskWiring`.** The retired `specCheckWiringViolations` shape, extended to the
  `spec:queue` entry: each task a mapping with only `desc`, `summary` and `cmds`, each command a plain string,
  the commands exactly those the spec names, in order. Sensitivity cases: a line removed, the two swapped,
  `ignore_error` on the task.
- **D4 the spec keeps three requirements.** What the queue prints and when the check fails are stated as the
  pin's behaviour with scenarios a reader can run on this tree; the 23 script requirements go with Reason and
  Migration lines, as the `merge-gate` spec did in PR B.

## Risks / Trade-offs

- ateam's validation deadline is 10 s; this repository validates in under a second. Past the deadline the check
  exits 1 naming the timeout, which fails closed.
- `ateam state spec-queue` lists a change directory with no `tasks.md` as `0/0`, where `openspec list` omitted it.
  Review only; no such directory exists on `main`.

## Migration Plan

1. Proposal, design, tasks, spec delta; draft PR; owner fills `Approved-by:` and removes the hold.
2. Task wiring, script and test removal, the wiring test, doctor comment, docs; `task verify`.
3. In-house review, Codex record, archive, merge. No rollback step: the script is in history at `127d252`.

## Open Questions

None. The one divergence from #450 (D2) is a measured fact, not a ruling.
