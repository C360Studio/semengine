# Tasks: fix-415-alias-index

Design track: proposal, design, ADR-114, spec deltas for `graph-index` and `graph-transport-boundary`. Tests come
first; each failing test is shown failing on the commit before its fix, named with the output in
`notes/<task>.md`. Each step ends when `task verify` passes on a clean tree at its commit (#171). One task is one
developer's unit.

## 1. Design

- [x] 1.1 Inventory reviewed (`INVENTORY PASS`); ADR-114, proposal, design and spec deltas pass the independent
  pre-owner review; owner approval. Gate: `task spec:check`, `task docs:check`.

## 2. One spelling of the framework alias predicate (design D4)

- [x] 2.1 `TestCoreIdentityAliasIsDeclared` (`DiscoverAliasPredicates` contains `core.identity.alias`), failing
  before its fix (premise `index.go:385` is the only spelling; `git grep -n 'core.identity.alias' -- '*.go'`).
- [x] 2.2 `vocabulary.CoreIdentityAlias` registered in `init` as an identity alias; `isAliasPredicate` reads the
  discovered set only; the integration tests' `alias` constant uses the new name. Gate:
  `go test -race -count=1 ./vocabulary ./processor/graph-index`.

## 3. Owner-keyed layout and replacement (ADR-114 §1-2, design D1)

- [x] 3.1 `TestAliasReadImplementsReplacement` (dropped, changed, deleted, through the public verb) and the flipped
  ALIAS_INDEX assertions of `TestIndexReplacementThroughEveryIndex` (`index_integration_test.go:303`, `:316`, and the
  delete step), failing before its fix.
- [x] 3.2 Alias key helpers beside NAME's, `reconcileAliasIndex` through `reconcileOwnedRows`, the `alias` delete set;
  `updateAliasIndex` goes, and `alias` joins the `updates_total` success loops (`index.go:453-455`, `:624-626`); the
  read lists by hash prefix and parses with `entityAndPredicate`. Gate:
  `go test -race -count=1 -tags integration ./processor/graph-index`.
- [x] 3.3 Extend `TestMaximumLengthIDsAndPredicatesThroughPublicReads` (`query_integration_test.go:243-298`): register a
  194-byte alias predicate (`WithAlias(AliasTypeIdentity, 0)`), give the 256-byte `src` an alias under it, read it
  through the `alias` verb (answers `src`), then delete `src` and read again (answers no ID). Shown failing before its
  fix (the read after the delete still answers `src`); passes after 3.2. Gate: as 3.2.

## 4. Shared alias refused (ADR-114 §3-4, design D2)

- [x] 4.1 `TestSharedAliasIsRefusedAsAmbiguous` (two holders refused, class invalid, code `alias_ambiguous`, alias in
  the message; resolves to the other after one drops it; with E1 and E2 holding it again, E2 deleted, X answers E1,
  so the `*.<entity6>.*` delete filter leaves E1's row; one entity under two predicates resolves), failing before its fix.
- [x] 4.2 `graph.ErrorCodeAliasAmbiguous` with its doc comment; the read decides on distinct entities. Gate: as 3.2
  and `go test -race -count=1 ./graph`.

## 5. Case folding and any string (ADR-114 §3, §5, design D3)

- [x] 5.1 `TestAliasMatchFoldsCase` and `TestKVUnsafeAliasIsIndexed`, failing before its fix;
  `TestUnsafeAliasSkipIsLoggedAndCounted` is removed with the behaviour it pinned (named in notes).
- [x] 5.2 The plan keeps any non-empty trimmed alias; the skip, its warning, `aliasesSkipped` and
  `aliases_skipped_total` go; the read refuses an alias empty after trimming. Gate: as 3.2.

## 6. Malformed alias rows (spec "Malformed index rows are skipped and declared")

- [x] 6.1 Extend `TestMalformedIndexRowsAreSkippedLoggedAndCounted` with one malformed key under an alias prefix
  (alias read answers the well-formed row, warning names alias, counter alias 1), failing before its fix.
- [x] 6.2 The alias read skips through `skipMalformedRow("alias", …)`. Gate: as 3.2.

## 7. Wording (every text naming the #415 exception)

- [ ] 7.1 Bring to the new behaviour every text naming the exception, done when
  `git grep -n '#415' -- ':!openspec/changes/archive' ':!openspec/specs' ':!docs/adr/063-store-substrate-and-resolver.md'`
  returns only this change and ADR-114 (ADR-063 `:45`, `:147`, `:455`, `:469` cite SemStreams gh#415; the specs
  change at 8.1): `graph/query_response.go:12-13`; `processor/graph-index/doc.go:12`, `README.md:45-46`, `:72-73`,
  `:174`, `:183-185`, `index.go` and `query.go` comments; `query_integration_test.go:556-558` (comment naming
  `aliases_skipped_total`); `docs/adr/077-…md:22-23` (port note points to ADR-114);
  `.agents/contracts/semengine-developer.md:161`; `.agents/contracts/semengine-reviewer.md:231`;
  `.agents/skills/query-pattern/SKILL.md:38`. ADR-114 status to Accepted with the approval comment. Gate:
  `task docs:check`.
- [ ] 7.2 Ledger: `processor/graph-index` row `:3426-3427`, `:3433-3434` (item 9 no longer lists
  `aliases_skipped_total`), `:3435-3436`, `:3443` and its proving tests; `vocabulary` row (`:477-512`) names
  `CoreIdentityAlias`; `graph` row (`:2064`, error codes) names `ErrorCodeAliasAmbiguous`. Gate:
  `task ledger:diff -- processor/graph-index vocabulary graph`, `task docs:check`.

## 8. Archive

- [ ] 8.1 Spec sync into `openspec/specs/graph-index` and `openspec/specs/graph-transport-boundary`, archive as the
  last content commit; then `git grep -n '#415' -- openspec/specs` returns nothing. The PR body carries
  `Closes #415`, `implemented-by:`, `reviewed-by:`. Gate: `task spec:check`.
