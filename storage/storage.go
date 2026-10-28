// Package storage provides pluggable backend interfaces for storage operations.
package storage

import (
	"context"
	"errors"
	"io"
)

// ErrObjectNotFound is the backend-agnostic sentinel a Store.Get returns (wrapped
// with %w) when the key resolved to NO stored object — as opposed to a transient
// backend fault. Callers match it with errors.Is to distinguish "the object is
// absent" (e.g. content-addressed evidence that was reclaimed, #600) from "the
// read faulted", which is a genuine error worth retrying/alerting. A not-found is
// deliberately NOT classified as transient: retrying will not conjure the object.
var ErrObjectNotFound = errors.New("object not found")

// Store is the pluggable backend interface for storage operations.
//
// Each storage component instance creates its own Store implementation with its
// own configuration (bucket name, connection string, etc.). Multiple Store instances
// can run concurrently, each backing a different storage component.
//
// The Store interface uses a simple key-value pattern where:
//   - Keys are strings (hierarchical paths supported via "/" separators)
//   - Values are binary data ([]byte) - supports JSON, videos, images, any binary format
//   - Operations are context-aware for cancellation and timeouts
//
// Example implementations:
//   - objectstore.Store: NATS JetStream ObjectStore backend
//   - s3store.Store: AWS S3 or MinIO backend (future)
//   - sqlstore.Store: PostgreSQL with (key, data) table (future)
//
// Thread Safety:
// All Store implementations must be safe for concurrent use from multiple goroutines.
//
// Example Usage:
//
//	// Create a NATS ObjectStore backend; the Store has nothing to close
//	store, err := objectstore.NewStoreWithConfig(ctx, natsClient, objectstore.Config{BucketName: "MESSAGES"})
//
//	// Store video file
//	videoData := []byte{...}
//	err = store.Put(ctx, "video/sensor-123/2024-10-08.mp4", videoData)
//
//	// Retrieve video
//	data, err := store.Get(ctx, "video/sensor-123/2024-10-08.mp4")
//
//	// List all videos for a sensor
//	keys, err := store.List(ctx, "video/sensor-123/")
type Store interface {
	// Put stores binary data at the specified key.
	// If the key already exists, behavior is implementation-specific:
	//   - NATS ObjectStore (objectstore.Store) replaces the object and keeps no
	//     earlier version. Observed with nats.go v1.54.0 and nats-server v2.15.1
	//     (probe P4, PR #432 comment 6445520357); the library does not promise it.
	//   - Mutable stores (S3, SQL) overwrite the existing value
	//
	// The data parameter accepts any binary format:
	//   - JSON-encoded messages
	//   - Video files (MP4, etc.)
	//   - Images (JPEG, PNG, etc.)
	//   - Any []byte data
	Put(ctx context.Context, key string, data []byte) error

	// Get retrieves binary data for the specified key.
	// Returns an error if the key does not exist.
	//
	// The returned []byte should be interpreted by the caller based on
	// their knowledge of what was stored (JSON, video, etc.).
	Get(ctx context.Context, key string) ([]byte, error)

	// List returns all keys matching the specified prefix.
	// The prefix parameter supports hierarchical key patterns:
	//   - "" (empty) lists all keys
	//   - "video/" lists all keys starting with "video/"
	//   - "video/sensor-123/" lists keys for a specific sensor
	//
	// Keys are returned in lexicographic order.
	// Returns an empty slice if no keys match the prefix.
	List(ctx context.Context, prefix string) ([]string, error)

	// Delete removes the data at the specified key.
	// Returns nil if the key doesn't exist (idempotent operation).
	//
	// objectstore.Store removes the object; it keeps no earlier versions to
	// remove (see Put).
	Delete(ctx context.Context, key string) error
}

// StreamableStore extends Store with streaming read support.
// Implementations that support chunked/streamed reads (NATS ObjectStore,
// filesystem) implement this for large content without loading everything
// into memory. Backends without native streaming wrap Get() bytes in
// an io.NopCloser(bytes.NewReader(data)).
type StreamableStore interface {
	Store

	// Open returns a streaming reader for the content at key.
	// The caller MUST close the reader when done.
	Open(ctx context.Context, key string) (io.ReadCloser, error)
}
