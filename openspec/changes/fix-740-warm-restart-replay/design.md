# Design: fix-740-warm-restart-replay

## Context

Facts from `inventory.md` (base 96d047a6, INVENTORY PASS at c3f49033, findings folded at d51b8aa7), cited by section.
Library facts are from probe740 (source, commands, versions and output posted on PR #747): nats-server 2.15.0 (the
`natsfixture/nats-image` pin), nats.go v1.55.0, 8 KiB values, a macOS client through the Docker Desktop port
forward (the reporter's setup) and, where named, a client inside the Docker network. 2.14.10 was not probed.

- Store-sized replays at boot, all on the one Client (§ 1.1, § 1.4): R1 to R4 (followers), R5 (websocket view), R6
  (graph-ingest's sweep, inside `Start`), graph-query's `COMMUNITY_INDEX` view, and at tier 1 three `EMBEDDING_INDEX`
  watches. Components start concurrently (`service/component_manager_lifecycle.go:74-85`), so the replays overlap.
- A lost connection fails `Start`: Subscribe refuses at once while reconnecting (`natsclient/client.go:1176`).
- **P1, the failure.** 230 MB, five watches on one connection: 10 server closes in 58.6 s, each logged
  `Slow Consumer Detected: MaxPending of 67108864 Exceeded` (the reporter's line); `pending_bytes` peaked at
  67,107,510. One watch: no close, peak 32.6 MB. Two: 3 closes; three: 4. Inside the Docker network, five: 4 closes.
- **P2, the size.** Five watches on one connection: 8 and 17 MB, no close; 33 MB, 1; 66 MB, 2; 132 MB, 5.
- **P3, the recovery.** In every run with closes, each watch delivered every key once, in revision order, with one
  end-of-snapshot marker, and its `Updates` channel stayed open. So a watch resumes where it was; the issue's "every
  replay starts again" is not what 2.15.0 and v1.55.0 do, and the follower's lost-feed latch
  (`graph/follower/follower.go:254`) was not reached.
- **P4, a stalled reader.** A watch whose reader never reads stopped at 1,393 entries (about 11.5 MB) for 20 s: no
  close, no client-side drop. One run whose reader blocked across several closes (a `Get` with no deadline) saw
  nats.go report `slow consumer, messages dropped` 19 times and did not finish in 120 s. Not promised either way.
- **P5, apart.** Five watches on five connections: no close, 7.7 s, peak 42.1 MB on any connection; ten on ten: no
  close, peaks 43.7 and 47.0 MB. Five one after another on one connection: no close, 6.3 s. graph-index's shape (a
  `Get` per entry, 100 µs per entry): one connection, 1 close in 27.6 s; five connections, none in 19.7 s.
- **P6, `MetaOnly`.** Five header-only watches at 230 MB: no close, 2.3 s.

Why one watch stays under the limit is the server's 32 MiB flow-control window (`server/consumer.go:607` at 2.15.3,
§ 1.6) against the 64 MiB default; both are server constants, measured here, not promised.

## Goals / Non-Goals

Goal: a warm restart starts on default broker settings at any store size, with nothing for an operator to predict.
Non-goals: as in the proposal.

## Decisions

**D1. How the boot replay is bounded.** Adopter seam for every option: today a product must know to raise
`max_pending` with its store, and if it does not, a warm restart fails `Start` with `not connected to NATS`, which
names no cause (§ adopter seam).

- **(B, recommended) One store-sized watch per connection.** Each such watch runs on a connection of its own, opened
  when the watch starts and closed before the code that started it returns. Measured: P5. Cost: one connection per
  watch (tier 0: five while running plus one in graph-ingest's `Start`; tier 1: eight plus two), a `natsclient`
  constructor, and seven call sites. Forecloses nothing; the readers stay independent. Adopter: nothing to set; an
  operator with a connection limit must allow the count (`docs/boot.md`). Needs owner question 2.
- **(A) One replay at a time on the shared connection.** A process-wide turn, held from a watch's start to its
  end-of-snapshot marker. Measured: P5 (6.3 s). Cost: shared state across six packages; graph-ingest's `Start`
  waits for every replay ahead of it. Hazard: the embedding worker's snapshot ends only after its pool has taken
  every pending record (`graph/embedding/worker.go:347-371`), and the pool waits on the provider. A provider that is
  slow at boot would then hold back graph-index's replay: tier 0 readiness waiting on tier 1. Leaving the worker out
  puts two replays on one connection again (P1: 3 closes at 230 MB).
- **(C) Retry a transient `Start` failure only.** P3 says the replays finish through the drops. Cost: the drops
  stay (10 at 230 MB), boot is 58 s instead of about 8, every subscription and request in the process breaks on
  each drop, and P4 shows a stalled reader can lose messages across them. Not a bound.
- **(D) Document `max_pending`.** No code. Forecloses the owner's rule that an operator not predict a buffer; P2
  puts the threshold between 17 and 33 MB.
- **(E) Pull-based ordered consumers with a byte limit.** Not probed. Rewrites six readers' KV decoding and needs
  a new consumer entry point (#422). Too large for alpha.8.
- **(F) `MetaOnly` for R1 and R4,** which read the entity again anyway (§ Facts asked for). P6 shows header-only
  watches cost nothing, but R2, R3, R5 and R6 need values, so four full replays remain (P1). Changes `Entry.Bytes`
  (`entity-state-follower` spec `:193`). Not recommended.

**D2. Where the extra connection comes from (under B).**

- **(i, recommended)** `natsclient.Client.NewSibling()` returns an unconnected Client with the same URLs,
  credentials, name, timeouts and reconnect settings; each reader connects it, watches, and closes it. No metrics
  registry (two Clients on one registry overwrite each other's series, `docs/admission-ledger.yaml:1175-1178`) and
  no connection-lost callback (the product's main Client keeps the one exit policy, `docs/boot.md:56-59`). Callers:
  `graph/follower`, `output/websocket`, `processor/graph-query`, `processor/graph-ingest`, `graph/embedding`.
  No product code changes; `follower.Config` and `graphview` are unchanged.
- **(ii)** A connection factory in `component.Dependencies`. Cost: every product `main` must fill it, and a missing
  one silently brings the bug back. Rejected.

The follower connects in `Run`, inside the startup wait it already uses for the bucket
(`graph/follower/follower.go:163`); a connection it cannot open there is a lost feed, as a watch that cannot start
is today (`:236-241`). Status reads stay on `Config.Client`.

**D3. graph-ingest's sweep (R6) stays inside `Start`** on its own connection, closed before `Start` returns
(ADR-113 `docs/adr/113-start-context-bounds-start.md:118`, unchanged). Owner question 4.

**D4. ADR-108.** Decision 7 (one follower per producer) is unchanged. Decision 4's list of what the follower owns
gains "the connection its watch runs on"; an amendment line, for the owner (question 6).

Invariant: on every connection the process opens, at most one whole-bucket watch is open at a time (`composed-stack`
delta). Proving test: `TestSmoke_WarmRestartOverALargeStore`, shown failing on the base commit.

## Risks / Trade-offs

- The margin is measured, not promised: one watch peaked at 47 MB against 64 MiB. A server that raises its window
  or an operator who lowers `max_pending` below about 50 MB can trip one watch.
- Two simultaneous requests to the message logger's KV watch on `ENTITY_STATES` put two full watches on the shared
  connection (`service/message_logger_kv_watch.go:138`). Request-driven, not boot; filed as a follow-up.
- A product's own full watch on its main Client is not covered; `docs/boot.md` says to open it on a sibling.

## Migration Plan

None. SemSource may drop its `max_pending` setting after alpha.8.

## Open Questions (owner)

1. For alpha.8: bound the replay, retry a failed `Start`, or both? Recommended: bound only (D1 B).
2. May a process hold more than one NATS connection (D1 B)? Recommended: yes. If no, D1 A, with its hazard.
3. Is a `docs/boot.md` note on `max_pending` alone enough? Recommended: no (P2).
4. Does graph-ingest's sweep stay inside `Start`? Recommended: yes (D3).
5. What proves #740 fixed? Recommended: `TestSmoke_WarmRestartOverALargeStore` gates the merge; the #11 tier 1
   scorecard run on the tag stays the release evidence.
6. ADR-108: amend Decision 4 by one clause (D4)? Recommended: yes.
