//go:build integration

package natsclient

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UpdateWithRetryRev must return the EXACT revision its own commit produced.
//
// The whole reason it exists is that a post-hoc `Get` is not equivalent:
// another writer can commit between the CAS and the re-read, and the re-read
// then reports that writer's revision. A caller attributing the reported
// revision to its own write — the rule engine's per-rule feedback-loop tracker
// does exactly that — would then record someone else's revision and, because
// the skip consumes it once and returns true, silently DROP that writer's
// genuine change.
//
// This test makes the external writer win that window deterministically: it
// commits between the CAS returning and the live read being taken.
func TestUpdateWithRetryRev_ReturnsOwnCommitNotALaterWriters(t *testing.T) {
	ctx := context.Background()
	testClient := newFixtureClient(t)
	bucket, err := testClient.Client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket: "test-update-rev", History: 5,
	})
	require.NoError(t, err)
	kv := testClient.Client.NewKVStore(bucket)

	const key = "subject.one"
	_, err = kv.Put(ctx, key, []byte(`{"v":0}`))
	require.NoError(t, err)

	ownRevision, err := kv.UpdateWithRetryRev(ctx, key, func([]byte) ([]byte, error) {
		return []byte(`{"v":1}`), nil
	})
	require.NoError(t, err)
	require.NotZero(t, ownRevision, "a committed write must yield a revision")

	// Another writer commits AFTER our CAS. This is the window.
	externalRevision, err := kv.Put(ctx, key, []byte(`{"v":2}`))
	require.NoError(t, err)
	require.Greater(t, externalRevision, ownRevision, "fixture sanity: the external write moved the revision")

	entry, err := kv.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, externalRevision, entry.Revision,
		"fixture sanity: a post-hoc re-read now returns the EXTERNAL writer's revision")
	assert.NotEqual(t, entry.Revision, ownRevision,
		"which is precisely why the committed revision must come from the CAS, not a re-read")
}

// The create path (key absent) must report its revision too — UpdateWithRetry
// creates when the key does not exist, and that commit is just as attributable.
func TestUpdateWithRetryRev_ReportsRevisionOnCreatePath(t *testing.T) {
	ctx := context.Background()
	testClient := newFixtureClient(t)
	bucket, err := testClient.Client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket: "test-update-rev-create", History: 5,
	})
	require.NoError(t, err)
	kv := testClient.Client.NewKVStore(bucket)

	revision, err := kv.UpdateWithRetryRev(ctx, "fresh.key", func(current []byte) ([]byte, error) {
		require.Empty(t, current, "fixture sanity: the key must be absent")
		return []byte(`{"born":true}`), nil
	})
	require.NoError(t, err)
	assert.NotZero(t, revision, "a create is a commit and must report its revision")

	entry, err := kv.Get(ctx, "fresh.key")
	require.NoError(t, err)
	assert.Equal(t, entry.Revision, revision)
}

// A closure that fails commits nothing, so there is no revision to report.
func TestUpdateWithRetryRev_ReportsNoRevisionWhenNothingCommits(t *testing.T) {
	ctx := context.Background()
	testClient := newFixtureClient(t)
	bucket, err := testClient.Client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket: "test-update-rev-fail", History: 5,
	})
	require.NoError(t, err)
	kv := testClient.Client.NewKVStore(bucket)

	_, err = kv.Put(ctx, "subject.one", []byte(`{"v":0}`))
	require.NoError(t, err)

	revision, err := kv.UpdateWithRetryRev(ctx, "subject.one", func([]byte) ([]byte, error) {
		return nil, assert.AnError
	})
	require.Error(t, err)
	assert.Zero(t, revision, "nothing committed, so nothing may be attributed")
}

// newUpdateReadStore returns a KVStore over a fresh bucket on its own fixture, and the bucket, whose
// last sequence shows whether a call wrote anything.
func newUpdateReadStore(t *testing.T, name string) (*KVStore, jetstream.KeyValue) {
	t.Helper()
	testClient := newFixtureClient(t)
	bucket, err := testClient.Client.CreateKeyValueBucket(t.Context(), jetstream.KeyValueConfig{
		Bucket: name, History: 5,
	})
	require.NoError(t, err)
	return testClient.Client.NewKVStore(bucket), bucket
}

func requireLastSeq(t *testing.T, bucket jetstream.KeyValue) uint64 {
	t.Helper()
	seq, err := BucketLastSeq(t.Context(), bucket)
	require.NoError(t, err)
	return seq
}

// readArgs is what one run of an UpdateWithRetryRead callback was given.
type readArgs struct {
	current  []byte
	revision uint64
}

