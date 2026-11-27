# Probe: graph-index reconcile cost, before and after the record (#765)

This branch never merges. It carries the probe test `processor/graph-index/probe765_integration_test.go`. No
production code is changed here.

- **Step 2.8 (below, first):** the record implementation, PR #774 at b9e68996, merged into this branch
  (merge commit dc90e27d). One run.
- **Step 2 (further down):** the code before the record, `origin/main` 3d823c82, at commit 24f3cc51. The test file
  at that commit is the one those figures came from.

## Step 2.8: with the per-entity record (PR #774 at b9e68996)

### Host

- Apple M3 Pro, 12 cores, 36 GiB, macOS 26.5.2, Go 1.26.11 darwin/arm64.
- NATS: the package's shared fixture, the pinned `nats:2.15.0-alpine` image (nats-server 2.15.0, as the server
  reports on connect), in Docker Desktop 29.9.3 (VM: 12 CPUs, 23.2 GiB). File storage inside the VM. Client nats.go
  v1.55.0 (`go.mod`).
- Load average (1, 5, 15 minutes) at each phase, from `sysctl -n vm.loadavg` inside the test: seed and cold-build
  start 6.51 / 8.77 / 11.80; cold-build end 6.63 / 7.79 / 9.42; warm-restart end 6.74 / 7.75 / 9.38; re-offer end
  6.33 / 7.61 / 9.30; live changes start 5.90 / 7.50 / 9.25. The run script waited 225 s for the 5-minute load to
  fall under 5 before starting; other sessions were running tests throughout. Per-30-second load and NATS container
  CPU are under "Raw load log (step 2.8)".
- One run of each phase at 40,000 entities. No repeat runs, so no spread across runs.

### Method

`TestProbeReconcileCost` (integration tag, `package graphindex`), one lane (`workers: 1`) in every phase:

1. **Seed** ENTITY_STATES with 40,000 entities, 16 writers. Each entity as in step 2: a `dc.terms.title` name, a
   `core.identity.alias`, two relationships (`(i+1) % N`, `(7i) % N`), three plain properties. 12 rows per entity:
   2 INCOMING, 1 OUTGOING, 7 PREDICATE, 1 ALIAS, 1 NAME.
2. **Cold build:** a graph-index over the seeded store with empty indexes and no records, timed from `Start` until
   its indexed floor reaches revision 40,000 and `QueryReady` answers nil. Then stopped.
3. **Warm restart:** a second graph-index over the same store, indexes and records, timed the same way.
4. **Equal re-offer:** on that ready component, every entity is written again with byte-identical statements (16
   writers), so each gets a new revision (40,001 to 80,000). Timed from the first write until the floor reaches
   80,000.
5. **Live-change breakdown**, on the ready, idle component, 1,000 entities (every 40th), one at a time. For each,
   timed separately: the ENTITY_STATES `Get`; decode and plan of a changed value (new title, new `test.link.near`
   target); the record `Get` and decode; the diff (`changeOps`); then the operations `runApply` makes, in its order:
   the pending record put, the row writes (5: OUTGOING put, INCOMING put and delete, NAME put and delete), the clean
   record put. ENTITY_STATES is not written in these steps, so the follower stays idle. Then the real
   `reconcileEntity` (revision 0, as a repair pass runs it): it reads the stored value, which differs from the
   record, and takes the index back to it, a change of the same size from the record. Then `reconcileEntity` again,
   which finds the record equal to the plan: the unchanged path.
6. **Live changes end to end:** 1,000 entities (every 40th), each written to ENTITY_STATES with another changed value
   (same shape), one at a time, timed from the `Put` until the follower's indexed floor reaches the put's revision.
   The wait reads the floor (an atomic) in a loop that yields, with no poll interval.
7. **Counts** for each phase: the component's `kv_operations_total` by operation and bucket (listings, row puts and
   deletes, record gets and puts); each index bucket's stream last sequence before and after (every row put or
   delete appends one message, whatever the component counts); consumer-create advisories on
   `$JS.EVENT.ADVISORY.CONSUMER.CREATED.>` (a listing creates one consumer); JetStream API calls from account info.
8. **Sizes:** each bucket's stream info (stored messages, subjects, bytes) after the cold build and after the run.

### Command

```bash
PROBE_765_SIZES=40000 go test -tags integration -count=1 -timeout 150m -run TestProbeReconcileCost -v ./processor/graph-index/
```

Run through `/tmp/probe765-2.8/run.sh` (local only, not committed), which waited for the 5-minute load average to fall
under 5 (at most 15 minutes) and logged load and NATS container CPU every 30 s. Without `PROBE_765_SIZES` the test
runs once at 200 entities (about 3 s). `PROBE_765_WORKERS` sets the lane count (default 1).

`task verify` was not run on this branch: it is a scratch branch and never merges. `go vet -tags integration
./processor/graph-index/` and `gofmt` pass on the probe file.

### Results (all measured, one run, 40,000 entities, one lane)

