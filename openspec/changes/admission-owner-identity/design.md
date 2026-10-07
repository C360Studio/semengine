# Design: admission-owner-identity

Status: revision 2, draft. It rests on `inventory.md` revision 2. Review round 1 asked for changes to both; revision
2 answers them: `scripts/doctor.sh` as a third judge (D1, "Files"), the context's error after a failed read (D2, D3),
the SemStreams pin probe and the two runner findings ("Not in this change"), the runner requirement's wording and
I2's exceptions (spec delta), the stale `absentPID` sentence and the identities of the absent-pid and `pid=x` cases
(D4), and a wall-clock step on Linux (D3). Issue #124, claim PR #125. Probes P1 to P9 and every pin are the
inventory's.

## Context

Two places judge whether the integration-test lock's owner is still running, by different rules (inventory § 2). The
runner, which writes the owner record, calls an owner stale when its pid is gone or now names a process with another
start time (`scripts/test-integration.sh:167-174`). Admission checks host and pid only
(`internal/harness/natsfixture/admission.go:75-91`), as its spec says (`openspec/specs/nats-fixture/spec.md:13`). So
after a runner is killed with SIGKILL and its pid is reused, a shell that still exports the dead run's token is
admitted without the lock (#124). No spec defines the runner's "start identity" (inventory § 2).

## Options

1. **Admission reads the start time with the runner's own command and compares it with `identity`** (recommended,
   D1 to D3). The definition is stated once, in the runner's spec; the fixture's spec refers to it. Cost: admission
   runs `ps` once per `Start`; `admit` takes a context; the owner records `admission_test.go` plants must carry a real
   identity.
2. **A native read in Go** (`sysctl kern.proc.pid` on macOS, `/proc` on Linux), formatted to match. Cost: on macOS it
   does not equal the record (P1: ps pads the column with four trailing blanks), and the text follows the reader's
   time zone and locale (P2), so Go would re-implement each system's ps formatting; two OS-specific files and a direct
   `golang.org/x/sys` dependency. It matched on Debian only because the locale was C (P5).
3. **Record a numeric start time that both sides read natively.** Cost: the owner record is SemStreams' format,
   adopted byte-compatibly (ledger `:202-203`; setup-02 design `:133-140`); changing it is a joint change with
   SemStreams and a new ledger row, by the owner's word.
4. **Do nothing.** Cost: #124's sequence admits a direct `go test` without the lock, while `admission.go:24-25` says a
   stale export cannot.

## Decisions

### D1. One rule, in the runner's spec

The runner writes the record (`test-integration.sh:101-102`, `:199`) and owns the lock protocol (the ledger's runner
row), so `integration-test-runner` › "Shared host lock" defines `identity` and a live owner, and `nats-fixture` ›
"Admission before Docker" refers to that definition instead of stating its own. Both requirements are MODIFIED.

- **identity:** the runner's start time as `ps -o lstart= -p <pid>` prints it in the runner's environment, with
  leading blanks removed and trailing blanks kept; `unknown` when that prints nothing.
- **live owner:** its host is this host, and `ps -o lstart= -p <pid>`, read the same way in the reader's environment,
  prints its identity. ps never prints `unknown`, so an `unknown` identity is never live.
- **The runner** quarantines only a same-host owner whose pid `kill -0` cannot signal (another user's process
  included, P4) or whose start time, read the same way, differs from an identity other than `unknown` (today's rule,
  `:167-174`, now stated against the definition); it respects every other owner.
- **Admission** admits only under a live owner.
- **ps reports start times to the second**, so a pid reused within the second its owner started reads as that owner,
  to the runner and to admission alike. The requirement states it.

A record therefore falls in one of three classes: live (admission admits; the runner respects it), provably dead
(admission refuses; the runner quarantines it), or neither: another host, a pid that is not a number, an `unknown`
identity, a start time that cannot be read (admission refuses; the runner respects it). Each side fails closed for its
own action: the runner's action destroys a lock, so it needs proof of death; admission's lets Docker work start, so it
needs proof of life.

No runner code changes. The runner's requirement gains the scenario for the start-time half of its stale rule, which
R2's "changed identity" case already holds (`runner_test.go:387-389`).

