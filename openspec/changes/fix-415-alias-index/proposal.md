# Proposal: fix-415-alias-index

## Why

graph-index's alias index keeps answering old aliases. When an entity drops or changes an alias, or is deleted, its
old aliases keep resolving to it, and two entities with one alias answer whichever wrote last
(`processor/graph-index/index.go:400-406`, `:516-531`, `:533-537`; asserted by `index_integration_test.go:316`). The
pin carried the same defect (SemStreams `8b99efe9`, DI-02, deferred). The owner waived the integrity gate for this one
read when graph-index was ported (PR #414 comment 6426254115, item 3), on two conditions: the alias key layout is
decided as a decision record before any code, and the fix lands as its own pull request before change 4 PR 9
(`processor/graph-query`), whose `entityByAlias` takes an alias hit as its answer. Graph "is in many ways our secret
sauce. it needs to be solid."

Approved-by:

## What Changes

- ADR-114 decides the layout: `hash(alias).entity6.hex(predicate)`, NAME's shape, owned by the entity and reconciled
  from current state with the other owner-keyed indexes; entity delete removes the rows (#415 comment 6557704316).
- The alias read folds case **and trims surrounding space**, exactly NAME's `normalizeName` (`name_index.go:26-28`),
  so ` sn-1 ` matches `SN-1`; approving this proposal approves the trimming as part of ruling 2. One holder answers
  its ID; two or more distinct holders are refused, class invalid, code `alias_ambiguous`, naming the alias.
  `graph.AliasData` keeps one ID.
- Any alias string is indexed; the KV-unsafe skip, `aliases_skipped_total` and the read-side refusal of such aliases go.
- `vocabulary` declares `core.identity.alias` (identity type); graph-index's literal second spelling goes (#415
  comment 6557766571).
- Every text naming the #415 exception is brought to the new behaviour: specs, README, doc comments, the developer
  and reviewer contracts, the `query-pattern` skill, ADR-077's port note, the ledger row.

## Capabilities

- Modified `graph-index` (two requirements modified, one added).
- Modified `graph-transport-boundary` (one requirement modified, one removed).

## Impact

- `processor/graph-index` (`index.go`, `query.go`, `name_index.go` or a new `alias_index.go`, `metrics.go`, tests,
  `README.md`, `doc.go`); `vocabulary` (one registration and constant); `graph` (one error-code constant,
  `query_response.go` comment); `docs/adr/114-…`, `docs/adr/077-…` port note; `docs/admission-ledger.yaml`
  (`processor/graph-index` row; repo-authored ADRs have no ledger row);
  `.agents/contracts/semengine-developer.md`, `semengine-reviewer.md`, `.agents/skills/query-pattern/SKILL.md`.
- New exported surface: `graph.ErrorCodeAliasAmbiguous`, `vocabulary.CoreIdentityAlias`. Removed: the
  `aliases_skipped_total` collector.
- No new subject, bucket, goroutine or import. No migration: deployments wipe.
- Issues: closes #415. #405 (graph-query must propagate alias refusals) stays with PR 9. Merges before PR 9; no file
  overlap with draft #494.
