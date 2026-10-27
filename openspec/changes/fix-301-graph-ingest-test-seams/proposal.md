# Proposal: fix-301-graph-ingest-test-seams

## Why

graph-ingest's production `Component` has four fields that exist only so tests can fake NATS: `waitForStreamInput`,
`consumeStream`, `watchEntityStates` and `subscribeRequests` (`processor/graph-ingest/component.go:67-84`). Production
never sets them, and each reader falls through to the real call when a field is nil. Two of the fields came from the
SemStreams pin; two were added during the port under a waiver, on the condition that #301 removes them. The repository
rule is that production code carries no back door (#186, #190).

The inventory (`inventory.md`, INVENTORY PASS in round 2) found the following:

- Most tests that use the fields can fail NATS on cue without them, in one of three ways:
  - a real server fault;
  - a log record seen through the component's own `deps.Logger`, which the tree already does
    (`lifecycle_integration_test.go:508-540`);
  - a direct call on a function that already takes its dependency as an argument.
- Two tests have no deterministic real cue: the callback that runs across a failed Start's rollback
  (`failed_start_callbacks_integration_test.go:210`, `:327`).

The owner ruled on 2026-10-27 (#301 comment 6453410228):

1. The two straddle tests are replaced by a unit test. It runs the message handler on a component whose failed Start
   was already rolled back and proves a Nak and no panic, with no field. natsclient keeps proving the straddle timing.
2. lifecycle-suite's "count every cleanup or external call" changes. An adapter may show a cleanup on the NATS server
   instead of counting it: after Stop, a published input stays pending and the request subjects have no responders.
   All four fields go.

## What Changes

- **Production builds a stream lane.** `setupJetStreamConsumer` builds an unexported `streamLane`, and its method is
  the consume callback production hands to natsclient. Tests build the same lane through the same constructor.
- **The tests move off the fields.** Each test now reaches its fault by a real server fault, a log-record cue, or a
  direct call on the boot sweep. The straddle pair is replaced per ruling 1.
- **The four fields and their nil-checks are removed.**
- **lifecycle-suite's "Observe adapter contract"** admits showing a cleanup on the server (ruling 2).
- No natsclient change (#170, ADR-112). No new exported surface.

## Capabilities

### Modified Capabilities

- `lifecycle-suite`: modified "Observe adapter contract".

## Impact

- Code: `processor/graph-ingest/component.go`, `consume.go`, `poison_inventory.go`.
- Tests: `config_fields_test.go`, `statement_metadata_test.go`, `guard_generation_test.go`,
  `lifecycle_adapter_test.go`, `lifecycle_integration_test.go`, `failed_start_callbacks_integration_test.go`,
  `long_apply_integration_test.go`, `readiness_integration_test.go`, `settlement_integration_test.go`, plus one new
  unit file, `stream_lane_test.go`.
- Retires #301 (`Closes #301`). #300, #270, #195 and #196 stay open; none touches these lines.
