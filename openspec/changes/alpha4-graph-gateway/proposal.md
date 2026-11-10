# Proposal: alpha4-graph-gateway

Approved-by:

Track: `design` (re-admits a package the ledger excludes and adds an HTTP surface).

## Why

The owner re-admitted `gateway/graph-gateway` as a trimmed Adapt for `v0.1.0-alpha.4` (#567, ruling 2026-11-10).
SemSource's UI profile, SemDragon's config and SemTeams (semteams #406) query its GraphQL route; without it SemSource
cannot pin SemEngine. The consumer assertion is semsource PR #259.

## What Changes

Pending the architect inventory and design: a read-only GraphQL adapter over `graph.query.*` with the operations
`entity`, `entitiesByPrefix`, `entitiesByPredicate`, `pathSearch` and `trajectory`. No agentic port, no
`agentic_queries` dependency, no import of `agentic` or the excluded `gateway` parent package. The ledger row flips
from `defer-exclude` to `adapt` in the code PR.

## Capabilities

Pending the design.

## Impact

Pending the design. No code before the owner approves.
