# Design: setup-04a-04-graph-roots-graph-ingest-split

Base `c1caa45` (`c1caa459f3bd403140c1678c5593672abb2b41ee`). Inventory: `inventory.md` (not yet reviewed). Brief: D8 of
PR #409's design, `openspec/changes/archive/2026-10-26-setup-04a-04-graph-roots-processor-shell/design.md:188-197`, and
the owner's ruling on #300 (comment 6416204673): six destinations, "moves only, no edits". Addresses #300 and #109.

## Options

- **A. Do nothing.** `component.go` stays 2,558 lines; #300 G stays open against an owner ruling that schedules it.
- **B. D8's map** (recommended). Six destinations, the names the owner's ruling repeats. `ingest.go` is 896 lines.
- **C. D8 plus a seventh file** for the write paths ("Entity Operations" and "Triple Operations", about 515 lines),
  leaving `ingest.go` at about 400. Every file under 700, but a file the ruling does not name, for a size target the
  ruling does not set. The decode, ingest and write paths are one flow, `decodeEntity` to `addTriplesLane`.
- **D. #139's map** (closed issue): the consume closure into `keyed_ingest.go` (496 lines today) and the write paths
  into `entity_writes.go` (502). Each would pass 700 and mix two files' subjects; D8 replaced it.

B keeps the owner's names; C is a one-line change to D1 if the reviewer or owner prefers it at acceptance.

## D1. File map

Line ranges are at the base and include each declaration's doc comment. "After" is `wc -l` on a dry run of the move
on a copy of the base (`git archive c1caa45`), with imports trimmed as below.

| Destination | Declarations (base lines) | After |
| --- | --- | --- |
| `metrics.go` (new) | fourteen constructors `newEntitiesUpdatedMetric` to `newStaleSetsMetric`, `type dedupLane`, `const dedupLaneAddBatch`, `newDuplicateTriplesSuppressedMetric` (`:46-275`); `const metricService`, `registerMetrics` (`:694-791`); `newBatchMissingMetric` (`:2532-2558`) | 366 |
| `config.go` (new) | `Config`, `Validate`, `ApplyDefaults`, `DefaultConfig`, `const (defaultIngestLanes, ingestLaneQueueDepth, ingestGuardMemMaxPerLane)`, `var schema` (`:277-384`) | 120 |
| `poison_inventory.go` (appended after `DebugStatus`) | `startEntityStateGuard`, `stopEntityStateGuardWatcher`, `sweepEntityStateGuardEntry`, `recordBootSweepPoison`, `classifyStoredStateRMWError` (`:1126-1285`) | 290 → 454 |
| `consume.go` (new) | banner "Subscription Management", `setupSubscriptions`, `const defaultIngestAckWait`, `ingestDeliveryTiming`, `setupJetStreamConsumer`, `waitForStream` (`:1306-1576`) | 286 |
| `query.go` (appended after `decodeCursor`) | `invalidateEntityCacheEntry`, `entityCacheHandle`, `loadEntityCacheGen`, `repopulateEntityCacheEntry` (`:2245-2319`) | 653 → 730 |
| `ingest.go` (new) | `decodeEntity` to `indexingProfileMetricLabel` (`:1578-1957`, fifteen); banner "Entity Operations", `validateEntityID`, `CreateEntity`, `mergeEntityOnLane`, `addToContainers`, `createEntityWithReceipt`, `deleteEntityAtRevision` (`:1958-2239`); banner "Triple Operations" (`:2240-2244`); `recordSuppressedDuplicates`, `addTriplesResult`, `addTriplesLane` (`:2321-2530`) | 896 |
| `component.go` (stays) | D2 | 699 |

Where this differs from D8's text, and why:

- `consume.go` also takes `setupSubscriptions`, `defaultIngestAckWait` and `waitForStream`, which D8's range skipped.
  The banner heads all five as one section; `setupSubscriptions` is `setupJetStreamConsumer`'s only caller
  (`component.go:1322`), `defaultIngestAckWait` is read only by `ingestDeliveryTiming` (`:1344`) and `waitForStream`
  only by `setupJetStreamConsumer` (`:1370`). Left behind, they would keep one call chain in two files.
