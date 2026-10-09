//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/inference"
	"github.com/c360studio/semengine/internal/graphmutation"
	"github.com/c360studio/semengine/internal/harness/payloadfixture"
	"github.com/c360studio/semengine/message"
)

// TestResidentUnregisteredStampIsNotPoison documents design §10 and is a
// barrier against a later registry-consulting codec: an entity persisted under
// a key no binary registers is swept without a poison entry, reads back with
// the stamp unchanged, and stays mutable through must-exist operations.
func TestResidentUnregisteredStampIsNotPoison(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	nc := newFixtureClient(t, entityStream)

	const id = "c360.test.resident.system.legacy.001"
	now := time.Now()
	kv, err := nc.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{Bucket: graph.BucketEntityStates})
	require.NoError(t, err)
	resident := &graph.EntityState{
		ID: id, MessageType: message.Type{Domain: "legacy", Category: "gone", Version: "v1"},
		UpdatedAt: now,
		Triples:   []message.Triple{{Subject: id, Predicate: "test.state.value", Object: "resident", Timestamp: now, Confidence: 1}},
	}
	encoded, err := graph.MarshalEntityState(resident)
	require.NoError(t, err)
	_, err = kv.Put(ctx, id, encoded)
	require.NoError(t, err)

	configJSON, err := json.Marshal(DefaultConfig())
	require.NoError(t, err)
	// The pin's registry was payloadbuiltins.NewTestRegistry; design D2 takes the
	// per-package RegisterPayloads graph-ingest's fixture registry uses. Neither holds
	// legacy.gone.v1.
	comp, err := CreateGraphIngest(configJSON, component.Dependencies{
		NATSClient: nc, PayloadRegistry: payloadfixture.NewWithSubset(t, message.RegisterPayloads, inference.RegisterPayloads),
		Platform: component.PlatformMeta{Org: "c360", Platform: "test"},
	})
	require.NoError(t, err)
	c := comp.(*Component)
	owner := newGraphIngestTestOwner(c)
	defer owner.finish(ctx, t)
	require.NoError(t, c.Initialize())
	require.NoError(t, c.Start(owner.startContext(ctx)))

	_, inventoried := poisonInventoryEntry(c, id)
	assert.False(t, inventoried, "a resident unregistered stamp is not poison")

	exact, err := graph.NewExactEntityReader(nc, 5*time.Second).ReadExactEntity(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "legacy.gone.v1", exact.Entity.MessageType.Key(), "the stamp reads back unchanged")

	client, err := graphmutation.NewClient(nc, 5*time.Second)
	require.NoError(t, err)
	appended, err := client.Append(ctx, graph.AppendTriplesRequest{Triples: withTestMetadata(message.Triple{
		Subject: id, Predicate: "test.event.value", Object: "appended", Timestamp: now, Confidence: 1,
	})})
	require.NoError(t, err)
	require.Len(t, appended.Results, 1)
	assert.Equal(t, graph.MutationApplied, appended.Results[0].Outcome, "must-exist mutations ignore the stamp")
}
