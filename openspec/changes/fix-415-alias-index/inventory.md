# Inventory: #415, the alias index keeps answering dropped and deleted aliases

base: 7ac1e135966609942c0cb812cc1efa64361eac6a (branch `claude/alias-index-415`; the claim commit is empty on top of
`1d9d4bd`, `git diff --stat origin/main..HEAD` prints nothing). Pin: SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`,
read from `gh api repos/C360Studio/semstreams/tarball/8b99efe9` (unpacked under `/tmp/415-stage/pintar/src`, local
only) and `contents/<path>?ref=8b99efe9`. Never the local semstreams checkout.

Architect: claude (opus 5.5, effort medium). Inventory only: no target state, no recommendation beyond the one
sentence in section 6.

## 0. Question, scope, track

Question: what key layout should `ALIAS_INDEX` have so that the alias read honours `[A] -> [B] -> []`, and what does
the tree and the pin already say about it. Repositories read (per `docs/inventory-scope.md`): semengine (this tree),
semstreams at the pin (code + `docs/adr`), semsource at `e4febc0d2e23bf33811edc20b9a4159cf5275dc4` (symbol-level use),
semconnect at `d0d06e00bf05a545f30ceea798db1c2b1ee47d4f` (main, symbol-level use).

Track: **design** is required. The fix edits two capabilities: `openspec/specs/graph-index/spec.md:111-114` and
`openspec/specs/graph-transport-boundary/spec.md:358-382` (see 2.6). It also owes an ADR (ruling condition 1).

Not a porting change: it carries no package from the pin; it repairs `processor/graph-index`, ported by PR #414
(`5d6042a`). So the three pin-probe runs and the shape sweep of the architect contract do not apply; the pin's
behaviour is cited from pin code lines, and the port's behaviour from a test run on this tree (1.6).

Owner ruling (PR #414 comment 6426254115, item 3, read in full): answer (b), keep the pin's behaviour; the integrity
gate is waived for the alias read alone until #415 lands; condition 1, the architect decides the alias key layout
ADR-077 left open, as a decision record, before any code; condition 2, the fix lands as its own PR before change 4
PR 9 (`processor/graph-query`), not inside it.

## 1. The pin (probes at 8b99efe9)

1.1 Write path. `processor/graph-index/component.go`:

- `:1390-1399` — `type aliasIndexWrite struct { alias string ... }`, `aliasWrites []aliasIndexWrite` in the plan.
- `:1426-1436` — an alias is any string object of a predicate in `c.aliasPredicates` or `core.identity.alias`; a
  value failing `natsclient.ValidateKVLiteralKey` is skipped with `"alias index: KV-unsafe alias skipped"`.
- `:1502-1507` — `// ALIAS_INDEX has no owner-complete axis and is intentionally outside` / `// replacement
  reconciliation. ... alias retirement remains explicit.` then `c.UpdateAliasIndex(ctx, write.alias, plan.entityID)`.
- `:1961` — `c.aliasBucket.Put(ctx, alias, []byte(entityID))`: unconditional Put, key = raw alias, value = entity ID.
  Last writer wins; two entities with one alias overwrite each other (code reading; ordering across lanes not probed).
- `:2044-2048` — `DeleteFromIndexes` doc: `// at the retired target. ALIAS still has no owner-key axis.`
- `:2236` — `DeleteFromAliasIndex` exists; no production caller (`grep -rn 'DeleteFromAliasIndex\|UpdateAliasIndex'
  --include='*.go' . | grep -v _test.go` at the pin: only `:1506` calls `UpdateAliasIndex`, nothing calls the delete).
- `vocabulary/registry.go:498-511` — `DiscoverAliasPredicates` returns predicate -> `AliasPriority`; the pin's
  graph-index never reads the priority (only membership, `:1426`).
- No pin test asserts the stale-alias behaviour: `grep -n -i alias *_test.go | grep -i
  'stale|retain|remain|still|drop|delete'`
  in `processor/graph-index` returns only `DeleteFromAliasIndex` unit tests (`component_test.go:1003`,
  `index_operations_test.go:264`).

1.2 Read path (graph-index). `processor/graph-index/query.go:332-367` — `handleQueryAliasNATS`: refuses empty and
KV-unsafe aliases (`:343-348`), `ensureQueryReady` (`:349`), `c.aliasBucket.Get(ctx, req.Alias)` (`:353`), not found ->
`AliasData{CanonicalID: nil}` (`:355-358`), else `CanonicalID: &string(entry.Value)` (`:363-366`). No check that the
entity still carries the alias or still exists.

