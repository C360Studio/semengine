# Inventory: #76, a `Hold:` below an open task's first line is read by no command

base: 8abb3830614adda6d0d12a769967b92caea0837f

- Branch `claude/spec-hold-check`, draft PR #113. The base is `origin/main` at `00b4a81` (#112) plus an empty claim
  commit. Every `path:line` below is at the base unless a commit is named.
- This is the inventory only (architect contract, Required workflow step 2). It holds no target state, options,
  recommendation or tasks.
- **Question:** where does SemEngine read the hold on an open OpenSpec task? Which commands could fail on a hold the
  queue cannot see, and what pins each one?
- **Repositories read** (`docs/inventory-scope.md`): this one; SemStreams at the pin `8b99efe9` for one code fact
  (does the pin carry a fixture test for the queue script; the `semstreams` row allows "code facts only from the
  `8b99efe9` snapshot with path:line"); the owner's global Claude skills, for the adopter seam only.
- **Terms.** A *first line* is a task's checkbox line (`- [ ] N.N ...`). A *continuation line* is any following line
  indented under it, a nested plain bullet included. A *block* is the first line plus its continuation lines.

## Tools as measured

- The OpenSpec CLI is pinned at 1.13.2: `package.json:7` — `"@fission-ai/openspec": "1.13.2",`. All runs below use that
  version, installed with `npm ci --ignore-scripts` from this tree's `package.json` and `package-lock.json` into a
  scratch directory. `openspec --version` printed `1.13.2`, and the tarball's sha512 matched the lockfile's
  `integrity`. CI installs the same: `.github/workflows/ci.yml:41` — `- run: npm ci`, then `:56` —
  `- run: task verify`.
- The claim worktree has no `node_modules` (`ls node_modules` printed `No such file or directory`), so `task spec:check`
  and `task spec:queue` cannot run there before `npm ci`. The primary checkout's `node_modules` holds OpenSpec
  **1.7.0**, not the pin (`node_modules/.bin/openspec --version` printed `1.7.0`). The check that reports this is
  `scripts/doctor.sh:52` — `check_npm_tool @fission-ai/openspec openspec`. The two versions parse tasks differently
  (§2.3).
- CLI source is cited inside the published package (`@fission-ai/openspec@1.13.2`, `dist/...`) because
  `node_modules` is not tracked.

## 1. How the queue script gets task text (categories 1 and 2)

**Measured: the CLI hands the script no task text.** The script asks the CLI only for the list of changes and their
counts, then reads `tasks.md` itself with its own `grep`, one line per checkbox.

- `scripts/openspec-queue.sh:98` — `if json=$(openspec list --json 2>&1); then status=0; else status=$?; fi`. This is
  the only CLI call. `git grep -n 'openspec ' -- scripts/openspec-queue.sh` found nothing else: the other hits are the
  comment at `:5`, the presence check at `:55-56`, and messages at `:100`, `:111` and `:114`.
- `scripts/openspec-queue.sh:116` — `for c in d["changes"]:`. For each change it keeps `name`, `completedTasks`,
  `totalTasks` and `lastModified` (`:117-122`), and no task text.
- `scripts/openspec-queue.sh:152` — `tasks_file="$CHANGES_DIR/$name/tasks.md"`. The script opens `tasks.md` itself.
- `scripts/openspec-queue.sh:182` — `done < <(grep -nE '^[[:space:]]*- \[( |~)\]' "$tasks_file" 2>/dev/null)`. A task's
  text is the one line this grep matches, with its line number. A continuation line, a nested `- Hold:` bullet, a
  heading or a paragraph never matches, so none of them reaches `label_for`.
- `scripts/openspec-queue.sh:169` — `*'- [~]'*) marker="WONTDO" ;;`, then `:171` —
  `[ -z "$marker" ] && marker="$(label_for "$text")"`. Only that one line is classified.
- `label_for` runs from `scripts/openspec-queue.sh:78` — `label_for() {` to `:86` — `}`. The issue's `:79-87` is one
  line late: `:79` is `local t=$1` and `:87` is blank.
- `scripts/openspec-queue.sh:82` —
  `printf '%s' "$t" | grep -qiE '\bhold\b|\bblocked\b|\bblocking\b'    && { echo "BLOCKED"; return; }`. The queue
  spells a hold as the case-insensitive word `hold` (or `blocked`, `blocking`), not as the case-sensitive `Hold:` the
  rule names. The word list itself is out of scope (issue #76).
- The `ok` line is `scripts/openspec-queue.sh:185` —
  `printf '      %-8s no halt/hold/deliberate marker in the open tasks\n' "ok"`. The issue's `:185` is right.
- Purpose and exit status: `:3` — `# openspec-queue.sh — surface WHY each in-flight change is still open.`; `:75` —
  `# being read, which returns us to the invisible-caveat state this script`; `:31` —
  `# Exit status is advisory-by-default and deliberately so: this is a`; `:33` —
  `# --strict to exit non-zero when any caveat is found (for CI or a`. The issue's `:7-31` covers the start of the
  "three costs" text. The "invisible caveat" wording is at `:75`, and the advisory sentence at `:31`.
- `--strict` exists but nothing uses it: `:46` — `--strict) STRICT=1; shift ;;` and `:197` —
  `if [ "$STRICT" -eq 1 ] && [ "$found_any" -eq 1 ]; then`. `git grep -n -e '--strict' -- Taskfile.yml scripts/
  .github/ .agents/ AGENTS.md docs/` found only the script's own lines (`:33`, `:37`, `:46`) apart from
  `validate --all --strict`. It exits 1 whenever any caveat is shown, a correctly placed hold included (§3.1: exit 1
  at `50b4a0c` and `b0bb713`, exit 0 at `b91c60d`).
- The CLI's own per-task interface also yields one line per task. `openspec instructions apply --change
  authority-one-spelling --json` at `b91c60d` returned, for task 3.1, `` "description": "3.1 (D)
  `TestNoSecondAuthorityNameSensitivity`, written first, in `authority_test.go`: the fixture of" ``, the checkbox
  line only. The source is 1.13.2 `dist/utils/task-progress.js:62-71` (`parseTaskLines`, whose `description` is
  `match[2].trim()` of the matching line). So the issue's sentence "the OpenSpec CLI hands it the task's first line"
  is false of the script, where the script's own `grep` does that, and true of the CLI's task interface, which the
  script does not use.
- `openspec list` skips the archive: 1.13.2 `dist/core/list.js:102` —
  `// Get all directories in changes (excluding archive)`, `:105` —
  `.filter(entry => entry.isDirectory() && entry.name !== 'archive')`.
- Nothing in this repository tests the queue script: `git grep -n 'openspec-queue' -- internal/` found no lines.
  CI does not run it: `ci.yml` runs `task doctor` (`:52`) and `task verify` (`:56`), and `AGENTS.md:54` —
  `# tracked files changed. Not included: doctor, fmt, spec:queue, merge:check`.

## 2. The `tasks.md` shape a check would parse (category 2)

### 2.1 The archive

These commands, run from the repository root at the base, reproduce every number in the table. They read all 11 files
matching `openspec/changes/archive/*/tasks.md`.

```bash
set -- openspec/changes/archive/*/tasks.md
cat "$@" | grep -cE '^[[:space:]]*([-*+]|[0-9]+[.)])[[:space:]]+\[.?\]'   # checkbox lines: 296
cat "$@" | grep -cE '^- \[.?\]'                                            # '- [' at column 0: 296
cat "$@" | grep -cE '^- \[x\]'; cat "$@" | grep -cE '^- \[ \]'; cat "$@" | grep -cE '^- \[~\]'   # 295, 0, 1
cat "$@" | grep -cE '^[[:space:]]+([-*+]|[0-9]+[.)])[[:space:]]+\[.?\]'   # indented checkbox lines: 0
cat "$@" | grep -E '^ +[^ ]' | awk '{match($0,/^ */); print RLENGTH}' | sort -n | uniq -c   # indented lines by width
cat "$@" | grep -E '^ +[-*+] ' | grep -vE '^ +[-*+] \[.?\]' \
  | awk '{match($0,/^ */); print RLENGTH}' | sort -n | uniq -c             # nested plain bullets by width
cat "$@" | grep -cE '^[[:space:]]*(```|~~~)'                               # fence lines: 0
cat "$@" | grep -c "$(printf '\t')"; cat "$@" | grep -c "$(printf '\r')"   # tab lines, CR lines: 0, 0
awk 'FNR==1{p="x"} p=="" && /^ +[^ ]/ && !/^ +- \[/{n++} {p=$0} END{print n+0}' "$@"   # blank, then indented: 0
awk 'FNR==1{t=0} /^- \[.?\] /{t=1; next} /^ /{next} t && /^[^#-]/ && $0!=""{n++} {t=0} END{print n+0}' "$@"  # 0
cat "$@" | grep -oE '^- \[.?\] [^ ]+' | sed -E 's/^- \[.?\] //; s/[0-9]+/N/g' | sort | uniq -c   # id shapes
grep -nE '^- \[.?\] .*Hold:' "$@"                                          # Hold: on a first line: 8, all [x]
awk '/^- \[.?\] /{t=FNR; o=($0 !~ /^- \[[xX]\]/); next} /^[^ ]/{t=0}
     t && o && /Hold:/{print FILENAME":"t": hold at "FNR}' "$@"            # unticked holds below line 1: none
```

| Measure | Result |
| --- | --- |
| Checkbox lines | 296 |
| List marker `-` / `*` / `+` / numbered | 296 / 0 / 0 / 0 |
| Mark `[x]` / `[ ]` / `[~]` / other | 295 / 0 / 1 / 0 |
| Indented (nested) checkbox lines | 0 |
| Indented lines | 1708 under tasks: 1287 at 6 spaces, 373 at 8, 48 at 10. Seven more, at 2 spaces, are header prose bullets in `2026-10-01-flake-defense/tasks.md:10-19`, outside every task |
| Nested plain bullets (`- text` under a task) | 99, all in `2026-10-05-setup-04a-01-floor` (58 at 6 spaces, 41 at 8) |
| Fence lines (```` ``` ```` or `~~~`) | 0 |
| Tabs / CR characters | 0 / 0 |
| Blank line followed by an indented non-task line | 0 |
| Unindented text directly after a task line | 0 |
| Task id shapes | `N.N` 273; `N.N` plus one letter 22 (`N.Na` 5, `N.Nb` 14, `N.Nc`, `N.Nd`, `N.Ne` 1 each); `N.NcN` 1 (`3.7c2`) |

