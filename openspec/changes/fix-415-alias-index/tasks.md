# Tasks: fix-415-alias-index

Design track: proposal, design, ADR-114, spec deltas for `graph-index` and `graph-transport-boundary`. Tests come
first; each failing test is shown failing on `7ac1e13` in `notes/<task>.md` before its fix. Each step ends when
`task verify` passes on a clean tree at its commit (#171). One task is one developer's unit.

## 1. Design

- [ ] 1.1 Inventory reviewed (`INVENTORY PASS`); ADR-114, proposal, design and spec deltas pass the independent
  pre-owner review; owner approval. Gate: `task spec:check`, `task docs:check`.
  Hold: OPEN-Q proposal awaits owner approval.

## 2. One spelling of the framework alias predicate (design D4)

- [ ] 2.1 `TestCoreIdentityAliasIsDeclared` (`DiscoverAliasPredicates` contains `core.identity.alias`), failing on
  `7ac1e13` (premise `index.go:385` is the only spelling; `git grep -n 'core.identity.alias' -- '*.go'`).
- [ ] 2.2 `vocabulary.CoreIdentityAlias` registered in `init` as an identity alias; `isAliasPredicate` reads the
  discovered set only; the integration tests' `alias` constant uses the new name. Gate:
  `go test -race -count=1 ./vocabulary ./processor/graph-index`.

## 3. Owner-keyed layout and replacement (ADR-114 §1-2, design D1)

- [ ] 3.1 `TestAliasReadImplementsReplacement` (dropped, changed, deleted, through the public verb) and the flipped
  ALIAS_INDEX assertions of `TestIndexReplacementThroughEveryIndex` (`index_integration_test.go:303`, `:316`, and the
  delete step), failing on `7ac1e13`.
- [ ] 3.2 Alias key helpers beside NAME's, `reconcileAliasIndex` through `reconcileOwnedRows`, the `alias` delete set;
  `updateAliasIndex` goes; the read lists by hash prefix and parses with `entityAndPredicate`. Gate:
  `go test -race -count=1 -tags integration ./processor/graph-index`.

## 4. Shared alias refused (ADR-114 §3-4, design D2)

- [ ] 4.1 `TestSharedAliasIsRefusedAsAmbiguous` (two holders refused, class invalid, code `alias_ambiguous`, alias in
  the message; resolves to the other after one drops it; one entity under two predicates resolves), failing on
  `7ac1e13`.
- [ ] 4.2 `graph.ErrorCodeAliasAmbiguous` with its doc comment; the read decides on distinct entities. Gate: as 3.2
  and `go test -race -count=1 ./graph`.

## 5. Case folding and any string (ADR-114 §3, §5, design D3)

- [ ] 5.1 `TestAliasMatchFoldsCase` and `TestKVUnsafeAliasIsIndexed`, failing on `7ac1e13`;
  `TestUnsafeAliasSkipIsLoggedAndCounted` is removed with the behaviour it pinned (named in notes).
- [ ] 5.2 The plan keeps any non-empty trimmed alias; the skip, its warning, `aliasesSkipped` and
  `aliases_skipped_total` go; the read refuses only an empty alias. Gate: as 3.2.

## 6. Malformed alias rows (spec "Malformed index rows are skipped and declared")

- [ ] 6.1 Extend `TestMalformedIndexRowsAreSkippedLoggedAndCounted` with one malformed key under an alias prefix
  (alias read answers the well-formed row, warning names alias, counter alias 1), failing on `7ac1e13`.
- [ ] 6.2 The alias read skips through `skipMalformedRow("alias", …)`. Gate: as 3.2.

## 7. Wording (every text naming the #415 exception)

- [ ] 7.1 Bring to the new behaviour, by `git grep -n '#415' -- ':!openspec/changes/archive'` returning only this
  change and ADR-114: `graph/query_response.go:12-13`; `processor/graph-index/doc.go:12`, `README.md:45-46`,
  `:72-73`, `:174`, `:183-185`, `index.go` and `query.go` comments; `docs/adr/077-…md:22-23` (port note points to
  ADR-114); `.agents/contracts/semengine-developer.md:161`; `.agents/contracts/semengine-reviewer.md:231`;
  `.agents/skills/query-pattern/SKILL.md:38`; `docs/admission-ledger.yaml:3426-3427`, `:3435-3436`, `:3443` and the
  row's proving tests. ADR-114 status to Accepted with the approval comment. Gate: `task docs:check`,
  `task ledger:diff -- processor/graph-index`.

## 8. Archive

- [ ] 8.1 Spec sync into `openspec/specs/graph-index` and `openspec/specs/graph-transport-boundary`, archive as the
  last content commit; the PR body carries `Closes #415`, `implemented-by:`, `reviewed-by:`. Gate:
  `task spec:check`.
