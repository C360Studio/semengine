package graphingest

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/graph/readiness"
	"github.com/c360studio/semengine/pkg/errs"
)

// statusMetricsInterval is graph-ingest's readiness heartbeat. It matches the
// framework default so every producer's freshness window is the same constant
// (three heartbeats, graph/readiness's freshness window).
const statusMetricsInterval = readiness.DefaultHeartbeat

// boundConsumer identifies one durable consumer this component bound, so the status
// tick can ask the client for its outstanding work.
//
// graph-ingest binds ONE consumer PER INPUT PORT, each on a possibly different
// stream, and the backlog is the sum across all of them — a per-port view would let a
// component report caught-up while another port was hours behind.
type boundConsumer struct {
	stream string
	name   string
	// closed is the Closed channel of the consume loop bound to this consumer. It is
	// receive-only, so the tick can see the loop end while the handle, and the authority
	// to drain it, stays with Stop (ADR-095).
	closed <-chan struct{}
	// lost is set once a tick has recorded this binding lost, and never cleared: a
	// consume loop that has ended does not start again, so only a restart repairs it.
	lost bool
}

// registerBoundConsumer records a consumer after ConsumeStreamWithConfig has
// committed it, so the readiness tick never asks about a consumer whose bind failed.
func (c *Component) registerBoundConsumer(stream, name string, closed <-chan struct{}) {
	c.boundConsumersMu.Lock()
	defer c.boundConsumersMu.Unlock()
	for _, existing := range c.boundConsumers {
		if existing.stream == stream && existing.name == name {
			return
		}
	}
	c.boundConsumers = append(c.boundConsumers, boundConsumer{stream: stream, name: name, closed: closed})
}

// observeLostBinding reports whether bc's binding is lost, recording it the first time a
// tick sees its consume loop ended while the shell is running (#216, design D1).
//
// The loop's end is read first and the shell's running state second. The shell stops
// running before release issues any Drain (component.Lifecycle.Start), so a tick that sees
// a loop Stop's drain closed then reads "not running" and records nothing. Read the other
// way round, a tick could read "running", then see the loop Stop had just drained, and
// record a clean shutdown as a lost binding.
func (c *Component) observeLostBinding(bc boundConsumer) bool {
	if bc.lost {
		return true
	}
	select {
	case <-bc.closed:
	default:
		return false
	}
	if !c.running() {
		return false
	}
	if c.recordLostBinding(bc.stream, bc.name) {
		c.logger.Error("graph-ingest: input binding lost; its consume loop ended and is not bound again "+
			"until the process restarts",
			slog.String("stream", bc.stream),
			slog.String("consumer", bc.name))
	}
	return true
}

// recordLostBinding marks the stream/name record lost and reports whether this call did,
// so the Error line is logged once.
func (c *Component) recordLostBinding(stream, name string) bool {
	c.boundConsumersMu.Lock()
	defer c.boundConsumersMu.Unlock()
	for index := range c.boundConsumers {
		bc := &c.boundConsumers[index]
		if bc.stream == stream && bc.name == name {
			if bc.lost {
				return false
			}
			bc.lost = true
			return true
		}
	}
	return false
}

// lostBindingFault is Health's stuck state for the lost bindings, or "" when none is lost.
func (c *Component) lostBindingFault() string {
	var lost []string
	for _, bc := range c.boundConsumerSnapshot() {
		if bc.lost {
			lost = append(lost, bc.stream+"/"+bc.name)
		}
	}
	if len(lost) == 0 {
		return ""
	}
	return "input binding lost for consumer " + strings.Join(lost, ", ") +
		": its consume loop ended; restart the process to consume again"
}

// running reports whether the shell is running: from a successful start until release
// begins, for a Stop or a failed start's rollback. Lifecycle.Health with no stuck state
// reads "stopped" exactly when it is not, so the tick reads the shell's own state and
// keeps no copy of it.
func (c *Component) running() bool {
	return c.lifecycle.Health().Status != "stopped"
}

