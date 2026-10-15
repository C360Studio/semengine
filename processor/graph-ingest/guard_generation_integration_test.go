//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
)

// restartConsumer is the durable consumer graph-ingest binds on the ENTITY stream's entity.> subject.
const restartConsumer = "graph-ingest-entity-wildcard"

// startIngestOn starts graph-ingest with its default configuration on natsClient, under the c360.test
// authority, with the merge-test payload registered before Start.
func startIngestOn(ctx context.Context, t *testing.T, natsClient *natsclient.Client) *graphIngestTestOwner {
	t.Helper()
	cfgJSON, err := json.Marshal(DefaultConfig())
	require.NoError(t, err)
	comp, err := CreateGraphIngest(cfgJSON, testDependencies(t, natsClient, withAuthority("c360", "test")))
	require.NoError(t, err)
	c := comp.(*Component)
	owner := newGraphIngestTestOwner(c)
	defer owner.provisionalFinish(ctx, t)
	require.NoError(t, c.Initialize())
	registerMergeTestPayload(t, c)
	require.NoError(t, c.Start(owner.startContext(ctx)))
	owner.transfer()
	return owner
}

// publishTriples publishes one input for entityID carrying triples and returns its stream sequence.
func publishTriples(ctx context.Context, t *testing.T, natsClient *natsclient.Client, entityID string, triples ...message.Triple) uint64 {
	t.Helper()
	payload := &mergeTestGraphable{entityID: entityID, triples: triples}
	data, err := json.Marshal(message.NewBaseMessage(payload.Schema(), payload, "test-source"))
	require.NoError(t, err)
	ack, err := natsClient.PublishToStreamWithAck(ctx, "entity."+entityID, data)
	require.NoError(t, err)
	return ack.Sequence
}

// waitSettled waits until graph-ingest's consumer has acknowledged every input up to seq.
func waitSettled(ctx context.Context, t *testing.T, f *natsfixture.Fixture, seq uint64) {
	t.Helper()
	require.Eventually(t, func() bool {
		consumer, err := f.JetStream().Consumer(ctx, entityStream.name, restartConsumer)
		if err != nil {
			return false
		}
		info, err := consumer.Info(ctx)
		return err == nil && info.AckFloor.Stream >= seq && info.NumAckPending == 0
	}, 20*time.Second, 20*time.Millisecond, "graph-ingest did not acknowledge the inputs up to sequence %d", seq)
}

