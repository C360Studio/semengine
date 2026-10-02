# Source license and provenance

SemEngine is built by extracting admitted packages from SemStreams, which is MIT licensed. These rules keep every line
of ported code traceable to its origin and keep the license obligations intact. They bind before the first port.

## Rules

1. **The MIT notice stays.** `LICENSE` keeps "Copyright (c) 2025 C360", the holder named in SemStreams' license. A
   port never removes or rewrites that notice, and a file ported verbatim keeps any notice it carries.
2. **Every ported unit is traceable.** Each retained unit has one entry in the admission ledger recording its
   SemStreams source path and the full 40-character commit SHA it was taken from. The unit is a package when the
   package is ported, or a single source file when only that file's pattern is taken, as the SETUP 02 harness does.
   An abbreviated SHA or a branch name is not provenance: it can move or become ambiguous.
3. **No ledger row, no port.** Code without a row does not land. The row also records the consumer purpose,
   destination, contract, dependencies and side effects, known risks, proving tests, owner, and disposition (carry,
   adapt, repair before port, or defer/exclude), as `docs/setup-plan.md` specifies under "Package admission
   heuristic".
4. **The pin is frozen** (owner ruling, 2026-09-30). The pin is SemStreams at the commit a ledger row's `source_sha`
   records. Repairs to ported code land in this repository behind a failing-first test and are not round-tripped
   through SemStreams. A SemStreams change made after the pin enters only as its own ledger row, with the source
   commit and a proving test. Nothing syncs automatically in either direction.
5. **Adapted code says so.** When a port is adapted rather than carried, its ledger row names the behavioral
   difference and the regression evidence. Do not leave a ported file that silently differs from its recorded source.
   `task ledger:check`, part of `task verify`, holds every `carry` row to the pin: each `.go` file directly in the
   package, test files included, and every file under its `testdata` directory must match the pin. The only
   differences allowed are the module path in import paths and comments (`semstreams` becomes `semengine`, and a
   package that a `carry` or `adapt` row places at a new destination takes that path) and gofmt formatting, such as
   re-sorted imports. A string that holds the module path is not rewritten, so changing it is a difference.
   `README.md` and sub-directories are not compared. A package with any other difference is `adapt`. When the check
   fails, run `task ledger:diff -- <source_path>` to see the lines, then either restore the file or change the row to
   `adapt` and name the behavior that changed.

## Reviewing a port

The reviewer of a pull request that ports packages runs `task ledger:diff` at the commit under review, naming the
change's source paths:

```bash
task ledger:diff -- <source_path>...                   # read the differences
task ledger:diff -- <source_path>... | shasum -a 256   # the hash for the verdict
```

Standard output is a unified diff from the pin to the tree for each compared file that differs, and
`only at the pin: pin/<path>` or `only in the tree: <path>` for a file on one side; it is empty when nothing differs.
Standard error has one line per entry: how many files were compared, how many differ, and how many are on one side
only, or why the entry was not compared. `adapt` rows are printed here and never failed by `task ledger:check`, so
this command is where an adapted port's differences are read. The output depends only on the commit and the pin, so it
is not committed; the verdict comment records the commit, the command line, the per-entry lines, and the SHA-256 of
standard output.

## Why

A later reader asking "where did this come from, and is it still what SemStreams shipped" must be able to answer from
the ledger alone. Without the full SHA and the frozen pin, SemEngine and SemStreams drift apart with no record of
which fix belongs to which side, and an upstream change can enter unreviewed.

## Status

The admission ledger is `docs/admission-ledger.yaml` (owner ruling of 2026-09-30, recorded on
[issue #6][setup-02-rulings]): a YAML list with one entry per SemStreams source path,
carrying the fields of rule 3 under the names given in the file's header comment. Its first entries record the
SemStreams files SETUP 02 adapts, carries, or reads and excludes; package rows are seeded from the measured dependency
closure during SETUP 03 and 04. SETUP 03B (change `setup-03b-contract-boundary`, PR #21) approved the tier-0 port set
(65 packages at the pin, its `design.md` D4); each Slice 04A change adds the rows for the packages it ports. No
SemStreams package has been ported. SETUP 02 adapted six patterns into `internal/harness/` and
`scripts/test-integration.sh`, recorded four exclusions, and carried two scripts byte-identical
(`scripts/lint-test-ports.sh` and its fixture test). Change `flake-defense` then adapted those two scripts: the
inline exemption marker is gone and the guidance names no SemStreams file, so both rows read `adapt`. `task
ledger:check` (contract test T-B7, `internal/harness/contract/ledger_test.go`) machine-checks the schema inside
`task verify`, then compares every `carry` row with the pin (rule 5). No row is `carry` today, so the check fetches
nothing yet.

[setup-02-rulings]: https://github.com/C360Studio/semengine/issues/6#issuecomment-5921046663