1.3 Readers of the alias verb at the pin (`grep -rn -e BucketAliasIndex -e ALIAS_INDEX -e query.alias -e
'Route("alias")'`
non-test):

- `processor/graph-query/router.go:26` — `"alias": "graph.index.query.alias"`.
- `processor/graph-query/query.go:143-200` — `handleQueryEntityByAlias`: `entityID := req.AliasOrID` (`:156`), asks the
  alias verb (`:159-172`), on a hit `entityID = *aliasResult.Data.CanonicalID` (`:175-176`), any alias error falls
  through to treating the input as an ID (`:166-171`), then fetches `entity` and returns its classified error
  unwrapped (`:188-194`). The issue's `:157-175` is this block (`:156-177` exactly).
- `processor/graph-query/entity_resolver.go:16-44, :48-75` — `resolvePartialEntityID` -> `resolveViaAlias`, called from
  `processor/graph-query/graphrag.go:422` and `:1016`. **Not named in #415**: a second graph-query reader.
- `gateway/graph-gateway/component.go:1111, :1219, :1844` — GraphQL `entityByAlias(alias: String!)` relays to
  graph-query; `gateway/graph-gateway` is a consumer adapter excluded by SETUP 03B D4.
- Test/e2e only: `test/e2e/scenarios/tiered_structural.go:1542`, `test/e2e/client/nats.go:511`; examples
  `examples/processors/iot_sensor/vocabulary.go:136`.

1.4 What a consumer meets at the pin (code reading of 1.3, not run):

- Alias dropped or changed while the entity lives: `entityByAlias(old)` returns the live entity that no longer carries
  it. Silent wrong answer.
- Entity deleted: `entityByAlias(alias)` resolves to the deleted ID, and the entity fetch returns the classified
  `entity_not_found` error (`query.go:188-194`). That is loud, not "no error" as #415's body says, but it also
  pre-empts the fallthrough at `:166-171` (an input that is itself a live entity ID is never tried). Correction to the
  issue text, for the design.
- Two entities sharing one alias: whichever Put last answers (unmeasured which).

1.5 semsource and semconnect (inventory-scope consumers):

- semsource `e4febc0d`: `cmd/semsource/run_test.go:261`, `cmd/semsource/subject_ownership_test.go:44` list
  `graph.query.entityByAlias` (tests of subject wiring). It registers only
  `vocabulary.WithAlias(vocabulary.AliasTypeLabel, 1)`
  (`source/ast/vocabulary.go:279`), which `CanResolveToEntityID` excludes; no `core.identity.alias`. So semsource
  writes no ALIAS_INDEX rows (search: `entityByAlias|query.alias|WithAlias|identity.alias|AliasType`).
- semconnect `d0d06e00`: the same search, non-test, returns nothing.

1.6 The port keeps the pin's behaviour (run on this tree, Docker 29.9.2, go1.26.10, nats.go v1.55.0 / nats-server
v2.15.1 per `go.mod:16-17`):
`go test -tags integration -count=1 -run 'TestIndexReplacementThroughEveryIndex|TestUnsafeAliasSkipIsLoggedAndCounted'
./processor/graph-index/`
-> both `PASS`, `ok ... 1.068s`. The first asserts
`processor/graph-index/index_integration_test.go:316` — `assertKeys(t, h, graph.BucketAliasIndex, sorted("alpha-1",
"beta-1")) // #415: a dropped alias stays`.

## 2. The current tree

2.1 Alias write: `processor/graph-index/index.go`

- `:296` `aliases []string` in `entityIndexPlan`; `:317-328` collect, with the declared skip `:319-325`
  (`c.metrics.aliasesSkipped.Inc()`, warning `"alias index: KV-unsafe alias skipped"`).
- `:382-386` — `isAliasPredicate`: `c.aliasPredicates[predicate]` or `"core.identity.alias"`; the priority value is
  unused (`:87` `c.aliasPredicates = vocabulary.DiscoverAliasPredicates()`).
- `:400-406` — `// ALIAS_INDEX has no owner axis and is outside replacement: a dropped alias keeps` /
  `// resolving until #415 lands ...` then a loop of `c.updateAliasIndex`.
