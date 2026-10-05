package natsclient

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
)

func TestReviewerStorageReportAccountDelete(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &fakeWatchStore{}
		consumer := runConsumer(t, store, nil)
		synctest.Wait()
		require.Equal(t, 1, store.watchCount())
		watcher := store.watchers[0]
		watcher.put(StorageAccountReportKey, AccountReport{ProducedBy: "probe", Tiers: []TierComparison{{Tier: TierFile, State: OvercommitmentWithin}}})
		watcher.synced()
		synctest.Wait()
		require.True(t, consumer.Snapshot().AccountKnown)
		watcher.del(StorageAccountReportKey)
		synctest.Wait()
		require.False(t, consumer.Snapshot().AccountKnown, "deleted account row must not remain known")
	})
}

func TestReviewerStorageReportResyncDropsAbsentRows(t *testing.T) {
	store := &fakeWatchStore{}
	first := newFakeKeyWatcher()
	first.put("GONE", reportRow("GONE", TierFile, PressureNormal))
	first.synced()
	close(first.updates)
	store.next = func() *fakeKeyWatcher { return first }
	consumer, err := NewStorageReportConsumer(store, StorageReportConsumerConfig{})
	require.NoError(t, err)
	consumer.watchOnce(context.Background())
	require.Len(t, consumer.Snapshot().Resources, 1)
	// A replacement watch's complete initial snapshot of an empty bucket has
	// only the sync marker (e.g. bucket recreated or delete markers purged).
	second := newFakeKeyWatcher()
	second.synced()
	close(second.updates)
	store.next = func() *fakeKeyWatcher { return second }
	consumer.watchOnce(context.Background())
	require.Empty(t, consumer.Snapshot().Resources, "complete new snapshot must retract rows absent from the bucket")
}

func TestReviewerStorageReportSnapshotAccountIsCopy(t *testing.T) {
	store := &fakeWatchStore{}
	consumer, err := NewStorageReportConsumer(store, StorageReportConsumerConfig{})
	require.NoError(t, err)
	consumer.putAccount(AccountReport{Tiers: []TierComparison{{Tier: TierFile, State: OvercommitmentOver}}})
	snapshot := consumer.Snapshot()
	snapshot.Account.Tiers[0].State = OvercommitmentWithin
	require.Equal(t, OvercommitmentOver, consumer.Snapshot().Account.Tiers[0].State, "snapshot edits must not alter consumer state")
}
