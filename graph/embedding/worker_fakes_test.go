package embedding

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

// discardLogger silences the Worker in tests that assert on state rather than logs. It is
// passed explicitly, never through slog.SetDefault.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// watchKV is a memKV whose Watch delivers what a real watch delivers: the current value of
// every key, the nil end-of-snapshot marker, then every later write and removal, in the order
// they were made. Worker tests run the production path through it (Run -> Watch -> keyed pool
// -> Storage), and the Worker sees its own writes as it does on a real bucket.
//
// deleteErr, when set, is returned by every Delete, as a transport failure would be.
// failUpdates makes every Update conflict, so a compare-and-set never converges. Like memKV's,
// Delete ignores a revision fence: the fence is proven against a real server.
type watchKV struct {
	*memKV

	// order holds a write and its delivery together, so deliveries follow write order.
	order    sync.Mutex
	watchers []*fanWatcher
	watches  atomic.Int32

	deleteErr   error
	failUpdates atomic.Bool
}

func newWatchKV() *watchKV { return &watchKV{memKV: newMemKV()} }

// fanWatcher is one watch on a watchKV. Stop closes its channel, as nats.go closes a stopped
// watch's channel; nothing is delivered after that.
type fanWatcher struct {
	mu      sync.Mutex
	closed  bool
	updates chan jetstream.KeyValueEntry
}

func (w *fanWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }

func (w *fanWatcher) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.closed = true
		close(w.updates)
	}
	return nil
}

func (w *fanWatcher) deliver(entry jetstream.KeyValueEntry) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed {
		w.updates <- entry
	}
}

func (k *watchKV) Watch(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	k.watches.Add(1)
	k.order.Lock()
	defer k.order.Unlock()
	w := &fanWatcher{updates: make(chan jetstream.KeyValueEntry, 1024)}
	k.memKV.mu.Lock()
	for key, value := range k.data {
		w.updates <- &memKVEntry{key: key, value: append([]byte(nil), value...), rev: k.revs[key], op: jetstream.KeyValuePut}
	}
	k.memKV.mu.Unlock()
	w.updates <- nil
	k.watchers = append(k.watchers, w)
	return w, nil
}

// deliverPut delivers key's current value to every watch; called with order held.
func (k *watchKV) deliverPut(key string) {
	entry, err := k.memKV.Get(context.Background(), key)
	if err != nil {
		return
	}
	for _, w := range k.watchers {
		w.deliver(entry)
	}
}

func (k *watchKV) Put(ctx context.Context, key string, value []byte) (uint64, error) {
	k.order.Lock()
	defer k.order.Unlock()
	rev, err := k.memKV.Put(ctx, key, value)
	if err == nil {
		k.deliverPut(key)
	}
	return rev, err
}

func (k *watchKV) Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	k.order.Lock()
	defer k.order.Unlock()
	rev, err := k.memKV.Create(ctx, key, value, opts...)
	if err == nil {
		k.deliverPut(key)
	}
	return rev, err
}

func (k *watchKV) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	if k.failUpdates.Load() {
		return 0, jetstream.ErrKeyExists
	}
	k.order.Lock()
	defer k.order.Unlock()
	rev, err := k.memKV.Update(ctx, key, value, revision)
	if err == nil {
		k.deliverPut(key)
	}
	return rev, err
}

func (k *watchKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	if k.deleteErr != nil {
		return k.deleteErr
	}
	k.order.Lock()
	defer k.order.Unlock()
	if err := k.memKV.Delete(ctx, key, opts...); err != nil {
		return err
	}
	for _, w := range k.watchers {
		w.deliver(&memKVEntry{key: key, op: jetstream.KeyValueDelete})
	}
	return nil
}

// stubEmbedder is an Embedder whose Generate each test supplies.
type stubEmbedder struct {
	model      string
	dimensions int
	generate   func(texts []string) ([][]float32, error)
}

func (e *stubEmbedder) Generate(_ context.Context, texts []string) ([][]float32, error) {
	return e.generate(texts)
}

func (e *stubEmbedder) GenerateQuery(ctx context.Context, texts []string) ([][]float32, error) {
	return e.Generate(ctx, texts)
}

func (e *stubEmbedder) Dimensions() int { return e.dimensions }
func (e *stubEmbedder) Model() string   { return e.model }
func (e *stubEmbedder) Close() error    { return nil }

// fixedEmbedder returns a stubEmbedder that embeds every text as {1, 2, 3}.
func fixedEmbedder() *stubEmbedder {
	return &stubEmbedder{model: "m", dimensions: 3, generate: func([]string) ([][]float32, error) {
		return [][]float32{{1, 2, 3}}, nil
	}}
}

// runWorker starts w.Run on its own goroutine and returns a function that ends Run's context
// and returns Run's error.
func runWorker(t *testing.T, w *Worker) func() error {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	return func() error { cancel(); return <-done }
}

// stopWorker ends a Run started by runWorker and fails the test unless Run returned
// context.Canceled.
func stopWorker(t *testing.T, stop func() error) {
	t.Helper()
	if err := stop(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v; want context.Canceled", err)
	}
}

// pendingText writes hop 1's pending record for an inline entity.
func pendingText(ctx context.Context, s *Storage, entityID, contentHash, text string, sourceRevision uint64) error {
	_, err := s.SavePendingGuarded(ctx, &Record{
		EntityID: entityID, ContentHash: contentHash, SourceText: text, SourceRevision: sourceRevision,
	})
	return err
}

// pendingRef writes hop 1's pending record for an offloaded entity.
func pendingRef(ctx context.Context, s *Storage, entityID, contentHash, identity string, ref *StorageRef, sourceRevision uint64) error {
	_, err := s.SavePendingGuarded(ctx, &Record{
		EntityID: entityID, ContentHash: contentHash, IdentityText: identity, StorageRef: ref, SourceRevision: sourceRevision,
	})
	return err
}

// terminalEvent is one call of the terminal callback.
type terminalEvent struct {
	entityID string
	revision uint64
	outcome  TerminalOutcome
	reason   string
}

// terminalLog records every terminal callback in the order the calls began.
type terminalLog struct {
	mu     sync.Mutex
	events []terminalEvent
}

func (l *terminalLog) callback(_ context.Context, entityID string, revision uint64, outcome TerminalOutcome, reason string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, terminalEvent{entityID, revision, outcome, reason})
}

func (l *terminalLog) all() []terminalEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]terminalEvent(nil), l.events...)
}

// logRecorder keeps the message of every record at or above a level.
type logRecorder struct {
	min     slog.Level
	mu      sync.Mutex
	records []string
}

func (r *logRecorder) Enabled(_ context.Context, level slog.Level) bool { return level >= r.min }

func (r *logRecorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, rec.Message)
	return nil
}

func (r *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *logRecorder) WithGroup(string) slog.Handler      { return r }

func (r *logRecorder) messages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.records...)
}
