//go:build integration

package storagereport

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/natsclient"
)

// reportBacklogRows is above nats.go's 256-entry watch updates buffer, so a watch nobody reads
// leaves its delivery blocked sending into the full buffer.
const reportBacklogRows = 600

// backloggedWatchStore opens the real watch, waits until nats.go's delivery is blocked on the
// full updates buffer, tells the test so on blocked, and hands the watcher over only once ctx has
// ended. The consumer therefore reads the watch only after the test's cancel, as a consumer whose
// context ends while the watch is backlogged does.
type backloggedWatchStore struct {
	kv      jetstream.KeyValue
	blocked chan error
}

func (s *backloggedWatchStore) WatchAll(ctx context.Context, opts ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	watcher, err := s.kv.WatchAll(ctx, opts...)
	if err != nil {
		s.blocked <- err
		return nil, err
	}
	s.blocked <- natsfixture.AwaitNativeDeliveryBlocked(ctx, failureBound)
	<-ctx.Done()
	return watcher, nil
}

// TestIntegration_ReportConsumer_RunLeavesNoWatchDelivery: a consumer whose context ends while
// its watch of a backlogged report bucket is blocked on nats.go's full updates buffer returns from
// Run, and no nats.go watch delivery is left running behind it. Ten cycles on one bucket, as
// pkg/graphview's TestRunLeavesNoWatchDelivery runs.
func TestIntegration_ReportConsumer_RunLeavesNoWatchDelivery(t *testing.T) {
	client, err := natsclient.NewClient(startFixture(t).URL(), natsclient.WithMaxReconnects(0))
	require.NoError(t, err)
	require.NoError(t, client.Connect(t.Context()))
	t.Cleanup(func() { closeClient(t, client) })
	js, err := client.JetStream()
	require.NoError(t, err)
	kv, err := js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{Bucket: "STORAGE_REPORT_BACKLOG"})
	require.NoError(t, err)
	for n := range reportBacklogRows {
		name := fmt.Sprintf("BACKLOG_%03d", n)
		body, err := json.Marshal(reportRow(name, TierFile, PressureNormal))
		require.NoError(t, err)
		_, err = kv.Put(t.Context(), name, body)
		require.NoError(t, err)
	}

	for cycle := range 10 {
		store := &backloggedWatchStore{kv: kv, blocked: make(chan error, 1)}
		consumer, err := NewConsumer(store, ConsumerConfig{RetryBackoff: time.Millisecond})
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			defer close(done)
			consumer.Run(ctx)
		}()
		if err := <-store.blocked; err != nil {
			cancel()
			t.Fatalf("cycle %d: %v", cycle, err)
		}
		cancel()
		<-done
		if err := natsfixture.AwaitNoStacks(t.Context(), failureBound, natsfixture.NativeDeliveryStacks); err != nil {
			t.Fatalf("cycle %d: nats.go's watch delivery is still running after Run returned: %v", cycle, err)
		}
	}
}
