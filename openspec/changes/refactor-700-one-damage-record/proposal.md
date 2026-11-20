# Proposal: refactor-700-one-damage-record

Approved-by:

Track: `design`. Capabilities modified: `graph-ingest-recovery` (one requirement added; the behaviour it states is
today's). `entity-state-follower` keeps its requirements unchanged.

## Why

graph-ingest and the follower each keep a record of damaged entities (stored `ENTITY_STATES` values the stored-value
rule refuses): a map with its own lock, its own rule for when an entry is replaced or cleared, and its own sorted
sample of 10. The two rules already agree except at one boundary, and nothing makes them keep agreeing. The owner
asked for one type (#700) so damage is recorded, cleared and sampled one way.

## What Changes

- A new internal package, `internal/damage`, holds one record type: one entry per key with the revision and the
  refusal; a report keeps the entry at the highest revision; a clear drops an entry recorded below a given revision;
  it answers the count, one key's entry, and the entries in key order up to a limit.
- The follower's record becomes this type. Its methods, sentences and envelope do not change.
- graph-ingest's poison inventory becomes this type. Its Health message, gauge, query-cache invalidation, log lines,
  re-check against the stored bytes and debug listing stay graph-ingest's, built on the shared record.
- graph-clustering's maintainer keeps its `refused` set: it records input the maintainer refuses, not a damaged
  stored value, and its stored damage already goes to the follower's record.
- `graph-ingest-recovery` gains the requirement that states graph-ingest's record and clear rule.

## Questions for the owner

- **Q1. Which records share the type.** Recommended: the follower and graph-ingest. graph-clustering's stored damage
  is already on the follower's record (`processor/graph-clustering/component.go:282`); its maintainer set holds only
  what `graph.UnmarshalEntityState` refuses first, so it is empty in the composed component and shares no rule worth
  a type. The other choices: all three (the maintainer would keep a second copy of each revision beside its
  members, for no reader), or none (two short records stay, each tested).
- **Q2. "Existing damage tests pass unchanged".** The follower's and graph-clustering's do. graph-ingest's assertions
  do too, but 11 test lines in 6 files and one helper read graph-ingest's private record size and must read it
  through the new type instead; no assertion, expected value or message changes. Recommended: accept that.

## Out of scope

The maintainer's `refused` set; `pkg/graphview`'s poisoned map (a derived view, public API, its own order rule);
Health and log wording; the gauge's name and meaning; the debug JSON.

## Impact

Packages: new `internal/damage`; `graph/follower`; `processor/graph-ingest`. One PR on draft #701, which
closes #700. No open PR overlaps. No consumer change.
