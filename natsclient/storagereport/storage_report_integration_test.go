//go:build integration

package storagereport

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/natsclient"
)

// reclaimBacklogStore is the real report bucket, except that its listing opens the real watch,
// waits until nats.go's delivery is blocked on the full updates buffer, and then cancels
// Publish's context before handing the listing over. reclaim therefore reads the listing only
// after the cancel, as a Publish whose context ends mid-listing does. blockedErr records a
// backlog that never formed, so the test fails rather than reading the listing error as a pass.
type reclaimBacklogStore struct {
	jetstream.KeyValue
	cancel     context.CancelFunc
	blockedErr error
}

func (s *reclaimBacklogStore) WatchFiltered(ctx context.Context, keys []string, opts ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	watcher, err := s.KeyValue.WatchFiltered(ctx, keys, opts...)
	if err != nil {
		s.blockedErr = err
		return nil, err
	}
	s.blockedErr = natsfixture.AwaitNativeDeliveryBlocked(ctx, failureBound)
	s.cancel()
	return watcher, nil
}

// reclaimListingStacks returns the stacks of nats.go's key lister goroutine (what reclaim read
// before #496) and of its watch delivery, or "".
func reclaimListingStacks() string {
	buf := make([]byte, 1<<20)
	var found []string
	for _, stack := range strings.Split(string(buf[:runtime.Stack(buf, true)]), "\n\n") {
		if strings.Contains(stack, "jetstream.(*kvs).ListKeys.func1") {
			found = append(found, stack)
		}
	}
	if delivery := natsfixture.NativeDeliveryStacks(); delivery != "" {
		found = append(found, delivery)
	}
	return strings.Join(found, "\n\n")
}

// TestPublishStoppedMidReclaimLeavesNoDelivery: a Publish whose context ends while reclaim's
// listing of a bucket holding more stale rows than nats.go's 256-entry buffers is backlogged
// returns an error matching context.Canceled, deletes no row, and leaves no key lister or watch
// delivery running (design D3 test 3). Ten cycles on one bucket, as graph/follower's
// TestFollowerRunReturnsAndLeavesNothing runs.
func TestPublishStoppedMidReclaimLeavesNoDelivery(t *testing.T) {
	client, err := natsclient.NewClient(startFixture(t).URL(), natsclient.WithMaxReconnects(0))
	require.NoError(t, err)
	require.NoError(t, client.Connect(t.Context()))
	t.Cleanup(func() { closeClient(t, client) })
	js, err := client.JetStream()
	require.NoError(t, err)
	kv, err := js.CreateKeyValue(t.Context(), jetstream.KeyValueConfig{Bucket: "STORAGE_REPORT_RECLAIM"})
	require.NoError(t, err)
	// Stale: no collection below names these rows, so a complete listing would reclaim them all.
	for n := range reportBacklogRows {
		_, err := kv.Put(t.Context(), fmt.Sprintf("STALE_%03d", n), []byte("stale"))
		require.NoError(t, err)
	}

	first := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	for cycle := range 10 {
		store := &reclaimBacklogStore{KeyValue: kv}
		publisher := newTestPublisher(t, store, defaultSource())
		ctx, cancel := context.WithCancel(t.Context())
		store.cancel = cancel

		result, err := publisher.Publish(ctx, inventoryAt(first.Add(time.Duration(cycle)*time.Minute)))
		cancel()
		if store.blockedErr != nil {
			t.Fatalf("cycle %d: %v", cycle, store.blockedErr)
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cycle %d: Publish error = %v, want context.Canceled", cycle, err)
		}
		if result.Deleted != 0 {
			t.Fatalf("cycle %d: Publish deleted %d rows from a listing cut short, want 0", cycle, result.Deleted)
		}
		if err := natsfixture.AwaitNoStacks(t.Context(), failureBound, reclaimListingStacks); err != nil {
			t.Fatalf("cycle %d: nats.go's key lister or watch delivery is still running after Publish returned: %v", cycle, err)
		}
	}

	// The oracle for "deleted no row" is the bucket itself, not Publish's count.
	keys, err := kv.Keys(t.Context())
	require.NoError(t, err)
	stale := 0
	for _, key := range keys {
		if strings.HasPrefix(key, "STALE_") {
			stale++
		}
	}
	require.Equal(t, reportBacklogRows, stale, "every stale row is still in the bucket")
}
