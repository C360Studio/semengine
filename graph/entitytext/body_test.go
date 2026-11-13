package entitytext_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"
	"unicode/utf8"

	"github.com/c360studio/semengine/graph/entitytext"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/storage"
)

// errStoreTimeout is the error the NATS object store returns when a read's deadline passes with
// the read in flight: its own timeout, which wraps no context error.
var errStoreTimeout = errors.New("nats: timeout")

// stores is a store registry: instance name to store.
type stores map[string]storage.StreamableStore

func (s stores) Streamable(instance string) (storage.StreamableStore, bool) {
	store, ok := s[instance]
	return store, ok
}

// fakeStore is a streamable store whose Open each test supplies; nothing else is read.
type fakeStore struct {
	open func(ctx context.Context, key string) (io.ReadCloser, error)
}

func (fakeStore) Put(context.Context, string, []byte) error      { return nil }
func (fakeStore) Get(context.Context, string) ([]byte, error)    { return nil, errors.New("not read") }
func (fakeStore) List(context.Context, string) ([]string, error) { return nil, nil }
func (fakeStore) Delete(context.Context, string) error           { return nil }
func (s fakeStore) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.open(ctx, key)
}

// serving is a store that answers every key with body at once.
func serving(body string) fakeStore {
	return fakeStore{open: func(context.Context, string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(body)), nil
	}}
}

// blockedReader is a read in flight that blocks until the context its stream was opened under
// ends, then fails with the store's own timeout.
type blockedReader struct{ ctx context.Context }

func (r blockedReader) Read([]byte) (int, error) {
	<-r.ctx.Done()
	return 0, errStoreTimeout
}

func (blockedReader) Close() error { return nil }

var ref = message.StorageReference{StorageInstance: "objstore", Key: "body/1"}

// TestBodyReadDeadline: a body read runs under a context that ends 10 seconds after the read
// begins, and when that context has ended by the time the store returns, the read says the
// deadline passed, whatever the store's error was. The stores here fail as the NATS object store
// does at its deadline: they wait for the context, then return their own timeout.
//
// Requirement: entity-text/An offloaded body is read with a deadline
// Requirement: entity-text/The read's own context decides that the deadline passed
func TestBodyReadDeadline(t *testing.T) {
	t.Run("the store sees a 10-second deadline", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var deadline time.Time
			var hasDeadline bool
			store := fakeStore{open: func(ctx context.Context, _ string) (io.ReadCloser, error) {
				deadline, hasDeadline = ctx.Deadline()
				return io.NopCloser(strings.NewReader("the body")), nil
			}}
			began := time.Now()
			body, err := entitytext.ReadBody(t.Context(), stores{"objstore": store}, ref, 100)
			if err != nil || body != "the body" {
				t.Fatalf("ReadBody = %q, %v; want the body", body, err)
			}
			if !hasDeadline || !deadline.Equal(began.Add(10*time.Second)) {
				t.Fatalf("store's deadline = %v (set %v), want %v", deadline, hasDeadline, began.Add(10*time.Second))
			}
		})
	})

	blocking := map[string]fakeStore{
		"an open that waits for its context": {open: func(ctx context.Context, _ string) (io.ReadCloser, error) {
			<-ctx.Done()
			return nil, errStoreTimeout
		}},
		"a read in flight that waits for its context": {open: func(ctx context.Context, _ string) (io.ReadCloser, error) {
			return blockedReader{ctx: ctx}, nil
		}},
	}
	for name, store := range blocking {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				began := time.Now()
				body, err := entitytext.ReadBody(t.Context(), stores{"objstore": store}, ref, 100)
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("ReadBody = %q, %v; want a deadline error", body, err)
				}
				if elapsed := time.Since(began); elapsed != 10*time.Second {
					t.Fatalf("ReadBody returned after %v, want 10s", elapsed)
				}
			})
		})
	}

	t.Run("a caller's cancel is not a deadline", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			store := fakeStore{open: func(ctx context.Context, _ string) (io.ReadCloser, error) {
				cancel()
				<-ctx.Done()
				return nil, errStoreTimeout
			}}
			_, err := entitytext.ReadBody(ctx, stores{"objstore": store}, ref, 100)
			if !errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("ReadBody = %v; want the caller's cancel, not a deadline", err)
			}
		})
	})
}

