package storagereport

import (
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReportConsumer_ReplacementWatchKeepsThePreviousViewUntilItsSyncMarker states
// what a Snapshot caller sees while a replacement watch delivers its initial values:
// the previous view, whole and unchanged, never a mix of the old view and part of the
// new one. At the new watch's sync marker the view becomes exactly the new watch's
// initial values in one step, so a row or account the bucket no longer holds is gone.
func TestReportConsumer_ReplacementWatchKeepsThePreviousViewUntilItsSyncMarker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &fakeWatchStore{}
		observer := &recordingObserver{}
		consumer := runConsumer(t, store, observer)
		synctest.Wait()
		require.Equal(t, 1, store.watchCount())

		first := store.watchers[0]
		first.put("KEPT", reportRow("KEPT", TierFile, PressureNormal))
		first.put("GONE", reportRow("GONE", TierFile, PressureNormal))
		first.put(StorageAccountReportKey, AccountReport{ProducedBy: "first"})
		first.synced()
		synctest.Wait()
		before := consumer.Snapshot()
		require.True(t, before.Synced)
		require.True(t, before.AccountKnown)
		require.Len(t, before.Resources, 2)

		close(first.updates)
		eventually(t, func() bool { return store.watchCount() == 2 }, "the dropped watch is replaced")
		store.mu.Lock()
		second := store.watchers[1]
		store.mu.Unlock()

		// The replacement's initial values: KEPT changed, NEW appeared, GONE and the
		// account row are absent. No sync marker yet.
		second.put("KEPT", reportRow("KEPT", TierFile, PressureCritical))
		second.put("NEW", reportRow("NEW", TierFile, PressureNormal))
		synctest.Wait()
		assert.Equal(t, before, consumer.Snapshot(),
			"before the replacement's sync marker a caller sees the previous view, not part of the new one")

		second.synced()
		synctest.Wait()
		after := consumer.Snapshot()
		assert.True(t, after.Synced)
		assert.False(t, after.AccountKnown, "an account row absent from the new initial values is retracted")
		assert.Equal(t, AccountReport{}, after.Account)
		names := make([]string, 0, len(after.Resources))
		for _, row := range after.Resources {
			names = append(names, row.Resource.Name)
		}
		assert.Equal(t, []string{"KEPT", "NEW"}, names)
		assert.Equal(t, PressureCritical, after.Resources[0].Pressure.State, "KEPT carries its new value")

		_, forgot, _ := observer.snapshot()
		assert.Equal(t, []string{"GONE"}, forgot, "the absent row is forgotten with its identity, once")
	})
}

// TestReportConsumer_ReplayedTombstoneForgetsTheRowOnce keeps the ordinary reconnect
// case working: a replacement watch replays a retained delete marker for a row this
// process held, and that row is forgotten exactly once, not once for the marker and
// again for its absence.
func TestReportConsumer_ReplayedTombstoneForgetsTheRowOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &fakeWatchStore{}
		observer := &recordingObserver{}
		consumer := runConsumer(t, store, observer)
		synctest.Wait()

		first := store.watchers[0]
		first.put("KEPT", reportRow("KEPT", TierFile, PressureNormal))
		first.put("DELETED", reportRow("DELETED", TierFile, PressureNormal))
		first.synced()
		synctest.Wait()
		close(first.updates)
		eventually(t, func() bool { return store.watchCount() == 2 }, "the dropped watch is replaced")

		store.mu.Lock()
		second := store.watchers[1]
		store.mu.Unlock()
		second.put("KEPT", reportRow("KEPT", TierFile, PressureNormal))
		second.del("DELETED")
		second.synced()
		synctest.Wait()

		snapshot := consumer.Snapshot()
		require.Len(t, snapshot.Resources, 1)
		assert.Equal(t, "KEPT", snapshot.Resources[0].Resource.Name)
		_, forgot, _ := observer.snapshot()
		assert.Equal(t, []string{"DELETED"}, forgot)
	})
}