// boundConsumerSnapshot copies the bound set for iteration off the lock, in a
// deterministic order so a degraded verdict names the same consumer run to run.
func (c *Component) boundConsumerSnapshot() []boundConsumer {
	c.boundConsumersMu.RLock()
	defer c.boundConsumersMu.RUnlock()
	out := make([]boundConsumer, len(c.boundConsumers))
	copy(out, c.boundConsumers)
	sort.Slice(out, func(i, j int) bool {
		if out[i].stream != out[j].stream {
			return out[i].stream < out[j].stream
		}
		return out[i].name < out[j].name
	})
	return out
}

// clearBoundConsumers forgets readiness observation labels only after owner
// cleanup. These records never carry Stop, Drain, or deletion authority.
func (c *Component) clearBoundConsumers() {
	c.boundConsumersMu.Lock()
	c.boundConsumers = nil
	c.boundConsumersMu.Unlock()
}

// statusMetricsLoop republishes the readiness envelope on the shared heartbeat.
// Mirrors SemStreams graph-index's loop (not ported yet): the write doubles as a
// liveness heartbeat, so it is unconditional rather than on-change.
func (c *Component) statusMetricsLoop(ctx context.Context) {
	// Publish once up front so the key is populated before the first tick rather
	// than reading absent (which consumers correctly treat as unknown) for a whole
	// interval after start.
	c.refreshReadinessStatus(ctx)
	ticker := time.NewTicker(statusMetricsInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.refreshReadinessStatus(ctx)
		}
	}
}

// refreshReadinessStatus computes the envelope ONCE and publishes it. The single
// compute is the contract, not an optimization: two computes at slightly different
// instants would let separate distribution channels disagree about readiness with no
// way for a consumer to tell which was right.
func (c *Component) refreshReadinessStatus(ctx context.Context) {
	status := c.computeBacklogStatus(ctx)
	// ONE compute feeds BOTH channels. Two computes at slightly different instants
	// would let the scraped gauges and the watched KV key disagree about readiness,
	// with no way for a consumer reconciling them to tell which was right.
	c.readinessGauges.Set(status)
	c.publishReadinessStatus(ctx, status)
}

// computeBacklogStatus reads outstanding work across every bound consumer and
// projects it through the shared backlog projection.
//
// A read failure on ANY consumer degrades the whole envelope. Summing only the
// consumers that answered would publish a number that looks like a backlog and is
// actually a partial — precisely the shape that makes a caught-up claim wrong while
// looking healthy. The partial total still rides along as an honest lower bound
// (graph.BacklogStatusInputs.ObservationFailed documents that), but Ready is forced
// false and State degraded.
func (c *Component) computeBacklogStatus(ctx context.Context) graph.IndexStatusResponse {
	// An EMPTY bound set is honestly caught up, not degraded. This is not an edge
	// case: MEASURED 2026-07-30, 9 of the 18 shipped graph-ingest instances bind no
	// jetstream consumer at all — they declare their input port as type "nats"
	// (core NATS request/reply for graph_mutations), which setupSubscriptions skips.
	// Those deployments have no backlog to be behind on. Reporting degraded here
	// would make half the shipped fleet permanently unready and every consumer
	// folding on this key defer forever.
	consumers := c.boundConsumerSnapshot()

	var outstanding uint64
	observationFailed := false
	var failed []string

	// A lost binding degrades this envelope and every later one, as a failed observation
	// does. Its backlog is still read below, so a failed lookup is still named.
	for _, bc := range consumers {
		if c.observeLostBinding(bc) {
			observationFailed = true
		}
	}

	for _, bc := range consumers {
		work, err := c.observeOutstandingWork(ctx, bc.stream, bc.name)
		if err != nil {
			if ctx.Err() != nil {
				// Shutdown, not a fault.
				return graph.IndexStatusResponse{State: graph.IndexStateBuilding}
			}
			observationFailed = true
			failed = append(failed, bc.stream+"/"+bc.name)
			c.logger.Warn("graph-ingest readiness: outstanding-work read failed",
				slog.String("stream", bc.stream),
				slog.String("consumer", bc.name),
				slog.Any("error", err))
			continue
		}
		outstanding += work
	}

	// Recorded only for a tick that ran to here: a tick cut short by shutdown, above,
	// records nothing. Health reads it, so Health and this envelope agree on the tick.
	if len(failed) > 0 {
		fault := "backlog observation failed for consumer " + strings.Join(failed, ", ")
		c.backlogObservationFault.Store(&fault)
	} else {
		c.backlogObservationFault.Store(nil)
	}

	return readiness.ComputeBacklogStatus(readiness.BacklogStatusInputs{
		Outstanding: outstanding,
		// graph-ingest's boot work is the ENTITY_STATES sweep, which start finishes
		// before it starts this loop, so Start returning is the bootstrap (ADR-109
		// Decision 3). A backlog standing is Ready's to report, not this flag's.
		BootstrapComplete:   true,
		BootstrapScope:      c.bootstrapScope(outstanding, observationFailed),
		ObservationFailed:   observationFailed,
		OldestOutstandingAt: c.oldestOutstandingAt(outstanding),
		Now:                 time.Now(),
	})
}

