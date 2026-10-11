# ADR-109: Ready After Boot, Readiness Rides the Reply

## Status

**Accepted (2026-10-11).** Ruled on #9, comment 6111608867, as written at `039e13c`, with Q1 ruled as recommended.
Drafted on draft PR #178 (`Addresses #110`), ruled on #9 with the other records change 3's design cites (#9 comment
6085243841). Scope: the owner's direction on #110 (comment 6109914179), "Rescope it to
ready-after-boot only: one 'finished startup' flag per worker, broken states go to health, catch-up and my-write-landed
stay per request", and the owner's challenge there: a streaming knowledge graph is never done catching up, how far
behind is acceptable depends on the use, and "when a question is asked we give the asker the current state". The owner
accepted the shape of the Decision section on 2026-10-11 (Enjoy conversation "PR 178 readiness design rescue"; the
design read is #178 comment 6111185820). It replaces the draft at `6c3d235`, which made the "finished startup" flag a
sign a consumer waits on. Here the flag lives in two places: in process, as the latch the worker's own responder checks
before it answers (Decision 1), and on the wire, as information (Decision 3). #110 stays open as the refactor.

Evidence is the architect's inventory (#178 comment 6110136781, sha256 `dd11d813…afcaf`) and the design read.
`#93:path:line` is PR #93 at `f3d5ae5`, `pin:path:line` is SemStreams at `8b99efe9`, and `ADR-0xx :line` is a line of a
record carried in `docs/adr/` (files under Related). A cite marked *re-read* was read at the pin or on #93 for this
record, not taken from the inventory or the read; none of them is a pin probe (a run of this repository's checks).

## Context

- A *worker* keeps derived state that other components read: graph-index, graph-embedding, graph-ingest and rule at the
  pin, and spatial and temporal when ported (ADR-108 ruling 2). A *consumer* reads from a worker. A *lifecycle owner*
  is a type that starts a goroutine or acquires a resource that outlives the call that created it and exposes a
  method that ends it, the unit a porting pull request carries (archived Slice 04A design, `design.md:84-97`).
- The *startup replay* is the rebuild a worker runs after its process starts, up to a target fixed when the replay
  began, not the moving head of the stream (ADR-085 :173-176). For the rule processor the replay runs once per
  *generation*, the watcher set it recreates when its configuration changes (ADR-088 :77-79).
- The *envelope* is the JSON status record (`graph.IndexStatusResponse`) each worker writes under its own *key* in the
  `GRAPH_STATUS` bucket of the NATS key-value store. Its field `bootstrap_complete` is the *flag*. A *hard stop* is an
  envelope state that says the worker is broken, `degraded` or `reset_required` (ADR-085 :89-90).
- A *classified* error carries a class (transient, fatal) and a code, so the caller can tell retry from stop. On a
  NATS request the class reaches the caller through `RequestClassified` (`AGENTS.md`, "Rules and what enforces them").
- *Health* is a component's own report that it is running and not stuck: `Health()` returns `Healthy`, and the
  process's `/readyz`, its one readiness endpoint (`/health` and `/healthz` are not,
  `pin:service/service_manager.go:1707-1709`, re-read), reads it (`:1848`, `:1854`) after *boot committed*, the
  process's own record that its boot finished (`:1795`, `:1836`, re-read). On graph-index's read path, the
  classified `index_not_ready` reply means the index is not yet built or is broken, never lag (ADR-084 D4, :126-134).

**Why a boot window exists.** "Started" is not "rebuilt". graph-index once reported `phase: ready` at about 30 s while
lookups of symbols that did exist kept failing for minutes, so a consumer that waited for "ready" got false negatives
(ADR-066 :128-134). For the rule processor, "`Start` returning is not that signal" (ADR-088 :36-38).

**What this record leaves out.** The pin also built freshness judged on the consumer's clock, a four-condition gate, a
helper that folds several keys, and the coverage field `ready`. The owner's challenge: a streaming knowledge graph is
never done catching up, how far behind is acceptable depends on the use, and a consumer that asks a question should get
the current state with the answer. It also leaves out any sign a consumer reads before it asks: the pin's `Watcher` and
fold (ADR-083, ADR-088) are not SemEngine's contract, and no SemEngine code reads the gate across a process (Decision 3).

## Decision

### Decision 1. Readiness rides the reply

A worker still running its startup replay answers a query with the classified transient `index_not_ready`, in process,
reading no status key (`pin:processor/graph-index/query.go:185`, re-read); the caller retries and gets the current
state once the replay is done. No consumer reads a sign to decide whether to ask. This is the pin's mechanism, kept:
graph-index's `ensureQueryReady` (`query.go:158-256`, re-read) reads the ENTITY_STATES sequence for its target
(`pin:processor/graph-index/watermark.go:77`, re-read), asks the gate in process with `Fresh: true`
(`query.go:236-237`), refuses with the code while its first build is not applied (`:254-255`) and returns at once after
it (`:217-219`); it never fires on ordinary lag (`:166`). Eight handlers call it: `query.go:134`, `:289`, `:349`,
`:386`, `:529`, `:591`, `:641` and `pin:processor/graph-index/name_index.go:288` (re-read); ADR-084 D4's narrowing
names the reverse-index and by-name contract (:133-134). On #93 the code is `graph.ErrorCodeIndexNotReady`
(`#93:graph/mutation_responses.go:79`); the lifecycle manager answers it when a workflow's watch cannot bind or closes
(`#93:pkg/lifecycle/manager_query.go:210`, `:245`), the meanings outside the reverse-index path that ADR-084 D4 keeps
(:131-134).

Which calls a startup withholds is the worker's own: spatial and temporal gate their own replies with the same code at
the pin (`pin:processor/graph-index-temporal/component.go:903`; inventory 2.5). Each worker's port names its withheld
calls and the test.

*A caller observes:* a request during the startup replay, or while graph-index's unresolved write failures hold
(`query.go:213-216`, re-read), fails with class transient and code `index_not_ready`, and the same request succeeds
once the replay is done or the failures are repaired, with the state as of the reply. The other stuck states Decision 2
names answer differently: `reset_required` is fatal `graph_state_reset_required` (`query.go:209-211`; pin ADR-074
`docs/adr/074-canonical-predicate-contract.md:70-71`; `pin:processor/graph-index/predicate_contract_test.go:38-45`, all
re-read), poison refuses only the poisoned entities (pin ADR-079 D3 `:38-40`, re-read), and a failed backlog
observation refuses nothing. *Proof:* at the pin, over NATS,
`pin:processor/graph-index/replacement_reconcile_integration_test.go:170-180` (re-read; an integration test, not run:
`RequestClassified` gets the code while a write failure is unresolved, and the repair clears it) and
`TestEnsureQueryReady_FailedCountWithholdsAfterBootstrap` (`pin:processor/graph-index/failure_honesty_test.go:20`,
re-read). No pin test asserts the code before the first build; the nearest,
`TestQueryStatus_NonEmptyReplay_NotReadyUntilCaughtUp` (`pin:processor/graph-index/query_integration_test.go:587`,
integration), checks only that the call does not succeed. graph-index's port (change 4) names the test that holds the
first-build window, and each later worker's port proves its own withheld calls.

### Decision 2. Ready means boot committed and every component healthy

Healthy means running and not stuck. There is one readiness endpoint, `/readyz`, and a component determines its own
health from its own stuck states, which are named: `reset_required` (pin ADR-074 `:69-71`, re-read); graph-index's
unresolved write failures (`query.go:213-216`); graph-ingest's poison inventory (pin ADR-079 D3,
`docs/adr/079-per-entity-poison-response-authoritative-surface.md:38-46`;
`#93:processor/graph-ingest/component.go:831`); a failed backlog observation while it persists
(`#93:processor/graph-ingest/readiness.go:137-146`). A stuck state that clears (a repair, an observation that
succeeds) raises `Healthy` again; `reset_required` never clears in process (ADR-084 D4 :130-131). The rule binds every
component's `Health()`, worker or not.