- The one partial task is `openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/tasks.md:126` —
  `- [~] 3.1 Every matrix row's SemSource "Observed" cell cites an observation name or states "not observed". Partly`.
  Its block has no `Hold:`.
- **Unticked tasks in the archive that carry `Hold:` anywhere: 0.** The archive has no `[ ]` task, and its one `[~]`
  task carries none.
- `Hold:` on ticked tasks: 8, every one on the first line straight after the task id. They are flake-defense 1.2,
  2.2, 2.3, 2.4 and 2.5 (`tasks.md:25`, `:32`, `:35`, `:38`, `:41`), setup-04a-foundation 1.2 (`:11`), and
  setup-04a-01-floor 3.0 (`:176`) and 7.1 (`:1109`).
- `Hold:` outside any task: 5, each in a header on line 3 (§6.4).

### 2.2 Active changes

- The base has no active change. `ls openspec/changes/` lists only `archive`. The base script run on the base's
  `openspec/` printed `(queue is empty)` and exited 0.
- The one active change in flight is PR #93's `openspec/changes/setup-04a-02-ingest-kernel/tasks.md`, re-pinned at
  #93's head `c758aaf5a0bcd77f3cc1b06fac20fafae43841ee` (295 lines). It has 57 checkbox lines, all `-` bullets at
  column 0: 38 `[ ]` and 19 `[x]`. Continuation lines sit at 6 spaces (201 lines), with no nested bullets, fences or
  tabs. **Of its 38 unticked tasks, 21 carry `Hold:`: 3 on the first line and 18 only on a continuation line** (§7).
  Since the earlier head `dbeb5fe8`, task 3.3a was ticked and every line from task 3.7 on moved down by 2. The 3/18
  split and the tasks in it are unchanged.

### 2.3 Three readers of "what is an open task"

- The queue script, `scripts/openspec-queue.sh:182` — `^[[:space:]]*- \[( |~)\]`: `-` bullets only, at any indent,
  `[ ]` and `[~]`.
- OpenSpec 1.13.2 (the pin, used in CI): `dist/utils/task-progress.js:53` —
  `const TASK_LINE_PATTERN = /^\s*(?:[-*+]|\d{1,9}[.)])\s*\[(?:\s*([^\]\s]?)\s*\](?![([])|\s+\])\s*(.*)/;`. It takes
  `-`, `*`, `+` and ordered markers, at any indent, with any one-character mark. Only `x` or `X` means done, so `[~]`
  counts as open.
