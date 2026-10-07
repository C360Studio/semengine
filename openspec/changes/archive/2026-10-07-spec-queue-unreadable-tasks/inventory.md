# Inventory: spec-queue-unreadable-tasks

- base: `8b60c4d095d3457af732cb572d0cfc322c7b4d5b`, the empty claim commit on `claude/spec-queue-unreadable-tasks`
  over `origin/main` `7353587db98f658a946581666c3c8bf4dda61d35`, so every pin below holds on both.
- revision 2, measured 2026-10-07; issue #115, claim PR #122. Revision 1 passed inventory review in round 1;
  revision 2 adds the help-text pin (I1) and #93's current file count and the `docs/repository-map.md` filter (I2).
- repositories read: this one only; the question needs no sister repository.
- probes: run on copies of the tree outside the worktree (`git archive HEAD`), with the pinned OpenSpec CLI 1.13.2.
  Pinned texts are trimmed of leading whitespace.

Question: what makes `task spec:queue` and `task spec:check` exit 2 on a `tasks.md` they cannot read, what holds
that today, which spec is its home, and what else claims the same files?

## 1. The claimed gap

**Claim A (#115): the script exits 2 when an in-flight change's `tasks.md` cannot be read.** Measured true.

| Pin | Text |
| --- | --- |
| `scripts/openspec-queue.sh:121` | `with open(sys.argv[1], encoding="utf-8") as f:` (inside `scan_tasks`, `:114`) |
| `scripts/openspec-queue.sh:202` | `scan=$(scan_tasks "$tasks_file") \|\| { echo "queue unavailable: cannot read $tasks_file" >&2; exit 2; }` (`--check`) |
| `scripts/openspec-queue.sh:249` | the same line, in the queue's loop |
| `scripts/openspec-queue.sh:209` | `done <<< "$scan"`; `:210` `done <<< "$rows"`; `:279` and `:285` the same for the queue: the loops are not subshells, so `exit 2` ends the script |
| `scripts/openspec-queue.sh:241` | `printf '  %-42s %s/%s%s\n' "$name" "$done" "$total" "$age_note"`: the queue prints a change's count line before it reads its `tasks.md` (`:249`) |

Probes (scratch copy; stand-in `openspec` for P1 and P2, the pinned CLI for P3):

| Probe | Input | Result |
| --- | --- | --- |
| P1 | change `fx` whose `tasks.md` contains the bytes `0xff 0xfe`, listed after a readable change `aa` | queue, and queue `--strict`: exit 2; standard output has `aa`'s count line and its `ok` line, then `fx`'s count line, and no `ok` line for `fx`. `--check`: exit 2, nothing on standard output. In all three, standard error is a Python traceback ending `UnicodeDecodeError: 'utf-8' codec can't decode byte 0xff ...`, then the line `queue unavailable: cannot read openspec/changes/fx/tasks.md` |
| P2 | the same `tasks.md`, readable text, mode 000, run as uid 501 | queue and `--check`: exit 2; the traceback ends `PermissionError: [Errno 13] Permission denied`, then the same line |
| P3 | a change whose `tasks.md` is not UTF-8, the real `openspec list --json` (1.13.2) | exit 0; the change is listed with `"totalTasks": 1`. The CLI reads the file without failing, so in real use the script's own read is the one that fails |

**Claim B (#115): no test holds it.** Measured true.

- `git grep -n -E '\\x[89a-fA-F][0-9a-fA-F]|chmod|0o000|0o200|Chmod' -- internal/harness/contract/specqueue_test.go`:
  no match. Every planted `tasks.md` is valid UTF-8 and readable.
- `internal/harness/contract/specqueue_test.go:60` — `if err := os.WriteFile(filepath.Join(change, "tasks.md"),
  []byte(tasks), 0o644); err != nil {`: the helper writes the case's string as bytes, so a Go string with invalid
  UTF-8 can be planted without a new helper.
- `internal/harness/contract/specqueue_test.go:102` — `if r.status != 0 {` (in `requireReported`, `:100`): every
  table case of `TestSpecQueueHolds` and `TestSpecQueueFirstLineCaveats` requires exit 0.
- `internal/harness/contract/specqueue_test.go:346` — `{"C7 the change list exits non-zero", listFails},` and `:347`
  `{"C7 the change list is not JSON", listNotJSON},`: the only exit-2 cases plant a bad change list, not a bad file.
- `internal/harness/contract/specqueue_test.go:351` — `if r.status != 2 || !strings.HasPrefix(r.stderr, "queue
  unavailable:") || strings.Contains(r.stdout, "holds: ok") {`: C7 checks standard error by prefix.

**Claim C (the brief): no file under `openspec/specs/` describes the queue.** Measured false at the base.

- `git log --oneline --diff-filter=A -- openspec/specs/spec-queue/spec.md` → `7353587 feat(spec): the queue shows a
  hold on any line of an open task; ...` (#113). The primary checkout's HEAD is `00b4a81`, which predates it; a
  search there finds nothing.
- `openspec/specs/spec-queue/spec.md:8` — `` `--strict`, exit 2 means it could not read the changes, and neither `task
  verify` nor CI runs it. Run with `--check`, ``: the Purpose, in general terms.
- `openspec/specs/spec-queue/spec.md:103-104` — `` queue SHALL print `ok` and `no halt/hold/deliberate marker in the
  open tasks`. Without `--strict`, the queue SHALL exit ``: the `ok` line, stated for "a change [with] no reported
  line".
- `openspec/specs/spec-queue/spec.md:162-164` — `` It SHALL then exit 1. When there is no such line it SHALL print
  `holds: ok (<n> tasks.md read)` and exit 0. When the list of in-flight changes cannot be read, ``: `--check`'s exit
  2 is stated for the change list only.
- `openspec/specs/spec-queue/spec.md:190` — `#### Scenario: The change list cannot be read`.
- `git grep -n -i -E 'cannot read|UTF-8|utf8|unreadable|not valid' -- openspec/specs/spec-queue/spec.md`: no match.
  No requirement or scenario names a `tasks.md` that cannot be read.

## 2. Every current spelling of the fact

The fact: a change's `tasks.md` was not read, so the script cannot vouch for it.

- `scripts/openspec-queue.sh:202` and `:249`: two copies of one branch, one per mode, with the same message. Both
  call the one reader, `scan_tasks` (`:114`). `scan=$(...)` runs the reader in a command substitution, a subshell,
  so an `exit` inside the reader would not end the script; each caller holds its own `exit 2`.
- `scripts/openspec-queue.sh:64` — `-h|--help) sed -n '2,51p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;`: lines 2 to 51
  are the script's `--help` text, not only a comment.
- `scripts/openspec-queue.sh:41` — `# pre-archive hook). Exit 2 means the queue could not be read.` (help text)
- `scripts/openspec-queue.sh:47` — `# exits 0. An unreadable change list exits 2, as for the queue.`: the `--check`
  paragraph of the help text names the change list only.
- `openspec/specs/spec-queue/spec.md:8`: the Purpose (above).
- Adjacent, a different fact (the file is absent, not unreadable): `scripts/openspec-queue.sh:201` — `[ -f
  "$tasks_file" ] || continue` (`--check` skips it) and `:244-245` — `if [ ! -f "$tasks_file" ]; then` /
  `printf '      (no tasks.md)\n\n'` (the queue notes it). No test plants a change without a `tasks.md`.
- Another reader of the same file: the `openspec` CLI, which reads it to count tasks and does not fail on invalid
  UTF-8 (P3).

## 3. Adjacent claims on the territory

- **Specs:** `git grep -n -l 'openspec-queue' -- openspec/specs` → `openspec/specs/spec-queue/spec.md` only.
- **ADRs:** `git grep -n -i -l 'spec:queue\|openspec-queue' -- 'docs/adr*'`: no match.
- **Active changes:** `ls openspec/changes` at the base → `archive` only.
- **Admission ledger:** `git grep -n 'openspec-queue' -- docs/admission-ledger.yaml docs/provenance.md`: no match.
  The script is repository tooling, not a ported package.
- **Rules index:** `AGENTS.md:107`, the row "Every OpenSpec task can be ticked in or before the archive commit; a
  hold is written as `Hold:` ...", names `TestSpecQueueCheck`, `TestSpecCheckWiring` and `TestSpecQueueHolds`.
- **Issues:** open issues whose title matches `queue|spec:check|openspec-queue|Hold:|tasks.md`, and a search for
  `openspec-queue.sh`: #115 only.
- **#113's record:** Codex's review names the untested branch as a limit (#113 comment 6041276484); the
  implementer's notes disclose it as behaviour design D3 does not list (comment 6041144906, item 1).
- **Open pull requests** (`gh pr list --state open --json number,title,isDraft,changedFiles,files`, filtered for
  `openspec-queue|specqueue|spec-queue|Taskfile.yml|docs/repository-map.md` and for deltas under
  `openspec/changes/*/specs/`; no open pull request touches `docs/repository-map.md`, which task 4.1 edits):

| PR | Files | Overlap |
| --- | --- | --- |
| #93 (draft) setup-04a-02 ingest kernel | 124, read in full with `gh api --paginate .../pulls/93/files` (its 18:26Z commits added `reservedsubjects_test.go`) | No shared file and no `spec-queue` delta. Its deltas: `component-registration`, `graph-entity-writes`, `graph-ingest-recovery`, `graph-transport-boundary`, `harness-boundaries`, `lifecycle-suite`, `metric-registry`, `nats-fixture`, `projection-mutation`. It changes five other files of the Go package `internal/harness/contract` (`boundaries_test.go`, `graphtransport_test.go`, `imagepin_test.go`, `promglobal_test.go`, `reservedsubjects_test.go`) |
| #117 bump `@fission-ai/openspec` 1.13.2 → 1.14.0 | `package.json`, `package-lock.json` | No shared file. It changes the CLI that `task spec:check` validates this change with, and that the script calls for `list --json` |
| #121 (draft) the name check reports a generic-promoted member once | 0 so far | Topic is the Go package `internal/harness/contract`; no shared file named |
| #116, #118, #119, #120 (dependency bumps), #123 (draft, natsfixture) | 2, 2, 2, 2, 0 | None |
| #122 (draft) | 0 | This claim |

## 4. The consumer at birth

The change adds no exported symbol, port, subject, bucket, config field, flag, message or exit status: the message
and the exit 2 exist at `scripts/openspec-queue.sh:202` and `:249`. It adds test cases and spec text only. Nothing
to name.

## 5. The problem shape

Shape: fail closed on an input the tool cannot read, as a classified refusal (exit 2 and a `queue unavailable:`
line), held by a test that plants the bad input. Closest instance: C7,
`internal/harness/contract/specqueue_test.go:345-356`, with its scenario at `openspec/specs/spec-queue/spec.md:190`:
same script, same exit status, same message prefix, same "no `holds: ok`" check. One measured difference: C7 checks
standard error by prefix (`:351`), and P1 shows a traceback before the `queue unavailable:` line for this input.

Not triggered, with the reason:

- **Adoption sweep:** the change adopts C7's shape; it establishes no primitive.
- **Intent check:** no boundary is set or moved.
- **Collision table:** no durable, communication or runtime-coordination primitive is proposed.
- **Extraction slices:** nothing is ported; no ledger row (above).
- **Context retention:** no production Go code is touched.

## Adopter seam inventory

Not triggered: the change adds or changes no surface, and the script is not reached from outside this repository.
`git grep -n -l 'openspec-queue' -- ':!openspec/changes/archive'` → `AGENTS.md`, `Taskfile.yml`, `docs/testing.md`,
`internal/harness/contract/specqueue_test.go`, `openspec/specs/spec-queue/spec.md`, `scripts/doctor.sh`,
`scripts/openspec-queue.sh`; its callers are `Taskfile.yml:114` (`spec:check`) and `:119` (`spec:queue`), run by
agents and the owner in this repository.

For the in-repository reader, as the four questions would put it: they need to know nothing; if they do nothing,
the command exits 2 and names the file it could not read; they find out from the exit status and standard error
(a typed runtime refusal); that is what they should have to know. The gap #115 names is not on this side: it is
that no test fails if the refusal is later turned into a skip.
