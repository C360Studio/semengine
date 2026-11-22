package config

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/c360studio/semengine/natsclient"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// endedWatcher is a watch whose updates have already ended, as a watch does
// once its subscription is gone.
type endedWatcher struct{ updates chan jetstream.KeyValueEntry }

func newEndedWatcher() endedWatcher {
	updates := make(chan jetstream.KeyValueEntry)
	close(updates)
	return endedWatcher{updates: updates}
}

func (w endedWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w endedWatcher) Stop() error                             { return nil }

// errorRecords keeps every Error record logged through it.
type errorRecords struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *errorRecords) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelError
}

func (h *errorRecords) Handle(_ context.Context, record slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, record.Clone())
	return nil
}

func (h *errorRecords) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *errorRecords) WithGroup(string) slog.Handler      { return h }

// watchNames returns the "watch" attribute of every Error record.
func (h *errorRecords) watchNames() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var names []string
	for _, record := range h.records {
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "watch" {
				names = append(names, attr.Value.String())
			}
			return true
		})
	}
	return names
}

func (h *errorRecords) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.records)
}

// watchLoop runs one of the Manager's two watch loops to its return.
type watchLoop struct {
	name string // the name the loss is reported under
	run  func(cm *Manager, ctx context.Context, watcher jetstream.KeyWatcher)
}

func watchLoops(family *KeyFamily) []watchLoop {
	return []watchLoop{
		{name: "config", run: func(cm *Manager, ctx context.Context, watcher jetstream.KeyWatcher) {
			cm.processWatcher(ctx, watcher)
		}},
		{name: family.prefix, run: func(cm *Manager, ctx context.Context, watcher jetstream.KeyWatcher) {
			cm.processFamily(ctx, family, watcher)
		}},
	}
}

func newWatchLostManager(t *testing.T) (*Manager, *KeyFamily, *errorRecords) {
	t.Helper()
	family, err := NewKeyFamily("rules", func(KeyFamilyEntry) {})
	require.NoError(t, err)
	logs := &errorRecords{}
	cm, err := NewConfigManager(&Config{Platform: PlatformConfig{Org: "acme", ID: "dep"}},
		new(natsclient.Client), slog.New(logs), WithKeyFamily(family))
	require.NoError(t, err)
	return cm, family, logs
}

// When a watch's updates end and nothing stopped the Manager, configuration
// changes no longer reach the process, and nothing reopens the watch. The
// Manager must say so in its log and in Health, naming the watch.
func TestWatchWhoseUpdatesEndIsReportedLost(t *testing.T) {
	t.Parallel()
	_, family, _ := newWatchLostManager(t)
	for _, loop := range watchLoops(family) {
		t.Run(loop.name, func(t *testing.T) {
			t.Parallel()
			cm, _, logs := newWatchLostManager(t)
			require.True(t, cm.Health().Healthy, "healthy before any watch ends: %+v", cm.Health())

			// The loop has returned, so everything it logged and recorded is in place.
			loop.run(cm, t.Context(), newEndedWatcher())

			require.Equal(t, []string{loop.name}, logs.watchNames())
			status := cm.Health()
			require.False(t, status.Healthy, "%+v", status)
			require.Equal(t, "unhealthy", status.Status)
			require.Contains(t, status.Message, loop.name)
			require.Contains(t, status.Message, "restart the process")
		})
	}
}

// Each lost watch is named once in Health, however many of its loops ended.
func TestHealthNamesEveryLostWatchOnce(t *testing.T) {
	t.Parallel()
	cm, family, _ := newWatchLostManager(t)
	// The Manager runs one loop per pattern it watches, all reported as "config".
	cm.processWatcher(t.Context(), newEndedWatcher())
	cm.processWatcher(t.Context(), newEndedWatcher())
	cm.processFamily(t.Context(), family, newEndedWatcher())

	require.Equal(t, "config watch lost (config, rules); restart the process", cm.Health().Message)
}

// A watch ended by Stop is the Manager shutting down, not a lost watch: it is
// neither logged nor reported.
func TestWatchEndedByStopIsNotReportedLost(t *testing.T) {
	t.Parallel()
	_, family, _ := newWatchLostManager(t)
	for _, loop := range watchLoops(family) {
		t.Run(loop.name+"/run context ended", func(t *testing.T) {
			t.Parallel()
			cm, _, logs := newWatchLostManager(t)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			// The loop's select picks at random between the ended context and
			// the ended updates; repeating it makes taking the updates branch,
			// the one this case is about, all but certain.
			for range 32 {
				loop.run(cm, ctx, newEndedWatcher())
			}

			require.Zero(t, logs.count())
			require.True(t, cm.Health().Healthy, "%+v", cm.Health())
		})
		t.Run(loop.name+"/watch stopped by Stop", func(t *testing.T) {
			t.Parallel()
			cm, _, logs := newWatchLostManager(t)
			// Stop stops each watch before it ends the run context, so a loop
			// sees its updates end while that context is still live. Stop has
			// already begun by then, as it has here.
			require.NoError(t, cm.Stop(t.Context()))

			loop.run(cm, t.Context(), newEndedWatcher())

			require.Zero(t, logs.count())
			require.True(t, cm.Health().Healthy, "%+v", cm.Health())
		})
	}
}

// After Stop the Manager watches nothing, so it has no lost watch to report.
func TestHealthIsHealthyAfterStop(t *testing.T) {
	t.Parallel()
	cm, _, _ := newWatchLostManager(t)
	cm.processWatcher(t.Context(), newEndedWatcher())
	require.False(t, cm.Health().Healthy)

	require.NoError(t, cm.Stop(t.Context()))

	require.True(t, cm.Health().Healthy, "%+v", cm.Health())
}
