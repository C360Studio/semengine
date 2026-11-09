# Design: `pkg/fusion/fusionnats` and the no-embedder proof

Base `468bac4f` (claim on main `7901bfda`); pin `8b99efe9`. Inventory: `inventory.md` (§ n below). Owner rulings:
split row D and ruling 4 (PR #550 comment 6716972354), tag after PR D (#9 comment 6727586862), consumer fixtures are
consumer-owned (#568). **Goal.** A product composes `fusion.Engine` over NATS with one call, gets one answer per query
in which a poisoned seed is listed, not fatal, and can run the tier-0 stack with no graph-embedding and see which
answers it has. **Non-goals:** `searchGraph`'s fallback (#563: fusionnats never calls it); graph-ingest's batch or
prefix wire; graph-query code (owner question 2's option C, if chosen, is its own PR).

## D1. Construction: one call, nothing running (N4, N5, N9)

`New(requester, timeout) (*Client, error)`, the `graph.NewExactEntityReader` rule (`graph/exact_entity.go:46-59`): a
nil requester and a negative timeout are refused; zero is natsclient's `DefaultRequestTimeout`. `New` reads the six
graph-query rows it uses (`entity`, `batch`, `prefix`, `byName`, `relationships`, `semantic`) from `graph.QueryVerbs()`
through an unexported lookup that takes a `[]graph.QueryVerb` and refuses, naming the verb, when a row is missing (the
shape of graph-query's `handlerFor`, `processor/graph-query/query.go:88-95`). The client holds no goroutine, no watch,
no context, no lock: `Status`,
`Close`, `statusWatcher`, `awaitFirstStatus`, `unknownCause` and their tests are not carried (D4). *Proof:*
`TestNewRefusesNilRequester`, `TestNewRefusesNegativeTimeout`, `TestSubjectsComeFromVerbTable` (each call's subject
equals its row's), `TestVerbLookupNamesMissingRow` (a table without `batch`), unit.

## D2. The tree's wire (N1-N3, N6-N8, N10)

Each reply is decoded by its row, never by its content (N2). An enveloped row (`byName`, `relationships`, `semantic`)
decodes `graph.QueryResponse[T]` and refuses a reply that names no `producer`, as graph-query's `decodeIndexReply` does
(`processor/graph-query/query.go:333-343`): `graph.NewQueryResponse` never builds one, so a bare body or `{}` is
refused. A bare row (`entity`, `batch`, `prefix`) is decoded as the row's type, unknown keys ignored, so an additive
field such as the batch's `missing` keeps working; the cost, stated: an envelope sent on a bare row is not detected
by shape (a prefix reads as no entities, a batch as every ID `unknown`). Both sides read the same table row and
graph-query forwards bare bytes unchanged (`processor/graph-query/doc.go:9-12`), and `entity` keeps the pin's checks
(an entity present, a non-zero `kv_revision`). Requests use `graph.EntityRequest`,
`EntityBatchRequest`, `PrefixQueryRequest`, `SemanticRequest` (its `scope` omitted when empty, ADR-071); byName and
relationships keep local structs (no `graph` type). `Neighbors` reads `predicate` (N3). An NL `Resolve` copies
`unreadable_records` and `failed_records` into `fusion.Resolution` (N6); symbol and prefix leave both 0. `Entity`
decodes through `graph.UnmarshalEntityState` (N10); `entity_not_found` is `(nil, nil)` as at the pin. Kept as at the
pin: the decoded-ID and decoded-state validators, `reconcileHydration` (ADR-084 D6), `Names` dedup and cap, the
client-side prefix and scope checks (N13), zero IDs sending no request (§ 9). *Proof:* the pin's resolve, entity,
batch, neighbors, names and reconcile tests, and its exact-entity tests (`slice_e_contract_test.go:16-53`), on the
tree's wire; `TestNeighborsReadsPredicate`,
`TestSemanticCountsReachResolution`, `TestEnvelopedRowNeedsProducer` (a bare body and `{}` on each enveloped row are
refused; replaces pin
`slice_e_contract_test.go:55-105`),
unit. Reconciliation is a law over interacting inputs (duplicates, an ID in both lists or in neither), so
`TestReconcileTotal` is generated (rapid, seed recorded): every distinct requested ID appears once across `Entities`
and `Unhydrated`, `Entities` in request order, against a reference model; the pin's table cases stay as witnesses.

## D3. A poisoned seed is unhydrated, not a failed answer (N11; ADR-109 D1)

graph-ingest refuses the whole batch when one entity is poisoned, naming the IDs only in text (§ 2; pin ADR-079 D3).
**O1 (recommended).** The trigger is the error the batch's `RequestClassified` call returns, before any decode,
when `graph.IsStateContractError` matches it (code `graph_state_reset_required`, never text). The client's own
`ValidateDecodedEntityStates` and `UnmarshalEntityState` refusals match the same check, so they are told apart by
where they arise, not by class: a refusal from decoding a reply is returned unchanged and never starts or feeds the
fallback. On a trigger, `Entities` sends one `entity` request per distinct requested ID: a reply is hydrated;
`entity_not_found` is `not_found`; a `RequestClassified` refusal with the reset code is unhydrated `error`; the
client's own `UnmarshalEntityState` refusal of a reply, any other error and an ended context are returned. An empty ID
is listed unhydrated `error` with no request, the reason the batch gives it (`processor/graph-ingest/query.go:162-165`),
not sent to be refused `invalid_request`. On the batch verb the code means poison only (§ 2). Cost: one request per
distinct requested ID, only while poison is present; the engine asks for at most `resolveLimit` (40) seeds. **O2.**
graph-ingest's batch lists poisoned IDs in `missing` with reason
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
`TestPoisonFallbackReturnsOtherError`, `TestPoisonFallbackEndedContext`, `TestPoisonFallbackEmptyID`
(`TestPoisonedSeedUnhydrated` puts the poisoned seed second of three and checks the other two in request order),
`TestOwnValidationNotRetried` (a batch reply its own validation refuses sends no `entity` request; a per-ID reply its
own decode refuses is returned), each shown to fail by a mutant. Real wire, integration, on the package's shared
`natsfixture` server through `natsfixture.Open` (#195): graph-ingest, graph-index, graph-query and graph-embedding
(`embedder_type` bm25) built with their factories as `processor/graph-query/composed_integration_test.go:66-107`
builds two of them. `TestRealRespondersAnswer` drives every `RetrievalClient` method: `Resolve` symbol (byName,
graph-index), prefix, NL with a scope (graph-embedding), `Entity`, `Entities`, `Neighbors` both directions, `Names`.
`TestPoisonedSeedOverRealResponders` Puts bytes the stored-value rule refuses under one key after graph-ingest's
`Start` returned, so the read path finds it, not the boot sweep (pin ADR-079 D2), and asks `Entities` for it and two
good IDs.

## D4. Readiness: none (ruling 4; ADR-109 D3)

Changes ADR-083 and ADR-066 item 3. PR C removed fusion's status copy (#110 item 2); fusionnats watches no key and reads
no status: readiness arrives as
`index_not_ready` on each reply, which
`Engine.Fuse` already reads (`fusion` spec). This removes the pin's detached watch (N5) and the one bounded wait that a
deployment without graph-index spent on its first call (`client.go:149-169`). `graph/readiness.Reading.Raw` had one
pin reader, fusionnats' `Status` (**measured** `grep -rn 'reading\.Raw'` at the pin: `client.go:227` only), so it is
removed; the watcher doc's sentence on fusionnats (`graph/readiness/watcher.go:23-25`) goes and its graph-clustering
reader stays; the ledger's open question goes
(`:2195-2198`). *Proof:* `go list -deps ./pkg/fusion/fusionnats` names no `graph/readiness` (in D5's contract test).

## D5. The no-embedder proof (`docs/setup-plan.md:439-444`)

**P1 (recommended).** `internal/smoke` gains `TestSmoke_NoEmbedderAnswers`: the child boots graph-ingest, graph-index
and graph-query from one configuration with no graph-embedding entry and no graph-embedding registration (an env
value selects the second configuration), publishes the fixed entities, waits on graph-ingest's and graph-index's keys,
and checks: all three components healthy and `GRAPH_STATUS` has no `graph-embedding` key; `fusion.Engine` over
`fusionnats` answers a symbol lens and a prefix lens with the expected nodes, relations and paths; an unknown name is a
miss; an NL lens's `Fuse` returns a refusal naming `graph.embedding.query.search`, never an empty answer: under the
recommendation (Q-B) today's transient `internal`; under option C the class and code that graph-query PR defines.
`TestFusionNATSImportsNoEmbedding` (`internal/harness/contract`)
fails if `fusionnats`' closure reaches `graph/embedding`, `processor/graph-embedding` or `graph/readiness`, with a
sensitivity case as `embeddingprovider_test.go` has. The SemSource composition itself stays the SemSource owner's
(plan `:440`). **P2.** fusionnats' own integration test only: no configuration boot, proves no composition. **P3.**
Leave it to SemSource: the plan asks SemEngine for the proof. How fast a request with no responder is refused depends
on nats.go and the server and is not promised; the test asserts the refusal only.

The plan asks for "explicitly admitted graph-only interfaces, not unsupported NL verbs" (`docs/setup-plan.md:439-440`).
Today graph-query still offers `semantic` with no embedder and, on no answer, refuses transient `internal`
(`processor/graph-query/query.go:158-159`; its spec, "A refusal reaches the caller intact",
`TestNoRespondersIsTransient`).
**Q-A (withdrawn, round 2).** Refusing "do not retry" on `natsclient.IsNoResponders` cannot tell a composition with no
embedder from one whose embedder is briefly unsubscribed, which natsclient calls temporary at startup and after a
reconnect (`natsclient/errors.go:333-336`; the window `TestSmoke_BrokerRestartUnderRunningStack` exercises), and it
reverses graph-query's spec. **C (config).** graph-query gains one config key admitting the embedding verbs; when it
says no, `semantic` and `similar` refuse at once, before any request, with class `invalid` (the composition does not
offer the verb; not retryable) and a new code in `graph` the owner names, naming the verb. Changes: the two forward
entries (`processor/graph-query/query.go:32-33`), not `ask`; one requirement added to the graph-query spec, nothing
reversed; the key's default is the owner's (on keeps every composition as it is; off makes the admission explicit
and breaks SemSource's and the smoke configs, a migration note). Two packages, so its own PR, issue and change, before
task 3.1. **Q-B (recommended).** Keep today's refusal for alpha.3 and file C as the issue's shape. PR D stays one
package; the plan line is met by what the proof covers: the graph-only interfaces are proven, and the NL verb is shown
refused, never answered empty. Cost: a caller retries a word question that cannot succeed until C lands.

## D6. ADRs, issues, order, size

Stale text this PR rewrites: port notes ADR-062 (`:17`), ADR-066 and ADR-083 (fusionnats watches nothing, ruling 4),
ADR-071 (`:13-14`, the scope is now sent), ADR-084 (`:16-17`); ledger `pkg/fusion` row and its comment
(`docs/admission-ledger.yaml:4592`, `:4600`, `:4627`, `:4631`) and the ADR-062 row (`:4694`); `fusion` spec
"A fatal seed read or an ended context fails the call" (`openspec/specs/fusion/spec.md:22-31`), MODIFIED: its reset
example becomes a prefix resolve, since fusionnats lists a poisoned seed instead. Issues: none retired. #195 stays
(other packages; this PR adopts it for fusionnats); #110 stays (item 3; a comment that item 2 is done); #563 stays.
Filed by this PR, `status:blocked` on it: graph-ingest's prefix read refuses a whole page for one poisoned entity,
which fusionnats cannot scope (§ 2); under Q-B, also graph-query's word search with no embedder (shape C). Ledger: new
row
`pkg/fusion/fusionnats` (adapt); `graph/readiness` edited; `scripts/cover-check.sh` gains `pkg/fusion/fusionnats`
(03B list). Migration (`pkg/fusion/MIGRATION.md`): `New` returns an error; no `Status`, no `Close`; poison is listed.
Pin lines 658 + 1,403; this PR about 450 + 1,500 with the smoke test (hand estimate), under the ~3,000 guide, one
package plus its proof. Order: no overlapping PR. Under Q-B no other PR; under C, the graph-query PR merges, this
branch merges `origin/main`, then task 3.1. Tag v0.1.0-alpha.3 after this PR merges.