- `:516-531` — `updateAliasIndex`: `c.aliasBucket.Put(ctx, alias, []byte(entityID))` (`:524`).
- `:533-537` — `deleteFromIndexes` doc: `ALIAS has no owner axis and is left (design D12, #415).`; `:550-554` delete
  sets
  are incoming, predicate, name only.
- `:442`, `:483-487` — `computeIndexProjection` already includes `alias:<predicate>:<alias>` parts.
- `:453-455` — success metrics loop names `"name", "predicate", "incoming", "outgoing"` (alias counted inside
  `updateAliasIndex`, `:529`).

2.2 Alias read: `processor/graph-index/query.go:227-257` — `handleQueryAlias`, same shape as the pin's (1.2) under the
port's envelope: `reply(revision, graph.AliasData{CanonicalID: &canonicalID})` (`:256`). Verb catalog
`graph/query_verbs.go:36` — `{Name: "alias", Subject: "graph.index.query.alias", Responder: ResponderGraphIndex}`.
Reply type `graph/query_index_types.go:29-32` — `type AliasData struct { CanonicalID *string \`json:"canonical_id"\` }`
(exported, one ID).

2.3 The pattern to mirror (owner-keyed rows, replaced from current state):

- `processor/graph-index/owner_reconcile.go:22-102` — `reconcileOwnedRows(ctx, indexName, bucket, ownerFilter,
  desired, overwriteExisting)`: validate filter and keys, `KeysByFilter(ownerFilter)`, dedupe, delete stale, put
  desired, join failures as transient `errIndexWritePartial`.
- NAME (open-content axis, hashed): `name_index.go:20-23` `nameIndexKey` = hex sha256 of case-folded trimmed name;
  `:33-35` `nameCompositeKey` = `nameHash + "." + entityID + "." + encodePredicateToken(predicate)`; `:39-41`
  `nameIndexEntityFilter` = `"*." + entityID + ".*"`; `:57-76` `reconcileNameIndex` (value JSON carries original-case
  name and priority, `overwriteExisting=true`). Read: `query.go:581-640` `handleQueryByName` lists
  `KeysByPrefix(nameIndexKey(name)+".")`, dedupes, bounds hydration (`nameQueryMaxHydration`), parses the tail with
  `entityAndPredicate` (`query.go:208-225`, exactly seven tokens).
- PREDICATE: `predicate_index.go:13-21` `predicate3.entity6`, filter `wildcardPositions(3) + "." + entityID`;
  `owner_reconcile.go:105-118`.
- INCOMING (owner is the source though the key starts with the target): `incoming_index.go:22-31`;
  `owner_reconcile.go:121-144`.
- Delete: `index.go:538-622` — lists every owned key for every family before the first delete (`:560-588`), then
  deletes; partial failure marks the entity failed and the repair loop retries.
- Read-side check precedent: `query.go:328-345` `entitiesWithValue` reads `ENTITY_STATES` (`c.entities.Get`) to keep
  only entities whose current state holds the value.

2.4 Key bounds: `natsclient/kv_key_contract.go:15` `MaxKVLiteralKeyBytes = 1024`, `:17` `MaxKVLiteralKeyTokens = 64`;
`ValidateKVLiteralKey` rejects empty tokens and `*`/`>` but allows dots (variable arity). Registered alias examples
contain dots: `vocabulary/registry.go:72` — `// Example: "ALPHA-1" (call sign), "drone.local" (hostname),
"mqtt-client-001"`.

2.5 Alias semantics: `vocabulary/registry.go:15-74` — resolvable types identity (owl:sameAs), alternate, external,
communication; label is not resolvable "ambiguous - many entities can share labels" (`:38`).
`vocabulary/predicates.go:311-313` — `AliasPriority defines conflict resolution order (lower number = higher priority)`;
no code reads it for ALIAS_INDEX (`git grep -n 'DiscoverAliasPredicates\|IsAlias' -- '*.go' ':!*_test.go'
':!vocabulary/*'`
-> only `index.go:87`).

