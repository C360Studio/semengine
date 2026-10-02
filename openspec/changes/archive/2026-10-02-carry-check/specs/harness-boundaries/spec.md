# harness-boundaries

## ADDED Requirements

### Requirement: Comparison with the pin

A ledger entry SHALL be compared with SemStreams by one rule, used by both `task ledger:check` and
`task ledger:diff`. The pin side is `source_path` at `source_sha`, fetched from the SemStreams repository. The tree
side is the entry's destination path: the leading run of letters, digits, `/`, `.`, `_` and `-` in `destination`;
anything after it is a note. A destination path SHALL be relative to the repository root and inside the repository:
one that starts with `/`, has a `..` segment, or resolves outside the repository is treated as a path that does not
exist.

When `source_path` is a file at the pin, the entry covers that file and the destination path SHALL be a file. When
it is a directory, the entry covers the `.go` files directly in it, test files included, and every file under its
`testdata` directory, and the same selection is made in the destination directory. Other files directly in the
directory, such as `README.md`, and other subdirectories, a new sub-package included, are not covered.

Before comparing, the pin side of a `.go` file outside `testdata` SHALL be rewritten, in import paths and in
comments only: `github.com/c360studio/semstreams` becomes `github.com/c360studio/semengine`. Where it is followed by
`/` and a package path that is exactly the `source_path` of a moved package, the package path becomes that package's
destination path. A package path ends before the first character that is not a letter, a digit, `/`, `_`, `-` or
`.`, and does not include a trailing `.` or `/`. A moved package is a `carry` or `adapt` entry with the same
`source_sha` as the file, whose `source_path` is a directory at the pin and whose destination path is a directory in
the tree and differs from `source_path`; an entry with any other disposition moves nothing. Text outside import paths
and comments, string literals included, SHALL NOT be rewritten. Both sides of the file SHALL then be formatted with
gofmt. A `.go` file gofmt cannot parse, and every other covered file, is compared as it stands. Two files are equal
when the results are byte-identical.

An entry SHALL be reported as not compared, with the reason, when its destination path does not exist in the tree,
when `source_path` does not exist at `source_sha`, when one side is a file and the other a directory, or when it is
a directory entry that covers no file at the pin and none in the tree.

#### Scenario: Module path in an import and a comment

- **WHEN** a pin file has `github.com/c360studio/semstreams/pkg/errs` in an import and in a doc comment, and the tree
  file has `github.com/c360studio/semengine/pkg/errs` in the same two places and is otherwise identical
- **THEN** the files are equal

#### Scenario: Module path in a string literal

- **WHEN** a pin file holds `github.com/c360studio/semstreams/natsclient` in a string literal and the tree file
  holds `github.com/c360studio/semengine/natsclient` there
- **THEN** the file is reported as different

#### Scenario: Import of a package the ledger moved

- **WHEN** the `carry` entry for `internal/semantictest` has destination `internal/harness/semantictest`, a pin file
  imports `github.com/c360studio/semstreams/internal/semantictest`, and the tree file imports
  `github.com/c360studio/semengine/internal/harness/semantictest`
- **THEN** the files are equal

#### Scenario: Excluded entry moves nothing

- **WHEN** a `defer-exclude` entry for the package `pkg/old` has a destination path that is a directory in the tree,
  a pin file imports `github.com/c360studio/semstreams/pkg/old`, and the tree file imports that directory instead
- **THEN** the file is reported as different

#### Scenario: Imports sorted differently after a move

- **WHEN** a moved package's new import path sorts to a different line of the import block in the tree file
- **THEN** the files are equal

#### Scenario: Package path in a doc link

- **WHEN** a pin comment holds `[github.com/c360studio/semstreams/pkg/tlsutil]:` and `pkg/tlsutil` is a moved package
- **THEN** the package path rewritten is `pkg/tlsutil`, and `]:` is left as it is

#### Scenario: Note after the destination path

- **WHEN** an entry's destination is `scripts/lint-test-ports.sh (adapted by change flake-defense)`
- **THEN** the tree side is the file `scripts/lint-test-ports.sh`

#### Scenario: Destination outside the repository

- **WHEN** an entry's destination is `../elsewhere/pkg` or `/tmp/pkg`, and a directory exists there
- **THEN** the entry is reported as not compared, because its destination is not a path in this repository

#### Scenario: README is not covered

- **WHEN** `README.md` in a package directory differs from the pin and every `.go` file is equal
- **THEN** no difference is reported for the entry

#### Scenario: Test data is covered

- **WHEN** a file under the package's `testdata` directory differs from the pin
- **THEN** the file is reported as different

#### Scenario: File at the pin, directory in the tree

- **WHEN** `source_path` is a file at the pin and the destination path is a directory
- **THEN** the entry is reported as not compared, with that reason

#### Scenario: Directory entry with nothing covered

