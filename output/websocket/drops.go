package websocket

import (
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/c360studio/semengine/metric"
)

// The reasons a frame is not sent (design D2, "Declared drops").
const (
	reasonNotJSON         = "not_json"
	reasonClientQueueFull = "client_queue_full"
)

// dropWarnEvery is how often a reason is logged after its first warning, which logs at once.
const dropWarnEvery = time.Minute

// newDroppedCounter builds messages_dropped_total and registers it on registry, the injected
// component.Dependencies.MetricsRegistry, through metric.RegisterOrGet, keeping the collector it
// returns, so instances on one registry share it and stay apart by listener. With a nil registry it
// is registered nowhere. The output's other metrics come with task 3.2.
func newDroppedCounter(registry *metric.MetricsRegistry) (*prometheus.CounterVec, error) {
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "semengine", Subsystem: key, Name: "messages_dropped_total",
		Help: "Frames not sent, by reason: not_json (a subject message that is not JSON), client_queue_full (a client's oldest queued frame)",
	}, []string{"listener", "reason"})
	if registry == nil {
		return counter, nil
	}
	return metric.RegisterOrGet(registry, key, "messages_dropped_total", counter)
}

// drops counts and logs each declared drop: every drop is counted at once, and each reason is
// logged the first time at once, then at most once every dropWarnEvery with the count since its
// last line.
type drops struct {
	listener string
	logger   *slog.Logger
	counter  *prometheus.CounterVec

	mu sync.Mutex
	// warned is when each reason was last logged; unlogged counts its drops since then.
	warned   map[string]time.Time
	unlogged map[string]int
}

func newDrops(listener string, logger *slog.Logger, counter *prometheus.CounterVec) *drops {
	return &drops{
		listener: listener, logger: logger, counter: counter,
		warned: map[string]time.Time{}, unlogged: map[string]int{},
	}
}

// record counts n drops for reason and logs message with attrs when the reason is due a line.
func (d *drops) record(reason string, n int, message string, attrs ...slog.Attr) {
	d.counter.WithLabelValues(d.listener, reason).Add(float64(n))
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
