//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
)

// startMetered starts graph-query on client with registry as its injected metrics registry, nil for none.
func startMetered(t *testing.T, client *natsclient.Client, registry *metric.MetricsRegistry) {
	t.Helper()
	built, err := CreateGraphQuery(json.RawMessage(`{}`), component.Dependencies{NATSClient: client, MetricsRegistry: registry})
	if err != nil {
		t.Fatalf("CreateGraphQuery: %v", err)
	}
	startLifecycle(t, "graph-query", built)
}

// stubPrefix stands in for graph-ingest's prefix verb, answering one matching row, with a
// continuation when more is true: a page of a group larger than the page.
func stubPrefix(t *testing.T, client *natsclient.Client, more *atomic.Bool) {
	t.Helper()
	serve(t, client, verbRow(t, graph.ResponderGraphIngest, "prefix").Subject, func(context.Context, []byte) ([]byte, error) {
		page := fmt.Sprintf(`{"entities":[{"id":%q,"triples":[]}],"total":1`, drone1)
		if more.Load() {
			page += `,"next_cursor":"after"`
		}
		return []byte(page + `}`), nil
	})
}

// TestTruncatedSearchIsCounted: each search answer cut short adds one to
// semengine_graph_query_search_truncated_total{verb="<verb>"}, read as a literal value from the
// injected registry, and an answer that is whole, or a request refused, adds none (design D6). A nil
// registry registers nothing on Prometheus' global registry, and the searches still answer.
func TestTruncatedSearchIsCounted(t *testing.T) {
	const request = `{"entity_id":"` + drone1 + `","query":"drone"}`

	t.Run("localSearch", func(t *testing.T) {
		client := fixtureClient(t)
		_, m := communityRecords(t, client)
		publishDrones(t, m, 1, drone1)
		registry := metric.NewMetricsRegistry()
		startMetered(t, client, registry)
		awaitCommunityView(t, client, drone1, 1)
		var more atomic.Bool
		stubPrefix(t, client, &more)
		gatherer := registry.PrometheusRegistry()

		for _, tc := range []struct {
			name    string
			more    bool
			request string
			want    float64
		}{
			{"a page that holds the whole group", false, request, 0},
			{"a page with a continuation", true, request, 1},
			{"another", true, request, 2},
			{"a refused request", true, `{"entity_id":"x"}`, 2},
			{"a whole page again", false, request, 2},
		} {
			more.Store(tc.more)
			// The reply returns only after the handler has counted, so the read is ordered after the write.
			_, _ = search(t, client, tc.request)
			if got := truncatedCount(t, gatherer, "localSearch"); got != tc.want {
				t.Errorf("%s: localSearch = %v, want %v", tc.name, got, tc.want)
			}
		}
		if got := truncatedCount(t, gatherer, "pathSearch"); got != 0 {
			t.Errorf("pathSearch = %v, want 0: it was never asked", got)
		}
	})

	t.Run("pathSearch", func(t *testing.T) {
		stack := startIngestAndIndex(t)
		registry := metric.NewMetricsRegistry()
		startMetered(t, stack.client, registry)
		// drone1 -> drone2, drone3.
		stack.create(t, drone1, [2]string{linkTo, drone2}, [2]string{linkTo, drone3})
		stack.create(t, drone2)
		stack.awaitIndexed(t, stack.create(t, drone3))
		gatherer := registry.PrometheusRegistry()

		for _, tc := range []struct {
			name  string
			nodes int
			want  float64
		}{
			{"the limit meets the graph", 3, 0},
			{"the limit one lower", 2, 1},
			{"the limit is the start", 1, 2},
		} {
			if _, err := askPathSearch(t, stack.client,
				fmt.Sprintf(`{"start_entity":%q,"direction":"outgoing","max_depth":1,"max_nodes":%d}`, drone1, tc.nodes)); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if got := truncatedCount(t, gatherer, "pathSearch"); got != tc.want {
				t.Errorf("%s: pathSearch = %v, want %v", tc.name, got, tc.want)
			}
		}
	})

	t.Run("a nil registry", func(t *testing.T) {
		client := fixtureClient(t)
		_, m := communityRecords(t, client)
		publishDrones(t, m, 1, drone1)
		startMetered(t, client, nil)
		awaitCommunityView(t, client, drone1, 1)
		var more atomic.Bool
		more.Store(true)
		stubPrefix(t, client, &more)
		raw, err := search(t, client, request)
		if _, data := searchSuccess(t, raw, err); !data.EntitiesTruncated {
			t.Fatalf("reply = %s, want a truncated answer", raw)
		}
		if got := truncatedCount(t, prometheus.DefaultGatherer, "localSearch"); got != 0 {
			t.Errorf("the global registry gathered localSearch = %v, want none registered", got)
		}
	})
}
