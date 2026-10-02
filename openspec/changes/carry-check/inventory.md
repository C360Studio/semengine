# Inventory: carry-check

base: `c3ebb73` (`origin/main`, 2026-10-02; re-based for design review round 1). PR #48's branch
`claude/setup-04a-01-floor` was read at `dcb301c` with `git show` (re-checked at `3439494` in review round 2:
no shared file), without checking it out. SemStreams was read only
from a scratch fetch of the two pins (`git fetch --depth 1 https://github.com/c360studio/semstreams.git <sha>` into
the session scratchpad); no sister checkout was read or used as a git target (`docs/inventory-scope.md` rule 4).

**Question.** What checks that a ledger entry with `disposition: carry` is what SemStreams shipped at its
`source_sha`, and what would a command that prints a ported package's difference from the pin have to cover?
**Repositories read:** semengine; semstreams at `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` and
`5457b3458936f668b71d2fea061f67f8d7d01e67`.

The measurement scripts are `trial/measure.sh` and `trial/trial.sh` beside this file in the scratchpad; they are
evidence for the review and are not committed.

## 1. The claimed gap

- G1. `Taskfile.yml:64` — `- go test -count=1 -run '^TestAdmissionLedger' ./internal/harness/contract/`: this is all
  `task ledger:check` runs.
- G2. `internal/harness/contract/ledger_test.go:17` — `func TestAdmissionLedger(t *testing.T) {` and `:63` —
  `func TestAdmissionLedgerSchemaSensitivity(t *testing.T) {`: both call `ledgerViolations` (`:115`), which reads
  field presence, the SHA shape (`:170`) and the disposition set (`:109`). It opens no ported file.
- G3. Searches that came up empty on `c3ebb73`, outside `openspec/changes/archive` and the ledger itself:
  `git grep -n -i "byte-identical\|byte-for-byte\|git archive\|ls-remote\|semstreams.git"` returns one line of prose,
  `docs/provenance.md:40` — `…and carried two scripts byte-identical`. No code compares a file with SemStreams.
- G4. `grep -c "disposition: carry" docs/admission-ledger.yaml` is 0 (8 `adapt`, 4 `defer-exclude`): the gap has
  never been exercised.

## 2. Every current spelling of the fact "this file is what the pin has"

- S1. The claim: `docs/admission-ledger.yaml:14` — `#   disposition  one of carry | adapt | repair-before-port |
  defer-exclude`, and `docs/provenance.md:22` — `5. **Adapted code says so.** …Do not leave a ported file that
  silently differs from its recorded source.`
- S2. The recorded source: `docs/admission-ledger.yaml:6` — `#   source_sha  full 40-character lowercase commit SHA
  the file was read at`. A commit SHA fixes the content of every file at that commit.
- S3. Where it landed: `docs/admission-ledger.yaml:8` — `#   destination  SemEngine path it lands in, or none`.
- S4. One reader of the ledger exists: `ledgerViolations` (`ledger_test.go:115`), in a test file. A tool that reads
  entries is a second reader; the schema (which fields, which values) must stay in the first.

## 3. Adjacent claims

- A1. `openspec/specs/harness-boundaries/spec.md:118` — `### Requirement: Admission ledger`: the schema requirement;
  its Purpose (`:3-6`) already says the capability keeps "every reused SemStreams file traceable in the admission
  ledger".
- A2. Owner ruling Q1 on #9 (comment 5941920346, item 1): "A ported package with repaired tests gets an `adapt`
  ledger row."
- A3. Owner ruling Q17 (`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:653-657`):
  "SemEngine packages are internal by default… Each ledger row's `destination` fixes the package path under that
  rule". Go treats a package as internal only under a directory named `internal`, so a package made internal moves
  and its import path changes by more than the module path.
- A4. `AGENTS.md:74` — the index row `| A ported package has an admission-ledger row | … | task ledger:check for the
  schema of the rows present; that a ported package has a row, and what the row says, are review only |`.
  `AGENTS.md:62`: "A change that can turn a "review only" row into a failing command should."
- A5. `docs/testing.md:97` — `scripts/merge-check.sh, which CI's merge-check job runs and task verify does not,
  because it reads GitHub.` Same statement at `.github/workflows/ci.yml:65-66`. `task vuln` (`Taskfile.yml:69`,
  `govulncheck`) is a verify step that already reads the network.
- A6. `openspec/specs/merge-gate/spec.md:13-16`: every test without a build tag runs seven times per `task verify`
  (once in `test:unit`, once in the integration lane, five times in `test:repeat`).
