# Await last-observation error clarification

## Why

Issue #51's owner ruling says Await reports the last observation's value and that observation's error,
if it had one. The implementation already does this, but the specification can be read as retaining
an earlier error. Existing tests do not cover an error followed by a clean final observation.

## What Changes

- Clarify the lifecycle-suite requirement "Probes retain observable state" and its Await scenario.
- Add one deterministic mixed-history test beside `TestAwaitReportsLastObservation`.
- Show the test detects a mutation that retains an earlier error, then restore Await exactly.
- Complete leg 1 of the owner's pickup demonstration and hand the draft PR to a fresh Claude session.

The owner direction is recorded in
[the ruling](https://github.com/C360Studio/semengine/issues/51#issuecomment-5951370165)
and [the pickup instructions](https://github.com/C360Studio/semengine/issues/51#issuecomment-5951968282).

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `lifecycle-suite`: clarify that the reported error belongs to the final observation.

## Impact

The Go change is confined to `internal/harness/probe/probe_test.go`. Await stays unchanged.
The delta carries the complete MODIFIED requirement; archive and current-spec synchronization belong to leg 2.