// recordBindBacklog captures this consumer's backlog AT BIND TIME, accumulating across
// ports, as the producer's BootstrapScope.
//
// Capturing here rather than at the first status tick is load-bearing, not tidiness.
// The tick fires after setupSubscriptions, createStatusBucket, and goroutine
// scheduling, and delivery is already live throughout — so a small backlog can drain
// before the first tick reads it, and the producer would publish
// `complete && scope == 0`, which the contract defines as "authoritatively nothing to
// do". That is a FALSE claim about a deployment that had real work, and its value
// would vary run to run with tick timing. At bind the window is a few microseconds and
// draining even one message costs a KV round trip.
//
// A failed read leaves the capture unclaimed so the first successful tick still fills
// it in — a late scope beats a fabricated zero.
func (c *Component) recordBindBacklog(ctx context.Context, stream, name string) {
	work, err := c.observeOutstandingWork(ctx, stream, name)
	if err != nil {
		// A PARTIAL capture is worse than none: with several bound consumers, one
		// success would otherwise mark the aggregate "known" and permanently omit the
		// failed consumer's backlog. If A measured zero and B held work, the producer
		// would publish `bootstrap_complete && bootstrap_scope == 0` — the contract's
		// "authoritatively nothing to replay" — while B had a real initial build.
		// Marking the whole capture partial hands it to the first full status tick
		// (bootstrapScope), which reads the TOTAL across all consumers at once.
		c.bootBacklogPartial.Store(true)
		c.logger.Warn("graph-ingest readiness: bind-time backlog read failed; "+
			"bootstrap scope will be captured whole at the first successful status tick",
			slog.String("stream", stream),
			slog.String("consumer", name),
			slog.Any("error", err))
		return
	}
	c.bootBacklog.Add(work)
}

// observeOutstandingWork performs one read-only lookup by durable identity. It
// deliberately retains no Consumer or ConsumeContext and exposes no lifecycle
// authority; the owning Start/Stop path separately retains the exact native
// ConsumeContext. Unknown observation remains an error, never a fabricated zero.
func (c *Component) observeOutstandingWork(ctx context.Context, streamName, consumerName string) (uint64, error) {
	js, err := c.natsClient.JetStream()
	if err != nil {
		return 0, errs.WrapTransient(err, "Component", "observeOutstandingWork", "get JetStream")
	}
	stream, err := js.Stream(ctx, streamName)
	if err != nil {
		return 0, errs.WrapTransient(err, "Component", "observeOutstandingWork", "look up stream "+streamName)
	}
	consumer, err := stream.Consumer(ctx, consumerName)
	if err != nil {
		return 0, errs.WrapTransient(err, "Component", "observeOutstandingWork", "look up consumer "+consumerName)
	}
	info, err := consumer.Info(ctx)
	if err != nil {
		return 0, errs.WrapTransient(err, "Component", "observeOutstandingWork", "read consumer info "+consumerName)
	}
	if info.NumAckPending < 0 {
		return info.NumPending, nil
	}
	return info.NumPending + uint64(info.NumAckPending), nil
}

