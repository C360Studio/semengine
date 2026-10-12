package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// productionExactReader is graph.NewExactEntityReader, the reader production
// managers use, over a test requester.
func productionExactReader(t *testing.T, requester interface {
	RequestClassified(context.Context, string, []byte, time.Duration) ([]byte, error)
}) graph.ExactEntityReader {
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
	if err := graph.UnmarshalEntityState(entry.Value(), &entity); err != nil {
		return nil, fmt.Errorf("decode fixture entity: %w", err)
	}
	return &graph.ExactEntity{Entity: entity.Clone(), KVRevision: entry.Revision()}, nil
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
