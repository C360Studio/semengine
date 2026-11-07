# Inventory: fix-408-revlag-simplify

base: 6190ce2de0fc5b04968756cd48dd93453ea60eae (claim commit, draft PR #548, on origin/main
2e59bb08d383d4943c663fe762a37a98fdd8a80d). Library pins: `go.mod:16` — `github.com/nats-io/nats-server/v2 v2.15.2`;
`go.mod:17` — `github.com/nats-io/nats.go v1.55.0`. No library behaviour is probed for this change; every library
statement below is marked **not promised** or cited to a pin ADR that asserts it.

Structural searches: `GOFLAGS=-tags=integration gopls references` on `internal/revlag/watermark.go` `:45:6`
(Watermark), `:83:6` (New), `:101:21` (Observe), `:151:21` (Complete), `:217:21` (Observed), `:237:21` (IndexedAt);
`graph/follower/follower.go:325:20` (Follower.Complete); `graph/readiness/index_status.go:26:2` (IndexStatusInputs.IndexedAt).
String searches: `git grep -n -i -e staleness -e covered -e revlag -e IndexedAt -e watermark -e 'commit time'`
over `*.go *.md *.yaml`, archive excluded.

## 1. The claimed gaps (premises of the ruling)

- **One caller observes, on one goroutine, in delivery order.** `gopls references :101:21` → one production site:
  `graph/follower/follower.go:264` — `f.watermark.Observe(entry.Revision(), entry.Key(), entry.Created())`, inside
  `observe`, called only from `follow`'s loop: `graph/follower/follower.go:239` — `f.observe(ctx, entry, validating)`
  (`follow` at `:206-241`, run by `Run` `:192`). Every other reference is in `internal/revlag/watermark_test.go`.
- **A fresh watermark per Start.** `graph/follower/follower.go:176` — `watermark: revlag.New(),` inside `Open`;
  `processor/graph-index/component.go:236` — `f, err := follower.Open(ctx, follower.Config{` inside `start`. `follow`
  runs one watch and never re-watches (`:206-241`; a closed channel ends in `loseFeed`, `:225`). No replay is fed into
  an old watermark.
- **Completion is out of order, on lanes.** `graph/follower/follower.go:325-327` — `f.watermark.Complete(key, revision)`;
  callers `processor/graph-index/index.go:102` (OnPanic) and `:171` (processWork); lanes from
  `processor/graph-index/index.go:89-91` — `dispatch.NewKeyedPool(... Lanes: c.config.Workers,`.
- **The walk-back exists.** `internal/revlag/watermark.go:122-131` — `insertLocked` walks back from `w.order.Back()`;
  re-observe branch `:112-115` — `if e, ok := w.pending[r]; ok {`.
- **The carried-forward estimate exists.** `internal/revlag/watermark.go:58-71` (`coveredRev`, `coveredAt` doc),
  `:174-176`, `:179-206` (`advanceCoveredLocked`), `:240` — `return w.indexedLocked(), w.coveredAt`.
- **Ascending delivery is asserted, not probed here.** `docs/adr/066-honest-graph-index-readiness.md:221-224` — "delivery
  is monotonic ascending in `Revision()`" (the pin's claim). `git grep -n -i ascending -- '*.go'` → only
  `internal/revlag/*` and `graph/clustering/*`; no test drives nats.go v1.55.0 to check it. **Not promised.**
- **Cost constraint still binding.** Owner Q1, PR #407 comment 6388065578: draining stops scanning every in-flight
  revision; archived `openspec/changes/archive/2026-10-24-setup-04a-04-graph-roots-revlag/design.md:69-88` (D5 claims
  1-3). Trigger: coalesced path (`processor/graph-index/index.go:112-114`) and graph-embedding cold start.

## 2. Every current spelling of the fact (the time behind staleness_ms)