// bootstrapScope returns BootstrapScope: the backlog this producer bound onto, summed over
// its consumers.
//
// It is normally captured at bind (recordBindBacklog). When any bind read failed, the
// accumulated partial is discarded and replaced by the first successful FULL observation: a
// partial scope is a false claim about the size of the initial build, and understating it can
// reach zero, which the contract reads as "there was nothing to do". Either way it is captured
// once and never moves, so a later burst cannot retroactively enlarge the "initial build". A
// failed observation captures nothing: zero read off it is the absence of a measurement.
//
// It is the size of the backlog at bind, never a claim that it was worked off:
// bootstrap_complete is true once Start returns (ADR-109 Decision 3), and Ready carries
// "caught up right now".
func (c *Component) bootstrapScope(outstanding uint64, observationFailed bool) uint64 {
	if !observationFailed && c.bootBacklogKnown.CompareAndSwap(false, true) && c.bootBacklogPartial.Load() {
		c.bootBacklog.Store(outstanding)
	}
	return c.bootBacklog.Load()
}

// oldestOutstandingAt reports the timestamp the staleness projection ages from, or the
// zero time when there is nothing to report.
//
// It does not report the oldest outstanding message's meta.Timestamp: tracking that
// exactly means per-message bookkeeping over up to 2048 in-flight messages on the
// ingest hot path, with insertion and removal on every submit and ack. This reports the AGE OF THE VIEW
// instead — now minus the JetStream timestamp of the most recently APPLIED message —
// which is SemStreams graph-index's semantic for the same field (its IndexedAt) and
// costs one atomic store on the applied path.
//
// The two coincide closely because delivery is approximately stream-ordered: the
// oldest outstanding message is the one just after the newest applied one. It is a
// FLOOR either way, and the field is REPORTED, never gating, so the residual
// difference cannot change a readiness verdict.
func (c *Component) oldestOutstandingAt(outstanding uint64) time.Time {
	if outstanding == 0 {
		// Caught up: there is no staleness to report. The projection encodes this as
		// 0 = no information.
		return time.Time{}
	}
	if v := c.lastAppliedAt.Load(); v != nil {
		if t, ok := v.(time.Time); ok {
			return t
		}
	}
	return time.Time{}
}

// recordApplied stamps the JetStream timestamp of a message whose graph write is
// durable. Called once the write and its guard record are durable, on the applied
// path only: an input left for redelivery, or a stale redelivery, has applied nothing.
func (c *Component) recordApplied(deliveredAt time.Time) {
	if deliveredAt.IsZero() {
		return
	}
	c.lastAppliedAt.Store(deliveredAt)
}

// publishReadinessStatus writes the envelope to the GRAPH_STATUS key (ADR-083).
//
// A failed write must never kill the tick loop or the component: the next heartbeat is
// the recovery path, and consumers already fail closed after three missed heartbeats,
// so the correct behavior is to leave evidence and keep ticking.
func (c *Component) publishReadinessStatus(ctx context.Context, status graph.IndexStatusResponse) {
	if err := c.statusPublisher.Publish(ctx, status); err != nil {
		if ctx.Err() != nil {
			// Shutdown, not a failure: Stop cancels mid-Put on the way out.
			return
		}
		c.readinessGauges.RecordPublishFailure()
		c.logger.Warn("readiness status publish failed",
			slog.String("bucket", graph.BucketGraphStatus),
			slog.String("key", c.statusPublisher.Key()),
			slog.Any("error", err))
	}
}

// createStatusBucket provisions the GRAPH_STATUS handle and builds the publisher.
// A provisioning failure is fatal to Start: a producer that cannot publish readiness
// would be indistinguishable on the wire from one that is merely absent, and
// consumers would defer forever with no way to tell why.
func (c *Component) createStatusBucket(ctx context.Context) error {
	bucket, err := kvcatalog.EnsureCatalogBucket(ctx, c.natsClient, graph.BucketGraphStatus)
	if err != nil {
		if ctx.Err() != nil {
			return errs.Wrap(ctx.Err(), "Component", "createStatusBucket", "context cancelled")
		}
		return errs.Wrap(err, "Component", "createStatusBucket",
			"KV bucket: "+graph.BucketGraphStatus)
	}
	publisher, err := readiness.NewPublisher(bucket, readiness.KeyGraphIngest)
	if err != nil {
		return errs.Wrap(err, "Component", "createStatusBucket", "readiness publisher")
	}
	c.statusPublisher = publisher
	return nil
}
