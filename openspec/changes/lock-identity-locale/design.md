# Design: lock-identity-locale

Status: revision 2, draft. It rests on `inventory.md` revision 2 (review round 1: `INVENTORY PASS`; its five
corrections are folded in). Issue #126, owner ruling #126 comment 6048776879, claim PR #127. Claims A to I, probes P10
to P17 and every pin are the inventory's unless a pin is given here. Revision 1 passed pre-owner review, the owner
accepted it (PR #127 comment 6049508662), and it was implemented at `33c5111` to `ecf2033`. Revision 2 answers Codex's
review of `ecf2033` (PR #127 comment 6059322575): D7 publishes the owner record whole (the BLOCKING finding), with
probes P18 to P21, L7, I5, and owner decision 8; owner decisions 5 and 6 carry the owner's answers, and "Conformance"
is added (the MEDIUM finding). Pins to `scripts/test-integration.sh` in D7 are at `ecf2033`.

## Context

The runner records its start time as `identity`, the text `ps -o lstart=` prints in its own environment, and a later
runner calls the owner stale when its own read of that pid prints another text (`scripts/test-integration.sh:101`,
`:172-173`). That text follows the reader's time zone and locale (P10, P17), so two runners on one host whose shells
differ quarantine each other's live lock (#126). SemStreams' runner at the pin writes and judges the same way and
ignores an owner-record key it does not know (claims D and E, P15). The owner ruled a SemEngine-only fix: keep
`identity` as written today, add one key read under `TZ=UTC LC_ALL=C`, and have SemEngine's two judges compare that key
when it is present and fall back to `identity` when it is absent.

## Options

1. **The ruling: one more key, compared when present** (recommended; D1 to D6). The runner writes `identity` as today
   and a seventh key, `identity_utc`. `owner_is_stale` and `ownerLive` compare `identity_utc` when the record has one,
   and `identity` otherwise. Costs: the record is no longer SemStreams' format byte for byte, so the texts that say it
   is change (D5); a SemEngine runner and a SemStreams runner whose shells differ keep #126 (accepted in the ruling);
   admission admits a direct `go test` from a shell with another time zone or locale, which it refuses today (D3).
   Admission still runs one `ps` per `Start`.
2. **Extend the existing key: write `identity` itself under the fixed environment.** The record stays six keys.
   Cost: SemStreams' runner reads `identity` in its own environment (claim D), so on a host whose shells are not UTC
   and C-like it would judge every live SemEngine owner changed and quarantine its lock (P15's Tokyo rows are that
   case). The fix becomes a joint change with SemStreams, which the owner ruled out.
3. **Extend the existing number: judge by `started`.** `started` is already free of the environment (`date +%s`,
   `:100`). Cost: it is not the kernel's start time, one second later in P16, so judging by it needs a native
   start-time read with a tolerance on each system (#125's option 2: `sysctl` on macOS, `/proc` on Linux, a direct
   `golang.org/x/sys` dependency), and the bash judge has no native read. SemStreams would not read it either.
4. **Option 1, but admission compares both keys.** Admission refuses unless `identity` and `identity_utc` both match,
   which keeps today's refusal of a direct `go test` from a shell with another time zone or locale. Cost: it departs
   from the ruling's "compare that key"; it keeps admission exposed to a system time-zone change during a run (Q2), the
   one way #126's mechanism can refuse a runner's own tests; and the refusal it keeps protects nothing (D3).
5. **Option 1, but admission has no fallback:** a record without `identity_utc` is refused. Cost: it departs from the
   ruling. Production would not notice (claim H: no runner at or after this change writes such a record, and no
   SemStreams record reaches admission), but the six-key records the tests plant, and a direct `go test` against a
   SemEngine runner older than this change, would be refused.
6. **Do nothing.** Cost: #126 stays. The owner ruled against parking it.

## Decisions

### D1. The record gains `identity_utc`, as its last line

- The runner writes SemStreams' six keys exactly as today, in SemStreams' order (`test-integration.sh:196-201`; pin
  `:214-219`), then one line `identity_utc=<text>`, and no other key. Every line ends with a newline: both bash parsers
  drop an unterminated last line (P15c).
- `<text>` is the runner's start time as `ps -o lstart= -p <pid>` prints it with `TZ=UTC` and `LC_ALL=C` set for that
  `ps` alone, trimmed as `identity` is (leading blanks removed, trailing blanks kept, `:101`), or `unknown` when that
  prints nothing. This text was the same under every reader environment tried (P10 to P14, P17); macOS keeps four
  trailing blanks and Linux none, each consistent with itself.
- Last, because SemStreams' six lines then stay byte-identical and in their order. P15 shows the pin's judge ignores
  the line wherever it sits; last keeps R4's oracle SemStreams' list with one key appended.
- The name `identity_utc` matches no reader's key test: bash `case` and Go map lookups are exact (`:141-148`; pin
  `:72-79`; `admission.go:67`), and doctor's `sed -n 's/^host=//p'` and `'s/^pid=//p'` (`doctor.sh:113-114`) and
  release's awk `$1 == "token"` (`:236`; pin `:130`) match other keys. The reviewer's P15 used this name. The owner may
  choose another (owner decision 2).
- The key is always written. "Absent" therefore means a record from a runner that does not know the key: SemStreams',
  or SemEngine's before this change.

### D2. One rule for both judges

A record's `identity_utc` is usable when its line is present and its value is not empty. The compared value is then
`identity_utc`, and the read is `ps -o lstart= -p <pid>` with `TZ=UTC` and `LC_ALL=C`. Otherwise the compared value
is `identity`, and the read is made in the reader's environment, as today. Both reads are trimmed alike.

| Record | Runner (`owner_is_stale`) | Admission (`ownerLive`) |
| --- | --- | --- |
| another host | respected | refused |
| pid not a number | respected | refused |
| pid `kill -0` cannot signal (runner); ps fails or prints nothing for it (admission) | stale | refused |
| compared value `unknown` | respected | refused, naming `unknown` |
| the read prints nothing | respected | refused |
| the read differs from the compared value | stale | refused, naming the compared value |
| the read equals the compared value | respected | admitted |

An absent and an empty `identity_utc` are one case on purpose. Neither parser tests for presence (inventory § 2, "How
each parser reads a key the record lacks"), and the runner never writes an empty value (D1), so an empty
`identity_utc` comes only from a hand-edited record; it gets today's rule. `identity_utc=unknown` is not empty: it is
compared, and `unknown` is never live, the same verdicts as `identity=unknown` (#125's owner decision 1). Each side
still fails closed for its own action (#125 design D1): the runner needs proof of death, admission proof of life.

### D3. Admission admits a test process in another time zone or locale

Under D2, a direct `go test` run from a shell whose time zone or locale differ from the runner's, holding a copy of a
live runner's token, is admitted. Today it is refused, because its read prints another text (inventory § 2, the last
table; #125 design `design.md:79-81`: "the safe direction"). Recommended: admit it. The refusal protects nothing:

- The same `go test`, with the same copied token, from a shell with the runner's time zone and locale is admitted
  today. `admit` checks only that the token is in the owner file and that the owner is live (`admission.go:29-76`);
  nothing asks whether the caller is the runner's descendant. The refusal follows the caller's shell settings, not
  whether the lock is held.
- The guarantee admission states is about a dead runner: "a stale shell export must not admit a direct `go test`, nor
  one whose runner died and left its pid to another process" (`admission.go:26-27`). The fixed read keeps it: a dead
  runner's pid prints nothing, or another start time, in every environment.
- It removes the one way #126's mechanism can refuse a runner's own tests: a change of the system time zone during a
  run (Q2). The fixed read sets `TZ`, and a set `TZ` overrides the system zone (P10, P12).

The other answer is option 4 (owner decision 1).

### D4. The fallback, with its reason stated per judge

The ruling's fallback is kept in both judges. Its stated reason, "a lock SemStreams wrote", holds for `owner_is_stale`:
the runner judges whatever owner holds the lock (`:208-209`), so every SemStreams record and every record of a
SemEngine runner older than this change reaches it (P15, P15b). It does not hold for `ownerLive`: admission compares
only after finding its own token in the record (`admission.go:53-55`, `:63-67`), and only SemEngine's runner exports
that token (`test-integration.sh:331`; claim H). In `ownerLive` the fallback serves a SemEngine runner older than this
change, met by a direct `go test` from a newer worktree with a copied token, and the six-key records tests plant. The
spec delta and the code comment say so; neither names SemStreams as the writer admission meets.

### D5. Texts that become untrue, and their new wording

| Text | Today | After |
| --- | --- | --- |
| `integration-test-runner` › "Shared host lock" | "exactly the keys host, pid, started, identity, token, command" | SemStreams' six keys in that order, then `identity_utc`; its definition; liveness by D2; the remaining limit (spec delta) |
| `nats-fixture` › "Admission before Docker" | "live as `integration-test-runner` › "Shared host lock" defines it, read in the test process's environment" | live as that requirement defines it, by `identity_utc` when usable, otherwise by `identity` (spec delta) |
| `docs/admission-ledger.yaml:203` (`contract`) | `byte-compatible lock protocol at /tmp/semstreams-integration.lock;` | `lock protocol at /tmp/semstreams-integration.lock that SemStreams' runner reads as an ordinary owner: its six owner keys unchanged, plus identity_utc, which it ignores;` |
| `docs/admission-ledger.yaml:207-209` (`known_risks`) | three risks | adds: `a SemEngine runner and a SemStreams runner whose shells differ in time zone or locale still read each other's live owner as changed (#126, accepted)` |
| `docs/admission-ledger.yaml:199` (`consumer_purpose`) | "owner keys" | unchanged: SemStreams' owner keys are still what the row carries |
| `scripts/admission-lock.sh:3-4` | "adopted byte-compatibly (owner ruling Q1, 2026-09-30) so either repository's runner sees the other as an ordinary owner" | adopted (Q1) so either runner sees the other as an ordinary owner; SemEngine's record adds `identity_utc`, which SemStreams' runner ignores (#126) |
| `scripts/test-integration.sh:5-6` | "the host lock is SemStreams' own, byte-compatible" | SemStreams' own; the owner record is SemStreams' plus one key SemStreams ignores |
| `scripts/test-integration.sh:134` | `lock (byte-compatible with SemStreams run-integration-tests.sh:78-242)` | the same protocol, plus `identity_utc` |
| R4: `runner_test.go:21-25`, `:519-521` | the six keys, "or neither can judge the other's lock" | SemStreams' six keys stay the independent oracle; R4 requires them in that order followed by `identity_utc`, and requires `identity_utc` to be neither empty nor `unknown`: stricter than the scenario "Owner record format", which requires only that it is not empty, and a premise of the hosts R4 runs on, where ps prints the runner's start time. R4 reads `go.owner` through `strings.TrimSpace` (`:515`), which strips the last line's trailing blanks, so it checks no value of `identity_utc` byte for byte |
| `admission.go:25-28`, `:78-83`, `:116-118` | the rule by `identity` only | D2's rule and D4's reason |
| `admission_test.go:49-51` (`runnerIdentity`) | one oracle | a second oracle for the fixed read (D6) |

Not changed: `scripts/doctor.sh:107-108` (still true: doctor judges by host and pid only) and the archived SETUP 02
design (history). The ledger and comment wording above is a proposal (owner decision 4).

### D6. The tests: failing first, then shown able to fail

Time zones are set with POSIX strings, `EST5` and `JST-9`, on both sides of each comparison. A zone name has no
effect on a host without zone files (P14, P17), and setting both sides keeps a test from depending on the host's own
zone, including a host already at UTC+9. Locales are varied too, with `LC_ALL`; that half can fail only on macOS: no
Linux image measured has a locale other than C (Q3), and there bash warns `setlocale: LC_ALL: cannot change locale`
on stderr and runs in C (measured in `golang:1.26.6-bookworm`). No test sleeps, skips, or binds an address.

Runner tests, `internal/harness/runner/runner_test.go`:

- **T1, `TestR1LiveOwnerInAnotherTimeZoneOrLocaleIsRespected`** (new, #126 end to end). Runner A, with `TZ=EST5`,
  `LC_ALL=en_GB.UTF-8` and `FAKE_GO_MODE=hang`, takes the lock and parks, as R3 does (`:416-445`). The test confirms
  A is still running (`docs/testing.md:467-471`), then runs runner B with `TZ=JST-9`, `LC_ALL=ja_JP.UTF-8` and no
  wait budget. B runs from its own harness (its own fake and evidence directories, the default `FAKE_GO_MODE`) with
  `SEMENGINE_DOCKER_ADMISSION_LOCK_DIR` set to A's lock directory, under a context with a deadline, so a B that
  acquires finishes instead of parking. Required: B exits 1 naming A's pid, the owner record still holds A's token,
  and A is still running; A is then sent TERM and exits 143. On the base code B quarantines A's live lock, prints
  `cleaned stale lock` and exits 0, so the test fails first for the reason #126 names. It plants no record and does
  not depend on the key's name. Both locales are non-C, so each half of the fixed read is held on macOS: a writer
  that drops `LC_ALL=C` records British English text that B's read does not print, and a judge that drops it reads Japanese
  text that A's record does not hold (R-M1 and R-M2 below).
- **R2 gains "changed identity_utc"**: a live pid (`os.Getpid()`), its real `identity` (the existing oracle), and
  `identity_utc=Mon Jan  1 00:00:00 1990` (R2's value, `:388`). Required: quarantined. On the base code the runner
  compares `identity`, which matches, and respects the lock: it fails first. It shows the runner compares
  `identity_utc` when it is usable.
- **R1 gains "identity_utc unknown"**: a live pid, its real `identity`, `identity_utc=unknown`. Required: exit 1, lock
  intact. It passes on the base code; it holds D2's `unknown` row against a stale verdict.
- R1 and R2 keep their six-key records (`writeOwner`, `:233`): they hold the fallback for a SemStreams record. R4 as
  D5.

Admission tests, `internal/harness/natsfixture/admission_test.go`, through `admit` or through `Start` with `noDocker`,
as today:

- `plantLock` writes seven keys: `identity` from `runnerIdentity`, and `identity_utc` from a second oracle, the
  runner's fixed command text run by bash, sharing no code with admission's read. The tests that plant a live owner
  and go on (`TestAdmissionAdmitsALiveOwner`, `TestStopHonoursItsContextWhileStartHoldsTheFixture`,
  `TestCancelledStartIsRecognisable`) use that record.
- **`TestAdmissionAdmitsALiveOwner` gains "an owner read in another time zone"**: `identity` read by the oracle under
  `TZ=EST5`, `identity_utc` by the fixed oracle; the test sets `TZ=JST-9` and `LC_ALL=de_DE.UTF-8` before `admit`.
  Required: admitted. On the base code admission reads under `JST-9`, the text differs, and it refuses: it fails first.
- **`TestAdmissionRequiresALiveOwner`** keeps its own six-key record (`admission_test.go:172-173`), so its five rows
  hold the fallback path, each differing from a live six-key owner in one field; its comment says so. It gains two
  cases with seven-key records, each differing from a live seven-key owner in `identity_utc` alone: "identity_utc
  names another start time" (live `identity`, `identity_utc` the 1990 value; the refusal names it) and "identity_utc
  unknown" (the refusal names `unknown`). On the base code both are admitted, since `identity` matches: both fail
  first.
- **`TestAdmissionJudgesARecordWithoutIdentityUTCByIdentity`** (new): a six-key record with the live `identity` is
  admitted, and a record with `identity_utc=` (empty) and the live `identity` is admitted. Both pass on the base code;
  they hold D2's fallback and D4. A six-key record with another identity is refused by the table's row "owner pid
  names a process with another start time".

Shown able to fail, with `task mutate:check -- -pkg ./internal/harness/natsfixture -file
internal/harness/natsfixture/admission.go`, each wrong change in a copy outside the repository:

| Wrong change | Test that must fail | Expected |
| --- | --- | --- |
| M1: the fixed read sets neither `TZ` nor `LC_ALL` | "an owner read in another time zone" | detection |
| M2: `identity` is compared even when `identity_utc` is usable | "identity_utc names another start time" | detection |
| M3: no fallback: a record without `identity_utc` is refused | the six-key live case of `TestAdmissionJudgesARecordWithoutIdentityUTCByIdentity` | detection |
| M4: `LC_ALL=C` is dropped and `TZ=UTC` kept | "an owner read in another time zone" | detection on macOS (the read prints German text); on Linux a survivor, equivalent there (no `de_DE` locale, Q3) |
| M5: an empty `identity_utc` is compared | the empty case of `TestAdmissionJudgesARecordWithoutIdentityUTCByIdentity` | detection |

`task mutate:check` refuses a script, so the runner's two wrong changes for the locale half are made in the tree with
the `cp` and checksum procedure of the reviewer contract, § Required review workflow, item 8:

| Wrong change to `scripts/test-integration.sh` | Test that must fail | Expected |
| --- | --- | --- |
| R-M1: the judge's fixed read without `LC_ALL=C` (`TZ=UTC` kept) | T1 | detection on macOS; on Linux a survivor, equivalent there (no `ja_JP` locale, Q3) |
| R-M2: the writer's fixed read without `LC_ALL=C` (`TZ=UTC` kept) | T1 | detection on macOS; on Linux a survivor, equivalent there (no `en_GB` locale, Q3) |

The rest of the runner's evidence is T1, R2's new case and R4 failing on the base code (task 2.1), and R2's six-key
"changed identity" case, which fails a runner that drops the fallback.

**Generated checks: examples suffice.** The rule picks one of two compared values, then tests string equality. What
can go wrong is the read's environment: a finite set, covered by the cases above and measured by P10 to P17. No
generated string reaches it.

### D7. The owner record is published whole

**The defect** (Codex, PR #127 comment 6059322575; P18). `acquire_lock` creates the lock directory and then writes the
record into `$lock_dir/owner` one `printf` at a time (`scripts/test-integration.sh:207-216`). A runner that reads it
after the `identity` line and before the `identity_utc` line finds six keys, takes D2's fallback and, when its shell's
time zone or locale differs, quarantines the live owner, which then runs without the lock. D2 is right for a finished
record; the fault is that a writer shows an unfinished one. T1 cannot see it: B starts after `go.ready`, after
publication.

**Options for publication** (D1 to D6 stand):

- **(a) Write the record to a file beside the lock directory after `mkdir` succeeds, then rename it to `owner`.**
  Between `mkdir` and the rename it runs `mktemp`, the writes, two command substitutions, `chmod` and `mv`: about 5 ms
  on macOS (L7). A SIGKILL in that window leaves an ownerless lock directory, which every SemEngine and SemStreams
  runner then refuses or waits on until a person removes it. It writes nothing while a runner waits, and keeps
  publication in one place.
- **(b) The same, with the file written before `mkdir`** (before the wait loop), and only the rename after it. Its
  window after `mkdir` is one `mv`. Its SIGKILL leftover is an inert file beside the lock that no reader in either
  repository reads; it is left by a SIGKILL at any time from its creation to the rename, a whole wait of up to 3600 s
  included, and the EXIT trap removes it on every other exit. One `BASH_ENV` hook can still pause both codes inside
  publication, with two triggers: `printf` on `command=%s\n` while `[ -d "$lock_dir" ]` (`ecf2033`'s in-place write),
  and an `mv` whose last argument ends in `/owner` (the rename under (b), which `ecf2033` never calls).
- **(c) The file inside the lock directory.** A runner killed before the rename leaves a lock directory `rmdir` cannot
  remove; both runners' quarantine removes only `owner` before `rmdir` (`:192-195`; pin `:112-115`).
- **(d) `identity_utc` written first,** so a partial record that holds `identity` holds it too. It contradicts the
  accepted order (owner decision 2, D1) and Codex's "preserve the approved field order".
- **(e) Readers tolerate an unfinished record:** a `command=semengine` record without `identity_utc` read as
  unfinished, or a re-read after a pause. The first ends the six-key fallback for SemEngine runners older than this
  change (L1); the second is a timing guess.
- **(f) Publish a prepared directory by renaming it to the lock path.** POSIX `rename()` replaces an existing empty
  directory, and `mv` onto an existing directory moves into it, so this breaks the `mkdir` exclusion SemStreams shares.

Options (c) to (f) each break an accepted decision or the shared `mkdir` exclusion. Between (a) and (b), recommended:
(a). What (a) risks is loud and safe: later runners refuse with "host lock is busy", `task doctor` names the state, and
no runner reaches Docker without the lock. It needs a SIGKILL within about 5 ms of `mkdir`: the runner traps INT and
TERM, and an interrupt during publication ends without wedging the lock (below). (b) narrows that window to one `mv`,
and in exchange leaves a file nobody notices after any SIGKILL during a wait of up to an hour, and splits preparation,
publication and cleanup across the script. Both windows are small against any plausible SIGKILL; (a) is the simpler
script. If the owner chooses (b), the rule's first three bullets move before the wait loop, the EXIT trap removes the
file, T2's hook gains the `mv` trigger, and T3 and R-M5 fail the rename with a fake `mv`, since `mktemp` then fails
before `mkdir`, and the runner spec's sentence that the record is written after the lock directory is created says
before it instead.

**The rule (option a).**

- After `mkdir "$lock_dir"` succeeds, the runner creates a file with `mktemp "${lock_dir}.owner.XXXXXX"`: beside the
  lock directory, in its parent directory and so on its filesystem (P21); unpredictable in a world-writable `/tmp`; and
  not under `${lock_dir}.stale.`, the quarantine name of both runners (`:190`; pin `:110`). Like that name, it assumes
  a lock path without a trailing slash (the default, `admission-lock.sh:7`).
- It writes the record there as D1 states: the same seven lines, in the same order, with the same `printf` calls. The
  record's bytes do not change, so SemStreams' six lines stay byte-identical (the Q1 ruling).
- It sets the file's mode to `0666` less the runner's umask, the mode the redirection gives today. `mktemp` creates
  `0600` (P21), which another user's `task doctor` or runner could not read.
- It renames the file to `$lock_dir/owner` with `mv`. Both names are on one filesystem, where POSIX `mv` acts as
  `rename()`, and `rename()` makes the new name refer to the whole file at once: a reader finds no `owner`, or the
  complete record.
- Only then is `lock_held=true`, and `cp "$lock_dir/owner" "$evidence_dir/lock-owner"` (`:218`) still copies the
  published record (P19: the evidence holds the seven keys).
- If `mktemp`, the write, `chmod` or `mv` fails, the runner removes the file if it exists and the lock directory,
  empty since nothing else is written in it, prints that it could not publish the owner record, and `acquire_lock`
  fails, so the runner exits 1 before any Docker call (`:339`).
- A SIGTERM sent to the runner alone only records the signal (`on_signal`, `:297-308`): publication completes,
  `exit_for_signal` (`:340`) exits, and `finish` releases the lock (`:324-331`). A terminal's Ctrl-C reaches the whole
  foreground process group, so it can kill `mktemp`, `chmod` or `mv`, and the failure path above runs (the reviewer
  measured a script that traps INT seeing its external child exit 130 and continuing). Killed before the rename, the
  file and the lock directory are removed and the runner exits 130. A `mktemp` killed after creating its file leaves
  that file, whose name the failure path never learned (inert, L7). An `mv` killed after the rename leaves the
  published record, the failure path's `rmdir` fails on it, and the runner exits 130: the record names a pid about to
  be dead, which the next runner quarantines by that dead pid. No outcome shows part of a record or runs Docker
  without the lock; only a SIGKILL inside the window wedges it (L7).

What each reader sees between `mkdir` and the rename (the lock directory exists, with no `owner`):

| Reader | Sees | Verdict | Pin |
| --- | --- | --- | --- |
| SemEngine runner | `read_owner` returns early; every observed value `unknown`, `identity_utc` empty | respected: `unknown` is not this host (`:176`); busy or waits (P19, P20) | `:143` |
| SemStreams' runner at the pin, which is its `main` (owner decision 5) | `read_owner` returns early; `observed_host="unknown"` | respected (pin `:98`; P20) | pin `:68-70` |
| `task doctor` | the path exists, with no `owner` | "exists without an owner file (being written, or foreign)" | `doctor.sh:120-121` |
| Admission | no owner file | refused, "no live lock owner file"; a runner's own tests start only after `acquire_lock` returns (`:339`) | `admission.go:44-47` |

This window is not new: at `ecf2033` it lies between `mkdir` and the redirection's open. What D7 removes is the record
seen in part. Nothing after the rename changes: `read_owner`, `owner_is_stale`, `clean_stale_lock` and `release_lock`
read and remove `owner` as now, and the lock directory holds only `owner`, so both runners' quarantine still empties it.

Measured for D7 (2026-10-08; the prototype is a copy of `ecf2033`'s runner with option a applied, local only, under
`/tmp/claude-126/proto/`, driven by Codex's probe with the runner's root as its argument):

| Probe | Input | Result |
| --- | --- | --- |
| P18 | Codex's probe on an unchanged copy of `ecf2033`'s runner: A `TZ=EST5`, B `TZ=JST-9`, both `LC_ALL=C`; A paused by a `BASH_ENV` hook after its `command=` line | macOS and Debian (`golang:1.26.6-bookworm`): A's lock directory holds `owner` (six lines); B exits 0 after `cleaned stale lock ... pid=<A>` and enters the fake Docker; A, resumed, prints `lock ownership changed`, and its `lock-owner` evidence is missing |
| P19 | the same probe on the prototype | macOS and Debian: while A is paused its lock directory is empty and `lock.owner.<random>` lies beside it; B exits 1, `host lock is busy and no wait budget was requested`, and never enters the fake Docker; A, resumed, exits 0 without `lock ownership changed`, its `lock-owner` evidence holds the seven keys, and no `lock*` path remains |
| P20 | the prototype's A killed with SIGKILL while paused | left: `lock` (empty) and `lock.owner.<random>` (mode `0600`, six lines). Runner B: exit 1, busy. SemStreams' pin judge and SemEngine's judge (P15, P15b) on that directory: respected, `host=unknown pid=unknown` |
| P21 | `mktemp "<dir>/lock.owner.XXXXXX"` on macOS and in Debian (GNU coreutils 9.1); a redirection under umask 022; `printf '%o' $((0666 & ~$(umask)))` under `/bin/bash` 3.2.57; `stat -f %d` of the parent and the file | `mktemp` creates mode `0600` on both; the redirection `0644`; the expression prints `644` under umask 022 and `600` under 077; parent and file on one device |

Tests for D7, in `internal/harness/runner/runner_test.go`; no sleep, skip or address:

- **T2, `TestR1ContenderDuringPublicationRespectsTheOwner`** (new; Codex's probe as a test). Runner A, `TZ=EST5
  LC_ALL=C`, starts with `BASH_ENV` naming a hook the test writes: a `printf` function that calls `builtin printf` and,
  on the first call whose format is `command=%s\n`, writes a ready file into A's fake directory and blocks reading a
  FIFO the test made (`syscall.Mkfifo`) and holds open for reading and writing, so neither side blocks on opening it.
  That call is inside publication in both codes: at `ecf2033` `owner` then holds six lines, under D7 the lock directory
  is empty. The test waits for the ready file with `startRunner` (`probe.Await`), confirms A is still running
  (`docs/testing.md:467-471`), and checks its premise: ps prints a start time for A's pid under `TZ=EST5 LC_ALL=C` and
  under `TZ=JST-9 LC_ALL=C`, and the two differ; a host where they do not fails the test, as Codex's sandboxed run,
  with `ps` denied, would otherwise pass. Runner B then runs from its own harness on A's lock directory, `TZ=JST-9
  LC_ALL=C`, no wait budget, under a context with a deadline. Required: B exits 1 naming the busy lock, never prints
  `cleaned stale lock`, and its fake `docker` records no call; A is still running. The test then writes to the FIFO.
  Required: A exits 0, its stderr lacks `lock ownership changed`, its `lock-owner` evidence holds the seven keys with
  A's token, and `filepath.Glob(a.lock + "*")` is empty. Every path out resumes A and waits for it with a bound. On
  `ecf2033` B exits 0, prints `cleaned stale lock` and enters Docker (P18, macOS and Linux): T2 fails first.
- **T3, `TestR1OwnerRecordThatCannotBePublishedReleasesTheLock`** (new). A fake `mktemp`, first in `PATH` from the
  harness's fake directory as `docker` and `go` are, fails for the `.owner.` template. Required: the runner exits
  non-zero naming the failure, its fake `docker` records no call, and `filepath.Glob(h.lock + "*")` is empty. On
  `ecf2033`, which calls no `mktemp`, the run exits 0: T3 fails first.
- **T1 gains a mode check** while A holds the lock (beside `runner_test.go:424`): the `owner` file's mode equals that
  of a file the test creates with `os.Create` in its temporary directory, which gets `0666` less the umask the runner
  inherits. It passes on `ecf2033`, whose redirection gives that mode; it holds D7's `chmod`.

Generated checks are not used for D7. Its history has one boundary: a reader that reads after `mkdir` and before
the rename. With D7, every such reader finds no owner file and respects the lock, so one held point is enough.
T2 holds the writer at the latest point of the old in-place write, where the most of the record is visible and
the old code misjudges it. An earlier point shows the old code less of the record, and the new code nothing.

Shown able to fail: `task mutate:check` refuses a script, so each wrong change is made in the tree with the `cp` and
checksum procedure of the reviewer contract, § Required review workflow, item 8:

| Wrong change to `scripts/test-integration.sh` | Test that must fail | Expected |
| --- | --- | --- |
| R-M3: the record written in place, line by line, into `$lock_dir/owner` (the `ecf2033` shape) | T2 | detection on macOS and on Linux |
| R-M4: the `chmod` left out | T1's mode check | detection where the umask is not `077`; under `077`, equivalent (`0600` either way) |
| R-M5: on a failed publication, the lock directory left in place | T3 | detection |

## Invariants and their spec homes

- **I1** (from #125, unchanged): `Start` makes a Docker call only when the owner record holds the test's token and the
  owner is live. Home: `nats-fixture` › "Admission before Docker" (MODIFIED).
- **I2:** a runner does not quarantine a live owner whose `identity_utc` is usable, whatever the time zone or locale
  of either runner's shell. Declared exceptions: a pid reused within the second its owner started; a pid `kill -0`
  cannot signal (#125's runner finding 1); a wall-clock step on Linux (L3). Home: "Shared host lock" (MODIFIED), its
  scenario "Owner in another time zone or locale is respected".
- **I3:** a SemEngine owner record holds SemStreams' six keys in SemStreams' order and then `identity_utc`, and nothing
  else. Home: "Shared host lock", its scenario "Owner record format".
- **I4:** `identity_utc` is the same text for every reader environment. Home: "Shared host lock", which defines the
  read with `TZ=UTC` and `LC_ALL=C` set. This is a property of `ps` under those settings: P10 to P14 and P17 measure it,
  T1 exercises it for the time zone everywhere and for the locale on macOS, as the admission case does.
- **I5:** a reader of the lock directory finds no owner record, or the complete record its writer published; never part
  of one. Home: "Shared host lock", its scenario "A contender during publication respects the owner".

## Declared limits

- **L1 (accepted in the ruling).** A SemEngine runner and a SemStreams runner whose shells differ in time zone or
  locale still read each other's live owner as changed, in both directions. So do two SemEngine runners when one is
  older than this change: it ignores `identity_utc` (P15b) and its own record has none. Stated in "Shared host lock".
- **L2 (Q2).** A change of the system time zone between a write and a read: closed for every record with a usable
  `identity_utc`, since the fixed read sets `TZ`. On the fallback path it is unchanged. Not measured (it needs root).
- **L3 (Q4).** A wall-clock step on Linux moves `lstart`, in both keys alike (#125 design D3), so a runner can judge a
  live owner changed after one. Unchanged and not measured. No issue tracks it (owner decision 7).
- **L4 (Q3).** On Linux, `LC_ALL=C` overriding a non-C locale is measured nowhere: no image has one. It is measured on
  macOS (P10, P11). The C locale is present on every system measured (P13: `C`, `C.utf8`, `POSIX`).
- **L5 (Q5).** If the fixed read prints nothing for the runner's own pid, `identity_utc` is `unknown`: the runner then
  respects the owner and admission refuses it, naming `unknown`. Not reproduced on either system.
- **L6.** One-second resolution, as before: a pid reused within the second its owner started reads as that owner.
- **L7 (D7).** A runner killed with SIGKILL between `mkdir` and the rename leaves the lock directory with no `owner`,
  and `${lock_dir}.owner.<random>` beside it (mode `0600`, the record's first lines; P20). Every reader respects that
  directory, so later runners refuse or wait until a person removes it (`rmdir`, and the stray file with `rm`); `task
  doctor` reports it. The class is not new: at `ecf2033` a SIGKILL between `mkdir` and the `pid=` line leaves a
  directory no runner quarantines either, since its pid reads `unknown` (`:177`). The window grows from one `printf`
  to three external commands (`mktemp`, `chmod`, `mv`): about 5 ms on macOS (the reviewer's measurement, 200
  iterations under `/bin/bash` 3.2.57: 9.4 ms against 4.4 ms for `mkdir`, publication, `rm` and `rmdir`). A Ctrl-C
  that kills `mktemp` after it creates its file also leaves `${lock_dir}.owner.<random>` beside the lock, but no lock
  directory: an inert file that no reader in either repository reads.

## Files

- `scripts/test-integration.sh`: the `identity_utc` read beside `:101-102`; the line in `acquire_lock` after `:201`;
  `read_owner` (`:136-150`) reads it; `owner_is_stale` (`:165-174`) applies D2; the comments at `:5-6` and `:134`.
  Bash 3.2 (`:13`): the fixed read is a variable prefix on `ps` inside a command substitution, run under `/bin/bash`
  3.2.57 in P10.
- `scripts/test-integration.sh`, `acquire_lock` (`:204-221` at `ecf2033`): publication as D7 states.
- `scripts/admission-lock.sh:3-4`: the comment (D5).
- `internal/harness/natsfixture/admission.go`: `ownerLive` takes the record's `identity_utc`; the fixed read under
  `Start`'s context with `TZ=UTC` and `LC_ALL=C` added to the inherited environment (os/exec: "If Env contains
  duplicate environment keys, only the last value in the slice for each duplicate key is used"; P11); refusal texts
  per D2; the comments (D5). The rest of `admit` (token, context's error) is unchanged.
- `internal/harness/natsfixture/admission_test.go`, `internal/harness/runner/runner_test.go`: D6; T2, T3 and T1's
  mode check (D7), with the hook and the fake `mktemp` written by the tests, as `fakeDocker` and `fakeGo` are.
- `docs/admission-ledger.yaml:203`, `:207-209`: D5.
- Not changed: `scripts/doctor.sh`, `internal/harness/natsfixture/fixture.go`, `internal/harness/prochost/`,
  `AGENTS.md` (no new rule), anything in SemStreams.
- By the archive: the two specs and `docs/repository-map.md` (task 4.1).

## Not in this change

- Anything in SemStreams; nothing is filed there (the ruling).
- `task doctor`'s rule: it reads no start time and is still described truly (`doctor.sh:107-108`).
- #125's runner finding 1 (`kill -0` calls another user's process dead).
- A native start-time read (option 3), and moving the test oracles into shared code.
- The runner's `now_ms` (`scripts/test-integration.sh:84-88`) splits bash 5's `EPOCHREALTIME` on a period, but bash
  writes it with the locale's decimal separator, a comma under `de_DE.UTF-8`, so it silently misreads the time:
  #128. T1 gives runner A `en_GB.UTF-8`, whose separator is a period, so T1 neither depends on nor shows that defect.
- `describe_owner` reports a lock directory with no `owner` as `host=unknown pid=unknown elapsed=<seconds since the
  epoch>s`, since `observed_started` is `0` (`:140`, `:157-164`; P19 printed `elapsed=1791460973s`). It predates this
  change; T2 sees it and does not depend on it.
- `clean_stale_lock` moves the lock path after judging an earlier read (`:190-191`; pin `:110-111`), so a runner that
  judged an older, stale owner can move a newer lock directory, one in its publication window included; D7's `rmdir`
  and `mv -f` then act on another runner's directory. The race exists at `ecf2033`, whose redirection writes into
  whatever directory the path names, and in SemStreams; D7 does not widen it, and its failure path does not cover it.

## Overlaps and order

- **#93** (draft, head `0ee1921`) changes three other files of `internal/harness/natsfixture` and ADDs a different
  `nats-fixture` requirement; this change MODIFIES "Admission before Docker". No file is shared, and either archive
  applies after the other. Both archives will likely amend `docs/repository-map.md` (`:55`, and a row after `:70`); the
  second to merge resolves it when it merges `origin/main`, and runs the package's tests on the merged tree. Neither
  waits; this change, being small, is expected to merge first.
- **#117** bumps the OpenSpec CLI to 1.14.0; these artifacts were validated with 1.13.2. Whichever merges second runs
  `task spec:check` on the other's tree in its CI.
- **#116, #118, #119, #120:** no shared file.

## Owner decisions

The owner answered decisions 1 to 7 on 2026-10-07 (PR #127 comment 6049508662): 1 to 4 as recommended, 5 and 6 as
below, and 7 left declared, with no issue filed. Decision 8 is new in revision 2.

1. **Admission admits a direct `go test` from a shell with another time zone or locale** that holds a live runner's
   token (D3). Recommended: admit, as the ruling's rule does. The other answer, option 4, keeps a refusal that protects
   nothing and keeps the Q2 exposure.
2. **The key: `identity_utc`, the record's last line, always written, `unknown` when the fixed read prints nothing**
   (D1, L5). Another name costs nothing now and is a protocol change later.
3. **An absent or empty `identity_utc` falls back to `identity` in both judges (D2), and the reason is stated per judge
   (D4):** SemStreams' records reach the runner's judge only; admission's fallback serves older SemEngine runners and
   tests. This keeps the ruling and corrects only its parenthetical.
4. **The new wording of the ledger row, the two script comments and R4 (D5).** The spec sentence changes as the ruling
   accepted.
5. **Q1, SemStreams `main`: settled.** The owner authorized one read. SemStreams `main` is the pin:
   `gh api repos/C360Studio/semstreams/commits/main` returned `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, and its
   `scripts/run-integration-tests.sh` is the file P15 ran (sha256 `9a4bcf19…58d9`), whose owner parser ignores a key it
   does not know. Claim E, P15, L1's bound and the ruling's premise that "SemStreams reads a SemEngine lock exactly as
   before" therefore hold for `main` as of 2026-10-07; a later push to SemStreams `main` reopens them.
6. **Q6, other users of the lock path: settled.** The owner authorized one code search: `gh search code --owner
   C360Studio` for `semstreams-integration.lock`, `SEMSTREAMS_INTEGRATION_LOCK_DIR` and `run-integration-tests.sh`
   found hits only in `semengine` and `semstreams`. Limit: code search reads each repository's default branch, and
   only repositories it has indexed.
7. **L3, a wall-clock step on Linux.** Declared and left alone here. File an issue for it, or leave it declared.
8. **D7: publish the owner record whole, by a file beside the lock directory renamed to `owner`. Choose (a) or (b),
   and accept its leftover.** Recommended: (a), for the reasons in D7.
   - (a), the file written after `mkdir`: about 5 ms after `mkdir` in which a SIGKILL leaves an ownerless lock
     directory that blocks both repositories' runners until removed by hand (L7).
   - (b), the file written before `mkdir`: one `mv` after `mkdir`; a SIGKILL from the file's creation to the rename,
     a wait included, leaves an inert file no reader reads.

   Either way a failed publication fails before Docker (T3), and options (c) to (f) are not offered.

## Conformance

Each accepted decision, and D7, mapped to the `file:line` that carries it out at `5755327` (task 2.10's commit)
and to the test or record that shows it. Comments are on PR #127.

| Decision | Carried out at | Shown by |
| --- | --- | --- |
| 1. Admit a test process in another time zone or locale (D3) | `internal/harness/natsfixture/admission.go:68`, `:92-128` (`ownerLive` judges a record with `identity_utc` by a read under `TZ=UTC LC_ALL=C`) | `TestAdmissionAdmitsALiveOwner/an owner read in another time zone` (`admission_test.go:274`); M1 and M4 (PR #127 comment 6049934316) |
| 2. `identity_utc`, last line, always written, `unknown` when empty (D1, L5) | `scripts/test-integration.sh:104-105` (the fixed read, `unknown` when empty); `:220` (written last, in `publish_owner`) | R4 (`runner_test.go:781`, keys at `:628`); the real runner's seven-key record (comments 6050018875 and 6060678797) |
| 3. Absent or empty `identity_utc` falls back to `identity`; reason per judge (D2, D4) | runner: `scripts/test-integration.sh:142`, `:152`, `:175-187`; admission: `admission.go:79-128` | `TestAdmissionJudgesARecordWithoutIdentityUTCByIdentity` (`admission_test.go:290`); the six-key rows of `TestAdmissionRequiresALiveOwner` (`:179`); R2's "changed identity_utc" (`runner_test.go:671`); M2, M3 and M5 (comment 6049934316) |
| 4. Wording of the ledger row, script comments and R4 (D5) | `scripts/test-integration.sh:5-7`, `:137`; `scripts/admission-lock.sh:3-6`; `docs/admission-ledger.yaml:202-212`; `runner_test.go:21-25` | `task ledger:check`; the early check (comment 6050069499) and Codex's review (comment 6059322575) found each text as D5 states it |
| 5. Q1 settled: SemStreams `main` is the pin | PR #127 comment 6049508662 | claim E, P15 |
| 6. Q6 settled: no other repository uses the lock path | PR #127 comment 6049508662 | the code search recorded there |
| 7. L3 left declared | `design.md`, L3 | no issue filed |
| 8. Publication whole (D7, L7) | `scripts/test-integration.sh:203-228` (`publish_owner`), `:234` (`acquire_lock` publishes, or fails before Docker) | T2 (`runner_test.go:509`), T3 (`:650`), T1's mode check (`:445`); R-M3, R-M4 and R-M5 (comment 6060487135) |
