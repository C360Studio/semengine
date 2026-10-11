# Inventory: lock-identity-locale

- base: `11e4d7f92eff723a9dc3b92a832995d5f7675c01`, the empty claim commit on `claude/lock-identity-locale` (draft
  PR #127) over `origin/main` `b34ab3a8ad4fa6d5c644b08370252899e53a95ef` (#125's squash merge), so every pin below
  holds on both.
- revision 2, measured 2026-10-07; issue #126; owner ruling, #126 comment 6048776879; claim PR #127. Revision 1
  (sha256 `0243de4900d5d33c928ce51dffef1bdb379dcde4037fee743d3e90a979424754`) passed inventory review round 1
  (`INVENTORY PASS`, with three MEDIUM corrections and two NITs; the record is to be posted on PR #127). Revision 2
  folds them in: P14's conclusion is narrowed to zone names and P17 adds POSIX `TZ` strings; § 2 states how each
  parser reads an absent key; § 3 adds the child-process liveness rule; #93's head is `0ee1921`; Q6 bounds the
  lock's reader set.
- repositories read: this one, and SemStreams at its pin `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` for two
  questions: does its runner ignore an owner-record key it does not know, and does anything else at the pin read the
  owner record. Code facts come from the pin snapshot, read with
  `gh api 'repos/C360Studio/semstreams/contents/<path>?ref=<sha>'` and one tarball of the pin
  (`gh api repos/C360Studio/semstreams/tarball/<sha>`, extracted under `/tmp/claude-126/pin-src`).
  `docs/inventory-scope.md` rule 4 says a sister checkout is never the target of `git` commands, so no `git -C` was
  run against `/Users/coby/Code/c360/semstreams`. SemStreams `main` was not read (Q1). Whether another repository in
  that table (semsource, semconnect, semteams, semboids) takes `/tmp/semstreams-integration.lock` is not a question
  the table admits, so it is not measured (Q6); the pin's two docs that name the path name only SemStreams' runner.
- reused: the archived `admission-owner-identity` inventory and design (#124, PR #125), cited as "archive inventory"
  and "archive design" (`openspec/changes/archive/2026-10-07-admission-owner-identity/`). Its probes P1 to P9 are not
  repeated; every pin of its that this inventory relies on was re-read at the base.
- probes: P10 to P17 below, run under `/tmp/claude-126` and in throwaway containers (`docker run --rm --network none`);
  nothing in the worktree or in the SemStreams checkout was written. Pinned texts are trimmed of leading whitespace.
  A text whose trailing blanks matter is shown in brackets, as the probe helpers print it.

Question: where is the lock owner's start time written, read and compared; which of those comparisons depend on the
reader's time zone and locale; would one more key, read under `TZ=UTC LC_ALL=C`, be ignored by every reader that does
not know it and read the same by every reader that does; and what else claims the owner record, its specs and its
readers?

## Problem statement

From #126 and the ruling. The runner records its start time as `identity`, the text `ps -o lstart=` prints in the
runner's environment, and a later runner judges the owner stale when its own read of that pid differs. Two runners on
one host whose shells differ in time zone or locale read different texts for one live owner, so the second
quarantines the first one's lock while the first runs Docker work. The owner ruled a SemEngine-only fix: the runner
keeps writing `identity` as today and adds one key, the same start time read under `TZ=UTC LC_ALL=C`; `owner_is_stale`
and `ownerLive` compare that key when it is present and fall back to `identity` when it is absent. Accepted costs: the
"exactly the keys" sentence of `integration-test-runner` › "Shared host lock"; a SemEngine and a SemStreams runner
whose shells differ keep today's limit; the full role loop.

## 1. The claimed gap

**Claim A (#126): the runner records `identity` as ps's start time in its own environment.** Measured true.

| Pin | Text |
| --- | --- |
| `scripts/test-integration.sh:101` | `owner_identity=$(ps -o lstart= -p "$owner_pid" 2>/dev/null \| sed 's/^[[:space:]]*//' \|\| true)` |
| `scripts/test-integration.sh:102` | `[ -n "$owner_identity" ] \|\| owner_identity="unknown"` |
| `scripts/test-integration.sh:199` | `printf 'identity=%s\n' "$owner_identity"` |

**Claim B (#126): the stale rule compares a read made in the judging runner's environment.** Measured true.

| Pin | Text |
| --- | --- |
| `scripts/test-integration.sh:167` | `owner_is_stale() {` |
| `scripts/test-integration.sh:172` | `live=$(ps -o lstart= -p "$observed_pid" 2>/dev/null \| sed 's/^[[:space:]]*//' \|\| true)` |
| `scripts/test-integration.sh:173` | `[ "$observed_identity" != "unknown" ] && [ -n "$live" ] && [ "$live" != "$observed_identity" ]` |
| `scripts/test-integration.sh:209` | `if owner_is_stale && clean_stale_lock; then continue; fi` |

**Claim C (#126): the text depends on the reader's time zone and locale.** Measured true again (P10 and P17 on macOS;
P13 and P17 on Linux for the time zone only).

**Claim D (#126): SemStreams' runner at the pin writes and judges `identity` the same way.** Measured true.

| Pin (`scripts/run-integration-tests.sh` at `8b99efe9`) | Text |
| --- | --- |
| `:50` | `owner_identity=$(ps -o lstart= -p "$owner_pid" 2>/dev/null \| sed 's/^[[:space:]]*//' \|\| true)` |
| `:51-53` | `if [[ -z "$owner_identity" ]]; then` / `owner_identity="unknown"` / `fi` |
| `:104` | `live_identity=$(ps -o lstart= -p "$observed_pid" 2>/dev/null \| sed 's/^[[:space:]]*//' \|\| true)` |
| `:105` | `[[ "$observed_identity" != "unknown" && -n "$live_identity" && "$live_identity" != "$observed_identity" ]]` |
| `:217` | `printf 'identity=%s\n' "$owner_identity"` |

**Claim E (ruling): SemStreams' runner ignores a key it does not know.** Measured true at the pin, by reading and by
running its judge (P15).

| Pin (`scripts/run-integration-tests.sh` at `8b99efe9`) | Text |
| --- | --- |
| `:71` | `while IFS='=' read -r key value; do` |
| `:72-79` | `case "$key" in`, the branches `host`, `pid`, `started`, `identity`, `token`, `command`, then `esac`; no `*)` branch |
| `:130` | `current_token=$(awk -F= '$1 == "token" {sub(/^token=/, ""); print; exit}' "$lock_dir/owner")`: release reads `token` only |

Other readers of the record at the pin:

| Search over the pin tarball | Result |
| --- | --- |
| `grep -rn -I -E 'semstreams-integration\.lock\|INTEGRATION_LOCK_DIR\|ADMISSION_TOKEN\|lock_dir/owner\|/owner"'` | production code only in `scripts/run-integration-tests.sh` (`:68`, `:80`, `:112`, `:129-130`, `:136`, `:220`); its contract test `test/testinfra/integration_runner_contract_test.go` (`:299`, `:310`, `:900` read its own runner's record, `:947` plants one); docs that name the path but no key (`docs/contributing/01-testing.md:354`, `docs/operations/23-natsclient-test-helpers.md:40`) |

**Claim F (ruling): a read under `TZ=UTC LC_ALL=C` prints the same text whatever the reader's environment.** Measured
true for every environment tried, in bash and in Go, on macOS and in two Linux images (P10 to P14, P17). Locales other than
C on Linux are not measured (Q3).

**Claim G (ruling): SemEngine has two readers that compare a start time, `owner_is_stale` and `ownerLive`.** Measured
true for production code. A third judge, `task doctor`, reads no start time:

| Pin | Text |
| --- | --- |
| `scripts/doctor.sh:107-108` | `# The shared admission lock, judged by host and pid only: a pid reused by another process reads as` / `# busy here, though the runner, which also compares the owner's start time, would quarantine it.` |
| `scripts/doctor.sh:117` | `elif [[ "$o_pid" =~ ^[0-9]+$ ]] && ! kill -0 "$o_pid" 2>/dev/null; then` |

Test oracles that read a start time are listed in § 2; each compares two reads made in one environment.

**Claim H (ruling): `ownerLive` falls back to `identity` when the key is absent, "a lock SemStreams wrote".** For
`ownerLive` that case is not reachable. Measured:

| Pin or search | Text or result |
| --- | --- |
| `internal/harness/natsfixture/admission.go:53-55` | `if scanner.Text() == "token="+token {` / `held = true` / `}` |
| `internal/harness/natsfixture/admission.go:63-66` | `if !held {` ... `the lock is not held for this run` ...: returns before `ownerLive` |
| `internal/harness/natsfixture/admission.go:67` | `if err := ownerLive(ctx, fields["host"], fields["pid"], fields["identity"]); err != nil {` |
| `scripts/test-integration.sh:331` | `export SEMENGINE_DOCKER_ADMISSION_TOKEN="$owner_token"`: the only writer of the token admission requires |
| pin: `grep -n 'export ' scripts/run-integration-tests.sh` | `:259` `export TESTCONTAINERS_RYUK_DISABLED=false`; `:333` `export GRAPH_INDEX_LATENCY_LOG="$latency_log"`; nothing else |
| pin tarball: `grep -rln -I -E 'DOCKER_ADMISSION\|INTEGRATION_ADMISSION'` | no match |
| `scripts/test-integration.sh:20-21` | `root=$(cd "$(dirname "$0")/.." && pwd)` / `cd "$root"`: a runner's tests build from the runner's own tree |

So a record SemStreams wrote never reaches `ownerLive`'s comparison. A record without the new key reaches it only when
(a) a SemEngine runner from a commit before this change wrote it and a test binary from a commit after it reads it,
which needs a direct `go test` in another worktree with the runner's exported token copied into that shell; or (b) a
test plants it (`admission_test.go:38`, `:172`, `:195`). For `owner_is_stale` the ruling's case holds: it judges
whatever owner holds the lock, SemStreams' included (`test-integration.sh:208-209`).

**Claim I (the gap): no key or reader today holds the start time in a form independent of the environment.**
Measured true; the searches came up empty.

| Search | Result |
| --- | --- |
| `git grep -n -i -E '(start\|identity\|lstart)_?(utc\|c\|fixed\|canonical\|epoch\|time)\b\|utc_?(start\|identity)\|start_?time=\|identity_utc' -- scripts internal/harness '*.sh' openspec/specs docs/admission-ledger.yaml` | no match |
| `git grep -n -E 'LC_ALL\|LC_TIME\|LC_COLLATE\|LANG=C\|\bTZ=\|"TZ=\|"LC_' -- scripts Taskfile.yml .github '*.go' '*.sh' ':!openspec/changes/archive'` | no match: nothing in the tree fixes the locale or time zone of any subprocess |
| `git grep -n -E 'Setenv\("(TZ\|LC_ALL\|LC_TIME\|LANG)"\|"TZ=\|"LC_ALL=' -- '*.go'` | no match: no test sets them |
| `gopls references internal/harness/natsfixture/admission.go:84:6` (`ownerLive`) | `admission.go:67` only |
| `gopls references internal/harness/natsfixture/admission.go:119:6` (`startTime`) | `admission.go:96` only |
| `gopls references internal/harness/natsfixture/admission.go:29:6` (`admit`) | `fixture.go:129`, `admission_test.go:216`, `:229` |
| pin: `grep -n -E 'LC_ALL\|LC_TIME\|LANG=\|\bTZ=' scripts/*.sh Taskfile.yml` | only `scripts/e2e-statistical-up.sh:9` — `local label=$1 rc LC_ALL=C`, unrelated to the lock |

The record's one numeric time, `started`, is not the process's start time (§ 2, P16).

### Probes

Run 2026-10-07. macOS 26.5.2 (25F84) arm64, `/bin/bash` 3.2.57, system zone `America/Chicago`
(`readlink /etc/localtime`), session `LANG=C.UTF-8` with `TZ`, `LC_ALL` and `LC_TIME` unset; Go 1.26.4 for the probe
binary (gopls resolves the repository's toolchain, go1.26.6); Docker 29.8.2. Containers: `golang:1.26.6-bookworm`
(Debian 12, procps-ng 4.0.2, zone files present, locales `C`, `C.utf8`, `POSIX` only) and `ubuntu:24.04` (procps-ng
4.0.4, no `/usr/share/zoneinfo`). Helpers under `/tmp/claude-126/scripts`: `read-plain.sh` runs the runner's line
(`:101`) on a pid and prints the result in brackets; `read-fixed.sh` does the same under `TZ=UTC LC_ALL=C`.
`start_utc` below is a placeholder key name used by the probes only.

| Probe | Input | Result |
| --- | --- | --- |
| P10 (macOS) | one process (the probe's shell), read by `read-plain.sh` and `read-fixed.sh` under outer environments: inherited; `TZ=Asia/Tokyo`; `TZ=` (set, empty); `TZ=America/Los_Angeles`; `LANG=de_DE.UTF-8`; `LC_ALL=de_DE.UTF-8`; `LC_TIME=ja_JP.UTF-8`; `LANG=ja_JP.UTF-8`; `LC_ALL=en_GB.UTF-8`; `LANG=C.UTF-8`; `TZ=Asia/Tokyo LC_ALL=de_DE.UTF-8 LANG=ja_JP.UTF-8`; `env -i PATH=/usr/bin:/bin` | plain: `[Wed Oct  7 18:21:02 2026    ]` (inherited and `LANG=C.UTF-8`); `[Thu Oct  8 08:21:02 2026    ]` (Tokyo); `[Wed Oct  7 23:21:02 2026    ]` (`TZ=`); `[Wed Oct  7 16:21:02 2026    ]` (Los Angeles); `[Mi.  7 Okt. 18:21:02 2026   ]` (German, three trailing blanks); `[水 10/ 7 18:21:02 2026     ]` (Japanese, five); `[Wed  7 Oct 18:21:02 2026    ]` (en_GB). Fixed: `[Wed Oct  7 23:21:02 2026    ]` under every one of them, four trailing blanks. The plain read's padding also varies with the locale |
| P11 (macOS, Go) | `exec.Command("ps", "-o", "lstart=", "-p", pid)` with `cmd.Env = append(os.Environ(), "TZ=UTC", "LC_ALL=C")`, trimmed as `admission.go:124` does, against `read-fixed.sh` on the same pid; run under the inherited environment, `TZ=Asia/Tokyo LC_ALL=de_DE.UTF-8`, and `TZ=America/Los_Angeles LANG=ja_JP.UTF-8` | equal in all three, trailing blanks included; the appended `TZ` and `LC_ALL` override the inherited ones. Probe source: `/tmp/claude-126/goprobe/main.go` |
| P12 (macOS) | `LC_ALL=C` with `TZ=UTC`, `UTC0`, `Etc/UTC`, `GMT`, `Nonexistent/Zone`; and `TZ=UTC LC_ALL=POSIX` | one text for all six |
| P13 (Debian container) | P10 and P11 again, outer `TZ=Asia/Tokyo`, `TZ=`, `LANG`/`LC_ALL`/`LC_TIME=C.UTF-8`; `TZ=UTC`, `UTC0`, `Etc/UTC`, `Nonexistent/Zone`; `od -c` of the fixed read | plain: `[Thu Oct  8 08:21:33 2026]` under Tokyo, else `[Wed Oct  7 23:21:33 2026]`. Fixed: `[Wed Oct  7 23:21:33 2026]` under all, no trailing blanks (`2 0 2 6 \n`); the four `TZ` spellings agree; the Go fixed read equals bash under the inherited environment and under Tokyo |
| P14 (`ubuntu:24.04` container) | plain and fixed reads, inherited and `TZ=Asia/Tokyo` | all four `[Wed Oct  7 23:21:42 2026]`: with no zone files, a zone name such as `Asia/Tokyo` has no effect. A POSIX `TZ` string still does (P17) |
| P15 (SemStreams pin's judge) | lines 61-106 of the pin's `scripts/run-integration-tests.sh` (`read_owner`, `owner_elapsed`, `describe_owner`, `owner_is_stale`; the fetched file's sha256 `9a4bcf19e1dff768cfa78ac8824ce645e095bfd504f554dac520d614223a58d9`), sourced under `set -euo pipefail` with `owner_host=$(hostname)` and a scratch `lock_dir`; owner = the probe's shell; records newline-terminated | six keys, `identity` read in the judge's environment: respected. Seven keys, `start_utc=<fixed read>` last: respected. Seven keys, `start_utc` between `identity` and `token`: respected. In all three `read_owner` returned the six written values unchanged. Seven keys with `identity` written under `TZ=Asia/Tokyo`: STALE; six keys, the same: STALE (#126, unchanged by the extra key) |
| P15b (SemEngine's judge at the base) | P15's records, with `scripts/test-integration.sh:136-150` and `:165-174` in place of the pin's lines | the same five verdicts: a SemEngine runner from before this change ignores the extra key too |
| P15c (both judges, a record without a final newline) | the first P15 attempt wrote records whose last line had no `\n` | both judges dropped the last line (`while IFS='=' read -r` stops at an unterminated line): `command` read `unknown`, or the extra key was lost. Both writers end every line with `\n` (`test-integration.sh:196-201`; pin `:214-219`); Go's `bufio.Scanner` (`admission.go:51-58`) keeps such a line |
| P17 (POSIX `TZ` strings; added in review round 1) | `/tmp/claude-126/scripts/posix-tz.sh`: plain and fixed reads of one shell under the inherited environment, `TZ=Asia/Tokyo`, `TZ=JST-9`, `TZ=UTC0` and `TZ=JST-9 LC_ALL=C`, in `ubuntu:24.04` and `golang:1.26.6-bookworm`; on macOS, `TZ=JST-9` and `TZ=JST-9 LC_ALL=de_DE.UTF-8` | `ubuntu:24.04` (no zone files): plain `[Wed Oct  7 23:44:43 2026]` under the inherited environment, `Asia/Tokyo` and `UTC0`, but `[Thu Oct  8 08:44:43 2026]` under `JST-9` and `JST-9 LC_ALL=C`; fixed `[Wed Oct  7 23:44:43 2026]` under all five. Debian: plain `[Thu Oct  8 08:44:43 2026]` under both `Asia/Tokyo` and `JST-9`; fixed unchanged under all five. macOS: plain `[Thu Oct  8 08:44:34 2026    ]` under `JST-9`; fixed `[Wed Oct  7 23:44:34 2026    ]` under `JST-9` and under `JST-9 LC_ALL=de_DE.UTF-8`. libc parses a POSIX rule string without zone files, so #126 reproduces on a host without zone data, and a test that varies `TZ` only by zone name cannot fail on such a host |
| P16 (CI, ubuntu-24.04) | main's run 37700040658 at `b34ab3a`, artifact `integration-evidence-37700040658-1`, file `lock-owner` | `identity=Wed Oct  7 23:05:49 2026`: C-locale text, no trailing blanks, UTC (the run started 23:03:50Z). `started=1791414350`, which is `Wed Oct  7 23:05:50 UTC 2026` (`date -u -r 1791414350`): one second after the kernel's start time. On that runner the plain read and a fixed read would print the same text |

Not measured: a non-C locale on Linux; the environment of the GitHub runner itself (P16 shows only its output); a
change of the system time zone between a write and a read (Q2); a wall-clock step on Linux (Q4); a pid reused by the
kernel.

## 2. Every current spelling of the fact

The fact: the owner process's start time, kept so that a later reader can decide whether the recorded pid still names
the process that took the lock.

| Home | Pin | Environment of the read | What it decides |
| --- | --- | --- | --- |
| SemEngine writer | `scripts/test-integration.sh:101-102`, `:199` | the runner's own | records `identity`, or `unknown` |
| SemEngine runner's judge | `scripts/test-integration.sh:167-174` | the judging runner's | stale: same host, numeric pid, and `kill -0` fails or a read differs from a known `identity` |
| SemEngine runner's parser | `scripts/test-integration.sh:136-150`; `:141-148` — `case "$key" in` ... `esac`, no `*)` branch | none | six keys into `observed_*`; any other key ignored (P15b) |
| SemEngine release | `scripts/test-integration.sh:236` — `[ -f "$lock_dir/owner" ] && current=$(awk -F= '$1 == "token" {sub(/^token=/, ""); print; exit}' "$lock_dir/owner")` | none | reads `token` only |
| Fixture admission | `internal/harness/natsfixture/admission.go:51-58` parses every key into `fields`; `:67` passes `host`, `pid`, `identity`; `:84-114` `ownerLive`; `:119-125` `startTime` | the test process's, inherited from the runner through `go test` (`test-integration.sh:331-334` exports) | live: this host, positive pid, a non-empty read equal to `identity` |
| `task doctor` | `scripts/doctor.sh:111-119`; `:112` — `owner=$(tr '\n' ' ' < "$admission_lock_default/owner")` | none | "busy" or "stale (dead pid ...)" from host and `kill -0`; prints the whole record, so a new key appears in its output |
| Evidence copy | `scripts/test-integration.sh:204` — `cp "$lock_dir/owner" "$evidence_dir/lock-owner"` | none | the record as written, kept as a CI artifact (P16) |
| `started` | `scripts/test-integration.sh:100` — `owner_started=$(date +%s)`; read at `:152-159` (elapsed), `:177` (quarantine name), `:103` (inside the token) | none: epoch seconds | not compared by any judge. It is the time the script reached `:100`, not the kernel's start time: one second later in P16 |
| SemStreams writer and judge | pin `:47-53`, `:61-81`, `:97-106`, `:213-220` | the SemStreams runner's | the same rule (claim D); unknown keys ignored (claim E) |
| Runner spec | `openspec/specs/integration-test-runner/spec.md:12` — `exactly the keys host, pid, started, identity, token, command (command naming this repository and worktree) before any`; `:13-16` — ``Docker call. The identity SHALL be the runner's start time as `ps -o lstart= -p <pid>` prints it in the runner's`` / ``environment, with leading blanks removed and trailing blanks kept, or `unknown` when that prints nothing. An owner`` / ``record is live when its host is this host and `ps -o lstart= -p <pid>`, read the same way in the reader's`` / ``environment, prints its identity; an `unknown` identity is never live. ps reports start times to the second, so a``; `:18-20` the quarantine rule; scenarios `:33-36`, `:38-41` | "the reader's environment" | defines `identity` and "live" |
| Fixture spec | `openspec/specs/nats-fixture/spec.md:13` — ``owner is live as `integration-test-runner` › "Shared host lock" defines it, read in the test process's environment.``; `:14-18`; scenarios `:25-40` | the test process's | refers to the runner's definition |
| R4, the key-list oracle | `internal/harness/runner/runner_test.go:25` — `var semstreamsOwnerKeys = []string{"host", "pid", "started", "identity", "token", "command"}`; `:519-521` — `if !slices.Equal(keys, semstreamsOwnerKeys) {` | none | fails on any seventh key; its comment `:21-24` cites SemStreams at `5457b345` (the ledger row's `source_sha`), not the pin |
| Runner test writer | `runner_test.go:227-238` `writeOwner`; `:233` writes six keys and `command=scripts/run-integration-tests.sh`, a SemStreams-shaped record; used by R1 `:340`, `:366` and R2 `:385`, `:388` | none | every planted owner lacks a new key |
| Runner test oracles | `runner_test.go:261-271` `psStartIdentity` and `trimIdentity`; `:277-286` `awaitLaterStartIdentity`; `:299-331` `deadPID` | the test's, which is also the runner's | `newHarness` (`:146-177`) hands the runner the test's environment minus `SEMENGINE_*`, `SEMSTREAMS_*` and `FAKE_*`, plus `extra...` (`:177`): a case can already give the runner another `TZ` or locale; none does |
| Fake `go` | `runner_test.go:67` copies the record to `go.owner`; `:113-116` `steal` mode rewrites `token` with `sed -i.bak` | none | none |
| Admission test writer and oracle | `admission_test.go:26-47` `plantLock` (six keys, `:38`), `:52-63` `runnerIdentity`; `:150-185` (cases `:164-168`, record `:172`); `:189-209` (a record with no `identity` key, `:195`); `:214-232` | the test's | every planted owner lacks a new key |
| prochost's oracle | `internal/harness/prochost/prochost_test.go:307-314` | one process, two reads | another fact: a helper is gone |
| SemStreams' own tests | pin `test/testinfra/integration_runner_contract_test.go:947` plants `identity=stale` | none | its own runner's tests |

How each parser reads a key the record lacks (added in review round 1; Q5 depends on it):

| Parser | Pin | An absent key reads as | Verdict on an absent `identity` |
| --- | --- | --- | --- |
| SemEngine `read_owner` | `scripts/test-integration.sh:137-138` — `observed_host=unknown observed_pid=unknown observed_started=0` / `observed_identity=unknown observed_token=unknown observed_command=unknown` | `unknown`: every `observed_*` is set before the file is read | respected, as `unknown` is (`:173`) |
| SemStreams `read_owner` | pin `scripts/run-integration-tests.sh:62-67` (`observed_identity="unknown"` at `:65`) | `unknown` (`observed_started` `0`) | respected (pin `:105`) |
| admission's parser | `internal/harness/natsfixture/admission.go:56-57` — `if k, v, ok := strings.Cut(scanner.Text(), "="); ok {` / `fields[k] = v`; `:67` reads `fields["identity"]` | `""`, a missing map key | refused: an empty read at `:107-108`, and a non-empty read never equals `""` (`:110-111`). `TestAdmissionRefusesAnEmptyStartTime` (`admission_test.go:189-209`) plants such a record |

Neither parser tests for presence. In bash an absent key and the value `unknown` are one string; in Go an absent key
and an empty value are one string. A new key read the way `read_owner` reads the others would make "absent" and "the
fixed read printed nothing" (Q5) one value.

Texts that state the record is SemStreams' format, unchanged: a second spelling of the format fact. The ruling names
only the spec sentence.

| Pin | Text |
| --- | --- |
| `docs/admission-ledger.yaml:199` | `host lock protocol (path, owner keys, stale rule, quarantine, token-checked release), Docker` |
| `docs/admission-ledger.yaml:203` | `byte-compatible lock protocol at /tmp/semstreams-integration.lock;` |
| `scripts/admission-lock.sh:3-4` | `# Docker admission lock path. It is SemStreams' lock, adopted byte-compatibly (owner` / `# ruling Q1, 2026-09-30) so either repository's runner sees the other as an ordinary` |
| `scripts/test-integration.sh:5-6` | `#   - the host lock is SemStreams' own, byte-compatible, so the two repositories` / `#     serialise on one daemon and each sees the other as an ordinary owner;` |
| `scripts/test-integration.sh:134` | `# ---- lock (byte-compatible with SemStreams run-integration-tests.sh:78-242) ----` |
| `internal/harness/runner/runner_test.go:23-24` | `// oracle: both runners must write and read exactly these keys, in this order, or neither can judge` / `// the other's lock.` |
| `openspec/changes/archive/2026-09-30-setup-02-isolated-harness/design.md:133-140` | history; archives are not edited |

How a comparison behaves today when two environments differ, from the pins above, P10 and P15:

| Writer → judge | Shells differ in time zone or locale | Same environment |
| --- | --- | --- |
| SemEngine runner → SemEngine runner | live owner judged stale, lock quarantined (#126) | respected |
| SemEngine runner → SemStreams runner, or the reverse | live owner judged stale (P15, Tokyo rows) | respected |
| SemEngine runner → its own `go test` | not reachable: the test binary inherits the runner's environment | admitted |
| SemEngine runner → a direct `go test` from another shell holding a copy of the live runner's token | refused, start time differs (`admission.go:110-111`); the archive design (`design.md:79-81`) calls this "the safe direction" | admitted |

## 3. Adjacent claims on the territory

| Kind | Search or pin | Result |
| --- | --- | --- |
| Specs | `git grep -n -l -i -E 'owner record\|owner file\|lock owner\|host lock\|lstart\|start identity\|start time' -- openspec/specs docs .agents AGENTS.md README.md Taskfile.yml ':!docs/admission-ledger.yaml'` | `openspec/specs/integration-test-runner/spec.md`, `openspec/specs/nats-fixture/spec.md`, `docs/repository-map.md` (`:47`, `:70`), `.agents/skills/semengine-preflight/SKILL.md:42` and `Taskfile.yml:96` (both say "host lock" only) |
| ADRs | `git grep -n -i -E 'lstart\|start identity\|admission lock\|owner record\|locale\|time zone' -- docs/adr` | no match (`docs/adr/` holds 102 and 104) |
| Active changes | `ls openspec/changes` | `archive` only |
| Archive, #125 | `admission-owner-identity/design.md:196-198` | runner finding 2: ``identities for one owner (P2), so one would judge the other's live owner changed and quarantine its lock. Present at`` / ``the pin (P8). The fix sets the environment of every `ps -o lstart` read on both sides, a joint change with`` / ``SemStreams. Tracked in #126. Admission is not exposed: it reads in the runner's own environment.``; owner decision 4 at `:222-223`, conformance at `:237` |
| Archive, SETUP 02 | `setup-02-isolated-harness/design.md:139-140` | `the initial bound (ruled). A SemStreams protocol change after the pin enters as its own ledger row; R4 pins the` / `owner-file format.` |
| Owner rulings | `setup-02-isolated-harness/design.md:402`; PR #125 comments 6047427432 and 6047433678; #126 comment 6048776879 | ``- Q1: adopt `/tmp/semstreams-integration.lock` as-is, exclusive as the initial bound, no SemStreams rename issue now.`` (on #6); #125's five decisions and acceptance; this change |
| Plan | `docs/setup-plan.md:267-268` | `saturation. During extraction, admission control for a shared Docker daemon must coordinate SemEngine and SemStreams;` / `independent repository locks must not each claim exclusive use of the same daemon.` |
| Ledger | `docs/admission-ledger.yaml:196-212` | the runner row: `source_sha` `5457b345...` (`:197`), disposition `adapt` (`:212`), contract `:202-204`, `:206` — `bash, ps, mkdir, docker CLI; creates the lock dir; may quarantine stale dirs; pulls images`, `:210` — `proving_tests: runner contract tests R1–R4; pass-evidence items 1–3` |
| Ledger, admission | `git grep -n -i -E 'ownerLive\|owner file\|owner record\|admission token' -- docs/admission-ledger.yaml` | no match: admission is in no row |
| Issues | `gh issue list --state all --search 'lstart OR "owner record" OR "owner file" OR "start identity" OR locale OR "time zone" OR "semstreams-integration.lock"'` | #126 (open, this change); #124 (closed by #125); #50 (closed); #6 (closed, SETUP 02 epic, where Q1 was ruled); #69, #19, #77 match other words |
| Issues, docs | #86 (open, no claim) | `docs/repository-map.md` omits three archive rows; this change's archive row would land beside `:70` |
| Rules index | `AGENTS.md` | rows that bind here: a failure path fails closed and a degrade is declared; tests use an independent oracle and are shown able to fail; the generated-checks decision; no `time.Sleep`, no skip and no fixed address in tests; the coverage floor (`scripts/cover-check.sh:24` — `"internal/harness/natsfixture integration"`); sister repositories read-only |
| Rules index, child processes | `AGENTS.md:81`; canonical `docs/testing.md:467-468` — ``- Check that a child process is alive before inspecting it. A test that looks at another process (with `ps`, a`` / ``signal, or its output) first confirms the process is still running, so a child that died early fails with that`` (to `:471`) | review only. Every test that reads another process's start time with `ps` falls under it (added in review round 1) |
| Runner constraint | `scripts/test-integration.sh:13` | `# Must stay bash 3.2 compatible: it is macOS's /bin/bash.` P10 ran the fixed read under `/bin/bash` 3.2.57 |
| Docs | `docs/repository-map.md` | `:37` (the `natsfixture` row), `:47` (the runner), `:55` (the two specs), `:70` (the last archive row) |

Open pull requests, from `gh pr list --state open --json number,title,isDraft,changedFiles,headRefOid,files`; #93 read
in full with `gh api --paginate repos/C360Studio/semengine/pulls/93/files --jq '.[].filename'` (210 files):

| PR | Files | Overlap |
| --- | --- | --- |
| #127 (draft) | 0 | this claim, head `11e4d7f` |
| #93 (draft) setup-04a-02 ingest kernel, head `0ee1921` | 210 | Same Go package: `internal/harness/natsfixture/fixture_integration_test.go`, `open.go`, `open_test.go`. Same capability: `openspec/changes/setup-04a-02-ingest-kernel/specs/nats-fixture/spec.md`, which ADDs only "Connected value for a package's tests" (`## ADDED Requirements` at `:3`, the requirement at `:5`). Read at `0ee1921` with `gh api .../contents/<path>?ref=0ee1921` (revision 1 read `4df8590`; the commits between touch `graph/` files and the change's `design.md` only): none of the four files contains `admit(`, `identity`, `plantLock`, `Setenv`, `ownerLive`, `lstart`, `startTime`, `LC_ALL` or `"TZ`. Not in its list: `admission.go`, `admission_test.go`, `fixture.go`, any `scripts/` file, `internal/harness/runner/`, `internal/harness/prochost/`, an `integration-test-runner` delta, `docs/repository-map.md`, `docs/admission-ledger.yaml`, `AGENTS.md`, `.github/` |
| #116, #118, #119, #120 (Dependabot, Go modules) | 2 each | `go.mod`, `go.sum`: none |
| #117 (Dependabot) `@fission-ai/openspec` 1.13.2 → 1.14.0 | 2 | `package.json`, `package-lock.json`: no shared file; it changes the CLI `task spec:check` runs |

## 4. The consumer at birth

New surface: one key in the owner record (name not chosen here). It is not an exported Go symbol, flag, environment
variable, subject, bucket or config field. Its writer would be `acquire_lock` (`scripts/test-integration.sh:195-202`);
the ruling names its two consumers, both present: `owner_is_stale` (`:167-174`) and `ownerLive` (`admission.go:84-114`).
The key-absent branch of `ownerLive` has no production writer that reaches it (claim H); the key-absent branch of
`owner_is_stale` is reached by every SemStreams record and every record of a SemEngine runner from before this change
(P15b).

Unexported surface the ruling's readers would touch, each with its one production caller: `ownerLive`'s parameters
(`admission.go:84`, caller `:67`) and `startTime` (`:119`, caller `:96`).

## 5. The problem shape

Two shapes, neither decided here.

1. **Fix a subprocess's environment so the text it prints does not depend on the caller.** Closest instances, none of
   which fixes a locale or a time zone (claim I):

   | Pin | Text |
   | --- | --- |
   | `internal/harness/pindiff/pin.go:23` | `func gitEnv() []string {` |
   | `internal/harness/pindiff/pin.go:30` | `return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0")`; used at `:35` and `:89` |
   | `internal/harness/mutcheck/runs.go:18-19` | `// withEnv returns environ with each entry of set in place of the entries with its key.` / `func withEnv(environ []string, set ...string) []string {`; used at `:39` |
   | `scripts/openspec-queue.sh:156` | `now_epoch=$(date -u +%s)` |

2. **An additive record field that readers prefer, beside an older one kept for writers that do not know the new
   one.** Closest instances:

   | Pin | Text |
   | --- | --- |
   | `natsclient/errors.go:17-18` | `// ce.Detail. (The earlier additive window — a legacy "error: <msg>" body kept` / `// alongside the headers — was retired with the PR-D breaking change.)`: the same shape, later retired by a breaking change |
   | `internal/cache/config.go:304` | `// Fall back to integer (nanoseconds) for backward compatibility`: one field that accepts an older encoding |

Not triggered, with the reason:

- **Adoption sweep:** no primitive meant for reuse across packages is proposed: one key in one record, read by one
  bash function and one Go function.
- **Intent check:** no boundary is set or moved.
- **Extraction slices:** nothing is ported. The runner's ledger row is `adapt`, and its contract text
  (`docs/admission-ledger.yaml:203`) is one of the format statements in § 2.
- **Context retention:** `git grep -n 'context.Context' -- internal/harness/natsfixture/admission.go` → parameters at
  `:29`, `:84`, `:119`; no struct field.

## Same-class collision table

Triggered: the owner record is a runtime-coordination primitive shared with another repository, and the ruling changes
its format.

| Dimension | Evidence |
| --- | --- |
| Semantic class | whether the host lock's holder is still the process that took it |
| Owners | SemEngine runner (`test-integration.sh:167-174`); fixture admission (`admission.go:84-114`); `task doctor` (`doctor.sh:111-119`, host and pid only); SemStreams runner (pin `:97-106`) |
| Catalogs | none besides the record. The lock path has one spelling, `scripts/admission-lock.sh:7` — `readonly admission_lock_default="/tmp/semstreams-integration.lock"`; also named in `docs/admission-ledger.yaml:203`, `openspec/specs/integration-test-runner/spec.md:11`, `.github/workflows/ci.yml:55` (comment) |
| Status | `describe_owner` (`test-integration.sh:161-163`, stderr); `record lock_quarantined` `:185`, `lock_refused` `:213`, `lock_waited_on` `:219`, `lock_released_ms` `:244` in `runner.env`; `doctor.sh:116-122`; admission's `ErrNotAdmitted` text (`admission.go:73`, `:111`) |
| Lifecycle | create: `acquire_lock` `:191-207` (atomic `mkdir`, then the record); quarantine: `clean_stale_lock` `:176-189`; release: `release_lock` `:233-245`. Production code never rewrites a record in place. SemStreams: pin `:208-222`, `:108-121`, `:123-140` |
| Ownership | one exclusive holder per host by atomic `mkdir` (spec `:11`), shared by both repositories (`admission-lock.sh:2-6`; plan `:267-268`); release only while `token` matches (`:236-240`; pin `:128-135`) |
| Readers | § 2: two runner parsers, two judges that compare a start time, admission, doctor, the evidence copy, R4, the fake `go` |
| Writers | SemEngine `:195-202`; SemStreams pin `:213-220`; tests `runner_test.go:233`, `admission_test.go:38`, `:172`, `:195`, pin `integration_runner_contract_test.go:947` |
| Recovery | quarantine only on proof of death (`:165-174`); the move is refused for another user's directory (archive inventory P9: sticky `/tmp`). The lock directory must hold only `owner`: quarantine's `rmdir` exits 1 on anything else (`test-integration.sh:180-182`; pin `:113-115`), and release warns (`:242`; pin `:137-139`). No rebuild or replay |

## Adopter seam inventory

Surfaces reached from outside the code being changed: the shared record, which SemStreams' runner and SemEngine runners
at older commits read (no other reader is measured: Q6); the developer who runs two runners at once; the developer who
runs `go test` directly; whoever reads `task doctor` output or the CI `lock-owner` evidence.

- **SemStreams' runner at the pin.**
  1. What must it know? Nothing, given two conditions it does not state: the new key is a newline-terminated line of
     `owner` (P15c), not a file in the lock directory (quarantine's `rmdir`, pin `:113-115`).
  2. If it does nothing: it reads `identity` as today (P15). Against a SemEngine runner whose shell differs, it still
     quarantines a live lock, and the reverse (accepted cost 2).
  3. Where does it find out? Nowhere until a collision: its own stdout line `cleaned stale lock` (pin `:117`).
  4. What should it have to know? Nothing. The remaining gap is the accepted cost, open until SemStreams reads the key.
- **A SemEngine runner from before this change** (any worktree not yet merged with `main`). Same as SemStreams: it
  ignores the key (P15b) and judges by `identity` in its own environment. A runner after the change, judging its
  six-key record, falls back to `identity`: today's behaviour.
- **A developer who starts two SemEngine runners from shells with different `TZ` or locale** (#126).
  1. Must know today: that the shells must match. Nothing says so.
  2. If they do nothing: the second runner quarantines the first's live lock and both run Docker work. The second
     prints `cleaned stale lock` (`test-integration.sh:184`); the first learns only at release:
     `lock ownership changed; refusing to remove` (`:238`).
  3. Where they find out: a log line, after the fact.
  4. Should know: nothing. That gap is #126.
- **A developer who runs `go test` directly** with a copy of a live runner's token, from a shell whose `TZ` or locale
  differ: refused today with the start-time message (`admission.go:111`). If admission compares a key read under a
  fixed environment, this case is admitted: the owner is live and holds the token. The archive design called the
  refusal "the safe direction" (`design.md:79-81`); the design states which it keeps.
- **Readers of the record as text:** `task doctor` prints the whole record (`doctor.sh:112`, `:116-119`) and CI keeps
  it as `lock-owner` (`test-integration.sh:204`, P16). A new key appears in both; no doc names it today.

Prefer observation to prediction: each reader today predicts that its own environment formats a time as the writer's
did, and it cannot observe the writer's environment. A key whose text no reader's environment changes removes that
prediction for the readers that know the key; readers that do not (SemStreams, older SemEngine runners) still predict.

## Open evidence questions

- **Q1. SemStreams `main` beyond the pin is not read.** The ruling says so too ("Not checked"). The runner that shares
  the host lock in practice runs from a SemStreams checkout, not from the pin, so claim E holds for the pin only.
  `docs/inventory-scope.md` (the semstreams row) admits `main` only "to find unmerged work touching port-set
  packages", and the brief limits code facts to the pin; reading `scripts/run-integration-tests.sh` at `main` needs the
  owner's or the orchestrator's word.
- **Q2. A change of the system time zone between a write and a read** (for example macOS's automatic time zone on a
  laptop that moves). With `TZ` unset, the plain read follows `/etc/localtime` (here `America/Chicago`). If ps reads
  the zone at each call, a runner and its own tests would read different texts after such a change, and the archive
  design's "Admission is not exposed" (`design.md:198`) would not hold. Not measured: changing the system zone needs
  root.
- **Q3. Locales on Linux.** Neither image has a non-C locale, so the plain read's locale dependence on Linux and the
  fixed read's independence from it are not measured. The GitHub runner's own `TZ` and `LANG` are not read; P16 shows
  only that its ps printed UTC in C-locale form.
- **Q4. A wall-clock step on Linux** (archive design D3, `design.md:109`): `lstart` there follows the boot time, which
  moves with the clock. A fixed environment does not change that. Not measured.
- **Q5. The fixed read printing nothing for the runner's own pid** was not reproduced on either system; what the
  record then holds is open.
- **Q6. Other readers of the lock path.** Whether semsource, semconnect, semteams or semboids takes
  `/tmp/semstreams-integration.lock` is not a question `docs/inventory-scope.md` admits for them, so it needs the
  owner's word to measure. Nothing at the pin points to another user of the path: its two docs that name it
  (`docs/contributing/01-testing.md:354`, `docs/operations/23-natsclient-test-helpers.md:40`) name only SemStreams'
  runner.