| Where | Text | Meaning today |
| --- | --- | --- |
| `internal/revlag/watermark.go:12-15` | "IndexedAt pairs the watermark with the KV COMMIT timestamp of the newest observed revision it covers" | newest covered |
| `internal/revlag/watermark.go:227-236` | "commitAt may be older than the floor's true coverage but never newer" | newest covered, estimate |
| `graph/follower/follower.go:263` | "entry.Created() is the KV commit time, which makes staleness_ms the age of the view (ADR-083 D3)" | carrier |
| `graph/follower/follower.go:402`, `:425` | `indexed, indexedAt := f.watermark.IndexedAt()` / `IndexedAt:   indexedAt,` | pass-through |
| `graph/readiness/index_status.go:23-26` | "IndexedAt is the KV commit time of the newest revision Indexed covers. ZERO means \"not computable\"" | newest covered |
| `graph/readiness/index_status.go:44-45`, `:200-204` | "StalenessMs = 0 when Ready or IndexedAt is unknown, else now-IndexedAt" | projection, unchanged |
| `graph/readiness/index_status.go:130-134` | "OldestOutstandingAt is the JetStream timestamp of the oldest outstanding message" | backlog input: oldest unapplied |
| `processor/graph-ingest/readiness.go:332-345` | "This reports the AGE OF THE VIEW instead — now minus the JetStream timestamp of the most recently APPLIED message" | graph-ingest's proxy for oldest outstanding |
| `graph/index_status.go:86-88` | "now minus the KV commit time of the newest ENTITY_STATES revision the index has fully covered" | newest covered |
| `graph/index_status.go:106-108` | "It is a FLOOR, not an oracle: a revision still undelivered server-side cannot age it." | caveat |
| `pkg/graphview/view.go:354-360` | "the KV SERVER WRITE TIME ... of the newest revision the view has applied ... The time mirrors readiness.IndexStatusInputs.IndexedAt" | graphview's own, newest applied |
| `openspec/specs/revision-watermark/spec.md:5-7`, `:88-114` | "IndexedAt pairs the floor with a covered commit time" | newest covered |
| `openspec/specs/entity-state-follower/spec.md:126` | "`ComputeIndexStatus` over the floor, its commit time and the bucket's last sequence" | carrier |
| `openspec/specs/graph-readiness/spec.md:80` | "`staleness_ms` 0 when ready or the commit time is unknown, else the age at least 1" | generic, unchanged |
| `docs/adr/083-readiness-as-distributed-state.md:100-103` | "`staleness_ms` — `0` when `Ready`, else `now − commit-time of the newest fully-covered revision`" | the decision |
| `docs/adr/085-gate-on-health-report-freshness.md:37`, `:105-110`, `:135-138` | "ADR-083 D3 ... is retained"; "Staleness is reported, never gating"; "`staleness_ms` is a *floor*" | constraints |
| `docs/adr/084-readiness-licenses-health-not-absence.md:174-178` | "`staleness_ms` is a **complete-coverage floor, not a snapshot age**" | about the floor, unchanged |
| `docs/admission-ledger.yaml:3424-3429`, `:3441-3443` | "IndexedAt pairs it with the commit time of a completed observed revision at or below it"; "the covered commit time may be older ... never newer" | row text |
| `docs/repository-map.md:98` | "`IndexedAt` returns the floor ... with the commit time of a completed revision it covers" | map text |
| `internal/revlag/pinref_test.go:1-224` | the pin's algorithm, the oracle of `TestPropWatermarkHistory` (`watermark_test.go:563-618`) | newest covered |

No producer other than the follower calls `IndexedAt` (gopls `:237:21`: `follower.go:337`, `:402`). No production code
outside `graph/readiness` reads `StalenessMs` (`git grep -n StalenessMs -- '*.go' ':!*_test.go'`: definitions and
projection only, plus the doc mention `pkg/graphview/view.go:377`).

### Tests that state the current behaviour (`internal/revlag/watermark_test.go`)

- Time assertions on the covered estimate: `:62-65` (zero while 50 in flight), `:81-86` (50's after 72 then 50 complete),
  `:134-148` (`MidBuildTakesNewestCovered`, 30's while J@40 in flight), `:207-228` (`ObserveDoesNotRegressCoveredTime`,
  10's while 50 in flight), `:398-415` (`DrainAtFloorIsCovered`, 10's while 11 in flight), `:437-440` (setup 10's while
  B@20 in flight), `:550-551` (`RetentionTracksInFlight`, zero while STUCK@1 in flight).
- Non-ascending and re-observe paths: spec scenario `revision-watermark/spec.md:30-33`; prop ops `lower`, `reobserve`
  (`watermark_test.go:658-666`).
- Revision 0 ignored: `:231-248`; spec `:72-75`.
- Private-state read: `:296-301`, `:316-321` — `len(w.pending)` (`TestWatermark_MemoryDoesNotGrowWithCompletions`).
- #408 subject: `:458-525` — `TestWatermark_ConcurrentReadersAndWriters`; the reader is unsynchronised with the writers
  (`:487-505`), `last` starts at 0 (`:490`), `reads` only reported (`:514`).
- Header claims "the pin's ... unchanged": `:14-17`, `:364-365`.
- Unaffected: no follower, graph-index or readiness test asserts a watermark time (`git grep -n StalenessMs --
  graph/follower processor/graph-index` → none; `index_status_test.go:29-69` feeds literal times).

## 3. Adjacent claims

- **Open PRs** (`gh pr list --state open --json number,title,isDraft,changedFiles,files`, 2026-11-07): #548 (this claim,
  0 files); #544 draft, 53 files, touches `pkg/graphview/view.go` (its hunk `@@ -428`, not `:354-391`),
  `docs/admission-ledger.yaml` (hunks at `:2110-2131`, `:3943+`, not `:584-597` or `:3411-3453`), `docs/repository-map.md`,
  no `internal/revlag`, `graph/follower`, `graph/readiness`, `openspec/specs/revision-watermark` or
  `entity-state-follower`. #547 is MERGED (not open).
