package cache

import (
	"context"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordHandler sends every record it handles to a channel the test reads.
type recordHandler struct{ records chan<- slog.Record }

func (h recordHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h recordHandler) Handle(_ context.Context, r slog.Record) error {
	h.records <- r
	return nil
}
func (h recordHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h recordHandler) WithGroup(string) slog.Handler      { return h }

// TestCoalescingSet_CallbackPanicIsRecoveredAndLaterBatchesFire (owner ruling, #9 comment
// 5994720412 item 3, applying 5985697767 item 3): a panic in the callback does not end the
// process. It is logged at error level with the batch size and counted, the batch is dropped, and
// the next batch still reaches the callback. Oracles: the log record, the counter's value and the
// later batch, each received under a bound.
func TestCoalescingSet_CallbackPanicIsRecoveredAndLaterBatchesFire(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		records := make(chan slog.Record, 4)
		counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_callback_panics_total", Help: "test"})
		batches := make(chan []string, 4)
		set := NewCoalescingSet(ctx, 50*time.Millisecond, func(keys []string) {
			if len(keys) == 1 && keys[0] == "poison" {
				panic("coalescing callback panic sentinel")
			}
			batches <- keys
		}, WithPanicLogger(slog.New(recordHandler{records: records})), WithPanicCounter(counter))

		set.Add("poison")
		select {
		case r := <-records:
			assert.Equal(t, slog.LevelError, r.Level)
			attrs := map[string]slog.Value{}
			r.Attrs(func(a slog.Attr) bool { attrs[a.Key] = a.Value; return true })
			assert.Equal(t, "coalescing callback panic sentinel", attrs["panic"].Any())
			assert.Equal(t, int64(1), attrs["batch_size"].Int64())
		case <-time.After(time.Second):
			t.Fatal("the recovered panic was not logged")
		}
		assert.Equal(t, 1.0, testutil.ToFloat64(counter), "the recovered panic was not counted")

		set.Add("after")
		select {
		case keys := <-batches:
			assert.Equal(t, []string{"after"}, keys)
		case <-time.After(time.Second):
			t.Fatal("the batch after the panic did not reach the callback")
		}
		require.NoError(t, set.Shutdown(t.Context()))
	})
}
