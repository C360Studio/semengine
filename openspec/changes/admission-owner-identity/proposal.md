# admission-owner-identity

Status: revision 2, draft. It rests on `inventory.md` (revision 2) and `design.md` (revision 2), which answer review
round 1. Issue #124, claim PR #125.

## Why

The integration runner and the NATS fixture's admission check judge whether the integration-test lock's owner is
still running by different rules. The runner calls the owner dead when its pid is gone or now names a process with
another start time (`scripts/test-integration.sh:167-174`); admission checks host and pid only
(`internal/harness/natsfixture/admission.go:75-91`), as the `nats-fixture` spec says. So when a runner is killed with
SIGKILL, its pid is reused, and a shell still exports its token, a direct `go test` is admitted without the lock, and
the next runner quarantines the lock and runs Docker work beside it (#124). Admission exists to stop exactly that
(`admission.go:24-25`).

## What Changes

- **One definition.** `integration-test-runner` › "Shared host lock" defines the owner record's `identity` (the
  runner's start time as `ps -o lstart= -p <pid>` prints it, leading blanks removed, or `unknown`) and a live owner
  (this host, and ps prints that identity for its pid). `nats-fixture` › "Admission before Docker" refers to that
  definition and states that admission refuses what it cannot prove live, an `unknown` identity included, where the
  runner respects what it cannot prove dead. No runner code changes.
- **Admission.** `admit` reads the owner's start time with the runner's own command, under `Start`'s context, and
  refuses when the text differs from the record's identity or cannot be read. The `kill(pid, 0)` check goes: ps
  answers both questions. If `Start`'s context has ended when the read fails, `Start` returns the context's error.
- **`task doctor`'s comment.** `scripts/doctor.sh:107` claims doctor judges the lock "the way the runner would judge
  it"; doctor checks host and pid only, so a reused pid reads as busy there. The comment is corrected; doctor's rule is
  not changed (`design.md`, D1).
- **Tests.** `admission_test.go` plants owners with their real identity, read by the runner's own command text. Two
  refusal cases (another start time; `unknown`) and an admission test with a cancelled-context case are added. Four
  wrong changes are run with `task mutate:check` (`design.md`, D4).

Not in this change: two measured runner findings, also present in SemStreams' runner at the pin: its `kill -0` calls
another user's process dead (accepted: the sticky `/tmp` turns the attempted quarantine into "busy"), and runners in
different time zones or locales read different identities (tracked in #126). Also out: a native start-time read and
any change to the owner record's format, which SemStreams shares (`design.md`, "Not in this change").

## Capabilities

### Modified Capabilities

- `integration-test-runner`: "Shared host lock" defines `identity` and a live owner, states ps's one-second
  resolution, says the stale rule's pid half is a pid `kill -0` cannot signal (as the code has it), and gains a
  scenario for the rule's start-time half, which R2's "changed identity" case already tests.
- `nats-fixture`: "Admission before Docker" admits only under a live owner as the runner's spec defines it, refuses
  what it cannot prove live, and returns the context's error if `Start`'s context has ended when the read fails; three
  scenarios are added.

## Impact

- `internal/harness/natsfixture/admission.go`, `fixture.go` (one line), `admission_test.go`; `scripts/doctor.sh`
  (one comment line); the two specs (by the archive); `docs/repository-map.md` (the archive row and the specs row).
- A code pull request: Codex's review of record is needed before merge.
- No new exported name, flag, environment variable, owner-record key or exit status. On a host where the runner
  records `identity=unknown`, every fixture test is now refused, with the reason.
- PR #93 changes three other files of the same Go package and ADDs a different `nats-fixture` requirement; no file is
  shared and neither waits (`design.md`, "Overlaps and order").
