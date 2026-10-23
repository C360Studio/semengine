# Inventory: fix-262-stale-days-operand

Base: main 46d3cd5. Issue #262, claim PR #381.

## 1. The claimed gap

The issue says line 57; the argument loop is at `scripts/openspec-queue.sh:59-67`. Line 57 is `STALE_DAYS=7`, and
`:63` is `--stale-days) STALE_DAYS="${2:-7}"; shift 2 ;;`. Reproduced: `timeout 5 scripts/openspec-queue.sh
--stale-days` exits 124. Probe on bash 3.2.57 (`/bin/bash`, the only bash on `PATH`): `set -- --stale-days; shift 2`
exits 1 and leaves one argument. The operand has one reader, `:237`: `[ "$age_days" -ge "$STALE_DAYS" ] &&
age_note=...`.

Run in a throwaway root with a stand-in CLI and a dated change, the same line has three more defects, all failing open:

- `--stale-days abc` prints `[: abc: integer expression expected` for each change, drops every stale note, and exits 0.
- `--stale-days --strict` takes `--strict` as the operand, so the run is no longer strict and exits 0.
- `--strict --stale-days abc` exits 0.

Other operands, as the base reads them:

- `0` and `08` work: the test reads `08` as decimal, and `0` marks every dated change stale.
- `-1` is accepted and marks every dated change stale.
- A digit string above bash's largest integer, from `9223372036854775808` (19 digits) up, passes the issue's digit
  pattern but fails `[ -ge ]`: each change prints `integer expression expected` on standard error, every stale note
  drops, and the run exits 0. It fails open (owner question Q1 in `design.md`).
- Every probe ran on bash 3.2.57 only. CI runs bash 5; its run of the new test cases is the bash 5 probe.
- There is no `--stale-days=N` form; it already exits 2 as an unknown argument.

## 2. Other spellings

- The default `7` appears at `:57`, and `:63` has a second default, `${2:-7}`.
- The usage line is `:50` (`# scripts/openspec-queue.sh [--strict] [--stale-days N]`).
- `:41` says "Exit 2 means the queue could not be read.", which is already incomplete: `:65` sends an unknown argument
  to exit 2.
- `--help` (`:64`) prints the fixed range `sed -n '2,51p'`, so adding a header line breaks `--help` unless the range
  moves.
- The spec Purpose (`openspec/specs/spec-queue/spec.md:8`) has the same incomplete "exit 2 means it could not read the
  changes". No spec requirement covers arguments: `stale`, `argument` and `usage` find only `--strict` and `--check`.
- `git grep stale-days` outside the script finds nothing.

## 3. Adjacent claims

- Callers: `Taskfile.yml:118` (`--check`) and `:123` (no arguments; `spec:queue` does not pass `CLI_ARGS` through). Only
  someone running the script directly reaches `--stale-days`.
- Test: `internal/harness/contract/specqueue_test.go:50-81`. `runSpecQueue` runs `exec.Command(...)` with no bound, and
  `task test:unit` (`Taskfile.yml:86`) sets no `-timeout`. A test that fails on the base would hang until `go test`'s
  default 10-minute timeout, and the panic would take the whole package with it.
- Precedent for a bound: `message/generic_json_shape_test.go:166-168` (`context.WithTimeout(t.Context(),
  10*time.Second)` and `exec.CommandContext`). The killed child returns exit -1, so the test fails instead of hanging.
  No sleep and no port. The file comment at `specqueue_test.go:18` says "all four requirements".
- `docs/testing.md:371-372` lists the four `TestSpecQueue*` tests.
- `.agents/rules-index.md:99` and `AGENTS.md:154` are about holds, not arguments; neither changes.
- `task mutate:check` takes only `.go` files (`internal/harness/mutcheck/inputs.go:131`), so sensitivity runs by hand
  (`docs/testing.md:230`).
- Open PRs: #381 (this claim) and #376 (ready for review since this inventory began, 29 files under `service/`,
  `docs/`, and `openspec/changes/setup-04a-03-boot-service-storage-observability/`). The only shared file is
  `docs/repository-map.md`. Neither depends on the other; whichever lands second merges `origin/main` and resolves the
  map rows.
- Open issues: `openspec-queue` finds #262 and #7; `stale-days` finds #262 and #9. #7 and #9 contain neither string in
  their bodies or comments; they match the search words only. #262 is retired by this change.
- The script has no ledger or provenance row.

## 4. Who it is for

No new exported symbol, subject, bucket, or config field. The new surface is one standard-error line and an exit
status; its contract line is the ADDED requirement.

## 5. Problem shape

Refuse a bad argument at the boundary. The nearest existing instance is the script's own `:65` (`unknown argument: $1`
with exit 2). The sister fix is semsparql commit 289d791, quoted in full in #262.

## 6. Pin and adopters

No package is carried from the pin; this is a script fix. No consumer reaches this script from outside the repo.