- A7. `AGENTS.md:113-114` (merge rule): a known flake is "never a network fetch that did not answer".
- A8. `docs/testing.md:73` — `The packages under internal/harness/ are test-only; a contract test refuses any import
  of them from production code.` The "Import graph" requirement (`harness-boundaries/spec.md:10-11`) allows
  `gopkg.in/yaml.v3` in non-test Go files only under `internal/harness/`, so a Go tool that reads the ledger lives
  there.

- A9. `docs/admission-ledger.yaml:139-142` — a `defer-exclude` row (`source_path:
  test/testinfra/integration_runner_contract_test.go`) whose `destination` is `internal/harness/runner (four focused
  tests written fresh)`: a directory that exists and holds nothing taken from that source. A row's destination
  existing does not show that the source moved there.

### Open pull requests that overlap

From `gh pr list --state open --json number,title,changedFiles,files`, 2026-10-02, on `c3ebb73`. No pull request has
more than 100 changed files, so each list is complete.

- P56. PR #56 `claude/pbt-rules` (draft, `fdfdaa8`, `changedFiles` 4): `docs/testing.md`, `AGENTS.md` and two role
  contracts. Its `docs/testing.md` hunk at `-98,7 +98,8` sits directly under `docs/testing.md:94-97`, the paragraph
  this change's task 5.3 edits; its `AGENTS.md` hunk at `-87,6 +87,9` adds index rows below the row this change
  edits (`AGENTS.md:74`). The `docs/testing.md` edits touch. Order: PR #56 merges first.
- P48. PR #48 `claude/setup-04a-01-floor` (draft, `dcb301c`, `changedFiles` 22). No file overlap: `git diff --stat
  origin/main...origin/claude/setup-04a-01-floor -- docs/admission-ledger.yaml internal/harness/contract Taskfile.yml
  scripts .github docs AGENTS.md` is empty. Capability overlap: its delta
  `openspec/changes/setup-04a-01-floor/specs/harness-boundaries/spec.md` MODIFIES "Import graph"; this change ADDS
  requirements to the same capability and modifies none. Order: this change merges before PR #48's section 3. Text
  it carries that this change touches:
  - `design.md:70` — "Ported files keep their pin paths under the SemEngine module."
  - `design.md:239-241` — "`destination` per D16 — public for the eight packages SemSource imports directly…
    internal for the other eight". Read with A3, this and `:70` cannot both hold for a package made internal.
  - `design.md:150` and `tasks.md:101-102` (task 2.8): `internal/semantictest` is rehomed at
    `internal/harness/semantictest`, "byte-for-byte except package path and import rewrites".
  - `design.md:245-248`: `adapt` for seven packages, "`carry` for the other eight" (`pkg/platform`, `pkg/security`,
    `pkg/timestamp`, `pkg/errs`, `vocabulary`, `pkg/types`, `pkg/projection/contract`, `message`).
  - `tasks.md:110-116` (section 3 preamble): "the ledger row is written first and `task ledger:check` passes; the
    package and its `_test.go` files are copied from the pin… with the module path rewritten".
  - `tasks.md:147-148` (task 3.6): "`message` row `carry` (…its two tests that used `internal/semantictest` import
    the harness copy)".
  - `tasks.md:205-208` (task 3.9): "a diff stat against the pin per package". `tasks.md:275` (task 7.1): the hold
    "on the full diff". `tasks.md:252-254` (task 5.4): `scripts/cover-check.sh` gets a target list.
- Merged while this design was in work, so no longer claims: PR #39 (`c0a5515`) and PR #55 (`c3ebb73`). The claim
  branch (`2c229d9`) is two commits behind `main`.

## 4. Consumers at birth

- `task ledger:diff`: the developer of PR #48 section 3 and the reviewer of its tasks 3.10 and 7.1 (P48).
- The `carry` comparison in `task ledger:check`: the eight `carry` rows PR #48 will add, and a `carry` row for
  `internal/semantictest` if PR #48 writes one.
- Moved-package handling in the rewrite: `message/payload_test.go:10` and `message/triple_helpers_test.go:9` at the
  pin (M7), and every importer of a package made internal under A3.
- The remote and the fetch bound as inputs set from the environment: the test that runs the real program against a
  local repository with a short bound. No flag or cache is proposed.

## 5. The problem shape

A repository-wide check with a paired sensitivity test, plus a script-like tool tested against a stand-in for an
outside service. Closest instances: `TestAdmissionLedger` and `TestAdmissionLedgerSchemaSensitivity`
(`ledger_test.go:17,63`); `internal/harness/contract/mergecheck_test.go:134`, which runs `scripts/merge-check.sh`
against canned GitHub answers. The design adopts both: a sensitivity table that plants each violation, and a local
fixture repository standing in for SemStreams. It establishes no new reusable primitive, so there is no adoption
sweep: later Slice 04A changes come under the check by writing a `carry` row, with nothing to adopt.

## 6. Not triggered

- Collision table: the change adds no durable, communication or runtime-coordination primitive.
- Intent check: no boundary (port set, tier, exclusion, deferral) is set or moved.
- Surface audit, returning guidance and the pin probe: the change ports no package. The trial in section 8 is the
  evidence for every statement here about the pin's files.

## 7. Adopter seam

No surface is reached from outside this repository. The people who carry it are the developer who ports a package
and the reviewer of that port.

1. What they must know: that a `carry` row is compared with the pin; that `destination` starts with the path.
2. If they learn neither: a `carry` row over edited files, or with a destination that names no path, fails
   `task verify` with a message naming the row and the file. An `adapt` row is never failed.
3. Where they find out: a failing command, before push. Not a document.
4. What they should have to know: nothing more. The rewrite is derived from the ledger's own `source_path` and
   `destination` values, so there is no list of moved packages to keep.

## 8. Measurements

- M1. `gh repo view c360studio/semstreams --json visibility` is `PUBLIC`. `curl` with no token on
  `api.github.com/repos/c360studio/semstreams/commits/8b99efe9…` returns 200. `git ls-remote` with no credential
  helper and the global and system git config disabled answers.
- M2. Under the same conditions `git fetch --depth 1 https://github.com/c360studio/semstreams.git 8b99efe9…` took
  2.4 s and stored 17 MB (4,679 files in the tree). The SHA `5457b345…` used by the twelve rows on `main` fetches the
  same way.