// UpdateWithRetryRead passes its callback the revision of the bytes it got, and 0 when the key is
// absent, whether never written, deleted or purged (design D23).
func TestUpdateWithRetryRead_PassesTheRevisionItRead(t *testing.T) {
	ctx := t.Context()
	kv, bucket := newUpdateReadStore(t, "test-update-read-revision")

	requireCommitted := func(t *testing.T, key string, committed uint64, want []byte) {
		t.Helper()
		entry, err := kv.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, want, entry.Value)
		assert.Equal(t, entry.Revision, committed, "the result is the revision of the call's own commit")
	}

	for _, tc := range []struct {
		name, key string
		value     []byte
	}{
		{"present", "present", []byte(`{"v":0}`)},
		{"empty value", "empty", []byte{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			readAt, err := kv.Put(ctx, tc.key, tc.value)
			require.NoError(t, err)

			var got []readArgs
			committed, err := kv.UpdateWithRetryRead(ctx, tc.key, func(current []byte, revision uint64) ([]byte, error) {
				got = append(got, readArgs{current, revision})
				return []byte(`{"v":1}`), nil
			})
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, readAt, got[0].revision, "the callback gets the revision of the bytes it read")
			assert.Equal(t, string(tc.value), string(got[0].current))
			requireCommitted(t, tc.key, committed, []byte(`{"v":1}`))
		})
	}

	for _, tc := range []struct {
		name, key string
		remove    func(key string) error
	}{
		{"never written", "unwritten", nil},
		{"deleted", "deleted", func(key string) error { return kv.Delete(ctx, key) }},
		{"purged", "purged", func(key string) error { return bucket.Purge(ctx, key) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.remove != nil {
				_, err := kv.Put(ctx, tc.key, []byte(`{"v":0}`))
				require.NoError(t, err)
				require.NoError(t, tc.remove(tc.key))
			}

			var got []readArgs
			committed, err := kv.UpdateWithRetryRead(ctx, tc.key, func(current []byte, revision uint64) ([]byte, error) {
				got = append(got, readArgs{current, revision})
				return []byte(`{"born":true}`), nil
			})
			require.NoError(t, err)
			assert.Equal(t, []readArgs{{nil, 0}}, got, "an absent key is no bytes at revision 0")
			requireCommitted(t, tc.key, committed, []byte(`{"born":true}`))
		})
	}
}