- **Open issues** (`gh issue list --state open --search` for `revlag`, `watermark`, `staleness`, `IndexedAt`):
  #408 (retired by this change); #108 names "revlag watermark ... stays" (graph-index resume point; unaffected, stays);
  #110 names the word staleness (legacy readiness trio; names the word, not the package); #9 epic (names the word).
- **Specs:** `revision-watermark` (`:12-148`), `entity-state-follower` (`:54-57`, `:86-102`, `:124-133`),
  `graph-readiness` (`:76-81`, unchanged).
- **Ledger rows:** `docs/admission-ledger.yaml:3414-3453` (`pkg/revlag`, adapt); `:584-597` (ADR-083, "the decision text
  as at the pin"; "no edits to the pin's decision text").
- **Archived binding design:** `2026-10-24-setup-04a-04-graph-roots-revlag/design.md` D5 (`:69-88`, cost claims, Q1),
  D7 (`:105-130`), D8 (`:132-151`), D9 (`:153-170`, mutation rows), D13 (`:233-242`).
- **Line citations into ADR-083:** `docs/adr/109-ready-after-boot.md:192`, `:197`, `:199-203` cite ADR-083 `:70-75`,
  `:87-91`, `:99-103`, `:105-106`, `:124-125`, `:136-137`, `:141`. Any line inserted above `:141` makes them stale.

## 4. Who it is for

No new exported symbol, subject, bucket or config field. Changed surface: `Watermark.Observe` gains an `error` result
(internal package; one caller, `graph/follower/follower.go:264`). Removed: nothing exported. `pinref_test.go` is test code.

## 5. Problem shape

- **Refusal at a seam plus a declared signal:** closest instance `graph/follower/follower.go:287-292` (`loseFeed`, used
  at
  `:210`, `:225`, `:409`): a latched cause reported through `Stuck`, the envelope and `QueryReady`. Adoptable as is.
- **Age of the oldest unapplied work:** closest instance `graph/readiness/index_status.go:130-134`
  (`OldestOutstandingAt`), the backlog producer's input. Same quantity.

## 6. Pin decision records

This change carries no package from the pin (`internal/revlag` is already ported, ledger row `pkg/revlag`). Records that
govern the touched code: ADR-066 section 1 (floor over delivered revisions, `:190-236`); ADR-083 D3 (`:100-103`);
ADR-085 D2 (`:105-110`) and D6's floor caveat (`:135-138`); ADR-084's floor consequence (`:174-178`); ADR-108
(`:138`, one watermark per producer, no second computation).

## Collision table and intent check

Not triggered: no durable, communication or coordination primitive is proposed, and no boundary moves.

## Adopter seam inventory

Surfaces reached from outside: (a) `staleness_ms` on graph-index's `GRAPH_STATUS` envelope (readers outside the repo
are out of scope; noted only); (b) `Observe`'s contract, for the next in-repo producer to adopt `internal/revlag`
(graph-embedding, `docs/admission-ledger.yaml:3416-3418`).

1. **What must they know today?** (b) Call `Observe` in ascending order; nothing checks it
   (`revision-watermark/spec.md:79-81`). Treat a zero time as unknown. (a) The time is an estimate that may run older
   than its own definition while the producer is behind (`watermark.go:63-69`), and is zero during cold start until
   the lowest delivered revision completes (`watermark_test.go:62-65`).
2. **If they do nothing?** (b) A non-ascending `Observe` lowers the floor or pairs it with a newer time, silently
   (`docs/admission-ledger.yaml:3440-3442`). (a) They read a number whose error depends on completion order.
3. **Where do they find out?** (b) A doc and a ledger row: "doc" for a correctness fact, which is a finding. (a) Nowhere
   but the code comment.
4. **What should they have to know?** (b) Nothing: a non-ascending `Observe` should fail loudly. (a) One definition,
   stated once, exact for delivered writes.
