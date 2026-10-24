# Inventory: setup-04a-04-graph-roots-revlag

base: `6b23fdf9341f60ccf240fdaf28bef373d6346551` (origin/main). Pin paths are SemStreams
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, written `pin:<path>:<line>`, read from a local extract of the pin tarball
(`gh api repos/C360Studio/semstreams/tarball/8b99efe9`; local only).

Status: not yet reviewed. No `INVENTORY PASS`; the design was drafted in the same pass at the caller's direction.

## Admission

- Admitted in the ruled 65: `docs/tier1-cross-check.md:71` (tier 0 only). Change 4's package list:
  `openspec/changes/archive/2026-10-01-setup-04a-foundation/design.md:167` — `4. ... pkg/revlag processor/graph-index`.
  Not on the 80% coverage list: 03B D10, `openspec/changes/archive/2026-10-01-setup-03b-contract-boundary/design.md:462`
  — `` `model`, the vocabularies, `pkg/{buffer,cache,retry,dispatch,worker,resource,revlag,timestamp,... ``.
- Port order: first of nine in change 4 (#9 comment 6386856657). No ledger row today (`git grep -n revlag
  docs/admission-ledger.yaml`: one hit, `:2753`, the word "watermark" in `config`'s row, not this package).
- Size: `pin:pkg/revlag/watermark.go` 213 lines, `watermark_test.go` 329 lines, imports `sync` and `time` only. Under
  the #159 bound; no waiver needed. No lifecycle owner: one mutex-guarded struct, no goroutine, no context, no I/O.
- The gap is real: no Go file in the tree names the package (`git grep -n -i revlag -- '*.go'`: empty).

## Pin probe

go1.26.4 darwin/arm64, package only. The lifecycle suite does not apply (no owner).

- `go test -race -count=1 -p 1 -cpu 1 -cover ./pkg/revlag`: ok, 91.7%. `-count=5 -p 1 -cpu 1 -shuffle=on`: ok (seed
  1792860625707021000).
- Uncovered at the pin: `pin:pkg/revlag/watermark.go:179` — `func (w *Watermark) Observed() uint64 {` (0%; tested only
  from graph-index and graph-embedding) and `:120-122` — `if len(drained) == 0 {` (`Complete` for a key with nothing
  in flight).
- Test-text rules: no `time.Sleep`, `Skip`, build tag or `t.Parallel` in either file. gofmt and the tree's pinned
  revive (`go tool revive -config revive.toml`): no findings.
- Behaviour probe (throwaway test in a temp copy outside both repositories, removed):
  - Out-of-order observation: `Observe(10,A)`, `Complete(A,10)`, `Observe(5,B)` gives `IndexedAt() = (4, commit time
    of 10)`: a floor paired with a revision above it. The doc's promise "never newer" (`:194`) holds only under
    ascending observation, the precondition stated at `:32-35`.
  - Cost: `Complete` and the floor read scan every in-flight revision (`:115`, `:207`). N revisions in flight, then N
    completions: N=1,000 9 ms; N=10,000 914 ms; N=40,000 15.4 s (quadratic).
  - Mutants against the pin's own tests: detected: floor off by one (`:212`), completion not key-scoped (`:116`),
    exact-revision completion (`:116`), covered floor check dropped (`:154`), revision 0 accepted (`:92-94`),
    caught-up time taken from the last drained revision (`:148`), `observedHighAt` not recorded (`:99`), `IndexedAt`
    returning `observedHighAt` (`:198`). Survivors: `Observed` returning the floor (`:182`); `r <= floor` as
    `r < floor` (`:154`, the boundary); the nothing-drained return deleted (`:120-122`). Two new examples detected the
    first two (an `Observed` read; `Observe(10,A)`, `Observe(11,B)`, `Complete(A,10)` expecting `(10, commit time of
    10)`). The third is equivalent: whenever nothing is in flight, the covered revision already equals the highest
    observed one.

## Reader concurrency at the pin (who makes the in-flight set large)

- graph-index observes before dispatch: `pin:processor/graph-index/component.go:1031` —
  `c.watermark.Observe(entry.Revision(), entry.Key(), entry.Created())`. With `coalesce_ms > 0` (`:716`, opt-in;
  `pin:configs/statistical.json:729` sets 100) the next step is a non-blocking `c.entityCoalescer.Add` (`:1049`), so
  the in-flight set can grow with the replay. Without it, `submitEntityWork` (`:1128-1134`) goes to the keyed
  dispatcher, which #107 replaces with the blocking `internal/dispatch.KeyedPool`.
- graph-embedding observes at hop 1 (`pin:processor/graph-embedding/component.go:1516`) and completes at hop 2's
  terminal outcome (`pin:processor/graph-embedding/readiness.go:43`), so every entity waiting for an embedding is in
  flight (pin ADR-066 section 3). That is the reader where the scan's cost bites; it ports in change 5.

## Shape sweep (pin `pkg/revlag`)

1. A second spelling of one thing: `pin:pkg/revlag/watermark.go:164` — `func (w *Watermark) Indexed() uint64 {` and
   `:195` — `func (w *Watermark) IndexedAt() (indexed uint64, commitAt time.Time) {` read the same floor (both call
   `indexedLocked`, `:202`). One computation, two accessors.
2. Copied shell: none (no Start, Stop, Health or watch loop).
3. Layered interface: none (no interface).
4. Exported surface with no reader: `Indexed()` (`:164`). No production caller at the pin: `grep -rn --include='*.go'
   -E '\.Indexed\(\)' . | grep -v _test.go` is empty. Test readers only: 17 in its own test, 16 in graph-index tests,
   14 in graph-embedding tests. `pin:graph/index_status.go:168` names it in a comment only. The other exported
   symbols have production readers (next section).
5. Classification by error text: none (no errors).

Doc-comment findings (surface audit (c)): none describes behaviour the code lacks. `:15` — `// (a revision count is
load- and coalesce-dependent; see gh#590).` cites a SemStreams issue number, which the tree's ported ADR-083 note marks
as SemStreams' (`docs/adr/083-readiness-as-distributed-state.md:9-10`). `:32-35` state a nats.go and nats-server
ordering guarantee (OrderedConsumer delivers in ascending revision order); carried text, never probed in this tree.
Eleven doc lines name `Indexed()`: `:8`, `:24`, `:36`, `:42`, `:52`, `:74`, `:136`, `:173`, `:176`, `:185`, `:201`.

## Who reads it (#190)

- `New`: `pin:processor/graph-index/component.go:706`, `pin:processor/graph-embedding/component.go:732`.
- `Observe`: graph-index `:1031`; graph-embedding `:1516`.
- `Complete`: graph-index `:1146`, `:1364`; `pin:processor/graph-embedding/readiness.go:43`.
- `Observed`: graph-index `:1017` (bootstrap target, ADR-084 D2); graph-embedding `:1474`.
- `IndexedAt`: `pin:processor/graph-index/watermark.go:59`; `pin:processor/graph-embedding/readiness.go:126`.
- `Watermark`: unexported fields only, graph-index `:274`, graph-embedding `:337`.
- `Indexed`: tests only (sweep item 4).
- Both readers are admitted (04A D2 changes 4 and 5); neither is a consumer adapter. In this tree nothing imports the
  package.
- Consumers (`docs/inventory-scope.md` starter set): `gh search code 'revlag' --repo C360Studio/<r> --language go`
  for semsource, semconnect, semteams, semboids: 0 each. `gh search code 'pkg/revlag'` finds 3 files in semsource
  (`docs/testing/*/dependency-results.json`), transitive-closure lists, not imports.

## Placement facts (`pkg/` or `internal/`)

- Standing rule (owner, #9 comment 5953295358; change 1 D5,
  `openspec/changes/archive/2026-10-05-setup-04a-01-floor/design.md:530-534`): public if a starter consumer imports it
  or an exported signature in a public package names one of its types; every other package moves to `internal/`,
  "with no exception". Change 2 D1 restates it
  (`openspec/changes/archive/2026-10-12-setup-04a-02-ingest-kernel/design.md:93-95`).
- Measured: no consumer imports it (above); at the pin no exported signature names `revlag.Watermark` (readers hold it
  in unexported fields; `pin:graph/index_status.go:168`, `:177`, `:201` and `pin:pkg/graphview/view.go:360` are
  comments).
- Applied precedents: `pkg/dispatch` to `internal/dispatch` (ledger `:2116-2121`), `pkg/cache` to `internal/cache`
  (`:799-804`), `pkg/resource`, `pkg/timestamp`, `pkg/tlsutil`; in change 3, `component/flowgraph` to
  `internal/flowgraph`. Exception on record: `pkg/rulepack` kept `pkg/` (`:2786-2792`) with no consumer import and no
  type-naming signature; its design does not discuss placement.
- A destination move does not force `adapt`: `task ledger:check` rewrites imports to a moved destination
  (`docs/provenance.md:24-30`); precedent `carry` row `internal/semantictest` to `internal/harness/semantictest`.

## Pin ADRs and guidance (#157)

All 85 records at the pin searched for `revlag`, `IndexedAt`, `low-water`, `Observed()` and `Watermark`. Hits: ADR-066,
068, 073, 077, 081, 082, 084; ADR-083 read by title (the package comment cites it).

- ADR-066 (in tree, ported without code): section 1 (`docs/adr/066-honest-graph-index-readiness.md:176-200`) decides
  the algorithm: floor over delivered revisions only, `pending.empty() ? observedHigh : minPending - 1`, one
  key-scoped completion rule. `:274` (graph-embedding reuses `pkg/revlag.Watermark`), `:383-386` (cost),
  `:400-412` (risks, sparse-stream tests), `:425-437` (migration, extraction to `pkg/revlag`). Binding.
- ADR-083 (in tree) decision 3 (`docs/adr/083-readiness-as-distributed-state.md:99-101`): staleness in time from the
  commit time of the newest fully covered revision. Binding on `IndexedAt`.
- ADR-068 (`pin:docs/adr/068-graph-retention-deletion-lifecycle.md:177`) and ADR-073 (`pin:...073-...md:128`): each
  component holds its own watermark. ADR-077 section 6 (`pin:...077-...md:101-104`): low-water completion stays
  mandatory. ADR-084 (`:159`): read paths gate on health. These govern the readers.
- ADR-081 (`pin:...081-...md:63`, `:115`): graphview does not sequence on the watermark. ADR-082
  (`pin:...082-...md:103-107`): rejected a bounded-ready predicate in `pkg/revlag`. Neither is binding here; ADR-082
  keeps the package to the watermark only.
- In-tree ADRs that name the package: ADR-108 `:25`, `:130` (the follower and the shared projection run over the
  watermark); ADR-109 `:59`, `:180` (the first-build target is `Observed()` at enumeration end, through graph-index).
  Both cite pin paths, which the ledger's `source_path` to `destination` maps.
- Context ownership: none in the package; ADR-113 does not arise.
- Guidance: the pin's watermark lines (`pin:.agents/contracts/semstreams-developer.md:123`, `semstreams-reviewer.md:166`)
  already have tree equivalents (`.agents/contracts/semengine-developer.md:152-153`, `semengine-reviewer.md:222`). No pin
  skill names the package.

## Open issues (#160)

- `gh issue list --state open --search revlag --limit 200`: #108, #9. `watermark`: #108, #9. `low-water`, `IndexedAt`:
  none. `IndexedRevision`: #103, #110, which name the envelope field, not this package.
- #108 names the package in its body ("The readiness core itself (revlag watermark, failed-count withholds Ready, every
  handler gated) is right and stays"). Its work is graph-index's (ruling, #9 comment 6386556850, item 2).
- #9 is the epic.

## Overlapping PRs

`gh pr list --state open --json number,title,changedFiles,files`:

- #401 (draft, 21 files): edits `docs/admission-ledger.yaml` inside the `service` row (hunks at `:2962`, `:2993`). This
  change appends a row at the end of the file. No shared lines.
- #403 (draft, 2 files): `scripts/port-status.sh`, `internal/harness/contract/portstatus_test.go`. This change does not
  touch either (below).
- #404 (draft, 6 files): its own `openspec/changes/fix-390-391-port-pick-family/` with `harness-boundaries` and
  `nats-fixture` deltas. No file or capability overlap.

## Current spellings of the fact (category 2) and problem shape (category 5)

The fact: the highest revision below which every delivered revision has completed.

- `graph/index_status.go:61-65` — `IndexedRevision uint64 ...`: the envelope field that carries the value; no
  computation.
- `natsclient/kv.go:100-112` — `func BucketLastSeq(...)`: the target, not the floor.
- `config/manager.go:80-88` — `appliedRev map[string]uint64`: per-key last applied revision used to skip stale events;
  a different fact.
- `graph/readiness/index_status.go:73`: graph-ingest's backlog projection, explicitly not a revision watermark.
- Shape: a low-water mark over asynchronous completions. No other instance in the tree (`git grep -n -i -E
  'minPending|observedHigh|low-water|lowWater' -- '*.go'`: empty). A ported primitive, not a new one; its readers
  are already named, so no adoption sweep.

| Dimension | Evidence |
| --- | --- |
| Semantic class | "caught up to revision R" for one in-process KV-watch pipeline |
| Owners | `pkg/revlag` only (pin); none in tree |
| Catalogs | none (in-memory) |
| Status | feeds `graph.ComputeIndexStatus` at the pin (`pin:graph/index_status.go:201`); not yet in tree |
| Lifecycle | built per `Start` by its reader (`pin:...graph-index/component.go:706`); no stop |
| Ownership | one per reader instance (ADR-068 `:177`) |
| Readers | graph-index, graph-embedding (above) |
| Writers | the same readers' watch goroutine and workers |
| Recovery | rebuilt from replay at each start (ADR-066 `:414-416`) |

## Adopter seam

Not reached from outside the module once under `internal/`. For the in-tree reader (graph-index's port), three facts:
observe every delivered entry in delivery order before dispatch; complete on every terminal path with the entry's key
and revision; read a zero commit time as "staleness unknown". graph-index's design carries them.

## Intent check

No capability boundary moves: the package is admitted and stays admitted. Only its path moves, under the standing rule.

## Coverage, status and docs

- `scripts/cover-check.sh:23-49`: no `revlag` target; 03B D10 rules it out of the 80% gate.
- `scripts/port-status.sh:94-97` lists `pkg/revlag` under change 4 and keys on the ledger's `source_path` (`:130-152`):
  the new row counts it ported with no script change.
- `docs/repository-map.md:12-19` still says change 3 is in progress with `service` PRs 5 to 7 remaining. All seven
  `service` PRs are merged (`b3a4fa9`, PR #393), and change 3's packages are in the tree.
- `docs/adr/066-honest-graph-index-readiness.md:15-18`, the port note, lists `pkg/revlag` among code not yet ported.
