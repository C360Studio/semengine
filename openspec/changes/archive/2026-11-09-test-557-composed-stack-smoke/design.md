# Design: test-557-composed-stack-smoke

Base `d15e6c4`. Inventory: `inventory.md` (sha256 in the PR). Premises cite it as `inv §n`.

## Options considered

- **Do nothing / the issue's first scope** (three components, hand-wired): rejected by the owner (comment 6729015840).
- **Extend `processor/graph-query/composed_integration_test.go`**: keeps hand wiring (inv §1); the owner requires the
  real startup path.
- **Stack in the test process**, a fresh `config.Manager`, `ServiceManager` and client per boot
  (`StartAll` after `StopAll` is refused, inv "Seams"), prochost only for case 2. Cheaper to debug; but the observer
  shares a process with what it observes, and case 2 still needs a child.
- **Stack in a child process for every boot (recommended, D3).** One way to start and stop; a restart is a real new
  process; the observer can hold nothing but public handles.
- Ports: **H1 (recommended)** the test picks the port; **H2** production change (honour an explicit kernel pick, add
  an exported bound-address getter): new exported surface for one test, and the adopter gap (inv, adopter item 1) is
  wider than ports. H2 goes to a follow-up issue with the rest of that gap.

## D1. Where the test lives

`internal/smoke/`: `doc.go` (package comment only: test-only, what it proves), `stack_integration_test.go` (the
composition root, the child helper, producer and observer), `smoke_integration_test.go` (the five tests below).
Not `service` (in-package tests read private state, `service/service_manager_test.go:97-104`), not `graph-query`
(the flow is wider than one component). `scripts/test-pkgs.sh integration` picks the package up; `TestMain` owns
the shared server; cases 3 and 4 start their own with `// Own server: restarts the server mid-test.`

## D2. The composition root

One JSON configuration, a Go constant, filled with the NATS URL and ports at run time. The child does what a `main`
would, in order: `config.NewLoader().LoadFromBytes`; `natsclient.NewClient` with defaults (infinite reconnects);
`config.NewConfigManager` and `Start`; `config.NewStreamsManager(...).EnsureStreams` (ENTITY declared bounded, file
storage); a component registry with the four `Register` calls; a payload registry with the framework payloads and
the test entity type; `service.RegisterAll`; `NewServiceManager`, `ConfigureFromServices`, `StartAll`. It waits for
SIGTERM, then `StopAll`, `config.Manager.Stop`, `Close`, each under the stop budget the service tests use.
graph-embedding runs BM25 only (no provider).

## D3. Child per boot, observer outside

`prochost.Helper` runs the root; `prochost.Start` boots it with the URL and ports in its environment. A clean stop is
SIGTERM and `Wait`; a kill is `Kill`. The test confirms `Alive` before every signal or inspection. The parent is the
producer (JetStream publish on `entity.<...>`, PubAck kept as "acknowledged") and the observer.

## D4. Ports and the health read

The test picks a port with `net.Listen("tcp", ":0")` (the family the ServiceManager binds, `":"+http_port`), with
`// Every address: the ServiceManager binds ":"+http_port` above it, closes it, and writes it as `http_port`. Metrics
gets a second picked port, so case 1 can record graph-index's reindex counter (#108: noted, not asserted or timed).
The observer reads component health over HTTP, `GET /components/health` (`service/service_manager_http.go:30`,
`service/component_manager_http.go:88`), dialled at `[::1]:<port>`: on a Mac, `127.0.0.1:<port>` can belong to
another program that bound IPv4 only (#390, port 63150). graph-embedding's `embedding worker watch lost` and the
follower's lost feed are read there and in the `GRAPH_STATUS` envelopes. An empty component list reads healthy
(`health/helpers.go:44-45`), so the check requires all four components by name, a total of 4, each `healthy: true`,
not only the overall verdict.

## D5. The fixed inputs and one round-trip check

About a dozen entities with known IDs, names, an alias, and relationships forming a known path. `checkAnswers(want)`
is the one oracle, written from the inputs, never from a reply: `entity` (each statement once), `relationships`,
`pathSearch` (the known path), `summary`, `entityByAlias`, `semantic` and `similar` (D9); one `EMBEDDING_INDEX` record
per entity with status generated. `localSearch` cannot answer in this stack (no production code writes community
records, inventory §2): its fixed answer is the refusal `index_not_ready` naming `COMMUNITY_INDEX`.
`TestSmoke_OneConfigAnswers` boots, checks, edits one entity, deletes one, and checks again.

**Every wait is ordered after the write it checks.** An empty graph reads ready before ingest, and `GRAPH_STATUS`
envelopes and `EMBEDDING_INDEX` records are file-backed and outlive the process, so "ready" alone can be stale. After
each batch the observer waits, in order: graph-ingest's durable consumer acknowledged up to the batch's last publish
sequence (graph-ingest's own ordering event; its readiness record carries no indexed revision,
`processor/graph-ingest/readiness.go:237`, `graph/index_status.go:63`, so it is only required not degraded); then the
`ENTITY_STATES` revision, read after that; then the graph-index and graph-embedding envelopes' indexed revision at
or above it; only then `checkAnswers`. Declared reads beside the public paths: `EMBEDDING_INDEX`, graph-ingest's
consumer info, the `ENTITY_STATES` status, and (case 2) graph-ingest's applied-sequence bucket.

## D6. Restart cases (each must pass)

After every restart in cases 1, 2 and 3 the producer publishes one new entity and one edit, and the check requires
their answers and their embedding records, which no earlier process can have written.

1. `TestSmoke_StackRestartKeepsAnswers`: SIGTERM, new child, old answers plus the new writes.
2. `TestSmoke_KillMidIngestKeepsAcknowledgedWrites`: in the first child, the injected logger holds graph-ingest on
   the "Entity ingested" record of one fixed entity, published last in the batch (a constant, so the hold always
   fires; precedent
   `settlement_integration_test.go:217-260`). The stream lane has no appending statement; a second application
   still shows, as a doubled statement. Lanes are serial per entity, so publishing it last leaves nothing queued
   behind it. Before the kill the test proves mid-ingest: the consumer's ack floor reaches the held entity's
   sequence minus 1 with exactly one pending, and the held entity's applied-sequence record is absent (as
   `settlement_integration_test.go:243-249`). Kill, new child: every acknowledged publish answers with each
   statement once; index and records catch up.