// TestBodyReadOutcomes: a body read tells a missing object, an instance with no registered
// store and any other failure apart with errors.Is, reads only from the store registered under
// the reference's instance, and reads at most enough bytes for the cap.
//
// Requirement: entity-text/A body read's failures are told apart
func TestBodyReadOutcomes(t *testing.T) {
	failing := func(openErr error) fakeStore {
		return fakeStore{open: func(context.Context, string) (io.ReadCloser, error) { return nil, openErr }}
	}
	notFound := failing(fmt.Errorf("get %q: %w", ref.Key, storage.ErrObjectNotFound))
	other := failing(errors.New("bucket deleted"))

	cases := []struct {
		name      string
		stores    entitytext.Stores
		ref       message.StorageReference
		missing   bool
		noStore   bool
		otherFail bool
	}{
		{name: "missing", stores: stores{"objstore": notFound}, ref: ref, missing: true},
		{name: "no store for the instance", stores: stores{"elsewhere": serving("not this one")}, ref: ref, noStore: true},
		{name: "no instance", stores: stores{"": serving("not this one")}, ref: message.StorageReference{Key: "k"}, noStore: true},
		{name: "no registry", stores: nil, ref: ref, noStore: true},
		{name: "an open that fails", stores: stores{"objstore": other}, ref: ref, otherFail: true},
		{name: "a read that fails", stores: stores{"objstore": fakeStore{open: func(context.Context, string) (io.ReadCloser, error) {
			return io.NopCloser(io.MultiReader(strings.NewReader("part"), iotestErrReader{})), nil
		}}}, ref: ref, otherFail: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := entitytext.ReadBody(t.Context(), tc.stores, tc.ref, 100)
			if err == nil {
				t.Fatal("ReadBody returned no error")
			}
			if got := errors.Is(err, storage.ErrObjectNotFound); got != tc.missing {
				t.Errorf("errors.Is(%v, ErrObjectNotFound) = %v, want %v", err, got, tc.missing)
			}
			if got := errors.Is(err, entitytext.ErrNoStore); got != tc.noStore {
				t.Errorf("errors.Is(%v, ErrNoStore) = %v, want %v", err, got, tc.noStore)
			}
			if errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("errors.Is(%v, DeadlineExceeded) = true, want false", err)
			}
			if tc.otherFail && (errors.Is(err, storage.ErrObjectNotFound) || errors.Is(err, entitytext.ErrNoStore)) {
				t.Errorf("a store failure %v reads as missing or as no store", err)
			}
		})
	}

	t.Run("the instance's own store", func(t *testing.T) {
		registry := stores{"objstore": serving("mine"), "other": serving("not mine")}
		if body, err := entitytext.ReadBody(t.Context(), registry, ref, 100); err != nil || body != "mine" {
			t.Fatalf("ReadBody = %q, %v; want %q", body, err, "mine")
		}
	})
	t.Run("at most enough bytes for the cap", func(t *testing.T) {
		full := strings.Repeat("é", 50)
		body, err := entitytext.ReadBody(t.Context(), stores{"objstore": serving(full)}, ref, 5)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(full, body) || utf8.RuneCountInString(body) < 5 || len(body) > 5*utf8.UTFMax+1 {
			t.Fatalf("ReadBody read %d bytes (%q), want a prefix holding the 5 capped characters in at most %d bytes",
				len(body), body, 5*utf8.UTFMax+1)
		}
	})
	t.Run("a nil context is refused before any store is asked", func(t *testing.T) {
		opened := false
		store := fakeStore{open: func(context.Context, string) (io.ReadCloser, error) {
			opened = true
			return io.NopCloser(strings.NewReader("body")), nil
		}}
		if _, err := entitytext.ReadBody(nil, stores{"objstore": store}, ref, 100); !errs.IsInvalid(err) {
			t.Fatalf("ReadBody(nil) = %v; want an invalid-input error", err)
		}
		if opened {
			t.Fatal("ReadBody(nil) opened the store")
		}
	})
	t.Run("a cap below one is refused", func(t *testing.T) {
		if _, err := entitytext.ReadBody(t.Context(), stores{"objstore": serving("body")}, ref, 0); err == nil {
			t.Fatal("ReadBody with a cap of 0 returned no error")
		}
	})
}

// iotestErrReader fails every read, as a stream cut off mid-body does.
type iotestErrReader struct{}

func (iotestErrReader) Read([]byte) (int, error) { return 0, errors.New("stream interrupted") }