- OpenSpec 1.7.0 (the primary checkout's stale install): `dist/utils/task-progress.js:5` —
  `const TASK_PATTERN = /^[-*]\s+\[[\sx]\]/i;`. Column 0 only; `[~]` is not counted at all.
- On the archive all three agree except on the one `[~]` line: 1.7.0 counts 295 tasks and 1.13.2 counts 296. On
  #93's `tasks.md` all three see the same 57 lines.

### 2.4 Where the queue, `spec:check` and `docs:check` look today

- The queue reads active changes only: `openspec list` skips the archive (§1), and the script then reads
  `openspec/changes/<name>/tasks.md` (`:152`).
- `spec:check` is `Taskfile.yml:113` — `- npx --no-install openspec validate --all --strict --no-interactive`. `--all`
  covers active changes and specs. The archive is a separate `--archived` scope, and the two cannot be combined (1.13.2
  `dist/commands/validate.js:41` — `? 'A validation report cannot combine archived and active scopes.'`). At the base
  it validated 11 items, the 11 specs: `Totals: 11 passed, 0 failed (11 items)`, exit 0.
- `docs:check` reads active **and** archived `tasks.md` (§4, item 2).
- 1.13.2's validator already reads `tasks.md` for two rules, neither about holds:
  - Task numbering (`dist/core/validation/task-numbering.js`): a task's leading number matches its `## N.` group,
    and no id repeats.
  - Plain bullets without a checkbox (`dist/core/validation/task-checkboxes.js`). This fires only when a change's
    whole tracked task set has no checkbox at all: `:81` —
    `* Reported only when the change's *whole* tracked set has zero checkboxes.`, and the early return at `:97` —
    `if (documents.some((document) => parseTaskLines(document.content).length > 0))`.

  Both rule lists are fixed inside the package.
- `openspec/config.yaml` has a `rules:` slot, commented out (`openspec/config.yaml:15` — `#   rules:`, `:19` —
  `#     tasks:`). In 1.13.2 the only readers of `rules` that `grep` finds are `dist/core/project-config.js` (parsing,
  `:35-41`) and `dist/core/artifact-graph/instruction-loader.js`, which puts the rules into AI instructions. No
  validator reads them.

### 2.5 Line length, and what the queue prints of a first line

- `.markdownlint.yaml:9` — `MD013:` and `:10` — `line_length: 120`; `.markdownlint-cli2.yaml:3` —
  `` # headings); its content is validated by `task spec:check`. Human-written openspec/changes/** stays linted. `` So a
  task's first line holds at most 120 characters.
- The queue prints at most 104 characters of a first line: `scripts/openspec-queue.sh:179` —
  `printf '      %-8s L%-5s %.104s\n' "$marker" "$lineno" "$clean"`. Here `clean` is the line with its
  leading list syntax (`- [ ]` and the spaces after it) removed, `**` removed and runs of whitespace squeezed
  (`:175-178`).
- In #93's `tasks.md` at `c758aaf5`, 43 of 57 first lines are 110 characters or longer. The three first-line holds
  sit late in their lines, and the queue never prints what any of them waits for:

  | Task | Line | Length | `Hold:` at column | Rest of the hold | Queue prints up to |
  | --- | --- | --- | --- | --- | --- |
  | 3.12b | L177 | 116 | 112 | `Hold:` ends the line; "task 3.12a." is on L178 | `... the tests of task 4.9.` (stops before `Hold:`) |
  | 4.6 | L203 | 114 | 105 | `Hold: task` ends the line; "3.12." is on L204 | `` ... (`component-registration`). Hold: `` (stops right after `Hold:`) |
  | 7.2 | L294 | 117 | 113 | `Hold:` ends the line; "task 7.1." is on L295 | `... the last content commit` (stops before `Hold:`) |

  The queue still labels all three `BLOCKED`, because `label_for` reads the whole line (§7). This bears on open
  question 2.

## 3. The two regressions as fixtures (category 1)

Every commit below is in the local object store (`git cat-file -e <sha>^{commit}` succeeded). Each is also on GitHub:
`gh api repos/C360Studio/semengine/commits/<sha>` returned the full sha for `50b4a0c`, `b91c60d`, `b0bb713`,
`e7cd3ae`, `a7dbf9d` and `c64ac338`. `claude/authority-one-spelling` is still on origin (`dd6edb99`).
`claude/setup-04a-01-floor` is not (`git ls-remote` returned nothing), so PR #48's branch commits survive by sha only
(PR #48's head was `b275663e`, merged as `deaafd48`).

Each run took the commit's `openspec/` with `git archive <sha> openspec` into a scratch directory, then ran the
**base's** script against it with the pinned CLI.

### 3.1 PR #73, task 3.1

File: `openspec/changes/authority-one-spelling/tasks.md` on `claude/authority-one-spelling`.

The branch order (`git log --reverse`, `git merge-base --is-ancestor`) is `50b4a0c` → `b91c60d` → `b0bb713`.

| Commit (committed 2026-10-03) | Where 3.1's `Hold:` sits | Queue, base script | `--strict` | `openspec validate --all --strict` |
| --- | --- | --- | --- | --- |
| `50b4a0c` 08:25:20 | first line, L53 | `BLOCKED  L53    3.1 Hold: until PR #48 merges ...` | exit 1 | not run |
| `b91c60d` 08:32:06 | continuation, L62 (first line L55) | `ok       no halt/hold/deliberate marker in the open tasks` | exit 0 | exit 0 |
| `b0bb713` 08:32:50 | first line, L55 | `BLOCKED  L55    3.1 Hold: until PR #48 merges ...` | exit 1 | not run |

The 18 later commits on the branch that touch this file, at its active or archived path, through `dd6edb9`, were
traced too. Every unticked `Hold:`
in them is on a first line. `b91c60d` is the only continuation-line hold in the branch's committed history.
`b0bb713`'s message reads: "The round-1 fix copy regressed the marker to a continuation line, which
`task spec:queue` does not see (the F7 class on #48)."

At `50b4a0c`, lines 53-61 (the hold is on the first line):

```markdown
- [ ] 3.1 Hold: until PR #48 merges — it carries `signatures_test.go`, and at its pushed head `c64ac338` the
      check names seven identifiers #48 is ruled to remove (design P2). (D)
      `TestNoSecondAuthorityNameSensitivity`, written first, in `authority_test.go`: the fixture of
      `design.md` D2 (the seven matching names across a public, an internal and a `main` package; the four
      non-matches: an unexported `federationMeta`, lowercase `federation`, a `_test.go` `TestFederation`, a comment
      and a string literal saying `BuildGlobalID`). Seen to fail first. Then `TestNoSecondAuthorityName` over the
      repository, as a second predicate over the public-signature loader (`signatures_test.go:212-258`): every
      exported object of every loaded module package — package scope, methods of named types, struct fields — whose
      name contains `Federation`, `GlobalID` or `EntityIRI`, with D2's failure line. Gate: `task test:unit`.
```

At `b91c60d`, lines 55-65 (the regression; the hold is on continuation lines 62-65):

```markdown
- [ ] 3.1 (D) `TestNoSecondAuthorityNameSensitivity`, written first, in `authority_test.go`: the fixture of
      `design.md` D2 (the seven matching names across a public, an internal and a `main` package; the four
      non-matches: an unexported `federationMeta`, lowercase `federation`, a `_test.go` `TestFederation`, a comment
      and a string literal saying `BuildGlobalID`). Seen to fail first. Then `TestNoSecondAuthorityName` over the
      repository, as a second predicate over the public-signature loader (`signatures_test.go:212-258`): every
      exported object of every loaded module package — package scope, methods of named types, struct fields — whose
      name contains `Federation`, `GlobalID` or `EntityIRI`, with D2's failure line. Gate: `task test:unit`.
      Hold: until PR #48 merges — it carries `signatures_test.go`, and at its pushed head `c64ac338` the check names
      seven identifiers #48 is ruled to remove (design P2). When the hold lifts, the `signatures_test.go` lines this
      file and `design.md` cite (`:212-258` at `c64ac338`) are re-cited at #48's merged head, where the load has
      moved into `loadModuleTypes`.
```

At `b0bb713`, lines 55-64 (the fix):

```markdown
- [ ] 3.1 Hold: until PR #48 merges — it carries `signatures_test.go`, and at its pushed head `c64ac338` the
      check names seven identifiers #48 is ruled to remove (design P2); those lines are re-cited at #48's merged
      head, where the load moved into `loadModuleTypes`.
      (D) `TestNoSecondAuthorityNameSensitivity`, written first, in `authority_test.go`: the fixture of
      `design.md` D2 (the seven matching names across a public, an internal and a `main` package; the four
      non-matches: an unexported `federationMeta`, lowercase `federation`, a `_test.go` `TestFederation`, a comment
      and a string literal saying `BuildGlobalID`). Seen to fail first. Then `TestNoSecondAuthorityName` over the
      repository, as a second predicate over the public-signature loader (`signatures_test.go:212-258`): every
      exported object of every loaded module package — package scope, methods of named types, struct fields — whose
      name contains `Federation`, `GlobalID` or `EntityIRI`, with D2's failure line. Gate: `task test:unit`.
```

The header the issue cites is at each of these commits on line 3:
`` Each task names the outcome and the gate that proves it. An unchecked task whose first line says `Hold:` is one ``.

### 3.2 PR #48, task 3.6

File: `openspec/changes/setup-04a-01-floor/tasks.md` on `claude/setup-04a-01-floor`.

- **The finding** is in comment 5968489295 (Codex review record, 2026-10-03T10:56:44Z, which reviewed
  `832bc556..a7dbf9d8`): "**F7 — MEDIUM `openspec/changes/setup-04a-01-floor/tasks.md:258` and
  `internal/cache/doc.go:265` — propagate task/path truth into the tool-visible places.** The message dependency is a
  plain nested `- Hold:` bullet, so `task spec:queue` shows only hold 7.1." The issue cites comment 5969256728, but
  that is the later re-check: "| F7 MEDIUM | **Resolved as originally reported.** ... Task 3.6's message dependency is
  complete and its hold is removed. The queue shows only hold 7.1. |".
- **Fixture commit:** `a7dbf9d8851ee5b6870048024e9fd38115d0c72f`, the commit F7 reviewed. Task 3.6 starts at L217,
  and the nested bullet is at L258-259. `e7cd3ae` introduced it (`` git log -S'- Hold: `message` waits' ``). By
  `c64ac338`, 3.6 is ticked and the bullet is gone.
- **Queue at `a7dbf9d`** (base script): one line, `BLOCKED  L381   7.1 Hold: independent change review. ...`, and
  nothing for 3.6. `--strict` exits 1 because of 7.1. `openspec validate --all --strict` exits 0.

At `a7dbf9d`, line 217 (the first line), then lines 241-259. Lines 219-240 are 22 continuation lines at 6 spaces
with no `Hold` and no nested bullet (`sed -n 219,240p | grep -c -E 'Hold|^\s+- '` printed 0):

```markdown
- [ ] 3.6 (D) `message`, `pkg/cache` (level 5), destinations by design D5 (#9 comment 5953295358, refining
      5952661571): `message` stays public at `message` (SemSource imports it), `pkg/cache` moves to `internal/cache`.
      [lines 219-240 elided]
      - `pkg/cache` done in 70fe194 (port, `RegisterOrGet`), b137ab3 (eviction callback removed), 5fc2b63 (sleep
        repair), c6d7108 (shapes 2 and 3), 420063f (generated check), ed40a15 (close-ordering example), e7cd3ae (ledger,
        design and task record) and the lint fix after it. Two of the 26 sleeps went with `TestEvictCallback`; the other
        24 are repaired, each on the row. The generated-check decision is in design D7. Failing first,
        implementer-reported, on a copy of the pin (PR #48): under seed `1790895004660367000`, 48 of 48 runs of
        `-count=5 -cpu 1` with 24 in parallel failed `TestCoalescingSet_EntityUpdateScenario`, and 11 of them hung in
        `BatchCleared`'s deferred `Close`; under seed `1790895074311951000`, 6 of 48 failed
        `TestAttack_ConcurrentAddRemove` (and 9 `TestCoalescingSet_CallbackFiresAfterWindow`).
        `TestCoalescingSet_ContextCancellation` did not fail in those 96 runs nor in 2,400 runs of it alone at 12 in
        parallel: not reproduced. After the repair, `task test:repeat -- ./internal/cache` passed on both seeds and on
        `1790972252212655000`, `1790972253482840000` and `1790972254702897000`, and 96 of 96 runs of the seeds at 24 in
        parallel passed.
      - Every `internal/cache` constructor that returns `Cache` with an error (`NewSimple`, `NewLRU`, `NewTTL`, the
        hybrid constructor, and `NewFromConfig` through them) returns a nil `Cache` on error, not a nil pointer
        inside a non-nil interface (`TestCacheConstructorsReturnNilCacheOnError`). Implementer-reported: it failed
        first for `NewSimple`, `NewLRU` and `NewFromConfig`'s simple and lru paths; the TTL and hybrid ones were
        fixed in c6d7108.
      - Hold: `message` waits for task 2.8. Its tests import `internal/semantictest`
        (`payload_test.go:10`, `triple_helpers_test.go:9` at the pin), and the harness copy is task 2.8's.
- [ ] 3.7 (D) `natsclient` (level 6): row `adapt`; `test_client.go` and `test_options.go` are not ported and their
```

Line 260 is the next task, 3.7, so the nested bullet is the last thing in 3.6's block.

## 4. Where a failing form could live, and what pins each place (category 3)

1. **`task spec:check`**, `Taskfile.yml:110` — `spec:check:` and `:113` — `- npx --no-install openspec validate --all
   --strict --no-interactive`. It is one command today.
   - Pinned by: nothing reads its `cmds`. `git grep -n 'spec:check' -- internal/` finds only a fixture string,
     `internal/harness/contract/mergegate_test.go:45` —
     `return []byte("#!/usr/bin/env bash\nsteps=(spec:check build\n  test:unit test:integration " + steps + ")\n")`.
   - Described at: `AGENTS.md:35` — `task spec:check   # strict OpenSpec validation`; `AGENTS.md:90` —
     `` | OpenSpec changes and specs are well formed | "Where state lives" below | `task spec:check` for document
     shape; truth against code is review only | ``; preflight `.agents/skills/semengine-preflight/SKILL.md:31` —
     ``| `task spec:check` | Validates all OpenSpec changes and specs strictly |``; and `SKILL.md:57-59` (below);
     `docs/setup-plan.md:242` — `` - `task verify` checks strict OpenSpec, formatting and module tidiness, ... ``.
   - It is the first step of `verify` (`scripts/verify.sh:12`). It runs `npx --no-install`, so it needs `npm ci`.
2. **`task docs:check`**, an existing `verify` step (second in `scripts/verify.sh:12`): `Taskfile.yml:120` —
   `docs:check:` and `:123` — `- npx --no-install markdownlint-cli2 "**/*.md"`.
   - **Reads** every Markdown file it does not ignore. The ignore list is `.markdownlint-cli2.yaml:4` — `ignores:`,
     `:5` — `- node_modules/**`, `:6` — `- "**/node_modules/**"`, `:7` — `- openspec/specs/**`, so it reads both
     active and archived `tasks.md`. Measured on a scratch copy of the base (`git archive 8abb383`), with one
     trailing-space line planted in `openspec/changes/archive/2026-10-07-runner-image-pin/tasks.md` and one in a new
     `openspec/changes/fx/tasks.md`. Running `markdownlint-cli2 "**/*.md"` there printed `Linting: 104 files` and
     `Summary: 2 issues in 2 files`, named `openspec/changes/archive/2026-10-07-runner-image-pin/tasks.md:58:9` and
     `openspec/changes/fx/tasks.md:4:22`, and exited 1.
   - **Rules** are in `.markdownlint.yaml` (MD013 at `:9-10`, among others). The pinned tool is 0.23.3:
     `package.json:8` — `"markdownlint-cli2": "0.23.3"`, and the installed package reports `0.23.3`. It accepts
     `customRules` as a config key: `schema/markdownlint-cli2-config-schema.json:17` — `"customRules": {`, `:18` —
     `"description": "Module names or paths of custom rules to load and use when linting : ..."`. Relative paths
     resolve from the config file (README `:340-343`). Each entry is a module loaded with Node's `require` or
     `import` (README `:431-433`), so a custom rule is JavaScript. The repository tracks no JavaScript file today:
     `git ls-files '*.js' '*.mjs' '*.cjs'` printed 0.
   - **Pinned by:** nothing. `git grep -n 'docs:check\|markdownlint' -- internal/ openspec/specs/ .github/` finds no
     line.
   - **Described at:** `AGENTS.md:37` — `task docs:check   # markdownlint`; the step list at `AGENTS.md:52`; preflight
     `SKILL.md:33` — ``| `task docs:check` | Lints Markdown with `markdownlint-cli2` |``; preflight `:57` selects it
     for documentation-only diffs.
   - Like `spec:check`, it runs `npx --no-install`, so it needs `npm ci`.
3. **`task lint`**, an existing `verify` step (`scripts/verify.sh:12`): `Taskfile.yml:48` — `lint:` through `:54`.
   It already runs a shell guard, and that guard's bash fixture test, as extra `cmds` inside one step:
   - `:51` — `- scripts/gopkgs.sh go tool revive -config revive.toml -formatter friendly ./...`
   - `:52` — `# Adapted from SemStreams (ledger rows L9, L10; no exemption marker); tests may not bind fixed ports.`
   - `:53` — `- scripts/lint-test-ports.sh`
   - `:54` — `- scripts/lint-test-ports_fixture_test.sh`

   Further facts:
   - **Reads:** `revive` reads Go packages. `scripts/lint-test-ports.sh` reads `*_test.go` files
     (`scripts/lint-test-ports.sh:46` — `` matches=$(grep -rnE "$LITERAL_PATTERN" --include='*_test.go' . 2>/dev/null
     || true) ``). Nothing in the step reads Markdown.
   - **Pinned by:** nothing reads its `cmds`. `git grep -n "lint:\|\"lint\"\|'lint'" -- internal/ openspec/specs/`
     finds only a CI-fixture job name at `mergegate_test.go:255`. The `harness-boundaries` spec requires the script
     to pass, not that `task lint` runs it: `openspec/specs/harness-boundaries/spec.md:106` —
     ``` `:8222"`) or bind a fixed TCP port (scripts/lint-test-ports.sh SHALL pass). ```
     ``` `scripts/lint-test-ports.sh` SHALL ```.
     `TestLintTestPortsPasses` (`addresses_test.go:51`) also runs the script over the repository under
     `task test:unit`.
   - **Described at:** `AGENTS.md:41` — `task lint         # pinned revive, plus the fixed-port guard for tests`;
     `AGENTS.md:82` (the rules row ends `` `scripts/lint-test-ports.sh` (`task lint`) ``); `docs/testing.md:116`;
     `docs/testing.md:462` — `` `task lint` and a contract test refuse fixed `net.Listen` ports and fixed broker
     addresses in tests. ``; and preflight `SKILL.md:37` — ``| `task lint` | Pinned `revive` |``, which does not
     mention the port guard or its fixture test.
4. **A new `task verify` step**: `scripts/verify.sh:12` —
   `steps=(spec:check docs:check fmt:check tidy:check cleanup-roots:check build vet lint vuln ledger:check test:unit`
   and `:13` — `test:integration cover:check test:repeat)`.
   - Pinned by: `TestUnitInvocationsPinned` (`mergegate_test.go:27`) through `unitInvocationViolations`. It holds only
     that `test:unit` and `test:repeat` are present (`:103-107`) and that `test:repeat` is last (`:109` —
     `if i := slices.Index(steps, "test:repeat"); i >= 0 && i != len(steps)-1 {`). A step added anywhere before
     `test:repeat` passes it. `TestUnitInvocationsPinnedSensitivity` (`:39`) builds its own fixture (`:45`).
   - Measured: in a scratch copy of the base, `scripts/verify.sh:13` was changed to
     `test:integration cover:check hold:check test:repeat)`. Then
     `go test -count=1 -v -run '^TestUnitInvocationsPinned$' ./internal/harness/contract/` printed
     `--- PASS: TestUnitInvocationsPinned (0.00s)` and exited 0.
   - **`AGENTS.md` overstates this pin.** `AGENTS.md:85` reads `` | Unit tests also run five times at one CPU,
     without the race detector, in shuffled order | `merge-gate` spec, "Varied and repeated unit runs" |
     `task test:repeat`, the last step of `task verify`; `TestUnitInvocationsPinned` fails if that command line or
     the step list changes | ``. The test
     (`mergegate_test.go:103-111`) and the spec (`merge-gate/spec.md:18`, `:22`) require only that `test:unit` and
     `test:repeat` be present, with `test:repeat` last. A new step would change the step list without failing the
     test, which makes the row's sentence untrue.
   - The `merge-gate` spec, `openspec/specs/merge-gate/spec.md:14` — `### Requirement: Varied and repeated unit runs`;
     `:18` — `` `scripts/verify.sh` SHALL run both, with `test:repeat` as its last step. ... ``; `:22` —
     `contract test SHALL fail when either command line, or the step list, differs from this requirement.` The
     requirement names no other step.
   - `TestCIWorkflowPinned` (`mergegate_test.go:134`) reads `ci.yml` only, not `verify.sh` or the step list. CI runs
     `task verify` as one step (`ci.yml:56`).
   - Text that lists the steps or the script guards, and so would change: `AGENTS.md:52-54` (the full list);
     `scripts/verify.sh:8-11` (comment); preflight `SKILL.md:47` —
     ``| `task verify` | The checks above except `doctor`, `fmt`, `spec:queue`, `mutate:check`, `merge:check`, ...``
     (a new task would need its own row in that table); `docs/testing.md:115` —
     ``from SemStreams). Four more guards are shell scripts: `task cleanup-roots:check` and `task cover:check`, which``
     through `:118`; `Taskfile.yml:126` (the `verify` desc, generic).
   - PR #113's body says a new step "touches `internal/harness/contract/mergegate_test.go` and the `merge-gate`
     spec". Measured: a new step before `test:repeat` forces neither. Both change only if the design pins the new
     step.
5. **A Go structural guard** in `internal/harness/contract`, run by `task test:unit`, with the shape of
   `TestNoSleepsInTests` (`testtext_test.go:20`) and `TestNoSleepsInTestsSensitivity` (`:25`).
   - Pinned by: only the package itself. `docs/testing.md:334-341` lists the paired guards.
   - Preflight's per-diff selection runs `task test:unit` for "Go behavior". For an OpenSpec-only diff it selects
     `spec:check` and `spec:queue`: `SKILL.md:57` —
     ``- **Documentation or skill instructions only:** `task fmt:check`, `task docs:check`, and `git diff --check`; add``,
     `:58` — `` `task spec:check` when `openspec/` changed. ``, `:59` —
     ``- **An OpenSpec change:** `task spec:check`, then read `task spec:queue` in this worktree.`` CI runs the
     whole of `verify` whatever the diff.
6. **The queue script's own `--strict`** (`:31-34`, `:46`, `:197-199`). No task, CI job or document runs it (§1).
   Measured: it fails on every caveat it shows, so it fails a correctly placed hold and passes the regression
   (§3.1).
7. **`task spec:queue` itself**, `Taskfile.yml:115` — `spec:queue:` and `:118` — `- scripts/openspec-queue.sh`. It
   is advisory (`:31`), not in `verify`, and not in CI (`AGENTS.md:54`).
8. **A new CI job.** `TestCIWorkflowPinned` pins `mergegate_test.go:124` —
   `requiredNeeds         = []string{"verify", "merge-check"}`, and the `merge-gate` spec's
   `### Requirement: Required needs both jobs` (`spec.md:203`) states the same. A third job changes both.
9. **OpenSpec's own validator.** Its task rules are fixed in the package, and `openspec/config.yaml` rules are
   prompt text only (§2.4). No hook from this repository into `openspec validate` was found.

## 5. How the repository tests a shell script from Go (category 5)

What `docs/testing.md` asks of a sensitivity test: `:337` —
`` and `TestNoSecondAuthorityFieldSensitivity`). The sensitivity test plants the violation in a temporary tree and ``,
continuing at `:338-339`: "requires the guard to name the planted file and what it violates, so a guard that fires
for the wrong reason, or matches nothing, fails". The helper for this is `internal/harness/contract/repo_test.go:102` —
`// requireViolation fails unless some violation mentions every fragment. Sensitivity tests use it`. "Show that the
test can fail" (`docs/testing.md:130-144`) is the general procedure.

| Test (file:line) | What it runs | Fixture and fakes | What it asserts |
| --- | --- | --- | --- |
| `TestMergeCheckKnownFlake` `mergecheck_test.go:219`; `TestMergeCheckReview` `:430`; harness `runMergeCheck` `:129` | `scripts/merge-check.sh` | `copyScript` (`repo_test.go:155`) into a temp root; fake `gh`, and `jq` when needed, first on `PATH` via `fakeBin` (`repo_test.go:132`); canned JSON. The fake refuses any call whose arguments differ (`:15-18`) | exit status and output fragments (`requirePass`, `requireFail`, `:181-197`) |
| `TestCleanupRootsCheckSensitivity` `cleanuproots_test.go:18` | `scripts/cleanup-roots-check.sh`, run by path against a temp root (`:26`) | `writeTree` (`repo_test.go:65`) plus `git init` | the clean tree passes; three seeded lines each fail naming `x/x_test.go:4:` |
| `TestCoverCheckSensitivity` `cover_test.go:45`; also `:170`, `:207` | `scripts/cover-check.sh` | `copyScript`; fake `go` on `PATH` (`:173`, `:209`) | failure output names the failing test |
| `TestLintTestPortsPasses` `addresses_test.go:51`; `TestLintTestPortsHonoursNoMarker` `:87` | `scripts/lint-test-ports.sh` | `copyScript` plus a planted `x/x_test.go` | exit 1, naming `x/x_test.go:3:` |
| `scripts/lint-test-ports_fixture_test.sh` (bash, no Go), run by `task lint` (`Taskfile.yml:54`) | `scripts/lint-test-ports.sh` | `mktemp` directories, PASS and FAIL counters | a matrix of match and no-match shapes |
| `TestLedgerCheckWiring` `ledgercheck_test.go:24`; `...Sensitivity` `:32` | none: parses `Taskfile.yml` | constants hold the spec's command lines (`:14-21`) | the task's wiring, not script behaviour |
| `TestUnitInvocationsPinned` `mergegate_test.go:27`; `TestCIWorkflowPinned` `:134` | none: parse `Taskfile.yml`, `scripts/verify.sh`, `ci.yml` | text fixtures | wiring |

- **The seam for the queue script.** It needs `$PWD/node_modules/.bin/openspec` to be executable (`:54` —
  `PATH="$PWD/node_modules/.bin:$PATH"`, `:55` — `if [ ! -x node_modules/.bin/openspec ]; then`). Measured: a stub
  at `<temp root>/node_modules/.bin/openspec` that prints `{"changes":[{"name":"fx",...}]}` for `list` drives the base
  script with `PATH=/usr/bin:/bin` and no npm. On `b91c60d`'s `tasks.md` planted as `openspec/changes/fx/tasks.md`, it
  printed `ok       no halt/hold/deliberate marker in the open tasks` and exited 0. So `copyScript`, a stub CLI
  and a planted `tasks.md` are enough, and no test needs `npm ci`. No Go test depends on `node_modules` today:
  `git grep -n node_modules -- internal/` finds only the comment at `repo_test.go:35`.
- **The SemStreams pin carries a fixture test** that SemEngine did not port: `scripts/openspec-queue_fixture_test.sh`
  at `8b99efe9` (117 lines; `gh api 'repos/C360Studio/semstreams/git/trees/8b99efe9?recursive=1'`, not truncated).
  The local script says so at `scripts/openspec-queue.sh:73` —
  `# as RED — SemStreams' fixture test caught it (not ported yet). Over-broad matching is not a`. That test stubs
  `openspec` on `PATH` (pin `:37-46`) and has 9 `check` cases (pin `:79-111`):
  - positives: HALT, `[~]` as WONTDO, RED, "On HOLD" as BLOCKED, OPEN-Q, deliberate wording as WONTDO;
  - negatives: a completed task that mentions halt, and an ordinary open task;
  - the explicit no-marker line.
  It has no continuation-line case. Unchanged, its stub would not reach SemEngine's script, which exits 2 unless
  `node_modules/.bin/openspec` exists (`:55-57`).
- **The nearest instance of "join a multi-line unit, report it at its first line"** is
  `internal/harness/contract/docker_test.go:161` —
  `// preserved: a joined command is reported at its first line, and its continuation lines are blank.`
  (`scriptLines`, `:157-180`).

## 6. Every document that states the Hold rule (category 2)

### 6.1 The protocol

- `.agents/protocol.md:20` — `` - **Target state, task truth, holds:** the OpenSpec change inside that PR; `task
  spec:queue`, run in the claim's ``
- `.agents/protocol.md:21` — `worktree, shows its holds. ...`
- `.agents/protocol.md:24` — `final CI run, the merge): such a task strands the change, and those steps are recorded
  on the PR. A hold is written`
- `.agents/protocol.md:25` — `` on the unticked task it stops, as `Hold:` followed by what it waits for (an issue or
  PR number, or the owner's ``
- `.agents/protocol.md:26` — `ruling). The queue reads unticked task lines only: a hold in a heading or a paragraph
  does not show.`
- `.agents/protocol.md:32` — `` - **Start:** `gh issue list --state open` · `gh pr list` (drafts are claims; skip
  them) · `task spec:queue` · ``

### 6.2 The `AGENTS.md` rules row

- `AGENTS.md:107` — `` | Every OpenSpec task can be ticked in or before the archive commit; a hold is written as
  `Hold:` on the unticked task it stops | `.agents/protocol.md`, "Target state, task truth, holds"; reviewer
  contract § Contract and task-truth review | review only; `task spec:queue`, run in the claim's worktree, displays
  a hold written this way and fails nothing | ``
- `AGENTS.md:36` — `task spec:queue   # in-flight OpenSpec changes and their holds`

### 6.3 The reviewer contract, § Contract and task-truth review (`:119`)

- `.agents/contracts/semengine-reviewer.md:159` — `` run). A hold that is not written as `Hold:` on the unticked
  task it stops is a finding too: `task spec:queue` does ``
- `.agents/contracts/semengine-reviewer.md:160` — `not show it. Run implementation review before archive. ...`
- The developer and technical-writer contracts do not mention `Hold:` (`git grep -n 'Hold:' -- .agents/` finds only
  `protocol.md:25` and `semengine-reviewer.md:159`).

### 6.4 The `tasks.md` header convention, line 3 of each archived file

Four wordings across eight files; three files have no such sentence.

- "says "hold"": `2026-10-01-flake-defense/tasks.md:3`, `2026-10-01-setup-04a-foundation/tasks.md:3`,
  `2026-10-02-carry-check/tasks.md:3` — `` ... An unchecked task that says "hold" is one `task spec:queue` reports as ``
- "says "Hold:"": `2026-10-05-setup-04a-01-floor/tasks.md:3` —
  `Each task names the outcome that proves it and the gate that checks it. An unchecked task that says "Hold:" is one`
- "whose first line says `Hold:`": `2026-10-03-review-gate-check/tasks.md:3`,
  `2026-10-06-authority-one-spelling/tasks.md:3`, `2026-10-07-runner-image-pin/tasks.md:3` —
  `` Each task names the outcome and the gate that proves it. An unchecked task whose first line says `Hold:` is one ``
- "whose first words are `Hold:`": `2026-10-06-mutation-check/tasks.md:3` —
  `` Each task names the outcome and the gate that proves it. An unticked task whose first words are `Hold:` waits for ``
- No such sentence: `2026-09-30-setup-02-isolated-harness`, `2026-10-01-setup-03b-contract-boundary`,
  `2026-10-02-await-last-error`.
- PR #93 at `c758aaf5` (unchanged from `dbeb5fe8`), `tasks.md:3` —
  `Each task names the outcome that proves it and the gate that checks it. An
  unticked task that says "Hold:" is one`.

### 6.5 Skills, maps and the plan that name the queue or holds

- `.agents/skills/semengine-pickup/SKILL.md:20` — `` Run `task spec:queue` from the claim's worktree and read its
  tasks and holds. The primary checkout is ``; `:35` — `task spec:queue`.
- `.agents/skills/semengine-handoff/SKILL.md:17` — `` Read the current PR, applicable issue decisions, and OpenSpec
  change. Run `task spec:queue` in the claim's ``.
- `.agents/skills/semengine-preflight/SKILL.md:32` —
  ``| `task spec:queue` | Shows each in-flight OpenSpec change with its holds and blocking conditions |``; `:59`
  (§4, item 5).
- `docs/repository-map.md:82` —
  `` | Target state, tasks, holds | The OpenSpec change inside that PR; `task spec:queue` reads the holds | ``.
- `docs/setup-plan.md:306-308`: "OpenSpec changes own designs, tasks, and holds."
- Specs: `git grep -n 'Hold:\|spec:queue\|openspec-queue' -- openspec/specs/` found nothing. ADRs:
  `git grep -l -i 'spec:queue\|openspec-queue\|Hold:' -- docs/adr/` found nothing.

### 6.6 What the statements say

None of the three rule statements (§6.1-6.3) says "first line". The protocol says "on the unticked task it stops"
and "The queue reads unticked task lines only: a hold in a heading or a paragraph does not show." The first-line
wording appears only in four archived headers: three say "first line" and one says "first words". PR #93's header
says only "says "Hold:"", and 18 of #93's 21 holds sit on continuation lines.

## 7. Open pull requests (category 3)

`gh pr list --state open --limit 100 --json number,title,isDraft,headRefName,changedFiles` returned two:

- **#113** (this claim): draft, 0 files.
- **#93** `feat(setup-04a-02): ingest kernel — ...`: draft, 98 files, head
  `c758aaf5a0bcd77f3cc1b06fac20fafae43841ee`, updated 2026-10-07T13:19:56Z. The full file list came from
  `gh api --paginate repos/C360Studio/semengine/pulls/93/files --jq '.[].filename'` (98 lines, under the 100-file
  limit). At the earlier head `dbeb5fe8` it had 81 files. The 17 added files are all under `graph/kvcatalog/` and
  `natsclient/`, so the overlap below is unchanged.

**#93's file overlap** with every file this inventory names:

- None in `scripts/`, `Taskfile.yml`, `AGENTS.md`, `.agents/`, `docs/testing.md`, `docs/repository-map.md`,
  `.github/` or `openspec/specs/merge-gate`.
- `internal/harness/contract/`: four files, `boundaries_test.go`, `graphtransport_test.go`, `imagepin_test.go` and
  `promglobal_test.go`. That is the same Go package a contract guard would join, but none of these is a file named
  above.
- **Capability overlap:** `openspec/changes/setup-04a-02-ingest-kernel/specs/harness-boundaries/spec.md`, a delta to
  `harness-boundaries`, the capability that holds the structural-guard requirements (for example "No sleeps in
  tests").

**#93's content overlap.** This is not a shared file: #93's own `tasks.md` is what a new check would read. At
`c758aaf5`, 18 unticked tasks carry `Hold:` only on a continuation line:

- 2.1 (task L49, hold L51), 2.4 (L57, L60), 2.5 (L62, L64);
- 3.2 (L87, L89), 3.3 (L90, L103), 3.7 (L130, L131), 3.12a (L171, L176), 3.13 (L179, L184);
- 4.1 (L188, L189), 4.2 (L191, L192), 4.3 (L193, L195), 4.4 (L196, L200), 4.5 (L201, L202), 4.7 (L205, L206),
  4.8 (L207, L216), 4.9 (L217, L219);
- 5.2 (L247, L249), 7.1 (L291, L293).

The three first-line holds are 3.12b (L177), 4.6 (L203) and 7.2 (L294). At `dbeb5fe8` the same 18 and 3 tasks held
the same placements, with every line from task 3.7 on two lower.

The base script on #93's tree at `c758aaf5` printed four lines and exited 0:

```text
      BLOCKED  L177   3.12b (D) Fail closed at birth and on the guard record (#111 item 1; design D21): the tests of task 4.9.
      BLOCKED  L203   4.6 (D) #33: the hierarchy refusal names `inference.RegisterPayloads` (`component-registration`). Hold: 
      RED      L207   4.8 (D) #100 and #98, the `graph-entity-writes` scenarios through graph-ingest, each written first and f
      BLOCKED  L294   7.2 (W) Specs synced from the deltas, the change archived; the archive commit is the last content commit
```

- None of the 18 continuation-line holds shows.
- For all three first-line holds, the 104-character cut at `:179` (§2.5) means the queue never prints what the hold
  waits for. For 3.12b and 7.2 the printed text stops before `Hold:`; for 4.6 it stops right after `Hold:`.
- `openspec validate --all --strict` on the same tree exits 0 (`Totals: 12 passed, 0 failed (12 items)`,
  `✓ change/setup-04a-02-ingest-kernel`).

Under the protocol, #93 is brought up to date by merging `origin/main` (`AGENTS.md:108`), and it merges only on a head
up to date with `main` (`AGENTS.md:137`). The two orders play out like this:

- If a failing check lands on `main` first, #93's next CI run after it merges `main` reads those 18 tasks.
- If #93 merges first, its archive commit, the last content commit (`.agents/protocol.md:21`), moves its `tasks.md`
  under `archive/`, out of view of both the queue and `spec:check` (§2.4). `docs:check` still reads it there
  (§4, item 2), but by then every task in it is ticked, because every task can be ticked in or before the archive
  commit (`.agents/protocol.md:22`).

**Other claims on the territory:**

- The owner's queue comment says this work runs alongside #92 and #93. #92 has merged (`1e383fe`,
  2026-10-07T10:18:37Z).
- Open issues: `gh issue list --state open --limit 200` returned 61. Filtering titles for
  `queue|hold|verify|spec:check|openspec|tasks.md|task truth|merge-gate|archive|step` gave #76 and #86 ("docs:
  repository-map omits three archived changes and the specs they amended", no claiming PR). #86 touches
  `docs/repository-map.md`, which says at `:82` that `task spec:queue` reads the holds.
- Body and comment search (`gh issue list --search '"<q>" in:body,comments'`): `spec:queue`, `openspec-queue` and
  `spec:check` returned #76 only, and `verify.sh` returned nothing.
- Ledger: `scripts/openspec-queue.sh` has no admission-ledger row (`git grep -n openspec-queue
  docs/admission-ledger.yaml` found nothing), although three ported scripts do (`docs/admission-ledger.yaml:196`,
  `:239`, `:258`). It arrived with setup-01 (`819c461`, #12) from SemStreams `5457b345` (`scripts/openspec-queue.sh:2`
  — `# Ported from SemStreams 5457b345; uses the repo-pinned OpenSpec CLI from node_modules.`), the same source as
  `.agents/` (`.agents/README.md:4-5`), and not from the pin `8b99efe9`.

## 8. Other readers of `tasks.md`, the parser and the seam (categories 4 and 5)

- Readers of `tasks.md` or task checkboxes:
  `git grep -n -E 'tasks\.md|\[ \]|\[x\]|\[~\]|checkbox|unticked|unchecked' -- scripts/ internal/ .github/
  Taskfile.yml .agents/skills/ .claude/` found `scripts/openspec-queue.sh` (`:13`, `:66`, `:152`, `:154`, `:158`,
  `:169`) and two comments in `mergegate_test.go` (`:191`, `:215`) that cite task numbers and read nothing. No Go
  code reads `tasks.md`. Separately, `task docs:check` reads every active and archived `tasks.md` as Markdown and
  applies style rules only, with no notion of a task (§4, item 2).
- Readers of the CLI's task data: `git grep -n -E 'openspec (list|show|instructions|status)' -- scripts/ internal/
  .github/ Taskfile.yml .agents/` found only `scripts/openspec-queue.sh:5`, `:98` and `:100`.
- Readers of the queue's output, as text: the skills and protocol lines in §6.1 and §6.5. The platform adapters are
  thin (`.claude/skills/*`, `.codex/agents/*.toml`); `git grep -n -i 'queue\|hold' -- .codex/ .claude/` found one
  unrelated line.
- Every spelling of a hold reader in code, CI and tasks: `git grep -n -i -E '\bhold\b|Hold:|holds\b|continuation|first
  line' -- scripts/ internal/ .github/ Taskfile.yml '*.go'` found the queue script plus unrelated hits: the
  continuation joiner in `docker_test.go`, the `hold://` remote in `pindiff`, and `hold` fields in `natsclient` test
  fakes.
- **Out of scope** (issue #76): `label_for`'s word list (`halt`, `blocked`, `deliberate`, `still open`, `:80-84`).
  One measurement, for the design's information only: if `label_for` read whole blocks instead of first lines, then
  on #93's `tasks.md` at `c758aaf5` the labelled tasks would go from 4 to 23. Four tasks that carry `Hold:` (L57,
  L90, L179, L217) would read RED, because RED is tested before BLOCKED (`:81-82`), and two tasks with no hold (L152,
  L256) would read RED from their prose.

## Contract categories (index)

1. **The claimed gap, "nothing fails": confirmed.** At `b91c60d` and `a7dbf9d`, `spec:check`'s command exits 0, the
   queue prints `ok` or hides the task with exit 0, and `--strict` exits 0 at `b91c60d` (§3). No Go test, CI step or
   other script reads `tasks.md` as tasks. `docs:check` reads every `tasks.md`, but only as Markdown (§4, item 2;
   §8).
2. **Every spelling of "this open task is held":** `label_for`'s case-insensitive `\bhold\b` on the checkbox line
   (§1); three readers of "open task" (§2.3); three rule statements without "first line", four header wordings, and
   #93's header (§6).
3. **Adjacent claims:** §7. Specs `merge-gate` (§4) and `harness-boundaries` (the structural guards; #93 has a
   delta). No ADR. The ledger row is absent (§7). The existing `verify` steps closest to the territory are
   `docs:check`, which already reads every `tasks.md`, and `lint`, which already carries a shell guard and its fixture
   test as extra `cmds`. Nothing pins either step's `cmds` (§4, items 2-3). `AGENTS.md:85` overstates what
   `TestUnitInvocationsPinned` holds (§4, item 4).
4. **The consumer at birth:** this inventory proposes no symbol. Present users of a hold check would be the
   protocol's rule (§6.1), the reviewer's finding (§6.3) and PR #93's 18 tasks (§7).
5. **The problem shape:** a structural guard over tracked text that fails naming `file:line`, with a paired
   sensitivity test. The closest instances are `scripts/cleanup-roots-check.sh:16` —
   `hits=$(git grep -n --untracked -E "$pattern" -- '*_test.go' || true)` with `TestCleanupRootsCheckSensitivity`
   (`cleanuproots_test.go:18`), and `TestNoSleepsInTests` / `TestNoSleepsInTestsSensitivity` (`testtext_test.go:20`,
   `:25`). Two narrower shapes: "assemble a multi-line unit, report it at its first line"
   (`docker_test.go:157-161`), and "read task lines" (`scripts/openspec-queue.sh:163-182`; OpenSpec's
   `parseTaskLines`). Two instances of where such a guard is wired: as a step of its own (`cleanup-roots:check`,
   `scripts/verify.sh:12`), or as extra `cmds` inside an existing step with a bash fixture test beside it
   (`Taskfile.yml:53-54`). A rule over every Markdown file would run under `docs:check`; `customRules` supports that
   (§4, item 2), and none is configured today.

## Same-class collision table: not triggered

The change adds no durable, communication or runtime-coordination primitive. It concerns repository tooling
(`scripts/`, `Taskfile.yml`, a contract test).

## Intent check: not triggered

No boundary is set or moved: no change to the port set, a tier, an exclusion or a deferral.

## Adopter seam inventory

**Reached from outside the repository:** the queue's *output*, not the script's code. The owner's global Claude
skills read it; they are user-private files, not tracked here.

- `~/.claude/skills/pickup/SKILL.md:45-50` finds a task named like the queue with
  `task --list 2>/dev/null | grep -Ei 'queue|openspec'` and reads what it prints.
- `~/.claude/skills/handoff/SKILL.md:23` routes a hold to a "`tasks.md` marker with the reason — what the queue
  target reads".
- `~/.claude/CLAUDE.md:16` says the same.

Measured by grepping those three paths for `spec:queue|openspec:queue|openspec-queue|Hold:|BLOCKED` and for
`queue|holds|openspec`. Nothing outside the repository reads the output of `spec:check` or `verify` except CI, which
is in this repository. The scripts are not published: `package.json:3` — `"private": true,` and `:4` —
`"description": "Pinned repository tooling for SemEngine (not a published package)",`. Sister repositories were not
searched for callers: `scripts/` cannot be imported, and `docs/inventory-scope.md` does not cover that question.

1. **What must they know?** That the task name contains `queue` (`spec:queue` does), and that a held task shows as a
   `BLOCKED` line, cut at 104 characters (§2.5).
2. **What happens if they do nothing?** A hold below a first line never reaches them: §3, and 18 of #93's 21 holds
   (§7). For #93's other 3 holds the line reaches them, but what the hold waits for does not (§2.5).
3. **Where do they find out?** Nowhere. No command fails.
4. **The gap between 1 and 4:** the default path in item 2 is silent today.

## Premises in issue #76, measured

| Issue premise | Measured | Where |
| --- | --- | --- |
| "the OpenSpec CLI hands it the task's first line" | False for the script: its own `grep` reads only checkbox lines, and the CLI supplies counts. True of the CLI's own task interface, which the script does not use | §1 |
| `label_for` at `:79-87` | Stale by one line: `:78-86` | §1 |
| the `ok` line at `:185` | True | §1 |
| purpose at `:7-31`; advisory at `:31` | `:31` is true; the "invisible caveat" text is at `:75` | §1 |
| `50b4a0c`: "hold on the last line, queue `ok`" | False: the hold is on the first line (L53), and the queue shows `BLOCKED  L53` | §3.1 |
| `b0bb713`: "first line, queue `BLOCKED L53`" | First line, but the queue shows `BLOCKED  L55` (L53 is `50b4a0c`'s) | §3.1 |
| `b91c60d`: "regressed, `ok`" | True | §3.1 |
| "Fixed by hand (`b0bb713`), then regressed ..., and fixed again" | The order is reversed: `b91c60d` (the regression) is the parent of `b0bb713` (the fix). The committed history has one regression and one fix | §3.1 |
| PR #48 F7 is comment 5969256728 | That comment is the re-check that marks F7 resolved. F7 itself is in comment 5968489295 | §3.2 |
| `tasks.md:3-4` states the first-line rule | True for `authority-one-spelling`. The protocol, `AGENTS.md` and the reviewer contract do not say "first line" | §6 |
| "Three documents state the rule; one script reads holds; nothing fails" | True | §1, §3, §6 |
| A new `verify` step touches `mergegate_test.go` and the `merge-gate` spec (PR #113 body) | Not forced by a step placed before `test:repeat` | §4 |
| "None [of the expected files] are in #93's files" (PR #113 body) | True for the files, at both `dbeb5fe8` (81 files) and `c758aaf5` (98). #93's `tasks.md` holds 18 continuation-line holds that a check would read | §7 |
| (repository, not the issue) `AGENTS.md:85`: `TestUnitInvocationsPinned` "fails if that command line or the step list changes" | Overstated: with a step added before `test:repeat`, the test passes | §4, item 4 |

## Open evidence questions

1. **Which reading of the rule binds?** The three rule statements (§6.1-6.3) say "on the unticked task it stops", not
   "on its first line". The first-line wording exists only in four archived headers, which split between "first line"
   and "first words" (§6.4). PR #93, in flight, follows a header that says neither and places 18 holds below the first
   line (§7). Whether "on the task" means "on its first line" is a reading of the rule, so it is for the owner rather
   than the design.
2. **"First line" or "first words".** #93's three first-line holds sit at columns 105-113 of 114-117-character lines
   and wrap what they wait for onto the next line: 3.12b and 7.2 end with `Hold:`, and 4.6 ends with `Hold: task`
   (§2.5). The queue prints at most 104 characters (`:179`), so for all three it never shows what the hold waits for,
   and for 3.12b and 7.2 it does not show `Hold:` itself. One archived header says "first words". The two readings
   classify these three tasks differently, and only "first words" puts the hold inside what the queue prints.
3. **Ordering against #93.** It depends on question 1 and on which change merges first (§7).

## Local-only evidence

These paths sit outside the repository and exist on the measuring machine only.

What reproduces from the commands in the text, with the repository and `gh`:

- the §2.1 archive counts (the command block in §2.1);
- #93's counts and hold lines in §2.2 and §7 (the same commands, run on
  `git show c758aaf5:openspec/changes/setup-04a-02-ingest-kernel/tasks.md`; the hold list comes from the last `awk`);
- the §2.5 columns: `awk '/^- \[ \] .*Hold:/{print FNR, length($0), index($0,"Hold:")}'` printed `177 116 112`,
  `203 114 105`, `294 117 113`.

What needs more than that:

- The queue, validate and `markdownlint-cli2` runs need the pinned tools (`npm ci`).
- The §8 whole-block comparison uses a local Python re-implementation of `:80-84`,
  `/tmp/semengine-76/tools/label_blocks.py`.

Files:

- `/tmp/semengine-76/tools/survey.py`: the first `tasks.md` survey; the §2.1 commands reproduce its numbers.
- `/tmp/semengine-76/tools/label_blocks.py`: the §8 comparison.
- `/tmp/semengine-76/npm/`: the pinned CLI 1.13.2, from `npm ci --ignore-scripts`.
- `/tmp/semengine-76/cli-1.13.2/`: the published tarball, whose `integrity` was checked.
- `/tmp/semengine-76/fx/<sha>/`: each commit's `openspec/` from `git archive`, with the validate logs.
- `/tmp/semengine-76/seam/`: the stub-CLI run.
- `/tmp/semengine-76/pin/`: SemStreams `8b99efe9`'s `scripts/openspec-queue.sh` and
  `scripts/openspec-queue_fixture_test.sh`, fetched with `gh api .../contents/<path>?ref=8b99efe9`.
- `/tmp/semengine-76/pr93-tasks.md`, `/tmp/semengine-76/pr93-files.txt`: PR #93's `tasks.md` and file list at
  `dbeb5fe8`; `/tmp/semengine-76/pr93-c758-tasks.md`, `/tmp/semengine-76/pr93-c758-files.txt`: the same at
  `c758aaf5`.
- `/tmp/semengine-76/fx/docscheck/`: a scratch copy of the base with the two planted lint lines (§4, item 2) and the
  added `hold:check` step (§4, item 4); `/tmp/semengine-76/fx/unitpinned.log`: that test run.