// A callback error matching ErrKVSkipWrite ends the call after that run: nothing is written, the
// value beside the error is ignored, and the call returns (0, nil); the same through the three
// wrappers (design D23).
func TestUpdateWithRetryRead_SkipWritesNothing(t *testing.T) {
	ctx := t.Context()
	kv, bucket := newUpdateReadStore(t, "test-update-read-skip")
	skip := fmt.Errorf("x: %w", ErrKVSkipWrite)

	stored := []byte(`{"v":0}`)
	storedAt, err := kv.Put(ctx, "present", stored)
	require.NoError(t, err)

	requireNothingWritten := func(t *testing.T, lastSeq uint64) {
		t.Helper()
		assert.Equal(t, lastSeq, requireLastSeq(t, bucket), "a skip writes nothing to the bucket")
		entry, err := kv.Get(ctx, "present")
		require.NoError(t, err)
		assert.Equal(t, stored, entry.Value)
		assert.Equal(t, storedAt, entry.Revision)
	}

	t.Run("present", func(t *testing.T) {
		lastSeq := requireLastSeq(t, bucket)
		runs := 0
		committed, err := kv.UpdateWithRetryRead(ctx, "present", func([]byte, uint64) ([]byte, error) {
			runs++
			return []byte(`{"v":1}`), skip
		})
		require.NoError(t, err, "a skip is not an error")
		assert.Zero(t, committed, "a skip commits nothing, so nothing is attributed")
		assert.Equal(t, 1, runs)
		requireNothingWritten(t, lastSeq)
	})

	t.Run("absent stays absent", func(t *testing.T) {
		lastSeq := requireLastSeq(t, bucket)
		runs := 0
		committed, err := kv.UpdateWithRetryRead(ctx, "absent", func([]byte, uint64) ([]byte, error) {
			runs++
			return []byte(`{"born":true}`), skip
		})
		require.NoError(t, err)
		assert.Zero(t, committed)
		assert.Equal(t, 1, runs)
		assert.Equal(t, lastSeq, requireLastSeq(t, bucket))
		_, err = kv.Get(ctx, "absent")
		assert.True(t, IsKVNotFoundError(err), "the key is still absent, got %v", err)
	})

	for _, tc := range []struct {
		name string
		call func(runs *int) (uint64, error)
	}{
		{"UpdateWithRetryRev", func(runs *int) (uint64, error) {
			return kv.UpdateWithRetryRev(ctx, "present", func([]byte) ([]byte, error) {
				*runs++
				return []byte(`{"v":2}`), skip
			})
		}},
		{"UpdateWithRetry", func(runs *int) (uint64, error) {
			return 0, kv.UpdateWithRetry(ctx, "present", func([]byte) ([]byte, error) {
				*runs++
				return []byte(`{"v":3}`), skip
			})
		}},
		{"UpdateJSON", func(runs *int) (uint64, error) {
			return 0, kv.UpdateJSON(ctx, "present", func(current map[string]any) error {
				*runs++
				current["v"] = 4
				return skip
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lastSeq := requireLastSeq(t, bucket)
			runs := 0
			committed, err := tc.call(&runs)
			require.NoError(t, err, "a skip is not an error through %s", tc.name)
			assert.Zero(t, committed)
			assert.Equal(t, 1, runs)
			requireNothingWritten(t, lastSeq)
		})
	}
}

// A run whose write conflicts is rerun with the bytes and revision that beat it (design D23). The
// other writer is a put made inside the first run, between its read and its write.
func TestUpdateWithRetryRead_ConflictPassesTheNewerRevision(t *testing.T) {
	ctx := t.Context()
	kv, _ := newUpdateReadStore(t, "test-update-read-conflict")
	stored := []byte(`{"v":0}`)
	foreign := []byte(`{"v":"foreign"}`)

	for _, tc := range []struct {
		name string
		skip bool
	}{
		{"writing", false},
		{"skipping", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			readAt, err := kv.Put(ctx, tc.name, stored)
			require.NoError(t, err)

			var foreignAt uint64
			var got []readArgs
			committed, err := kv.UpdateWithRetryRead(ctx, tc.name, func(current []byte, revision uint64) ([]byte, error) {
				got = append(got, readArgs{current, revision})
				if len(got) == 1 {
					var putErr error
					foreignAt, putErr = kv.Put(ctx, tc.name, foreign)
					require.NoError(t, putErr)
					return []byte(`{"v":1}`), nil
				}
				if tc.skip {
					return nil, ErrKVSkipWrite
				}
				return []byte(`{"v":2}`), nil
			})
			require.NoError(t, err)
			require.Len(t, got, 2, "the conflict reruns the callback once")
			assert.Equal(t, readArgs{stored, readAt}, got[0])
			assert.Equal(t, readArgs{foreign, foreignAt}, got[1],
				"the rerun gets the bytes the other writer committed and their revision")

			entry, err := kv.Get(ctx, tc.name)
			require.NoError(t, err)
			if tc.skip {
				assert.Zero(t, committed, "the skip attributes no commit to the call")
				assert.Equal(t, foreignAt, entry.Revision)
				assert.Equal(t, foreign, entry.Value)
				return
			}
			assert.Greater(t, committed, foreignAt)
			assert.Equal(t, entry.Revision, committed)
			assert.Equal(t, []byte(`{"v":2}`), entry.Value)
		})
	}
}

// Any other callback error, and a context that ended before the call, attribute no commit to the
// call (design D23).
func TestUpdateWithRetryRead_ErrorsAttributeNoCommit(t *testing.T) {
	ctx := t.Context()
	kv, bucket := newUpdateReadStore(t, "test-update-read-errors")
	stored := []byte(`{"v":0}`)
	storedAt, err := kv.Put(ctx, "present", stored)
	require.NoError(t, err)

	t.Run("callback error", func(t *testing.T) {
		lastSeq := requireLastSeq(t, bucket)
		refused := errors.New("callback refused")
		runs := 0
		committed, err := kv.UpdateWithRetryRead(ctx, "present", func([]byte, uint64) ([]byte, error) {
			runs++
			return []byte(`{"v":1}`), refused
		})
		require.ErrorIs(t, err, refused)
		assert.Zero(t, committed)
		assert.Equal(t, 1, runs, "a callback error ends the call after one run")
		assert.Equal(t, lastSeq, requireLastSeq(t, bucket))
		entry, err := kv.Get(ctx, "present")
		require.NoError(t, err)
		assert.Equal(t, storedAt, entry.Revision)
	})

	t.Run("context cancelled first", func(t *testing.T) {
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		runs := 0
		committed, err := kv.UpdateWithRetryRead(cancelled, "present", func([]byte, uint64) ([]byte, error) {
			runs++
			return []byte(`{"v":1}`), nil
		})
		require.ErrorIs(t, err, context.Canceled)
		assert.Zero(t, committed)
		assert.Zero(t, runs, "an ended context starts no run")
	})
}
