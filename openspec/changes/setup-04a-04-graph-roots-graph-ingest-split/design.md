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

One script, run under bash from the repository root as `bash proof.sh c1caa45 <move commit>`. Its text goes in
`notes/2.1.md` with the run; it is not committed as a harness script, since it proves one pull request and harness
code exists only for a failure on today's tree (`docs/test-infrastructure.md`). It stops at the first failure
(`set -euo pipefail`) and prints its last line only when every check passes.

```bash
#!/usr/bin/env bash
# Moves-only proof for the graph-ingest split. Run from the repository root: bash proof.sh <base> <move>.
set -euo pipefail
base=$1 move=$2 d=processor/graph-ingest
t=$(mktemp -d); trap 'rm -rf "$t"' EXIT
files="component config consume ingest metrics poison_inventory query"
ranges() { case $1 in              # D1's base line ranges of component.go, in order
  component) echo 1,44 386,395 397,692 793,1124 1287,1305 ;;  metrics) echo 46,275 694,791 2532,2558 ;;
  config) echo 277,384 ;;  poison_inventory) echo 1126,1285 ;;  consume) echo 1306,1576 ;;
  query) echo 2245,2319 ;;  ingest) echo 1578,2244 2321,2530 ;; esac; }
body() { awk '/^import \($/{skip=1;next} skip{if(/^\)$/)skip=0;next} /^package /{next} NF'; }
imports() { awk '/^import \($/{skip=1;next} skip&&/^\)$/{exit} skip&&NF{sub(/^[ \t]+/,"");print}'; }
git show "$base:$d/component.go" > "$t/base.go"
# A: the move commit changes exactly these paths.
git diff --name-only "$base" "$move" > "$t/paths"
printf '%s\n' natsclient/consumer_policy_callsite_test.go $(for f in $files; do echo "$d/$f.go"; done) |
  LC_ALL=C sort | diff - "$t/paths"
# B: the ranges cover every non-blank base line once; each file is its base text, then its ranges, in order.
for f in $files; do for r in $(ranges "$f"); do seq "${r%,*}" "${r#*,}"; done; done | LC_ALL=C sort > "$t/used"
uniq -d "$t/used" | { ! grep .; }
awk 'NF{print NR}' "$t/base.go" | LC_ALL=C sort | comm -23 - "$t/used" | { ! grep .; }
for f in $files; do
  { [ "$f" = component ] || git show "$base:$d/$f.go" 2>/dev/null || true
    for r in $(ranges "$f"); do sed -n "${r}p" "$t/base.go"; done; } | body > "$t/want"
  git show "$move:$d/$f.go" | body | diff "$t/want" -
done
# C: every import of the seven files was already in base component.go or in that file at the base.
for f in $files; do
  { imports < "$t/base.go"; git show "$base:$d/$f.go" 2>/dev/null | imports || true; } | LC_ALL=C sort -u > "$t/allowed"
  git show "$move:$d/$f.go" | imports | LC_ALL=C sort -u | comm -23 - "$t/allowed" | { ! grep .; }
done
# D: go doc for the package, then each function, method and type base component.go declares; stops on any failure.
syms=()
while read -r s; do syms+=("$s"); done < <(grep -oE '^(func( \([a-z]+ \*?[A-Za-z]+\))? [A-Za-z_]+|type [A-Za-z_]+)' "$t/base.go" |
  sed -E 's/^func \([a-z]+ \*?([A-Za-z]+)\) /\1./; s/^(func|type) //')
[ "${#syms[@]}" -eq 83 ]
srcdoc() { mkdir "$t/tree-$1"; git archive "$2" | tar -x -C "$t/tree-$1"; cd "$t/tree-$1/$d"; go doc -all -u .
  for s in "${syms[@]}"; do echo "=== $s"; go doc -u -src "$s"; done; cd - > /dev/null; }
srcdoc before "$base" > "$t/before"; srcdoc after "$move" > "$t/after"
[ "$(grep -c '^=== ' "$t/after")" -eq 83 ]
awk '/^=== /{if(h&&!n)bad=1;h=1;n=0;next} /^(func|type) /{n=1} END{exit (bad||!n)}' "$t/after"
diff "$t/before" "$t/after"
echo "moves only: A, B, C and D pass"
```