- `entityCacheHandle` came with PR #409, after D8; it sits inside the cache-coherence block and moves with it.
- `dedupLane` and `dedupLaneAddBatch` lie inside D8's metric range and move with it: they are the label of
  `newDuplicateTriplesSuppressedMetric`.
- The "Triple Operations" banner stays above the triple writers in `ingest.go`; D8's cache range began after it.

Each block keeps its order. A banner moves with the section it heads, its text unchanged. Each new file holds
`package graphingest`, one import block and the moved blocks, separated by one blank line; no doc comment. Imports:
start from `component.go`'s import block and delete what the file does not use, by hand or with `goimports -w`, which
the repository does not pin (`task fmt` is `gofmt -w .`, `Taskfile.yml:19-23`). `goimports` cannot add the aliased
`semtypes` import on its own, so removing from the full block is the way that works. Groups stay as the repository
writes them: standard library, a blank line, the rest. In `query.go` the move adds `internal/cache`; in
`poison_inventory.go`, `errors`, `pkg/errs` and `nats.go/jetstream`.

## D2. What stays in `component.go`

The package doc comment, imports and interface assertions (`:1-44`); `hierarchyStore` (`:386-395`, the adapter
`initHierarchyInference` builds); `Component`, its two consumer types, `DeclarePorts`, `resolveConfig`,
`CreateGraphIngest` (`:397-692`); `Register`, the four Discoverable methods, the lifecycle methods with `start`,
`release`, `cleanup`, `clearLifecycleHandles` and `initStorage` (`:793-1124`, both banners); `initHierarchyInference`
(`:1287-1305`). That is what builds a graph-ingest, starts it and stops it: 699 lines. Moving the lifecycle too would
leave a file of a struct and a factory, and the lifecycle is what the next change-4 pull requests read beside
`component.Lifecycle`.

## D3. Proof of "moves only"

Two checks, run from the repository root with the base and the move commit. Neither is committed as a script: it is
a one-time proof for this pull request, and harness code exists only for a failure on today's tree
(`docs/test-infrastructure.md`). Task 2.1 records the run in `notes/2.1.md`.

```bash
base=c1caa45 move=HEAD d=processor/graph-ingest
strip() { awk '/^import \($/{skip=1;next} skip{if(/^\)$/)skip=0;next} /^package /{next} NF' | LC_ALL=C sort; }
# P1: every non-blank line outside package clauses and import blocks, as a sorted list.
diff <(git show "$base:$d/component.go" "$base:$d/poison_inventory.go" "$base:$d/query.go" | strip) \
     <(for f in component config consume ingest metrics poison_inventory query; do git show "$move:$d/$f.go"; done | strip)
# P2: go doc for the package, then the source of each function, method and type component.go declared at the base.
syms=$(git show "$base:$d/component.go" |
  grep -oE '^(func( \([a-z]+ \*?[A-Za-z]+\))? [A-Za-z_]+|type [A-Za-z_]+)' |
  sed -E 's/^func \([a-z]+ \*?([A-Za-z]+)\) /\1./; s/^(func|type) //')
p2() { local t; t=$(mktemp -d); git archive "$1" | tar -x -C "$t"
  (cd "$t/$d" && go doc -all -u . && for s in $syms; do echo "=== $s"; go doc -u -src "$s"; done); rm -rf "$t"; }
diff <(p2 "$base") <(p2 "$move")
```

Expected: both `diff`s print nothing and exit 0. P1 catches any added, removed or changed line, comments and banners
included. P2 lists the same 83 declarations with their source in a fixed order, so it also catches lines reordered
within a declaration, which P1 cannot see; its `go doc -all -u` part shows the package's symbols are the same. Both
ran on the dry run and printed nothing. Shown able to fail on the dry run: one banner word changed
(`Subscription Management` to `Subscription management` in `consume.go`) fails P1; two lines of `waitForStream`
swapped pass P1 and fail P2.