**A third judge, `task doctor`** (`scripts/doctor.sh:107-118`, inventory § 2), reports the lock "busy" or "stale
(dead pid; ...)" from host and `kill -0` alone, under a comment at `:107` that says it judges "the way the runner
would judge it". For a reused pid that is false: doctor says busy where the runner would quarantine. Doctor only
reports, nothing tests it (`git grep -n 'doctor' -- internal/harness`: no match), and adopting the definition there
would need a test of its own. So doctor keeps its rule, and this change corrects the `:107` comment to say what doctor
checks: host and pid only, so a pid reused by another process reads as busy here, though the runner, which also
compares the start time, would quarantine it.

### D2. The read

Admission runs `ps -o lstart= -p <pid>` under `Start`'s context (`exec.CommandContext`, as `pindiff/pin.go:34` and
`mutcheck/runs.go:38` do), removes the one trailing newline and the leading blanks and tabs, keeps trailing blanks,
and compares the result with the recorded `identity` byte for byte. That is the runner's line (`:101`) and the read
R1 already shows equal to it on macOS and on ubuntu-24.04 (P1, P6). The existing check that the pid is a positive
integer (`admission.go:83-85`) stays before the read.

- **Environment.** The read inherits the test process's environment. `go test` and the test binary inherit the
  runner's, so time zone and locale match the record, as P2 shows they must. A shell with another `TZ` or locale reads
  another text and is refused: the safe direction.
- **`kill(pid, 0)` goes.** ps answers both questions: for an absent pid it prints nothing and exits 1 on both systems
  (P3, P5). The EPERM branch (`admission.go:87-88`) goes with it: a pid owned by another user is live only if its
  start time is the record's, which a reused pid's is not.
- **Refusals name the reason.** A mismatch names the pid, the recorded identity and the text read; a failed read names
  the pid and the error. Both stay `ErrNotAdmitted` naming `task test:integration` (`admission.go:66`); the refusal
  for a pid with no process keeps the words "not live", which the existing case checks (`admission_test.go:143`).
- **Context.** If `Start`'s context has ended when the read fails, `Start` returns the context's error, as it does
  when the context ended before admission (`fixture.go:113-115`), and makes no Docker call. `admit` checks
  `ctx.Err()` after a failed read, because the read's own error does not say so: a context that ends while ps runs
  makes `exec` kill ps, and the error is `signal: killed`, an `*exec.ExitError` (the reviewer's Go 1.26.4 probe,
  review round 1); a context that ended before the read stops `exec` before ps starts, with `context.Canceled` (P7).
  The test's already-cancelled context exercises the check.

The reader is unexported in `admission.go`. The test copies stay as they are: `runner_test.go:261-271` is R1's
independent oracle for the script, and `prochost_test.go:311-314` the oracle for another fact; making admission's
reader their source would turn independent oracles into the code under test. `prochost.psState` (no context) is not
touched.

### D3. Failure paths, declared

| Case | Admission | Why |
| --- | --- | --- |
| `identity` is `unknown`, or the key is absent | refused. No special case: ps never prints `unknown` or an empty line, so the comparison fails | admission acts on proof of life. The runner records `unknown` only when ps printed nothing for its own pid (`:101-102`), and ps is a declared runner dependency (ledger `:206`). On such a host every fixture test is refused, with the recorded identity in the message |
| ps fails or prints nothing | refused | the pid names no process, or the host cannot show that it does |
| the text read differs | refused | #124's case |
| the read fails and `Start`'s context has ended | the context's error | a cancelled caller is told so, as `TestCancelledStartIsRecognisable` requires of the start phase (`admission_test.go:199-200`) |
| a pid reused within the second the owner started | admitted, and the runner respects it | lstart has one-second resolution, as `prochost_test.go:308-310` notes; the runner's requirement states it. The runner would have to die, and its pid be reused, within the second it started |
| the wall clock is stepped between the runner's write and admission's read (Linux) | refused: a live owner is refused, loudly | the reviewer reports (review round 1) that on Linux lstart is derived from `/proc/stat`'s boot time, which moves when the wall clock is stepped, so the text read no longer equals the record. Not measured, and not known for macOS. Fail closed; a rerun under a fresh runner is admitted. A runner judging another runner's owner after such a step would read it as changed, the same class as #126 |

The runner's degrade on `unknown` (it respects such an owner) stays: its action is the destructive one.

### D4. The tests

