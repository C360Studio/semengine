# ADR-109: Ready After Boot, One Startup Flag per Worker

## Status

**Proposed (2026-10-11).** Drafted on draft PR #178 (`Addresses #110`), ruled on #9 with the other records change 3's
design cites (#9 comment 6085243841). Scope: the owner's direction on #110 (comment 6109914179), "ready-after-boot only:
one 'finished startup' flag per worker, broken states go to health, catch-up and my-write-landed stay per request". #110
stays open as the refactor.

Evidence is the architect's inventory (sha256 `dd11d813…afcaf`). `#93:path:line` is PR #93 at `f3d5ae5`, `pin:path:line`
is SemStreams at `8b99efe9`, and `ADR-0xx :line` is a line of a record carried in `docs/adr/` (files under Related).

## Context

- A *worker* keeps derived state that other components read: graph-index, graph-embedding, graph-ingest and rule at the
  pin, and spatial and temporal when ported (ADR-108 ruling 2). A *consumer* reads from a worker.
- The *startup rebuild* is the replay a worker runs after its process starts, up to a target fixed when the replay
  began, not the moving head of the stream (ADR-085 :173-176).
- The *envelope* is the JSON status record (`graph.IndexStatusResponse`) each worker writes under its own *key* in the
  `GRAPH_STATUS` bucket of the NATS key-value store. Its field `bootstrap_complete` is the *flag*.
- *Health* is a worker's report that it works: `Health()` returns `Healthy`, and the process's `/readyz` reads it
  (`pin:service/service_manager.go:1848`, `:1854`). On graph-index's read path, the classified `index_not_ready` reply
  also means a health failure, never lag (ADR-084 D4, :126-134).

**Why a boot window exists.** "Started" is not "rebuilt". graph-index once reported `phase: ready` at about 30 s while
lookups of symbols that did exist kept failing for minutes, so a consumer that waited for "ready" got false negatives
(ADR-066 :128-134). For the rule processor, "`Start` returning is not that signal" (ADR-088 :36-38).

**What this record leaves out.** The pin also built freshness judged on the consumer's clock, a four-condition gate, a
helper that folds several keys, and the coverage field `ready`. The owner's challenge: a streaming knowledge graph is
never done catching up, how far behind is acceptable depends on the use, and a consumer that asks a question should get
the current state with the answer.

## Decision

### Decision 1. Readiness is one flag per worker

The flag means "I have finished my startup rebuild in this process": the envelope's `bootstrap_complete` (ADR-084 D2,
ADR-085 :167-189). It is false when the process starts and becomes true when the rebuild is done, or at once when there
was nothing to rebuild. It goes false again only when a worker starts a new rebuild in the same process: rule for each
watcher generation (ADR-088 D5), graph-embedding when its watcher starts
(`pin:processor/graph-embedding/component.go:1225`). Lag never makes it false, and neither does a reset: a reset is a
broken state, reported through health (Decision 3), and the pin keeps the flag latched under it
(`pin:processor/graph-index/watermark.go:26-32`).

A consumer may rely on exactly that and nothing more. A true flag does not mean caught up, and a lookup that returns
nothing is still not proof of absence (ADR-084 D3).

graph-ingest on #93 does not yet fit the fixed-target definition. It sets its flag the first time its whole backlog, old
and new messages together, reads zero (`#93:processor/graph-ingest/readiness.go:234`, set at `:267-268`). That is the
moving head ADR-085 warns "would never flip under a firehose" (:188), so under steady traffic a consumer that names
graph-ingest can wait indefinitely (traced, not run). Q3 asks what to do.

*A caller observes:* a worker's key shows `bootstrap_complete: false` until its rebuild ends, then `true`, and new load
does not change it. *Proof:* `TestLatchBootstrap_DrainedIsALatchNotALiveRead`
(`#93:processor/graph-ingest/readiness_test.go:83`) and `TestLatchBootstrap_ObservationFailureNeverLatches` (`:61`);
each later worker's port proves the same for its rebuild.

### Decision 2. Each worker publishes its own flag

