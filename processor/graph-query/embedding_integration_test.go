//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/errs"
	graphembedding "github.com/c360studio/semengine/processor/graph-embedding"
)

// TestSemanticForwardsMinRevision: graph-query's semantic verb forwards its request, min_revision
// included, unchanged to a real graph-embedding, and passes its answer and its refusal back. The
// embedding verbs claim no ENTITY_STATES revision, so a min_revision above 0 is refused class
// invalid, invalid_request, never answered, and 0 is answered with indexed_revision 0 (change 5
// design D4; graph-embedding spec, "The embedding verbs claim no revision"; graph-query spec, the
// derived-index reads receive min_revision unchanged).
func TestSemanticForwardsMinRevision(t *testing.T) {
	client := fixtureClient(t)
	if _, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEntityStates); err != nil {
		t.Fatalf("ensure %s: %v", graph.BucketEntityStates, err)
	}
	built, err := graphembedding.CreateGraphEmbedding(json.RawMessage(`{}`), component.Dependencies{
		NATSClient: client, MetricsRegistry: metric.NewMetricsRegistry(),
	})
	if err != nil {
		t.Fatalf("CreateGraphEmbedding: %v", err)
	}
	startLifecycle(t, "graph-embedding", built)
	startComponent(t, client, `{}`)
	semantic := verbRow(t, graph.ResponderGraphQuery, "semantic").Subject

	// Answered once graph-embedding's cache has applied its first snapshot; until then it refuses
	// index_not_ready, and the reply that ends the wait is the event this reads.
	type outcome struct {
		raw []byte
		err error
	}
	got, err := probe.Await(t.Context(), func(ctx context.Context) (outcome, error) {
		raw, err := client.RequestClassified(ctx, semantic, []byte(`{"query":"widget","min_revision":0}`), requestTimeout)
		return outcome{raw, err}, nil
	}, func(o outcome) bool {
		var refusal *errs.ClassifiedError
		return !(errors.As(o.err, &refusal) && refusal.Code == graph.ErrorCodeIndexNotReady)
	})
	if err != nil || got.err != nil {
		t.Fatalf("semantic with min_revision 0: %v, %v", err, got.err)
	}
	var reply graph.QueryResponse[graph.SemanticData]
	if err := json.Unmarshal(got.raw, &reply); err != nil {
		t.Fatalf("semantic reply %s: %v", got.raw, err)
	}
	if reply.Producer != graph.ResponderGraphEmbedding || reply.IndexedRevision != 0 {
		t.Errorf("semantic reply = %s, want producer graph-embedding and indexed_revision 0", got.raw)
	}

	raw, err := client.RequestClassified(t.Context(), semantic, []byte(`{"query":"widget","min_revision":7}`), requestTimeout)
	if err == nil {
		t.Fatalf("semantic with min_revision 7 answered %s; want a refusal", raw)
	}
	if refusal := refusalOf(t, err); refusal.Class != errs.ErrorInvalid || refusal.Code != graph.ErrorCodeInvalidRequest {
		t.Errorf("semantic with min_revision 7: class %v code %q (%v), want invalid %s", refusal.Class, refusal.Code,
			err, graph.ErrorCodeInvalidRequest)
	}
}
