# Proposal: simplify-04a-01-timestamp-ledger

## Why

1. `docs/admission-ledger.yaml` has grown to 3,418 lines (~274 KB) because `adapt` package entries carry long narrative
   recaps of line-by-line diffs and test assertions that `task ledger:diff -- <source_path>` already prints on demand
   from the pinned SemStreams tree (`8b99efe9`).
2. `internal/timestamp/timestamp.go:87-154` carries `Parse(input any) int64`, which accepts `any`, silently returns `0`
   on invalid input, and guesses between seconds and milliseconds using a `1e12` cutoff. Since `message` wire decoding
   was tightened in `setup-04a-01-floor` (`message-codec` spec, "Message timestamps travel as integer milliseconds"),
   `timestamp.Parse` has zero callers in this repository or in any named consumer (`docs/inventory-scope.md`).

Issue #217 (PR 1 of the pre-Change 3 simplification wave, Epic #223) removes dead `timestamp.Parse` and trims `adapt`
entries in `docs/admission-ledger.yaml` to concise contract/drop/risk summaries while leaving the three entries modified
by open PR #206 (`scripts/run-integration-tests.sh`, `test/testinfra/integration_runner_contract_test.go`, and
`natsclient`) untouched.

## What Changes

- **`internal/timestamp` dead-code removal**:
  - Delete `Parse(input any) int64` (`internal/timestamp/timestamp.go:87-154`) and its `strconv` import.
  - Remove `Parse` doc-comment examples and sections from `internal/timestamp/timestamp.go` (`:14, 31-33`) and
    `internal/timestamp/doc.go` (`:13, 51-64, 103-105, 111-118`).
  - Remove `ExampleParse` from `internal/timestamp/example_test.go` (`:17-35`) and `TestParse`, `TestParseEdgeCases`,
    `TestFormatRoundTrip`, `BenchmarkParseString`, and `BenchmarkParseInt64` from
    `internal/timestamp/timestamp_test.go` (`:12, 134-265, 612-650, 683-697`).
- **`docs/admission-ledger.yaml` diet**:
  - Change `pkg/timestamp` (`internal/timestamp`) from `carry` to `adapt`, recording the removal of `Parse(input any)`.
  - Trim bloated `adapt` and `repair-before-port` package entries in `docs/admission-ledger.yaml` so `contract`,
    `known_risks`, and `proving_tests` concisely state the retained contract, behavioral differences / dropped symbols,
    and proving test files/suites without duplicating `task ledger:diff -- <source_path>`.
  - Leave the three entries modified by in-flight PR #206 (`scripts/run-integration-tests.sh`,
    `test/testinfra/integration_runner_contract_test.go`, and `natsclient`) byte-identical so PR #206 merges without
    conflict.
- **`docs/provenance.md` sync**:
  - Update `docs/provenance.md:75-80` to reflect four `carry` rows (`pkg/projection/contract`,
    `internal/semantictest`, `model`, and `storage`) after `pkg/timestamp` moves to `adapt`.

## Capabilities

No normative capability requirements change (`skip_specs: true`). `harness-boundaries` › "Admission ledger" and
"Carried entries match the pin" continue to govern `docs/admission-ledger.yaml`.

## Impact

- `internal/timestamp/timestamp.go`, `internal/timestamp/doc.go`, `internal/timestamp/example_test.go`,
  `internal/timestamp/timestamp_test.go`.
- `docs/admission-ledger.yaml`, `docs/provenance.md`.
- Closes #217; addresses #223.