| Phase | Time | per entity | Listings | Row writes | Record puts | Consumers created |
| --- | --- | --- | --- | --- | --- | --- |
| Cold build (no records) | 666.0 s | 16.65 ms | 160,000 (4.00 each) | 480,000 puts | 80,000 | 160,001 |
| Warm restart | 14.7 s | 0.368 ms | 0 | 0 | 0 | 1 (the follower's watch) |
| Equal re-offer | 14.3 s (writes 1.1 s) | 0.358 ms | 0 | 0 | 0 | 0 |
| 1,000 live changes | 1.62 ms mean each | | 0 | 5,000 (5 each) | 2,000 (2 each) | 0 |

Each phase read 1 record per entity (40,000; 1,000 for the live changes). The index streams' sequences moved by 0 in
the warm restart and in the re-offer, and by 5,000 in the live changes, matching the counted writes.

Live-change breakdown, ms, 1,000 entities one at a time:

| Step | median | p90 | p99 | mean |
| --- | --- | --- | --- | --- |
| ENTITY_STATES Get | 0.155 | 0.169 | 0.323 | 0.165 |
| decode + plan | 0.030 | 0.035 | 0.047 | 0.031 |
| record Get + decode | 0.154 | 0.171 | 1.413 | 0.197 |
| diff | 0.012 | 0.015 | 0.021 | 0.012 |
| pending record put | 0.166 | 0.185 | 1.543 | 0.216 |
| row writes (5) | 0.796 | 0.838 | 1.232 | 0.843 |
| clean record put | 0.163 | 0.175 | 0.209 | 0.165 |
| record puts (both) | 0.329 | 0.362 | 1.715 | 0.381 |
| sum of steps | 1.479 | 1.659 | 4.121 | 1.633 |
| reconcileEntity, a change (real) | 1.460 | 1.527 | 1.759 | 1.474 |
| reconcileEntity, unchanged (real) | 0.345 | 0.365 | 0.435 | 0.347 |
| live change, put to applied | 1.600 | 1.680 | 2.069 | 1.621 |
| of which ENTITY_STATES put | 0.172 | 0.184 | 0.209 | 0.171 |

Bucket sizes after the run:

| Bucket | Stored messages | Subjects | Bytes |
| --- | --- | --- | --- |
| GRAPH_INDEX_RECORDS | 40,000 | 40,000 | 79,194,585 |
| ENTITY_STATES | 40,000 | 40,000 | 49,554,890 |
| INCOMING_INDEX | 82,000 | 82,000 | 12,128,000 |
| OUTGOING_INDEX | 40,000 | 40,000 | 10,040,000 |
| PREDICATE_INDEX | 280,000 | 280,000 | 28,320,000 |
| ALIAS_INDEX | 40,000 | 40,000 | 7,400,000 |
| NAME_INDEX | 42,000 | 42,000 | 8,808,890 |

INCOMING and NAME hold 2,000 more messages than after the cold build: the delete markers the live changes left (a KV
delete stores a marker under the key). After the cold build the five index buckets held 65,908,890 bytes and the
record bucket 79,194,445 (1,980 bytes per entity, for 12 rows).

### Reading

- **Warm restart: 14.7 s, not about 38 minutes.** No listing, no row write, no record put: one record read per
  entity. The design estimated about 0.5 ms per unchanged entity and about 20 s; measured 0.368 ms and 14.7 s. Step 2
  measured 17.4 entities/s over the first 5 minutes at this size; this run did 2,717/s to the end.
- **Equal re-offer: the same path, 0.358 ms per entity**, with nothing listed or written.
- **Live change: 1.6 ms put to applied, 1.46 ms in `reconcileEntity`**, against the design's about 1.7 ms and step 2's
  43 ms (`reconcileEntity`) at 40k. The row writes are about half; the record Get and the two record puts about a
  third.
- **Cold build: 666 s on one lane, 16.65 ms per entity, with exactly 4 listings per entity.** This is the design's
  non-goal (a first seed lists as before). It is faster per entity than step 2's warm 1-lane replay (57 ms) because
  the indexes start empty and grow; step 2's cold build used 16 lanes (93 s), so the two cold figures are not
  comparable.
- **The record bucket is the largest:** 79 MB for 40,000 entities, about 1.2 times the five index buckets together
  and 1.6 times ENTITY_STATES. Inferred, not measured: its size is about 1,980 bytes per entity at 12 rows, so it
  grows with each entity's row count.

### Skipped (step 2.8)

- Repeat runs: each phase ran once.
- Per-step timing inside the running lane during the warm restart: the unchanged path is timed separately in the
  breakdown (`reconcileEntity`, unchanged), not inside the replay.
- Other sizes, more lanes, the 105k watermark plateau, the Mac mini.
- A cold build on 16 lanes for comparison with step 2's 93 s.

### Raw test output (step 2.8; testcontainers and bucket lines removed)

```text
    probe765_integration_test.go:91: nats-server 2.15.0
    probe765_integration_test.go:120: load at seed: { 6.51 8.77 11.80 }
    probe765_integration_test.go:122: seeded 40000 entities in 880ms
    probe765_integration_test.go:125: load at cold build start: { 6.51 8.77 11.80 }
    probe765_integration_test.go:316:   progress 30s: indexed revision 5327 (177.5/s overall)
    probe765_integration_test.go:316:   progress 1m0s: indexed revision 9377 (156.2/s overall)
    probe765_integration_test.go:316:   progress 1m30s: indexed revision 12646 (140.5/s overall)
    probe765_integration_test.go:316:   progress 2m0s: indexed revision 15161 (126.3/s overall)
    probe765_integration_test.go:316:   progress 2m30s: indexed revision 17301 (115.3/s overall)
    probe765_integration_test.go:316:   progress 3m0s: indexed revision 18929 (105.2/s overall)
    probe765_integration_test.go:316:   progress 3m30s: indexed revision 20708 (98.6/s overall)
    probe765_integration_test.go:316:   progress 4m0s: indexed revision 22603 (94.2/s overall)
    probe765_integration_test.go:316:   progress 4m30s: indexed revision 24340 (90.1/s overall)
    probe765_integration_test.go:316:   progress 5m0s: indexed revision 26047 (86.8/s overall)
    probe765_integration_test.go:316:   progress 5m30s: indexed revision 27486 (83.3/s overall)
    probe765_integration_test.go:316:   progress 6m0s: indexed revision 29020 (80.6/s overall)
    probe765_integration_test.go:316:   progress 6m30s: indexed revision 30262 (77.6/s overall)
    probe765_integration_test.go:316:   progress 7m0s: indexed revision 31409 (74.8/s overall)
    probe765_integration_test.go:316:   progress 7m30s: indexed revision 32457 (72.1/s overall)
    probe765_integration_test.go:316:   progress 8m0s: indexed revision 33693 (70.2/s overall)
    probe765_integration_test.go:316:   progress 8m30s: indexed revision 34717 (68.1/s overall)
    probe765_integration_test.go:316:   progress 9m0s: indexed revision 35583 (65.9/s overall)
    probe765_integration_test.go:316:   progress 9m30s: indexed revision 36524 (64.1/s overall)
    probe765_integration_test.go:316:   progress 10m0s: indexed revision 37487 (62.5/s overall)
    probe765_integration_test.go:316:   progress 10m30s: indexed revision 38584 (61.2/s overall)
    probe765_integration_test.go:316:   progress 11m0s: indexed revision 39763 (60.2/s overall)
    probe765_integration_test.go:130: load at cold build end: { 6.63 7.79 9.42 }
    probe765_integration_test.go:131: cold build (workers 1): 40000 entities ready in 11m6.04s (60.1/s, 16.651 ms per entity), consumer-create advisories 160001 (4.00 per entity), JetStream API calls 320197 (8.00 per entity)
    probe765_integration_test.go:136: cold build ops: listings 160000 (4.00 per entity), row puts 480000, row deletes 0, record gets 40000, record puts 80000, record deletes 0
    probe765_integration_test.go:137:   ENTITY_STATES: 40000 stored messages, 40000 subjects, 49552890 bytes
    probe765_integration_test.go:137:   INCOMING_INDEX: 80000 stored messages, 80000 subjects, 11760000 bytes
    probe765_integration_test.go:137:   OUTGOING_INDEX: 40000 stored messages, 40000 subjects, 10040000 bytes
    probe765_integration_test.go:137:   PREDICATE_INDEX: 280000 stored messages, 280000 subjects, 28320000 bytes
    probe765_integration_test.go:137:   ALIAS_INDEX: 40000 stored messages, 40000 subjects, 7400000 bytes
    probe765_integration_test.go:137:   NAME_INDEX: 40000 stored messages, 40000 subjects, 8388890 bytes
    probe765_integration_test.go:137:   GRAPH_INDEX_RECORDS: 40000 stored messages, 40000 subjects, 79194445 bytes
    probe765_integration_test.go:144: load at warm restart start: { 6.63 7.79 9.42 }
    probe765_integration_test.go:150: load at warm restart end: { 6.74 7.75 9.38 }
    probe765_integration_test.go:151: warm restart (workers 1): 40000 entities ready in 14.724s (2716.7/s, 0.368 ms per entity), consumer-create advisories 1, JetStream API calls 52 (0.00 per entity)
    probe765_integration_test.go:156: warm restart ops: listings 0 (0.00 per entity), row puts 0, row deletes 0, record gets 40000, record puts 0, record deletes 0
    probe765_integration_test.go:157: warm restart index stream messages appended: 0 (ALIAS_INDEX +0, INCOMING_INDEX +0, NAME_INDEX +0, OUTGOING_INDEX +0, PREDICATE_INDEX +0)
    probe765_integration_test.go:161: load at re-offer start: { 6.74 7.75 9.38 }
    probe765_integration_test.go:171: load at re-offer end: { 6.33 7.61 9.30 }
    probe765_integration_test.go:172: equal re-offer (workers 1): 40000 writes in 1.075s, all applied 14.303s after the first write (2796.6/s, 0.358 ms per entity), consumer-create advisories 0, JetStream API calls 4 (0.00 per entity)
    probe765_integration_test.go:178: equal re-offer ops: listings 0 (0.00 per entity), row puts 0, row deletes 0, record gets 40000, record puts 0, record deletes 0
    probe765_integration_test.go:179: equal re-offer index stream messages appended: 0 (ALIAS_INDEX +0, INCOMING_INDEX +0, NAME_INDEX +0, OUTGOING_INDEX +0, PREDICATE_INDEX +0)
    probe765_integration_test.go:183: load at breakdown start: { 6.33 7.61 9.30 }
    probe765_integration_test.go:184: breakdown of a live change over 1000 entities (every 40th), ms:
    probe765_integration_test.go:184:   | step                     |    median |       p90 |       p99 |      mean |
    probe765_integration_test.go:478:   | ENTITY_STATES get        |     0.155 |     0.169 |     0.323 |     0.165 |
    probe765_integration_test.go:478:   | decode + plan            |     0.030 |     0.035 |     0.047 |     0.031 |
    probe765_integration_test.go:478:   | record get + decode      |     0.154 |     0.171 |     1.413 |     0.197 |
    probe765_integration_test.go:478:   | diff (changeOps)         |     0.012 |     0.015 |     0.021 |     0.012 |
    probe765_integration_test.go:478:   | pending record put       |     0.166 |     0.185 |     1.543 |     0.216 |
    probe765_integration_test.go:478:   | row writes               |     0.796 |     0.838 |     1.232 |     0.843 |
    probe765_integration_test.go:478:   | clean record put         |     0.163 |     0.175 |     0.209 |     0.165 |
    probe765_integration_test.go:478:   | record puts (both)       |     0.329 |     0.362 |     1.715 |     0.381 |
    probe765_integration_test.go:478:   | sum of steps             |     1.479 |     1.659 |     4.121 |     1.633 |
    probe765_integration_test.go:478:   | reconcileEntity, change  |     1.460 |     1.527 |     1.759 |     1.474 |
    probe765_integration_test.go:478:   | reconcileEntity, same    |     0.345 |     0.365 |     0.435 |     0.347 |
    probe765_integration_test.go:184: row writes per change: 5.00
    probe765_integration_test.go:184: breakdown steps ops: listings 0 (0.00 per entity), row puts 3000, row deletes 2000, record gets 0, record puts 2000, record deletes 0
    probe765_integration_test.go:184: breakdown reconcileEntity change ops: listings 0 (0.00 per entity), row puts 3000, row deletes 2000, record gets 1000, record puts 2000, record deletes 0
    probe765_integration_test.go:184: breakdown reconcileEntity same ops: listings 0 (0.00 per entity), row puts 0, row deletes 0, record gets 1000, record puts 0, record deletes 0
    probe765_integration_test.go:184: consumer-create advisories: 0.00 per change reconcile, 0.00 per unchanged reconcile
    probe765_integration_test.go:185: load at live changes start: { 5.90 7.50 9.25 }
    probe765_integration_test.go:186: live changes: 1000, one at a time, each awaited until applied, ms:
    probe765_integration_test.go:186:   | step                     |    median |       p90 |       p99 |      mean |
    probe765_integration_test.go:186:   | ENTITY_STATES put        |     0.172 |     0.184 |     0.209 |     0.171 |
    probe765_integration_test.go:186:   | put to applied           |     1.600 |     1.680 |     2.069 |     1.621 |
    probe765_integration_test.go:186: live changes ops: listings 0 (0.00 per entity), row puts 3000, row deletes 2000, record gets 1000, record puts 2000, record deletes 0
    probe765_integration_test.go:186: live changes index stream messages appended: 5000 (ALIAS_INDEX +0, INCOMING_INDEX +2000, NAME_INDEX +2000, OUTGOING_INDEX +1000, PREDICATE_INDEX +0)
    probe765_integration_test.go:186: live changes consumer-create advisories: 0
    probe765_integration_test.go:187: load at live changes end: { 5.90 7.50 9.25 }
    probe765_integration_test.go:191: after the run:
    probe765_integration_test.go:192:   ENTITY_STATES: 40000 stored messages, 40000 subjects, 49554890 bytes
    probe765_integration_test.go:192:   INCOMING_INDEX: 82000 stored messages, 82000 subjects, 12128000 bytes
    probe765_integration_test.go:192:   OUTGOING_INDEX: 40000 stored messages, 40000 subjects, 10040000 bytes
    probe765_integration_test.go:192:   PREDICATE_INDEX: 280000 stored messages, 280000 subjects, 28320000 bytes
    probe765_integration_test.go:192:   ALIAS_INDEX: 40000 stored messages, 40000 subjects, 7400000 bytes
    probe765_integration_test.go:192:   NAME_INDEX: 42000 stored messages, 42000 subjects, 8808890 bytes
    probe765_integration_test.go:192:   GRAPH_INDEX_RECORDS: 40000 stored messages, 40000 subjects, 79194585 bytes
--- PASS: TestProbeReconcileCost (702.13s)
    --- PASS: TestProbeReconcileCost/40000 (702.13s)
PASS
ok      github.com/c360studio/semengine/processor/graph-index    702.708s
exit 0
```

### Raw load log (step 2.8; every 30 s: load average, NATS container CPU and memory)

```text
10:42:33 load={ 4.99 8.51 11.73 } 
10:43:04 load={ 6.16 8.46 11.58 } 8c710caccc7a cpu=65.21% mem=141.3MiB / 23.19GiB
10:43:36 load={ 6.28 8.27 11.40 } 8c710caccc7a cpu=75.38% mem=184.3MiB / 23.19GiB
10:44:07 load={ 5.91 7.99 11.19 } 8c710caccc7a cpu=82.78% mem=196.3MiB / 23.19GiB
10:44:38 load={ 5.66 7.73 10.99 } 8c710caccc7a cpu=82.79% mem=194.2MiB / 23.19GiB
10:45:09 load={ 7.62 7.93 10.92 } 8c710caccc7a cpu=84.02% mem=199.1MiB / 23.19GiB
10:45:40 load={ 7.81 7.96 10.83 } 8c710caccc7a cpu=84.47% mem=200.8MiB / 23.19GiB
10:46:11 load={ 7.02 7.77 10.66 } 8c710caccc7a cpu=85.56% mem=209.5MiB / 23.19GiB
10:46:42 load={ 6.47 7.56 10.48 } 8c710caccc7a cpu=86.87% mem=223.1MiB / 23.19GiB
10:47:13 load={ 6.15 7.37 10.31 } 8c710caccc7a cpu=88.51% mem=223.6MiB / 23.19GiB
10:47:44 load={ 5.95 7.16 10.12 } 8c710caccc7a cpu=84.68% mem=229.1MiB / 23.19GiB
10:48:15 load={ 5.81 6.99 9.95 } 8c710caccc7a cpu=88.91% mem=230.3MiB / 23.19GiB
10:48:47 load={ 5.32 6.75 9.76 } 8c710caccc7a cpu=89.09% mem=237.8MiB / 23.19GiB
10:49:18 load={ 7.47 7.13 9.79 } 8c710caccc7a cpu=92.03% mem=243.1MiB / 23.19GiB
10:49:49 load={ 7.64 7.21 9.71 } 8c710caccc7a cpu=87.87% mem=250.3MiB / 23.19GiB
10:50:20 load={ 7.42 7.19 9.62 } 8c710caccc7a cpu=88.97% mem=249.1MiB / 23.19GiB
10:50:51 load={ 6.41 6.96 9.45 } 8c710caccc7a cpu=89.15% mem=252.6MiB / 23.19GiB
10:51:22 load={ 11.99 8.29 9.85 } 8c710caccc7a cpu=89.65% mem=259.5MiB / 23.19GiB
10:51:53 load={ 10.40 8.25 9.77 } 8c710caccc7a cpu=89.87% mem=260.2MiB / 23.19GiB
10:52:24 load={ 10.73 8.57 9.83 } 8c710caccc7a cpu=88.89% mem=264.4MiB / 23.19GiB
10:52:55 load={ 8.51 8.23 9.66 } 8c710caccc7a cpu=88.90% mem=288.5MiB / 23.19GiB
10:53:27 load={ 7.38 7.98 9.52 } 8c710caccc7a cpu=90.61% mem=287.6MiB / 23.19GiB
10:53:58 load={ 6.74 7.75 9.38 } 8c710caccc7a cpu=57.14% mem=419MiB / 23.19GiB
```

## Step 2: before the record (origin/main 3d823c82, commit 24f3cc51)

### Host

- Apple M3 Pro, 12 cores, 36 GiB, macOS 26.5.2, Go 1.26.11 darwin/arm64.
- NATS: the package's shared fixture, `nats:2.15.0-alpine@sha256:ac8f88a6…` (nats-server 2.15.0), in Docker Desktop
  29.9.3 (VM: 12 CPUs, 23.2 GiB). File storage inside the VM. Client nats.go v1.55.0.
- Load average during the runs: 4.6 to 12 (1-minute); other sessions were running tests. Before the first run it was
  2.3; between the 10k and 20k runs it reached 16 from another session's tests, and the 20k run waited until it fell
  under 5. Per-30-second load and NATS container CPU are under "Raw load log" below.

### Method

`TestProbeReconcileCost` (integration tag, `package graphindex`), one subtest per store size:

1. Seed ENTITY_STATES with N entities, 16 writers. Each entity: a `dc.terms.title` name, a `core.identity.alias`,
   two relationships to other entities (`(i+1) % N`, `(7i) % N`) and three plain properties. Rows per entity:
   2 INCOMING, 1 OUTGOING, 7 PREDICATE, 1 ALIAS, 1 NAME (row counts are logged and match).
2. Cold build: graph-index with 16 lanes over the store, awaited until `QueryReady` is nil and the floor reaches N.
3. Breakdown, on that ready, idle component (every row is stored, which is what a warm reconcile finds): 1,000
   entities, every (N/1000)th, one at a time like one lane. Per entity, timed separately, in this order:
   ENTITY_STATES `Get`; decode and plan; each of the four owner listings (`KeysByFilter` with the production
   filter); the writes a warm reconcile makes (the OUTGOING put and the NAME row put, which `overwriteExisting`
   rewrites; INCOMING, PREDICATE and ALIAS rows already stored are not rewritten); then the real
   `reconcileEntity` (revision 0, as a repair pass runs it) as the total. "sum of steps" is the separately timed
   steps added; `reconcileEntity` is the real path.
4. Stop it, then a warm restart: a second graph-index (1 lane, SemSource's default `workers: 0`) over the same store
   and indexes, timed from `Start` to ready. `PROBE_765_WARM_WINDOW` ends the observation early and reports the
   speed reached.
5. Consumers: a plain connection subscribes to `$JS.EVENT.ADVISORY.CONSUMER.CREATED.>`, flushed before each read;
   JetStream API call count from account info before and after the warm restart.

The 10k run used an earlier version of the test in which the breakdown ran on the warm component after its replay
rather than on the cold one; the indexes are the same in both cases.

### Commands

```bash
PROBE_765_SIZES=10000 go test -tags integration -count=1 -timeout 60m -run TestProbeReconcileCost -v ./processor/graph-index/
PROBE_765_SIZES=20000 go test -tags integration -count=1 -timeout 60m -run TestProbeReconcileCost -v ./processor/graph-index/
PROBE_765_SIZES=40000 PROBE_765_WARM_WINDOW=5m go test -tags integration -count=1 -timeout 60m -run TestProbeReconcileCost -v ./processor/graph-index/
PROBE_765_SIZES=20000 PROBE_765_WARM_WORKERS=16 PROBE_765_WARM_WINDOW=5m go test -tags integration -count=1 -timeout 60m -run TestProbeReconcileCost -v ./processor/graph-index/
```

Without the environment variables the test runs once at 200 entities (about 4 s).

### Results

#### Warm restart, 1 lane (measured)

| Store | Time to ready | Entities/s | ms per entity (wall / N) | Consumers created per entity |
| --- | --- | --- | --- | --- |
| 10,000 | 176.9 s | 56.5 | 17.69 | 4.00 (40,001 total) |
| 20,000 | 610.4 s | 32.8 | 30.52 | 4.00 (80,001 total) |
| 40,000 | not reached; 5,235 indexed in the 300 s window | 17.4 | 57.31 | 4.00 (20,942 for 5,235) |

The extra one consumer per run is the follower's own watch. JetStream API calls during the warm replay: 8.87, 9.47 and
10.74 per entity (a listing is a consumer create and a delete, so 8 of them are the four listings).

Inferred, not measured: at 17.4/s a full 40,000 warm restart takes about 2,300 s (38 minutes). In the 10k and 20k
runs the rate rose in the last part of the replay (20k: about 25/s at first, 54 to 60/s in its last 40 s), so the
full 40k run may be somewhat shorter.

#### Per-entity breakdown, ms (measured, 1,000 entities each, one at a time)

40,000 entities:

| Step | median | p90 | p99 | mean |
| --- | --- | --- | --- | --- |
| ENTITY_STATES Get | 0.240 | 0.392 | 1.680 | 0.362 |
| decode + plan | 0.043 | 0.165 | 0.206 | 0.073 |
| list INCOMING | 14.372 | 19.081 | 26.760 | 14.500 |
| list PREDICATE | 6.928 | 9.443 | 17.787 | 7.036 |
| list ALIAS | 12.920 | 17.769 | 22.793 | 13.266 |
| list NAME | 13.912 | 17.931 | 23.348 | 14.532 |
| four listings | 48.340 | 62.813 | 77.805 | 49.335 |
| writes (OUTGOING + NAME put) | 0.598 | 1.802 | 2.307 | 0.775 |
| sum of steps | 49.488 | 64.747 | 81.141 | 50.547 |
| reconcileEntity (real) | 43.446 | 54.804 | 74.926 | 43.778 |

20,000 entities (first run, before its 1-lane warm restart):

| Step | median | p90 | p99 | mean |
| --- | --- | --- | --- | --- |
| ENTITY_STATES Get | 0.229 | 0.470 | 2.046 | 0.352 |
| decode + plan | 0.038 | 0.053 | 0.113 | 0.042 |
| list INCOMING | 9.631 | 16.719 | 26.130 | 10.845 |
| list PREDICATE | 5.241 | 8.022 | 12.382 | 5.406 |
| list ALIAS | 8.497 | 12.441 | 19.671 | 9.064 |
| list NAME | 10.202 | 14.459 | 22.634 | 10.855 |
| four listings | 34.658 | 50.481 | 68.635 | 36.171 |
| writes | 0.531 | 1.203 | 3.213 | 0.741 |
| sum of steps | 35.739 | 52.167 | 72.639 | 37.308 |
| reconcileEntity (real) | 30.800 | 45.494 | 65.188 | 31.953 |

A second 20,000 sample (the 16-lane run) gave four listings 27.9 / 36.2 / 43.0 / 28.2 and reconcileEntity
24.8 / 32.2 / 40.0 / 25.2 (median / p90 / p99 / mean).

10,000 entities (breakdown after the warm replay):

| Step | median | p90 | p99 | mean |
| --- | --- | --- | --- | --- |
| ENTITY_STATES Get | 0.244 | 1.247 | 3.730 | 0.513 |
| decode + plan | 0.050 | 0.115 | 0.176 | 0.064 |
| list INCOMING | 4.854 | 8.735 | 18.986 | 5.804 |
| list PREDICATE | 3.438 | 5.686 | 10.345 | 3.440 |
| list ALIAS | 3.648 | 8.357 | 18.120 | 4.733 |
| list NAME | 4.206 | 7.906 | 19.415 | 5.341 |
| four listings | 16.191 | 30.575 | 60.895 | 19.320 |
| writes | 0.544 | 1.421 | 4.934 | 0.833 |
| sum of steps | 17.253 | 32.739 | 65.122 | 20.731 |
| reconcileEntity (real) | 15.310 | 28.566 | 56.823 | 18.336 |

200 entities (smoke run): one listing 0.53 to 0.57 ms median, four listings 2.2 ms, reconcileEntity 2.8 ms.

Consumers created, breakdown: exactly 4.00 per entity for the four timed listings and 4.00 per `reconcileEntity`, at
every size.

#### Per-entity cost against store size (measured)

| Store | mean of one listing | four listings, mean | reconcileEntity, mean | warm wall per entity |
| --- | --- | --- | --- | --- |
| 200 | 0.59 | 2.36 | 2.88 | 2.89 |
| 10,000 | 4.83 | 19.32 | 18.34 | 17.69 |
| 20,000 | 9.04 / 7.04 | 36.17 / 28.15 | 31.95 / 25.24 | 30.52 |
| 40,000 | 12.33 | 49.34 | 43.78 | 57.31 (first 5,235) |

#### Lanes (measured, one run)

| Store | Lanes | Warm time to ready | Entities/s | NATS container CPU |
| --- | --- | --- | --- | --- |
| 20,000 | 1 | 610.4 s | 32.8 | about 105 to 113 % |
| 20,000 | 16 | 121.2 s | 165.0 | about 630 to 730 % |

Cold builds, 16 lanes, empty indexes at start: 10k 10.3 s (973/s), 20k 32.4 s and 25.2 s (617/s, 795/s), 40k 93.2 s
(429/s).

### Reading

- **Confirmed: per-entity cost is the whole wait.** With one lane the warm wall time per entity (17.7, 30.5 ms) equals
  the separately measured `reconcileEntity` mean (18.3, 32.0 ms) at 10k and 20k. At 40k the replay ran at 57 ms per
  entity against a 44 ms sampled mean; the replay pays also for the follower and lane hand-off, and the sample ran
  on an idle server, so the per-entity figure accounts for at least three quarters of the wait there.
- **Where the time goes: the four listings, 93 to 98 % of the summed steps.** The `Get` is 0.35 to 0.5 ms mean
  (1 to 2.5 %), decode and plan under 0.1 ms, the writes 0.75 to 0.85 ms (1.5 to 4 %).
- **Consumers: 4.00 per entity, exactly**, at every size, plus the follower's one watch.
- **Per-entity cost grows with store size**, so a full replay grows faster than linearly: one listing costs 0.6 ms at
  200 entities, 4.8 at 10k, 7 to 9 at 20k and 12.3 at 40k. The speed falls as reported: 56.5/s at 10k, 32.8/s at 20k,
  17.4/s at 40k. On this M3 the 40k rate matches SemSource's 16/s on the 2018 Mac mini, so the last row of the
  issue's table is not explained by the slower host alone (inferred from the two figures; the two runs differ in
  data shape, NATS deployment and settings).
- **The listing is server CPU.** One lane keeps the NATS container at about one core; 16 lanes keep it at 6 to 7 cores
  and the warm restart is five times faster. Each listing filter starts with wildcards
  (`*.*.*.*.*.*.<source>.*`, `*.*.*.<entity>`, `*.<entity>.*`), so a reading, not measured here: the server cannot
  narrow such a filter by its leading tokens and its cost follows the bucket's size.

### Skipped

- A full 40,000 warm restart: projected at about 38 minutes, over the time-box; the 40k figure is the first five
  minutes of the replay.
- Per-step timing inside the running lanes during the replay itself: no production hook; the breakdown is a separate
  pass of the same calls on the same component and server, one at a time.
- The 105k watermark plateau, SemSource's own settings, and a run on the Mac mini.
- Each size ran once (20k twice for the breakdown); no repeat runs to give a spread across runs.

### Raw test output (testcontainers and bucket lines removed)

```text
=== run-10k.log
    probe765_integration_test.go:133: seeded 10000 entities in 202ms
    probe765_integration_test.go:138: cold build: 10000 entities ready in 10.277s (973.0/s), consumer-create advisories 40001 (4.00 per entity)
    probe765_integration_test.go:142:   ENTITY_STATES: 10000 stored messages, 10000 subjects, 12379890 bytes
    probe765_integration_test.go:142:   INCOMING_INDEX: 20000 stored messages, 20000 subjects, 2940000 bytes
    probe765_integration_test.go:142:   OUTGOING_INDEX: 10000 stored messages, 10000 subjects, 2510000 bytes
    probe765_integration_test.go:142:   PREDICATE_INDEX: 70000 stored messages, 70000 subjects, 7080000 bytes
    probe765_integration_test.go:142:   ALIAS_INDEX: 10000 stored messages, 10000 subjects, 1850000 bytes
    probe765_integration_test.go:142:   NAME_INDEX: 10000 stored messages, 10000 subjects, 2088890 bytes
    probe765_integration_test.go:182:   progress 30s: indexed revision 1207 (40.2/s overall)
    probe765_integration_test.go:182:   progress 1m0s: indexed revision 2508 (41.8/s overall)
    probe765_integration_test.go:182:   progress 1m30s: indexed revision 4020 (44.7/s overall)
    probe765_integration_test.go:182:   progress 2m0s: indexed revision 6047 (50.4/s overall)
    probe765_integration_test.go:182:   progress 2m30s: indexed revision 7972 (53.1/s overall)
    probe765_integration_test.go:149: warm restart (workers 1): 10000 entities ready in 2m56.91s (56.5/s, 17.69 ms per entity), consumer-create advisories 40001 (4.00 per entity), JetStream API calls 88660 (8.87 per entity)
    probe765_integration_test.go:156: breakdown over 1000 entities (every 10th), ms:
    probe765_integration_test.go:156:   | step             |    median |       p90 |       p99 |      mean |
    probe765_integration_test.go:303:   | get              |     0.244 |     1.247 |     3.730 |     0.513 |
    probe765_integration_test.go:303:   | decode+plan      |     0.050 |     0.115 |     0.176 |     0.064 |
    probe765_integration_test.go:303:   | list incoming    |     4.854 |     8.735 |    18.986 |     5.804 |
    probe765_integration_test.go:303:   | list predicate   |     3.438 |     5.686 |    10.345 |     3.440 |
    probe765_integration_test.go:303:   | list alias       |     3.648 |     8.357 |    18.120 |     4.733 |
    probe765_integration_test.go:303:   | list name        |     4.206 |     7.906 |    19.415 |     5.341 |
    probe765_integration_test.go:303:   | four listings    |    16.191 |    30.575 |    60.895 |    19.320 |
    probe765_integration_test.go:303:   | writes           |     0.544 |     1.421 |     4.934 |     0.833 |
    probe765_integration_test.go:303:   | sum of steps     |    17.253 |    32.739 |    65.122 |    20.731 |
    probe765_integration_test.go:303:   | reconcileEntity  |    15.310 |    28.566 |    56.823 |    18.336 |
    probe765_integration_test.go:156: consumer-create advisories: 4.00 per entity for the four timed listings, 4.00 per reconcileEntity
--- PASS: TestProbeReconcileCost (227.65s)
ok      github.com/c360studio/semengine/processor/graph-index    228.049s
=== run-20k.log
    probe765_integration_test.go:133: seeded 20000 entities in 460ms
    probe765_integration_test.go:196:   progress 30s: indexed revision 18646 (621.3/s overall)
    probe765_integration_test.go:138: cold build: 20000 entities ready in 32.407s (617.1/s), consumer-create advisories 80001 (4.00 per entity)
    probe765_integration_test.go:141:   ENTITY_STATES: 20000 stored messages, 20000 subjects, 24770890 bytes
    probe765_integration_test.go:141:   INCOMING_INDEX: 40000 stored messages, 40000 subjects, 5880000 bytes
    probe765_integration_test.go:141:   OUTGOING_INDEX: 20000 stored messages, 20000 subjects, 5020000 bytes
    probe765_integration_test.go:141:   PREDICATE_INDEX: 140000 stored messages, 140000 subjects, 14160000 bytes
    probe765_integration_test.go:141:   ALIAS_INDEX: 20000 stored messages, 20000 subjects, 3700000 bytes
    probe765_integration_test.go:141:   NAME_INDEX: 20000 stored messages, 20000 subjects, 4188890 bytes
    probe765_integration_test.go:145: breakdown over 1000 entities (every 20th), ms:
    probe765_integration_test.go:145:   | step             |    median |       p90 |       p99 |      mean |
    probe765_integration_test.go:325:   | get              |     0.229 |     0.470 |     2.046 |     0.352 |
    probe765_integration_test.go:325:   | decode+plan      |     0.038 |     0.053 |     0.113 |     0.042 |
    probe765_integration_test.go:325:   | list incoming    |     9.631 |    16.719 |    26.130 |    10.845 |
    probe765_integration_test.go:325:   | list predicate   |     5.241 |     8.022 |    12.382 |     5.406 |
    probe765_integration_test.go:325:   | list alias       |     8.497 |    12.441 |    19.671 |     9.064 |
    probe765_integration_test.go:325:   | list name        |    10.202 |    14.459 |    22.634 |    10.855 |
    probe765_integration_test.go:325:   | four listings    |    34.658 |    50.481 |    68.635 |    36.171 |
    probe765_integration_test.go:325:   | writes           |     0.531 |     1.203 |     3.213 |     0.741 |
    probe765_integration_test.go:325:   | sum of steps     |    35.739 |    52.167 |    72.639 |    37.308 |
    probe765_integration_test.go:325:   | reconcileEntity  |    30.800 |    45.494 |    65.188 |    31.953 |
    probe765_integration_test.go:145: consumer-create advisories: 4.00 per entity for the four timed listings, 4.00 per reconcileEntity
    probe765_integration_test.go:196:   progress 30s: indexed revision 747 (24.9/s overall)
    probe765_integration_test.go:196:   progress 1m0s: indexed revision 1458 (24.3/s overall)
    probe765_integration_test.go:196:   progress 1m30s: indexed revision 2282 (25.4/s overall)
    probe765_integration_test.go:196:   progress 2m0s: indexed revision 3146 (26.2/s overall)
    probe765_integration_test.go:196:   progress 2m30s: indexed revision 4047 (27.0/s overall)
    probe765_integration_test.go:196:   progress 3m0s: indexed revision 5018 (27.9/s overall)
    probe765_integration_test.go:196:   progress 3m30s: indexed revision 5908 (28.1/s overall)
    probe765_integration_test.go:196:   progress 4m0s: indexed revision 6412 (26.7/s overall)
    probe765_integration_test.go:196:   progress 4m30s: indexed revision 7215 (26.7/s overall)
    probe765_integration_test.go:196:   progress 5m0s: indexed revision 8031 (26.8/s overall)
    probe765_integration_test.go:196:   progress 5m30s: indexed revision 8873 (26.9/s overall)
    probe765_integration_test.go:196:   progress 6m0s: indexed revision 9971 (27.7/s overall)
    probe765_integration_test.go:196:   progress 6m30s: indexed revision 11125 (28.5/s overall)
    probe765_integration_test.go:196:   progress 7m0s: indexed revision 12160 (29.0/s overall)
    probe765_integration_test.go:196:   progress 7m30s: indexed revision 13160 (29.2/s overall)
    probe765_integration_test.go:196:   progress 8m0s: indexed revision 13913 (29.0/s overall)
    probe765_integration_test.go:196:   progress 8m30s: indexed revision 14993 (29.4/s overall)
    probe765_integration_test.go:196:   progress 9m0s: indexed revision 16172 (29.9/s overall)
    probe765_integration_test.go:196:   progress 9m30s: indexed revision 17759 (31.2/s overall)
    probe765_integration_test.go:196:   progress 10m0s: indexed revision 19394 (32.3/s overall)
    probe765_integration_test.go:167: warm restart (workers 1): 20000 entities, ready after 10m10.391s (32.8/s, 30.52 ms per entity), consumer-create advisories 80001 (4.00 per indexed entity), JetStream API calls 189432 (9.47 per indexed entity)
--- PASS: TestProbeReconcileCost (713.36s)
ok      github.com/c360studio/semengine/processor/graph-index    713.812s
=== run-40k.log
    probe765_integration_test.go:133: seeded 40000 entities in 877ms
    probe765_integration_test.go:196:   progress 30s: indexed revision 17160 (571.7/s overall)
    probe765_integration_test.go:196:   progress 1m0s: indexed revision 29628 (493.7/s overall)
    probe765_integration_test.go:196:   progress 1m30s: indexed revision 39053 (433.9/s overall)
    probe765_integration_test.go:138: cold build: 40000 entities ready in 1m33.194s (429.2/s), consumer-create advisories 160001 (4.00 per entity)
    probe765_integration_test.go:141:   ENTITY_STATES: 40000 stored messages, 40000 subjects, 49552890 bytes
    probe765_integration_test.go:141:   INCOMING_INDEX: 80000 stored messages, 80000 subjects, 11760000 bytes
    probe765_integration_test.go:141:   OUTGOING_INDEX: 40000 stored messages, 40000 subjects, 10040000 bytes
    probe765_integration_test.go:141:   PREDICATE_INDEX: 280000 stored messages, 280000 subjects, 28320000 bytes
    probe765_integration_test.go:141:   ALIAS_INDEX: 40000 stored messages, 40000 subjects, 7400000 bytes
    probe765_integration_test.go:141:   NAME_INDEX: 40000 stored messages, 40000 subjects, 8388890 bytes
    probe765_integration_test.go:145: breakdown over 1000 entities (every 40th), ms:
    probe765_integration_test.go:145:   | step             |    median |       p90 |       p99 |      mean |
    probe765_integration_test.go:325:   | get              |     0.240 |     0.392 |     1.680 |     0.362 |
    probe765_integration_test.go:325:   | decode+plan      |     0.043 |     0.165 |     0.206 |     0.073 |
    probe765_integration_test.go:325:   | list incoming    |    14.372 |    19.081 |    26.760 |    14.500 |
    probe765_integration_test.go:325:   | list predicate   |     6.928 |     9.443 |    17.787 |     7.036 |
    probe765_integration_test.go:325:   | list alias       |    12.920 |    17.769 |    22.793 |    13.266 |
    probe765_integration_test.go:325:   | list name        |    13.912 |    17.931 |    23.348 |    14.532 |
    probe765_integration_test.go:325:   | four listings    |    48.340 |    62.813 |    77.805 |    49.335 |
    probe765_integration_test.go:325:   | writes           |     0.598 |     1.802 |     2.307 |     0.775 |
    probe765_integration_test.go:325:   | sum of steps     |    49.488 |    64.747 |    81.141 |    50.547 |
    probe765_integration_test.go:325:   | reconcileEntity  |    43.446 |    54.804 |    74.926 |    43.778 |
    probe765_integration_test.go:145: consumer-create advisories: 4.00 per entity for the four timed listings, 4.00 per reconcileEntity
    probe765_integration_test.go:196:   progress 30s: indexed revision 478 (15.9/s overall)
    probe765_integration_test.go:196:   progress 1m0s: indexed revision 997 (16.6/s overall)
    probe765_integration_test.go:196:   progress 1m30s: indexed revision 1509 (16.8/s overall)
    probe765_integration_test.go:196:   progress 2m0s: indexed revision 2034 (16.9/s overall)
    probe765_integration_test.go:196:   progress 2m30s: indexed revision 2593 (17.3/s overall)
    probe765_integration_test.go:196:   progress 3m0s: indexed revision 3140 (17.4/s overall)
    probe765_integration_test.go:196:   progress 3m30s: indexed revision 3735 (17.8/s overall)
    probe765_integration_test.go:196:   progress 4m0s: indexed revision 4273 (17.8/s overall)
    probe765_integration_test.go:196:   progress 4m30s: indexed revision 4756 (17.6/s overall)
    probe765_integration_test.go:196:   progress 5m0s: indexed revision 5235 (17.4/s overall)
    probe765_integration_test.go:167: warm restart (workers 1): 40000 entities, NOT ready, window ended at indexed revision 5235 after 5m0.01s (17.4/s, 57.31 ms per entity), consumer-create advisories 20942 (4.00 per indexed entity), JetStream API calls 56238 (10.74 per indexed entity)
--- PASS: TestProbeReconcileCost (489.31s)
ok      github.com/c360studio/semengine/processor/graph-index    489.761s
=== run-20k-w16.log
    probe765_integration_test.go:133: seeded 20000 entities in 402ms
    probe765_integration_test.go:138: cold build: 20000 entities ready in 25.172s (794.5/s), consumer-create advisories 80001 (4.00 per entity)
    probe765_integration_test.go:141:   ENTITY_STATES: 20000 stored messages, 20000 subjects, 24770890 bytes
    probe765_integration_test.go:141:   INCOMING_INDEX: 40000 stored messages, 40000 subjects, 5880000 bytes
    probe765_integration_test.go:141:   OUTGOING_INDEX: 20000 stored messages, 20000 subjects, 5020000 bytes
    probe765_integration_test.go:141:   PREDICATE_INDEX: 140000 stored messages, 140000 subjects, 14160000 bytes
    probe765_integration_test.go:141:   ALIAS_INDEX: 20000 stored messages, 20000 subjects, 3700000 bytes
    probe765_integration_test.go:141:   NAME_INDEX: 20000 stored messages, 20000 subjects, 4188890 bytes
    probe765_integration_test.go:145: breakdown over 1000 entities (every 20th), ms:
    probe765_integration_test.go:145:   | step             |    median |       p90 |       p99 |      mean |
    probe765_integration_test.go:325:   | get              |     0.252 |     0.358 |     1.645 |     0.355 |
    probe765_integration_test.go:325:   | decode+plan      |     0.085 |     0.165 |     0.195 |     0.090 |
    probe765_integration_test.go:325:   | list incoming    |     7.888 |    11.089 |    15.344 |     8.025 |
    probe765_integration_test.go:325:   | list predicate   |     4.903 |     7.250 |    11.064 |     4.945 |
    probe765_integration_test.go:325:   | list alias       |     6.805 |     8.369 |    12.014 |     6.962 |
    probe765_integration_test.go:325:   | list name        |     8.123 |     9.668 |    13.311 |     8.220 |
    probe765_integration_test.go:325:   | four listings    |    27.853 |    36.215 |    43.033 |    28.153 |
    probe765_integration_test.go:325:   | writes           |     0.601 |     2.013 |     2.260 |     0.833 |
    probe765_integration_test.go:325:   | sum of steps     |    28.938 |    38.045 |    46.193 |    29.432 |
    probe765_integration_test.go:325:   | reconcileEntity  |    24.782 |    32.164 |    39.953 |    25.239 |
    probe765_integration_test.go:145: consumer-create advisories: 4.00 per entity for the four timed listings, 4.00 per reconcileEntity
    probe765_integration_test.go:196:   progress 30s: indexed revision 3443 (114.7/s overall)
    probe765_integration_test.go:196:   progress 1m0s: indexed revision 7620 (127.0/s overall)
    probe765_integration_test.go:196:   progress 1m30s: indexed revision 12611 (140.1/s overall)
    probe765_integration_test.go:196:   progress 2m0s: indexed revision 19516 (162.6/s overall)
    probe765_integration_test.go:167: warm restart (workers 16): 20000 entities, ready after 2m1.189s (165.0/s, 6.06 ms per entity), consumer-create advisories 80001 (4.00 per indexed entity), JetStream API calls 165749 (8.29 per indexed entity)
--- PASS: TestProbeReconcileCost (202.30s)
ok      github.com/c360studio/semengine/processor/graph-index    202.768s
```

### Raw load log (every 30 s: load average, NATS container CPU and memory)

```text
=== load-10k.log
20:04:48 { 2.32 2.24 1.88 } 
20:05:18 { 3.95 2.67 2.05 } 
20:05:49 { 4.76 3.01 2.19 } 
20:06:21 { 4.87 3.21 2.29 } 
20:06:52 { 4.79 3.35 2.38 } 
20:07:23 { 5.50 3.65 2.52 } 
20:07:54 { 6.56 4.09 2.72 } 
20:08:26 { 10.63 5.23 3.17 } 
=== load-20k.log
20:11:05 load={ 4.57 6.15 4.04 } 
20:11:36 load={ 7.86 6.78 4.33 } 4c9dc4aad303 cpu=578.18% mem=215.5MiB / 23.19GiB 
20:12:07 load={ 8.76 7.08 4.52 } 4c9dc4aad303 cpu=113.18% mem=275.2MiB / 23.19GiB 
20:12:39 load={ 8.33 7.17 4.65 } 4c9dc4aad303 cpu=107.71% mem=281.4MiB / 23.19GiB 
20:13:10 load={ 6.46 6.84 4.63 } 4c9dc4aad303 cpu=128.14% mem=278.4MiB / 23.19GiB 
20:13:41 load={ 5.57 6.59 4.61 } 4c9dc4aad303 cpu=126.91% mem=278.6MiB / 23.19GiB 
20:14:12 load={ 4.41 6.21 4.54 } 4c9dc4aad303 cpu=123.04% mem=260.5MiB / 23.19GiB 
20:14:43 load={ 5.30 6.27 4.62 } 4c9dc4aad303 cpu=121.85% mem=247.4MiB / 23.19GiB 
20:15:15 load={ 5.18 6.11 4.63 } 4c9dc4aad303 cpu=114.23% mem=241.8MiB / 23.19GiB 
20:15:46 load={ 5.78 6.22 4.72 } 4c9dc4aad303 cpu=116.46% mem=237.4MiB / 23.19GiB 
20:16:17 load={ 6.11 6.23 4.78 } 4c9dc4aad303 cpu=122.47% mem=241.9MiB / 23.19GiB 
20:16:48 load={ 9.00 6.90 5.07 } 4c9dc4aad303 cpu=112.95% mem=241.6MiB / 23.19GiB 
20:17:20 load={ 7.49 6.76 5.09 } 4c9dc4aad303 cpu=117.00% mem=252.1MiB / 23.19GiB 
20:17:51 load={ 6.75 6.63 5.10 } 4c9dc4aad303 cpu=117.74% mem=245.3MiB / 23.19GiB 
20:18:22 load={ 6.34 6.54 5.12 } 4c9dc4aad303 cpu=113.34% mem=249.2MiB / 23.19GiB 
20:18:53 load={ 5.00 6.21 5.05 } 4c9dc4aad303 cpu=109.68% mem=245.9MiB / 23.19GiB 
20:19:25 load={ 4.63 6.01 5.01 } 4c9dc4aad303 cpu=108.67% mem=248.8MiB / 23.19GiB 
20:19:56 load={ 5.32 6.00 5.05 } 4c9dc4aad303 cpu=112.14% mem=256.4MiB / 23.19GiB 
20:20:27 load={ 4.33 5.70 4.97 } 4c9dc4aad303 cpu=100.44% mem=253.8MiB / 23.19GiB 
20:20:58 load={ 6.95 6.18 5.17 } 4c9dc4aad303 cpu=111.27% mem=254.2MiB / 23.19GiB 
20:21:30 load={ 5.59 5.91 5.11 } 4c9dc4aad303 cpu=111.22% mem=247.1MiB / 23.19GiB 
20:22:01 load={ 5.96 6.03 5.18 } 4c9dc4aad303 cpu=105.99% mem=249.2MiB / 23.19GiB 
20:22:32 load={ 5.23 5.85 5.14 } 4c9dc4aad303 cpu=105.33% mem=247.3MiB / 23.19GiB 
=== load-40k.log
20:23:13 load={ 4.47 5.60 5.09 } 
20:23:44 load={ 9.30 6.62 5.47 } 50c4fb56a76a cpu=527.96% mem=190.3MiB / 23.19GiB 
20:24:15 load={ 10.36 7.18 5.72 } 50c4fb56a76a cpu=679.88% mem=259.4MiB / 23.19GiB 
20:24:47 load={ 12.19 7.94 6.05 } 50c4fb56a76a cpu=736.61% mem=307.2MiB / 23.19GiB 
20:25:18 load={ 8.76 7.53 5.96 } 50c4fb56a76a cpu=110.63% mem=383.9MiB / 23.19GiB 
20:25:49 load={ 6.89 7.22 5.90 } 50c4fb56a76a cpu=110.93% mem=403.3MiB / 23.19GiB 
20:26:20 load={ 6.09 6.92 5.84 } 50c4fb56a76a cpu=109.57% mem=373MiB / 23.19GiB 
20:26:51 load={ 4.82 6.53 5.74 } 50c4fb56a76a cpu=124.64% mem=408MiB / 23.19GiB 
20:27:22 load={ 3.88 6.13 5.62 } 50c4fb56a76a cpu=130.13% mem=403MiB / 23.19GiB 
20:27:53 load={ 4.55 6.05 5.61 } 50c4fb56a76a cpu=113.47% mem=387MiB / 23.19GiB 
20:28:25 load={ 4.00 5.78 5.52 } 50c4fb56a76a cpu=129.37% mem=374.1MiB / 23.19GiB 
20:28:56 load={ 3.56 5.47 5.42 } 50c4fb56a76a cpu=123.42% mem=372.4MiB / 23.19GiB 
20:29:27 load={ 3.91 5.39 5.39 } 50c4fb56a76a cpu=110.81% mem=371.1MiB / 23.19GiB 
20:29:58 load={ 3.74 5.21 5.32 } 50c4fb56a76a cpu=123.39% mem=373MiB / 23.19GiB 
20:30:29 load={ 4.23 5.17 5.31 } 50c4fb56a76a cpu=116.34% mem=377.1MiB / 23.19GiB 
20:31:00 load={ 5.22 5.36 5.37 } 50c4fb56a76a cpu=111.48% mem=364.1MiB / 23.19GiB 
=== load-20k-w16.log
20:31:44 load={ 4.95 5.32 5.36 } 
20:32:15 load={ 7.79 5.92 5.57 } 695e67d384f2 cpu=121.87% mem=272.4MiB / 23.19GiB 
20:32:46 load={ 6.16 5.73 5.52 } 695e67d384f2 cpu=112.47% mem=279.2MiB / 23.19GiB 
20:33:17 load={ 5.41 5.57 5.46 } 695e67d384f2 cpu=703.41% mem=295.6MiB / 23.19GiB 
20:33:49 load={ 6.84 5.92 5.60 } 695e67d384f2 cpu=631.26% mem=310.7MiB / 23.19GiB 
20:34:20 load={ 7.92 6.30 5.75 } 695e67d384f2 cpu=676.97% mem=312.4MiB / 23.19GiB 
20:34:51 load={ 8.74 6.67 5.90 } 695e67d384f2 cpu=731.19% mem=318.9MiB / 23.19GiB 
```
