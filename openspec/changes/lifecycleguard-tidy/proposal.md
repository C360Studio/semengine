# Proposal: lifecycleguard-tidy

## Why

Issue #196 (comment of 2026-10-16, moved from #153 item 6; owner, 2026-10-18: "Claim PR 1, the lifecycleguard tidy
from #196, as a draft and write its design"). `internal/lifecycleguard` spells one refusal several ways, the refusal
of a concurrent `Stop` has no test, and the cleanup is passed separately to `Start` and `Stop`, so graph-ingest passes
two different values. This is PR 1 of the order recorded on #282 (comment 6249934379, point 5), ahead of the
`Start`-context change (PR 2, ADR-113). This file is the claim; the inventory and design follow.

## What Changes

- To be written with the design.

## Capabilities

- To be named with the design.

## Impact

- `internal/lifecycleguard`, graph-ingest's two guard calls, and whatever the inventory finds. Addresses #196.
