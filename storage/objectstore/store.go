package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/storage"
)

var (
	_ storage.Store           = (*Store)(nil)
	_ storage.StreamableStore = (*Store)(nil)
)

// Store is a storage.StreamableStore over one NATS ObjectStore bucket. It is
// safe for concurrent use. It keeps no copy of any object: every read goes to
// NATS. It holds no context and runs no goroutine, so it has nothing to close.
type Store struct {
	instanceName string
	objects      jetstream.ObjectStore
	metrics      *storeMetrics
}

// NewStoreWithConfig creates the configured bucket, or opens it when it exists,
// and removes any TTL or size limit from it (see the package doc). It records no
// metrics.
func NewStoreWithConfig(ctx context.Context, client *natsclient.Client, cfg Config) (*Store, error) {
	if ctx == nil {
		return nil, nilContext("NewStoreWithConfig")
	}
	return NewStoreWithConfigAndMetrics(ctx, client, cfg, nil)
}

// NewStoreWithConfigAndMetrics is NewStoreWithConfig recording the Store's
// metrics on registry. A nil registry records nothing.
func NewStoreWithConfigAndMetrics(
	ctx context.Context, client *natsclient.Client, cfg Config, registry *metric.MetricsRegistry,
) (*Store, error) {
	if ctx == nil {
		return nil, nilContext("NewStoreWithConfigAndMetrics")
	}
	if client == nil {
		return nil, errs.WrapInvalid(errors.New("nil client"), "Store", "NewStoreWithConfigAndMetrics",
			"missing NATS client")
	}
	cfg = cfg.withDefaults()

	js, err := client.JetStream()
	if err != nil {
		return nil, errs.WrapInvalid(err, "Store", "NewStoreWithConfigAndMetrics", "get JetStream context")
	}

	// No TTL and no size limit: objects are pointed at by graph entities that
	// outlive any age (ADR-068). The guard below removes either from an
	// existing bucket.
	objects, err := js.CreateObjectStore(ctx, jetstream.ObjectStoreConfig{
		Bucket:      cfg.BucketName,
		Description: "Immutable message storage",
	})
	if err != nil {
		// The bucket exists with another config, or the create failed for another
		// reason: open it, and report the open's error if that fails too.
		objects, err = js.ObjectStore(ctx, cfg.BucketName)
		if err != nil {
			return nil, errs.WrapTransient(err, "Store", "NewStoreWithConfigAndMetrics", "create/get object store")
		}
	}

	if err := reconcileNoLifecycleRetention(ctx, js, cfg.BucketName, cfg.Logger); err != nil {
		return nil, err
	}

	metrics, err := newStoreMetrics(registry, cfg.BucketName)
	if err != nil {
		return nil, errs.WrapInvalid(err, "Store", "NewStoreWithConfigAndMetrics", "register metrics")
	}

	return &Store{instanceName: cfg.InstanceName, objects: objects, metrics: metrics}, nil
}

// InstanceName returns the value the Store stamps into
// StorageReference.StorageInstance: Config.InstanceName, or the bucket name.
func (s *Store) InstanceName() string {
	return s.instanceName
}

// Put stores data at key, replacing any object already there. NATS keeps no
// earlier version.
func (s *Store) Put(ctx context.Context, key string, data []byte) (err error) {
	defer s.metrics.recordError("put", &err)
	if ctx == nil {
		return nilContext("Put")
	}
	start := time.Now()
	s.metrics.recordWriteOp("put")
	if _, err := s.objects.PutBytes(ctx, key, data); err != nil {
		return errs.WrapTransient(err, "Store", "Put", "put data")
	}
	s.metrics.recordWriteLatency("put", start)
	return nil
}

// Get returns the bytes stored at key. A missing object returns an error
// matching storage.ErrObjectNotFound, which is not transient.
func (s *Store) Get(ctx context.Context, key string) (_ []byte, err error) {
	defer s.metrics.recordError("get", &err)
	if ctx == nil {
		return nil, nilContext("Get")
	}
	start := time.Now()
	s.metrics.recordReadOp("get")
	data, err := s.objects.GetBytes(ctx, key)
	if err != nil {
		return nil, readError(err, "Get", key)
	}
	s.metrics.recordReadLatency("get", start)
	return data, nil
}

// Open returns a reader that streams the object at key in chunks. The caller
// closes it. A missing object returns an error matching
// storage.ErrObjectNotFound, which is not transient.
func (s *Store) Open(ctx context.Context, key string) (_ io.ReadCloser, err error) {
	defer s.metrics.recordError("open", &err)
	if ctx == nil {
		return nil, nilContext("Open")
	}
	start := time.Now()
	s.metrics.recordReadOp("open")
	result, err := s.objects.Get(ctx, key)
	if err != nil {
		return nil, readError(err, "Open", key)
	}
	s.metrics.recordReadLatency("open", start)
	return result, nil
}

// readError maps a missing object to storage.ErrObjectNotFound, unclassified so
// it is not transient (a retry will not bring the object back), and classifies
// any other read failure as transient.
func readError(err error, method, key string) error {
	if errors.Is(err, jetstream.ErrObjectNotFound) {
		return fmt.Errorf("Store.%s %q: %w", method, key, storage.ErrObjectNotFound)
	}
	return errs.WrapTransient(err, "Store", method, "read object")
}

// Delete removes the object at key. A key with no object returns nil.
func (s *Store) Delete(ctx context.Context, key string) (err error) {
	defer s.metrics.recordError("delete", &err)
	if ctx == nil {
		return nilContext("Delete")
	}
	start := time.Now()
	s.metrics.recordDeleteOp()
	if err := s.objects.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrObjectNotFound) {
		return errs.WrapTransient(err, "Store", "Delete", "delete object")
	}
	s.metrics.recordDeleteLatency(start)
	return nil
}

// List returns the names of the objects that start with prefix, sorted, and an
// empty slice when none do. It reads every object's metadata from NATS and
// filters here, so its cost grows with the number of objects in the bucket.
func (s *Store) List(ctx context.Context, prefix string) (_ []string, err error) {
	defer s.metrics.recordError("list", &err)
	if ctx == nil {
		return nil, nilContext("List")
	}
	start := time.Now()
	s.metrics.recordListOp()
	entries, err := s.objects.List(ctx)
	if err != nil && !errors.Is(err, jetstream.ErrNoObjectsFound) {
		return nil, errs.WrapTransient(err, "Store", "List", "list objects")
	}
	keys := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name, prefix) {
			keys = append(keys, entry.Name)
		}
	}
	sort.Strings(keys)
	s.metrics.recordListLatency(start)
	return keys, nil
}

// nilContext is the refusal every exported call returns for a nil context,
// before any I/O.
func nilContext(method string) error {
	return errs.WrapInvalid(errors.New("nil context"), "Store", method, "missing context")
}
