//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
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

// stubOneHit stands in for graph-lexical's word search and graph-ingest's batch verb, answering one
// hit, drone1, of two matches when more is true and of one when it is false.
func stubOneHit(t *testing.T, client *natsclient.Client, more *atomic.Bool) {
	t.Helper()
	hit := []graph.LexicalHit{{EntityID: drone1, Score: 1}}
	whole := lexicalReply(t, 1, graph.LexicalSearchData{Results: hit, Matched: 1})
	cut := lexicalReply(t, 1, graph.LexicalSearchData{Results: hit, Matched: 2})
	serve(t, client, verbRow(t, graph.ResponderGraphLexical, "search").Subject, func(context.Context, []byte) ([]byte, error) {
		if more.Load() {
			return cut, nil
		}
		return whole, nil
	})
	stubBatch(t, client, `{"entities":[{"id":"`+drone1+`","triples":[]}]}`)
}

// TestTruncatedSearchIsCounted: each localSearch, globalSearch or searchGraph answer cut short adds
// one to semengine_graph_query_search_truncated_total for the verb asked, read as a literal value
// from the injected registry, and an answer that is whole, or a request refused, adds none. A nil
// registry registers nothing on Prometheus' global registry, and the searches still answer.
func TestTruncatedSearchIsCounted(t *testing.T) {
	const request = `{"entity_id":"` + drone1 + `","query":"drone"}`

	t.Run("localSearch", func(t *testing.T) {
		client := fixtureClient(t)
		registry := metric.NewMetricsRegistry()
		startMetered(t, client, registry)
		var more atomic.Bool
		stubOneHit(t, client, &more)
		gatherer := registry.PrometheusRegistry()

		for _, tc := range []struct {
			name    string
			more    bool
			request string
			want    float64
		}{
			{"every match returned", false, request, 0},
			{"more matched than returned", true, request, 1},
			{"another", true, request, 2},
			{"a refused request", true, `{"entity_id":"x"}`, 2},
			{"every match again", false, request, 2},
		} {
			more.Store(tc.more)
			// The reply returns only after the handler has counted, so the read is ordered after the write.
			_, _ = search(t, client, tc.request)
			if got := truncatedCount(t, gatherer, "localSearch"); got != tc.want {
				t.Errorf("%s: localSearch = %v, want %v", tc.name, got, tc.want)
			}
		}
	})

	// searchGraph is globalSearch's handler under its own name, counted under that name.
	for _, verb := range []string{"globalSearch", "searchGraph"} {
		t.Run(verb, func(t *testing.T) {
			client := fixtureClient(t)
			registry := metric.NewMetricsRegistry()
			startMetered(t, client, registry)
			var more atomic.Bool
			stubOneHit(t, client, &more)
			gatherer := registry.PrometheusRegistry()
			// Two words and no hint: text alone.
			const text = `{"query":"drone fleet"}`

			for _, tc := range []struct {
				name    string
				more    bool
				request string
				want    float64
			}{
				{"every match returned", false, text, 0},
				{"more matched than returned", true, text, 1},
				{"another", true, text, 2},
				{"a refused request", true, `{"query":"drone fleet","level":3}`, 2},
				{"every match again", false, text, 2},
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
		startMetered(t, client, nil)
		var more atomic.Bool
		more.Store(true)
		stubOneHit(t, client, &more)
		raw, err := search(t, client, request)
		if _, data := searchSuccess(t, raw, err); !data.Truncated {
			t.Fatalf("reply = %s, want a truncated answer", raw)
		}
		if got := truncatedCount(t, prometheus.DefaultGatherer, "localSearch"); got != 0 {
			t.Errorf("the global registry gathered localSearch = %v, want none registered", got)
		}
	})
}
