// Package readiness writes and reads the readiness envelope a producer keeps in the
// GRAPH_STATUS KV bucket, and holds the gate that judges one.
//
// The envelope (graph.IndexStatusResponse) is a producer's own account of its
// progress: whether it has finished its initial build, how far behind it is, and
// whether it is broken. A producer such as graph-ingest writes it to its own key on
// every heartbeat tick through a Publisher, and Gauges exposes the same fields to
// Prometheus. ComputeBacklogStatus fills the envelope for a producer whose progress is
// a message backlog, as graph-ingest's is.
//
// The bucket is information, not a gate (ADR-109 Decision 3). It is for operators and
// for tools that show a producer's state to people or agents. Nothing in SemEngine
// waits on it.
//
// EvaluateReadinessGate answers one Reading with a DeferReason: proceed, or why not.
// Its one reader in SemEngine will be graph-index, which asks it in process about the
// status it has just computed.
//
// The Watcher reads one producer's key from another process and holds the last
// envelope, so a read needs no NATS round trip. It judges freshness by when each value
// arrived, never by comparing the producer's clock with its own (ADR-083 Decision 2);
// the envelope's published_at is not read here. Nothing in SemEngine reads a Watcher
// yet: it is kept for the readers at the SemStreams pin, fusion and graph-clustering,
// when they are ported.
//
// The key names live here, for producers and readers both, so the two cannot drift
// onto different names. The envelope type stays in graph, so a reader of GRAPH_STATUS
// needs no NATS import.
package readiness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/natsclient"
	"github.com/nats-io/nats.go/jetstream"
)

// The GRAPH_STATUS keys, shared by producers and consumers. The bucket's name is
// graph.BucketGraphStatus.
const (
	// KeyGraphIndex is graph-index's status key (one key per producer).
	KeyGraphIndex = "graph-index"
	// KeyGraphEmbedding is graph-embedding's status key.
	KeyGraphEmbedding = "graph-embedding"
	// KeyGraphIngest is graph-ingest's status key. Its envelope is a BACKLOG
	// envelope (ComputeBacklogStatus): Lag is in MESSAGES, and the revision
	// fields are absent because graph-ingest consumes multiple streams whose
	// sequence spaces are independent.
	KeyGraphIngest = "graph-ingest"
	// KeyRule is the rule processor's status key, reporting bootstrap-replay
	// completion for its entity watchers.
	KeyRule = "rule"
)

// Declaring a key here does NOT make it mandatory. There is deliberately no
// framework-level "all producers" list: which producers run depends on the
// deployment, so a framework-declared mandatory list would break every deployment
// that runs one producer without another. Each CONSUMER declares the keys it
// depends on and folds them client-side.

const (
	// DefaultHeartbeat is the producers' status tick: they publish the envelope
	// every tick unconditionally, so the write doubles as a liveness heartbeat.
	DefaultHeartbeat = 5 * time.Second
	// freshnessMultiplier is how many heartbeats a held status stays fresh. It is a
	// CONSTANT, not config (owner decision): 3 tolerates a lost write and a slow
	// delivery without letting a dead feed masquerade as live, and one fewer knob
	// is one fewer way to configure a fail-open.
	//
	// "Freshness" here is about the ENVELOPE, not the view it describes — can this
	// consumer still vouch for the status reading it is holding, or has the producer
	// gone quiet? That is a transport-liveness question, and it is what
	// Reading.Fresh answers and what the health gate fails closed on. It is NOT
	// the retired view-age concept (how far behind ENTITY_STATES the index itself is);
	// view age is reported on IndexStatusResponse.StalenessMs and gates nothing.
	freshnessMultiplier = 3
	// rebindDelay is how long the watcher waits before re-opening the bucket
	// or watch after a failure. Well under the freshness window, so a transient
	// bucket outage that resolves quickly never surfaces as unknown.
	rebindDelay = time.Second
)

