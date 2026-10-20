# Proposal: setup-04a-03-boot-service-base

## Why

Change 3 (`setup-04a-03-boot`) of epic #9 ports the boot-layer packages one per pull request (#159). The owner set the
order on 2026-10-19 (#9 comment 6270875286): `health`, `internal/logforwarderpolicy`, `component/flowgraph`, `config`,
`pkg/rulepack`, `composition`, then `service`. Five are merged (`config` in two parts, #322 and #329). `service` is
about 11,800 pin lines at SemStreams `8b99efe9` and is split by lifecycle owner (archived 04A foundation design,
`design.md:84-97` and D2 `:155-178`; owner choice 2026-10-20, #9 comment 6294662570). The split comes out of an
inventory of the package at the pin, for the owner to accept on #9, before any owner's design is written.

## What Changes

The owner accepted the split on 2026-10-20 (#9 comment 6296023694, option B): this change is PR 1 of 7, `BaseService`
and the shared files of `service`. Its design replaces the paragraphs below.

- An inventory of `service` at the pin: every lifecycle owner, what it starts and ends, what it imports from the
  port set and from outside it, the shape sweep, the pin ADRs it carries, and every open issue naming it (#157,
  #159, #160).
- A proposed split into one porting pull request per lifecycle owner, with its merge order; the component manager
  waits for `composition` (#338) and cites ADR-113.
- Once the owner accepts the split, this change becomes the first owner's porting change and is renamed to say so.

## Capabilities

- None yet; the first owner's change names its capability once the split is accepted.

## Impact

- `openspec/changes/setup-04a-03-boot-service/inventory.md` only, until the split is accepted.
