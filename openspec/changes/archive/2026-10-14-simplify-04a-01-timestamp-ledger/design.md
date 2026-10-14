# Design: simplify-04a-01-timestamp-ledger

## Context

Issue #217 is PR 1 of Epic #223 (the pre-Change 3 simplification wave). Before claiming work, the owner confirmed two
scoping decisions under the Clarify gate:

1. **Zero conflict with open PR #206 (`claude/test-infra-audit`)**: trim `adapt` rows across
   `docs/admission-ledger.yaml` while leaving the three entries PR #206 touches (`scripts/run-integration-tests.sh`,
   `test/testinfra/integration_runner_contract_test.go`, and `natsclient`) untouched.
2. **Stop boundary**: run autonomously through worktree claim, OpenSpec change, implementation, local `task verify`,
   push, and post the cross-agent `Review request` comment on the draft PR.

## Decisions

### D1 Remove `timestamp.Parse(input any) int64` from `internal/timestamp`

- **Pin behavior (`pkg/timestamp/timestamp.go:87-154` at `8b99efe9`)**: `Parse(input any) int64` switches on `int64`,
  `float64`, `int`, `int32`, `string`, `time.Time`, and `*time.Time`, treating values `<= 1e12` as Unix seconds and
  values `> 1e12` as Unix milliseconds, and returning `0` on parse failure or unsupported types.
- **Callers**: `git grep -n "timestamp\.Parse"` across the repository shows zero production callers. The only pin caller
  (`message/base_message.go:272-284`) was replaced in `setup-04a-01-floor` by `wireMillis` (`message/base_message.go:244`).
  Because `internal/timestamp` is an internal package (`pkg/timestamp` moved to `internal/timestamp` in
  `setup-04a-01-floor` design D5), no external consumer can import it.
- **Change**: delete `Parse` (`timestamp.go:78-154`) and its unused `"strconv"` import; remove the `Parse` bullets and
  examples from `timestamp.go:14, 31-33` and `doc.go:13, 51-64, 103-105, 111-118`; remove `ExampleParse`
  (`example_test.go:17-35`) and `TestParse`, `TestParseEdgeCases`, `TestFormatRoundTrip`, `BenchmarkParseString`, and
  `BenchmarkParseInt64` (`timestamp_test.go:12, 134-265, 612-650, 683-697`).
- **Proving tests**: `go test -race -count=1 ./internal/timestamp/` and `task ledger:check` (with `pkg/timestamp`
  moved to `adapt`).

### D2 Concise `adapt` entries in `docs/admission-ledger.yaml`

- **Schema invariance**: every entry in `docs/admission-ledger.yaml` retains all ten required non-empty scalar fields
  (`source_path`, `source_sha`, `consumer_purpose`, `destination`, `contract`, `dependencies_and_side_effects`,
  `known_risks`, `proving_tests`, `owner`, `disposition`) enforced by `TestAdmissionLedger`
  (`internal/harness/contract/ledger_test.go`).
- **Content rule**: for adapted package rows (excluding `scripts/run-integration-tests.sh`,
  `test/testinfra/integration_runner_contract_test.go`, and `natsclient`), `contract` states the retained capability and
  behavioral adaptations from the pin, `known_risks` states dropped surface, context-root disposition, and open
  follow-up issues (`#n`), and `proving_tests` names the proving test files and suites without repeating line-by-line
  diff inventories that `task ledger:diff -- <source_path>` computes directly from the pin.
- **`docs/provenance.md` update**: update lines 75–80 to state that the ledger has four `carry` rows
  (`pkg/projection/contract`, `internal/semantictest`, `model`, and `storage`) and fourteen `adapt` floor packages now
  that `pkg/timestamp` is `adapt`.

### D3 Generated checks

Examples and existing deterministic unit/contract tests (`internal/timestamp`, `TestAdmissionLedger`, and `pindiff`)
suffice (`docs/testing.md`, "Decide whether generated checks are needed"): this change removes one dead function and
shortens YAML scalar strings without adding new branching logic or interacting inputs.
