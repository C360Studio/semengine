# carry-check

## Why

A ledger row with `disposition: carry` says a SemStreams package was ported unchanged. Nothing checks it:
`task ledger:check` reads the ledger's fields and opens no ported file (issue #52; `inventory.md` G1 to G4). PR #48
is the first port, about 60,000 lines in 15 packages, and its review is held "on the full diff". Without a command,
the reviewer reads lines that did not change, or trusts the rows.

## What Changes

- **`task ledger:diff -- [<source_path>...]`** prints what a ported package changed against SemStreams at the row's
  `source_sha`, after the change of import path that porting makes. An unchanged file prints nothing.
- **`task ledger:check` also checks `carry` rows.** It fails when a `carry` row's Go files or test data differ from
  the pin, are missing, or are extra. Test files are included. `adapt` rows are printed by the first command and
  never failed.
- **The pin is fetched** from the public SemStreams repository when a `carry` row exists. Nothing is recorded beyond
  the row's `source_sha`. The ledger on `main` has no `carry` row, so nothing is fetched until a port adds one.
- **Import paths of moved packages** are read from the ledger's own `carry` and `adapt` package rows, so a package
  that moves under `internal/` does not turn its importers into `adapt` rows. Only import paths and comments are
  rewritten; a changed string literal always shows as a difference.
- **Docs**: `docs/provenance.md` says how a port is reviewed with the command; the ledger header, `docs/testing.md`,
  `docs/repository-map.md` and the `AGENTS.md` rule index name the check.

Not in this change: any row's disposition, any check on `adapt` rows, a review of PR #48, a new CI job, a cache, and
anything in `.agents/contracts/`.

## Capabilities

### Modified Capabilities

- `harness-boundaries`: three requirements added ("Comparison with the pin", "Carried entries match the pin", "Pin
  difference command"). There is no new capability, so no Purpose is written; the existing one covers the ledger.

## Impact

- A Go command under `internal/harness/`; `Taskfile.yml` (`ledger:check`, new `ledger:diff`); one contract test.
- Once a `carry` row exists, `task verify` and CI make one unauthenticated `git fetch` from `github.com` per
  distinct `source_sha` (2.4 s measured). `design.md` declares this and five other costs; the owner accepted the
  fetch on #52 (comment 5951926749). It gates section 3 of PR #48 and merges before it. PR #56 merged first.