Review aids, not proof: `git diff -M -C --stat c1caa45 <move>` (dry run: `component.go` 1,859 deletions; `config.go`
120, `consume.go` 286, `ingest.go` 896, `metrics.go` 366 insertions; `poison_inventory.go` 164 and `query.go` 77
insertions; the `natsclient` test); `git blame -C -C` on each new file, which on the dry run attributes every line to
the base except package clauses, import lines and blank lines. `git log --follow` does not trace a split, since
`component.go` stays (inventory, first list). The checks read only the seven production files, so they pass on the
squash commit on `main` as on the branch.

## D4. Citations and issue comments

In the move commit, because the move fails the test otherwise (inventory section 3):
`natsclient/consumer_policy_callsite_test.go:323` and `:331` name `processor/graph-ingest/consume.go`, and `:317`
reads `// GetConsumerConfig reader, both in consume.go (at the pin both are in graph-ingest's component.go).`

In the docs commit: `processor/graph-ingest/hierarchy_replay_integration_test.go:7` says `ingest.go`;
`processor/graph-ingest/readiness_integration_test.go:204` says `consume.go`; `graph/readiness/gauges_test.go:44` says
`metrics.go`. No current doc (`docs/*.md`, `.agents/`, `openspec/specs/`, the package `README.md`) cites the file by
line (inventory section 4), so none changes. ADRs, archived changes and issue comments are records of their date and
are not edited.

Comments after the move commit, each naming the commit, "pending merge", and the new `file:line` by function: #300
(G and D's seams done; the rest of D, and C, F and H, stay open); #109 (the seams routed there are done); #111 (the
hierarchy birth gate, now in `ingest.go`'s `mergeEntityOnLane` and `createEntityWithReceipt`); #134 (the two
`hierarchyInference != nil` checks in `ingest.go`; `initHierarchyInference` and `hierarchyStore` stay); #301 (readers
of the four fields: `startEntityStateGuard` in `poison_inventory.go`, `setupJetStreamConsumer` in `consume.go`,
`subscribeForRequests` stays); #106 (`query.go`'s lines move down by one). No comment where the cited code stays in
`component.go` (#9, #33, #110, #196, #309) or the cited file does not change (inventory section 6).

## D5. Ledger

`docs/admission-ledger.yaml:2402-2404`, the `component.go` row's destination, becomes "processor/graph-ingest/ingest.go
(decodeEntity) and processor/graph-ingest/consume.go (consume closure), with
processor/graph-ingest/fact_lane_fence_test.go and fact_lane_fence_integration_test.go". Its `source_path` stays the
pin's file name. The disposition stays `adapt`; `ledger:check` compares `carry` rows only
(`internal/harness/pindiff/main.go:85-86`), so no pin comparison is involved. The `processor/graph-ingest` row
(`:2337`) names no file and does not change.

## D6. Tests

No new test. The evidence is D3 and `task verify` on the move commit and at the head, integration tests included. No
test cites a line of `component.go` (inventory section 4). On the dry run, `go test ./...` failed only the census
test until its names changed (inventory section 3); `go vet`, `gofmt -l` and the pinned `revive` were clean. Examples
suffice, with no generated check: D3 compares every line and every declaration, so there is no input left to vary.

## D7. ADRs

None changed. ADR-108 and ADR-113 govern the lifecycle that stays in `component.go`; ADR-110's write seam is
`entity_writes.go`, untouched. Their text is not edited (D4).

## D8. Unit of work and merge order

One pull request. Commits: the move with the census update (task 2.1), then citations and ledger (task 3.1), then the
OpenSpec records. About 1,860 lines move; the #159 bound counts pin lines ported, and this ports none, so no waiver
is needed. No open pull request overlaps (`gh pr list`: empty). Merge before the next change-4 pull request that
edits `processor/graph-ingest`, so it starts from the split files.

## Owner question

None. Option C is open to the reviewer or owner at acceptance; the design recommends B.
