# ADR-108: One processor shell and one ENTITY_STATES follower

## Status

**Proposed (2026-10-09), for the owner's ruling on #9.** Drafted on draft PR #163 (`Addresses #109`) under "Ruled 2"
(#9 comment 6085243841). It does not close #109, which stays the refactor. Change 3's design and the follower ports
(changes 4, 5 and 7) cite it. Mechanics (what each call returns, lock and join order, test names) belong to the
capability spec the first port writes, not here (pin `docs/adr/README.md`). Pin cites are `path:line` at SemStreams
`8b99efe9`; PR #93 cites are at its head `3d66c91`; the inventory accompanies this record on PR #163.

## Context

Two coined terms. The *processor shell* is the part of a component that is the same for every component: starting once,
stopping, cleaning up a start that failed, and answering `Health()` and `DataFlow()`. The *ENTITY_STATES follower* is
the part that is the same for every component that keeps a projection of the entity graph: watching the `ENTITY_STATES`
bucket, replaying its snapshot before live updates (the *validating phase*, in which each stored entity is checked
against the graph-state contract before anything is served), holding a *reset latch* (a flag that is set once a stored
entity proves unreadable and stays set until the process restarts), holding a *degrade flag* (set when the feed is lost
or a required index write failed), advancing a *watermark* (a low-water mark: every delivered revision up to it has been
applied, `pkg/revlag`, ADR-066), and publishing the *readiness envelope* (the `GRAPH_STATUS` record a consumer reads
before relying on this producer).

Seven `processor/graph-*` components at the pin write both by hand, and the copies are identical where they should be
one thing: `Health` and `DataFlow` of graph-index-spatial and graph-index-temporal differ by 0 lines, `DataFlow`
differs by 0 lines in six of seven, and the follower loop of spatial (`graph-index-spatial/component.go:690-856`),
temporal (`:712-886`), graph-embedding (`:1417-1716`) and graph-index (`:971-1066`) is one loop in four spellings
(graph-index's omits the validating phase; its reset latch is set from the indexing path instead, `:1525-1541`,
through a compare-and-swap in `watermark.go:181`). The one-shot start rule is spelled nine ways because the contract
leaves same-instance restart "not a portable guarantee" (`component/lifecycle.go:59-62`). Spatial and temporal publish
no envelope and write `reset_required` and `degraded` into the free-text `HealthStatus.Status` instead (`:354-358`;
temporal `:364-368`), so "ready" has two spellings: the envelope a consumer gates on (ADR-083, ADR-088) and a health
string the manager serves at `/health`. Both carry a `workers` setting that is validated, defaulted and logged but
drives nothing (`:36-153`, `:581`). The inventory's category 2 lists every site.

PR #93 (change 2) landed half of the shell: `internal/lifecycleguard.Guard` (`guard.go:41`, `:85`) owns one Start,
the Stop ordering and the cleanup-pending retry, and rolls a failed Start back through the public
`pkg/lifecyclecleanup.RollbackFailedStart` under #77's ruling; graph-ingest composes it
(`processor/graph-ingest/component.go:396`, `:919`). Its design (`design.md:1308-1310`) already says where each half
can live: "change 4 homes [the follower] in a graph package. The processor shell imports no graph package and can live
in `component`". D13 (`:611-657`) kept the guard internal because no consumer carried a copy of it. Outside this
module, semboids' sim component writes its own lifecycle against an older contract (`internal/sim/component.go:146`,
`:546` at `8c03cc53`: `started bool`, `Stop(timeout time.Duration)`, on semstreams `v1.0.0-beta.160`); whether
semteams' components do the same at their current head was not measured (only the pin's agentic-model site is in
evidence, #77). A component author outside this module has to know seven facts to get the lifecycle right.

The owner's reason for deciding this once (#9, Ruled 2): otherwise each lifecycle fix (one-shot, the #38/#77 rollback,
the reset latch) is applied and proven seven times, inside seven port pull requests.

## Decision

The shell and the follower are two primitives, each with one home. A graph processor is the shell, plus a follower if
it projects `ENTITY_STATES`, plus a *plug-in*: the code that differs between processors, and only that.