All in `internal/harness/natsfixture/admission_test.go`; no `time.Sleep`, no skip, no address or port.

- **The oracle.** `plantLock` records the owner's real identity, read by the runner's own command text run by bash:
  `bash -c 'printf %s "$(ps -o lstart= -p "$1" 2>/dev/null | sed "s/^[[:space:]]*//")"' _ <pid>`. That is `:101`
  with its command substitution, which removes the trailing newline as the runner's does; measured on this Mac equal
  to the runner's line for one process, trailing blanks included. It shares no code with `admission.go`'s Go read. An
  empty result fails the test: its premise, that the test process has a readable start time, does not hold. The
  "owner on another host" case uses the same oracle, so it differs from a live owner by host alone.
- **`TestAdmissionRequiresALiveOwner`** gains an identity per case and two cases. Each goes through `Start` with
  `noDocker`, as the existing three do, and requires `ErrNotAdmitted`:
  - "owner pid names a process with another start time": this host, `os.Getpid()`, identity
    `Mon Jan  1 00:00:00 1990` (R2's value, `runner_test.go:388`). The message names that identity.
  - "owner identity unknown": this host, `os.Getpid()`, identity `unknown`. The message names `unknown`.

  The existing "owner pid is not a running process" and "owner pid unreadable" (`pid=x`) cases carry the test
  process's real identity, read by the oracle, so each differs from a live owner by its pid alone.
- **`TestAdmissionAdmitsALiveOwner`** (new): with `plantLock`'s live owner, `admit(t.Context())` returns no error and
  the planted evidence directory and image; with a context already cancelled, `errors.Is(err, context.Canceled)`.
  Calling `admit` is the lowest level that shows admission passing without a Docker call.
- **Comments.** In `absentPID`'s comment, the sentences at `admission_test.go:117-120` go stale: "kill(pid, 0) finds
  no such process, which is how ownerLive sees an owner that has exited and been reaped" (`kill` is no longer called)
  and "admission compares host and pid alone, so a reused pid admits (#50) ... which admission does not read". They
  are rewritten to the new rule, as is `ownerLive`'s comment (`admission.go:71-74`).

The tests that plant a live owner and go on to a Docker double (`TestStopHonoursItsContextWhileStartHoldsTheFixture`,
`TestCancelledStartIsRecognisable`) hold the match path too. End to end, every test under `task test:integration` is
admitted through the real runner's record, on macOS locally and on ubuntu-24.04 in CI: if the Go read and the
runner's record differed on either system, every fixture test there would be refused.

**Generated checks: examples suffice.** The rule is a conjunction of three independent conditions (this host, a pid
that parses, an equal start time). One example fails each and one passes all; the comparison is equality of opaque
strings. What could go wrong is the read producing another text than the runner's (padding, environment, field). No
generated string finds that; P1, P5, P6 and the integration run on two systems do.

**Shown able to fail** (`task mutate:check -- -pkg ./internal/harness/natsfixture -file
internal/harness/natsfixture/admission.go`, each wrong change in a copy outside the repository):

| Wrong change | Test that must fail | Expected |
| --- | --- | --- |
| M1: the comparison is removed (the defect #124 names) | `TestAdmissionRequiresALiveOwner/owner_pid_names_a_process_with_another_start_time` | detection |
| M2: an `unknown` identity is treated as live (the runner's leniency copied) | `TestAdmissionRequiresALiveOwner/owner_identity_unknown` | detection |
| M3: the read is trimmed with `strings.TrimSpace` | `TestAdmissionAdmitsALiveOwner` | detection on macOS (P1's trailing blanks). On Linux a survivor, equivalent there: procps-ng 4.0.2 (P5) and 4.0.4 (the reviewer, review round 1) print no padding. Run on macOS; a run on Linux is recorded as "survivor, equivalent on Linux" |
| M4: the read uses `exec.Command`, without the context | `TestAdmissionAdmitsALiveOwner`, its cancelled-context case | detection |

M4 assumes `admit` checks the context only after a failed read (D2). A check of the context before the read, beside
`Start`'s own (`fixture.go:113-115`), would make M4 a survivor by construction; it is then recorded as such.

## Invariants and their spec homes

- **I1.** `Start` makes a Docker call only when the owner record holds the test's token and is live: its host is
  this host and ps prints its identity for its pid. Home: `nats-fixture` › "Admission before Docker" (MODIFIED), which
  refers to `integration-test-runner` › "Shared host lock" (MODIFIED) for "live".
- **I2.** The runner does not quarantine a live owner: a live record's start time, read again, equals its identity,
  and the runner quarantines a pid it can signal only when that read differs. Two exceptions are declared: a pid
  reused within the second the owner started (D3), and a pid the runner may not signal, which it calls dead ("Not in
  this change"). Home: "Shared host lock", which states both exceptions.

## Files

- `internal/harness/natsfixture/admission.go`: `admit(ctx)`, `ownerLive(ctx, host, pid, identity)`, the reader, and
  the comments at `:24-26` and `:71-74`.
- `internal/harness/natsfixture/fixture.go:129`: `admit(ctx)`. One line; #93 does not change this file.
- `internal/harness/natsfixture/admission_test.go`: D4.
- `scripts/doctor.sh:107`: the comment only (D1). No open pull request changes this file.
- Not changed: `scripts/test-integration.sh`, the rest of `scripts/doctor.sh`, `internal/harness/runner/`,
  `internal/harness/prochost/`, `docs/admission-ledger.yaml`, `AGENTS.md` (no new rule; the failure paths are declared
  above).

## Not in this change

SemStreams' runner at the pin writes and judges the identity exactly as SemEngine's does (P8:
`scripts/run-integration-tests.sh:50-53` and `:97-106` at `8b99efe9`), so both findings below hold for it too.

- **Runner finding 1, accepted: the runner's `kill -0` calls another user's process dead** (P4; inventory § 2). For
  a same-host record whose pid now belongs to another user, the runner judges the owner stale and tries to quarantine
  the lock. On the shared lock the move is refused: `/tmp` is sticky (P9: macOS `/private/tmp` is `drwxrwxrwt`; the
  reviewer measured `mv` of another user's directory in ubuntu:24.04's `/tmp`: `Operation not permitted`, exit 1), so
  `clean_stale_lock` fails and the runner falls through to "busy" (`test-integration.sh:209-214`). The spec delta
  states the rule as the code has it. A fix would be on the read side and change no byte of the record; it is not
  needed for the shared lock, and admission no longer depends on the check (D2).
- **Runner finding 2, declared and out of scope: two runners in different time zones or locales** read different
  identities for one owner (P2), so one would judge the other's live owner changed and quarantine its lock. Present at
  the pin (P8). The fix sets the environment of every `ps -o lstart` read on both sides, a joint change with
  SemStreams. Tracked in #126. Admission is not exposed: it reads in the runner's own environment.
- A native read (Option 2) and a numeric record (Option 3).
- Moving `runner_test.go`'s or `prochost_test.go`'s reader into shared code (D2).
- `prochost.psState`'s missing context.

## Overlaps and order

- **#93** shares no file with this change. Both change the `nats-fixture` spec: #93 ADDs "Connected value for a
  package's tests", this change MODIFIES "Admission before Docker", so either archive applies after the other. Both
  archives will likely amend `docs/repository-map.md:55`; the second to merge resolves it when it merges
  `origin/main`. Both work in the same Go package, so the second runs the package's tests on the merged tree. Neither
  waits; this change, being small, is expected to merge first.
- **#116, #118, #119, #120:** no shared file. Neither waits.
- **#117** bumps the OpenSpec CLI to 1.14.0; this change was validated with 1.13.2. Whichever merges second runs
  `task spec:check` on the other's tree in its CI.

## Owner decisions

1. Admission refuses an `unknown` identity where the runner respects one (D1, D3). Recommended: each side fails
   closed for its own action. The other answer, admitting under `unknown`, keeps the integration suite running on a
   host whose ps cannot read the runner's start time, and reopens #124's window there.
2. The definition lives in the runner's spec, which this change MODIFIES though no runner code changes (D1).
3. `Start` returns the context's error if its context has ended when the read fails (D2): behaviour #124 did not ask
   for, which comes with the contract's rule that a blocking call takes the caller's context.
4. The two runner findings under "Not in this change": finding 1 recorded as accepted, with the sticky `/tmp`
   measurement that turns the attempted quarantine into "busy"; finding 2 declared out of scope and tracked in #126.
   The owner confirms both or rules otherwise.
5. `task doctor` keeps its host-and-pid rule; only its `:107` comment is corrected (D1).
