# Proposal: design-rule-core-target-shape

Approved-by:

Track: `design`. No spec Requirement changes; the PRs that build toward this page carry their own spec deltas.

## Why

The owner ruled on 2026-11-25 (PR #752, comment 7162146299) that the rule-core port needs one page before PR 2. The
page names the types the engine hangs on: the rule definition, an action and its handler, the one fire function, and
the stored state record. PRs 2 to 12 build toward it; PR 1 does not wait for it.

## What changes

- `design.md` is that page. For each type it gives a Go sketch, the PR that builds it, and the pin code it replaces.
- `inventory.md` holds the pin and product measurements the page rests on.

## Decisions you are making by approving

- **An action is its type, `when`, `id` and `max_iterations`; the rest is kept as the bytes the author wrote.** Every
  action type, built-in ones included, is one handler in one map. A rule with an unknown type is refused when it
  loads, naming type, rule and file. This is item 1 of the ruling made concrete.
- **An action's firing-cap counter is keyed by its `id`, or else by a hash of rule, type and body.** Any edit to an
  action restarts its count. Today only edits to its subject, predicate, object, bucket, key, reason or role do. 12 of
  semteams' 124 actions set `id`.
- **Cron actions honour `when`, and `max_iterations` on a cron action is refused at load.** Today cron ignores both
  without a word. Without the refusal, the default cap of 3 would stop a cron action after its third tick. No product
  cron action sets either key.
- **A built-in action whose dependency is missing is refused at load.** Today it is logged and dropped. Example: a
  triple action with graph integration off. semboids runs that way and uses no triple action, so it is unaffected.
- **A product adds its action types at registration.** `Register(registry)` stays and carries the built-ins only.
  `RegisterWith(registry, Extensions{Handlers: …})` adds a product's types and is called from the product's `main`. A
  product type that reuses a built-in name is refused. PR 2's Fable read settles the final names.

## Impact

Docs only. Changes the wording accepted on #25; its two proving tests stay (`design.md` § 2). Affects semteams'
registration and its contract tests that build executors.
