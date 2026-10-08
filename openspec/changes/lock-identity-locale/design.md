# Design: lock-identity-locale

Status: revision 1, draft. It rests on `inventory.md` revision 2 (review round 1: `INVENTORY PASS`; its five
corrections are folded in). Issue #126, owner ruling #126 comment 6048776879, claim PR #127. Claims A to I, probes P10
to P17 and every pin are the inventory's unless a pin is given here.

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

## Files

- `scripts/test-integration.sh`: the `identity_utc` read beside `:101-102`; the line in `acquire_lock` after `:201`;
  `read_owner` (`:136-150`) reads it; `owner_is_stale` (`:165-174`) applies D2; the comments at `:5-6` and `:134`.
  Bash 3.2 (`:13`): the fixed read is a variable prefix on `ps` inside a command substitution, run under `/bin/bash`
  3.2.57 in P10.
- `scripts/admission-lock.sh:3-4`: the comment (D5).
- `internal/harness/natsfixture/admission.go`: `ownerLive` takes the record's `identity_utc`; the fixed read under
  `Start`'s context with `TZ=UTC` and `LC_ALL=C` added to the inherited environment (os/exec: "If Env contains
  duplicate environment keys, only the last value in the slice for each duplicate key is used"; P11); refusal texts
  per D2; the comments (D5). The rest of `admit` (token, context's error) is unchanged.
- `internal/harness/natsfixture/admission_test.go`, `internal/harness/runner/runner_test.go`: D6.
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
5. **Q1, SemStreams `main` (open; needs your word).** Not read. The claims that rest on it: SemStreams' runner reads a
   SemEngine record as an ordinary owner and ignores `identity_utc` (claim E, P15, I3's purpose), and therefore L1's
   bound and the ruling's premise that "SemStreams reads a SemEngine lock exactly as before". They are measured at the
   pin only. If SemStreams' runner at `main` refuses or quarantines a record with a key it does not know, this change
   would make it misjudge every live SemEngine lock. Recommended: authorize one read of
   `scripts/run-integration-tests.sh` at SemStreams `main` (one `gh api` call) before task 2.2 (task 1.4), or rule that
   the pin's measure stands.
6. **Q6, other users of the lock path (open; needs your word).** Whether semsource, semconnect, semteams or semboids
   takes `/tmp/semstreams-integration.lock` is not measured; `docs/inventory-scope.md` does not admit the question.
   Nothing at the pin points to one.
7. **L3, a wall-clock step on Linux.** Declared and left alone here. File an issue for it, or leave it declared.
