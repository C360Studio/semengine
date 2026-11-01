# Design: fix-415-alias-index

## Context

Inventory: PR #495 (`INVENTORY PASS`; base `7ac1e13`). The decision and its rejected options are ADR-114; this
document states what a caller observes and the test that proves each, not lock or join mechanics.

- Writes: `index.go:296`, `:317-328` (collect, with the skip `:319-325`), `:400-406` (alias outside replacement),
  `:516-531` (`Put(alias, entityID)`); delete sets `index.go:550-554` (incoming, predicate, name).
- Read: `query.go:227-257`; reply `graph/query_index_types.go:30` (one `CanonicalID`).
- Pattern mirrored: `owner_reconcile.go:22-102`; NAME `name_index.go:20-76`; NAME read `query.go:581-640` with tail
  parser `entityAndPredicate` (`query.go:208-225`) and `skipMalformedRow` (`query.go:691`).
- Alias predicates: `index.go:85-87` (`DiscoverAliasPredicates` once, at pool start), `:383-386` (plus the literal
  `core.identity.alias`); built-in registrations by `init` in `vocabulary/labels.go:16-22`, `lifecycle.go:16-26`.
- Error-code precedent for a per-entity invalid refusal: `graph/mutation_responses.go:88-102`
  (`ErrorCodeEmbeddingUnavailable`, class invalid).

## Goals / Non-Goals

Goals: `[A] -> [B] -> []` and delete on the alias read; the owner's three rulings; one spelling of the framework
alias predicate. Non-goals: graph-query (PR 9, #405); picking up alias predicates registered after Start (limit
stated in ADR-114); rebuild cost (#108); a cap on listing an alias many entities share (not measured, ADR-114 §4).

## Decisions

D1 Layout and ownership: ADR-114 §1-2. The key helpers sit beside NAME's and reuse `nameIndexKey` and
`encodePredicateToken`; the rows reconcile through `reconcileOwnedRows` with an empty marker and
`overwriteExisting=false`; delete adds an `alias` set with filter `*.<entity6>.*`. Adopter: nothing to learn.

D2 Read: ADR-114 §3. Trim and fold, list by prefix, parse with `entityAndPredicate`, skip and count malformed rows
under `index_type="alias"`, decide on distinct entities. Ambiguity: `errs.ClassifiedCode(errs.ErrorInvalid,
graph.ErrorCodeAliasAmbiguous, …)` with the alias in the message. Adopter: a caller of the alias verb must handle one
new code; the PR 9 consequence is ADR-114's.

D3 Any string: the plan keeps every alias whose trimmed form is non-empty; the skip, its warning, `aliasesSkipped`
and `aliases_skipped_total` go; the read refuses an alias empty after trimming (NAME answers a whitespace-only name,
`query.go:591-593`; alias does not). `updates_total{index_type="alias"}` moves from the per-alias increment
(`index.go:529`) into the success loops (`index.go:453-455`, `:624-626`), once per entity like name and predicate.

D4 One spelling: `vocabulary.CoreIdentityAlias = "core.identity.alias"`, registered in `init` with
`WithAlias(AliasTypeIdentity, 0)` and `WithDataType(DataTypeString)`; `isAliasPredicate` reads the discovered set
only. Risk: a test that calls `vocabulary.ClearRegistry` before graph-index starts loses the predicate; no
graph-index test calls it (`git grep -n ClearRegistry -- processor/graph-index`: none).

D5 Observe-only projection (`computeIndexProjection`, `index.go:465-490`) keeps raw alias parts; folding there would
change only the `reindex_events_total` instrument, so it stays.

## Proving tests

All integration tests drive `ENTITY_STATES` writes and read through the public `graph.index.query.alias` verb, with
literal expected answers; each is shown failing on `7ac1e13` in `notes/<task>.md`.

| Behaviour | Test |
| --- | --- |
| dropped alias answers none; changed alias answers the new one only; deleted entity's alias answers none | `TestAliasReadImplementsReplacement` |
| ALIAS_INDEX keys after A, B, none, delete are exactly the current rows | `TestIndexReplacementThroughEveryIndex` (line 316 flips) |
| shared alias refused naming it; resolves to the other after one drops it, and after one is deleted (the `*.<entity6>.*` delete filter leaves the other's row); one entity under two predicates resolves | `TestSharedAliasIsRefusedAsAmbiguous` |
| 256-byte entity ID and 194-byte alias predicate: the longest alias key is written, read through the verb, deleted | `TestMaximumLengthIDsAndPredicatesThroughPublicReads` (extended) |
| `SN-1` found by ` sn-1 ` | `TestAliasMatchFoldsCase` |
| `two words` indexed and found; other rows written | `TestKVUnsafeAliasIsIndexed` (replaces `TestUnsafeAliasSkipIsLoggedAndCounted`) |
| malformed alias row skipped, warned, counted | `TestMalformedIndexRowsAreSkippedLoggedAndCounted` |
| `core.identity.alias` discovered | `TestCoreIdentityAliasIsDeclared` (unit, `vocabulary`) |

Generated checks: not used. The law (one holder answers, two refuse, none answers none) is over a set of owners whose
cases the table enumerates; ordering between entity lanes is settled by awaiting each write's indexed revision.

## Risks / Trade-offs

Case folding merges aliases that differ only in case (owner's ruling 2). A widely shared alias costs one full prefix
listing per read (unmeasured). Restart replay of deletes is shared with the other owner-keyed stores (ADR-114).

## Migration Plan

None. Deployments wipe and start fresh; no reader of the raw layout remains.

## Open Questions

None open. The code name `alias_ambiguous` and class invalid are this design's choice under ruling 1; the owner may
rename it at approval.
