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
4. **The pin is frozen** (owner ruling, 2026-09-30). Repairs to ported code land in this repository behind a
   failing-first test and are not round-tripped through SemStreams. A SemStreams change made after the pin enters
   only as its own ledger row, with the source commit and a proving test. Nothing syncs automatically in either
   direction.
5. **Adapted code says so.** When a port is adapted rather than carried, its ledger row names the behavioral
   difference and the regression evidence. Do not leave a ported file that silently differs from its recorded source.

## Why

A later reader asking "where did this come from, and is it still what SemStreams shipped" must be able to answer from
the ledger alone. Without the full SHA and the frozen pin, SemEngine and SemStreams drift apart with no record of
which fix belongs to which side, and an upstream change can enter unreviewed.

## Status

The admission ledger is `docs/admission-ledger.yaml` (owner ruling of 2026-09-30, recorded on
[issue #6][setup-02-rulings]): a YAML list with one entry per SemStreams source path,
carrying the fields of rule 3 under the names given in the file's header comment. Its first entries record the
SemStreams files SETUP 02 adapts, carries, or reads and excludes; package rows are seeded from the measured dependency
closure during SETUP 03 and 04. Nothing has been ported yet. The schema is to be machine-checked by
`task ledger:check` inside `task verify`; that check does not exist yet and is built by SETUP 02 (change
`setup-02-isolated-harness`, task 1.3).

[setup-02-rulings]: https://github.com/C360Studio/semengine/issues/6#issuecomment-5921046663
