# Design: spec-queue-unreadable-tasks

Status: revision 2, draft. It rests on `inventory.md`, which passed its independent review in round 1 (`INVENTORY
PASS`; revision 2 of the inventory adds the help-text pin and #93's current file count). Round 1 asked for changes to
this design; revision 2 answers them: the precedence over exit statuses (spec delta), the help-text line (D4), the
`docs/repository-map.md` amendment (task 4.1), and the departure from #115 named for the owner ("Owner decision").
Probes P1 to P3 and pins are the inventory's; P4 and P5 are measured below.

## Context

`scripts/openspec-queue.sh` exits 2, with `queue unavailable: cannot read <path>` on standard error, when an
in-flight change's `tasks.md` cannot be opened or is not valid UTF-8, in both modes (inventory § 1, claim A, P1, P2).
No test plants such a file (claim B), and the `spec-queue` spec states exit 2 only for an unreadable change list
(claim C). If the refusal were turned into a skip, every gate would stay green and the queue could again print `ok`
for a change it never read: the state #76 closed.

## Options

1. **An ADDED requirement in `spec-queue`** (recommended, D1). Cost: one requirement and two scenarios appended to the
   spec; the `ok` and `holds: ok` lines and the exit 0 and exit 1 of the first three requirements stay as written,
   so the new requirement says it takes precedence over them.
2. **MODIFIED requirements, as #115 suggests:** a sentence and a scenario added to "A hold on any line of an open
   task" and to "A hold outside every task fails spec:check". Cost: a MODIFIED requirement replaces the whole block
   when archived, so the delta re-copies both, `spec.md:14-88` and `:155-200` (121 lines, 16 scenarios), and any word
   that drifts in the copy changes current truth. The first requirement is about which holds the queue shows; a read
   failure is not a hold. The `ok` line it would have to override is stated in the second requirement (`:103-104`),
   which this option leaves untouched.
3. **A new capability.** Cost: one script's behaviour split across two specs, when `spec-queue` already owns it.
4. **Do nothing.** Cost: the wrong changes in D3 pass `task verify` (measured below: each exits 0 on the fixture).

## Decisions

### D1. The requirement's home

An ADDED requirement in `spec-queue`, "A tasks.md that cannot be read stops the queue and the check", with one
scenario per mode. `spec-queue` is the only spec that describes this script (inventory § 3); no open pull request has
a delta for it (#93's nine deltas are listed in inventory § 3); and an ADDED requirement states both modes once
without re-copying text that is already current truth. The archive appends it after the third requirement.

### D2. The two test cases

Both plant, with `listOneChange`, a `tasks.md` whose open task contains the bytes `0xff 0xfe` (a Go string literal
`"\xff\xfe"` passed to `runSpecQueue`, which writes it as bytes, `specqueue_test.go:60`). Both expect, written by hand
from the requirement:

- **`H12 a tasks.md that is not UTF-8`**, in `TestSpecQueueHolds`, as its own `t.Run` beside H11, because the table's
  `requireReported` requires exit 0 (`:102`). Run without arguments. Exit 2; standard error, split into lines, has the
  line `queue unavailable: cannot read openspec/changes/fx/tasks.md`; no line of standard output has `ok` as its first
  word.
- **`C8 a tasks.md that is not UTF-8`**, in `TestSpecQueueCheck`, as its own `t.Run` after C7. Run with `--check`.
  Exit 2; the same standard-error line; standard output has no `holds: ok`. Not added to C7's loop: C7 checks
  standard error by prefix (`:351`), and P1 shows a traceback before the line.

Each case's comment names the requirement, `spec-queue › "A tasks.md that cannot be read stops the queue and the
check"`, as `:282` names its own; the file comment at `:18`, "all three requirements", becomes "all four".

Invalid UTF-8, not mode 000: a process running as root reads a mode-000 file (#115), so that fixture would pass or
fail by the runner's user. P2 shows mode 000 reaches the same line as uid 501.

### D3. Shown able to fail, by hand

`task mutate:check` takes only a Go source file, so the wrong changes are made by hand (`docs/testing.md`, "Run it by
hand"): a copy and checksum before, the wrong change, the restore and checksum after, all three runs recorded. Measured
on a scratch copy of the base with the D2 fixture:

| Wrong change | Queue result | `--check` result | Must fail |
| --- | --- | --- | --- |
| M1a: `:249`'s `\|\| { echo ...; exit 2; }` becomes `\|\| continue` | exit 0, no `queue unavailable:` line, no `ok` line | (not reached) | H12, by exit status and the missing line |
| M1b: `:202`'s branch becomes `\|\| continue` | (not reached) | exit 0, `holds: ok (0 tasks.md read)` | C8 |
| M2: `:121` opens with `errors="replace"` | exit 0, an `ok` line | exit 0, `holds: ok (1 tasks.md read)` | H12 and C8 |

The probe applied M1 to both sites at once; the developer runs M1a and M1b apart, so each case is shown to catch its
own site. Under M1a the queue prints no `ok` line either, so that assertion alone would let M1a survive; exit status
and the standard-error line catch it.

### D4. The help text

`scripts/openspec-queue.sh:64` prints lines 2 to 51 for `-h` and `--help`, so `:47` is help text a user reads, and it
names only the change list as a cause of `--check`'s exit 2. It becomes, in place and still one line:

```text
# exits 0. An unreadable change list or tasks.md exits 2, as for the queue.
```

One line for one, so the pins `:121`, `:202` and `:249` that D3 names do not move, and `:64`'s range still ends at
the help text's last line. No behaviour changes.

## Invariant

For every run in which an in-flight change's `tasks.md` cannot be read: exit 2, the `queue unavailable: cannot read`
line on standard error, and no `ok` or `holds: ok` for that change. Its spec home is the ADDED requirement. Examples
suffice (`docs/testing.md`, "Decide whether generated checks are needed"): there is one branch per mode, both causes
reach the same line (P1, P2), and no input interacts with another.

## Not in this change

Each is measured and left unheld by a test:

- `--strict`: P1 measured exit 2; the requirement's exit 2 has no condition, but no case runs it.
- A mode-000 file (P2).
- More than one change, and the precedence over exit 1. Measured on a scratch copy of the base, with change `aa`
  listed before a non-UTF-8 `fx`: P1, `aa`'s lines stay printed; P4, `aa` holds a task and the queue runs with
  `--strict`: one `BLOCKED` line, then exit 2; P5, `aa` has a `Hold:` after its task list and `--check` runs:
  `openspec/changes/aa/tasks.md:7: Hold: outside every task; ...`, then exit 2. The stand-in lists one change, so
  no case holds these.
- A change with no `tasks.md` (inventory § 2): a different fact; `--check` skips it and the queue notes it.
- The message is written twice, at `:202` and `:249`. The reader runs in a subshell, so each caller keeps its own
  `exit 2`; H12 and C8 hold the two sites apart. Not consolidated.
- The Purpose (`spec.md:8`, "exit 2 means it could not read the changes") already covers a `tasks.md`; unchanged.
- `AGENTS.md:107`: no change. No rule of conduct is added, and the test functions it names keep their names.
- #115's own exclusions: `AGENTS.md:85` and the trailing space in a printed hold.

## Overlaps and order

- #93: no shared file and no `spec-queue` delta; it changes five other files of `internal/harness/contract`. Neither
  waits; the later one merges `origin/main`.
- `docs/repository-map.md`, which task 4.1 edits: no open pull request touches it (inventory § 3).
- #117 bumps the OpenSpec CLI from 1.13.2 to 1.14.0. This delta was validated with 1.13.2. Neither waits; whichever
  merges second runs `task spec:check` on the other's tree in its CI.
- #121 (draft, no files yet) works in the same Go package. Neither waits.

## Owner decision

The design departs from #115's recommendation 2, which adds a scenario to requirements 1 and 3 of `spec-queue` (a
MODIFIED delta), and adds one requirement instead (Options, 2; D1). The owner accepts or rejects that choice in task
1.3. Otherwise the change adds no surface and no rule.