2.6 Every live text that names the exception (`git grep -n '#415' -- ':!openspec/changes/archive'`, SemStreams gh#415
hits in ADR-063 excluded): `openspec/specs/graph-index/spec.md:113-114`;
`openspec/specs/graph-transport-boundary/spec.md:361-369`
and the whole requirement `:373-382` ("Alias replies may resolve stale aliases while issue #415 is open");
`graph/query_response.go:12-13`; `processor/graph-index/doc.go:12`; `README.md:45-46, :72-73, :183-185`;
`index.go:400-401, :537`; `query.go:227-228`; `index_integration_test.go:284, :316, :541`;
`docs/admission-ledger.yaml:3436, :3443`;
`docs/adr/077-...md:22-23` (port note); `.agents/contracts/semengine-developer.md:161`;
`.agents/contracts/semengine-reviewer.md:231`; `.agents/skills/query-pattern/SKILL.md:38`.
Tests that skip alias by name: `query_integration_test.go:301-305` (`TestQueryReadsImplementReplacement`, "every public
read but alias").

2.7 graph-query is not in the tree (`ls processor/` -> `graph-index graph-ingest`); no open PR claims it.

## 3. Decision records

Search: all 107 pin ADRs fetched to `/tmp/415-stage/pin/adr/`; `grep -l -i -e ALIAS_INDEX -e 'alias index' -e
AliasIndex`
-> 068 only; `grep -n -i alias` over 065/068/077/078; `grep -l -i -e 'key layout' -e 'owner filter' -e 'owner axis'`
-> 056 (superseded by 091; its "alias" is predicate aliases, `:927-932`, not ALIAS_INDEX), 065, 068, 077. Repo
`docs/adr` same search -> 077 only; `git grep -n -i alias docs/adr` -> 077 `:22-23, :65, :67, :211`, others are
unrelated "no compatibility alias" uses.

- **ADR-077** (pin and repo, decision text identical; repo adds the port note `:3-23`). Owner matrix
  pin `:42` / repo `:65` — `| ALIAS | variable raw alias key, entity in value | separate owner | unavailable by
  owner-key filter |`;
  pin `:44` / repo `:67` — `ALIAS ... are not made safe by this decision. ... Absence from this matrix is never
  permission for a bucket scan or best-effort deletion.`; §2 (pin `:48-52`) — owner discovery is approved only for a
  layout whose complete arity and every entity-bearing position are fixed; literal single-token wildcards; no `>`, no
  variable arity, no ownership inferred from a value; proof at the 256-byte entity bound; pin `:188` / repo `:211` —
  `ALIAS ... remain explicit follow-on work rather than being hidden behind a graph-index helper.` Leaves open: the
  ALIAS layout, its owner, its byte maximum, and collision semantics.
- **ADR-068** (pin only; not carried, design D14 of the graph-index port). `:34-36` — `ALIAS_INDEX remains the base
  graph-index exception: its single-value layout / has no owner axis and production removal is incomplete.`;
  `:104-106` same; `:264-281` historical per-entity reverse index (`entity -> {predicates, names, aliases, ...}`),
  explicitly "not implementation authority" (`:36-40`) and "Alias ... require fresh design" (`:279-281`). Leaves open:
  the alias mechanism; it forbids TTL/MaxBytes as a cleanup substitute (non-evictable buckets, AGENTS.md § 2).
- **ADR-078** (pin and repo). Repo `:27-31` — hashing was dropped for PREDICATE because ADR-074 made predicates three
  bounded NATS-safe tokens; `:103-105` — `NAME and CONTEXT retain hashing for their open-content axes.` Leaves open:
  nothing on alias; its rule (hash only an open-content axis) bears on the choice, since an alias is open content
  (2.4).
- **ADR-065** (pin only, not carried): chose `hash(name).entityID.hex(predicate)` for NAME (`:580`); its alias lines
  (`:16`, `:520`, `:565`, `:584`) concern cleanup lists and readiness gating, not a layout.
- Next free repo ADR number: 114 (`ls docs/adr` ends at 113; pin ends at 107).

## 4. Open issues and pull requests

Queries: `gh issue list --state open --search <q> --limit 200` for `graph-index`, `ALIAS_INDEX`, `alias`, `graphindex`.

Kept (name the package or the alias seam):

- #415 — this issue. Retired by this PR (`Closes #415`).
- #428 — graph-index completes an entity whose index plan it refused, silently. Touches `buildEntityIndexPlan`
  (`index.go:301-380`), where the alias skip lives. Not this issue's work; stays unless the design chooses to.
- #108 — rebuild-on-restart measurement; a new alias layout adds list+diff per entity on replay (cost unmeasured).
  Stays.
- #405 — graph-query's resolver (PR 9) keeps exact + alias lookups; the alias reply shape this PR settles is its input.
  Stays (graph-query PR).
- #105 — graph-query split; names alias only through the resolver. Stays.
- #9 — epic; addressed, not closed.
Names the word or the package in passing, not this seam: #482, #109, #103, #485, #141, #106, #111, #110, #195, #16,
and #68, #196.

Open PRs (`gh pr list --state open --json number,title,isDraft,changedFiles,files`):

- #495 (draft, this claim) — 0 files.
- #494 (draft, `claude/kvwatch-adoption-461`, `Closes #461`) — 0 files (`gh api .../pulls/494/files` empty). Its
  issue's site list (#461) has no `processor/graph-index` file; it names `natsclient/kv.go:699` (`keysByFilter`),
  which `reconcileOwnedRows` and `deleteFromIndexes` call through `KVStore.KeysByFilter`. Overlap: indirect only, no
  shared file today. Merge order: none required; whichever merges second merges `origin/main`. Re-check its file list
  before #495 leaves draft.
- Required order by ruling: #495 before the unclaimed change 4 PR 9 (`processor/graph-query`).

## 5. Ledger

`docs/admission-ledger.yaml:3408` — `- source_path: processor/graph-index`, `:3409` source_sha `8b99efe9...`,
`:3468` `disposition: adapt`. Adaptation item (10) `:3435-3436` — `the alias index keeps the pin's behaviour, a /
dropped or changed alias and a deleted entity's alias keep resolving, until #415 (D12; owner ruling item 3)`;
`known_risks` `:3443` — `G8 (#415): the alias read is the one declared exception to [A] -> [B] -> [] replacement`;
item (5) `:3426-3427` (`the alias reply ... promises nothing about which aliases resolve`). ADR-077's own row
`:3528`. All edit with the fix.

## 6. Candidate layouts (candidates only)

Common to every candidate: deployments wipe and start fresh (no migration of raw `alias` keys); the bucket name and
`kvcatalog` row (`graph/kvcatalog/kvcatalog.go:174`) stay; the proving tests extend
`TestIndexReplacementThroughEveryIndex` (assert ALIAS_INDEX keys after A, B, none, delete) and
`TestQueryReadsImplementReplacement` (alias read answers A, then B, then none), with literal expected sets as the
oracle,
shown able to fail against today's code (it fails at `index_integration_test.go:316`'s expectation).

