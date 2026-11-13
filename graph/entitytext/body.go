package entitytext

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
	"unicode/utf8"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/storage"
)

// readTimeout bounds one body read, from the open to the last byte.
const readTimeout = 10 * time.Second

// ErrNoStore says no store is registered under a body's storage instance. A missing object is
// storage.ErrObjectNotFound; any other failure wraps neither.
var ErrNoStore = errors.New("entitytext: no store is registered for the storage instance")

// Stores finds the store registered under a storage instance; *storeregistry.Registry is one.
// The store is borrowed for one read, never kept or closed.
type Stores interface {
	Streamable(instance string) (storage.StreamableStore, bool)
}

// ReadBody reads the offloaded body ref points to from the store registered under its instance,
// and no other, reading at most enough bytes to hold maxLen characters; the caller cuts it to the
// cap. The read runs under a context derived from ctx that ends 10 seconds after it begins.
//
// When that context has ended by the time the store returns, the error wraps the context's error,
// context.DeadlineExceeded for the deadline, whatever the store returned: a store may answer its
// deadline with an error of its own that wraps no context error. A store that ignores its context
// is not bounded by the deadline.
func ReadBody(ctx context.Context, stores Stores, ref message.StorageReference, maxLen int) (string, error) {
	if maxLen < 1 {
		return "", fmt.Errorf("entitytext: read body %q: cap %d is below 1 character", ref.Key, maxLen)
	}
	var store storage.StreamableStore
	if stores != nil && ref.StorageInstance != "" {
		store, _ = stores.Streamable(ref.StorageInstance)
	}
	if store == nil {
		return "", fmt.Errorf("%w: %q", ErrNoStore, ref.StorageInstance)
	}

	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	body, err := read(readCtx, store, ref.Key, maxLen)
	if err != nil {
		if ctxErr := readCtx.Err(); ctxErr != nil {
			return "", fmt.Errorf("entitytext: read body %q from %q: %w (store: %v)", ref.Key, ref.StorageInstance, ctxErr, err)
		}
		return "", fmt.Errorf("entitytext: read body %q from %q: %w", ref.Key, ref.StorageInstance, err)
	}
	return body, nil
}

// read opens key on store and reads it, up to maxLen characters' worth of bytes.
func read(ctx context.Context, store storage.StreamableStore, key string, maxLen int) (string, error) {
	reader, err := store.Open(ctx, key)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer reader.Close()

	// A character is at most utf8.UTFMax bytes, so this budget holds maxLen whole characters when
	// the body has them; the one byte more shows the body went past them.
	data, err := io.ReadAll(io.LimitReader(reader, int64(maxLen)*utf8.UTFMax+1))
	if err != nil {
		return "", fmt.Errorf("read: %w", err)
	}
	return string(data), nil
}
