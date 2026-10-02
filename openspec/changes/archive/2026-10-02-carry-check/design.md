# Design: carry-check

Issue #52. The evidence is in `inventory.md` (base `c3ebb73`), cited as G, S, A, P and M. The design says what a
caller can observe and names the test for each behaviour; locks, types and file layout are the developer's.

## Terms

- **The pin**: SemStreams at an entry's `source_sha`. **Covered files**: what an entry is compared on (D2).
  **The rewrite**: the change of import path that porting makes and that is not a difference (D3).
- **Destination path**: the leading run of letters, digits, `/`, `.`, `_` and `-` in `destination`; a note may
  follow it, as on eight of the nine destinations on `main` (M12). It is relative to the repository root and inside
  it: one that starts with `/`, has a `..` segment or resolves outside counts as a path that does not exist.
- **The program**: the Go command that both tasks run. Exit codes are the program's. `go run` and `task` replace
  them with their own (M13), so through `task` only pass or fail and the message are promised.

## Decisions

### D1. The pin is fetched; `source_sha` is the one recorded hash (question 1)

The program runs `git fetch --depth 1` of each distinct `source_sha` it needs from
`https://github.com/c360studio/semstreams.git` into a temporary directory outside the repository, and removes it.
SemStreams is public and the fetch needs no credentials (M1); it took 2.4 s and 17 MB (M2). Git names every object by
the hash of its content, so the 40-character `source_sha` the schema already requires (`ledger_test.go:170`) is a
recorded hash of every file at the pin, and git checks it on each fetch; nothing else needs recording. With no `carry`
entry nothing is fetched, so `main` today stays offline (G4). All the fetches of one run share one bound of two minutes,
not two minutes each; past it the pin is unreadable and every process a fetch started (git's helper too) is killed.

### D2. The unit is the ledger entry; test files are covered (questions 2 and 3)

The check works on whatever `source_path` is at the pin, read there and not declared: a file or a directory. A directory
entry covers the `.go` files directly in it and every file under its `testdata` directory. Test files are covered: the
owner ruled that a package with a repaired test is `adapt` (A2), and `task ledger:diff` then shows the repair. A
sub-directory is another package (`vocabulary` has eight, M3). `README.md` is not covered: three planned `carry`
packages have one that fails `task docs:check` as it stands at the pin (M9). A `carry` entry must have every covered pin
file in the tree and no covered file the pin lacks; one that covers no file on either side is not compared.

### D3. What is not a difference

Only `.go` files outside `testdata` are rewritten; every other covered file is compared byte for byte.

1. The module path, in import paths and comments only: `github.com/c360studio/semstreams` becomes
   `github.com/c360studio/semengine`. A string literal is never rewritten, so a string that holds the module path
   and was changed shows as a difference (seven lines in the floor, all in one `natsclient` test file, M4).
2. A moved package: where the module path is followed by a package path that is exactly the `source_path` of a
   moved package, it becomes that package's destination path. A moved package is a `carry` or `adapt` entry at the
   same `source_sha`, a directory at the pin, whose destination path is a directory and differs. No other
   disposition moves anything: a `defer-exclude` row can name an existing directory (`admission-ledger.yaml:139-142`).
   Needed now for two `message` test files (M7) and for importers of a package made internal (A3). A package path
   ends before the first character that is not a letter, a digit, `/`, `_`, `-` or `.`, less a trailing `.` or `/`.
3. Formatting: both sides are formatted with gofmt. A move re-sorts import lines (six files in the trial, M6), and
   a change to gofmt would otherwise fail every `carry` entry at once.

Everything else is a difference, a comment that renames SemStreams included (M11).

### D4. `task ledger:diff -- [<source_path>...]`

- Arguments: ledger `source_path` values; with none, every `carry` and `adapt` entry, in ledger order.
- Standard output: a unified diff per differing covered file, from the rewritten pin text (`--- pin/<pin path>`) to the
  tree file (`+++ <tree path>`); `only at the pin: pin/<pin path>` or `only in the tree: <tree path>` for a one-sided
  file. An equal file prints nothing. The rewrite keeps line numbers, apart from re-sorted import lines.
- Standard error: one line per entry, `<source_path> (<disposition>): N files, K differ, A only at the pin, B only
  in the tree`, N counting covered files on both sides, or `…: not compared: <reason>` (D2's reasons).
- The program exits 0 when it ran, differences or not; 2 when an argument names no entry, a named entry was not
  compared, or the ledger or the pin could not be read.

### D5. `task ledger:check`

Runs the schema tests as now, then the program over every `carry` entry, with D4's comparison: a file prints nothing
there exactly when it passes here. The program exits 1 on a covered file that differs or is on one side only, and on a
`carry` entry that cannot be compared. Each line names the entry, the file and the kind; the closing lines say
`task ledger:diff -- <source_path>` prints the lines and that a package that differs is `adapt` (`docs/provenance.md`
rule 5). `adapt` entries are never failed. An unreadable pin exits 2, saying so and that no entry was checked.

### D6. Home and constraints

- The program lives under `internal/harness/` (proposed `internal/harness/pindiff`), the only place a non-test Go
  file may import `gopkg.in/yaml.v3` (A8). It is not a contract test: those run seven times per verify (A6).
- It reads the ledger and the tree from its working directory. The remote and the fetch bound default to D1's and
  can be set from the environment, so a test runs the real program against a local repository, bound under a second.
- The schema stays in `ledgerViolations` (S4); the program decodes the fields it needs and does not validate. No new
  module dependency, cache or CI job. `main` makes the one root context; the fetch receives it with the bound.

### D7. What the reviewer is handed (question 4)

The reviewer of a pull request that ports packages runs `task ledger:diff -- <the change's source paths>` at the
commit under review. The output depends only on that commit and the pins, so it is neither committed nor uploaded.
The verdict comment records the commit, the command line, the per-entry lines and the SHA-256 of standard output;
`docs/provenance.md` says so. For PR #48 this feeds tasks 3.10 and 7.1, and the per-entry lines are task 3.9's "diff
stat against the pin per package" (P48).

## Tests

Each is written first and seen to fail. They run offline against a local git repository that stands in for
SemStreams; their git calls ignore the user's git configuration. Expected texts are written out in the test, never
produced by the code under test (`docs/testing.md:23-27`).

| Behaviour | Test (in `pindiff` unless said) |
| --- | --- |
| Module path rewritten in an import and a comment, not in a string; a package path in a doc link ends at `]` | `TestRewriteModulePath` |
| Moved package rewritten from a `carry` or `adapt` directory entry; re-sorted imports are equal | `TestRewriteMovedPackage` |
| README and sub-directory out, `testdata` in, note after the path ignored | `TestCoveredFiles` |
| Unchanged prints nothing; a changed line, a one-sided file and the per-entry lines are printed | `TestDiffOutput` |
| Exit 2 for an unknown argument and a named entry not compared; exit 0 otherwise | `TestDiffExitCodes` |
| Each planted violation exits 1 naming entry, file and kind: a `carry` directory entry covering nothing; a one-line edit in a non-test file, a test file and a `testdata` file; a file removed; a file added; a changed string literal that holds the module path; an import redirected to the destination of a `defer-exclude` entry; a destination that does not exist, starts with `/` or has `..` | `TestCheckSensitivity` |
| A carried entry passes; an `adapt` entry is never failed; no `carry` entry passes with an unreachable remote | `TestCheckScope` |
| Pin unreadable exits 2; a fetch that never answers ends at a bound under a second | `TestCheckPinUnreadable` |
| `check` and `diff`, passing, violated and pin unreadable, in a read-only tree: its files and hashes unchanged | `TestWritesNothingInTree` |
| The program, run as a separate process the way `task ledger:check` runs it, in a temporary tree with a planted `carry` violation, exits non-zero and prints the violation; with no `carry` entry it exits 0 | `TestCommandExitStatus` |
| The printed diff, applied to the pin text, gives the tree text; changes six unchanged lines apart share a hunk and seven apart do not; an empty range is named by the line before it; a last line without a newline is marked | `TestUnifiedDiff` and `FuzzUnifiedDiffApplies` |
| After a fetch is cut off, no process the fetch started, git's transport helper included, is still running | `TestFetchLeavesNoProcess` |
| A changed middle too large to match line by line is printed as one removal and one addition, announced on standard error naming the file, and the diff still applies | `TestDiffLargeFileFallback` |
| `ledger:check` in `Taskfile.yml` runs the schema tests, then that program, with nothing that discards its exit status; a planted removal fails | `TestLedgerCheckWiring` and its `Sensitivity` pair, in `contract` |

## Options considered

| Option | Cost | Outcome |
| --- | --- | --- |
| A. Fetch the pin at check time, inside `task ledger:check` | verify reads `github.com` once a `carry` row exists | chosen (D1): the existing task and capability are extended, no new one |
| B. Per-file hashes beside the ledger | a hash file, a generator, and a step that checks the hashes against the pin; that step fetches the pin too, or is review only | rejected: three more parts; a hash file is honest only while something compares it with the pin, which needs the fetch |
| C. Another source for the pin: `go mod download`, or a copy in this repository | the module proxy and checksum database; or 17 MB and 4,679 files (M2) | rejected: the first has the same network need and brings SemStreams into module tooling; the second is a second copy of the source |
| D. Module path only, no moved packages | smallest | rejected: `message` could not be `carry` (M7), and every importer of an internal package would be `adapt` with no behaviour to name (A3, rule 5) |
| E. A CI job of its own, like `merge-check` | workflow, `merge-gate` spec and its contract tests change | rejected: larger; verify already has a network step (A5) |
| F. A contract test in `internal/harness/contract` | seven fetches per verify (A6) | rejected |
| G. Do nothing | the reviewer reads about 60,000 lines or trusts the rows | rejected (#52) |

## Overlapping claims and merge order

- **PR #56** (merged as `4b5a264`): edits `docs/testing.md` next to the lines task 5.3 edits, and the `AGENTS.md` rule
  index (P56). It merged first; task 5.3 is written on top of it.
- **PR #48** (draft, `3439494`): no shared file; both add to `harness-boundaries`, in different requirements (P48).
  This change merges before PR #48's section 3.
- PR #39 and PR #55 are merged (`c0a5515`, `c3ebb73`); the claim branch is brought up to date first.

For PR #48, stated and not reviewed: `message` is `carry` only with a `carry` or `adapt` entry for
`internal/semantictest` (M7); `pkg/types` only with `entity_id_prop_test.go` and `testdata/`, which need
`pgregory.net/rapid` (M10); a `carry` row lands with its files, because a row with no files fails (D5). Whether the
internal packages move (P48, `design.md:70` against `:239-241`) does not change this design.

## Declared costs and the owner question

- C1. Once a `carry` row exists, `task verify` and CI need `github.com` for one unauthenticated fetch per distinct
  `source_sha`: the first verify step that reads GitHub (A5). What it reads is fixed by the SHA, where `merge-check`
  reads state that changes. A fetch that does not answer is a red run and a rerun, not a known flake (A7).
- C2. SemStreams must stay fetchable at the pin. If it is deleted or made private the check fails until the
  program's remote points at a copy; any copy will do, because the SHA fixes the content.
- C3. `README.md`, other non-Go files outside `testdata`, and a new sub-package added under a carried destination
  are outside the claim; they stay review only (M9).
- C4. A carried file keeps the pin's words, the name SemStreams included (M11).
- C5. `adapt` entries are printed and never checked: nothing fails if one drifts after its review.
- C6. The new package is not added to `task cover:check`: PR #48's task 5.4 rewrites that script's target list.

**Owner ruling, Q1 (2026-10-02, issue #52, comment 5951926749):** the fetch is accepted, with C1 and C2. Option B
(verify stays offline, at the price of a hash file and its verifying step) is not taken.
