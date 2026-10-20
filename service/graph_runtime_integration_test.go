//go:build integration

package service

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/projection"
	"github.com/c360studio/semengine/vocabulary"
)

// graphRuntimeContract is a valid mutation contract, so the mutation client builds; its predicates
// are registered for the test only. The shape is pkg/projection's projectionTestContract.
func graphRuntimeContract(t *testing.T) projection.Contract {
	t.Helper()
	t.Cleanup(vocabulary.SnapshotRegistry())
	vocabulary.Register("test.identity.name")
	vocabulary.Register("test.value.name")
	return projection.Contract{
		Name: "graph-runtime", MessageType: message.Type{Domain: "test", Category: "fixture", Version: "v1"},
		EntityPattern:   "*.*.test.system.widget.*",
		BirthPredicates: []string{"test.identity.name"},
		Groups: []projection.PredicateGroup{
			{Name: "state", Mode: projection.ModeReconcile, Predicates: []string{"test.value.name"}},
		},
	}
}

// TestWireGraphRuntimeStripsAForeignTTL: WireGraphRuntime runs the catalog-retention backstop
// before it builds the mutation client, so a catalog bucket left with a foreign TTL has it
// stripped and keeps its keys (design D4; pin ADR-068 D1; the bucket set-up is
// graph/kvcatalog's TestIntegration_EnsureCatalogRetentionClean_StripsForeignTTL).
//
// spec: service-lifecycle / WireGraphRuntime cleans catalog retention first
func TestWireGraphRuntimeStripsAForeignTTL(t *testing.T) {
	ctx := t.Context()
	client := fixtureClient(t)

	dirty, err := client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
		Bucket: graph.BucketEmbeddingIndex,
		TTL:    7 * 24 * time.Hour,
	})
	require.NoError(t, err)
	_, err = dirty.Put(ctx, "entity.key.one", []byte("survivor"))
	require.NoError(t, err)
	maxAge, _, err := natsclient.BucketRetention(ctx, dirty)
	require.NoError(t, err)
	require.Equal(t, 7*24*time.Hour, maxAge, "precondition: the bucket carries the foreign TTL")

	mutations, err := WireGraphRuntime(ctx, client, nil, graphRuntimeContract(t))
	require.NoError(t, err)
	require.NotNil(t, mutations)

	fresh, err := client.GetKeyValueBucket(ctx, graph.BucketEmbeddingIndex)
	require.NoError(t, err)
	maxAge, _, err = natsclient.BucketRetention(ctx, fresh)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), maxAge, "WireGraphRuntime left the foreign TTL in place")
	entry, err := fresh.Get(ctx, "entity.key.one")
	require.NoError(t, err, "the stored key must survive the strip")
	assert.Equal(t, []byte("survivor"), entry.Value())
}

// TestWireGraphRuntimeRefusesANilContext: a nil context is refused as invalid by the backstop, and
// no client is returned (design D4).
//
// spec: service-lifecycle / WireGraphRuntime cleans catalog retention first
func TestWireGraphRuntimeRefusesANilContext(t *testing.T) {
	client := fixtureClient(t)
	var nilCtx context.Context

	mutations, err := WireGraphRuntime(nilCtx, client, nil, graphRuntimeContract(t))
	require.Error(t, err)
	assert.True(t, errs.IsInvalid(err), "want an invalid-class error, got %v", err)
	assert.Nil(t, mutations)
}
