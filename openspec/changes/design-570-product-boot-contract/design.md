# Design: what a product's own main must know to boot SemEngine (#570)

## Context

The inventory (`inventory.md`, INVENTORY PASS round 2) re-checked the issue's seven items and found six more facts
(F1-F6). It counts about thirteen facts for a `main`; the adopter bar is two. Most already fail loudly (boot errors:
items 1, 2, 3 clash, F5). The silent ones are F2 (`deps.Platform` from the loaded file), F1 (vocabulary registered
late), item 7 (file edits ignored at an equal version), item 5 (the reconnect stall) and item 3's every-interface
binding. The named consumer, SemSource, pins `v0.1.0-alpha.7` and meets items 1, 2, 3, 5 and 7.

## Goals / Non-Goals

Goals: close each gap with the cheapest surface that makes it found: a doc where the failure is already loud or
unobservable, code only where the framework holds the value. Non-goals: as in the proposal.

## Decisions

D1. One guide, `docs/boot.md`, linked from `docs/contract.md:146` (the consumer's first-read section),
`service/README.md:82` and `config/README.md:170`. Steps in order, as `internal/smoke` runs them
(`stack_integration_test.go:242-320`), which is the tested reference: (1) load the file; (2) connect and wait for
the connection; (3) start `config.Manager`; (4) read `effective := mgr.GetConfig().Get()`;
(5) `EnsureStreams(ctx, effective)`; (6) register components, vocabulary and payloads; (7) `RegisterAll`;
(8) `ConfigureFromServices(effective.Services, deps)` with `deps.Platform` from `effective`; (9) optional calls;
(10) `StartAll`; (11) tear down with `StopAll`, then `Manager.Stop`, then `Close`. Component writes
(`PutComponentToKV`, `DeleteComponentFromKV`) between (3) and (8) land in this boot; after (8), only in the next
(F3). Each step says what happens if it is skipped and links the package README. Other homes considered: extending
`docs/contract.md` (it is about guarantees, and thirteen steps would bury them); package READMEs only (today's
state, which is the gap).

D2. Per item. Options are listed with cost, blast radius and what each forecloses; the chosen one is marked.

- Item 1, streams.
  - **(A) Doc, chosen.** The skip is already a boot error, about 49 s late.
  - (B) `StartAll` runs `EnsureStreams` itself. This changes ADR-058's Phase A split, so it needs the owner. It
    touches every composition and forecloses a product's own stream order.
  - (C) An exported boot helper. New API with two callers; ADR-058 allows helpers but needs none.
- Item 2, payloads.
  - **(A) Doc, chosen.** A nil registry is already refused by the components that decode
    (`graph-ingest/component.go:272`; objectstore with inputs, `storage/objectstore/component.go:189-191`). An
    unregistered product type is the product's own domain.
  - (B) The component manager refuses a nil registry. A composition without a decoding component needs none.
- Item 3, ports.
  - **(A) Doc, chosen:** the defaults, every interface, `metrics: {"enabled": false}`.
  - (B) Port 0 means kernel-picked, plus an exported address reader. That changes what an absent key means (a
    pointer or a new key), touches `service` and `metric`, and retires #614's hook. Owner question 2.
  - (C) A bind-host key. No asker.
- Item 4, restart. **Leave, chosen** (ADR-095); one line in the guide.
- Item 5, reconnect. **Leave, chosen.** The guide states the defaults (`natsclient/client.go:311-312`) and
  `WithConnectionLossTimeout` as an exit policy. It says the stall's length is nats.go's and not promised (ADR-112).
- Item 6, minted ID. ADR-104 decided it.
  - **(A) Doc plus a spec Requirement for the record shape, chosen.** No code: the test exists
    (`manager_identity_integration_test.go:160`).
  - (B) An exported reader. No named outside caller (inventory E4).
- Item 7, the bucket wins. Already documented (`config/README.md:222-242`).
  - **(A) Doc pointer, chosen;** the defects stay with #646 and #647.
  - (B) The engine owns a drift warning like SemSource's (`config_drift.go`, 257 lines); it belongs with #646, #647.
- F1, vocabulary. The framework cannot observe what a product never registered.
  - **(A) Doc, chosen.**
  - (B) Read the vocabulary per use: hot paths in graph-index and `entitytext`, across packages.
  - (C) Warn on a registration after the first read: global state, and the missing case stays unseen.
- F3, F4, F5, F6. **Doc, chosen.** All are loud or ordering facts.

D3. F2, the platform copy.

- (A) Doc only: the failure stays silent.
- **(B) Refuse, chosen.** `NewComponentManager` refuses a `deps.Platform` with either field set whose pair
  (`Org`, `Platform`) differs from `deps.Manager.GetConfig().Get().Platform`'s (`org`, `id`); the snapshot is
  already read at `service/component_manager.go:189-192`. The refusal is classified invalid (`errs.WrapInvalid`, as
  graph-ingest's at `component.go:281-283`), names both pairs, and comes before any component factory runs (before
  `cm.initialize()`, `:226`). `ConfigureFromServices` returns it and composes nothing. A half-empty pair is refused
  as a mismatch. A wholly empty `deps.Platform` is not refused here; the components that need one refuse it
  (`graph-ingest/component.go:280`), and 35 `Dependencies{` literals in `service` tests pass none. Blast radius:
  package `service`; three test fixtures (task 2.2); no exported change.
- (B') Fill a wholly empty `deps.Platform` from the effective pair and still refuse a mismatch. It does not break
  anything, and it removes a fact from the guide. Cost: the 35 fixtures receive a real platform, graph-ingest's
  empty-platform refusal becomes unreachable through the component manager, and the field has two meanings (given,
  or left for the framework). It is the half-step to (C) and is offered in owner question 1.
- (C) Remove `service.Dependencies.Platform` and read it from `deps.Manager`. This removes the spelling entirely. It
  is a breaking field removal, costs one line in SemSource, and changes the AGENTS.md authority row,
  `harness-boundaries` (`:700-705`) and ADR-104 `:178`. Owner question 1.
- The services and streams copies (`effective.Services`, `EnsureStreams(effective)`) get doc only. The component
  manager cannot tell which services map it was handed.
- Generated checks (`docs/testing.md:438`): not used. The rule reads two fields, each empty, equal or different:
  nine cases, which one table test covers exhaustively.

D4. Correct the examples: `service/doc.go:80-84`, `service/README.md:97-101,240,348`, and `docs/contract.md:146-191`
(the stale note at `:150-152`; the sketch points to `docs/boot.md`). ADR-104 `:183` gets a port-note correction:
the record's shape lives in the `config-manager` spec.

## Invariants (spec homes)

- Deployment authority is set once by `config.Manager.Start` and carried as `deps.Platform`
  (`harness-boundaries`, "No second spelling of deployment authority"). D3 makes the carried value equal the one that
  was set up (delta: `service-lifecycle`, "A platform the configuration manager did not set is refused").
- The identity record holds exactly `org`, `stem` and `id` (delta: `config-manager`, "The platform identity record
  is a read contract").

## Risks / Trade-offs

- A doc can drift. The guide names `internal/smoke`'s composition root as the tested order; the reviewer checks
  they match. D3 does not catch an unstarted `config.Manager`, which still reports the file's ID; the guide says
  Start comes first.

## Migration Plan

None for state. A product passing the un-minted ID gets a boot error naming both pairs; it fixes this by reading
`GetConfig().Get()` after `Start`.

## Merge order

No open PR overlaps (#696, #697 and #698 touch `go.mod`/`go.sum` only). #614, #646 and #647 have no PR. Whichever
lands second updates the one `docs/boot.md` line it changes.

## Open Questions (owner)

1. OPEN-Q: F2. Refuse a mismatched `deps.Platform` (D3 B, recommended), also fill an empty one (D3 B'), or remove
   the field (D3 C)?
2. OPEN-Q: item 3. Take the kernel-picked port and bound-address reader in this change, or leave them with #614
   (recommended: leave)?