// TestReportConsumer_UndecodableValueInAReplacementKeepsTheHeldRow applies the live
// rule (one undecodable value does not blank a row; the next publication repairs it)
// to a replacement watch's initial values: the key is still in the bucket, so the
// row this process held stays rather than being retracted as absent.
func TestReportConsumer_UndecodableValueInAReplacementKeepsTheHeldRow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &fakeWatchStore{}
		observer := &recordingObserver{}
		consumer := runConsumer(t, store, observer)
		synctest.Wait()

		first := store.watchers[0]
		first.put("ROW", reportRow("ROW", TierFile, PressureNormal))
		first.put(StorageAccountReportKey, AccountReport{ProducedBy: "first"})
		first.synced()
		synctest.Wait()
		close(first.updates)
		eventually(t, func() bool { return store.watchCount() == 2 }, "the dropped watch is replaced")

		store.mu.Lock()
		second := store.watchers[1]
		store.mu.Unlock()
		second.raw("ROW", []byte("{not json"))
		second.raw(StorageAccountReportKey, []byte("{not json"))
		second.synced()
		synctest.Wait()

		snapshot := consumer.Snapshot()
		require.Len(t, snapshot.Resources, 1)
		assert.Equal(t, "ROW", snapshot.Resources[0].Resource.Name)
		assert.True(t, snapshot.AccountKnown)
		assert.Equal(t, "first", snapshot.Account.ProducedBy)
		_, forgot, _ := observer.snapshot()
		assert.Empty(t, forgot)
	})
}

// TestReportConsumer_SnapshotSharesNoMutableMemory walks two snapshots of a view in
// which every pointer field is set and every slice is non-empty, and fails if any
// pointer target, slice backing array or map is reachable from both. It walks the
// types by reflection rather than naming fields, so a pointer, slice or map field
// added to a row later is covered without editing this test.
func TestReportConsumer_SnapshotSharesNoMutableMemory(t *testing.T) {
	store := &fakeWatchStore{}
	consumer, err := NewConsumer(store, ConsumerConfig{})
	require.NoError(t, err)

	var row ResourceReport
	fillPointers(reflect.ValueOf(&row).Elem())
	row.Resource.Name = "FULL"
	row.Pressure = Pressure{Evaluated: true, State: PressureNormal}
	consumer.putResource("FULL", row)
	account := AccountReport{Tiers: make([]TierComparison, 2)}
	fillPointers(reflect.ValueOf(&account).Elem())
	consumer.putAccount(account)

	first, second := consumer.Snapshot(), consumer.Snapshot()
	seen := map[uintptr]string{}
	firstCount := mutableAddresses(reflect.ValueOf(first), "snapshot", seen)
	// Every Capacity, Growth and Projection pointer, the row and tier slices and the
	// count map: well over ten. A walk that found none would assert nothing.
	require.Greater(t, firstCount, 10, "the walk must reach the snapshot's mutable memory")

	shared := map[uintptr]string{}
	mutableAddresses(reflect.ValueOf(second), "snapshot", shared)
	for addr, path := range shared {
		if firstPath, ok := seen[addr]; ok {
			t.Errorf("%s and %s share memory: a caller editing one snapshot edits the consumer's view", firstPath, path)
		}
	}
}

// fillPointers sets every nil exported pointer reachable from v to a new zero value.
func fillPointers(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		fillPointers(v.Elem())
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				fillPointers(v.Field(i))
			}
		}
	case reflect.Slice:
		for i := range v.Len() {
			fillPointers(v.Index(i))
		}
	}
}

// mutableAddresses records the address of every pointer target, slice backing array
// and map reachable through exported fields of v, and returns how many it found.
// Unexported fields are skipped: the only ones reachable are time.Time's location,
// which is never written.
func mutableAddresses(v reflect.Value, path string, into map[uintptr]string) int {
	count := 0
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return 0
		}
		into[v.Pointer()] = path
		return 1 + mutableAddresses(v.Elem(), path, into)
	case reflect.Slice:
		if v.Len() == 0 {
			return 0
		}
		into[v.Pointer()] = path
		count++
		for i := range v.Len() {
			count += mutableAddresses(v.Index(i), path+"[i]", into)
		}
	case reflect.Map:
		if v.Len() == 0 {
			return 0
		}
		into[v.Pointer()] = path
		count++
	case reflect.Struct:
		for i := range v.NumField() {
			field := v.Type().Field(i)
			if field.IsExported() {
				count += mutableAddresses(v.Field(i), path+"."+field.Name, into)
			}
		}
	}
	return count
}

