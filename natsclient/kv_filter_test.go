package natsclient

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// watchFilteredOnly offers FilteredKeys nothing but WatchFiltered, so the test
// fails to compile if FilteredKeys asks its reader for more.
type watchFilteredOnly struct {
	watcher jetstream.KeyWatcher
	filters []string
}

func (r *watchFilteredOnly) WatchFiltered(
	_ context.Context,
	filters []string,
	_ ...jetstream.WatchOpt,
) (jetstream.KeyWatcher, error) {
	r.filters = append([]string(nil), filters...)
	return r.watcher, nil
}

func TestFilteredKeysAcceptsMinimalReader(t *testing.T) {
	watcher := closedOnStopWatcher(ownershipEntry{key: "one"}, nil)
	reader := &watchFilteredOnly{watcher: watcher}

	keys, err := FilteredKeys(context.Background(), reader, "domain.>")

	require.NoError(t, err)
	assert.Equal(t, []string{"one"}, keys)
	assert.Equal(t, []string{"domain.>"}, reader.filters)
	assert.EqualValues(t, 1, watcher.calls.Load())
}
