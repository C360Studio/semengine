# Design: simplify-04a-08-lifecycle

Status: draft, written before inventory review. Inventory: `inventory.md` (base `127d4c0`).

## Context

Issues #154, #193, #199 and #257 against `pkg/lifecycle`, which is already ported (ledger row `adapt`). Nothing is
carried from the pin. ADR-111 keeps the package as ADR-047, ADR-049 and ADR-091 made it; every decision below keeps
those (inventory § 6). Size: about 10% of 3,775 non-test lines, within the one-package bar (#159).

## Options

- **Write errors (#257):** (1) call `ClassifyMutationError` (recommended; it is what 9a exported it for);
  (2) copy projection's code list into lifecycle, a second spelling; (3) do nothing, which leaves #257 open.
- **Nil client (#154 item 1):** (1) keep nil and give it one refusal (recommended, owner question 1); (2) refuse nil
  in `NewManager` as #154 suggests, which changes the signature at 8 consumer call sites and removes the client-free
  registration check 3 consumer tests use; (3) do nothing.
- **Complete (#193 item 2):** (1) the current phase's declared edge order (recommended); (2) document "by name";
  (3) refuse when more than one terminal is reachable, which breaks `Complete` for agent-run's `executing`.
- **Children (#193 item 3):** (1) filter, then page (recommended, as `List` does); (2) document short pages, which
  leaves a caller no way to tell the end.

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
`ListWorkflows`, `AssertRuleWritable` and `FieldType`. Every call that needs NATS returns one error naming the
missing client and sends nothing: Get, GetWithRevision, GetRaw, Create, CreateFromOperator, Transition,
TransitionWith, UpdateFromOperator, Complete, Fail, Despawn, DespawnWith, List, Watch, WatchEvents, History,
Children, References and LookupByEntityID. The emitter's own refusal and its unreachable branches go. Proving test:
`TestManagerWithoutClientRefusesNATSWork` calls each of those on `NewManager(nil, nil)` with no injection and
checks the one error, with no panic. It replaces `TestManagerWithoutClientRefusesEmit`.

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
registration lookup treat a malformed key as not a match. `matchPattern` and `TestMatchPattern` go. Proving test: a
Despawn of a six-part ID with an uppercase segment returns `ErrEntityIDPatternMismatch` and sends no delete (fails
on base).

### D5 Fail and Complete say what they do (#193 items 1–2; owner question 2)

`Fail`'s doc states that it is a Transition to `failed` and needs a declared edge from the current phase; the
behaviour is unchanged (proof: `TestIntegration_FailMovesToFailedWithTheReason` already covers the refusal).
`Complete` (and `DespawnWith`, which shares the choice) picks the first terminal phase in the current phase's
declared out-edges. The Warn on ambiguity stays. Proving test: with `"executing": {"completed", "cancelled"}`,
`Complete` lands on `completed` (fails on base, which picks `cancelled`).
`TestIntegration_CompleteMovesToTheReachableTerminal` still passes, because its fixture's edge order and name order
agree.

### D6 Children filters, then pages (#193 item 3; owner question 3)

`Children` sorts the links as today. It drops a child that is absent, not lifecycle-managed, or of an unregistered
workflow, and then applies Offset and Limit, so a page shorter than Limit is the last page. With owner question 3
answered yes, any other read error fails the call, as `List` does (D-1). Proving test: links a..f with b and d
absent. Every (Offset, Limit) with both from 0 to 6 returns exactly `kept[Offset:Offset+Limit]` of the kept list
(an exhaustive small-scope check against the filter-then-slice oracle; fails on base). A child read failing with
`nats.ErrTimeout` fails the call (if accepted).

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

Generated checks: D6 uses exhaustive small-scope enumeration. Elsewhere the inputs are a finite set of codes, edges
and refusals, and the tables cover each one (`docs/testing.md`).

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

No open PR shares a file. #268 can land before or after this PR.

## Owner questions

1. **Keep allowing a manager with no NATS client? Recommended: yes.** The issue suggested refusing it. But semboids
   and semteams build one in three tests just to check that a workflow registers, and refusing it would change the
   constructor for every caller. Instead, such a manager still registers workflows, and anything that needs NATS
   fails with one clear error. The admission ledger's line about this changes to match.
2. **When an entity can reach several end phases, should Complete pick the first one listed for its current phase?
   Recommended: yes.** Today it picks alphabetically: for the agent-run workflow, "complete" on a running run would
   land on "cancelled", not "completed". Listing order lets the workflow's author decide. Fail keeps needing a
   declared path to "failed", and its doc will say so.
3. **Should Children fail when a child can't be read, instead of quietly leaving it out? (yes or no)
   Recommended: yes.** Today a child read that times out just disappears from the answer. List already fails in
   that case. The fix sits in the loop this PR rewrites anyway.
