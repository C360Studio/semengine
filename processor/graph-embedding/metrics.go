package graphembedding

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/c360studio/semengine/graph/embedding"
	"github.com/c360studio/semengine/metric"
)

// metricSubsystem is the Prometheus subsystem of graph-embedding's collectors, derived from
// its key ("graph-embedding" -> "graph_embedding") so the name is spelled once. The readiness
// gauges are the follower's.
var metricSubsystem = strings.ReplaceAll(key, "-", "_")

// embeddingMetrics holds graph-embedding's own collectors.
type embeddingMetrics struct {
	// contentUnresolved counts entities whose offloaded body was left out of the embedding
	// because no store is registered for its storage instance.
	contentUnresolved prometheus.Counter
	// truncated counts entities embedded from only the opening of their text, labelled by who
	// cut it: the provider, to fit the model's input window, or graph-embedding's text cap.
	truncated *prometheus.CounterVec
}

// newEmbeddingMetrics builds the collectors and registers them on registry, the injected
// component.Dependencies.MetricsRegistry, through metric.RegisterOrGet, keeping the collector
// each call returns. With a nil registry they are built and registered nowhere.
func newEmbeddingMetrics(registry *metric.MetricsRegistry) (*embeddingMetrics, error) {
	m := &embeddingMetrics{
		contentUnresolved: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "semengine", Subsystem: metricSubsystem, Name: "content_unresolved_total",
			Help: "Entities whose offloaded body was excluded from embeddings because no store is registered for its storage instance",
		}),
		truncated: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "semengine", Subsystem: metricSubsystem, Name: "truncated_total",
			Help: "Entities embedded from only the opening of their text, by who cut it: provider (the model's input window) or text_cap (max_text_len)",
		}, []string{"by"}),
	}
	if registry != nil {
		var err error
		if m.contentUnresolved, err = metric.RegisterOrGet(registry, key, "content_unresolved_total", m.contentUnresolved); err != nil {
			return nil, err
		}
		if m.truncated, err = metric.RegisterOrGet(registry, key, "truncated_total", m.truncated); err != nil {
			return nil, err
		}
	}
	// Both series exist from the start, so a scrape reads 0 cuts rather than no answer.
	for _, by := range []string{embedding.TruncatedByProvider, embedding.TruncatedByTextCap} {
		m.truncated.WithLabelValues(by)
	}
	return m, nil
}

// embeddingMetrics is the Worker's metrics receiver (embedding.WorkerMetrics), so a body the
// Worker excludes, on a record whose store is gone since hop 1 wrote it (a restart without that
// storage component), is counted on the same collector as hop 1's exclusions, and a cut text on
// truncated. The Worker's other counts have no collector here yet; they are dropped, and the
// failed map carries its failures.
func (m *embeddingMetrics) IncContentUnresolved() { m.contentUnresolved.Inc() }

func (m *embeddingMetrics) IncTruncated(by string) { m.truncated.WithLabelValues(by).Inc() }

func (*embeddingMetrics) IncDedupHits()                 {}
func (*embeddingMetrics) IncDedupSkipped(string)        {}
func (*embeddingMetrics) IncOffloadedIdentityIncluded() {}
func (*embeddingMetrics) IncOffloadedIdentityAbsent()   {}
func (*embeddingMetrics) IncFailed()                    {}
func (*embeddingMetrics) IncFailedReason(string)        {}
func (*embeddingMetrics) IncContentResolveError()       {}
func (*embeddingMetrics) IncContentResolved()           {}