// BucketSource opens a KV bucket by name. *natsclient.Client satisfies it through
// GetKeyValueBucket, whose signature this must match exactly. The Watcher takes this
// one method rather than the client, so its tests need no live NATS.
type BucketSource interface {
	GetKeyValueBucket(ctx context.Context, name string) (jetstream.KeyValue, error)
}

// freshnessWindow is how long a status value stays fresh for a producer publishing on
// the given heartbeat.
func freshnessWindow(heartbeat time.Duration) time.Duration {
	return time.Duration(freshnessMultiplier) * heartbeat
}

// Reading is the consumer-side answer: the last-known envelope plus everything
// needed to attribute a defer without correlating log lines.
//
// Status is authoritative ONLY when Fresh. When Fresh is false the envelope is
// last-known-and-aged, or no longer the key's current value (or the zero value if none
// ever arrived), and is carried for DIAGNOSTICS only — a caller must fail closed, never
// read the stale bits as index state, so a dead feed is never mistaken for an index
// that is not ready.
type Reading struct {
	// Status is the last envelope decoded from the key.
	Status graph.IndexStatusResponse
	// Raw is the untouched wire value the Status field was decoded from, for a caller
	// that decodes into its OWN struct with the same fields. Decoding the bytes again
	// keeps every field; copying fields across by hand can drop one, and a dropped
	// IndexedRevision or Lag reads as caught up. Nil when !Known.
	Raw []byte
	// Fresh reports that the held envelope arrived within three
	// heartbeats, was not already older than that when it arrived, and is still the
	// key's current value (no undecodable value has replaced it). False means
	// UNKNOWN — fail closed.
	Fresh bool
	// Known reports that an envelope has been received at least once. It separates
	// "never saw the producer" (absent bucket/key, standalone deployment) from "the
	// feed went quiet", which are different operator problems.
	Known bool
	// Age is how long ago the held envelope arrived, consumer-local. Zero when
	// !Known.
	Age time.Duration
	// Err is the last bucket/watch/decode failure, if any, for the structured defer
	// log. It is cleared by the next good update.
	Err error
}

// Watcher holds the last-known readiness envelope for ONE producer key. One
// instance serves every gate decision in a consumer: Read is safe from any
// goroutine, and Run runs on a goroutine the owning component starts and stops by
// ending Run's context.
type Watcher struct {
	src    BucketSource
	bucket string
	key    string

	heartbeat time.Duration
	logger    *slog.Logger

	mu          sync.Mutex
	status      graph.IndexStatusResponse
	raw         []byte
	lastArrival time.Time
	known       bool
	// revoked marks a held envelope Read must never report fresh, whatever its
	// arrival age: one that was ALREADY older than the freshness window when it was
	// delivered (see apply for why this one commit-time comparison exists), or one an
	// undecodable value has since replaced on the key. Each decoded update sets it
	// afresh.
	revoked bool
	lastErr error

	// first is closed once an envelope has been decoded and held, so a consumer
	// binding lazily can wait for its first answer instead of failing closed on a
	// watch that has simply not delivered yet.
	first     chan struct{}
	firstOnce sync.Once
}

// NewWatcher builds a watcher for one producer key (KeyGraphIndex,
// KeyGraphEmbedding) in the GRAPH_STATUS bucket. heartbeat is the producer's publish
// interval (DefaultHeartbeat for the framework's producers), which sets the freshness
// window to three heartbeats; logger receives bind and decode failures. It performs no
// I/O and checks nothing; Run refuses incomplete wiring.
func NewWatcher(src BucketSource, key string, heartbeat time.Duration, logger *slog.Logger) *Watcher {
	return &Watcher{
		src:       src,
		bucket:    graph.BucketGraphStatus,
		key:       key,
		heartbeat: heartbeat,
		logger:    logger,
		first:     make(chan struct{}),
	}
}

