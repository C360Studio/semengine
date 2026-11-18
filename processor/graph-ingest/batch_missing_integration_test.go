//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_BatchQuery_ReportsMissingIDs drives the production handler: a requested ID
// whose KV read comes back not-found must not vanish from the response, leaving the caller a
// shorter list with no correspondence to what it asked for. Nothing downstream could tell
// "this entity does not exist" from "this read did not reach it", and the symptom would
// surface several hops away as a missing search result.
//
// The response names every unhydrated ID with a reason, and — this is the part worth pinning —
// the entity list is UNCHANGED for a fully-hydrated batch, so a consumer that only reads
// `entities` is not broken by the `missing` list.
func TestIntegration_BatchQuery_ReportsMissingIDs(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	c, owner := startBatchTestComponent(ctx, t)
	defer owner.finish(ctx, t)

	present := "c360.test.batchmissing.system.drone.001"
	alsoPresent := "c360.test.batchmissing.system.drone.002"
	absent := "c360.test.batchmissing.system.drone.404"
	alsoAbsent := "c360.test.batchmissing.system.drone.405"

	for _, id := range []string{present, alsoPresent} {
		require.NoError(t, c.CreateEntity(ctx, &graph.EntityState{
			ID:          id,
			MessageType: testEntityType(),
			Triples: withTestMetadata(message.Triple{
				Subject: id, Predicate: "core.identity.type", Object: "drone", Confidence: 1.0,
			}),
		}))
	}

	query := func(t *testing.T, ids []string) graph.EntityBatchResponse {
		t.Helper()
		body, err := json.Marshal(map[string]any{"ids": ids})
		require.NoError(t, err)
		raw, err := c.handleQueryBatchNATS(ctx, body)
		require.NoError(t, err, "a not-found ID must not fail the call — partiality is not the bug")
		var resp graph.EntityBatchResponse
		require.NoError(t, json.Unmarshal(raw, &resp))
		return resp
	}

	t.Run("missing IDs are named with a reason", func(t *testing.T) {
		resp := query(t, []string{present, absent, alsoPresent, alsoAbsent})

		assert.Len(t, resp.Entities, 2, "both existing entities must still hydrate")
		require.Len(t, resp.Missing, 2, "both absent IDs must be reported, not dropped")

		byID := map[string]graph.MissingReason{}
		for _, m := range resp.Missing {
			byID[m.ID] = m.Reason
		}
		assert.Equal(t, graph.MissingNotFound, byID[absent])
		assert.Equal(t, graph.MissingNotFound, byID[alsoAbsent])

		// Every requested ID is accounted for in exactly one of the two lists. This is
		// the property that makes the response reconcilable at all — without it a
		// caller is back to diffing sets and guessing.
		accounted := map[string]int{}
		for _, e := range resp.Entities {
			accounted[e.ID]++
		}
		for _, m := range resp.Missing {
			accounted[m.ID]++
		}
		for _, id := range []string{present, absent, alsoPresent, alsoAbsent} {
			assert.Equal(t, 1, accounted[id], "id %s must appear exactly once across entities+missing", id)
		}
	})

	t.Run("a fully hydrated batch omits the field entirely", func(t *testing.T) {
		body, err := json.Marshal(map[string]any{"ids": []string{present, alsoPresent}})
		require.NoError(t, err)
		raw, err := c.handleQueryBatchNATS(ctx, body)
		require.NoError(t, err)

		// Asserted on the RAW wire, not the decoded struct: the compatibility claim is
		// that the bytes an old consumer sees are unchanged, which a struct with an
		// empty slice would hide.
		var keyed map[string]any
		require.NoError(t, json.Unmarshal(raw, &keyed))
		_, present := keyed["missing"]
		assert.False(t, present, "a complete batch must not grow a missing key: %s", raw)
		assert.Contains(t, keyed, "entities", "the pre-existing field must keep its name")
	})

	t.Run("an all-missing batch is a success with an empty entity list", func(t *testing.T) {
		resp := query(t, []string{absent, alsoAbsent})
		assert.Empty(t, resp.Entities)
		assert.Len(t, resp.Missing, 2,
			"an all-missing batch must report every ID rather than looking like an empty graph")
	})
}

