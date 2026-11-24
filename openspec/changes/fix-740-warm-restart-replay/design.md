# Design: fix-740-warm-restart-replay

## Context

Facts from `inventory.md` (base 96d047a6, INVENTORY PASS at c3f49033, findings folded at d51b8aa7), cited by section.
Library facts are from probe740 (posted on PR #747; `probes.sh` re-runs each P-number into `out/<P-number>.txt`):
nats-server 2.15.0 (the `natsfixture/nats-image` pin, started as `natsfixture` starts it), nats.go v1.55.0, 8 KiB
values, a macOS client through the Docker Desktop port forward (the reporter's setup) or, in-network, a container on
the server's Docker network. `pending_bytes` is sampled every 250 ms, so peaks are lower bounds. 2.14.10 was not
probed. A range spans round 1 (output not saved), round 2 (`out/`) and the reviewer's runs.

- Store-sized replays at boot, all on the one Client (§ 1.1, § 1.4): R1 to R4 (followers), R5 (websocket view), R6
  (graph-ingest's sweep, inside `Start`), graph-query's `COMMUNITY_INDEX` view, and at tier 1 three `EMBEDDING_INDEX`
  watches. Components start concurrently (`service/component_manager_lifecycle.go:74-85`), so the replays overlap.
- A lost connection fails `Start`: Subscribe refuses at once while reconnecting (`natsclient/client.go:1176`).
- **P1, the failure.** 230 MB, five watches on one connection (P1a): 7 to 10 server closes in 43 to 59 s, each
  logged `Slow Consumer Detected: MaxPending of 67108864 Exceeded` (the reporter's line); pending peaked at 67.1 MB.
  One watch (P1b): no close, peak 32.6 to 33.7 MB. Two (P1c): 3 closes; three (P1d): 4 to 6. In-network (P1e): 4.
- **P2, the size.** Five watches on one connection, port-forwarded: 8 and 17 MB, no close; 33 MB, 1; 66 MB, 2 to 3;
  132 MB, 5. In-network (P2net): 8 and 17 MB, none; 33 MB, 1; 66 MB, 1; 132 MB, 2.
- **P3, the recovery** (read from P1a). Each watch delivered every key once, in revision order, with one
  end-of-snapshot marker, and its `Updates` channel stayed open. A watch resumes where it was; the issue's "every
  replay starts again" is not what 2.15.0 and v1.55.0 do, and the follower's lost-feed latch
  (`graph/follower/follower.go:254`) was not reached.
- **P4, a stalled reader.** P4a: a reader that never reads holds the server at 1,393 to 5,948 entries for 20 s, with
  no close and no client-side drop. P4b: one reader blocked in a `Get` whose deadline was the run's 120 s, sent
  across a close, read 5 entries in 120 s while nats.go reported `slow consumer, messages dropped` 19 times. Under
  a context with no deadline nats.go applies its own 5 s (`jetstream/jetstream.go:461`, `:1277-1282`, source read),
  which is graph-index's case (`processor/graph-index/index.go:211`).
- **P5, apart.** P5a, five watches on five connections: no close, 7.7 to 10.2 s, peak 42.1 MB on any connection.
  P5b, ten on ten: none, peaks 42.1 to 47.0 MB. P5c, five one after another on one connection: none, 6.3 to 9.4 s.
  P5d and P5e, graph-index's shape (a `Get` per entry with a 2 s deadline, 100 µs per entry): one connection, 1 close
  in 19.5 to 27.6 s; five connections, none in 17.8 to 19.7 s.
- **P6, `MetaOnly`.** Five header-only watches at 230 MB: no close, 2.3 to 2.4 s.
- **P7, what a test can see.** On a fresh server, P7run's 10 closes raised the account's `slow_consumers` in
  `$SYS.REQ.ACCOUNT.PING.STATZ` from 0 to 10, as `/varz` did. `$SYS.REQ.ACCOUNT.PING.CONNZ` listed no closed
  connection (`{"state":1}`: total 0) while the HTTP monitor listed all 10 as `Slow Consumer (Pending Bytes)`.

Why one watch stays under the limit is the server's 32 MiB flow-control window (`server/consumer.go:607` at 2.15.3,
§ 1.6) against the 64 MiB default; both are server constants, measured here, not promised.

## Goals / Non-Goals

Goal: a warm restart starts on default broker settings at any size, nothing for an operator to predict. Non-goals:
the proposal's.

## Decisions

**D1. How the boot replay is bounded: one store-sized watch per connection** (rulings 1, 2; measured: P5). Each
boot-time whole-bucket reader opens a connection when its watch starts and closes it before the code that started it
returns. Not chosen: one replay at a time on the shared connection (a provider stall at boot would hold
graph-index's replay behind the embedding Worker's, `graph/embedding/worker.go:347-371`); a retry of a failed
`Start` alone (P3: the drops stay); a `max_pending` note (P2); pull consumers with a byte limit (not probed, six
readers rewritten); `MetaOnly` for R1 and R4 (P6, but four full replays remain). Adopter seam: a product sets
nothing; an operator with a connection limit allows the count `docs/boot.md` gives.

**D2. The sibling.** `natsclient.Client.NewSibling() (*Client, error)` returns an unconnected Client with the
parent's URLs, credentials, timeouts, reconnect settings and name; every sibling shares the name, so the server lists
a stack's connections under one name. It has no metrics (two Clients on one registry overwrite each other's series,
`docs/admission-ledger.yaml:1175-1178`): no collector, no poller, and a permission refusal on it is logged but not
counted (`natsclient/client.go:1903-1906`). It has no connection-lost callback, so the product's main Client keeps the
one exit policy (`docs/boot.md:56-59`). The parent's `Close` does not close a sibling; its opener does. Considered:
a connection factory in `component.Dependencies`, which every product `main` would have to fill.

**D3. Who opens a sibling, and the exported surface.** No exported signature changes but `NewSibling`.

- The follower opens its own in `Run`, covering all four followers; `follower.Config` is unchanged. `Run` waits for
  it with `Config.StartupAttempts` and `Config.StartupInterval`, a wait of its own: `Open`'s wait at
  `graph/follower/follower.go:163` is for the bucket and is over by then. A sibling that does not connect in that
  wait is a lost feed, as a watch that cannot start is (`:236-241`). Status reads stay on `Config.Client`.
- websocket's `runView` (`output/websocket/state.go:113`) and graph-query's `runCommunityView`
  (`processor/graph-query/community.go:82-94`) open the reader on a sibling; `graphview` is unchanged.
- graph-ingest's boot sweep (`processor/graph-ingest/poison_inventory.go:272`) runs on a sibling inside `Start`
  (ruling 4; ADR-113 `:118` unchanged).
- graph-embedding: `embedding.Storage` is two bucket handles (`graph/embedding/storage.go:173-176`), so the
  component builds, for the failed-record scan (`processor/graph-embedding/component.go:346`), the Worker (`:389`)
  and the vector cache (`:420`), a `NewStorage` whose `EMBEDDING_INDEX` handle is on that user's sibling.
  `NewStorage`, `NewWorker`, `NewVectorCache` and `ScanFailed` keep their signatures, and their unit tests over
  in-memory buckets (`graph/embedding/worker_test.go:161`) keep running without a server. Searched read-only for
  callers outside this repo: none in semsource f769f0b, semteams c8571b34, semconnect dff1265, semboids 37dbdb0
  (semboids builds its own `graphview` on SemStreams, `internal/api/graphstream_views.go:157`).

**D4. What a stop looks like.** For every reader above, a clean stop returns what it returns today (the follower's
`Run` nil, so graph-index logs no Warn, `processor/graph-index/component.go:273-275`), logs nothing at Error from the
close, and leaves no natsclient goroutine running; a failed `Start` returns its cause and leaves none either. Today a
`Close` under an ended context returns that context's error without joining (`natsclient/client.go:697-702`) and
logs `Context cancelled during drain, force closing` at Error (`:889-895`), so the close context matters; the
developer picks it under the contract's rule on terminal, bounded detachment and records it in the task notes. Each
task's tests check the three things above (`natsfixture.AwaitNoStacks` for the goroutines).

**D5. ADR-108.** Decision 7 (one follower per producer) is unchanged; Decision 4's list of what the follower owns
gains "the connection its watch runs on" (ruling 6).

Invariant: no connection carries two of the named watches (`composed-stack` delta). Proving test:
`TestSmoke_WarmRestartOverALargeStore`, red on Linux CI before the fix (ruling 5).

## Risks / Trade-offs

- The margin is measured, not promised, and from 250 ms samples, so every peak is a lower bound: one watch on its
  own connection peaked at 47.0 MB (the reviewer saw 33.7 and 42.08). A server that raises its window, or an
  operator who lowers `max_pending` below about 50 MB, can trip one watch.
- In-network runs closed fewer times than port-forwarded ones (P1e, P2), so 66 MB may not fail on Linux CI; task
  2.1 requires the red CI run and raises the size until it has one.
- Watches outside the named list stay on the main connection: two simultaneous message-logger KV watches on
  `ENTITY_STATES` (`service/message_logger_kv_watch.go:138`) can trip it (P1c; filed as a follow-up), and a
  product's own whole-bucket watch should run on a sibling (`docs/boot.md`).

## Migration Plan

None; SemSource may drop its `max_pending` setting after alpha.8.

## Rulings

The owner ruled on 2026-11-24 (#740, comment 7136495793), accepting each recommendation of the design at e6367899:
(1) bound the boot replay only, no retry of a failed `Start`; (2) a process may hold one NATS connection per
whole-bucket watch; (3) a `max_pending` note alone is not enough; (4) graph-ingest's boot sweep stays inside
`Start`, on its own connection; (5) `TestSmoke_WarmRestartOverALargeStore` (at least 66 MB, default `max_pending`)
gates the merge, and the #11 tier 1 scorecard run on the tag is the release evidence; (6) ADR-108 Decision 4 gains
"the connection its watch runs on". `Approved-by:` follows the design review.
