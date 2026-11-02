# Proposal: setup-04a-04-graph-roots-graph-query

Approved-by:

## Why

Change 4 of epic #9 ports the graph roots in the owner's order (#9 comment 6386856657); item 9, `processor/graph-query`,
is the last. It is 16,773 Go lines at the pin against a porting cap that now counts tests (#500 comment 6580240783), so
it splits. The owner, 2026-11-02: "graph-query is essential to our success - many lessons learned in semstreams gets us
pretty close but does not mean we can not improve from ground up."

## What Changes

- Design stage only so far: `inventory.md` records the pin, its readers, the open issues, and a proposed split into five
  pull requests, with the questions the owner rules on first. Nothing is decided and no code is written.

## Capabilities

- To be named by the design, once the owner has ruled on the split.

## Impact

- This change adds no code. The target state follows the owner's rulings on `inventory.md`.
