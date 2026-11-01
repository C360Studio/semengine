# ADR-114: ALIAS_INDEX Keys an Alias by Its Owner

## Status

**Proposed (2026-11-01).** Drafted on PR #495 (`Closes #415`) ahead of any code, as the owner's ruling on PR #414
(comment 6426254115, item 3, condition 1) requires: "the architect decides the alias key layout ADR-077 left open, as
a decision record". It becomes accepted when the owner approves the change `fix-415-alias-index` (its `Approved-by:`).
It records these owner rulings and adds the layout, the reasons and the consequences:

- 2026-11-01, #415 comment 6557704316: (1) an alias held by two or more live entities is refused as ambiguous, with
  an error naming the alias, and `graph.AliasData` keeps a single ID; (2) alias matching folds case as NAME's
  `normalizeName` does; (3) KV-unsafe aliases are indexed, and the skip, `aliases_skipped_total` and the read-side
  `invalid_request` refusal for them go. Layout direction: candidate A, mirroring NAME.
- 2026-11-01, #415 comment 6557766571: SemVocab support changes none of this; `core.identity.alias` is declared in the
  `vocabulary` package so graph-index stops spelling it as a second case.

It fills the one row ADR-077 left open (`docs/adr/077-bounded-owner-discovery-and-incoming-ownership.md:65`, `ALIAS |
variable raw alias key, entity in value | separate owner | unavailable by owner-key filter`); ADR-077's other rows and
its §2 rules stand. Tree cites are at `7ac1e13`; pin cites are at SemStreams `8b99efe9`, read from the pin tarball
(`gh api repos/C360Studio/semstreams/tarball/8b99efe9`). Mechanics belong to the change's `graph-index` spec delta.

## Context

An *alias* is a string an entity carries under a predicate the vocabulary marks as a resolvable alias (identity,
alternate, external or communication; never a display label, `vocabulary/registry.go:15-87`). The alias read
(`graph.index.query.alias`) answers which entity carries a given alias.

At the pin, and in this tree since the graph-index port, `ALIAS_INDEX` holds one key per raw alias with the entity ID
as its value, written by an unconditional `Put` (pin `processor/graph-index/component.go:1961`; tree
`processor/graph-index/index.go:524`). Nothing removes a row: replacement skips alias (pin `:1502-1504`, tree
`index.go:400-401`), entity delete skips it (pin `:2044-2048`, tree `index.go:533-537`), and the pin's
`DeleteFromAliasIndex` (`:2236`) has no production caller. The pin recorded the defect as DI-02 ("same-alias
last-writer collision, stale alias after predicate/entity removal", pin
`docs/proposals/post-g-foundation-remap-inventory.md:197`) and deferred it
(`openspec/changes/archive/2026-08-14-post-g-tag-safety-closeout/disposition-ledger.md:26`). This tree's
`TestIndexReplacementThroughEveryIndex` asserts the stale row today (`index_integration_test.go:316`; passed on
`7ac1e13`, `go test -tags integration`).

What a reader meets (pin code reading, not run): graph-query's `entityByAlias` returns a live entity that dropped the
alias, silently (pin `processor/graph-query/query.go:156-177`); GraphRAG's partial-ID resolver takes any hit and skips
its suffix step (pin `processor/graph-query/entity_resolver.go:27-40`). Two entities with one alias answer whichever
wrote last.

The other four graph-index stores already keep `[A] -> [B] -> []`: each key names its owner at fixed token positions,
and `reconcileOwnedRows` (`owner_reconcile.go:22-102`) lists the owner's rows by a fixed-arity filter, deletes the
ones current state lacks and writes the ones it adds. NAME is the open-content case: its key is
`hash(name).entity6.hex(predicate)`, hashed because a name may hold any bytes (`name_index.go:17-41`; ADR-078 §4,
`docs/adr/078-raw-canonical-predicate-membership-keys.md:103-105`). An alias is open content too: a valid KV key
allows dots, so its arity varies (`natsclient/kv_key_contract.go:15-17`), and `"drone.local"` is a documented alias
(`vocabulary/registry.go:72`).

## Decision

1. **Layout.** An `ALIAS_INDEX` key is `nameIndexKey(alias) + "." + entityID + "." + encodePredicateToken(predicate)`:
   the hex sha256 of the trimmed, case-folded alias (`name_index.go:20-28`), the six entity tokens, and the asserting
   predicate. Eight tokens, fixed. The value is an empty marker; nothing is read from it. At the 256-byte entity bound
   the key's maximum equals NAME's, which ADR-077 bounds at 710 bytes (pin ADR-077 `:59`).
2. **Owner.** The entity that carries the alias owns the row. Owner discovery is `*.<entity6>.*`, NAME's filter
   (`name_index.go:39-41`), within ADR-077 §2. Every index write reconciles the entity's alias rows through
   `reconcileOwnedRows` from current state; entity delete lists and removes them with the other families
   (`index.go:550-554`).
