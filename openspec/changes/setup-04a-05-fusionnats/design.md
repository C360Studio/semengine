# Design: `pkg/fusion/fusionnats` and the no-embedder proof

Base `468bac4f` (claim on main `7901bfda`); pin `8b99efe9`. Inventory: `inventory.md` (§ n below). Owner rulings:
split row D and ruling 4 (PR #550 comment 6716972354), tag after PR D (#9 comment 6727586862), consumer fixtures are
consumer-owned (#568). **Goal.** A product composes `fusion.Engine` over NATS with one call, gets one answer per query
in which a poisoned seed is listed, not fatal, and can run the tier-0 stack with no graph-embedding and see which
answers it has. **Non-goals:** `searchGraph`'s fallback (#563: fusionnats never calls it); graph-ingest's batch or
prefix wire; a "not served" code in graph-query (owner question 2).

## D1. Construction: one call, nothing running (N4, N5, N9)

`New(requester, timeout) (*Client, error)`, the `graph.NewExactEntityReader` rule (`graph/exact_entity.go:46-59`): a
nil requester and a negative timeout are refused; zero is natsclient's `DefaultRequestTimeout`. `New` reads the six
graph-query rows it uses (`entity`, `batch`, `prefix`, `byName`, `relationships`, `semantic`) from `graph.QueryVerbs()`
and refuses, naming the verb, if one is missing. The client holds no goroutine, no watch, no context, no lock: `Status`,
`Close`, `statusWatcher`, `awaitFirstStatus`, `unknownCause` and their tests are not carried (D4). *Proof:*
`TestNewRefusesNilRequester`, `TestNewRefusesNegativeTimeout`, `TestSubjectsComeFromVerbTable` (each call's subject
equals its row's), unit.

## D2. The tree's wire (N1-N3, N6-N8, N10)

Each reply is decoded by its row: `graph.QueryResponse[T]` when `Enveloped` (`byName`, `relationships`, `semantic`),
the bare type otherwise; a reply of the other shape is an error, never sniffed (N2). Requests use `graph.EntityRequest`,
`EntityBatchRequest`, `PrefixQueryRequest`, `SemanticRequest` (its `scope` omitted when empty, ADR-071); byName and
relationships keep local structs (no `graph` type). `Neighbors` reads `predicate` (N3). An NL `Resolve` copies
`unreadable_records` and `failed_records` into `fusion.Resolution` (N6); symbol and prefix leave both 0. `Entity`
decodes through `graph.UnmarshalEntityState` (N10); `entity_not_found` is `(nil, nil)` as at the pin. Kept as at the
pin: the decoded-ID and decoded-state validators, `reconcileHydration` (ADR-084 D6), `Names` dedup and cap, the
client-side prefix and scope checks (N13), zero IDs sending no request (§ 9). *Proof:* the pin's resolve, entity,
batch, neighbors, names and reconcile tests on the tree's wire; `TestNeighborsReadsPredicate`,
`TestSemanticCountsReachResolution`, `TestReplyShapeDecidedByVerb` (replaces pin `slice_e_contract_test.go:55-105`),
unit. Reconciliation is a law over interacting inputs (duplicates, an ID in both lists or in neither), so
`TestReconcileTotal` is generated (rapid, seed recorded): every distinct requested ID appears once across `Entities`
and `Unhydrated`, `Entities` in request order, against a reference model; the pin's table cases stay as witnesses.

## D3. A poisoned seed is unhydrated, not a failed answer (N11; ADR-109 D1)

graph-ingest refuses the whole batch when one entity is poisoned, naming the IDs only in text (§ 2; pin ADR-079 D3).
**O1 (recommended).** When the batch request itself is refused with code `graph_state_reset_required` (read by
`graph.IsStateContractError`, never by text), `Entities` reads each distinct requested ID once with `entity`: found is
hydrated, `entity_not_found` is `not_found`, a `graph_state_reset_required` refusal is unhydrated `error`, and any
other error, an ended context included, is returned. A refusal from the client's own validation of a reply is returned,
not retried. On the batch verb the code means poison only (§ 2). Cost: one extra request per seed (at most 40,
`resolveLimit`), only while poison is present. **O2.** graph-ingest's batch lists poisoned IDs in `missing` with reason
`error`, one request; changes pin ADR-079 D3 and graph-ingest's change-2 D5, a second package (owner question 1).
**O0.** Do nothing: one poisoned seed fails every answer it ranks in, against ADR-109 D1.

| Seeds | Under O1 |
| --- | --- |
| none | no request (§ 9) |
| one of three poisoned | two nodes; `unhydrated` names the third, reason `error` |
| all poisoned | `deferred`, `all_seeds_unhydrated`, each listed `error` (`fusion` spec, "An answer that read nothing") |
| poisoned twice in the request | read once, listed once |
| poisoned and a transient fault | the fault is returned; `Fuse` fails the call |

`UnhydratedError`'s doc (`pkg/fusion/retrieval.go:109-110`) says it now carries a refused entity. *Proof:* unit
`TestPoisonedSeedUnhydrated`, `TestAllSeedsPoisonedDefer`, `TestPoisonedDuplicateReadOnce`,
`TestPoisonFallbackReturnsOtherError`, `TestPoisonFallbackEndedContext`, `TestOwnValidationNotRetried`; integration
`TestPoisonedSeedOverRealResponders` (graph-ingest and graph-query on a `natsfixture` server, one key Put with bytes the
stored-value rule refuses; adopts `natsfixture.Open`, #195), each shown to fail by a mutant.

## D4. Readiness: none (ruling 4; ADR-109 D3; ADR-083, ADR-066 item 3 changed)

fusionnats watches no key and reads no status: readiness arrives as `index_not_ready` on each reply, which
`Engine.Fuse` already reads (`fusion` spec). This removes the pin's detached watch (N5) and the one bounded wait that a
deployment without graph-index spent on its first call (`client.go:149-169`). `graph/readiness.Reading.Raw` had one
pin reader, fusionnats' `Status` (**measured** `grep -rn 'reading\.Raw'` at the pin: `client.go:227` only), so it is
removed with the watcher doc's sentence (`graph/readiness/watcher.go:22-25`) and the ledger's open question
(`:2195-2198`). *Proof:* `go list -deps ./pkg/fusion/fusionnats` names no `graph/readiness` (in D5's contract test).

## D5. The no-embedder proof (`docs/setup-plan.md:439-444`)

**P1 (recommended).** `internal/smoke` gains `TestSmoke_NoEmbedderAnswers`: the child boots graph-ingest, graph-index
and graph-query from one configuration with no graph-embedding entry and no graph-embedding registration (an env
value selects the second configuration), publishes the fixed entities, waits on graph-ingest's and graph-index's keys,
and checks: all three components healthy and `GRAPH_STATUS` has no `graph-embedding` key; `fusion.Engine` over
`fusionnats` answers a symbol lens and a prefix lens with the expected nodes, relations and paths; an unknown name is a
miss; an NL lens's `Fuse` returns graph-query's refusal, class transient, code `internal`, naming
`graph.embedding.query.search`, never an empty answer. `TestFusionNATSImportsNoEmbedding` (`internal/harness/contract`)
fails if `fusionnats`' closure reaches `graph/embedding`, `processor/graph-embedding` or `graph/readiness`, with a
sensitivity case as `embeddingprovider_test.go` has. The SemSource composition itself stays the SemSource owner's
(plan `:440`). **P2.** fusionnats' own integration test only: no configuration boot, proves no composition. **P3.**
Leave it to SemSource: the plan asks SemEngine for the proof. How fast a request with no responder is refused depends
on nats.go and the server and is not promised; the test asserts the refusal only.

## D6. ADRs, issues, order, size

Ported ADRs touched: ADR-083 and ADR-066 port notes say fusionnats watches nothing (ruling 4); ADR-071's says the scope
is sent; ADR-084's that fusionnats is ported. Issues: none retired. #195 stays (other packages; this PR adopts it for
fusionnats); #110 stays (item 3); #563 stays (graph-query). Filed by this PR, `status:blocked` on it: graph-ingest's
prefix read refuses a whole page for one poisoned entity, which fusionnats cannot scope (§ 2). Ledger: new row
`pkg/fusion/fusionnats` (adapt); `graph/readiness` edited; `scripts/cover-check.sh` gains `pkg/fusion/fusionnats`
(03B list). Migration (`pkg/fusion/MIGRATION.md`): `New` returns an error; no `Status`, no `Close`; poison is listed.
Pin lines 658 + 1,403; this PR about 450 + 1,500 with the smoke test (hand estimate), under the ~3,000 guide, one
package plus its proof. Order: no overlapping PR; tag v0.1.0-alpha.3 after merge.