// TestIntegration_IngestGuard_StreamRecreatedWithLowerSequences holds graph-ingest-recovery,
// "Stream recreated with lower sequences" (design D8 row #15, task 4.3), the shape SemSource SETUP 03A
// reproduced: a memory input stream, the applied-sequence bucket retained across a broker restart,
// and the current source re-ingested into the recreated stream at sequences lower than the recorded
// ones. The re-ingested relationships and content are applied, and an entity query returns them.
func TestIntegration_IngestGuard_StreamRecreatedWithLowerSequences(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancelOperation()
	const (
		entityID = "c360.test.wire.restart.entity.001"
		targetID = "c360.test.wire.restart.target.001"
		content  = "test.content.body"
		relation = "test.relation.target"
	)
	now := time.Now()

	// Own server: restarts the server mid-test.
	f := natsfixture.New(t)
	require.NoError(t, f.Start(ctx))
	_, err := natsfixture.CreateStream(ctx, f.JetStream(), jetstream.MemoryStorage, entityStream.name, entityStream.subjects...)
	require.NoError(t, err)

	// The first generation: three older states of the entity, at sequences 1 to 3.
	first, closeFirst, err := openFixtureClient(ctx, f.URL())
	require.NoError(t, err)
	firstClosed := false
	t.Cleanup(func() {
		if firstClosed {
			return
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := closeFirst(closeCtx); err != nil {
			t.Errorf("close the first client: %v", err)
		}
	})
	firstOwner := startIngestOn(ctx, t, first)
	defer firstOwner.finish(ctx, t)
	var recorded uint64
	for i := 1; i <= 3; i++ {
		recorded = publishTriples(ctx, t, first, entityID, message.Triple{
			Subject: entityID, Predicate: content, Object: fmt.Sprintf("older-%d", i), Timestamp: now, Confidence: 1.0,
		})
	}
	waitSettled(ctx, t, f, recorded)

	// Stop what was built on the old URL, restart the broker, and start graph-ingest on the new one.
	// The memory stream does not survive the restart; the applied-sequence and entity buckets do.
	require.NoError(t, firstOwner.stop(ctx))
	firstClosed = true
	require.NoError(t, closeFirst(ctx))
	require.NoError(t, f.Restart(ctx))
	_, err = natsfixture.CreateStream(ctx, f.JetStream(), jetstream.MemoryStorage, entityStream.name, entityStream.subjects...)
	require.NoError(t, err)
	second := natsfixture.Open(t, f.URL(), openFixtureClient)
	secondOwner := startIngestOn(ctx, t, second)
	defer secondOwner.finish(ctx, t)

	// Re-ingestion of the current source, at a sequence lower than the one recorded.
	reingested := publishTriples(ctx, t, second, entityID,
		message.Triple{Subject: entityID, Predicate: content, Object: "current", Timestamp: now, Confidence: 1.0},
		message.Triple{Subject: entityID, Predicate: relation, Object: targetID, Timestamp: now, Confidence: 1.0},
	)
	require.Less(t, reingested, recorded, "the recreated stream must restart its sequences")
	waitSettled(ctx, t, f, reingested)

	req, err := json.Marshal(map[string]string{"id": entityID})
	require.NoError(t, err)
	body, err := second.RequestClassified(ctx, "graph.ingest.query.entity", req, wireRequestTimeout)
	require.NoError(t, err)
	var exact graph.ExactEntity
	require.NoError(t, json.Unmarshal(body, &exact))
	require.NotNil(t, exact.Entity)
	var contents, targets []any
	for _, tr := range exact.Entity.Triples {
		switch tr.Predicate {
		case content:
			contents = append(contents, tr.Object)
		case relation:
			targets = append(targets, tr.Object)
		}
	}
	require.Equal(t, []any{"current"}, contents, "the queried entity must carry the re-ingested content only")
	require.Equal(t, []any{targetID}, targets, "the queried entity must carry the re-ingested relationship")
}

// TestIntegration_IngestGuard_FileStreamKeepsItsGenerationAcrossRestart holds "Redelivery within one
// generation" across a broker restart. A file-backed stream survives the restart as the same
// generation: the server keeps its creation time. With graph-ingest's consumer deleted, its
// replacement delivers every input again from sequence 1, and each is dropped as already applied:
// the entity's stored revision does not change.
func TestIntegration_IngestGuard_FileStreamKeepsItsGenerationAcrossRestart(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancelOperation()
	const entityID = "c360.test.wire.restart.entity.002"
	now := time.Now()

	// Own server: restarts the server mid-test.
	f := natsfixture.New(t)
	require.NoError(t, f.Start(ctx))
	_, err := natsfixture.CreateStream(ctx, f.JetStream(), jetstream.FileStorage, entityStream.name, entityStream.subjects...)
	require.NoError(t, err)

	first, closeFirst, err := openFixtureClient(ctx, f.URL())
	require.NoError(t, err)
	firstClosed := false
	t.Cleanup(func() {
		if firstClosed {
			return
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := closeFirst(closeCtx); err != nil {
			t.Errorf("close the first client: %v", err)
		}
	})
	firstOwner := startIngestOn(ctx, t, first)
	defer firstOwner.finish(ctx, t)
	var last uint64
	for _, value := range []string{"v1", "v2"} {
		last = publishTriples(ctx, t, first, entityID, message.Triple{
			Subject: entityID, Predicate: "test.content.body", Object: value, Timestamp: now, Confidence: 1.0,
		})
	}
	waitSettled(ctx, t, f, last)
	_, revision, err := firstOwner.component.readEntity(ctx, entityID)
	require.NoError(t, err)

	require.NoError(t, firstOwner.stop(ctx))
	firstClosed = true
	require.NoError(t, closeFirst(ctx))
	require.NoError(t, f.Restart(ctx))
	require.NoError(t, f.JetStream().DeleteConsumer(ctx, entityStream.name, restartConsumer))
	second := natsfixture.Open(t, f.URL(), openFixtureClient)
	secondOwner := startIngestOn(ctx, t, second)
	defer secondOwner.finish(ctx, t)
	waitSettled(ctx, t, f, last)

	state, got, err := secondOwner.component.readEntity(ctx, entityID)
	require.NoError(t, err)
	require.Equal(t, revision, got, "a redelivery within the generation must not write the entity again")
	var contents []any
	for _, tr := range state.Triples {
		if tr.Predicate == "test.content.body" {
			contents = append(contents, tr.Object)
		}
	}
	require.Equal(t, []any{"v2"}, contents)
}
