# Proposal: fix-262-stale-days-operand

## Why

`scripts/openspec-queue.sh` loops forever when `--stale-days` is its last argument (#262). The argument loop
(`scripts/openspec-queue.sh:63`) runs `STALE_DAYS="${2:-7}"; shift 2`. With one argument left, `shift 2` fails without
shifting, so `$1` stays `--stale-days` and `while [ $# -gt 0 ]` never ends. `timeout 5 scripts/openspec-queue.sh
--stale-days` exits 124. The same line also accepts any other text as the number of days. `--stale-days abc` prints
`integer expression expected` for each change, drops every stale note, and exits 0. `--stale-days --strict` uses
`--strict` as the number of days, so the run is not strict and exits 0 (`inventory.md`, item 1).

## What Changes

- The script refuses a `--stale-days` operand that is not a whole number. It prints `--stale-days needs a whole
  number of days` on standard error and exits 2 before it reads anything. This is the fix #262 quotes from
  semsparql 289d791.
- The usage line (`:50`) and the exit-2 sentence (`:41`) of the header, which `--help` prints, say so. The header
  keeps its line count, so the `--help` range `2,51` (`:64`) still covers it.
- `spec-queue` gains two short requirements on argument handling.
- The contract tests bound each run of the script, so a run that does not end fails the test instead of hanging the
  suite.

## Capabilities

### Modified Capabilities

- `spec-queue`: ADDED "--stale-days takes a whole number of days" and "An unknown argument is refused" (each
  under the 500-character requirement limit #305 tracks). At sync, the Purpose's "exit 2 means it could not read the
  changes" gains "or refused its arguments".

## Impact

- Code: `scripts/openspec-queue.sh`; `internal/harness/contract/specqueue_test.go`.
- Docs: `docs/testing.md:371-372` names the new test; `docs/repository-map.md` rows at archive.
- Behaviour: arguments that hung the script, or that it accepted and silently misread, now exit 2. No caller in the
  tree passes `--stale-days` (`Taskfile.yml:118`, `:123`).
