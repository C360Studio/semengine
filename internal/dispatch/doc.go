// Package dispatch provides KeyedPool, a worker pool that keeps per-key
// order: work items that share a key are processed one at a time, in
// the order they were submitted, while items with different keys run
// concurrently.
//
// # How it works
//
// KeyedPool has a fixed number of lanes. Each lane is one goroutine
// reading its own bounded queue. A work item goes to lane
// fnv1a(KeyOf(work)) % Lanes, so every item with the same key lands on
// the same lane and is processed in submit order. Process receives the
// lane index, so a caller can keep per-lane state (for example an
// applied-sequence guard) without locking: only that lane's goroutine
// touches it. A panic in Process is recovered; the lane keeps running
// and the optional OnPanic callback is called with the item, so the
// caller can dispose of it (for example Nak the message it came from).
//
// Use it when two items with the same key must never be processed out
// of order or at the same time, as graph-ingest needs for one entity's
// updates (ADR-072, in SemStreams).
//
//	pool, err := dispatch.NewKeyedPool(ctx, dispatch.KeyedConfig[ingestWork]{
//	    Lanes:      lanes,
//	    QueueDepth: 256,
//	    Name:       "graph_ingest",
//	    KeyOf:      func(w ingestWork) string { return w.entityID },
//	    Process:    processIngest, // func(ctx, lane, work) error
//	    OnPanic:    func(w ingestWork, _ any) { _ = w.msg.Nak() },
//	}, dispatch.KeyedDeps{MetricsRegistry: registry, Logger: logger})
//
// SubmitBlocking applies backpressure: when the item's lane queue is
// full, the call waits until the lane has room or its context ends.
//
// # Stopping
//
// Cancelling the context given to NewKeyedPool aborts the pool: the
// lanes stop without processing what is still queued. Shutdown(ctx) is
// the graceful stop. From its first call, SubmitBlocking returns
// ErrStopped; submits already accepted land in their queues, every lane
// finishes its queue and exits, and Shutdown returns nil. If ctx ends
// first, Shutdown returns ctx.Err() and the lanes keep draining; call
// Shutdown again to wait for them. Shutdown has no timeout of its own: a
// context with no deadline waits as long as Process takes. A nil context
// is refused with an error.
//
// A producer parked in SubmitBlocking on a full lane holds Shutdown up
// until the lane takes its item. On shutdown, cancel the context passed
// to SubmitBlocking first, then call Shutdown.
//
// # Metrics
//
// With KeyedDeps.MetricsRegistry set, the pool registers six collectors
// labelled pool=<Name> through metric.RegisterOrGet:
// dispatch_queue_wait_seconds, dispatch_processing_duration_seconds,
// dispatch_queue_depth, dispatch_inflight, dispatch_submitted_total and
// dispatch_completed_total. Two pools with the same Name on one
// registry share these collectors. A registration the registry refuses
// makes NewKeyedPool return the error. With no registry the pool keeps
// no metrics.
package dispatch