A count of past errors is a metric: `ErrorCount` stays on `HealthStatus` (`#93:component/discovery.go:74-81`) and
never lowers `Healthy`. This changes the pin. Six components carry `Healthy: c.running && errorCount == 0 && ...`
(graph-index `:527`, graph-ingest `:871`, graph-embedding `:568`, graph-clustering `:876`, spatial `:366`, temporal
`:376`, each `pin:processor/<name>/component.go`, re-read); graph-ingest's counter has nine writers and no reset
(`#93:processor/graph-ingest/component.go:1411-2383`, re-read), and `/readyz` reports `not_ready` while any component
is not `Healthy` (`pin:service/service_manager.go:1853-1856`), so one processing error leaves the process not ready
until restart. The error-count term goes; the other terms stay, since each names a stuck state: the reset term four of them
carry (graph-embedding, graph-clustering, spatial, temporal) and the transport term `watchUnavailable` three carry
(`:569`, `:367`, `:377` of the same files). Who applies it: graph-ingest in the follow-up (Consequences),
graph-index in change 4, graph-embedding in change 5, graph-clustering, spatial and temporal in their ports. The pin's `service`
package has no such term; change 3, which ports it, carries the `/readyz` rule unchanged (boot committed, every
component healthy, no data-plane coverage; ADR-088 :117-119). The pin's rule processor already keeps `Health()` in line
with its own broken states (`pin:processor/rule/processor.go:499-515`).

