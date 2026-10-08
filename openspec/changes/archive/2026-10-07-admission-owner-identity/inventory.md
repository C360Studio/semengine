# Inventory: admission-owner-identity

- base: `5d31992a188a38fcd86968739641eb2d68609350`, the empty claim commit on `claude/admission-owner-identity` over
  `origin/main` `18d72af7d154e8b0ec12340864de25bd5b3b25fa`, so every pin below holds on both.
- revision 2, measured 2026-10-07; issue #124, claim PR #125. Revision 1 had review round 1 (`INVENTORY CHANGES
  REQUESTED`); revision 2 adds `scripts/doctor.sh` as a third judge (§ 2), the SemStreams pin probe P8, the `/tmp`
  mode P9, issue #126 and #93's new head (§ 3).
- repositories read: this one, and SemStreams at its pin `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` for one question
  added in review round 1: does its runner write and judge `identity` as SemEngine's does (P8). Code facts come from
  the pin snapshot, cited with `gh api ... ?ref=<sha>` (`docs/inventory-scope.md`, the semstreams row and rule 4).
  Admission reads only an owner record that holds the token a SemEngine runner exported, and this change alters
  neither the record nor the runner. The record's six keys are already held by R4
  (`internal/harness/runner/runner_test.go:484`).
- probes: P1 to P9 below, run in a scratch directory outside the worktree and in one throwaway container
  (`docker run --rm --network none`); nothing in the worktree was written. Pinned texts are trimmed of leading
  whitespace.

Question: where is the integration-test lock's owner judged alive or dead, how does each place read a process's start
time, can a Go read equal the runner's recorded `identity` on macOS and on Linux, and what else claims the same files
and specs?

## 1. The claimed gap

**Claim A (#124): admission judges the owner live by host and pid alone.** Measured true.

| Pin | Text |
| --- | --- |
| `internal/harness/natsfixture/admission.go:54-55` | `if k, v, ok := strings.Cut(scanner.Text(), "="); ok {` / `fields[k] = v`: every key, `identity` included, is parsed |
| `internal/harness/natsfixture/admission.go:65` | `if err := ownerLive(fields["host"], fields["pid"]); err != nil {`: `identity` is not passed |
| `internal/harness/natsfixture/admission.go:75` | `func ownerLive(host, pid string) error {` |
| `internal/harness/natsfixture/admission.go:87-88` | `// Signal 0 checks existence without signalling; EPERM still means the process exists.` / `if err := syscall.Kill(n, 0); err != nil && !errors.Is(err, syscall.EPERM) {` |
| `internal/harness/natsfixture/admission.go:24-25` | `// admit reads the runner's environment and requires its token in the live lock owner file. The` / `` // environment alone is never trusted: a stale shell export must not admit a direct `go test`. ``: the claim #124's sequence breaks |

**Claim B (#124): the runner also compares the owner's start time.** Measured true.

| Pin | Text |
| --- | --- |
| `scripts/test-integration.sh:101` | `owner_identity=$(ps -o lstart= -p "$owner_pid" 2>/dev/null \| sed 's/^[[:space:]]*//' \|\| true)` |
| `scripts/test-integration.sh:102` | `[ -n "$owner_identity" ] \|\| owner_identity="unknown"` |
| `scripts/test-integration.sh:199` | `printf 'identity=%s\n' "$owner_identity"` (inside `acquire_lock`, `:191`) |
| `scripts/test-integration.sh:165-166` | `# Stale only when provably dead on this host: the pid is gone, or it now names a` / `# process with a different start time. Another host's owner is never judged here.` |
| `scripts/test-integration.sh:170` | `kill -0 "$observed_pid" 2>/dev/null \|\| return 0` |
| `scripts/test-integration.sh:172-173` | `live=$(ps -o lstart= -p "$observed_pid" 2>/dev/null \| sed 's/^[[:space:]]*//' \|\| true)` / `[ "$observed_identity" != "unknown" ] && [ -n "$live" ] && [ "$live" != "$observed_identity" ]` |

**Claim C (#124): the spec defines a live owner by host and pid.** Measured true.
`openspec/specs/nats-fixture/spec.md:13` — `owner file is live when its host is this host and its pid is a running
process. The refusal SHALL make no Docker`.

**Claim D (#124): `ps -o lstart` appears only in the runner and two tests; nothing in Go reads a start time outside a
test.** Measured true.

- `git grep -n 'lstart' -- ':!openspec/changes/archive'`: code at `scripts/test-integration.sh:101`, `:172`;
  `internal/harness/runner/runner_test.go:262`, `:282`; `internal/harness/prochost/prochost_test.go:312`; the rest are
  comments in the same two test files.
- `git grep -n -E 'kinfo|/proc/|ProcessStart' -- '*.go'`: no match.
- `gopls workspace_symbol -matcher=fuzzy StartIdentity`: `prochost_test.go:311` `startIdentity`,
  `runner_test.go:252` `startIdentity`, `:261` `psStartIdentity`, `:277` `awaitLaterStartIdentity`. No non-test
  function.
- `gopls references admission.go:27:6` (`admit`): `internal/harness/natsfixture/fixture.go:129` only.
  `gopls references admission.go:75:6` (`ownerLive`): `admission.go:65` only.

### Probes

Run 2026-10-07. macOS 26.5.2 (25F84) arm64, Go 1.26.6, `/bin/bash` 3.2.57; the Linux container is
`golang:1.26.6-bookworm` (Debian 12, procps-ng 4.0.2, `CLK_TCK` 100) on Docker 29.8.2, kernel 7.0.14-linuxkit arm64.

| Probe | Input | Result |
| --- | --- | --- |
| P1 (macOS) | one live bash process; the runner's line (`:101`) run in it, then from Go: `exec.Command("ps", "-o", "lstart=", "-p", pid).Output()` with the one trailing newline and leading blanks and tabs removed; and a native read, `unix.SysctlKinfoProc("kern.proc.pid", pid)`'s `P_starttime` formatted `Mon Jan _2 15:04:05 2006` in local time | runner: `"Wed Oct  7 16:09:39 2026    "` (four trailing blanks: ps pads the column; `od -c` of raw ps shows them before the newline). Go exec read: the same string, equal. Native read: `"Wed Oct  7 16:09:39 2026"`, not equal; equal only after the padding is removed |
| P2 (macOS) | the same process's `ps -o lstart=` under other environments | `TZ=UTC` and `TZ=` (set, empty): `Wed Oct  7 21:09:47 2026`; `TZ=Asia/Tokyo`: `Thu Oct  8 06:09:47 2026`; `LC_ALL=de_DE.UTF-8`: `Mi.  7 Okt. 16:09:47 2026`; `LC_ALL=en_GB.UTF-8`: `Wed  7 Oct 16:18:20 2026`; `LANG=ja_JP.UTF-8`: `水 10/ 7 16:09:47 2026`; `LANG=en_US.UTF-8`, `LANG=C.UTF-8`, `LANG=` and `TZ` unset: the inherited text. The text depends on the reader's time zone and locale |
| P3 (macOS) | `ps -o lstart= -p` an absent pid | in range (98947): no output, exit 1. `2147483647`: no standard output, `ps: process id too large: 2147483647` on standard error, exit 1 |
| P4 (macOS) | `/bin/bash -c 'kill -0 <pid>'` as uid 501 | pid 1 (root's): `kill: (1) - Operation not permitted`, exit 1. Absent pid: `No such process`, exit 1. Bash's `kill -0` cannot tell the two apart |
| P5 (Linux container) | P1 and P3 again; a native read from `/proc/stat` `btime` plus `/proc/<pid>/stat` field 22 at 100 ticks a second; 3 s of repeated Go reads of one process | runner: `"Wed Oct  7 21:10:24 2026"`, no trailing blanks. Go exec read: equal. Native read: equal (C locale, no padding). 2670 repeated reads, 0 differed. `TZ=Asia/Tokyo` changes the text; `LC_ALL=de_DE.UTF-8` does not, but that locale is not installed in the image, so locale sensitivity on Linux is not measured. Absent pids `2147483647` and `99999`: no output, exit 1 |
| P6 (CI, ubuntu-24.04) | the evidence artifact of main's run 37685816240 (commit `18d72af`), file `lock-owner` | `identity=Wed Oct  7 20:57:57 2026`, no trailing blanks, not `unknown`. The same run's `task verify` log: `ok  github.com/c360studio/semengine/internal/harness/runner 17.659s`. R1 (`runner_test.go:337`) plants a live owner whose identity is the Go exec read (`:340`) and requires the real runner to respect it (exit 1), so on ubuntu-24.04 the Go exec read equals the runner's read of the same process. No test may skip (`TestNoSkippedTests`) |
| P7 (macOS) | `exec.CommandContext(ctx, "ps", ...).Output()` with `ctx` already cancelled | `context canceled`, `errors.Is(err, context.Canceled)` true; ps is not started |
| P8 (SemStreams pin) | `gh api 'repos/C360Studio/semstreams/contents/scripts/run-integration-tests.sh?ref=8b99efe9c66a4faa4fa509f9f62cc6bad8392128'` | `:50` — `owner_identity=$(ps -o lstart= -p "$owner_pid" 2>/dev/null \| sed 's/^[[:space:]]*//' \|\| true)`; `:51-53` set `unknown` when empty. `:97-106` (`owner_is_stale`): same host, numeric pid, `if ! kill -0 "$observed_pid" 2>/dev/null; then return 0`, then `[[ "$observed_identity" != "unknown" && -n "$live_identity" && "$live_identity" != "$observed_identity" ]]`. The same write and the same rule as SemEngine's runner |
| P9 (macOS) | `ls -ld /private/tmp` | `drwxrwxrwt ... root wheel`: sticky, so a user cannot rename another user's entry. The reviewer measured the same refusal on ubuntu:24.04 (review round 1): `mv` of another user's directory in `/tmp` printed `Operation not permitted` and exited 1 |

Not measured: ubuntu-24.04's procps version and its output beyond P6; locale sensitivity on Linux; a pid actually
reused by the kernel; a wall-clock step on either system.

## 2. Every current spelling of the fact

The fact: the process that wrote the lock's owner record is still running.

| Home | Pin | What it decides |
| --- | --- | --- |
| Writer | `scripts/test-integration.sh:101-102`, `:199` | records `identity`: ps's start time, leading blanks removed, or `unknown` |
| Runner's judge | `scripts/test-integration.sh:167-174` (`owner_is_stale`) | stale: same host, numeric pid, and `kill -0` fails or a read start time differs from a known identity |
| Admission's judge | `internal/harness/natsfixture/admission.go:71-91` (`ownerLive`) | live: same host, positive pid, `kill(pid, 0)` succeeds or fails with EPERM |
| Runner's spec | `openspec/specs/integration-test-runner/spec.md:12` — `(1–3600) is set; SHALL quarantine only a same-host owner whose pid is dead or whose start identity changed; SHALL`; scenarios `:20-28` | "start identity" is not defined anywhere in a spec (`git grep -n -i 'identity' -- openspec/specs/integration-test-runner openspec/specs/nats-fixture`: `:10`, `:12`, `:27` only) |
| Admission's spec | `openspec/specs/nats-fixture/spec.md:11-14` | live by host and pid (claim C) |
| Runner's test oracle | `internal/harness/runner/runner_test.go:249-271` (`startIdentity`, `psStartIdentity`, `trimIdentity`); R1 `:340` — `h.writeOwner(t, hostname(t), os.Getpid(), startIdentity(t, os.Getpid()))`; R2 `:388` — `h.writeOwner(t, hostname(t), os.Getpid(), "Mon Jan  1 00:00:00 1990")` | the Go exec read, kept independent of the script it tests; `:250-251` records macOS's trailing blanks |
| prochost's test oracle | `internal/harness/prochost/prochost_test.go:307-314` (`startIdentity`) | the same read, for another fact: that a test's helper process is gone |
| Admission's planted records | `internal/harness/natsfixture/admission_test.go:37` and `:149` — `host=%s\npid=...\nstarted=1\nidentity=x\ntoken=...` | every planted owner records `identity=x`, which no ps read prints |
| Ledger | `docs/admission-ledger.yaml:206` — `bash, ps, mkdir, docker CLI; creates the lock dir; may quarantine stale dirs; pulls images` | ps is a declared dependency of the runner |
| `task doctor`'s report | `scripts/doctor.sh:107` — `# The shared admission lock, judged the way the runner would judge it.`; `:114` — `if [ "$o_host" != "$(hostname)" ]; then`; `:116` — `elif [[ "$o_pid" =~ ^[0-9]+$ ]] && ! kill -0 "$o_pid" 2>/dev/null; then`; `:117` — `warn admission "stale (dead pid; the next runner quarantines it): $owner"`; `:118` — `else ok admission "busy: $owner"; fi` | a third judge: "stale" from host and `kill -0` alone; it never reads `identity`, so the comment at `:107` is false for a reused pid. No test runs doctor (`git grep -n 'doctor' -- internal/harness`: no match) |

How the three judges differ, read from the pins above and P4:

| Owner record | Runner (`owner_is_stale`) | Admission (`ownerLive`) | Doctor (`doctor.sh:110-118`) |
| --- | --- | --- | --- |
| same host, pid names no process | stale (`:170`) | refused (`:88`) | "stale (dead pid ...)" (`:116-117`) |
| same host, pid names a process with another start time | stale (`:173`) | **admitted** (#124) | "busy" (`:118`) |
| same host, running pid, identity `unknown` | respected (`:173`) | admitted | "busy" (`:118`) |
| same host, pid names another user's process | stale: `kill -0` exits 1 on EPERM (P4) | admitted (`:87-88`) | "stale (dead pid ...)" (`:116-117`) |
| another host | respected (`:168`) | refused (`:80-81`) | "held from another host" (`:114-115`) |
| pid not a number | respected (`:169`) | refused (`:84-85`) | "busy" (`:118`) |

Measured besides (P2): two readers whose time zone or locale differ read different texts for one process. The
runner's own identity and every later read of it are made in the reader's environment. The test binary inherits the
runner's environment through `go test`; no test sets `TZ`, `LC_ALL`, `LC_TIME` or `LANG`
(`git grep -n -E 'Setenv\("(TZ|LC_ALL|LANG|LC_TIME)"' -- '*.go'`: no match).

## 3. Adjacent claims on the territory

- **Specs:** `git grep -n -l -i 'owner record\|owner file\|lock owner' -- openspec/specs` → `integration-test-runner`
  and `nats-fixture` only.
- **ADRs:** `git grep -n -i 'lstart\|start identity\|admission lock\|owner record' -- docs/adr`: no match.
- **Active changes:** `ls openspec/changes` at the base → `archive` only.
- **Archive:** `openspec/changes/archive/2026-09-30-setup-02-isolated-harness/design.md:131-143` (the lock and the
  Go-side refusal; `:138` "quarantines only a provably dead same-host holder"); `:321` — invariant I1, "A fixture never
  starts a container without an admission token observed in the live lock owner file". `git log -S'func ownerLive'`
  and `git log -S'its pid is a running process'` → both `34c9dc6` (#13, SETUP 02).
- **Ledger:** the runner row, `docs/admission-ledger.yaml:196-212`, disposition `adapt`, contract `:202-203`
  "byte-compatible lock protocol at /tmp/semstreams-integration.lock". The `natsfixture` row (`:31-62`) is the
  `natsclient/test_client.go` pattern; admission is not in it
  (`git grep -n -i 'ownerLive\|owner file\|owner record\|admission token' -- docs/admission-ledger.yaml`: no match).
- **Issues:** `gh issue list --state all --search 'ownerLive OR lstart OR "reused pid" OR "start identity" OR
  "pid reuse"'` → #124 (open), #50 (closed by #123). Filed during review round 1: #126, "test-integration: the lock
  owner's start-time identity depends on the reader's time zone and locale" (open).
- **#123's record:** Codex's review (#123 comment 6046177843): "fixture admission does not consume that identity.
  Issue #124 describes that pre-existing contract difference accurately."
- **Rules index:** `AGENTS.md`, the row "A failure path fails closed; a skip, drop or degrade is declared" (review
  only); the test-text rows (no `time.Sleep`, no skips, no fixed addresses) bind the new tests.
- **Docs:** `docs/repository-map.md:37` (the `natsfixture` row), `:47` (the runner), `:55` (the
  `integration-test-runner` and `nats-fixture` specs row), `:69` (the latest archive row, the shape for a new one).
- **Open pull requests** (`gh pr list --state open --json number,title,isDraft,changedFiles,files`; #93 read in full
  with `gh api --paginate repos/C360Studio/semengine/pulls/93/files`; no open pull request changes
  `scripts/doctor.sh`):

| PR | Files | Overlap |
| --- | --- | --- |
| #93 (draft) setup-04a-02 ingest kernel, head `cd4707e` | 167 | Same Go package: `internal/harness/natsfixture/open.go`, `open_test.go`, `fixture_integration_test.go`. Same capability: its delta `openspec/changes/setup-04a-02-ingest-kernel/specs/nats-fixture/spec.md` ADDs "Connected value for a package's tests". None of `admission.go`, `admission_test.go`, `fixture.go`, an `integration-test-runner` delta, `internal/harness/runner/`, `internal/harness/prochost/`, `scripts/doctor.sh` or `docs/repository-map.md` is in its list (rechecked at `cd4707e`). Its three files call `Start` under the real runner only: read at `cd4707e` with `gh api .../contents/<path>?ref=cd4707e`, none has `plantLock`, `admit(`, `identity` or `Setenv`, and its `fixture.go` still calls `admit()` at `:129` |
| #116, #118, #119, #120 (Dependabot, Go modules) | 2 each | `go.mod`, `go.sum`: none |
| #117 (Dependabot) `@fission-ai/openspec` 1.13.2 → 1.14.0 | 2 | `package.json`, `package-lock.json`: no shared file; it changes the CLI `task spec:check` runs |
| #125 (draft) | 0 | this claim |

## 4. The consumer at birth

No exported symbol, flag, environment variable, owner-record key, file or message format is added. Candidate
unexported changes, each with its one caller: a `context.Context` parameter on `admit` (caller `fixture.go:129`) and
on `ownerLive` (caller `admission.go:65`), and an unexported start-time reader in `admission.go` (caller `ownerLive`).

## 5. The problem shape

Shape: admit or refuse at a seam on proof of a fact about another process, its identity by pid and start time.
Closest instance: the runner's own judge of the same fact, `scripts/test-integration.sh:167-174`, which reads with
`ps -o lstart=` and is the writer's own spelling. Others:

- `internal/harness/prochost/prochost.go:184-186` — `// Alive reports whether the helper has not yet exited. It reads
  the host's own record of the` / `// child, not the pid, so a reused pid cannot make it true.`: answers liveness from
  the parent's wait. Not available to admission: the test binary is the runner's descendant, not its parent.
- `internal/harness/prochost/prochost.go:219-225` — `func psState(pid int) (string, error) {` /
  `out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()`: the one non-test ps reader, of
  another field, without a context.
- Non-test `exec` calls under `internal/harness` take a context:
  `internal/harness/pindiff/pin.go:34` and `internal/harness/mutcheck/runs.go:38` use `exec.CommandContext`;
  `prochost.go:97` and `:220` do not.

Not triggered, with the reason:

- **Collision table:** no durable, communication or runtime-coordination primitive is proposed; this change alters
  one reader of an existing one. The lock's writer and both readers are in § 2.
- **Adoption sweep:** no primitive meant for reuse is established; the reader stays unexported in one package.
- **Intent check:** no boundary is set or moved.
- **Extraction slices:** nothing is ported; the runner row's lock protocol is not changed.
- **Context retention:** `git grep -n 'context' -- internal/harness/natsfixture/admission.go`: no match today. No
  struct gains a field.

## Adopter seam inventory

Two surfaces are reached from outside the code being changed: the lock directory, which SemStreams' runner shares
(ledger `:202-203`), and the refusal a developer reads when a direct `go test` is not admitted.

- **SemStreams' runner.** It reads and writes the same owner record. This change alters neither the record nor
  SemEngine's runner, so it needs to know nothing, and doing nothing changes nothing for it.
- **A developer who runs `go test` on a fixture package directly**, in a shell that still exports a dead run's
  `SEMENGINE_DOCKER_ADMISSION_TOKEN` and `SEMENGINE_DOCKER_ADMISSION_LOCK_DIR`:
  1. What must they know? Today: that a stale export can admit them when the dead runner's pid has been reused (#124's
     sequence); nothing tells them.
  2. If they do nothing: in that window their tests start containers outside the lock, and the next runner
     quarantines the lock and runs Docker work beside them. Silent.
  3. Where do they find out? Nowhere, until two runs collide.
  4. What should they have to know? Nothing: a refusal naming `task test:integration`, as the other refusals give
     (`admission_test.go:85`). That gap is this change.
- **A test author in this repository who plants an owner record** (`admission_test.go:37`, `:149`): the planted
  `identity=x` admits today and would not after a start-time check. They find out from the failing test.
