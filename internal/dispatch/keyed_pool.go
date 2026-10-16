package dispatch

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/c360studio/semengine/metric"
	"github.com/prometheus/client_golang/prometheus"
)

// KeyedPool is the framework's keyed-ordered bounded-concurrency
// primitive: it partitions work into N lanes by the key each
// SubmitBlocking call names.
// All items sharing a key route to the same lane and are processed
// serially in submit order, while items with different keys spread
// across lanes and run concurrently.
//
// It exists for at-least-once consumers that need per-key ordered
// concurrency — e.g. graph-ingest, whose arrival-order (full-set-
// replace) merge requires that all messages for one entity apply in
// order, but which is otherwise latency-bound on a serial Get+CAS
// chain (ADR-072).
//
// Structure: N separate bounded lane channels, one goroutine per lane.
// A shared channel read by N workers cannot preserve per-key order,
// which is why each lane has its own channel and goroutine.
//
// Concurrency: SubmitBlocking and Shutdown are safe to call from
// concurrent goroutines. The Process function for a given lane is only
// ever invoked by that lane's single goroutine, so a composer
// maintaining per-lane state (indexed by the lane number passed into
// Process) needs no locking on it.
type KeyedPool[W any] struct {
	lanes   int
	process func(ctx context.Context, lane int, work W)
	onPanic func(work W, recovered any)
	logger  *slog.Logger
	name    string

	laneCh []chan keyedItem[W]
	wg     sync.WaitGroup

	lifecycleMu sync.Mutex
	stopped     bool
	stopDone    chan struct{}

	// submitWG tracks in-flight SubmitBlocking calls between the
	// stopped-check (which registers the WG) and the channel send. The
	// drain waits on it BEFORE closing the lane channels, so no submit
	// sends on a closed channel — the accept/stop handoff is atomic (a
	// submit either lands before the drain starts and is guaranteed
	// processed, or sees stopped=true and is rejected).
	submitWG sync.WaitGroup

	metrics *keyedMetrics
}

// keyedItem wraps a work item with the timestamp it was submitted, so
// the lane can observe queue-wait (submit → pickup) at process start.
// W is generic, so the timestamp can't live on the work value itself.
type keyedItem[W any] struct {
	work     W
	submitAt time.Time
}

// KeyedConfig parameterizes KeyedPool construction. The zero value is
// NOT valid — NewKeyedPool rejects Lanes <= 0, QueueDepth <= 0, and a
// missing Process.
type KeyedConfig[W any] struct {
	// Lanes is the number of parallel serial lanes. Same-key work is
	// serialized within a lane; different keys spread across lanes.
	// Must be > 0. Fixed for the pool's lifetime (lane assignment is a
	// pure function of the key over this count).
	Lanes int

	// QueueDepth bounds each lane's queue. SubmitBlocking waits while
	// the target lane is at capacity. Must be > 0.
	QueueDepth int

	// Process handles one work item. It is called by the assigned
	// lane's single goroutine, with that lane's index — a composer can
	// use the index to shard per-lane state without locking (the pool
	// guarantees at most one goroutine per lane). Must be non-nil. The
	// pool reports nothing about the outcome: Process disposes of its own
	// failures (logs them, settles the message it came from).
	Process func(ctx context.Context, lane int, work W)

	// OnPanic — optional. When Process panics, the pool recovers it,
	// keeps the lane goroutine alive, and (if set) calls OnPanic with
	// the work item and the recovered value so the composer can
	// dispose of it (e.g. Nak the underlying message). Called from the
	// lane goroutine. If nil, a recovered panic is logged and dropped.
	OnPanic func(work W, recovered any)

	// Name labels this pool's metrics (the `pool` label). Optional;
	// the pool has metrics only when MetricsRegistry is set.
	Name string

	// MetricsRegistry — optional. When set, the pool registers its
	// metrics on it; when nil, the pool keeps no metrics.
	MetricsRegistry *metric.MetricsRegistry

	// Logger — optional; nil means slog.Default.
	Logger *slog.Logger
}

