# lock-identity-locale

Status: revision 1, draft. It rests on `inventory.md` (revision 2) and `design.md` (revision 1). Issue #126, owner
ruling #126 comment 6048776879, claim PR #127.

## Why

The integration runner records its lock owner's start time as `identity`, the text `ps -o lstart=` prints in the
runner's own environment, and a later runner judges the owner stale when its own read of that pid prints another text
(`scripts/test-integration.sh:101`, `:172-173`). That text follows the reader's time zone and locale. So two runners on
one host whose shells differ in either read different texts for one live owner, and the second quarantines the first
one's lock while it runs Docker work (#126). The lock is shared with SemStreams, whose runner at the pin writes and
judges the same way and ignores a key it does not know. The owner ruled a fix in SemEngine only.

## What Changes

- **One more key.** The runner writes SemStreams' six owner keys exactly as today, then `identity_utc`: the same start
  time read with `TZ=UTC` and `LC_ALL=C` set for `ps`, or `unknown`. Its text is the same whatever the reader's time
  zone or locale. SemStreams' runner ignores it (measured at the pin).
- **One rule for both judges.** The runner's stale check and the fixture's admission compare `identity_utc` when the
  record has a non-empty one, and `identity` otherwise, as today (`design.md`, D2). A record without `identity_utc`
  reaches the runner from SemStreams or from a SemEngine runner older than this change; it reaches admission only from
  such an older SemEngine runner or from a test, since only SemEngine's runner exports the admission token (D4).
- **Admission and another shell.** A direct `go test` from a shell whose time zone or locale differ from the
  runner's, holding a copy of a live runner's token, is now admitted, as it already is from a shell that matches (D3).
- **Texts that said the record is SemStreams' format byte for byte** change: the runner's spec, the admission
  ledger's runner row, two script comments, and R4's key-list oracle (D5).
- **Tests.** A two-runner test in two time zones (#126 end to end) and new runner and admission cases, run first on
  the base code, where the cases that change behaviour fail; five wrong changes run with `task mutate:check` (D6).

Not in this change: anything in SemStreams. A SemEngine runner and a SemStreams runner whose shells differ keep
today's limit until SemStreams reads the key (accepted in the ruling); so does a SemEngine runner older than this
change. Also out: `task doctor`, a native start-time read, and a wall-clock step on Linux (`design.md`, L3).

## Capabilities

### Modified Capabilities

- `integration-test-runner`: "Shared host lock" lists SemStreams' six keys followed by `identity_utc`, defines it,
  judges liveness by it when it is usable and by `identity` otherwise, states the limit that remains, and gains two
  scenarios: an owner in another time zone or locale is respected, and the record's format.
- `nats-fixture`: "Admission before Docker" judges liveness as the runner's spec now defines it, says which records
  can lack `identity_utc` there, and gains two scenarios: a test process in another time zone is admitted, and a
  record without `identity_utc` is judged by `identity`.

## Impact

- `scripts/test-integration.sh`, `scripts/admission-lock.sh` (a comment), `docs/admission-ledger.yaml` (the runner
  row), `internal/harness/natsfixture/admission.go`, `admission_test.go`, `internal/harness/runner/runner_test.go`; the
  two specs and `docs/repository-map.md` by the archive.
- A code pull request: Codex's review of record is needed before merge.
- The owner record gains one key; no exported name, flag, environment variable or exit status is added. The record is
  no longer byte-identical to SemStreams' format; SemStreams' runner reads it as before (measured at the pin only:
  `design.md`, owner decision 5).
- PR #93 changes three other files of the same Go package and ADDs a different `nats-fixture` requirement; no file is
  shared and neither waits (`design.md`, "Overlaps and order").
