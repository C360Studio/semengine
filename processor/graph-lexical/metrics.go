package graphlexical

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/c360studio/semengine/metric"
)

// metricSubsystem is the Prometheus subsystem of graph-lexical's collectors, derived from its key
// ("graph-lexical" -> "graph_lexical") so the name is spelled once. The readiness gauges are the
// follower's.
var metricSubsystem = strings.ReplaceAll(key, "-", "_")

// lexicalMetrics holds graph-lexical's own collectors: the size of its index, which grows with
// the entities and the text they hold.
type lexicalMetrics struct {
	// entities is the number of entities the index holds.
	entities prometheus.Gauge
	// words is the number of words the index holds, every occurrence in every entity counted.
	words prometheus.Gauge
}

// newLexicalMetrics builds the collectors and registers them on registry, the injected
// component.Dependencies.MetricsRegistry, through metric.RegisterOrGet, keeping the collector each
// call returns. With a nil registry they are built and registered nowhere.
func newLexicalMetrics(registry *metric.MetricsRegistry) (*lexicalMetrics, error) {
	m := &lexicalMetrics{
		entities: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "semengine", Subsystem: metricSubsystem, Name: "index_entities",
			Help: "Entities the word index holds",
		}),
		words: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "semengine", Subsystem: metricSubsystem, Name: "index_words",
			Help: "Words the word index holds, every occurrence in every entity counted",
		}),
	}
	if registry == nil {
		return m, nil
	}
	var err error
	if m.entities, err = metric.RegisterOrGet(registry, key, "index_entities", m.entities); err != nil {
		return nil, err
	}
	if m.words, err = metric.RegisterOrGet(registry, key, "index_words", m.words); err != nil {
		return nil, err
	}
	return m, nil
}