3. `TestSmoke_BrokerRestartBetweenRuns`: SIGTERM, `natsfixture.Restart`, new child, old answers plus the new writes.
4. `TestSmoke_BrokerRestartUnderRunningStack`: `Restart` while the child runs; a second batch, an edit and a delete
   are published after; the graph-index and graph-embedding keys reach their revision, graph-ingest's reads not
   degraded, with the extended answers; `/components/health` lists all four components, each healthy.

In cases 3 and 4 the test requires the fixture's URL unchanged across `Restart`, as `restartKeepingURL` does
(`processor/graph-ingest/memory_restart_health_integration_test.go:22-27`): Docker does not promise it
(`natsfixture/restart.go:25-29`), and a remapped port then fails as a fixture failure, not a stack one.

Waits read envelopes and answers; `go test -timeout` is the only deadline. A latched state the observer sees (lost
feed, `reset_required`, `embedding worker watch lost`) fails the test at once, naming it.

Examples, not generated checks (`docs/testing.md`): restart history is order-dependent, but the owner named the four
histories that matter; each is a fixed sequence checked against the oracle at every step. A generated history of
restarts and writes needs a reference model of the whole stack, which this change does not build.

## D7. Shown able to fail

`task mutate:check` runs no integration-tagged test (`mutation-check` spec, "Nothing selected"), and a killed child
writes no coverage, so each failure is shown by hand: the mutant applied in a scratch copy of the tree, the failing `go
test -tags integration` run recorded verbatim in `notes/<task>.md` with the patch. A run that would only end at the
timeout passes a shorter `-timeout` on its command line. Mutants: graph-embedding disabled in the configuration (records
missing); graph-index dropping an outgoing edge (path); graph-ingest writing the applied-sequence record before the
entity, and its stream lane keeping replaced statements (case 2: record seen before the kill, a doubled statement). The
guard-off mutant survives case 2 by construction: the hold sits before the record write, as in graph-ingest's own
settlement test, so the redelivered input has no record and the stream lane's replace keeps it single (`notes/2.3.md`);
the follower's stream-creation comparison inverted (case 4: lost feed seen at once).

## D8. If case 4 fails on main

Probe (inv "Probe", three runs): the follower's watch survives and the stream keeps its creation time, so no fix is
expected. If the composed test still fails (graph-embedding's watches, `config.Manager`'s watches, graph-query's
subscriptions were not probed), the failure is filed as an issue with the test output. A fix within one package and
smaller than the test is task 2.6 of this change, with its own failing-first package test. A larger fix (a new
mechanism, such as re-binding a lost watch, which the follower spec rules out today, `entity-state-follower` spec
:86-91) is its own OpenSpec change and PR, merged first; task 2.5 then carries `Hold: #<that PR>`. Either way the case stays
required: this change does not archive with case 4 red.

## D9. Search checks and merge order

PR #558 merged first (67a8650), so `semantic` and `similar` are in `checkAnswers` and run in every restart case
(task 3.2, `notes/3.2.md`). BM25's statistics live in memory and depend on arrival order, so the checks assert which
entities score above 0, never a score: each entity's own name word finds it alone, and `similar` answers the entities
whose names share a word, as a set. The verbs answer from a vector cache that claims no revision, so the checks first
wait for one search on every entity's word to answer exactly the live entities. No D8 fix was needed (task 2.6).

## Residual risk

- Ports: between the pick and the bind the whole child boots, twice per boot (HTTP and metrics), so another program
  can take the port: a loud boot failure naming it. No retry until one is seen (natsfixture's one retry, #345).
- Restart ports: a restart boots on the first child's two ports, which the configuration bucket holds, so another
  program can take one between the stop and the next boot: a loud boot failure naming it.
- `[::1]` needs an IPv6 loopback; a host without one fails the first health read, naming the address.
- `config.Manager`'s watch ends silently when its channel closes (`config/manager.go:633-635`), so case 4 cannot
  observe it; filed as an issue (task 3.4), not proven here.
- Run time: each broker restart costs about 20 s of watch stall (nats.go's timing, not promised, ADR-112), so the
  package takes minutes inside the integration run's 10 m.
- Case 4's stall: after a broker restart under the running stack, the writes published after it were indexed and
  answered about 19.7 s after `Restart` returned (`notes/2.5.md`). Nobody has read whether graph-index or
  graph-embedding report ready while their watch is down in that window; the test checks only after the writes.
- A product's own `main` still has to know six things (inventory, adopter seam): a follow-up, not fixed here.