- **WHEN** `source_path` is a directory at the pin that holds only `README.md` and a sub-directory, and the
  destination directory holds only `README.md`
- **THEN** the entry is reported as not compared, because no file is covered at the pin or in the tree

### Requirement: Carried entries match the pin

After the schema check, `task ledger:check` SHALL run a program that compares every entry whose disposition is `carry`,
test files included. The program SHALL exit 1 when a covered file differs from the pin, exists only at the pin, or
exists only in the tree, or when a `carry` entry cannot be compared. Each failure SHALL name the entry's `source_path`,
the file, and which of those four it is. The output SHALL say that `task ledger:diff -- <source_path>` prints the lines,
and that a package that differs is `adapt` (`docs/provenance.md` rule 5). Entries with any other disposition SHALL NOT
be compared by the check. With no `carry` entry the program SHALL exit 0 without contacting SemStreams. When SemStreams
cannot be read within the fetch bound, which is two minutes for all the fetches of one run together, the program SHALL
exit 2 with a message that says the pin could not be read and that no entry was checked. When a fetch fails or is cut
off, by the fetch bound or by an interrupt, no process the program started for it SHALL still be running when the
program exits.
The program SHALL write nothing inside the repository. `task ledger:check` SHALL fail whenever the program exits
non-zero; `task` and `go run` replace the program's exit status with their own, so through `task` only pass or fail and
the message are promised.

#### Scenario: Carried package unchanged

- **WHEN** every covered file of a `carry` entry equals the pin
- **THEN** ledger:check passes

#### Scenario: One line edited in a carried file

- **WHEN** one line of a non-test `.go` file under a `carry` entry is edited
- **THEN** ledger:check fails naming the entry and the file

#### Scenario: Test repaired in a carried package

- **WHEN** one line of a `_test.go` file under a `carry` entry is edited
- **THEN** ledger:check fails naming the entry and the file

#### Scenario: File left out

- **WHEN** a `.go` file at the pin is absent from the destination directory of a `carry` entry
- **THEN** ledger:check fails naming the entry and the file as only at the pin

#### Scenario: File added

- **WHEN** the destination directory of a `carry` entry holds a `.go` file the pin does not have
- **THEN** ledger:check fails naming the entry and the file as only in the tree

#### Scenario: Destination names no path

- **WHEN** a `carry` entry's destination path does not exist in the tree
- **THEN** ledger:check fails naming the entry and the reason

#### Scenario: Adapted entry differs

- **WHEN** an `adapt` entry's files differ from the pin and no `carry` entry does
- **THEN** ledger:check passes

#### Scenario: No carried entry

- **WHEN** the ledger has no `carry` entry and SemStreams cannot be reached
- **THEN** ledger:check passes

#### Scenario: Fetch cut off at the bound

- **WHEN** a fetch still waiting on SemStreams is cut off, by the fetch bound or by an interrupt
- **THEN** the program exits 2, and no process it started for the fetch, git's transport helper included, is still
  running when it returns

#### Scenario: Pin cannot be read

- **WHEN** the ledger has a `carry` entry and SemStreams cannot be reached
- **THEN** the program exits 2 saying the pin could not be read and no entry was checked, and ledger:check fails

### Requirement: Pin difference command

`task ledger:diff -- <source_path>...` SHALL run the same program to compare each named entry and, with no argument,
every entry whose disposition is `carry` or `adapt`, in ledger order. For each covered file that differs it SHALL
print on standard output a unified diff from the rewritten pin text, labelled `pin/` followed by the file's
SemStreams path, to the tree file, labelled with its path. For a file on one side only it SHALL print
`only at the pin: pin/<path>` or `only in the tree: <path>`. An equal file prints nothing, so standard output is empty
when nothing differs. On standard error it SHALL print one line per entry: the `source_path`, the disposition, and
either the number of files compared (covered files present on both sides), differing, only at the pin and only
in the tree, or `not compared` with the reason.
The program SHALL exit 0 when it ran, whether or not files differ, and 2 when an argument names no entry,
when a named entry could not be compared, or when the ledger or the pin could not be read; through `task` only pass
or fail and the message are promised. It SHALL write nothing inside the repository.

#### Scenario: Unchanged package prints nothing

- **WHEN** every covered file of the named entry equals the pin
- **THEN** standard output is empty and the program exits 0

#### Scenario: Changed line is printed

- **WHEN** one line of a covered file differs from the pin
- **THEN** standard output holds a unified diff with the pin's line and the tree's line, and the program exits 0

#### Scenario: Unknown source path

- **WHEN** an argument is not the `source_path` of any entry
- **THEN** the program exits 2 naming the argument

#### Scenario: Entry that cannot be compared, no argument given

- **WHEN** no argument is given and an `adapt` entry has a file at the pin and a directory in the tree
- **THEN** standard error says the entry was not compared and why, and the program exits 0