3. **Read.** The alias is trimmed and case-folded, then the read lists keys under its hash prefix (keys only; no
   value read). Malformed keys are skipped and declared as for NAME. No row: no ID. Rows of one entity, under any
   number of predicates: that entity. Rows of two or more distinct entities: refused, class invalid, code
   `alias_ambiguous` (`graph.ErrorCodeAliasAmbiguous`), the message naming the alias. `graph.AliasData` keeps one ID.
4. **Read bound.** Two distinct owners decide the answer, so the read hydrates nothing and needs no
   `nameQueryMaxHydration`-style cap. Collecting the listing observes the store's operation timeout
   (`natsclient/kv.go:616-623`) and the handler's `queryTimeout`; after collection, `keysByFilter` keeps its existing
   watcher stop and drain (`kv.go:699-706`), so no end-to-end return bound is promised. `KeysByPrefix` returns its
   whole snapshot (`kv.go:594-601`). Stopping after two owners is possible only by building a second key lister in
   graph-index on `KVStore.Watch` (`kv.go:725`), which would copy `keysByFilter`'s snapshot-and-drain handling
   (`kv.go:652`), the code #461 and draft #494 are still settling; one listing with one implementation is kept
   instead. The listing cost of an alias many entities share is not measured.
5. **Any string.** An alias is indexed whatever its bytes; a value empty after trimming is not an alias: it is not
   indexed, and a request for it is refused as `invalid_request`. NAME differs here: it refuses only `""` and answers
   a whitespace-only name (`query.go:591-593`). The KV-unsafe skip and `aliases_skipped_total` go. The alias rows
   count in `updates_total{index_type="alias"}` once per indexed or deleted entity, as name and predicate do (the
   success loops `index.go:453-455`, `:624-626` gain `alias`; the per-alias increment at `index.go:529` goes with
   `updateAliasIndex`).
6. **One spelling.** `core.identity.alias` is declared once in `vocabulary` (identity type) and reaches graph-index
   through `DiscoverAliasPredicates`, like every other alias predicate; the literal at `index.go:385` goes.
7. **No migration.** Rows in the raw layout are not read or converted; deployments wipe and start fresh.

## Alternatives rejected

- **`hash(alias).entity6`** (no predicate token): a second tail grammar beside the one NAME and INCOMING share
  (`query.go:208-225`), for a 321-byte bound instead of 710; it also drops which predicate asserted the alias.
- **Keep the raw key, check the entity on read**: a shared alias still answers whichever entity wrote last, so once
  that one drops it the other holder is lost; stale keys pile up with no TTL to remove them (ADR-068).
- **One key per alias holding the set of owners, under compare-and-set**: every holder's lane writes the same key,
  breaking the invariant each owner-keyed row rests on (`name_index.go:31-32`: written "by its entity's lane alone")
  and adding a retry loop.
- **Owner first, `entity6.hash(alias).hex(predicate)`**: owner discovery becomes a prefix, but the hot read, by alias,
  becomes a leading six-token wildcard on every query.
- **Raw alias with the owner beside it** (`alias.entity6`, `entity6.alias`): variable arity around the owner, which
  ADR-077 §2 forbids.

## Consequences

- The alias read keeps `[A] -> [B] -> []`: a dropped or changed alias, or a deleted entity's alias, stops resolving
  once graph-index has indexed the revision. The `graph-index` and `graph-transport-boundary` exceptions for #415 go.
- Matching is case-insensitive and ignores surrounding space: `SN-1` and `sn-1` are one alias, and held by two entities
  they are refused as ambiguous.
- New exported surface: `graph.ErrorCodeAliasAmbiguous`, whose contract line is the `graph-index` requirement "An
  alias resolves to its one current holder", first read in this repository by graph-query; and
  `vocabulary.CoreIdentityAlias`, the predicate constant, read by graph-index through `DiscoverAliasPredicates` and
  offered to products that write the framework alias (contract line: "Every declared alias predicate is indexed").
  Removed: the `aliases_skipped_total` collector.
- **graph-query (change 4 PR 9)** must stop swallowing alias-verb errors. At the pin `entityByAlias` falls through to
  treating the input as an ID on any error (pin `query.go:166-177`) and the resolver returns "no match" (pin
  `entity_resolver.go:62-65`). PR 9 propagates `alias_ambiguous`, `index_not_ready` and `internal` as classified
  refusals; #405 already requires that of the resolver. Not found keeps falling through.
- **Limit, not changed here:** graph-index reads the alias predicate set once, in `startIndexPool`
  (`index.go:85-87`); a predicate registered after Start is not indexed until restart. No rule or test orders
  vocabulary registration before Start (AGENTS.md § 1's registration rule covers payloads and components, not
  vocabulary); a late registration is untested.
- **Limit shared with every owner-keyed store, not measured:** whether a restart's replay delivers delete markers for
  entities deleted while graph-index was down (`graph/follower/follower.go:265` handles markers it receives).
- Rebuild on start reconciles one more owner filter per entity (#108 measures rebuild cost).

## Related

ADR-077 (fills its ALIAS row), ADR-078 (§4, hashing for open content), ADR-068 at the pin (DI-02, not carried);
issues #415, #405, #108, #170, PR #414 (graph-index port), PR #495.