// NewKeyedPool constructs and starts a KeyedPool. Lane goroutines are
// running and ready to receive SubmitBlocking calls when it returns —
// callers don't call a separate Start.
//
// The ctx is handed to every Process call and read nowhere else: its
// end stops no lane, refuses no submit and drops no queued item, so
// items processed after it ended see the ended ctx. Shutdown is the
// only stop; the lanes run until it is called. The ctx must be non-nil.
//
// Returns ErrInvalidConfig for a nil ctx or a missing or out-of-range
// required field, and the registry's refusal when one of the pool's
// metrics cannot be registered; either way no goroutine is started.
func NewKeyedPool[W any](ctx context.Context, cfg KeyedConfig[W]) (*KeyedPool[W], error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is required", ErrInvalidConfig)
	}
	if cfg.Lanes <= 0 {
		return nil, fmt.Errorf("%w: Lanes must be > 0 (got %d)", ErrInvalidConfig, cfg.Lanes)
	}
	if cfg.QueueDepth <= 0 {
		return nil, fmt.Errorf("%w: QueueDepth must be > 0 (got %d)", ErrInvalidConfig, cfg.QueueDepth)
	}
	if cfg.Process == nil {
		return nil, fmt.Errorf("%w: Process is required", ErrInvalidConfig)
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	p := &KeyedPool[W]{
		lanes:    cfg.Lanes,
		process:  cfg.Process,
		onPanic:  cfg.OnPanic,
		logger:   logger,
		name:     cfg.Name,
		laneCh:   make([]chan keyedItem[W], cfg.Lanes),
		stopDone: make(chan struct{}),
	}

	if cfg.MetricsRegistry != nil {
		metrics, err := newKeyedMetrics(cfg.MetricsRegistry, cfg.Name)
		if err != nil {
			return nil, fmt.Errorf("dispatch: register the metrics of pool %q: %w", cfg.Name, err)
		}
		p.metrics = metrics
	}

	for i := 0; i < cfg.Lanes; i++ {
		p.laneCh[i] = make(chan keyedItem[W], cfg.QueueDepth)
		p.wg.Add(1)
		go p.lane(ctx, i, p.laneCh[i])
	}

	return p, nil
}

// laneFor maps a key to its lane via FNV-1a over the key mod Lanes.
// Inlined (no hasher allocation) since it is on the submit hot path.
func (p *KeyedPool[W]) laneFor(key string) int {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	h := uint64(offset64)
	for i := 0; i < len(key); i++ {
		h ^= uint64(key[i])
		h *= prime64
	}
	return int(h % uint64(p.lanes))
}

// SubmitBlocking routes work to the lane its key maps to, blocking
// until the lane has capacity or the ctx ends (returns ctx.Err()).
// Items submitted with equal keys are processed one at a time, in
// submit order. A call that begins after Shutdown has begun returns
// ErrStopped; a call already waiting on a full lane when Shutdown
// begins may still be accepted, and is then processed before Shutdown
// returns nil. This is the backpressure form — a full lane blocks the
// caller rather than dropping.
//
// Composer note (ADR-072 M3): Shutdown waits for a call already parked
// on a full lane, and that call returns only when the lane takes its
// item or its ctx ends. On shutdown, cancel the ctx passed here BEFORE
// calling Shutdown, so a synchronous producer (e.g. a NATS consume
// callback) parked here unblocks and can dispose of its message.
func (p *KeyedPool[W]) SubmitBlocking(ctx context.Context, key string, work W) error {
	p.lifecycleMu.Lock()
	if p.stopped {
		p.lifecycleMu.Unlock()
		return ErrStopped
	}
	// Register in-flight UNDER the lock, alongside the stopped-check, so
	// the drain's submitWG.Wait() cannot begin until this send completes
	// (atomic handoff).
	p.submitWG.Add(1)
	p.lifecycleMu.Unlock()
	defer p.submitWG.Done()

	lane := p.laneFor(key)
	item := keyedItem[W]{work: work, submitAt: time.Now()}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case p.laneCh[lane] <- item:
		if p.metrics != nil {
			p.metrics.submitted.Inc()
			p.metrics.queueDepth.Inc()
		}
		return nil
	}
}

// lane is one lane's goroutine: it runs each item of its channel
// through Process, in order, and exits once Shutdown's drain has closed
// the channel and the last item is processed. ctx is only handed to
// Process.
func (p *KeyedPool[W]) lane(ctx context.Context, idx int, ch chan keyedItem[W]) {
	defer p.wg.Done()
	for item := range ch {
		p.runOne(ctx, idx, item)
	}
}

// runOne invokes Process for one item with panic recovery (ADR-072
// H2): a panic in Process must not crash the host or wedge the lane.
// The recovered panic is logged, the composer's OnPanic disposition is
// invoked, and the lane goroutine continues to the next item.
func (p *KeyedPool[W]) runOne(ctx context.Context, lane int, item keyedItem[W]) {
	if p.metrics != nil {
		p.metrics.queueDepth.Dec()
		p.metrics.queueWait.Observe(time.Since(item.submitAt).Seconds())
		p.metrics.inflight.Inc()
	}
	defer func() {
		if p.metrics != nil {
			p.metrics.inflight.Dec()
			p.metrics.completed.Inc()
		}
		if r := recover(); r != nil {
			p.logger.Error("dispatch: recovered panic in Process; lane continues",
				slog.String("pool", p.name),
				slog.Int("lane", lane),
				slog.Any("panic", r))
			if p.onPanic != nil {
				p.onPanic(item.work, r)
			}
		}
	}()

	start := time.Now()
	p.process(ctx, lane, item.work)
	if p.metrics != nil {
		p.metrics.processing.Observe(time.Since(start).Seconds())
	}
}

