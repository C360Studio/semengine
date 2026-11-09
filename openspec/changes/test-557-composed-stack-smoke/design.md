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

## D4. Ports

`net.Listen("tcp", ":0")`, read the port, close, write it as `http_port`; a comment `// Every address: the
ServiceManager binds ":"+http_port` sits above (rule #390, #391). Metrics gets a second picked port, so case 1 can
record graph-index's reindex counter (#108: recorded in the notes, not asserted, not timed). A bind clash fails the
boot loudly naming the port; no retry is added unless it is observed (natsfixture's one retry, #345, is the model).

## D5. The fixed inputs and one round-trip check

About a dozen entities with known IDs, names, an alias, and relationships forming a known path. `checkAnswers(want)`
is the one oracle, written from the inputs, never from a reply: `entity` (each statement once), `relationships`,
`pathSearch` (the known path), `summary`, `localSearch`, `entityByAlias`; one `EMBEDDING_INDEX` record per entity
with status generated. It first waits for all three readiness keys ready and health healthy (events, not time).
`TestSmoke_OneConfigAnswers` boots, checks, edits one entity (answer and record follow), deletes one (gone from
answers, record removed), checks again.

## D6. Restart cases (each must pass)

1. `TestSmoke_StackRestartKeepsAnswers`: SIGTERM, new child, same answers, same record set.
2. `TestSmoke_KillMidIngestKeepsAcknowledgedWrites`: the child's injected logger holds graph-ingest on the k-th
   "Entity ingested" record (precedent `settlement_integration_test.go:217-260`). Before the kill the test proves
   mid-ingest: at least one entity answers and graph-ingest's durable consumer reports an unacknowledged tail (a
   declared read). Kill, new child: every acknowledged publish answers exactly once; index and records catch up.
3. `TestSmoke_BrokerRestartBetweenRuns`: SIGTERM, `natsfixture.Restart`, URL unchanged, new child, same answers.
4. `TestSmoke_BrokerRestartUnderRunningStack`: `Restart` while the child runs; the producer reconnects; a second
   batch, an edit and a delete are published after; all keys ready again with the extended answers.

Waits read readiness envelopes and answers; `go test -timeout` is the only deadline. A latched state seen by the
observer (lost feed, `reset_required`, `embedding worker watch lost`) fails the test at once with the state named,
so a regression fails fast, not at the timeout.

## D7. Shown able to fail

Each test has a recorded failing run in `notes/<task>.md`: a stopped component (graph-embedding disabled in the
configuration fails the record check), and `task mutate:check` mutants: graph-ingest acknowledging before its commit
(case 2 loses a write), the follower's stream-creation comparison inverted (case 4 sees a lost feed), graph-index
dropping an outgoing edge (path check). Survivors are reported as survivors.

## D8. If case 4 fails on main

Probe (inv "Probe", three runs): the follower's watch survives and the stream keeps its creation time, so no fix is
expected. If the composed test still fails (graph-embedding's watches, `config.Manager`'s watches, graph-query's
subscriptions were not probed), the failure is filed as an issue with the test output. A fix within one package and
smaller than the test is task 2.6 of this change, with its own failing-first package test. A larger fix (a new
mechanism, such as re-binding a lost watch, which the follower spec rules out today, `entity-state-follower` spec
:86-91) is its own OpenSpec change and PR, merged first; task 2.5 then holds on it. Either way the case stays
required: this change does not archive with case 4 red.

## D9. Search checks and merge order

PR #558 shares no file with this change; it edits `graph/follower/follower.go` and `graph/embedding/worker.go`,
where a D8 fix would land. Whichever merges second carries the search checks (branches update by merging
`origin/main`): if this merges first, #558 adds `semantic`/`similar` to `checkAnswers` (comment posted on #558,
task 3.2); if #558 merges first, task 2.1 adds them.
A D8 fix in `graph/follower` waits for #558 or is coordinated on it.

## Residual risk

Run time: each broker restart costs about 20 s of watch stall (nats.go's timing, not promised, ADR-112), so the
package takes minutes inside the integration run's 10 m. A product's own `main` still has to know six things
(inv, adopter seam): filed as a follow-up, not fixed here.
