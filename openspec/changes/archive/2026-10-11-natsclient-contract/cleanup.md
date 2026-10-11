# Cleanup: the 46 false or unmeasured statements, and 2 this change makes false

This table rests on inventory revision 3 (`/tmp/pr177/inventory.md`). The rows are its 16 contradicted and 30
unmeasured rows on `main`; line numbers are `main`'s (`0e13a73`).

How to read the columns:

- **Now.** What the line says today.
- **After this PR.** What the line says once this pull request has merged, and why.
- **Backed by.** Each new statement rests on one of:
  - a probe: P10 to P25, or the reviewer's p15b;
  - one of the new tests T1 to T9, defined in `design.md` and `tasks.md`;
  - a test already in the tree.
- **#93.** A file that PR #93 changes is listed with #93's hunks (from `diff` of `0e13a73` against `f3d5ae56`).
  - `natsclient/README.md`: lines 251-256 and 271-276.
  - `natsclient/kv.go`: lines 6-12, 320-332, 374-381, 735-771 and 774-777.
  - `natsclient/storage_inventory.go`: lines 98-104 and 194-200.

  No other file below is changed by #93. The spec deltas use headings #93's deltas do not touch.

## The 16 shown false

| Row | Where | Now | After this PR | Backed by | #93 |
| --- | --- | --- | --- | --- | --- |
| M2 | `transport-client` spec `:8` (Purpose) | `Close` stays within the caller's deadline | true for a connection the client dialled. The archive commit edits the Purpose to say so, because a `SetConnection` connection is not cut | P17, P23, T1, T8 | spec |
| M6 | spec `:57-58` | Close returns within its deadline | true with the fix; the requirement adds that Close closes the socket when its context ends | P17, T1 | spec |
| M8 | spec `:64-66` (scenario) | Close returns within the deadline | true with the fix; a new scenario covers a held write | P17, T1 | spec |
| M11 | spec `:90-91` | each Close returns within its own context | true with the fix (the socket close, and no natsclient lock held across a nats.go call); unchanged | P17, P21, T1, T2 | spec |
| M19 | spec `:209-212` | the setup context bounds setup | cut: the context is used for setup, and a held write holds setup | P13, P14 | spec |
| M36 | `README.md:3`, `:9-11` | automatic reconnection; context propagation throughout all operations | names what the context ends (the wait for the server's answer) and what it does not end (a held write; ADR-112) | P13, P14, P15 | no overlap |
| M41 | `README.md:173-178` | `KVOptions.Timeout` is the operation timeout | it bounds the wait for the server's answer, not a held write | P14 | no overlap |
| M42 | `README.md:220-221` | every I/O operation takes a context for cancellation and timeout | every call that waits for the server takes a context, which ends that wait; `RTT` takes none | P13, P18 | no overlap |
| M46 | `README.md:340-352` | example: `Publish` with automatic reconnection, and a branch on `ErrCircuitOpen` | `Publish` returns `ErrNotConnected` at once while reconnecting, and has no circuit check, so the branch goes | P15 | no overlap |
| M47 | `doc.go:4-6` | exponential backoff for reconnection; context propagation throughout | as M36 and M57 | P13, P14 | unchanged by #93 |
| M59 | `doc.go:448-449` | as M42 | as M42 | P13, P18 | unchanged |
| M61 | `client.go:884-902` (`Close`) | returns `ctx.Err()` at once when ctx ends | true with the fix; adds that Close closes the socket and ends a reconnect dial, so a held write ends | P17, P23, T1, T8 | unchanged |
| M72 | `request.go:174-177` (`Request`) | the timeout controls how long to wait for a response | adds: it does not end a held write | P13, P14 | unchanged |
| M74 | `errors.go:216-217`, `:251-253` | no responders and timeout classify as transient through `IsTransient` | the timeout (`context.DeadlineExceeded`) stays, true as today. "No responders ... transient" is deleted: it is false on `main`, and its class is not promised until #93 settles raw nats.go errors. The comment says no responders matches `nats.ErrNoResponders` | P10 | unchanged |
| M76 | `kv.go:29` | `Timeout` is the operation timeout | as M41 | P14 | no overlap (`:6-12` is #93's) |
| M86 | `stream.go:427-437`, `:467-474`, `:723-730` | ctx bounds setup only | as M19 | P13 | unchanged |

## The 30 unmeasured

| Row | Where | Now | After this PR | Backed by | #93 |
| --- | --- | --- | --- | --- | --- |
| M9 | spec `:85-88` | the async publish error handler is joined only as far as nats.go lets the client see it | kept: it declares a residual, which promises nothing | n/a | spec |
| M16 | spec `:177-178` | a delivery after the recorded end does not run the handler | kept: natsclient's own guard | fake-driven test `client_close_final_test.go:1186` | spec |
| M17 | spec `:179-181` | redelivered with acks; lost with `AckPolicy: none` or core | kept. "Redelivered" cites the test; "lost" is a declared loss, which promises nothing | `delivery_settlement_integration_test.go:140`, `:237` | spec |
| M21 | spec `:268-274` | the report view changes at the watch's sync marker | kept: natsclient's own logic; the marker is nats.go's watcher API | fake-driven `storage_report_resync_test.go:19` | spec |
| M32 | `nats-fixture` spec `:98-101` | Stop returns `ctx.Err()` when ctx ends first | kept: the fixture's broker always reads, so a held write does not arise | the fixture's own Stop tests | spec |
| M35 | `nats-fixture` spec `:220-227` | the broker refuses an oversize publish | cut: nats.go refuses it with `nats.ErrMaxPayload` before it sends | `fixture_integration_test.go:1029` | #93's delta only ADDs |
| M39 | `README.md:49-52` | panic is Nak'd and redelivered; an unsettled message is redelivered at AckWait | cites the tests | `delivery_settlement_integration_test.go:140` (M18) | no overlap |
| M50 | `doc.go:75-80` | what a clustered node says before the meta assignment | marked not measured: no test here runs a cluster | none | unchanged |
| M51 | `doc.go:82-88` | consumer setups wait out the propagation window | kept for natsclient's own logic; the cluster premise is marked as M50 | fake-driven `stream_visibility_test.go` | unchanged |
| M52 | `doc.go:108-110` | 0 and -1 both mean unlimited | `MaxBytes` 0 and -1 both mean unlimited; the `MaxAge -1` claim is deleted | P16 | unchanged |
| M57 | `doc.go:352-353` | reconnect uses exponential backoff, at most one minute | corrected: reconnects wait `ReconnectWait` (2 s, natsclient's setting); the one-minute exponential backoff is the circuit breaker's, for `Connect` | `client.go:345`, `:349` (natsclient's own values) | unchanged |
| M62 | `client.go:1214-1220` (`Subscription.Drain`) | the join, and the discard of queued messages | kept for natsclient's join; the discard is reworded "may be discarded" | fake-driven `subscription_test.go:146` | unchanged |
| M67 | `client.go:1718-1733` (`PublishBatchToStream`) | waits for all acks, bounded by ctx | adds: a held write holds it (ADR-112) | `publish_async_integration_test.go:79`, `:308`; P13 | unchanged |
| M68 | `client.go:1799-1805` (`GetStream`) | a clustered node's answer | marked as M50 | none | unchanged |
| M69 | `client.go:1954-1956` (`WaitForBucket`) | retries until the timeout or the context ends | adds: a held write holds an attempt | P13 (`getbucket`) | unchanged |
| M71 | `request.go:81-86` (readiness budget) | bounds the total wall-clock | adds: a held write holds an attempt | P13 | unchanged |
| M75 | `errors.go:297-304` (`IsNoResponders`) | no responders against a timeout depends on the server's configuration | the default configuration answers no responders at once (P15); other configurations are marked not promised | P15 | unchanged |
| M79 | `kv.go:323` | on any error the revision is 0 and nothing committed | not edited here: #93's N3 rewrite is the truth | P15, p15b | **overlaps** `:320-332` (#93 rewrites it) |
| M81 | `kv.go:560-561`, `:583-584` | a returned native watcher owns delivery until `Updates` closes | cites the test | `kv_watcher_ownership_integration_test.go:48` | no overlap |
| M85 | `stream.go:307-326` (`ErrStreamNotVisible`) | produced only after the budget | kept for natsclient's own logic; the cluster premise is marked as M50 | fake-driven `stream_visibility_test.go` | unchanged |
| M87 | `delivery_settlement.go:273-276` | a bare Nak, which JetStream redelivers as fast as it can | "as fast as it can" is deleted (filler) | `delivery_settlement_integration_test.go:237` | unchanged |
| M90 | `client.go:759-760` | nats.go fails `jetstream.New` only when an option does | marked not promised, with the read of nats.go v1.54.0 as its reason; the error is still returned | none | unchanged |
| M92 | `client.go:1053-1054`, `:1066-1067` | `Consumer.Info`'s unsynchronized write is still present upstream | marked not promised, with the read of v1.54.0 as its reason; the guard stays | none | unchanged |
| M98 | `docs/adr/110-write-seam.md:192` | a watcher sees one revision per write | left to ADR-109 (owner Q6); the note is on #110 (comment 6109839042) | none | n/a |
| M99 | `docs/adr/111-…:104-105`, `:116-117` | redelivery after a crash or restart | left to the change that relies on it (owner Q6); ADR-112 marks a broker restart not promised | none | n/a |
| M100 | `client.go:516-519` | the server's typed PubAck says the stream reached a ceiling | corrected: the server sends one code for every store failure (10077) and names the limit only in its description, which nats.go has no code for; natsclient reads that description | `stream_capacity_circuit_integration_test.go:14` | unchanged |
| M101 | `storage_inventory.go:21-24` | the server moves offline streams; the Go client drops them | marked not promised, with the read of nats-server and nats.go as its reason | none | no overlap |
| M102 | `storage_inventory.go:421-422` | the server does not exclude offline streams from the name listing | as M101 | none | no overlap |
| M103 | `storage_inventory.go:455-460` | the page offset and the server's cursor diverge | as M101 | none | no overlap |
| M104 | `stream.go:100-103` | `Duplicates` must be at most `MaxAge` or the server rejects creation | the server's rule is marked not measured; natsclient's clamp is natsclient's own | read | unchanged |

## The 2 this change makes false

| Row | Where | Now | After this PR | Backed by | #93 |
| --- | --- | --- | --- | --- | --- |
| M105 | `README.md:249-250` | prints `ErrCircuitOpen = errors.New("circuit breaker is open")` and `ErrNotConnected = errors.New("not connected to NATS")` | prints the new definitions: `errs.ErrCircuitOpen`, and a wrap of `errs.ErrConnectionLost` | T7 | no overlap: #93 inserts after `:253`, three unchanged lines away |
| M106 | `doc.go:247-248` | the same two definitions | as M105 | T7 | unchanged |

**Overlap with #93.** One row, M79 (`kv.go:323`), lies in a #93 hunk. It is left to #93. Every other edit lies
outside #93's hunks or in a file #93 does not change.

Two more overlaps are outside this table:

- `docs/admission-ledger.yaml`: both pull requests append to the end of the natsclient row's contract text, at
  `main:1695`. The second to merge keeps both texts.
- `docs/repository-map.md`: `#176` has merged, so there is no conflict.

**Tests whose evidence is text.** `request_integration_test.go:70-74` and `:429-434` compare error text, so they
measure neither answer (M75). Task 4.2 changes them to `errors.Is(err, nats.ErrNoResponders)`.
