# Probe: one entity's reconcile cost on a warm restart (#765, step 2)

Measured on `origin/main` 3d823c82 (keys-only follower watch merged) plus the probe test
`processor/graph-index/probe765_integration_test.go`. No production code changed. This branch never merges.

## Host

- Apple M3 Pro, 12 cores, 36 GiB, macOS 26.5.2, Go 1.26.11 darwin/arm64.
- NATS: the package's shared fixture, `nats:2.15.0-alpine@sha256:ac8f88a6…` (nats-server 2.15.0), in Docker Desktop
  29.9.3 (VM: 12 CPUs, 23.2 GiB). File storage inside the VM. Client nats.go v1.55.0.
- Load average during the runs: 4.6 to 12 (1-minute); other sessions were running tests. Before the first run it was
  2.3; between the 10k and 20k runs it reached 16 from another session's tests, and the 20k run waited until it fell
  under 5. Per-30-second load and NATS container CPU are under "Raw load log" below.

## Method

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

## Commands

```bash
PROBE_765_SIZES=10000 go test -tags integration -count=1 -timeout 60m -run TestProbeReconcileCost -v ./processor/graph-index/
PROBE_765_SIZES=20000 go test -tags integration -count=1 -timeout 60m -run TestProbeReconcileCost -v ./processor/graph-index/
PROBE_765_SIZES=40000 PROBE_765_WARM_WINDOW=5m go test -tags integration -count=1 -timeout 60m -run TestProbeReconcileCost -v ./processor/graph-index/
PROBE_765_SIZES=20000 PROBE_765_WARM_WORKERS=16 PROBE_765_WARM_WINDOW=5m go test -tags integration -count=1 -timeout 60m -run TestProbeReconcileCost -v ./processor/graph-index/
```

Without the environment variables the test runs once at 200 entities (about 4 s).

## Results

### Warm restart, 1 lane (measured)

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

### Per-entity breakdown, ms (measured, 1,000 entities each, one at a time)

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

### Per-entity cost against store size (measured)

| Store | mean of one listing | four listings, mean | reconcileEntity, mean | warm wall per entity |
| --- | --- | --- | --- | --- |
| 200 | 0.59 | 2.36 | 2.88 | 2.89 |
| 10,000 | 4.83 | 19.32 | 18.34 | 17.69 |
| 20,000 | 9.04 / 7.04 | 36.17 / 28.15 | 31.95 / 25.24 | 30.52 |
| 40,000 | 12.33 | 49.34 | 43.78 | 57.31 (first 5,235) |

### Lanes (measured, one run)

| Store | Lanes | Warm time to ready | Entities/s | NATS container CPU |
| --- | --- | --- | --- | --- |
| 20,000 | 1 | 610.4 s | 32.8 | about 105 to 113 % |
| 20,000 | 16 | 121.2 s | 165.0 | about 630 to 730 % |

Cold builds, 16 lanes, empty indexes at start: 10k 10.3 s (973/s), 20k 32.4 s and 25.2 s (617/s, 795/s), 40k 93.2 s
(429/s).

## Reading

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

## Skipped

- A full 40,000 warm restart: projected at about 38 minutes, over the time-box; the 40k figure is the first five
  minutes of the replay.
- Per-step timing inside the running lanes during the replay itself: no production hook; the breakdown is a separate
  pass of the same calls on the same component and server, one at a time.
- The 105k watermark plateau, SemSource's own settings, and a run on the Mac mini.
- Each size ran once (20k twice for the breakdown); no repeat runs to give a spread across runs.

## Raw test output (testcontainers and bucket lines removed)

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

## Raw load log (every 30 s: load average, NATS container CPU and memory)

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
