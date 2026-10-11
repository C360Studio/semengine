package natsclient

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// T6, design "Tests that replace the probes" and D6. CreateKeyValueBucket
// takes a create that finds the bucket already there for a race with another
// creator by matching jetstream.ErrBucketExists. This pins the nats.go fact it
// relies on: a second create of a bucket, with a different config, matches
// ErrBucketExists (jetstream/kv.go:540-545 at v1.54.0). It guards #118's
// upgrade.
func TestCreateKeyValueOfAnExistingBucketIsErrBucketExists(t *testing.T) {
	nc, err := nats.Connect(embeddedJetStreamURL(t))
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), lifecycleBound)
	defer cancel()

	_, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "t6", History: 1, Storage: jetstream.MemoryStorage})
	require.NoError(t, err)
	_, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: "t6", History: 2, Storage: jetstream.MemoryStorage})
	require.ErrorIs(t, err, jetstream.ErrBucketExists)
}

// CreateKeyValueBucket's ErrBucketExists branch, reached with no race. A plain
// stream named KV_<bucket> is not a KV bucket (MaxMsgsPerSubject < 1), so the
// first get returns ErrBadBucket (jetstream/kv.go:523-524 at v1.54.0), the
// create collides on the stream name and carries ErrBucketExists (:545), and
// the branch's second get returns ErrBadBucket again, wrapped with its action.
// Without the branch the create's error comes back as it is, and it does not
// match ErrBadBucket. The branch's success arm, a second get that finds a
// bucket another creator just made, is not reached here.
func TestCreateKeyValueBucketGetsAgainWhenItsStreamExists(t *testing.T) {
	url := embeddedJetStreamURL(t)
	nc, err := nats.Connect(url)
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), lifecycleBound)
	defer cancel()
	_, err = js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "KV_plain", Subjects: []string{"plain.>"}, Storage: jetstream.MemoryStorage,
	})
	require.NoError(t, err)

	client, err := NewClient(url, WithHealthInterval(0))
	require.NoError(t, err)
	require.NoError(t, client.Connect(ctx))
	closeInCleanup(t, client)

	_, err = client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: "plain", Storage: jetstream.MemoryStorage})
	require.ErrorIs(t, err, jetstream.ErrBadBucket)
	require.ErrorContains(t, err, "access existing bucket plain")
}
