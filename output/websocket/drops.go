package websocket

import (
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// The reasons a frame is not sent, or a change is merged into a later one (design D2, "Declared
// drops").
const (
	reasonNotJSON         = "not_json"
	reasonClientQueueFull = "client_queue_full"
	reasonClientBehind    = "client_behind"
)

// dropWarnEvery is how often a reason is logged after its first warning, which logs at once.
const dropWarnEvery = time.Minute

// drops counts and logs each declared drop: every drop is counted at once, and each reason is
// logged the first time at once, then at most once every dropWarnEvery with the count since its
// last line.
type drops struct {
	listener string
	logger   *slog.Logger
	// counters is each reason's counter: messages_dropped_total for not_json and
	// client_queue_full, changes_coalesced_total for client_behind.
	counters map[string]prometheus.Counter

	mu sync.Mutex
	// warned is when each reason was last logged; unlogged counts its drops since then.
	warned   map[string]time.Time
	unlogged map[string]int
}

func newDrops(listener string, logger *slog.Logger, m *outputMetrics) *drops {
	return &drops{
		listener: listener, logger: logger,
		counters: map[string]prometheus.Counter{
			reasonNotJSON:         m.dropped.WithLabelValues(listener, reasonNotJSON),
			reasonClientQueueFull: m.dropped.WithLabelValues(listener, reasonClientQueueFull),
			reasonClientBehind:    m.coalesced,
		},
		warned: map[string]time.Time{}, unlogged: map[string]int{},
	}
}

// record counts n drops for reason and logs message with attrs when the reason is due a line.
func (d *drops) record(reason string, n int, message string, attrs ...slog.Attr) {
	d.counters[reason].Add(float64(n))
	d.mu.Lock()
	d.unlogged[reason] += n
	now := time.Now()
	if last, ok := d.warned[reason]; ok && now.Sub(last) < dropWarnEvery {
		d.mu.Unlock()
		return
	}
	count := d.unlogged[reason]
	d.warned[reason], d.unlogged[reason] = now, 0
	d.mu.Unlock()
	args := []any{slog.String("reason", reason), slog.String("listener", d.listener), slog.Int("dropped", count)}
	for _, attr := range attrs {
		args = append(args, attr)
	}
	d.logger.Warn(message, args...)
}
