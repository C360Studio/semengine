//go:build integration

package graphingest

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/c360studio/semengine/graph"
)

// TestIntegration_DamagedRefusalNamesEntitiesInDetail: a caller refused for damaged entities reads
// which ones from the refusal's detail, through RequestClassified as a product would, without
// parsing its message. The entity verb on damaged B names B; a prefix page over A, B and C, with B
// and C damaged, names B and C, sorted.
func TestIntegration_DamagedRefusalNamesEntitiesInDetail(t *testing.T) {
	ctx := t.Context()
	c, owner := startBatchTestComponent(ctx, t)
	defer owner.finish(ctx, t)

	const prefix = "c360.test.damageddetail.system.drone"
	idA, idB, idC := prefix+".001", prefix+".002", prefix+".003"
	createDrones(ctx, t, c, idA)
	// C is stored before B, so a list in write order would not be sorted.
	putDamaged(ctx, t, c, idC)
	putDamaged(ctx, t, c, idB)

	t.Run("entity verb", func(t *testing.T) {
		body, err := json.Marshal(map[string]string{"id": idB})
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.natsClient.RequestClassified(ctx, ingestVerbSubject(t, "entity"), body, wireRequestTimeout)
		assertIngestResetRequired(t, err)
		if got := graph.StateRefusalEntityIDs(err); !slices.Equal(got, []string{idB}) {
			t.Errorf("StateRefusalEntityIDs = %v, want [%s]; refusal %v", got, idB, err)
		}
	})

	t.Run("prefix verb", func(t *testing.T) {
		body, err := json.Marshal(graph.PrefixQueryRequest{Prefix: prefix, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.natsClient.RequestClassified(ctx, ingestVerbSubject(t, "prefix"), body, wireRequestTimeout)
		assertIngestResetRequired(t, err)
		if got := graph.StateRefusalEntityIDs(err); !slices.Equal(got, []string{idB, idC}) {
			t.Errorf("StateRefusalEntityIDs = %v, want [%s %s]; refusal %v", got, idB, idC, err)
		}
	})
}
