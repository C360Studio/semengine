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

// TestTruncatedSearchIsCounted: each localSearch, globalSearch or searchGraph answer cut short adds one to
// semengine_graph_query_search_truncated_total for the verb asked, read as a literal value from the
// injected registry, and an answer that is whole, or a request refused, adds none (step 3 design D6;
// step 5 design D4). A nil registry registers nothing on Prometheus' global registry, and the searches
// still answer.
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
	})

	// searchGraph is globalSearch's handler under its own name, counted under that name (design D7).
	for _, verb := range []string{"globalSearch", "searchGraph"} {
		t.Run(verb, func(t *testing.T) {
			client := fixtureClient(t)
			_, m := communityRecords(t, client)
			publishDrones(t, m, 1, drone1)
			registry := metric.NewMetricsRegistry()
			startMetered(t, client, registry)
			awaitTextMark(t, client, 0, 1)
			var more atomic.Bool
			stubPrefix(t, client, &more)
			gatherer := registry.PrometheusRegistry()
			// drone1's group's summary holds "drone", and drone1 matches by its type.
			const text = `{"query":"drone fleet"}`

			for _, tc := range []struct {
				name    string
				more    bool
				request string
				want    float64
			}{
				{"a page that holds the whole group", false, text, 0},
				{"a page with a continuation", true, text, 1},
				{"another", true, text, 2},
				{"a refused request", true, `{"query":"drone fleet","level":3}`, 2},
				{"a whole page again", false, text, 2},
			} {
				more.Store(tc.more)
				// The reply returns only after the handler has counted, so the read is ordered after the write.
				_, _ = client.RequestClassified(t.Context(), verbRow(t, graph.ResponderGraphQuery, verb).Subject, []byte(tc.request), requestTimeout)
				for _, counted := range []string{"localSearch", "globalSearch", "searchGraph"} {
					want := 0.0
					if counted == verb {
						want = tc.want
					}
					if got := truncatedCount(t, gatherer, counted); got != want {
						t.Errorf("%s: %s = %v, want %v", tc.name, counted, got, want)
					}
				}
			}
		})
	}

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
