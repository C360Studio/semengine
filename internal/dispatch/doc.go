// Package dispatch provides KeyedPool, a worker pool that keeps per-key
// order: work items that share a key are processed one at a time, in
// the order they were submitted, while items with different keys run
// concurrently.
//
// # How it works
//
// KeyedPool has a fixed number of lanes. Each lane is one goroutine
// reading its own bounded queue. SubmitBlocking takes the item's key
// beside the item, and the item goes to lane fnv1a(key) % Lanes, so
// every item with the same key lands on the same lane and is processed
// in submit order. Process receives the lane index, so a caller can
// keep per-lane state (for example an applied-sequence guard) without
// locking: only that lane's goroutine touches it. Process returns
// nothing: it disposes of its own failures. A panic in Process is
// recovered; the lane keeps running and the optional OnPanic callback
// is called with the item, so the caller can dispose of it (for example
// Nak the message it came from).
//
// Use it when two items with the same key must never be processed out
// of order or at the same time, as graph-ingest needs for one entity's
// updates (ADR-072, in SemStreams).
//
//	pool, err := dispatch.NewKeyedPool(ctx, dispatch.KeyedConfig[ingestWork]{
//	    Lanes:           lanes,
//	    QueueDepth:      256,
//	    Name:            "graph_ingest",
//	    Process:         process, // func(ctx, lane, work)
//	    OnPanic:         func(w ingestWork, _ any) { _ = w.msg.Nak() },
//	    MetricsRegistry: registry, // optional
//	    Logger:          logger,   // optional
//	})
//	err = pool.SubmitBlocking(submitCtx, work.entityID, work)
//
// SubmitBlocking applies backpressure: when the item's lane queue is
// full, the call waits until the lane has room or its context ends.
//
// # Contexts and stopping
//
// The context given to NewKeyedPool reaches only Process. Ending it
// stops nothing: the lanes keep running, and items queued or accepted
// afterwards still reach Process, which sees the ended context. Ending
// it is the owner's way to make the remaining Process calls finish
// fast.
//
// Shutdown(ctx) is the only stop. A SubmitBlocking call that begins
// after the first Shutdown returns ErrStopped; one already waiting on a
// full lane may still be accepted, and is then processed. Every lane
// finishes its queue and exits, and Shutdown returns nil once they
// have, even if ctx has ended by then. If ctx ends first, Shutdown
// returns ctx.Err() and the lanes keep draining; call Shutdown again to
// wait for them. Shutdown has no timeout of its own: a context with no
// deadline waits as long as Process takes. A nil context is refused
// with an error. A pool nobody shuts down keeps its lanes until the
// process ends.
//
// A producer parked in SubmitBlocking on a full lane holds Shutdown up
// until the lane takes its item. On shutdown, cancel the context passed
// to SubmitBlocking first, then call Shutdown.
//
// The pool runs one goroutine per lane, plus one that the first
// Shutdown starts to close the queues and join the lanes. Shutdown
// returning nil means all of them have exited.
//
// # Metrics
//
// With KeyedConfig.MetricsRegistry set, the pool registers six
// collectors labelled pool=<Name> through metric.RegisterOrGet:
// dispatch_queue_wait_seconds, dispatch_processing_duration_seconds,
// dispatch_queue_depth, dispatch_inflight, dispatch_submitted_total and
// dispatch_completed_total. Two pools with the same Name on one
// registry share these collectors. dispatch_queue_depth counts the
// items accepted and not yet passed to Process: it rises when
// SubmitBlocking accepts an item and falls when a lane picks it up, so
// it sums the waiting items of every pool of that Name. A registration
// the registry refuses makes NewKeyedPool return the error. With no
// registry the pool keeps no metrics.
package dispatch
