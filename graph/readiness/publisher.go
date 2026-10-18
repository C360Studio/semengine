package readiness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semengine/graph"
)

// This file is the PRODUCER side of the ADR-083 contract; watcher.go is the reading
// side. Both live here so the key names, heartbeat, and value encoding cannot drift
// apart between producers and readers. The bucket's SHAPE (name, History, retention)
// is declared once, in the framework KV catalog (the graph/kvcatalog package).

// publishTimeout bounds one heartbeat write. It is well under DefaultHeartbeat so a
// wedged Put cannot eat the whole interval it exists to announce: the tick loop must
// come back around and try again (and a Ticker drops, never queues, missed ticks).
const publishTimeout = 2 * time.Second

// StatusWriter writes one status value. jetstream.KeyValue satisfies it; taking the
// one method the publisher needs keeps the seam narrow and the unit tests honest.
type StatusWriter interface {
	Put(ctx context.Context, key string, value []byte) (uint64, error)
}

// Publisher writes one producer's readiness envelope to its GRAPH_STATUS key. One
// instance per producer; Publish is called from the status tick and is safe to call
// from any single goroutine.
type Publisher struct {
	bucket StatusWriter
	key    string
}

// NewPublisher builds a publisher for one producer key (KeyGraphIndex,
// KeyGraphEmbedding, KeyGraphIngest). It refuses a nil bucket and an empty key, so a
// producer wired without either fails its Start instead of publishing nothing while
// consumers read the key as absent. A typed-nil bucket is not detected.
func NewPublisher(bucket StatusWriter, key string) (*Publisher, error) {
	if bucket == nil {
		return nil, errors.New("readiness: nil status bucket")
	}
	if key == "" {
		return nil, errors.New("readiness: empty producer key")
	}
	return &Publisher{bucket: bucket, key: key}, nil
}

// Publish writes the envelope as the KV value for this producer's key.
//
// The value is PLAIN graph.IndexStatusResponse JSON, with no BaseMessage wrapper. The
// payload registry governs polymorphic message publishes on subjects, where a receiver
// must discriminate a type it did not choose; this is a KV value on a fixed key whose
// type is fixed by the contract, and wrapping it would break every reader's plain
// decode, the Watcher's included, for nothing.
//
// Callers publish on EVERY tick, unconditionally, without comparing to the last
// value: the write is the liveness heartbeat that lets a consumer tell "not ready"
// from "the producer is gone", and skipping unchanged values would make a healthy
// steady state indistinguishable from a dead one.
//
// Publish sets the envelope's PublishedAt to the wall-clock time of this write, in UTC,
// RFC 3339 with nanoseconds, replacing whatever the caller passed, so no producer can
// leave it out or carry an old one forward. It states when the producer wrote, not when
// anything in the graph was asserted.
//
// The error is returned, never swallowed, so the tick loop can count and log it; the
// loop must keep ticking regardless, because the next heartbeat is the recovery. A nil
// context is refused with an error before any write.
func (p *Publisher) Publish(ctx context.Context, status graph.IndexStatusResponse) error {
	if ctx == nil {
		return errors.New("readiness: nil context")
	}
	status.PublishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	data, err := json.Marshal(status)
	if err != nil {
		// Unreachable (strings, numbers, bools and one map with string keys) but
		// wrapped rather than ignored: a silent marshal failure would stop the
		// heartbeat with no evidence.
		return fmt.Errorf("readiness: marshal status for %s/%s: %w", graph.BucketGraphStatus, p.key, err)
	}
	putCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()
	if _, err := p.bucket.Put(putCtx, p.key, data); err != nil {
		return fmt.Errorf("readiness: put %s/%s: %w", graph.BucketGraphStatus, p.key, err)
	}
	return nil
}

// Key reports the producer key this publisher writes, for the caller's failure log.
func (p *Publisher) Key() string {
	return p.key
}
