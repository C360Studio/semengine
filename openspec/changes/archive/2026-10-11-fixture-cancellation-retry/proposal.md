# Fixture cancellation after a permitted retry

## Why

`TestS1_2CancellationDuringIO/connect` assumes that cancellation always occurs on attempt 1. A real mapped-port
failure can occur first, while the parent context is live. The fixture is allowed to replace that container, then
cancellation can occur during connect on attempt 2. `Start` joins both errors, so `errors.As` selects the earlier
mapped-port error and the test rejects a valid cleanup history. This failed on PR #118 at `829e2e5` and in a focused
repeat on the same tree. It is tracked as [flake #179](https://github.com/C360Studio/semengine/issues/179).

## What changes

Repair this integration test's oracle. Exercise cancellation on the first attempt and after one deliberately injected
mapped-port failure. Find the terminal cancellation error, check that every recorded container is absent, and reject
any start call after cancellation. Preserve the existing fixture implementation and its error contract.

## Capabilities

No capability changes. The current `nats-fixture` requirement "Phased start with typed failure" already permits one
mapped-port replacement only while the parent is live with successful cleanup; its cancellation scenario and
requirement rule out replacement after cancellation.

## Impact

This OpenSpec change covers only `internal/harness/natsfixture/fixture_integration_test.go`. PR #118 separately
carries its dependency and Docker image pin updates. The test correction needs no production API or spec delta.
