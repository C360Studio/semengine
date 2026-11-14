# Design: fix-578-prefix-damaged-entities

Premises cite `inventory.md` (base `ecbd4b92`).

## Options

- **A (owner's choice). The prefix reply lists damaged entities; readers in this repository that would hide a partial
  page refuse it.** Cost: five packages in one pull request. A direct outside reader that ignores `missing` sees fewer
  entities (migration note).
- **B. The request opts in** (for example `partial: true`, sent only by fusionnats); by default the page is still
  refused. Cost: a new request field and two behaviours for one verb. No other reader changes. The owner chose
  against this shape ("return the rest"); recorded for completeness.
- **C. Do nothing.** Fusion keeps failing a prefix answer for one damaged record; ADR-109 D1 stays untrue for prefix.

## D1. What the prefix reply says

``PrefixQueryResponse.Missing []MissingEntity `json:"missing,omitempty"` ``. Each entry is a key on this page whose
stored value failed the decode rule, reason `MissingError`. `MissingError`'s doc comment changes: it is now also
sent for a stored value the prefix read refused. The batch verb keeps refusing such a value (inventory § 1).

- *Total*: unchanged, the key listing (`query.go:303`).
- *Fit and cursor*: the page is the sorted run of selected keys. Each key is either an entity or a missing entry.
  `marshalFittingPrefixPage` keeps the longest leading run whose full encoding (entities, `missing`, `total`, cursor)
  fits. The cursor is the last key kept when the run is cut short, and the existing following boundary otherwise. A
  damaged key that ends the page therefore moves the cursor on. One that falls past a byte-trimmed boundary is not
  listed and is read on the next page. A first key too large to fit is still `response_too_large`.
- *Not found*: unlisted, as today. The comment at `query.go:325-329` stays true for not-found keys and gains one
  sentence on decode failures.
- *Fail closed*: any other read error, or an ended context, refuses the page transient `internal`, even when damaged
  keys are also present (today the damaged-entity error wins, `query.go:603-607`).
- *Side effects kept*: the record at the revision read, health, and no cache entry (`mutation_runtime.go:159`;
  graph-entity-writes `:291`).

## D2. Readers

- fusionnats' `resolvePrefix` (`client.go:123-143`): seeds are entities plus `missing` IDs, merged in ID order.
  `Entities` then takes its existing reset-refusal fallback and lists the damaged seed as unhydrated `error`
  (`client.go:268-270`, `:319-321`). An empty or invalid `missing` ID is refused like an invalid entity ID.
- graph-query's `readCountPage` (`counts.go:32-75`): when `missing` is non-empty it refuses fatal
  `graph_state_reset_required`, naming the IDs. That is what its callers return today when the responder refuses
  (`openspec/specs/graph-query/spec.md:186-187`). Counting damaged IDs as rows is a possible follow-up (owner Q1).
- graph-gateway's `entitiesByPrefix` (`resolve.go:128-158`): a page with `missing` makes the field `null` with an error
  naming the path and the IDs, as an invalid entity does today (spec `:71-74`). Adding `missing` to `EntityPage` is a
  possible follow-up (owner Q1).

## Proving tests (failing first)

- graph-ingest unit, `poison_scoping_test.go`: A good, B damaged, C good → entities A and C, `missing` [B `error`],
  `total` 3, B recorded, not cached, not deleted. This replaces the prefix halves of
  `TestAggregateReadNamesEveryPoisonedEntity` and `TestEveryEntityReadRefusesTheSameStoredPoison`; the batch halves
  stay as they are.
- Damaged key last on a limited page → cursor is that key and the next page starts after it. Byte fit cut before the
  damaged key → it is not listed and appears on the next page. Every key damaged → `entities` `[]`, `missing` lists
  all, cursor set when more keys remain. The existing fit tests (`query_prefix_unit_test.go:196-274`) pass unchanged.
- A KV read error together with a damaged key → transient `internal`, no body.
- fusionnats: a prefix reply with `missing` [B] → `Fuse` answers with A and C, and B unhydrated `error`.
- graph-query: a count page with `missing` → fatal `graph_state_reset_required`. graph-gateway: `entitiesByPrefix`
  `null` with the path.
- Generated checks: the page/cursor walk has a stated law. Following cursors to the end visits every listed key
  exactly once, as an entity or a missing entry. A generated test checks it against a sorted-key reference model,
  with damaged keys and payload limits drawn at random.

## Owner questions

- Q1. Keep refusing in graph-query and the gateway (drafted), or show damaged entities there (gateway `missing`
  field, counts by ID)?
- Q2. Reuse reason `error` (drafted), or add a new reason (fusion maps an unknown reason to `unknown`,
  `client.go:344`)?
- Q3. Pin ADR-079 D3 ("aggregate reads failing loudly") is changed for prefix and kept for batch. Is batch to follow
  later?

## Merge order

PR #655 merges before #654; #654 merges origin/main afterwards. No overlap with `processor/graph-lexical`.