// TestReportConsumer_AccountDeleteReachesObserver: a delete or purge of the account key,
// when this process held an account row, tells the observer once, with the row it last
// observed, and the snapshot stops reporting an account. A tombstone of an account the
// process does not hold tells the observer nothing (#85).
func TestReportConsumer_AccountDeleteReachesObserver(t *testing.T) {
	t.Run("a held account", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := &fakeWatchStore{}
			observer := &recordingObserver{}
			consumer := runConsumer(t, store, observer)
			synctest.Wait()

			watcher := store.watchers[0]
			watcher.put(StorageAccountReportKey, AccountReport{ProducedBy: "held"})
			watcher.synced()
			synctest.Wait()
			_, _, observed := observer.snapshot()
			require.Len(t, observed, 1)
			require.Empty(t, observer.forgottenAccounts(), "nothing is forgotten before the delete")

			watcher.del(StorageAccountReportKey)
			synctest.Wait()
			assert.False(t, consumer.Snapshot().AccountKnown)
			assert.Equal(t, observed, observer.forgottenAccounts(),
				"the observer forgets the account row it was given, once")

			// A second tombstone finds no held account.
			watcher.del(StorageAccountReportKey)
			synctest.Wait()
			assert.Len(t, observer.forgottenAccounts(), 1)
		})
	})
	t.Run("no held account", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := &fakeWatchStore{}
			observer := &recordingObserver{}
			consumer := runConsumer(t, store, observer)
			synctest.Wait()

			watcher := store.watchers[0]
			watcher.synced()
			watcher.del(StorageAccountReportKey)
			synctest.Wait()
			assert.True(t, consumer.Snapshot().Synced)
			assert.Empty(t, observer.forgottenAccounts())
		})
	})
}

// TestReportConsumer_ResyncRetractionReachesObserver: a replacement watch whose initial
// values lack the account key, when this process held an account row, tells the
// observer once, at the sync marker and not before it. An undecodable account value in
// the initial values keeps the held row and tells the observer nothing (#85).
func TestReportConsumer_ResyncRetractionReachesObserver(t *testing.T) {
	replace := func(t *testing.T, store *fakeWatchStore, observer *recordingObserver) (*fakeKeyWatcher, []AccountReport) {
		t.Helper()
		first := store.watchers[0]
		first.put("ROW", reportRow("ROW", TierFile, PressureNormal))
		first.put(StorageAccountReportKey, AccountReport{ProducedBy: "held"})
		first.synced()
		synctest.Wait()
		_, _, observed := observer.snapshot()
		require.Len(t, observed, 1)
		close(first.updates)
		eventually(t, func() bool { return store.watchCount() == 2 }, "the dropped watch is replaced")
		store.mu.Lock()
		defer store.mu.Unlock()
		return store.watchers[1], observed
	}

	t.Run("account key absent", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := &fakeWatchStore{}
			observer := &recordingObserver{}
			consumer := runConsumer(t, store, observer)
			synctest.Wait()
			second, observed := replace(t, store, observer)

			second.put("ROW", reportRow("ROW", TierFile, PressureNormal))
			synctest.Wait()
			assert.Empty(t, observer.forgottenAccounts(), "nothing is forgotten before the sync marker")

			second.synced()
			synctest.Wait()
			assert.False(t, consumer.Snapshot().AccountKnown)
			assert.Equal(t, observed, observer.forgottenAccounts(),
				"the observer forgets the account row it was given, once, at the marker")
		})
	})
	t.Run("account value undecodable", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			store := &fakeWatchStore{}
			observer := &recordingObserver{}
			consumer := runConsumer(t, store, observer)
			synctest.Wait()
			second, _ := replace(t, store, observer)

			second.put("ROW", reportRow("ROW", TierFile, PressureNormal))
			second.raw(StorageAccountReportKey, []byte("{not json"))
			second.synced()
			synctest.Wait()
			assert.True(t, consumer.Snapshot().AccountKnown, "the held account row is kept")
			assert.Empty(t, observer.forgottenAccounts())
		})
	})
}