The cost: `/readyz` goes not ready while graph-index's unresolved write failures or graph-ingest's failed backlog
observations hold, and ready again when they clear. Today graph-index's delete and list failures already lower
`Healthy` through the error count until restart (`pin:processor/graph-index/component.go:2106`, `:2134`, `:2151`,
re-read), and graph-ingest's observation failure reaches no `Healthy` at all (it feeds only the envelope,
`#93:processor/graph-ingest/readiness.go:142`, re-read); under this decision each lowers it until it is repaired or
clears. An operator sees each.

*A caller observes:* `/readyz` says ready once boot committed and every component reports `Healthy: true`; a
component's `Healthy` is false while one of its named stuck states holds and true again when it clears, whatever
`ErrorCount` says. *Proof:* change 3 carries the pin's `/readyz` tests,
`TestReadinessRequiresCommitAndClearsBeforeChildStop` (`pin:service/startup_observability_amendment_test.go:151`),
`TestStartupReadinessAndAtomicPromotion` and `TestReadinessIncludesHealthyNonLifecycleDiscoverables`
(`pin:service/startup_observability_test.go:224`, `:264`; unit tests, no build tag, re-read). For `Health()`,
on #93 `TestPoisonRepairRecoveryWithoutRestart` (`#93:processor/graph-ingest/poison_scoping_test.go:71`, re-read)
proves the poison state: not healthy at `:87`, healthy again at `:111`. Nothing on #93 tests the failed-observation
state or a nonzero `ErrorCount` reading healthy (the only `ErrorCount` in graph-ingest's tests is
`component_test.go:622`); the follow-up adds both, and each other component's port adds its own (a stuck state reads
false, its clearing reads true, a nonzero `ErrorCount` still reads true).

### Decision 3. `GRAPH_STATUS` is information, not a gate