What each check covers:

- **A.** The move commit changes exactly eight paths: the seven files and the census test. An edit anywhere else
  fails it.
- **B.** D1's ranges cover every non-blank line of the base `component.go` once, and each of the seven files, blank
  lines, package clause and import block left out, is its own base text (none for a new file) followed by its ranges,
  in order. A changed, added, removed or reordered line, or a declaration in the wrong file, fails it.
- **C.** Every import line of the seven files was in the base `component.go` import block or in that file's base
  import block. An added import, such as a blank `_ "net/http/pprof"`, fails it.
- **D.** `go doc -all -u` for the package, then `go doc -u -src` for each of the 83 functions, methods and types the
  base `component.go` declares, is the same before and after; the script checks there are 83 sections, each with a
  `func` or `type` line, and stops if any `go doc` call fails. This adds the blank lines inside declarations, which B
  leaves out.

Not covered: blank lines between declarations; a second blank line beside an existing one inside a body, which
`go doc` prints as one but `task fmt:check` refuses (`gofmt -l` lists such a file, measured); the census test's text,
read in the diff (D4); and anything after the move commit, which task 3.1 bounds by the paths it may change.

Measured on a scratch repository holding the base and the dry-run move as two commits: the script passes; each
mutant below, committed on top of the move, fails it at the check named, and a variant with one symbol that does not
exist stops at that symbol's `go doc` with exit 1.

| Mutant | Fails at |
| --- | --- |
| a comment added to `keyed_ingest.go` | A |
| "Subscription Management" banner changed to "Subscription management" in `consume.go` | B |
| two lines of `waitForStream` swapped | B |
| `wireMessageType` moved from `ingest.go` to the end of `consume.go` | B |
| `_ "net/http/pprof"` added to `consume.go`'s imports | C |
| a blank line inserted between two statements of `waitForStream` | D |

Review aids, not proof: `git diff -M -C --stat c1caa45 <move>` (dry run: `component.go` 1,859 deletions; `config.go`
120, `consume.go` 286, `ingest.go` 896, `metrics.go` 366 insertions; `poison_inventory.go` 164 and `query.go` 77
insertions; the `natsclient` test); `git blame -C -C` on each new file, which on the dry run attributes every line to
the base except package clauses, import lines and blank lines. `git log --follow` does not trace a split, since
`component.go` stays (inventory, first list).

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

No new test. The evidence is D3 on the move commit and `task verify` on the move commit and at the head, integration
tests included. No test cites a line of `component.go` (inventory section 4). On the dry run, `go test ./...` failed
only the census test until its names changed (inventory section 3); `go vet`, `gofmt -l` and the pinned `revive` were
clean. Examples suffice, with no generated check: D3's B compares every line of the seven files with its source line,
so there is no input left to vary.

## D7. ADRs

None changed. ADR-108 and ADR-113 govern the lifecycle that stays in `component.go`; ADR-110's write seam is
`entity_writes.go`, untouched. Their text is not edited (D4).

## D8. Unit of work and merge order

One pull request. Commits: the move with the census update (task 2.1), then citations and ledger (task 3.1), then the
OpenSpec records. About 1,860 lines move; the #159 bound counts pin lines ported, and this ports none, so no waiver
is needed. No open pull request overlaps (`gh pr list`: empty). Merge before the next change-4 pull request that
edits `processor/graph-ingest`, so it starts from the split files.

## Owner acceptance

No owner question. What the owner accepts with this design, beyond moving whole declarations:

- **Imports.** Each new file gets a package line and an import block, and `query.go` and `poison_inventory.go` each
  gain the import lines the moved code needs (D1).
- **One test in another package.** `natsclient/consumer_policy_callsite_test.go` checks which file holds
  graph-ingest's stream consumer. Its two expected file names change from `component.go` to `consume.go`, and one
  comment line says so, in the move commit, because the test fails otherwise (D4).
- **Three code comments** that say the code is in `component.go` name its new file: two graph-ingest test files and
  `graph/readiness/gauges_test.go` (D4).
- **One admission-ledger entry.** The destination of the `component.go` row names `ingest.go` and `consume.go` (D5).
- **The file map's choice.** Option B, D8's six files with `ingest.go` at 896 lines, over option C's seventh file.