// Run watches the key until ctx ends, then returns ctx.Err(). The caller runs it on a
// goroutine it owns; Run starts no goroutine of its own and releases the KV watch
// before it returns, so its return is the join.
//
// Run does NOT fail when the bucket or key is absent: a deployment running a consumer
// without its producer is a legitimate configuration whose readiness is simply
// UNKNOWN, and Read reports it as such (fail closed) while Run keeps trying to bind,
// rebinding after any failure. It never fabricates an envelope.
// Misconfiguration is refused at the call instead: a nil context, a nil bucket source,
// an empty key, a heartbeat of zero or less, or a nil logger returns an error before
// any I/O.
func (w *Watcher) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("readiness: nil context")
	}
	if w.src == nil {
		return errors.New("readiness: nil bucket source")
	}
	if w.key == "" {
		return errors.New("readiness: empty status key")
	}
	if w.heartbeat <= 0 {
		return fmt.Errorf("readiness: heartbeat %s is not positive", w.heartbeat)
	}
	if w.logger == nil {
		return errors.New("readiness: nil logger")
	}
	for {
		if err := w.watchOnce(ctx); err != nil && ctx.Err() == nil {
			w.recordErr(err)
			w.logger.Debug("readiness watch unavailable",
				slog.String("bucket", w.bucket), slog.String("key", w.key), slog.Any("error", err))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(rebindDelay):
		}
	}
}

// Read returns the current reading. It never blocks on I/O — the answer comes from
// held state, which is the point of watching rather than polling.
func (w *Watcher) Read() Reading {
	w.mu.Lock()
	defer w.mu.Unlock()
	r := Reading{Status: w.status, Raw: w.raw, Known: w.known, Err: w.lastErr}
	if !w.known {
		return r
	}
	r.Age = time.Now().Sub(w.lastArrival)
	// A negative age (clock stepped backwards) is still within the window: the
	// update did arrive, and the comparison is entirely local.
	r.Fresh = !w.revoked && r.Age <= freshnessWindow(w.heartbeat)
	return r
}