// TestIntegration_BatchQuery_EmptyIDIsAccountedFor pins the totality contract against
// its most likely accidental input. An empty requested ID skipped outright would
// appear in NEITHER entities nor missing — the response would claim to account for
// every requested ID while quietly dropping one.
//
// It is reported as `error`, not `not_found`: the key was never looked up, and calling
// it not-found would assert something unobserved, which is the exact move the missing
// list exists to stop.
func TestIntegration_BatchQuery_EmptyIDIsAccountedFor(t *testing.T) {
	ctx, cancelOperation := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancelOperation()
	c, owner := startBatchTestComponent(ctx, t)
	defer owner.finish(ctx, t)

	present := "c360.test.batchempty.system.drone.001"
	require.NoError(t, c.CreateEntity(ctx, &graph.EntityState{
		ID:          present,
		MessageType: testEntityType(),
		Triples: withTestMetadata(message.Triple{
			Subject: present, Predicate: "core.identity.type", Object: "drone", Confidence: 1.0,
		}),
	}))

	body, err := json.Marshal(map[string]any{"ids": []string{present, ""}})
	require.NoError(t, err)
	raw, err := c.handleQueryBatchNATS(ctx, body)
	require.NoError(t, err, "a malformed ID must not fail the whole batch")

	var resp graph.EntityBatchResponse
	require.NoError(t, json.Unmarshal(raw, &resp))

	assert.Len(t, resp.Entities, 1)
	require.Len(t, resp.Missing, 1, "the empty ID must appear in the accounting, not vanish")
	assert.Equal(t, "", resp.Missing[0].ID)
	assert.Equal(t, graph.MissingError, resp.Missing[0].Reason,
		"an ID that was never looked up cannot be reported as not_found")
}

// TestIntegration_BatchQuery_NamesDamagedEntities sends a batch over NATS for two stored
// entities, one whose stored bytes the stored-value rule refuses, and one absent ID. The
// caller named every ID, so the reply returns the two readable entities and names the
// damaged one `damaged` and the absent one `not_found`, rather than refusing the whole
// request. The damaged entity is still recorded, so graph-ingest reports itself unhealthy,
// and the entity verb on it still refuses.
func TestIntegration_BatchQuery_NamesDamagedEntities(t *testing.T) {
	ctx := t.Context()
	c, owner := startBatchTestComponent(ctx, t)
	defer owner.finish(ctx, t)

	idA := "c360.test.batchdamaged.system.drone.001"
	idB := "c360.test.batchdamaged.system.drone.002"
	idC := "c360.test.batchdamaged.system.drone.003"
	absent := "c360.test.batchdamaged.system.drone.404"
	createDrones(ctx, t, c, idA, idC)
	putDamaged(ctx, t, c, idB)

	before := batchMissingSeries(t, c)
	assert.Contains(t, before, string(graph.MissingDamaged), "the damaged series must exist before any request")
	assert.Zero(t, before[string(graph.MissingDamaged)])

	resp := requestBatch(ctx, t, c, []string{idA, idB, idC, absent})
	var got []string
	for _, e := range resp.Entities {
		got = append(got, e.ID)
	}
	assert.ElementsMatch(t, []string{idA, idC}, got, "the readable entities must be returned")
	assert.ElementsMatch(t, []graph.MissingEntity{
		{ID: idB, Reason: graph.MissingDamaged},
		{ID: absent, Reason: graph.MissingNotFound},
	}, resp.Missing)

	// The reply arrived after the handler returned, and the handler records the damaged
	// entity and counts the missing IDs before it replies.
	assert.False(t, c.Health().Healthy, "a recorded damaged entity makes graph-ingest unhealthy")
	after := batchMissingSeries(t, c)
	assert.Equal(t, before[string(graph.MissingDamaged)]+1, after[string(graph.MissingDamaged)])
	assert.Equal(t, before[string(graph.MissingNotFound)]+1, after[string(graph.MissingNotFound)])

	entityRequest, err := json.Marshal(map[string]string{"id": idB})
	require.NoError(t, err)
	_, err = c.natsClient.RequestClassified(ctx, ingestVerbSubject(t, "entity"), entityRequest, wireRequestTimeout)
	assertIngestResetRequired(t, err)
}

