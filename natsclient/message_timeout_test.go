package natsclient

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semengine/metric"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMessageHandlerContext_DisabledUsesLifecycleContext(t *testing.T) {
	parent, parentCancel := context.WithCancel(context.Background())
	ctx, cancel := messageHandlerContext(parent, time.Second, true)
	defer cancel()

	if _, ok := ctx.Deadline(); ok {
		t.Fatal("disabled message timeout unexpectedly installed a deadline")
	}
	parentCancel()
	select {
	case <-ctx.Done():
		if ctx.Err() != context.Canceled {
			t.Fatalf("context error = %v, want context.Canceled", ctx.Err())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("lifecycle cancellation did not reach no-timeout message context")
	}
}

func TestMessageHandlerContext_PositiveRetainsWorkDeadline(t *testing.T) {
	ctx, cancel := messageHandlerContext(context.Background(), time.Second, false)
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("positive message timeout did not install a work deadline")
	}
}

// TestSafeHandleMessageRecoversPanicNaksLogsAndCounts is owner ruling 3 (#9 comment 5985697767): a
// panic in a consumer's message handler is recovered, the message is Nak'd, and the panic is
// logged at error level and counted on the JetStream error metric as operation handler_panic. A
// Nak that fails is logged too, never discarded.
func TestSafeHandleMessageRecoversPanicNaksLogsAndCounts(t *testing.T) {
	for _, nakErr := range []error{nil, errors.New("nak refused")} {
		name := "nak-ok"
		if nakErr != nil {
			name = "nak-fails"
		}
		t.Run(name, func(t *testing.T) {
			logs := newRecordingHandler("")
			c, err := NewClient("nats://unused", WithLogger(slog.New(logs)), WithMetrics(metric.NewMetricsRegistry()))
			if err != nil {
				t.Fatal(err)
			}
			msg := &mockMsg{subject: "orders.created"}
			msg.nakErr = nakErr

			panicked, _ := recoverCall(func() error {
				c.safeHandleMessage(t.Context(), msg, func(context.Context, jetstream.Msg) { panic("handler bug") })
				return nil
			})
			if panicked != nil {
				t.Fatalf("the handler's panic escaped: %v", panicked)
			}
			if got := msg.nakCount.Load(); got != 1 {
				t.Errorf("Nak calls = %d, want 1", got)
			}
			if got := testutil.ToFloat64(c.jsMetrics.errors.WithLabelValues("handler_panic")); got != 1 {
				t.Errorf("handler_panic count = %v, want 1", got)
			}
			logged := logs.find(func(r loggedRecord) bool {
				return r.level == slog.LevelError && r.attrs["panic"] == "handler bug" && r.attrs["subject"] == "orders.created"
			})
			if len(logged) != 1 {
				t.Errorf("error records naming the panic and subject = %d, want 1", len(logged))
			}
			nakLogged := logs.find(func(r loggedRecord) bool { return r.attrs["nak_error"] == "nak refused" })
			if want := map[bool]int{false: 0, true: 1}[nakErr != nil]; len(nakLogged) != want {
				t.Errorf("records naming the Nak error = %d, want %d", len(nakLogged), want)
			}
		})
	}
}
