package lifecycle

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// fixtureSource and fixtureTime stamp every statement this package's tests
// store themselves, as another producer's write would carry them. graph-ingest's
// stored-value rule (graph.UnmarshalEntityState) refuses a statement with no
// Source or no Timestamp with graph_state_reset_required, and this package's
// doubles apply the same rule, so an unstamped value is a state the real
// authority would never answer with a success.
const fixtureSource = "lifecycle-test-producer"

var fixtureTime = time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)

// productionExactReader is graph.NewExactEntityReader, the reader production
// managers use, over a test requester.
func productionExactReader(t *testing.T, requester interface {
	RequestClassified(context.Context, string, []byte, time.Duration) ([]byte, error)
}) *graph.ExactEntityReader {
	t.Helper()
	reader, err := graph.NewExactEntityReader(requester, time.Second)
	if err != nil {
		t.Fatalf("NewExactEntityReader: %v", err)
	}
	return reader
}

// bucketExactEntityReader adapts the in-memory KV fixture used by lifecycle
// tests. Production managers always use graph.NewExactEntityReader over the
// admitted request/reply operation; there is no direct-KV production fallback.
type bucketExactEntityReader struct {
	bucket jetstream.KeyValue
}

func (r bucketExactEntityReader) ReadExactEntity(ctx context.Context, entityID string) (*graph.ExactEntity, error) {
	if r.bucket == nil {
		return nil, jetstream.ErrKeyNotFound
	}
	entry, err := r.bucket.Get(ctx, entityID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeEntityNotFound, err)
		}
		return nil, err
	}
	var entity graph.EntityState
	if err := graph.UnmarshalEntityState(entityID, entry.Value(), &entity); err != nil {
		return nil, err
	}
	return &graph.ExactEntity{Entity: &entity, KVRevision: entry.Revision()}, nil
}

func newManagerForTest(logger *slog.Logger, emitter graphEmitter, bucket jetstream.KeyValue) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		logger:             logger,
		emitter:            emitter,
		exactReader:        bucketExactEntityReader{bucket: bucket},
		entityStatesBucket: bucket,
		registrations:      make(map[string]*registration),
	}
}

// refusedStoredValue is a value stored under key that graph-ingest's
// stored-value rule refuses, with the reason it gives.
type refusedStoredValue struct {
	name   string
	value  []byte
	reason graph.StateResetReason
}

// refusedStoredValues builds one value for each refusal of graph-ingest's
// stored-value rule beyond decoding: a statement with no Source, one with no
// Timestamp, and a value naming another entity than its key.
func refusedStoredValues(t *testing.T, key string) []refusedStoredValue {
	t.Helper()
	encode := func(id, source string, timestamp time.Time) []byte {
		t.Helper()
		body, err := graph.MarshalEntityState(&graph.EntityState{ID: id, Triples: []message.Triple{{
			Subject: id, Predicate: "mission.lifecycle.phase", Object: "planning", Source: source, Timestamp: timestamp,
		}}})
		if err != nil {
			t.Fatalf("MarshalEntityState: %v", err)
		}
		return body
	}
	other := key + "-other"
	return []refusedStoredValue{
		{"a statement with no source", encode(key, "", fixtureTime), graph.GraphStateReasonMissingStatementMetadata},
		{"a statement with no timestamp", encode(key, fixtureSource, time.Time{}),
			graph.GraphStateReasonMissingStatementMetadata},
		{"a value naming another entity", encode(other, fixtureSource, fixtureTime),
			graph.GraphStateReasonNoncanonicalEntityID},
	}
}

// requireStoredValueRefused checks err is graph-ingest's refusal of a stored
// value: fatal graph_state_reset_required, naming reason. The reason is read
// from the message, which is what survives a NATS reply.
func requireStoredValueRefused(t *testing.T, err error, reason graph.StateResetReason) {
	t.Helper()
	var classified *errs.ClassifiedError
	if !errors.As(err, &classified) || classified.Class != errs.ErrorFatal ||
		classified.Code != graph.ErrorCodeGraphStateResetRequired {
		t.Fatalf("error = %v, want fatal %s", err, graph.ErrorCodeGraphStateResetRequired)
	}
	if !strings.Contains(err.Error(), string(reason)) {
		t.Fatalf("error = %v, want reason %q", err, reason)
	}
}