Each worker writes its own key; there is no aggregate key and no framework list of required workers (ADR-083 D1, ADR-088
D1 and D3). A consumer that depends on several workers names their keys and, at its own startup, waits until each named
flag is true. A worker it does not need is one it does not name (ADR-088 :59); a named key nobody writes keeps it
waiting, so a missing worker fails closed. It may read the flag with a key-value get, a watch or #93's
`readiness.Watcher`; no helper is promised (`readiness.Set` stays dropped, #91 comment 6085720598). Two instances of one
worker type in one deployment would share a key; that is not measured and not promised.

*A caller observes:* `nats kv get GRAPH_STATUS <key>` returns that worker's envelope with its flag. *Proof:*
`TestIntegration_ReadinessEnvelope_PublishedAndCatchesUp`
(`#93:processor/graph-ingest/readiness_integration_test.go:122`).

### Decision 3. Broken states are health, not readiness

While a worker's state is `degraded` or `reset_required`, its `Health()` reports `Healthy: false`. The flag does not
change. Facts only the worker holds, such as graph-ingest's poison inventory, keep lowering `Healthy` (pin ADR-079 D3,
pin `docs/adr/079-per-entity-poison-response-authoritative-surface.md:38-46`).

This changes the pin. On #93 graph-ingest's `Healthy` ignores its envelope
(`#93:processor/graph-ingest/component.go:831`), which degrades when a backlog read fails
(`#93:processor/graph-ingest/readiness.go:137-146`); the pin's graph-index is the same
(`pin:processor/graph-index/component.go:527`). Only the pin's rule processor keeps the two in line
(`pin:processor/rule/processor.go:499-515`).