- M3. The 15 floor directories at the pin: 222 `.go` files directly in them. Seven have a `README.md`. `pkg/types`
  has `testdata/` (2 files). `vocabulary` has eight sub-package directories. No other non-Go file.
- M4. The module path `github.com/c360studio/semstreams` (pin `go.mod:1`) is on 111 lines in 83 of the 222 files: 94
  import lines, 10 comment lines and 7 lines inside string literals. The seven are all in
  `natsclient/consumer_policy_callsite_test.go` (`:199, :365, :371, :377, :383, :441, :497`). In the eight planned
  `carry` packages the non-import lines are four, all comments: `pkg/security/doc.go:138-139`,
  `pkg/timestamp/doc.go:127`, `pkg/errs/doc.go:231`. In the ten comment lines the path is followed by `]` (7) or `"`
  (3). No file under a `testdata` directory holds the module path.
- M5. Replacing the module path and then undoing it restores all 222 files byte for byte. No pin file in the 15
  directories contains `semengine`. The module path is never followed by a character other than `/` or a closing
  delimiter (no `semstreams-ui` style collision).
- M6. `gofmt -l` (go1.26.4) reports no pin file in the 15 directories, and none after the module-path replacement.
  After a trial move of seven packages under `internal/`, it reports six files whose imports it would re-sort
  (`message/base_message.go`, `pkg/tlsutil/tlsutil.go`, `metric/handler.go`, `metric/handler_test.go`,
  `payloadregistry/registry.go`, `natsclient/client.go`).
- M7. Imports, inside the 15 directories, of a package that PR #48 moves: `internal/semantictest`, twice
  (`message/payload_test.go:10`, `message/triple_helpers_test.go:9`).
- M8. `pkg/retry`: the replacement touches no line. `pkg/timestamp`: two lines (`doc.go:127`,
  `example_test.go:7`). Both build in a scratch module named `github.com/c360studio/semengine`. A one-line edit
  planted at `timestamp.go:40` is found at line 40 after the replacement is undone.
- M9. This repository's markdownlint configuration on the pin's READMEs: `vocabulary` 9 findings, `pkg/types` 3,
  `message` 1 (planned `carry`); `pkg/retry` 5, `metric` 24, `pkg/cache` 1, `natsclient` 13.
- M10. `pkg/types/entity_id_prop_test.go` imports `pgregory.net/rapid`; `grep rapid go.mod` is empty on `main` and on
  PR #48's branch.
- M11. Lines that say "SemStreams" outside the module path in the eight planned `carry` packages: 69
  (`vocabulary` 50, `message` 16, `pkg/errs` 2, `pkg/platform` 1); about 45 of them are inside string literals.
- M12. The ledger on `main`: twelve rows, all file rows at `5457b345…`. Nine destinations are not `none`: eight are
  a path followed by a note (`docs/admission-ledger.yaml:32, :64, :80, :95, :128, :142, :154, :173`) and one is a bare
  path (`:112`). Of the eight `adapt` rows, three have a file at the destination path (`:112, :154, :173`) and five
  have a directory.
- M13. A Go program that calls `os.Exit(2)` exits 1 under `go run` and 201 under `task` 3.51.1 (scratch module and
  Taskfile, `trial/exitcode`).
