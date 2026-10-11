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
