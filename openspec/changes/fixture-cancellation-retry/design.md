# Cancellation test oracle

## Scope and existing contract

The relevant path is `Fixture.Start` in `internal/harness/natsfixture/fixture.go`: it records each attempt, permits one retry after a mapped-port failure with a live parent and successful rollback, and joins the errors if the retry fails. `TestS1_2CancellationDuringIO` in `fixture_integration_test.go` controls cancellation at the start or connect hook. Its current `errors.As` call selects the first joined error, and its one-attempt assertion rejects a permitted earlier retry. `openspec/specs/nats-fixture/spec.md` already states the rule. The change does not alter production behavior or introduce a new reusable surface.

## Observable outcomes and proof

The two permitted histories are (1) cancellation during the first start or connect attempt and (2) a fully cleaned-up mapped-port failure with a live parent, followed by cancellation during connect on attempt 2. Keep the first history for both existing phases; add a connect case that injects exactly one initial mapped-port failure. Synchronize cancellation after the target hook has entered, as the current test does.

For each history, `Start` must satisfy `errors.Is(err, context.Canceled)`. The final attempt's typed `Error` must name the targeted phase and carry `context.Canceled` in `ParentErr`, with successful cleanup. Earlier errors may remain joined, so inspect the terminal error explicitly. The evidence record must have the expected one or two attempts, each recorded container must be absent by Docker inspection, and no further start call may occur after cancellation. The earlier retry is allowed only before cancellation. A test-owned counter at the start hook records whether a start call entered after cancellation, independent of the fixture's total call count.

## Test fidelity and verification

The fixed synchronization gives a finite, named history for each case; generated checks would add no relevant states. The failure oracle is the existing spec and the injected sequence, independent of the fixture's retry decision. Before repairing the assertions, add the deterministic retry/cancel case and run it through `task test:integration` to observe the old one-attempt/first-error assertion fail. Then repair the oracle and rerun that focused integration test and the required verification gates. Keep the unchanged test's SHA-256 baseline and report exact red/green commands and results. No production mutation or new helper API is involved.