// WaitForFirst blocks until an envelope has been received on the key, or ctx ends. It
// returns nil once something has arrived (fresh or not — Read is what judges that) and
// ctx.Err() otherwise.
//
// It is for a consumer that binds the watch lazily, on its first gate decision, as
// fusion does at the SemStreams pin: without a bounded wait, that first decision would
// fail closed only because the watch had not delivered yet. Waiting once at bind, with
// a bounded context, delays only the first gated call; every later call is a local
// read. Callers must wait AT MOST once: with no producer deployed nothing will
// ever arrive, and a per-call wait would turn the standalone deployment into a
// per-call stall. A nil context returns an error, even after an envelope has arrived.
func (w *Watcher) WaitForFirst(ctx context.Context) error {
	if ctx == nil {
		return errors.New("readiness: nil context")
	}
	select {
	case <-w.first:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// watchOnce opens the bucket and the key watch and consumes updates until the watch
// ends or the context is cancelled. A missing bucket is an ordinary error here —
// held state stays unknown and the caller retries.
func (w *Watcher) watchOnce(ctx context.Context) error {
	bucket, err := w.src.GetKeyValueBucket(ctx, w.bucket)
	if err != nil {
		return fmt.Errorf("readiness: open bucket %s: %w", w.bucket, err)
	}
	// No UpdatesOnly: the current value must be delivered immediately on bind, so a
	// restarted consumer is fresh within one delivery rather than one heartbeat.
	watcher, err := bucket.Watch(ctx, w.key)
	if err != nil {
		return fmt.Errorf("readiness: watch %s/%s: %w", w.bucket, w.key, err)
	}
	defer func() {
		if stopErr := watcher.Stop(); stopErr != nil {
			w.logger.Debug("readiness watch stop", slog.Any("error", stopErr))
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case entry, ok := <-watcher.Updates():
			if !ok {
				return fmt.Errorf("readiness: watch %s/%s closed", w.bucket, w.key)
			}
			// nil is the end-of-initial-values marker, not a value. An absent key
			// simply means no envelope has ever been published: stay unknown.
			if entry == nil {
				continue
			}
			w.apply(entry)
		}
	}
}

// apply folds one delivered entry into held state and stamps the CONSUMER-LOCAL
// arrival time. A tombstone clears knownness and an undecodable value revokes the held
// envelope's freshness. Neither stamps an arrival time: neither is evidence the
// producer is alive and healthy, and stamping one would make garbage read as fresh.
func (w *Watcher) apply(entry jetstream.KeyValueEntry) {
	if natsclient.IsKVTombstone(entry.Operation()) {
		w.mu.Lock()
		w.known = false
		w.raw = nil
		w.revoked = false
		w.lastErr = fmt.Errorf("readiness: status key %s/%s deleted", w.bucket, w.key)
		w.mu.Unlock()
		return
	}

	// The KV value is plain graph.IndexStatusResponse JSON, not a payload-registry
	// envelope: this is operational state written to KV, not a published message
	// payload.
	var status graph.IndexStatusResponse
	if err := json.Unmarshal(entry.Value(), &status); err != nil {
		// The key's current value is now one nothing can read, so the held envelope no
		// longer describes the producer's status: revoke its freshness at once rather
		// than let it serve the gate for the rest of its window. It stays held, with its
		// arrival stamp, for diagnostics only; the next decoded update restores freshness.
		w.mu.Lock()
		w.revoked = true
		w.lastErr = fmt.Errorf("readiness: decode %s/%s: %w", w.bucket, w.key, err)
		w.mu.Unlock()
		w.logger.Warn("readiness status undecodable",
			slog.String("bucket", w.bucket), slog.String("key", w.key), slog.Any("error", err))
		return
	}

	arrival := time.Now()

	// THE ONE COMMIT-TIME COMPARISON, and it applies only to a value that was already
	// old when it reached us. A watch delivers the key's CURRENT value the instant it
	// binds, so a consumer that starts while its producer is dead receives that
	// producer's final envelope — possibly Ready=true, written an hour ago — and pure
	// arrival-time freshness would stamp it as brand new and read it as live for a
	// whole window, a fail-OPEN. Steady-state freshness still compares no clocks
	// (ADR-083 Decision 2): once a
	// live producer's heartbeat lands, arrival time governs, and the 3x window absorbs
	// ordinary skew, so no healthy producer is ever rejected here.
	staleOnArrival := false
	var staleErr error
	if created := entry.Created(); !created.IsZero() {
		if age := arrival.Sub(created); age > freshnessWindow(w.heartbeat) {
			staleOnArrival = true
			// Attributable, because this rejection is otherwise INVISIBLE: knownness
			// stays true and the arrival stamp is recent, so a consumer's defer log
			// would read "status_known=true status_age=1ms reason=status_unknown"
			// with nothing explaining it. It also names the one skew-sensitive
			// comparison in the package — a consumer clock far enough ahead of the
			// NATS server rejects a healthy producer's every write, and this is the
			// line that says so instead of blaming the producer.
			staleErr = fmt.Errorf(
				"readiness: %s/%s was already %s old on arrival (window %s) — dead producer, or consumer clock ahead of the server",
				w.bucket, w.key, age.Round(time.Millisecond), freshnessWindow(w.heartbeat))
		}
	}

	w.mu.Lock()
	w.status = status
	w.raw = entry.Value()
	w.lastArrival = arrival
	w.known = true
	w.revoked = staleOnArrival
	w.lastErr = staleErr
	w.mu.Unlock()

	// Signalled for ANY held envelope, stale or not: a waiting consumer wants the
	// first answer, and "the key holds a dead producer's last word" is an answer. Read
	// is what judges it.
	w.firstOnce.Do(func() { close(w.first) })
}

// recordErr stores the last failure for the structured defer log without touching
// the arrival stamp — an error must never extend freshness.
func (w *Watcher) recordErr(err error) {
	w.mu.Lock()
	w.lastErr = err
	w.mu.Unlock()
}