**Decision 1. Two primitives, two homes, over one state machine.** The lifecycle state machine (today's guard,
`internal/lifecycleguard`: one Start, the Stop ordering, the cleanup-pending retry) stays its own primitive, because
services need it and cannot compose the shell: the pin's `ComponentManager` (`service/component_manager.go:95-104`) is
not a `Discoverable`, and `MessageLogger` embeds `*BaseService`, whose `Health()` returns `health.Status`
(`service/base.go:202`), not `component.HealthStatus`. Services compose the state machine; components compose the shell,
which composes the state machine and adds `Health()`, `DataFlow()` and the counters. The shell is graph-free and lives
in `component`, public, as the one way a SemEngine component implements `LifecycleComponent`. The
follower decodes `graph.EntityState`, latches a `graph.StateResetReason` and publishes through `graph/readiness`, so
it cannot live in `component` (`component-registration` delta; #91 ruling D; #93 `design.md:1308-1310`). It lives in
one package under `graph/` that imports `graph` and `graph/readiness` and nothing under `processor/`; the first
follower port names the package. #109's "both in `component`" does not stand for the follower.

**Decision 2. When each lands.** The state machine is on PR #93; change 3's two services
(`service/component_manager.go:102`, `service/message_logger.go:234`) adopt it. The shell lands with change 4, the first
group with a component that answers `Health()` and `DataFlow()` (graph-index, graph-query, objectstore), and
graph-ingest moves onto it then. The follower lands with change 4's graph-index. The other answer, the shell in change
3, moves graph-ingest, which already answers `Health()` and `DataFlow()` and composes the guard (`component.go:396` on
PR #93), onto it one group earlier; that move is its cost. Owner question 3.

**Decision 3. What the shell owns, so each rule is fixed and proven once.** The portable floor as the `lifecycle-suite`
spec states it (`:9-20`: nil and ended contexts refused, Stop before Start safe, controlled and abort Stop, repeated
Stop a no-op, second Start refused) and the cleanup-pending retry D13 lists, with these additions and changes: (a)
there is no same-instance restart, ever (ADR-094 and ADR-095 kept); graph-index's second `Start` on a running
component returns nil today (`component.go:620-622`) and refusing it is a behaviour change its port carries as an
`adapt` item; (b) a failed Start is rolled back through `pkg/lifecyclecleanup` and a cleanup that fails stays pending
(the #38 exception, #93's `lifecycle-suite` delta `:52-58`); (c) `Health()` and `DataFlow()` are computed by the
shell from state it holds (running, start time, messages, bytes, errors, last activity) and one verdict the follower
or the plug-in supplies; (d) configuration is validated once, at construction, and `Initialize` is idempotent with no
I/O; (e) the `LifecycleComponent` doc comment states these rules in plain words (#153 item 4), which binds only an
author who reads it: for one who does not compose the shell the do-nothing path is unchanged, and no check catches
it. The shell's rules are tested in the shell's package by the suite on a shell-only owner. That run is in addition
to, never instead of, each component's own suite run through its `Observe` adapter (`lifecycle-suite` `:102`) and
each ported component's own test of the retained-cleanup branch (#77 item 2; the delta `:52-58`).

**Decision 4. What the follower owns, and what passes between it and a plug-in.** The follower owns everything between
"the bucket exists" and "this entry is yours": the bounded wait for the bucket; one `WatchAll` watcher; the
snapshot-then-live bootstrap with its validating phase, in which it decodes each delivered value; the reset latch, which
it sets when that decode fails or when the plug-in reports a reset reason, and which holds for the process lifetime
(ADR-074 `:66-80`; ADR-079 decisions 2 and 3); the degrade flag, set when the feed is lost or when the plug-in reports a
required write failed (ADR-082); the bootstrap-complete latch and bootstrap target (ADR-084 decision 2, `:84-100`;
ADR-088 decision 5, `:61-66`: process-lifetime scope for these producers); the watermark, advanced by the plug-in's
completions (ADR-066); and the readiness tick. A lost feed is declared: at the pin graph-index returns from a closed
channel (`:1001-1004`) and from a failed `WatchAll` (`:982-985`) with no mark, and its stuck-watermark detector
(`watermark.go:16-19`, 30 s) reports `degraded` late and indirectly; the port writes a failing test for that first. The
plug-in receives the entry (key, revision, tombstone or not) and may re-read current truth at apply time, as graph-index
(`processEntityWork` to `reconcileEntity`, `:1138-1147`, `:1328`, `Get` at `:1339`) and embedding (`:1597`, `Get` at
`:1601`) do: "each lane item applies current truth rather than its stale event snapshot", which is what keeps a repair
item and a watcher item for the same entity from applying in the wrong order; applying the delivered value instead
brings that bug back. It returns one of: applied, failed (counted toward the degrade flag), or a reset reason, a failed
decode included. graph-index's revision coalescer (`revision_coalescer.go:18`) and its keyed pool (#107, ADR-072) sit
between delivery and apply and are the plug-in's; the repair loops (graph-index `:1201`, embedding `:1266`) re-drive the
plug-in's own failed writes and are the plug-in's, the follower reading only the count they report. What a caller
observes: after a `Stop` that returned nil, nothing of the follower is left, no goroutine, no watcher
(`background-work`, the `synctest` test; the component's `Observe` adapter lists the watcher).

**Decision 5. What a plug-in supplies, and what it never holds.** Its apply step, its index codecs and output
buckets, its query handlers, its failed-write count, and, for graph-embedding, its own completion rule (ADR-066's
terminal-outcome watermark). A plug-in never holds a watcher, a reset or bootstrap flag, a watermark, a publisher or
lifecycle state. graph-clustering and graph-query have no follower: each composes the shell and supplies its detection
loop or its handlers as the plug-in.

**Decision 6. Spatial and temporal stay two component types, each a projection plug-in.** Names, ports, buckets and
subjects are unchanged; they are consumer wire (semconnect's geo and temporal seams). No mode flag and no shared
"projection component": one component with the index swapped is the diagnosis, and a flag would be a second channel
for the same fact. `workers` is removed from both; under strict decoding a configuration still carrying it fails at
boot naming the key.

**Decision 7. One projection per producer.** The follower publishes through the shared projection
(`graph.ComputeIndexStatus` and `ComputeBacklogStatus`, pin `graph/index_status.go:221`, `:345`; on PR #93
`ComputeBacklogStatus` stays at `graph/readiness/index_status.go:183`, called by graph-ingest's `readiness.go:154`,
while `ComputeIndexStatus` leaves with task 3.12k, `tasks.md:410-411`, and returns with graph-index in change 4 under
ruling P as narrowed) over `pkg/revlag.Watermark`, supplying its observations; no component computes a second one, and
spatial and temporal, which publish nothing at the pin, publish under their own keys when ported (ADR-083 decision 1,
ADR-088 decision 1 kept). What "ready" computes, what `Health()` reports from it (#110's fix shape), the envelope's
fields, freshness (the `published_at` conflict, #110 comment 6085234022), who aggregates, and where the gate lives (#91
comment 6085720598) are ADR-109's.

**Boundaries with the parallel records.** ADR-110 (#141): the plug-in's index writes, including spatial's and
temporal's read-then-write updates (`:944`; temporal `:1054`, `:1138`), follow the write seam; the follower and the
plug-in only read `ENTITY_STATES`. ADR-111 (#24): the shell is component lifecycle, not entity workflow; `pkg/lifecycle`
(ADR-047, ADR-049) is untouched, including its own pattern watch on `ENTITY_STATES` (`manager_query.go:216`), which
is a workflow reader, not a projection.

**Alternatives rejected.** *Seven copies policed by review*: the pin's state, and the defect class SemStreams' record
shows review did not close. *One primitive for both halves*: in `component` it imports `graph` (ruled out, D17 and
ruling D); under `graph/`, every non-graph component imports the graph root. *A manager-side wrapper*: ADR-095 rejects
a name-routed lifecycle operation, ADR-096 makes the manager the owner of handles but not of a component's state
machine, and #77 item 1 keeps failed-start cleanup inside the component. *One spatial/temporal component with a mode*:
a configuration flag standing in for a type, and a wire change for the consumers that compose them. *A shell kept
internal*: the bill stays with every outside component author (seven facts), which the adopter seam inventory names
as the design gap; this is owner question 1, not a rejection.

## Consequences

- ADR-095's shared-helper ceiling (`:53-55`: "one stateless context wait helper plus the bounded failed-Start rollback
  helper ... There is no managed lifecycle wrapper") is changed: the context wait helper and the rollback helper stay,
  and the state machine, one shell and one follower are added. The record's own test for what ADR-095 guarded against is
  whether a helper is composed by the component or applied to it by name from outside; the guard passes that test, so
  #93 was within ADR-095's reason while outside its letter, which this record now rewrites. No lifecycle catalog,
  deletion knob or name-routed operation is added. - Owner question 1 reopens ADR-095 `:53-55` and #77 item 1, whose
  ruling reads "using a helper SemEngine makes public ... This keeps the pin's ADR-095 shape" and was costed as one
  public function and no managed wrapper. A public shell is more than that ruling granted; it is asked for here as a new
  question, not derived from it. - Each shell rule is implemented once; each follower rule once. A follower port carries
  an `adapt` row for its reshaped `Start`, drops its copies, keeps its own #38 test, and runs the suite through its
  adapter. Adoption, in port order: the two change 3 services, on the state machine (Decision 2); graph-index,
  graph-query and `storage/objectstore/component.go:49` (change 4); graph-embedding and
  `output/websocket/websocket.go:157` (5); rule (`processor.go:102`) and its cron scheduler (`cron_scheduler.go:54`) (6:
  the shell, yes; rule's follower is a variant with per-pattern, replaceable watchers and per-generation bootstrap,
  ADR-088 decision 5, and change 6's design decides whether the follower grows a generation or rule keeps its own);
  spatial, temporal and clustering (7). External adopters, never an obligation: semboids' sim component; semteams'
  components if they still carry a lifecycle by hand (unmeasured). - Costs: one exported type in `component` whose
  behaviour is API; two new `GRAPH_STATUS` keys with no present consumer (owner question 2); graph-index's second-Start
  behaviour changes; a configuration carrying `workers` for spatial or temporal fails at boot. - Not decided here:
  method and package names, locks and join order (the developer's, under `-race` and `synctest`), the pool's bounds
  (#107, #136), the projection's content and `Health()`'s reading of it (ADR-109).

**Owner questions.** (1) Is the shell public in `component`? Recommended yes: an outside component author composes one
type and supplies two functions, instead of seven facts found out nowhere. This reopens ADR-095 `:53-55` and #77 item 1.
The other answer, internal as D13 did for the guard, keeps both as ruled, costs nothing inside SemEngine, and leaves
outside authors as they are today. (2) Do spatial and temporal publish their own readiness keys when ported? Recommended
yes: a follower that can run without publishing is a follower with a silent half, and ADR-088 makes a new key additive.
Its cost: two keys with no present consumer, against the rule "New surface needs a present consumer" (`AGENTS.md`). The
other answer's cost: `Health()` stays the only spelling for those two until a consumer declares the key, and the
follower grows a "publish or not" switch. (3) Does the shell land with change 4, change 3's services composing the state
machine alone? Recommended yes (Decision 2); the other answer moves graph-ingest onto the shell one group earlier.

## Related

ADR-095, ADR-094, ADR-096, ADR-058 (lifecycle) · ADR-074, ADR-079 (the reset latch) · ADR-066, ADR-082, ADR-083,
ADR-084, ADR-085, ADR-088 (readiness: producer side kept, consumer side to ADR-109) · ADR-072 (keyed lanes, the
plug-in's) · ADR-075 (admission test 2) · #109 · #9 comment 6085243841 (Ruled 2) · #77 comment 6035317931 · #38
comment 5934015068 · #153 items 4 and 6 · #107 · #110 → ADR-109 · #141 → ADR-110 · #24 → ADR-111 · PR #93 D5, D7,
D13, D17, `design.md:1308-1310`.
