package metric

import (
	stderrors "errors"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/c360studio/semengine/pkg/errs"
)

// RegisterOrGet registers candidate under the logical key serviceName.metricName,
// or returns the collector already registered under that key. There is one
// canonical collector per key: the caller must use the collector this returns,
// never its own candidate, which is not gathered when another collector already
// owns the key.
//
// Same key: the existing collector is returned only when it has the exact
// concrete type of candidate (a gauge satisfies prometheus.Counter, so the
// interface alone is not enough) and the same descriptors. A nil or typed-nil
// candidate, a type or descriptor mismatch, and a candidate whose descriptor
// another key, a core metric or a collector registered directly through
// PrometheusRegistry already owns, are refused with a fatal error; nothing is
// stored and the canonical collector is untouched.
func RegisterOrGet[C prometheus.Collector](r *MetricsRegistry, serviceName, metricName string, candidate C) (C, error) {
	var zero C
	r.mu.Lock()
	defer r.mu.Unlock()
	if isNilCollector(candidate) {
		return zero, errs.WrapFatal(fmt.Errorf("candidate collector is nil"),
			"MetricsRegistry", "RegisterOrGet", "invalid metric registration")
	}

	key := fmt.Sprintf("%s.%s", serviceName, metricName)
	if existingCollector, exists := r.registeredMetrics[key]; exists {
		existing, ok := existingCollector.(C)
		if !ok || reflect.TypeOf(existingCollector) != reflect.TypeOf(candidate) {
			return zero, errs.WrapFatal(
				fmt.Errorf("logical metric key %q is registered as %T, not %T", key, existingCollector, candidate),
				"MetricsRegistry", "RegisterOrGet", "registered collector is of another type")
		}
		if !sameCollectorDescriptors(existingCollector, candidate) {
			return zero, errs.WrapFatal(fmt.Errorf("logical metric key %q has a different descriptor", key),
				"MetricsRegistry", "RegisterOrGet", "incompatible metric registration")
		}
		return existing, nil
	}
	if err := r.prometheusRegistry.Register(candidate); err != nil {
		var alreadyRegistered prometheus.AlreadyRegisteredError
		if stderrors.As(err, &alreadyRegistered) {
			return zero, errs.WrapFatal(err, "MetricsRegistry", "RegisterOrGet",
				"collector descriptor is already owned by another registration")
		}
		return zero, errs.WrapFatal(err, "MetricsRegistry", "RegisterOrGet", "incompatible metric registration")
	}
	r.registeredMetrics[key] = candidate
	return candidate, nil
}

// isNilCollector reports a nil interface or a typed nil, which would panic in
// Describe.
func isNilCollector(c prometheus.Collector) bool {
	v := reflect.ValueOf(c)
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return v.IsNil()
	}
	return false
}

func sameCollectorDescriptors(left, right prometheus.Collector) bool {
	return reflect.DeepEqual(collectorDescriptors(left), collectorDescriptors(right))
}

func collectorDescriptors(collector prometheus.Collector) []string {
	if collector == nil {
		return nil
	}
	descriptors := make(chan *prometheus.Desc)
	values := make([]string, 0, 1)
	go func() {
		collector.Describe(descriptors)
		close(descriptors)
	}()
	for descriptor := range descriptors {
		values = append(values, descriptor.String())
	}
	sort.Strings(values)
	return values
}

// MetricsRegistry manages the registration and lifecycle of metrics
type MetricsRegistry struct {
	prometheusRegistry *prometheus.Registry
	Metrics            *Metrics
	registeredMetrics  map[string]prometheus.Collector
	mu                 sync.RWMutex
}

// NewMetricsRegistry creates a new metrics registry with core platform metrics
func NewMetricsRegistry() *MetricsRegistry {
	prometheusRegistry := prometheus.NewRegistry()

	registry := &MetricsRegistry{
		prometheusRegistry: prometheusRegistry,
		registeredMetrics:  make(map[string]prometheus.Collector),
	}

	// Initialize and register core metrics
	registry.Metrics = NewMetrics()
	registry.registerMetrics()

	// Add Go runtime metrics
	registry.prometheusRegistry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	return registry
}

// PrometheusRegistry returns the underlying Prometheus registry
func (r *MetricsRegistry) PrometheusRegistry() *prometheus.Registry {
	return r.prometheusRegistry
}

// CoreMetrics returns the core platform metrics
func (r *MetricsRegistry) CoreMetrics() *Metrics {
	return r.Metrics
}

// Unregister removes a metric from the registry
func (r *MetricsRegistry) Unregister(serviceName, metricName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s.%s", serviceName, metricName)

	collector, exists := r.registeredMetrics[key]
	if !exists {
		return false
	}

	success := r.prometheusRegistry.Unregister(collector)
	if success {
		delete(r.registeredMetrics, key)
	}

	return success
}

// register Metrics registers all core platform metrics
func (r *MetricsRegistry) registerMetrics() {
	r.prometheusRegistry.MustRegister(
		r.Metrics.ServiceStatus,
		r.Metrics.MessagesReceived,
		r.Metrics.MessagesProcessed,
		r.Metrics.MessagesPublished,
		r.Metrics.ProcessingDuration,
		r.Metrics.ErrorsTotal,
		r.Metrics.HealthCheckStatus,
		r.Metrics.LogEntriesTotal,
		r.Metrics.NATSConnected,
		r.Metrics.NATSRTT,
		r.Metrics.NATSReconnects,
		r.Metrics.NATSCircuitBreaker,
	)
}