**A. NAME mirror: `hash(alias).entity6.hex(predicate)`** — owner filter `*.entity6.*` (`nameIndexEntityFilter`'s shape),
query `KeysByPrefix(hash(alias)+".")`, tail parsed by `entityAndPredicate`; value carries the original alias (and
priority if wanted). Replacement through `reconcileOwnedRows`; delete adds one set to `index.go:550-554`.
Observable: A->B->none and delete answer exactly current state; every entity carrying the alias is visible, with the
predicate that asserted it, so a collision is detectable and `AliasPriority` (`predicates.go:311-313`) is available to
order it. KV-unsafe aliases (`"two words"`) become indexable, so the declared skip and `aliases_skipped_total` would
lose their reason (surface change, design decides). Max key bytes: 64 + 1 + 256 + 1 + hex(predicate) — the same form
ADR-077 bounds NAME at 710 (pin `:59`) [arithmetic, not run]. Open: case folding (NAME folds; whether an identity
alias should is unmeasured); reply shape when more than one entity answers. Proving tests: the two above, plus two
entities sharing alias X, one drops X -> X answers the other; both hold X -> the chosen collision rule.
**B. `hash(alias).entity6`** — same as A without the predicate token (7 tokens, owner filter `*.entity6`, would need its
own tail parser). Observable: as A, but which predicate asserted the alias, and its priority, is not in the key
(value could carry it). Max bytes 64 + 1 + 256 = 321 [arithmetic]. Proving tests: as A.
**C. Keep `alias -> entity6` raw; verify on read** — the handler reads `ENTITY_STATES` and answers only if the entity
still carries the alias (precedent `query.go:328-345`); stale rows are not deleted. Observable: a dropped alias or a
deleted entity answers none; but with two entities sharing X, after the last writer drops X the other holder is lost
(X answers none though A carries it) — `[A] -> [B]` still fails in that case [code reading]; stale keys accumulate
(no TTL allowed, ADR-068). ADR-077's matrix row stays "unavailable by owner-key filter". Proving tests: the two above
plus the shared-alias case, which C fails by construction.
Excluded raw owner-keyed shapes (`raw-alias.entity6`, `entity6.raw-alias`): variable arity before or after the owner,
refused by ADR-077 §2 (no `>`, no variable arity); a one-token alias rule would refuse `drone.local` (`registry.go:72`).

One sentence: A is the layout that mirrors what graph-index already does for NAME, line for line.

Unmeasured: collision frequency in any consumer (none of semsource/semconnect writes resolvable aliases, 1.5);
replay cost of a fourth owner filter (#108); whether a restart replay delivers delete markers for entities deleted
while graph-index was down (`graph/follower/follower.go:265` handles markers it receives; delivery on replay is not
probed at nats.go v1.55.0, and applies equally to NAME, PREDICATE and INCOMING today).

## 7. Collision table (semantic class: resolve an external string to an entity)

| Dimension | Evidence |
|---|---|
| Owners | graph-index `ALIAS_INDEX` (`index.go:516-531`); NAME_INDEX for labels (`name_index.go`), ranked list, not resolution; retired ENTITY_SUFFIX_INDEX (#104 closed 2026-10-12); graph-query resolver at the pin (`entity_resolver.go:16`) |
| Catalogs | `graph/kvcatalog/kvcatalog.go:174`; vocabulary registry `DiscoverAliasPredicates` (`vocabulary/registry.go:446-460`) |
| Status | readiness gate via `queryRevision` (`query.go:244`); `aliases_skipped_total` (`metrics.go:24`) |
| Lifecycle | rebuilt on every start (#108); no TTL (ADR-068); delete path skips alias (`index.go:533-537`) |
| Ownership | none today (ADR-077 matrix row); last-writer-wins Put |
| Readers | graph-index alias verb only in tree; pin graph-query `query.go:159-176`, `entity_resolver.go:48`; graph-gateway (excluded) |
| Writers | `updateAliasIndex` only (`index.go:403`); pin `DeleteFromAliasIndex` dropped by the port (design D8/S9) |
| Recovery | repair loop re-indexes failed entities (`markEntityFailed`); no alias-specific repair |

## 8. Adopter seam

1. Must know today: an alias hit may be stale; a deleted entity's alias yields `entity_not_found` downstream; two
   entities with one alias give an arbitrary answer. Three facts, none surfaced (doc only: `README.md:183-185`).
2. Do nothing: silent wrong entity on a dropped alias (1.4).
3. Where they find out: nowhere at runtime; a doc and a spec requirement.
4. Should know: nothing, except the collision rule if more than one entity may hold an alias — the one fact the
   framework cannot observe away. `graph.AliasData` (`graph/query_index_types.go:30`) carries one ID; whether its
   shape changes is an exported-surface question for the design and the owner.

## 9. Intent check

No boundary moves (no port set, tier, exclusion or deferral change). Not applicable.

## Open evidence questions for the design

- Collision rule (refuse as ambiguous with a classified code, order by `AliasPriority` then entity ID, or return all):
  owner question; changes `graph.AliasData` if "all".
- Case folding of the hashed alias.
- Fate of the declared skip and `aliases_skipped_total` under a hashed key.

Skipped: pin-probe triple runs and shape sweep (not a porting change); no run of the pin's own tests; no measurement of
replay cost or delete-marker replay. Residual risk: the 1.4 consumer behaviour is from code reading, not a run.

## 10. Review fold-ins (INVENTORY PASS, three MEDIUMs)

10.1 The two pin readers fail differently (replaces the single reading in 1.3-1.4):

- `processor/graph-query/query.go:143-200` `entityByAlias`: a stale hit on a live entity returns that entity, silently;
  a hit on a deleted entity returns the classified `entity_not_found` (`:188-194`) and skips the fallthrough that
  treats the input as an ID (`:166-171`); any alias-verb error falls through to the ID path silently (`:172-177`).
- `processor/graph-query/entity_resolver.go:16-44` `resolvePartialEntityID` (callers `graphrag.go:422`, `:1016`): any
  alias-verb error returns `""` (`:62-65`) and the resolver moves on, with no log above Debug; a hit (stale or not)
  returns at once (`:27-31`, logged at Debug) and **suppresses the suffix fallback** (`:34-40`). A stale alias therefore
  pins GraphRAG to the wrong entity with no signal.
- Consequence for the owner's ruling 1 (ambiguity refused with an error): both pin readers swallow alias-verb errors
  (`query.go:166-177`, `entity_resolver.go:62-65`), so the refusal reaches a caller only if graph-query's PR 9 stops
  swallowing it; #405 already requires a classified refusal from the resolver. Adopter-seam item for PR 9, not #495.

10.2 Pin records of the defect (pin `8b99efe9`):

- `docs/adr/068-graph-retention-deletion-lifecycle.md:39-40` — `authority. Any future work must begin from the
  Derived-Index Convergence Program` / `inventory rather than treating the historical proposal as an approved plan.`
- `docs/proposals/post-g-foundation-remap-inventory.md:197` — the ALIAS_INDEX row: `Raw alias → single entityID; no
  owner axis`, `Blind last-writer Put`, `DeleteFromAliasIndex(alias) exists, but production search finds no caller;
  entity delete omits alias`, readers `alias query, graph-query/GraphRAG resolution`, `**DI-02, graph-index:**
  same-alias
  last-writer collision, stale alias after predicate/entity removal, and unreachable production delete helper`.
- `:216-219` — `ENTITY_SUFFIX_INDEX and ALIAS_INDEX: both reduce a potentially many-owner lookup to one last-writer
  value, but their public semantics differ.`
- `openspec/changes/archive/2026-08-14-post-g-tag-safety-closeout/disposition-ledger.md:26` — `| DI-02 | Alias
  collision/retraction | Defer explicitly; publish the limitation. | deferred-named-program | Derived-Index Convergence
  Program | PENDING — product Release notes |`. The pin deferred DI-02; it never fixed it.

10.3 Further excluded shapes, and a read budget:

- `alias -> {entity set}` under compare-and-set: one key per alias written by every entity that holds it, so two
  entity lanes contend on one key. Breaks the invariant the owner-keyed layouts rest on (`incoming_index.go:19-21`,
  `name_index.go:31-32`: "the key is written by its entity's lane alone", no CAS) and needs a CAS retry loop (#141's
  `UpdateWithRetryRead`): new mechanism.
- Owner-first `entity6.hash(alias)[.hex(predicate)]`: the owner filter becomes a plain prefix, but the forward read,
  the hot one, needs leading wildcards over six entity tokens (`*.*.*.*.*.*.<hash>[.*]`) on every alias query. ADR-077
  §2 approves fixed-position wildcards for owner discovery; the read-path cost is unmeasured.
- Read budget: a shared alias lists one key per holder. NAME bounds the same listing with `nameQueryMaxHydration`
  (`query.go:32` — `const nameQueryMaxHydration = 2000`; refusal `query.go:609-612`, `resource_exhausted`). Candidate
  A/B
  need a bound of the same shape; under ruling 1, two keys already decide "ambiguous", so the listing can stop early
  (design decides; not measured).

## 11. Owner rulings recorded on #415 (comment 6557704316, relayed)

(1) A shared alias is refused as ambiguous with an error naming the alias; `graph.AliasData` stays single-ID.
(2) Alias matching folds case as `normalizeName` does (`name_index.go:26-28`). (3) KV-unsafe aliases are indexed; the
skip (`index.go:319-325`), `aliases_skipped_total` (`metrics.go:24`) and the read-side `invalid_request` refusal
(`query.go:241-243`) are dropped. Layout direction: candidate A. Sections 6 and 8's open questions are answered by
these.

## 12. SemVocab

Question (owner, relayed): does SemVocab support change any of #415? Read: issue #350 with comments;
`gh issue list --search semvocab --state all` (#350 open, #415, #256 and #234 closed); `git grep -l -i semvocab --
docs/adr
openspec/specs` (ADR-107 `:12`, `openspec/specs/vocabulary-packs/spec.md:7`); semvocab at `main`
`b9ca45c9bbcdf1cb1838ba82596da6fddfff83f0` (tarball, local only under `/tmp/415-stage/semvocab/src`); semvocab issues
`--search "alias OR sameAs OR identifier OR identity"` (#1, #17: neither about alias identity).

12.1 How an alias predicate is declared here (measured):

- In code: `vocabulary.Register(name, vocabulary.WithAlias(type, priority))` (`vocabulary/registry.go:193` sets
  `AliasPriority`). In a pack: `PredicateSpec.IsAlias`, `AliasType`, `AliasPriority` (`vocabulary/pack.go:88-90`),
  copied
  into the registry by `RegisterPack` (`pack.go:333-337`); `alias metadata requires IsAlias=true` (`pack.go:308-309`).
- Resolvable types: identity, alternate, external, communication; label excluded (`registry.go:15-87`).
- graph-index already indexes **every** registered resolvable alias predicate, not only `core.identity.alias`:
  `index.go:87` `c.aliasPredicates = vocabulary.DiscoverAliasPredicates()` and `index.go:383-386`
  `isVocabAlias || predicate == "core.identity.alias"`. The coordinator's premise (a) is already true.
- `core.identity.alias` is a literal spelled only at `processor/graph-index/index.go:385`
  (`git grep -n 'core.identity.alias' -- '*.go' | grep -v _test`); no vocabulary registers it. A second spelling of
  "is an alias predicate", inherited from the pin (`component.go:1427`).
- Discovery runs once, in `startIndexPool` (`index.go:85-87`), so a pack registered after graph-index starts is not
  indexed. No rule or test orders pack registration before graph-index's Start: AGENTS.md § 1's registration rule covers
  payload and component registration (`RegisterPayloads`, `Register`), not vocabulary packs. Not tested here.

12.2 What changed versus the pin to prepare for SemVocab (measured): `vocabulary/pack.go` (PackSpec, LoadPackJSON
`:136`, LoadPackYAML `:159`, RegisterPack `:237`), `entity_type.go`, `projection.go` are new (`ls` of both trees;
history `7e2724a` #235 and `11da472` #261, 2026-10-15). The `AliasType` constants and `DiscoverAliasPredicates` body are
identical to the pin (`diff` of the `const (` block and the function: no difference but the line number, pin `:498` vs
tree `:452`). So the preparation made aliases declarable **from a pack**; it did not change alias semantics.

12.3 SemVocab itself (measured at `b9ca45c9`):

- Declares no alias predicate: `grep -rn -i -e is_alias -e alias_type -e alias_priority -e IsAlias -e AliasType
  -e AliasPriority -e WithAlias` over non-test, non-archive files returns nothing. Its SHACL compiler builds
  `vocabulary.PredicateSpec` from pack predicates (`shacl/compile.go:305-307`) and never sets alias fields.
- Packs: `packs/bfo-cco-core.yaml`, `packs/swe-core.yaml`, `packs/diekm/diekm.yaml` (predicates from `:50`, none with
  alias fields).
- Identifier-like terms exist only as catalog constants: SensorML `PropIdentifiers`
  (`catalog/sensorml/sensorml.go:187`),
  `PropUniqueID` (`:281`). Not registered as predicates, not marked alias.
- Uniqueness and case: SHACL `sh:maxCount` is per entity (cardinality), not across entities; no
  `owl:InverseFunctionalProperty`
  or `owl:hasKey` (`grep -rn -i -e InverseFunctional -e hasKey` over shacl, packs, catalog, docs: nothing); the only
  "unique" term is `sh:uniqueLang`, rejected by the profile (`docs/shacl-compilation.md:333`). No case rule for values.
- Constraint model ownership: #350 (open) proposes moving `min_count`/`max_count`/`in`/`pattern`/bounds into SemEngine's
  `PredicateSpec`, with SemVocab compiling into it. It names no alias, identity-key or cross-entity uniqueness
  constraint; its open question 4 (#350 comment 2026-10-20T21:38) is an identity-catalog **birth** gate, a different
  fact.

12.4 Verdict

- (a) Which predicates feed ALIAS_INDEX: **no change** [measured]. Already every registered resolvable alias
  predicate, pack or code; SemVocab registers none today. Candidate A's `hex(predicate)` token keeps which predicate
  asserted an alias, so more pack-declared alias predicates need no layout change [reading]. The hardcoded
  `core.identity.alias` literal is the one finding: either register it once in `vocabulary` or keep it (design decides;
  optional under Bounded port scope).
- (b) Case folding: **no change** [measured: no vocabulary or SemVocab construct declares value case rules]. A
  per-predicate case flag would be new surface with no consumer; ruling 2 stands.
- (c) Ambiguity rule: **no change** [measured]. `AliasPriority` exists ("conflict resolution order",
  `predicates.go:311-313`) but orders predicates, not entities, and nothing reads it; SemVocab sets none and has no
  cross-entity uniqueness construct. Ruling 1 (refuse) stands; candidate A keeps the predicate in the key, so a later
  owner ruling to rank by priority would need no re-key [reading].
- (d) Else: if #350 lands, a cross-entity "unique identifier" constraint would be a new constraint kind it does not list
  today; and a writer-side check cannot prevent a collision between two writers anyway, so the read-side refusal is
  still needed [reading]. No SemVocab or #350 work blocks or reorders #495.
