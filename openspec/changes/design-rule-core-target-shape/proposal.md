# Proposal: design-rule-core-target-shape

Approved-by:

Track: `design`. No spec Requirement changes; the PRs that build toward this page carry their own spec deltas.

## Why

The owner ruled on 2026-11-25 (PR #752, comment 7162146299) that the rule-core port needs one page before PR 2. The
page names the types the engine hangs on: the rule definition, an action and its handler, the one fire function, and
the stored state record. PRs 2 to 12 build toward it; PR 1 does not wait for it.

## What changes

- `design.md` is that page. For each type it gives a Go sketch, the PR that builds it, and the pin code it replaces.
- `inventory.md` holds the measurements it rests on.

## Decisions you are making by approving

- **An action is its type, `when`, `id` and `max_iterations`, with the rest kept as written.** Every action type,
  built-in ones included, is one handler in one map. Each handler checks its action at load (needing no connection)
  and runs it at fire. A rule with an unknown type is refused when it loads, naming type, rule and file.
- **An action's firing-cap counter is keyed by its `id`, or else by a hash of rule, type and body.** The body is
  hashed in canonical form (keys sorted, no spacing), so storing and reloading a rule does not reset its counts. Any
  real edit to an action restarts its count; today only edits to its subject, predicate, object, bucket, key, reason
  or role do. The other way, each handler naming the fields that count, is one more method every product handler
  must get right. An author who wants a count to survive edits sets `id`, as 12 of semteams' 124 actions do.
- **Built-in actions refuse body keys their type does not read, naming key, rule and file.** Today a misspelt key
  (`predicat`) is ignored without a word. Every key in the products' checked-in built-in actions is one its type reads,
  so none is refused. Product handlers choose for their own types.
- **Cron actions keep having no firing cap, but now honour `when`.** An explicit `max_iterations` on a cron action is
  refused at load. Today cron ignores both keys without a word. No product cron action sets either.
- **A triple action with no triple writer is refused at load.** That happens when graph integration is off. Today it
  is logged and dropped. semboids runs with graph integration off and uses no triple action, so it is unaffected.
  semteams' contract tests that build executors with no writer need a writer or must use no triple action.
- **The framework keeps enforcing the reserved `user.response.>` space (ADR-093) for every action type.** The
  publisher the framework hands each handler refuses that space, checked on the subject after substitution. The
  built-in `publish` also refuses a declared reserved subject at load. At the pin, `publish_agent` already publishes
  through the executor's publisher (`actions.go:2025-2033`), so semteams' handler gets the guard by using it. A
  product handler that publishes over its own connection bypasses it; PR 2 documents that it must not.
- **A product adds its action types at registration.** `Register(registry)` carries the built-ins only.
  `RegisterWith(registry, Extensions{Handlers: …})` adds a product's types and is called from the product's `main`.
  A product type that reuses a built-in name is refused. PR 2's Fable read settles the names.

## Impact

Docs only. It changes the wording accepted on #25; the two proving tests stay (`design.md` § 2). It affects
semteams' registration, its hot-reload seam (PR 12), and its contract tests that build executors.
