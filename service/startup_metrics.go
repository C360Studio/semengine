package service

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/errs"
)

// startupUnitCounts are the component startup counts the manager reports, in
// /readyz's startup snapshot and the semengine_startup_units gauge.
type startupUnitCounts struct {
	Admitted              int `json:"admitted"`
	LifecycleParticipants int `json:"lifecycle_participants"`
	StartsInvoked         int `json:"starts_invoked"`
	StartsCompleted       int `json:"starts_completed"`
	StartsFailed          int `json:"starts_failed"`
}

// serviceStartupCounts are the service startup counts the manager reports.
type serviceStartupCounts struct {
	Admitted        int `json:"admitted"`
	StartsInvoked   int `json:"starts_invoked"`
	StartsCompleted int `json:"starts_completed"`
	StartsFailed    int `json:"starts_failed"`
}

// startupUnitsDesc describes semengine_startup_units{owner,stage}, the gauge the
// startup collector reports.
var startupUnitsDesc = prometheus.NewDesc(
	"semengine_startup_units",
	"Process-local startup unit counts by manager owner and fixed lifecycle stage",
	[]string{"owner", "stage"}, nil,
)

// startupCollector reports the startup counts as they are when the registry is
// gathered. It holds no value of its own, so a collected count is never older
// than the counts at the start of that collection.
type startupCollector struct {
	serviceSnapshot   func() serviceStartupCounts
	componentSnapshot func() startupUnitCounts
}

// registerStartupCollector registers the startup collector directly on the
// registry. It refuses a registry that already holds a collector of that name
// rather than adopting another one's gauge, which metric.RegisterOrGet would do.
func registerStartupCollector(
	registry *metric.MetricsRegistry,
	serviceSnapshot func() serviceStartupCounts,
	componentSnapshot func() startupUnitCounts,
) error {
	if registry == nil {
		return errs.WrapFatal(
			fmt.Errorf("metrics registry is nil"),
			"startupCollector", "register", "startup metric registry unavailable",
		)
	}
	if serviceSnapshot == nil || componentSnapshot == nil {
		return errs.WrapFatal(
			fmt.Errorf("startup snapshot function is nil"),
			"startupCollector", "register", "startup metric observation unavailable",
		)
	}
	collector := &startupCollector{serviceSnapshot: serviceSnapshot, componentSnapshot: componentSnapshot}
	if err := registry.PrometheusRegistry().Register(collector); err != nil {
		return errs.WrapFatal(err, "startupCollector", "register", "register fresh startup metric collector")
	}
	return nil
}

// Describe implements prometheus.Collector.
func (c *startupCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- startupUnitsDesc
}

// Collect implements prometheus.Collector: nine series, four for services and
// five for components.
func (c *startupCollector) Collect(ch chan<- prometheus.Metric) {
	gauge := func(owner, stage string, value int) {
		ch <- prometheus.MustNewConstMetric(startupUnitsDesc, prometheus.GaugeValue, float64(value), owner, stage)
	}
	services := c.serviceSnapshot()
	gauge("services", "admitted", services.Admitted)
	gauge("services", "starts_invoked", services.StartsInvoked)
	gauge("services", "starts_completed", services.StartsCompleted)
	gauge("services", "starts_failed", services.StartsFailed)
	components := c.componentSnapshot()
	gauge("components", "admitted", components.Admitted)
	gauge("components", "lifecycle_participants", components.LifecycleParticipants)
	gauge("components", "starts_invoked", components.StartsInvoked)
	gauge("components", "starts_completed", components.StartsCompleted)
	gauge("components", "starts_failed", components.StartsFailed)
}
