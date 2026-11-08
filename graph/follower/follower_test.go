package follower

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/revlag"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// TestOpenRefusesMissingInputs: Open refuses each input the entity-state-follower spec names
// ("Open waits for ENTITY_STATES within a bound") before it makes any NATS call, so a zero client
// value is enough to reach every check after the client's.
func TestOpenRefusesMissingInputs(t *testing.T) {
	valid := func() Config {
		return Config{
			Client:  &natsclient.Client{},
			Key:     "graph-index",
			Deliver: func(context.Context, Entry) {},
			Failed:  func() int64 { return 0 },
		}
	}
	cases := []struct {
		name  string
		ctx   context.Context
		edit  func(*Config)
		names string
	}{
		{"nil context", nil, func(*Config) {}, "nil context"},
		{"nil client", t.Context(), func(c *Config) { c.Client = nil }, "nil client"},
		{"empty key", t.Context(), func(c *Config) { c.Key = "" }, "empty status key"},
		{"nil Deliver", t.Context(), func(c *Config) { c.Deliver = nil }, "nil Deliver"},
		{"nil Failed", t.Context(), func(c *Config) { c.Failed = nil }, "nil Failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid()
			tc.edit(&cfg)
			f, err := Open(tc.ctx, cfg)
			if err == nil || f != nil {
				t.Fatalf("Open = %v, %v; want a refusal", f, err)
			}
			if !errs.IsInvalid(err) {
				t.Errorf("Open error %v is not class invalid", err)
			}
			if !strings.Contains(err.Error(), tc.names) {
				t.Errorf("Open error %q does not name %q", err, tc.names)
			}
		})
	}
}

// openWatcher is a jetstream.KeyWatcher whose update channel never closes: the state nats.go
// v1.55.0 leaves a watch in when its context ends between Subscribe and the install of the
// handler that closes the channel (#454).
type openWatcher struct{ updates chan jetstream.KeyValueEntry }

func (w openWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w openWatcher) Stop() error                             { return nil }

// TestStopWatchGivesUpOnADeliveryThatNeverCloses: after Run's context ends, stopWatch waits for
// the watch's delivery to close for at most the named budget, 5 s by the entity-state-follower
// spec ("The watch's delivery never closes"), then reports that it did not close. The bubble's
// clock advances only when every goroutine in it is blocked, so time.Since reads exactly how long
// stopWatch waited; with no bound the bubble deadlocks and the test fails.
func TestStopWatchGivesUpOnADeliveryThatNeverCloses(t *testing.T) {
	if watchDrainBudget != 5*time.Second {
		t.Fatalf("watchDrainBudget = %v; the spec names 5s", watchDrainBudget)
	}
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		start := time.Now()
		err := stopWatch(ctx, openWatcher{updates: make(chan jetstream.KeyValueEntry)})
		if !errors.Is(err, errWatchDeliveryOpen) {
			t.Fatalf("stopWatch = %v; want errWatchDeliveryOpen", err)
		}
		if elapsed := time.Since(start); elapsed != watchDrainBudget {
			t.Errorf("stopWatch returned after %v; want %v", elapsed, watchDrainBudget)
		}
	})
}

// putEntry is a delivered live write: only what observe reads is implemented.
type putEntry struct {
	jetstream.KeyValueEntry
	key      string
	revision uint64
}

func (e putEntry) Key() string                     { return e.key }
func (e putEntry) Revision() uint64                { return e.revision }
func (e putEntry) Created() time.Time              { return time.Unix(int64(e.revision), 0) }
func (e putEntry) Operation() jetstream.KeyValueOp { return jetstream.KeyValuePut }
func (e putEntry) Value() []byte                   { return []byte("{}") }

// closingWatcher delivers what its channel holds and closes the channel on Stop, as nats.go's
// watcher does once it is stopped.
type closingWatcher struct {
	updates chan jetstream.KeyValueEntry
	once    sync.Once
}

func (w *closingWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w *closingWatcher) Stop() error {
	w.once.Do(func() { close(w.updates) })
	return nil
}

// watchOnlyReader is a kvcatalog.CatalogReader with only WatchAll implemented: follow calls
// nothing else.
type watchOnlyReader struct {
	kvcatalog.CatalogReader
	watcher jetstream.KeyWatcher
}

func (r watchOnlyReader) WatchAll(context.Context, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return r.watcher, nil
}

// TestFollowerDeclaresLostFeedOnOutOfOrderDelivery: a watch that delivers revision 5 and then
// revision 3 has broken the ascending order the floor rests on, so the follower declares the feed
// lost (entity-state-follower spec, "A revision delivered out of order"), and still delivers the
// entry (design D2). The watch delivers the snapshot marker first, so the two writes are live and
// no stored entity is validated.
func TestFollowerDeclaresLostFeedOnOutOfOrderDelivery(t *testing.T) {
	updates := make(chan jetstream.KeyValueEntry, 3)
	updates <- nil
	updates <- putEntry{key: "k5", revision: 5}
	updates <- putEntry{key: "k3", revision: 3}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var delivered []uint64
	f := &Follower{
		key:    "graph-index",
		logger: slog.New(slog.DiscardHandler),
		deliver: func(_ context.Context, e Entry) {
			delivered = append(delivered, e.Revision)
			if e.Revision == 3 {
				cancel() // both writes are delivered: follow stops the watch and returns
			}
		},
		failed:    func() int64 { return 0 },
		reader:    watchOnlyReader{watcher: &closingWatcher{updates: updates}},
		watermark: revlag.New(),
	}

	// follow runs on this goroutine, so its return orders the reads below after both deliveries.
	if err := f.follow(ctx); err != nil {
		t.Fatalf("follow = %v", err)
	}
	if !slices.Equal(delivered, []uint64{5, 3}) {
		t.Errorf("delivered revisions %v; want [5 3]", delivered)
	}
	// Fatal: without a lost feed QueryReady would go on to read the stream status, which the fake
	// reader does not implement.
	if !slices.ContainsFunc(f.Stuck(), func(s string) bool { return strings.Contains(s, "feed lost") }) {
		t.Fatalf("Stuck = %q; want the lost feed named", f.Stuck())
	}
	var ce *errs.ClassifiedError
	err := f.QueryReady(t.Context())
	if !errors.As(err, &ce) || ce.Class != errs.ErrorTransient || ce.Code != graph.ErrorCodeIndexNotReady ||
		!strings.Contains(err.Error(), "feed is lost") {
		t.Errorf("QueryReady = %v; want transient %s for the lost feed", err, graph.ErrorCodeIndexNotReady)
	}
}
