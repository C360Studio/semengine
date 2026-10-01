# flake-defense

## Why

A flaky test is one that passes and fails on the same code. Owner direction, 2026-10-01 (issue #42): "flaky tests
have to not only be fixed but we have to stop the bleeding properly. flaky tests were part of what spelled doom for
semstreams".

On the day that was said, a test in this repository's own harness had failed in three CI runs on documentation-only
pull requests (issues #40 and #41). SemStreams filed 23 flaky-test issues and closed 21 of them one at a time, after
the fact. Fixing #40 does not stop the next one.

## What Changes

Nothing yet. This change is in its inventory phase. `inventory.md` records what lets a flaky test be born, merged,
and survive here today, and how SemStreams' flaky tests were born and closed. It deliberately holds no options,
recommendation, or design.

The next steps, in order, are: an independent review of the inventory; options with their costs, for the owner to
rule on; then the design, the spec changes, and the implementation.

## Capabilities

None yet. `.openspec.yaml` sets `skip_specs: true` only because no ruled design exists to write a spec change from.
The design removes that setting when it adds its spec changes.

## Impact

Documents only in this phase: this folder. No Go code, no workflow, and no Taskfile change.