// Shutdown is the pool's only stop: it stops accepting new work (a
// SubmitBlocking call that begins after the first Shutdown returns
// ErrStopped; one already waiting on a full lane may still be
// accepted, and is processed), lets each lane process its queued
// items, and returns nil once every lane has exited, also when ctx has
// ended by then. If ctx ends first it returns
// ctx.Err(); the lanes keep processing and exit once their queues are
// empty, and a later Shutdown returns nil. Given a ctx with no
// deadline, Shutdown waits for the join however long it takes: it has
// no wait of its own. A nil ctx is refused with an error and begins no
// shutdown.
//
// Shutdown does not end the context NewKeyedPool was given; ending it
// is the owner's, and makes the remaining Process calls see an ended
// context.
func (p *KeyedPool[W]) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("dispatch: Shutdown needs a non-nil context")
	}
	p.lifecycleMu.Lock()
	if !p.stopped {
		p.stopped = true // reject NEW submits before the drain starts
		go p.drain()
	}
	p.lifecycleMu.Unlock()

	// The lanes having exited is the answer even when ctx has also ended.
	select {
	case <-p.stopDone:
		return nil
	default:
	}
	select {
	case <-p.stopDone:
		return nil
	case <-ctx.Done():
		p.logger.Warn("dispatch: Shutdown's context ended before accepted work drained",
			slog.String("pool", p.name))
		return ctx.Err()
	}
}

// drain runs once, started by the first Shutdown: it waits for the
// submits accepted before Shutdown to land, closes every lane channel
// so each lane exits after its last item, and closes stopDone once
// every lane has exited.
func (p *KeyedPool[W]) drain() {
	p.submitWG.Wait()
	for _, ch := range p.laneCh {
		close(ch)
	}
	p.wg.Wait()
	close(p.stopDone)
}

// keyedMetrics holds the Prometheus metrics for one KeyedPool,
// distinguished by the `pool` const-label so multiple pools coexist.
type keyedMetrics struct {
	queueWait  prometheus.Histogram
	processing prometheus.Histogram
	queueDepth prometheus.Gauge
	inflight   prometheus.Gauge
	submitted  prometheus.Counter
	completed  prometheus.Counter
}

// newKeyedMetrics registers the pool's collectors through
// metric.RegisterOrGet and keeps the collectors it returns: a second
// pool with the same name on the same registry writes the first pool's
// collectors, so its writes are the ones gathered. A refusal is
// returned, never dropped.
func newKeyedMetrics(registry *metric.MetricsRegistry, name string) (*keyedMetrics, error) {
	labels := prometheus.Labels{"pool": name}
	m := &keyedMetrics{
		queueWait: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:        "dispatch_queue_wait_seconds",
			Help:        "Time a work item waited between submit and process start (lane queue wait)",
			ConstLabels: labels,
			Buckets:     []float64{0.0001, 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		}),
		processing: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:        "dispatch_processing_duration_seconds",
			Help:        "Time spent in the Process function",
			ConstLabels: labels,
			Buckets:     []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5},
		}),
		queueDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "dispatch_queue_depth",
			Help:        "Items accepted and not yet passed to Process, across all lanes of every pool of this name",
			ConstLabels: labels,
		}),
		inflight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name:        "dispatch_inflight",
			Help:        "Lanes currently executing Process (achieved concurrency)",
			ConstLabels: labels,
		}),
		submitted: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "dispatch_submitted_total",
			Help:        "Total work items accepted onto a lane",
			ConstLabels: labels,
		}),
		completed: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "dispatch_completed_total",
			Help:        "Total work items whose Process returned or was recovered",
			ConstLabels: labels,
		}),
	}

	// serviceName groups these under the primitive; the pool name in each
	// metric name keeps pools with different names apart.
	const serviceName = "keyed_dispatch"
	var err error
	if m.queueWait, err = metric.RegisterOrGet(registry, serviceName, "dispatch_queue_wait_seconds/"+name, m.queueWait); err != nil {
		return nil, err
	}
	if m.processing, err = metric.RegisterOrGet(registry, serviceName, "dispatch_processing_duration_seconds/"+name, m.processing); err != nil {
		return nil, err
	}
	if m.queueDepth, err = metric.RegisterOrGet(registry, serviceName, "dispatch_queue_depth/"+name, m.queueDepth); err != nil {
		return nil, err
	}
	if m.inflight, err = metric.RegisterOrGet(registry, serviceName, "dispatch_inflight/"+name, m.inflight); err != nil {
		return nil, err
	}
	if m.submitted, err = metric.RegisterOrGet(registry, serviceName, "dispatch_submitted_total/"+name, m.submitted); err != nil {
		return nil, err
	}
	if m.completed, err = metric.RegisterOrGet(registry, serviceName, "dispatch_completed_total/"+name, m.completed); err != nil {
		return nil, err
	}
	return m, nil
}