// TestIntegration_BatchQuery_AllDamaged: a batch naming only a damaged entity is answered,
// not refused, with no entities and the ID named `damaged`.
func TestIntegration_BatchQuery_AllDamaged(t *testing.T) {
	ctx := t.Context()
	c, owner := startBatchTestComponent(ctx, t)
	defer owner.finish(ctx, t)

	idB := "c360.test.batchalldamaged.system.drone.002"
	putDamaged(ctx, t, c, idB)

	resp := requestBatch(ctx, t, c, []string{idB})
	require.NotNil(t, resp.Entities, "the reply carries an empty entity list, not none")
	assert.Empty(t, resp.Entities)
	assert.Equal(t, []graph.MissingEntity{{ID: idB, Reason: graph.MissingDamaged}}, resp.Missing)
}

func createDrones(ctx context.Context, t *testing.T, c *Component, ids ...string) {
	t.Helper()
	for _, id := range ids {
		require.NoError(t, c.CreateEntity(ctx, &graph.EntityState{
			ID:          id,
			MessageType: testEntityType(),
			Triples: withTestMetadata(message.Triple{
				Subject: id, Predicate: "core.identity.type", Object: "drone", Confidence: 1.0,
			}),
		}))
	}
}

// putDamaged stores bytes under id that the stored-value rule refuses, a two-part
// predicate, straight through the KV bucket as an outside writer would. Put returns once
// the value is stored, so every later read sees it.
func putDamaged(ctx context.Context, t *testing.T, c *Component, id string) {
	t.Helper()
	js, err := c.natsClient.JetStream()
	require.NoError(t, err)
	kv, err := js.KeyValue(ctx, graph.BucketEntityStates)
	require.NoError(t, err)
	_, err = kv.Put(ctx, id,
		[]byte(`{"id":"`+id+`","triples":[{"subject":"`+id+`","predicate":"legacy.predicate","object":1}]}`)) // predicate-audit:invalid {"kind":"stored-predicate","value":"legacy.predicate","reason":"arity"}
	require.NoError(t, err)
}

// requestBatch sends ids on graph-ingest's batch verb and decodes the reply; a refusal
// fails the test.
func requestBatch(ctx context.Context, t *testing.T, c *Component, ids []string) graph.EntityBatchResponse {
	t.Helper()
	body, err := json.Marshal(graph.EntityBatchRequest{IDs: ids})
	require.NoError(t, err)
	raw, err := c.natsClient.RequestClassified(ctx, ingestVerbSubject(t, "batch"), body, wireRequestTimeout)
	require.NoError(t, err, "a damaged entity must not refuse the batch")
	var resp graph.EntityBatchResponse
	require.NoError(t, json.Unmarshal(raw, &resp))
	return resp
}

// ingestVerbSubject is the subject graph-ingest answers verb name on, from the verb table.
func ingestVerbSubject(t *testing.T, name string) string {
	t.Helper()
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder == graph.ResponderGraphIngest && verb.Name == name {
			return verb.Subject
		}
	}
	t.Fatalf("the verb table has no graph-ingest verb %q", name)
	return ""
}

// batchMissingSeries reads every series of the batch missing counter by reason, without
// creating one, so a series that was never seeded is absent from the map.
func batchMissingSeries(t *testing.T, c *Component) map[string]float64 {
	t.Helper()
	ch := make(chan prometheus.Metric, 8)
	c.batchMissing.Collect(ch)
	close(ch)
	series := map[string]float64{}
	for m := range ch {
		var pb dto.Metric
		require.NoError(t, m.Write(&pb))
		for _, label := range pb.GetLabel() {
			if label.GetName() == "reason" {
				series[label.GetValue()] = pb.GetCounter().GetValue()
			}
		}
	}
	return series
}