The alignment work rests on this record once ruled (Q2): each worker's own port makes `Health()` follow its broken
states, with a test. graph-ingest's alignment follows #93 in its own small pull request and never rides a porting pull
request (one package per pull request, #159). `/readyz` stays as at the pin, reading `Healthy` and not the flag (03B D15
ruling, #8 comment 5931143569; ADR-088 :117-119). The cost: `/readyz` can go not ready during a short-lived degrade.

*A caller observes:* whenever a worker's key shows `degraded` or `reset_required`, its `Health()` reports
`Healthy: false`. *Proof:* no test on #93 covers it; graph-ingest's alignment pull request adds the first, each worker's
port its own.

### Decision 4. Catch-up, read-your-writes and coverage are not readiness

None of these is part of the flag, and none makes a consumer wait. A consumer that needs one asks *per request* (it asks
with its request and gets the current answer with the reply) or reads the reported number when it needs it.

- *Catch-up*, how far behind a worker is (`lag`, `staleness_ms`), is reported, never a gate (ADR-085 D2; ADR-084 D1,
  "lag alone never defers a consumer that declared no bound", :90-91).
- *Read-your-writes* compares the `kv_revision` a write returned with the worker's `indexed_revision` (ADR-084 D3). It
  stays #103's per-request option, within ADR-084 D4. What a watcher of `ENTITY_STATES` may rely on about revisions
  (ADR-110 :192 against ADR-066 :408-409, raised by draft PR #177) goes to #103 too.
- *Coverage*, the `ready` field, is reported, never a gate.

*A caller observes:* new load leaves a worker's flag true, so nothing in readiness makes a worker that is behind look
not ready. *Proof:* `TestLatchBootstrap_DrainedIsALatchNotALiveRead`
(`#93:processor/graph-ingest/readiness_test.go:83`). That a worker that is behind still answers its queries is proved by
graph-index's responder test when graph-index ports in change 4.

### Decision 5. Whether a flag is still current is health's job

Readiness has no freshness rule and compares no timestamps. If a worker dies after setting its flag, the bucket still
says true; finding a dead or broken worker is health's job. This removes the design review's R1 (the clock comparison)
and R2 (an unprobed NATS timestamp). The cost: a consumer that starts while a worker is dead reads its last flag, which
may say true (inventory §5.6 traces this for semsource); how it then learns the worker is gone is not promised here.

*A caller observes:* nothing in readiness changes with time. *Proof:* none is needed, since the flag has no time input.

### Decision 6. Code on #93 that does more ships as it is

PR #93 is frozen and ships as it is, including `published_at` (ruling F, #91 comment 6037287957), the Watcher's
freshness logic (`#93:graph/readiness/watcher.go:124-149`) and the gate's freshness, state and hard-stop checks
(`#93:graph/readiness/readiness_gate.go:103`). None of them is part of the contract; SemSource and SemConnect rely on
the flag only. Removing them is optional under `.agents/protocol.md` § Bounded port scope unless the owner rules
otherwise (Q1). With the contract one field, the gate's home (inventory C2) is no longer a readiness question; it stays
in `graph/readiness`.

*A caller observes:* #93's envelope, Watcher and gate behave as #93's tests pin them. *Proof:* those tests, unchanged.

## The pin's records: kept, changed or dropped

| Record, decision | Here | Where |
| --- | --- | --- |
| ADR-066 :128-134 (the "ready at ~30 s" bug); ADR-088 :36-38 | Kept as the reason a boot window exists | Context |
| ADR-066 :89-92 (`ready` is coverage); ADR-088 D4 :69-73 (caught-up is backlog), :95-98 (coverage predicate) | Kept as reported numbers; not readiness | D4 |
| ADR-066 :85, ADR-083 :141 (`allow_ungated_reads`) | Dropped: with no freshness there is no unknown case to escape; a consumer that does not need a worker does not name it | D2, D5 |
| ADR-066 :338-339 (fusion's copy of the envelope) | Not decided here; fusion's port (#135 (g)) | — |
| pin ADR-074 :67-74 (readiness false under a reset, for the life of the process); :78-80 (a transport failure degrades without latching) | Kept: that false is the pin's `Ready`, coverage, while the flag stays latched (`pin:processor/graph-index/watermark.go:26-32`); the reset and the transport failure go to health, still without latching | D3 |
| pin ADR-079 D3 :38-46 (poison lowers `Health()` only) | Kept | D3 |
| ADR-083 D1 :70-75; ADR-088 D1 :46-54, D3 :64-67 (a key per producer, no aggregate, no mandatory list) | Kept | D2 |
| ADR-083 D2 :87-91 (freshness on arrival) | Dropped from the contract; #93's code ships | D5, D6 |
| ADR-083 D3 :99-103; ADR-085 D2 :94-99 (staleness reported, never gating) | Kept | D4 |
| ADR-083 D4 :105-106 (one evaluator in `graph`) | Dropped as moot | D6 |
| ADR-083 :124-125 (no producer timestamp against the consumer's clock) | Kept | D5 |
| ADR-083 :136-137 (one shared code path) | Changed: any read of the one field will do | D2 |
| ADR-084 D1 :85-98 | Kept: lag never defers (its freshness parameter was already superseded, :21) | D4 |
| ADR-084 D2 :100-115; ADR-085 :167-189 (`bootstrap_complete`, what it means) | Kept as the flag; the only narrowing of "latched, never cleared" (:173) is ADR-088 D5's, a new rebuild in the same process | D1 |
| ADR-084 D3 :117-124 (no absence from a miss; read-your-writes by revision) | Kept | D1, D4 |
| ADR-084 D4 :126-134 (`index_not_ready` means a health failure) | Kept | Context |
| ADR-084 D7 :155-163 (hard stops bind every envelope consumer) | Changed: they bind through health, not the flag; graph-index's own responder check (:159-163) is kept | D3 |
| ADR-085 D1 :86-92 (four gate conditions) | Changed: readiness is the fourth, the flag; freshness goes to D5, state and hard stops to health | D1, D3, D5 |
| ADR-085 D4 :108-113 (four defer reasons) | Ship in #93's gate, outside the contract | D6 |
| ADR-088 D2 :56-62 (fold through `Set` and the gate) | Changed: the consumer waits on each flag it names, with no helper | D2 |
| ADR-088 D5 :75-80, D6 :82-88 (rule per watcher generation; scope never gated) | Kept | D1 |
| ADR-088 :117-119 (no readiness in `/readyz`) | Kept | D3 |

## Open for the owner

- **Q1. What happens to `published_at` and the freshness code on #93?** *Recommended: remove `published_at` in one small
  pull request after #93; leave the Watcher and the gate as they are.* `published_at` has no reader
  (`#93:graph/readiness/watcher.go:18`), fails "New surface needs a present consumer" (`AGENTS.md:96`) and invites the
  clock comparison ADR-083 rejected. The Watcher and the gate have tests and pin readers that port later (fusion,
  graph-clustering), so removing them now is work no requirement asks for. *Other answer (leave all three):* every
  worker writes a field nobody reads on every heartbeat, and a raw reader may still compare it with its own clock. *Your
  cost:* you reverse the part of ruling F that added the field; one small pull request removes it,
  `TestPublishStampsPublishedAt` (`#93:graph/readiness/publisher_test.go:206`) and the publish-time sentence of #93's
  `graph-transport-boundary` delta.
- **Q2. Do you accept the health-alignment work in Decision 3?** *Recommended: yes.* It is what "broken states go to
  health" means in code; today a degraded graph-ingest on #93 and a degraded graph-index at the pin both report healthy,
  and the pin's rule processor already does it. *Other answer (no):* broken states stay only in the envelope, which no
  consumer is asked to read for them, so nothing a consumer relies on reports them, or readiness grows the hard-stop
  check back. *Your cost:* one small pull request for graph-ingest after #93, a test in each later worker's port, and a
  `/readyz` that goes not ready during a short degrade. ADR-088 rejected a `/readyz` that flaps "under ordinary write
  load" (:117-119); a degrade is not ordinary load, but an operator will see it.
- **Q3. When does graph-ingest say "finished starting"?** Today it says so only when it has no backlog at all, counting
  messages that arrived after it started; under steady traffic that can never happen, and anything waiting on it waits
  with it (Decision 1). *Recommended: have graph-ingest's alignment pull request (Q2) latch the flag when the work
  outstanding at bind is done.* That is the fixed target Decision 1 defines and ADR-085 requires (:173-176), and #93
  already records the backlog at bind (`#93:processor/graph-ingest/readiness.go:178`). *Other answer (keep #93's latch
  and declare the wait):* no work, but a consumer that names graph-ingest may never start under load, which is the
  "never caught up" problem you raised, and each such consumer has to say so. *Your cost:* the alignment pull request
  grows. It needs a sound way to tell work that was waiting at bind from work that arrived later, and the obvious
  shortcut, the consumer's acknowledgement floor, was measured wrong at the pin (ADR-088 :106-114); it adds a test that
  the flag sets under steady traffic.

## Consequences

- Change 3's design cites this record: `/readyz` ports as at the pin, reading `Healthy` only.
- On adoption, SemSource reads `bootstrap_complete` for readiness instead of `ready`, its coverage gate today (inventory
  §5.6).
- The archived 03B design's Readiness row
  (`openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:703`, Keep | Keep) now reads: readiness is
  the startup flag, and a miss is never absence (ADR-084 D3; the row's "absence only when ready" already misstated it).
  Its proof, SemConnect's `index-readiness` probe, keeps its checks
  (`semconnect:conformance/cmd/index-readiness/main.go:179-182`): `BootstrapComplete` is the readiness check; `Ready`
  and the revision checks test catch-up, which Decision 4 moves out of readiness; the gate call checks state, hard
  stops and catch-up together.
- Handed questions: ADR-108 :128-130 is answered by Decisions 1 to 3, 5 and 6; ADR-110 :149 and :192 by Decision 4;
  ADR-111 :129 by Decision 1 (whether recovery is part of a worker's startup is that worker's port's call).
- Work required only if ruled as recommended: the alignment (Q2) with graph-ingest's fixed-target latch (Q3), and
  removing `published_at` (Q1). Nothing is filed.

## Related

Carried in `docs/adr/`: `066-honest-graph-index-readiness.md`, `083-readiness-as-distributed-state.md`,
`084-readiness-licenses-health-not-absence.md`, `085-gate-on-health-report-freshness.md`,
`088-readiness-is-per-producer-aggregation-is-the-consumers.md` · pin ADR-074, ADR-079 (not carried) · ADR-108, ADR-110,
ADR-111, draft ADR-112 (#177) · #110, #103, #135, #101
