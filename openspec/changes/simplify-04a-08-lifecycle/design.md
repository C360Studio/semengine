# Design: simplify-04a-08-lifecycle

Status: accepted (owner answers on #154, comment 6214484329, 2026-10-16).
Inventory: `inventory.md` (base `127d4c0`), `INVENTORY PASS` with fixes applied.

## Context

Issues #154, #193, #199 and #257 against `pkg/lifecycle`, which is already ported (ledger row `adapt`). Nothing is
carried from the pin. ADR-111 keeps the package as ADR-047, ADR-049 and ADR-091 made it. Each decision below keeps
those records' decisions; D6 keeps ADR-049's bound on Children's reads (inventory § 6). Size: about 10% of 3,775
non-test lines, within the one-package bar (#159).

## Options

- **Write errors (#257):** (1) call `ClassifyMutationError` (recommended; it is what 9a exported it for);
  (2) copy projection's code list into lifecycle, a second spelling; (3) do nothing, which leaves #257 open.
- **Nil client (#154 item 1):** (1) keep nil and give it one refusal (recommended, owner question 1); (2) refuse nil
  in `NewManager` as #154 suggests, which changes the signature at all seven consumer call sites and removes the
  client-free registration check three consumer tests use; (3) do nothing.
- **Complete (#193 item 2):** (1) the current phase's declared edge order (recommended); (2) document "by name";
  (3) refuse when more than one terminal is reachable, which breaks `Complete` for agent-run's `executing`.
- **Children (#193 item 3):** (1) read children in link order until the page is full (recommended: reads stay
  bounded by the page, as ADR-049 decides); (2) load every child, then page, as `List` does: correct, but every
  page reads every child, against ADR-049 `:544-547`; (3) document short pages, which leaves a caller no way to
  tell the end.

## Decisions

### D1 Write failures use the one mapping (#257)

Each emitter write returns `projection.ClassifyMutationError(op, err)` for its own operation (create, reconcile,
delete). A not-committed error with code entity-not-found or entity-exists is wrapped in `ErrEntityNotFound` or
`ErrAlreadyExists`. `errors.Is(err, errs.ErrRevisionMismatch)` still holds for revision mismatch (ADR-060).
`ErrEmitFailed` is no longer returned for a reply; its doc lists the two remaining cases (`manager.go:1054`,
`:1059`). Proving tests (package `lifecycle`, through `graphEmitterNATS` over `graphmutation.NewClient`):

- a transient `internal` reply is commit-unknown, class fatal, `errs.IsTransient` false, not `ErrEmitFailed`, for
  each write (fails on base);
- an already-cancelled context sends nothing (requester calls 0) and is not committed, `MutationUnavailable`, class
  transient, `errors.Is(err, context.Canceled)` (fails on base);
- `TestEmitMappingsKeepTheClassifiedCause`, `TestNoRespondersIsTransientAndNotCommitted` and
  `TestMutationAmbiguousReplyReturnsCommitUnknownAfterOneAttempt` pass unchanged.

### D2 No client, one refusal (#154 item 1; owner question 1)

`NewManager(nil, …)` returns a manager that registers workflows and answers `GetWorkflowDefinition`,
`ListWorkflows`, `AssertRuleWritable` and `FieldType`.

**The error.** One unexported error, "lifecycle: manager has no NATS client". It is unclassified, so
`errs.Classify` reads it as fatal: a wiring mistake, never retried. It does not match `ErrEmitFailed`, and nothing
does on this path today either: a nil-client write already fails at its read with "exact entity reader
unavailable" (`manager.go:207`), so D17's "every emit returns `ErrEmitFailed`"
(`archive/2026-10-12-setup-04a-02-ingest-kernel/design.md:1147`) is reachable only by injecting a reader in a test.
Ledger contract item (4) is rewritten to this rule.

**Where it sits.** It is returned at the two places a NATS dependency is used and found missing: the exact read
when the manager has no reader, and `ensureBucket` when it has no bucket and no client to open one. A check on
`natsClient` alone would refuse the managers `newManagerForTest` builds with an injected reader and bucket (six
test files), so the check is on the missing dependency. No production hook is added. With no client the manager
holds no emitter. Every write reads first (inventory § 1), so the read refuses before any emit; the emitter's
`unavailable` field, its three checks and its unreachable branches go.

**Check order.** Each call keeps its current order. Refusals that need no NATS still come first: a nil context or
callback (Watch, WatchEvents), Fail's empty reason, an unregistered workflow, an ID that does not match its
workflow, and a malformed initial state. The no-client error comes only from the call's first read.

Proving test: `TestManagerWithoutClientRefusesNATSWork` calls Get, GetWithRevision, GetRaw, Create,
CreateFromOperator, Transition, TransitionWith, UpdateFromOperator, Complete, Fail, Despawn, DespawnWith, List,
Watch, WatchEvents, History, Children, References and LookupByEntityID on `NewManager(nil, nil)` with a registered
workflow and no injection. Each returns the one error with no panic, and the registration and definition calls
succeed. It replaces `TestManagerWithoutClientRefusesEmit`.

### D3 One watch loop, one report (#154 item 2)

`WatchEvents` delivers Upserted and Deleted events from one loop; `Watch` delivers the Upserted ones as
Participants. What each returns is unchanged (ledger contract (1)). The two Warn logs go: the returned error already
names the workflow, entity and revision, and wraps the `*graph.StateContractError` with its reason. Proof: the
existing watch tests pass. `TestLifecycleWatchPoisonWarningNamesSubscriptionAndAuthorityEntryOnce` becomes an
assertion on the returned error (`errors.As` reason, entity, revision 41) plus "no WARN logged". The closed-watcher
case asserts no WARN too.

### D4 One matcher (#154 item 3)

Every "is this ID this workflow's" check uses `semtypes.MatchEntityIDPattern`. Create, Despawn and DespawnWith
report a malformed ID as `ErrEntityIDPatternMismatch` with the reason, as Create already does. List, Watch and the
registration lookup treat a malformed key as not a match. A malformed key cannot be a workflow's entity: only
graph-ingest writes `ENTITY_STATES`, and it validates every ID before writing (inventory § 1). `matchPattern` and
`TestMatchPattern` go. Proving test: a Despawn of a six-part ID with a segment starting with `-` returns
`ErrEntityIDPatternMismatch` and sends no delete. Base already sends no delete for it (its read refuses the ID), so
the test fails on base only on the error type.

### D5 Fail and Complete say what they do (#193 items 1–2; owner question 2)

`Fail`'s doc states that it is a Transition to `failed` and needs a declared edge from the current phase; the
behaviour is unchanged (proof: `TestIntegration_FailMovesToFailedWithTheReason` already covers the refusal).
`Complete` (and `DespawnWith`, which shares the choice) picks the first terminal phase in the current phase's
declared out-edges. The Warn on ambiguity stays. Proving test: with `"executing": {"completed", "cancelled"}`,
`Complete` lands on `completed` (fails on base, which picks `cancelled`).
`TestIntegration_CompleteMovesToTheReachableTerminal` still passes, because its fixture's edge order and name order
agree. Its subtest "two terminals reachable: the first by name, with a Warn"
(`query_transition_integration_test.go:347`) is renamed to say "the first declared".

### D6 Children reads until the page is full (#193 item 3; owner question 3)

`Children` sorts the links as today, reads children in that order, and drops only a child that is absent
(`ErrEntityNotFound`) or not lifecycle-managed (`ErrEntityNotLifecycleManaged`), as `List` does. Offset and Limit
count kept children, with List's slice rule:

- Offset ≤ 0 starts at the first kept child;
- Limit ≤ 0 means no limit;
- a page shorter than Limit is the last page;
- an empty result is a non-nil empty slice. List's shape; base returns nil, which the pin gateway already turns
  into an empty list.

It stops reading once Offset+Limit kept children are collected. A page therefore costs at most
Offset + Limit + (dropped) reads, one Get per child, as ADR-049 `:544-547` decides. That is Offset more reads than
today's short page; Limit ≤ 0 reads every link, as today. A negative Offset no longer panics (inventory D-3).

Any other read error fails the call, as `List` does, including `ErrWorkflowNotRegistered` for a child workflow
nobody registered (D-1; owner question 3: yes).

Proving test: links a..f with b and d absent (kept = a, c, e, f). For every Offset in -1..7 and Limit in -1..7 the
result equals the oracle: start = max(Offset, 0) capped at len(kept); end = len(kept) when Limit ≤ 0, else
min(start+Limit, len(kept)); want kept[start:end] as a non-nil slice. The reads made are counted against the bound
above. This is an exhaustive small-scope check; it fails on base. A child read failing with
`nats.ErrTimeout` fails the call, and so does a link to an unregistered child workflow.

### D7 Doubles refuse what graph-ingest refuses (#199)

The unit exact reader and the integration exact-read and reconcile responders refuse a stored value whose statement
lacks Source or Timestamp, or whose entity ID is not its key, with `graph_state_reset_required` and graph-ingest's
reason. This is a test-package copy of about 15 lines. Its comment says it can fall behind and names
`TestBootSweepInventoriesStatementWithoutMetadata`, the graph-ingest test that holds the rule. Each test that then
fails stamps its fixture or asserts the refusal. Proof: one test per double stores an unstamped value and asserts
the refusal. Exporting the rule from `graph` is D-2's work (PR 8).

### D8 Docs and nits (#154 items 4–5)

`CreateFromOperator`'s and `ErrInvalidInitialState`'s docs say what is decoded and refused, with no history.
`_ = probe` goes. The ledger contract item (4) and the new emitter, Complete and Children rules are recorded on the
`pkg/lifecycle` row. Gate: review, plus `task ledger:check`.

Generated checks: D6 uses exhaustive small-scope enumeration against an independent oracle. Elsewhere the inputs
are a finite set of codes, edges and refusals, and the tables cover each one (`docs/testing.md`).

## Issues

| Issue | Disposition |
| --- | --- |
| #154, #193, #199, #257 | Retired (`Closes`): tasks 3.1–3.8 |
| #268 | Stays: `internal/graphmutation`; until it lands, natsclient's pre-send refusals stay commit-unknown here too |
| #132 | Stays: PR 8. If `ExactEntityReader` becomes a struct, lifecycle declares its own one-method interface then |
| #20, #24, #33, #129, #195 | Stay (inventory § 3) |

## Not in this change

D-2 (Watch's stored-value check; `graph`, PR 8). #268. #258. #270.

## Merge order

PR #273 (draft, PR 7, `component`) has no files yet and will likely edit the `component` row of
`docs/admission-ledger.yaml`; this PR edits the `pkg/lifecycle` row. Whichever lands second merges `origin/main`
first. No other open PR shares a file. #268 can land before or after this PR.

## Owner questions

Answered 2026-10-16 (#154 comment 6214484329): all three yes, as recommended.

| Ruling (2026-10-16) | Implementation | Proving test |
| --- | --- | --- |
| 1. Keep the no-client manager; one refusal | `pkg/lifecycle/errors.go:156` (`errNoNATSClient`), returned at `manager.go:203` and `:218` (D2, `56f1737`) | `TestManagerWithoutClientRefusesNATSWork` (`graph_emit_test.go:14`) |
| 2. Complete picks the first declared terminal | `pkg/lifecycle/manager.go:808` (`selectReachableTerminal` walks the current phase's out-edges; Complete calls it at `:776`) (D5) | `TestManager_CompleteFollowsDeclaredEdgeOrder` (`manager_test.go:1282`) |
| 3. Children fails on an unreadable child | pending: task 3.6 (D6) | pending: task 2.5 |

1. **Keep allowing a manager with no NATS client? Recommended: yes.** The issue suggested refusing it. But semboids
   and semteams build one in three tests just to check that a workflow registers, and refusing it would change the
   constructor at all seven places their code calls it. Instead, such a manager still registers workflows, and
   anything that needs NATS fails with one clear error saying there is no NATS client.

   This changes a promise made in the earlier ingest-kernel design (its D17) and recorded in the admission ledger:
   that such a manager's writes fail with `ErrEmitFailed`. In practice they never get that far today. Each write
   reads first, and the read fails first with a different message. So nothing that could see `ErrEmitFailed` there
   is lost, and the ledger line changes to the new rule.
2. **When an entity can reach several end phases, should Complete pick the first one listed for its current phase?
   Recommended: yes.** Today it picks alphabetically.
   - For the agent-run workflow, completing a run that is executing lands on "cancelled" today, and on "completed"
     with listing order.
   - A run still waiting to start ("dispatched", which can go to executing, failed or cancelled) would complete to
     "failed" with listing order, and to "cancelled" today. Neither is ideal; listing order at least lets the
     workflow's author choose, by putting the preferred end phase first.

   Fail keeps needing a declared path to "failed", and its doc will say so.
3. **Should Children fail when a child can't be read, instead of quietly leaving it out? (yes or no)
   Recommended: yes.** Today a child whose read times out, or whose workflow was never registered, just disappears
   from the answer. List already fails in those cases, and still leaves out a child that is gone or not under
   lifecycle management. The fix sits in the loop this PR rewrites anyway.