The bucket, the key per worker and the envelope stay as #93 built them (PR #93 is frozen): `bootstrap_complete`,
`bootstrap_scope`, `ready`, `lag`, `staleness_ms`, the state and the heartbeat, with `published_at` pending Q1. They
are for operators and for semsource's MCP gateway, which surfaces graph-index's and graph-embedding's keys to agents
(`semsource:processor/mcp-gateway/tools.go:141-142`). Nothing in SemEngine waits on the bucket: the `Watcher`
(`#93:graph/readiness/watcher.go`) has no SemEngine reader, and its readers at the pin, fusion and graph-clustering
(inventory 5.3), are each port's question when ported. The gate (`#93:graph/readiness/readiness_gate.go`) has one
SemEngine reader from change 4, graph-index, which asks it in process about the status it has just computed
(`query.go:236-237`; #93 carries the gate for that reader,
`#93:openspec/changes/setup-04a-02-ingest-kernel/design.md:284-287`, re-read); no SemEngine reader asks it across a
process (semconnect's conformance probe does, Consequences). A key nobody writes, or a worker that died after writing
its key, is not readiness's problem: a consumer that asks gets a
reply or a transport error.

graph-ingest's flag: its boot work is the ENTITY_STATES sweep, which `Start` waits on
(`#93:processor/graph-ingest/readiness.go:235-237`), so `bootstrap_complete` is true once `Start` returns. The
latch #93 sets afterwards, when the message backlog first reads zero (`:234`, `:267-268`), is catch-up, which
Decision 4 keeps per request; it leaves in the follow-up after #93 lands (Consequences). `bootstrap_scope` keeps
reporting the backlog at bind (`:178`), as information (ADR-088 D6 kept); once the latch goes, `complete` no longer
says that scope was worked off, and the follow-up says so in the envelope's doc comment.

*A caller observes:* `nats kv get GRAPH_STATUS <key>` returns that worker's envelope as #93's tests pin it, and no value
in the bucket, and no reader of it, changes what a query returns. *Proof:*
`TestIntegration_ReadinessEnvelope_PublishedAndCatchesUp`
(`#93:processor/graph-ingest/readiness_integration_test.go:122`) pins the envelope as published today. The latch
tests (`#93:processor/graph-ingest/readiness_test.go:61`, `:83`) call `latchBootstrap` and leave with it; the
follow-up names the test that replaces them, which proves `bootstrap_complete` is true once `Start` returns with a
backlog still standing. That no SemEngine code reads the bucket across a process before it asks is review only: the
`Watcher` has no caller on #93 (inventory 5.3), and a port that adds one cites this record.

### Decision 4. Catch-up and read-your-writes are per request

A streaming graph is never done catching up, so no reply waits for it (owner direction, #110 comment 6109914179).

- *Catch-up*, how far behind a worker is (`lag`, `staleness_ms`), is reported, never a gate (ADR-085 D2 :94-99;
  ADR-084 D1, "lag alone never defers a consumer that declared no bound", :91).
- *Read-your-writes* compares the `kv_revision` a write returned with the worker's `indexed_revision` (ADR-084 D3
  :120-123). It stays #103's per-request option (`min_revision` on the request, `indexed_revision` on the reply;
  inventory 5.1), with one constraint from this record (inventory C8): a `min_revision` the index has not reached is
  ordinary catch-up, and `index_not_ready` is reserved for the startup replay and graph-index's unresolved write
  failures (Decision 1; ADR-084 D4 :126-128), so #103 answers it another way; which way is #103's. What a watcher of
  `ENTITY_STATES` may rely on about revisions (ADR-110 :192, one revision per applied write, against ADR-066 :408-409,
  superseded revisions never delivered; neither probed, #110 comment 6109839042) goes to #103 with the generation
  question (#15).
- *Coverage*, the `ready` field, is reported, never a gate (ADR-066 :89-92).

*A caller observes:* a worker that is behind still answers, with the state as of the reply; a caller that needs its own
write asks with its revision (#103). *Proof:* at the pin, `TestLatchBootstrap_ContinuousWrite`
(`pin:processor/graph-index/watermark_test.go:271`, re-read): the first build ends at its fixed target while writes
continue, after which `ensureQueryReady` returns at once (`query.go:217-219`);
`TestIntegration_KVRevisionAndIndexedRevisionShareOneSpace`
(`pin:processor/graph-index/revision_space_integration_test.go:38`; an integration test, not run, inventory 15) for the
revision comparison. Both return with graph-index in change 4.

## The pin's records: kept, changed or dropped

| Record, decision | Here | Where |
| --- | --- | --- |
| ADR-066 :128-134 (the "ready at ~30 s" bug); ADR-088 :36-38 | Kept as the reason a boot window exists; the reply in that window is `index_not_ready` | Context, D1 |
| ADR-066 :89-92 (`ready` is coverage); ADR-088 D4 :69-73 (caught-up is backlog), :95-98 (coverage predicate) | Kept as reported numbers in the envelope; information, never a gate | D3, D4 |
| ADR-066 :85, ADR-083 :141 (`allow_ungated_reads`) | Dropped: no consumer reads a sign before it asks, so there is no unknown case to escape | D1, D3 |
| ADR-066 :338-339 (fusion's copy of the envelope) | Not decided here; fusion's port (#135 (g)) | — |
| ADR-066 :408-409 (superseded revisions never delivered) against ADR-110 :192 | Not decided here; #103 | D4 |
| pin ADR-074 :69-71 (readiness false under a reset, for the life of the process); :78-80 (a transport failure degrades without latching) | Kept through health: a sticky reset is a named stuck state and lowers `Healthy`, and the responder answers it fatal; the envelope's `Ready` under it is information; a transport failure that persists is stuck while it lasts and still latches nothing | D1, D2, D3 |
| pin ADR-079 D3 :38-46 (poison lowers `Health()` only; the read surface refuses only poisoned entities) | Kept: the poison inventory is a named stuck state, and the refusal stays per entity | D1, D2 |
| ADR-083 D1 :70-75 (published as state, not answered as a query; a key per producer) | Changed: the state stays published, as information, one key per producer; what a consumer acts on is the reply, whose classified-transient retry contract ADR-083 left untouched (:80-81) | D1, D3 |
| ADR-088 D1 :46-54, D3 :64-67 (no aggregate, no mandatory list) | Kept as #93 built it, as information; nothing waits on a key | D3 |
| ADR-083 D2 :87-91 (freshness on arrival) | Dropped from the contract; #93's `Watcher` ships with no SemEngine reader | D3 |
| ADR-083 D3 :99-103; ADR-085 D2 :94-99 (staleness reported, never gating) | Kept | D3, D4 |
| ADR-083 D4 :105-106 (one evaluator in `graph`) | Changed: one evaluator stays, in `graph/readiness` on #93, with one SemEngine reader, graph-index in process (change 4); no SemEngine consumer across a process reads it | D3 |
| ADR-083 :124-125 (no producer timestamp against the consumer's clock) | Kept; it is why Q1 removes `published_at` | Q1 |
| ADR-083 :136-137 (one shared code path) | Dropped: there is no consumer gate path to share; the reply carries readiness | D1 |
| ADR-084 D1 :85-98 | Kept: lag never defers (:91); the health question is the responder's own check, in process | D1, D4 |
| ADR-084 D2 :100-115; ADR-085 :167-189 (`bootstrap_complete`, what it means) | Changed: the flag stays in the envelope as information; the fact it encodes, "my first build is applied", is what the responder checks before it answers (`query.go:217-219`) | D1, D3 |
| ADR-084 D3 :117-124 (no absence from a miss; read-your-writes by revision) | Kept | Context, D4 |
| ADR-084 D4 :126-134 (`index_not_ready` means not built or broken, never ordinary catch-up) | Kept as the mechanism; it also bounds #103 | D1, D2, D4 |
| ADR-084 D7 :155-163 (hard stops bind every envelope consumer) | Changed: hard stops bind through the reply and `Health()`, not the envelope; graph-index's own responder check (:159-163) is the kept pattern | D1, D2 |
| ADR-085 D1 :86-92 (four gate conditions) | Changed: the gate is not a cross-process contract; graph-index asks its four conditions in process with `Fresh: true` (`query.go:237`), where the hard stops and bootstrap-incomplete become the reply and `Health()`; no SemEngine reader asks them across a process | D1, D2, D3 |
| ADR-085 D4 :108-113 (four defer reasons) | Ship in #93's gate; graph-index's in-process reader logs them (`query.go:248-253`) | D3 |
| ADR-088 D2 :56-62 (fold through `Set` and the gate) | Dropped: nothing folds keys; a consumer asks each worker and reads its reply | D1, D3 |
| ADR-088 D5 :75-80 (rule per watcher generation) | Kept as information: the flag per generation is published, nothing waits on it; whether rule's replies carry `index_not_ready` during a generation's replay is rule's port's call under D1 | D1, D3 |
| ADR-088 D6 :82-88 (scope never gated) | Kept; with graph-ingest's latch gone, `complete` no longer says the scope was worked off | D3 |
| ADR-088 :117-119 (no readiness in `/readyz`) | Kept: `/readyz` reads boot committed and `Healthy`, never the envelope | D2 |

## Open for the owner

Ruled on #9, comment 6111608867: Q1 as recommended. `published_at` is removed in the follow-up after #93, with
graph-ingest's backlog latch and its `Health()` terms; nothing is filed until #93 lands. The question stays below as
the record of the options.

- **Q1. What happens to `published_at` on #93?** *Recommended: remove it in the follow-up after #93; leave the `Watcher`
  and the gate as they are.* `published_at` has no reader (`#93:graph/readiness/watcher.go:18`), fails "New surface
  needs a present consumer" (`AGENTS.md:96`) and invites the clock comparison ADR-083 rejected (:124-125). The `Watcher`
  and the gate have tests, one in-process reader and pin readers that port later (fusion, graph-clustering), so
  removing them now is work no requirement asks for. *Other answer (leave it):* every worker writes a field nobody reads
  on every heartbeat, and a raw reader may still compare it with its own clock. *Your cost:* you reverse the part of
  ruling F that added the field (#91 comment 6037287957); the follow-up removes it, `TestPublishStampsPublishedAt`
  (`#93:graph/readiness/publisher_test.go:206`) and the publish-time sentence of #93's `graph-transport-boundary` delta.
- Nothing else. Which stuck states a later component names (Decision 2) and which calls its startup withholds (Decision
  1) are its port's design, read against this record; whether recovery is part of a lifecycle owner's boot (ADR-111
  :129) is that port's call.

## Consequences

- Change 3's design cites this record: `/readyz` ports as at the pin, reading boot committed and `Healthy` only, with
  the three `/readyz` tests Decision 2 names. The error-count rule has no site in `service`; its adopters are the six
  components Decision 2 lists, each in its own port.
- One follow-up pull request after #93 lands, never a porting pull request (#159): graph-ingest's backlog latch leaves,
  so `bootstrap_complete` is true once `Start` returns, with the latch tests replaced by one that proves it with a
  backlog standing (Decision 3); its `Health()` drops the error-count term and gains the failed-observation state, with
  a test for each and one for a nonzero `ErrorCount` reading healthy (Decision 2); the envelope's doc comment says what
  `bootstrap_scope` means without the latch; `published_at` leaves if Q1 is ruled as recommended. Nothing is filed.
- `/readyz` goes not ready while graph-index's unresolved write failures or graph-ingest's failed backlog
  observations hold, and ready again when they clear (Decision 2's cost; today some of graph-index's failures hold it
  not ready until restart), from the ports that apply the rule.
- On adoption, semsource's readers (inventory 5.6) stop gating on `ready` and treat `index_not_ready` as a retry; its
  MCP gateway keeps reading the keys as information. semsource's own gating was left to semsource by ADR-066 :447-449.
- The archived 03B design's Readiness row
  (`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:703`, Keep | Keep) now reads: readiness
  rides the reply, and a miss is never absence (ADR-084 D3; the row's "absence only when ready" already misstated it).
  Its proof moves from SemConnect's `index-readiness` probe to graph-index's responder tests (Decision 1, change 4). The
  probe (`semconnect:conformance/cmd/index-readiness/main.go:179-182`) keeps its checks as a check of the envelope
  (Decision 3), not of the contract.
- Handed questions. ADR-108 :128-130: what "ready" computes stays as #93 built it (Decision 3); `Health()` reports
  nothing from the envelope, only the component's own stuck states (Decision 2); the envelope's fields are Decision 3
  and Q1; freshness is dropped (Decision 3); nobody aggregates (Decisions 1 and 3); the gate stays in `graph/readiness`
  with its one in-process reader, graph-index (Decision 3). ADR-108 :169, "the projection's content and `Health()`'s
  reading of it": the content stays as #93 built it (Decision 3) and `Health()` reads none of it (Decision 2).
  ADR-110 :149: a committed revision means nothing to readiness; it is the per-request comparison (Decision 4); :192
  goes to #103. ADR-111 :129: a process is ready when boot committed and every component healthy (Decision 2); whether
  a lifecycle owner's recovery is part of its boot, or runs after it answering `index_not_ready` meanwhile (Decision
  1), is that owner's port's call.
- What moves to #103: the watcher-delivery conflict (ADR-110 :192 against ADR-066 :408-409; #110 comment 6109839042),
  `min_revision` and the reply's `indexed_revision` under Decision 4's constraint (inventory C8), and a revision across
  bucket generations (#15).
- Merge order with draft PR #177: #177 is in implementation on an accepted design and merges first; its draft ADR-112
  still routes ADR-110 :192 "for ADR-109 (#110)" (`openspec/changes/natsclient-contract/adr-112.md:174` at `786c1f6`,
  re-read). The pull request that lands second changes that one line to #103: this record's (#178) if ADR-112 is in
  `docs/adr/` by then, #177 before it merges if this record lands first.

## Related

Carried in `docs/adr/`: `066-honest-graph-index-readiness.md`, `083-readiness-as-distributed-state.md`,
`084-readiness-licenses-health-not-absence.md`, `085-gate-on-health-report-freshness.md`,
`088-readiness-is-per-producer-aggregation-is-the-consumers.md` · pin ADR-074, ADR-079 (not carried) · ADR-108, ADR-110,
ADR-111, draft ADR-112 (#177) · #110, #103, #135, #101, #15
