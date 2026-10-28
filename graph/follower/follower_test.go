package follower

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"

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
// v1.54.0 leaves a watch in when its context ends between Subscribe and the install of the
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
