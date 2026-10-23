package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"

	"github.com/c360studio/semengine/internal/lifecycleguard"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// metricsForwarderServiceName is the name RegisterAll gives the metrics forwarder.
const metricsForwarderServiceName = "metrics-forwarder"

// defaultPushInterval is the forwarder's push interval when the configuration gives none.
const defaultPushInterval = 5 * time.Second

// MetricsForwarderConfig holds configuration for the MetricsForwarder service.
type MetricsForwarderConfig struct {
	// PushInterval between publishing rounds (e.g. "5s", "1m"). Default: "5s".
	PushInterval string `json:"push_interval"`

	// IncludeGoMetrics forwards the go_* runtime families (goroutines, memory, GC). Default: false.
	IncludeGoMetrics bool `json:"include_go_metrics"`

	// IncludeProcMetrics forwards the process_* families (CPU, open FDs, memory). Default: false.
	IncludeProcMetrics bool `json:"include_proc_metrics"`
}

// MetricsForwarder gathers the composition's metrics registry when it starts and then once per
// push interval, and publishes each sample as one JSON object on metrics.<component>.<name>
// through the JetStream publish of the NATS client it was built with. A sample that does not
// encode or does not publish, and a gather that fails, are counted on semengine_errors_total and
// logged at Debug; the loop goes on.
type MetricsForwarder struct {
	*BaseService

	interval           time.Duration
	includeGoMetrics   bool
	includeProcMetrics bool
	client             *natsclient.Client
	registry           *metric.MetricsRegistry
	logger             *slog.Logger

	// lifecycle admits one Start and makes Stop the only stop (ADR-113).
	lifecycle lifecycleguard.Guard

	// runCancel and loopDone are written once, by start, before Start returns, and read by
	// cleanup. The guard runs cleanup only after start has returned, so they need no lock.
	// runCancel ends the run context the loop runs under.
	runCancel context.CancelFunc
	// loopDone is closed once the loop has returned; it is nil when no loop was started.
	loopDone chan struct{}
}

// NewMetricsForwarderService builds the metrics forwarder from its JSON configuration
// (MetricsForwarderConfig). A key the configuration does not define, or one in another letter
// case, is refused, naming the key. It refuses, as invalid data and naming the input, nil
// dependencies, a nil NATS client, a nil metrics registry, and a push_interval that does not parse
// or is not positive; an empty or absent push_interval takes 5s. It does no I/O.
func NewMetricsForwarderService(rawConfig json.RawMessage, deps *Dependencies) (Service, error) {
	if deps == nil {
		return nil, refuseMetricsForwarder(errors.New("metrics-forwarder requires dependencies"), "check dependencies")
	}
	if deps.NATSClient == nil {
		return nil, refuseMetricsForwarder(errors.New("metrics-forwarder requires NATS client"), "check dependencies")
	}
	if deps.MetricsRegistry == nil {
		return nil, refuseMetricsForwarder(errors.New("metrics-forwarder requires metrics registry"), "check dependencies")
	}
	var cfg MetricsForwarderConfig
	if err := decodeStrictServiceJSON(rawConfig, &cfg); err != nil {
		return nil, refuseMetricsForwarder(err, "parse metrics-forwarder config")
	}
	interval, err := serviceDuration(metricsForwarderServiceName, "push_interval", cfg.PushInterval, defaultPushInterval)
	if err != nil {
		return nil, refuseMetricsForwarder(err, "validate metrics-forwarder config")
	}

	logger := slog.Default().With("service", metricsForwarderServiceName)
	if deps.Logger != nil {
		logger = deps.Logger.With("service", metricsForwarderServiceName)
	}

	return &MetricsForwarder{
		BaseService:        NewBaseServiceWithOptions(metricsForwarderServiceName, WithLogger(deps.Logger)),
		interval:           interval,
		includeGoMetrics:   cfg.IncludeGoMetrics,
		includeProcMetrics: cfg.IncludeProcMetrics,
		client:             deps.NATSClient,
		registry:           deps.MetricsRegistry,
		logger:             logger,
	}, nil
}

func refuseMetricsForwarder(err error, action string) error {
	return errs.WrapInvalid(err, "MetricsForwarder", "NewMetricsForwarderService", action)
}

// Start starts the base service, then the publish loop, and returns nil; it does no I/O. It
// refuses a nil or ended context, a second Start and a Start after any Stop
// (errs.ErrAlreadyStarted); a refused Start holds nothing. ctx bounds Start only; ending it after
// Start returned stops nothing.
func (mf *MetricsForwarder) Start(ctx context.Context) error {
	return mf.lifecycle.Start(ctx, mf.start, mf.cleanup)
}

// start is Start's work, run by the lifecycle guard; a failure is rolled back through cleanup.
func (mf *MetricsForwarder) start(ctx context.Context) error {
	if err := mf.BaseService.Start(ctx); err != nil {
		return err
	}

	// The run context: Start's values without its cancellation, ended only by Stop (ADR-113).
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	mf.runCancel = cancel
	done := make(chan struct{})
	mf.loopDone = done
	go func() {
		defer close(done)
		mf.run(runCtx)
	}()

	mf.logger.Info("metrics forwarder started", "push_interval", mf.interval)
	return nil
}

// run publishes a round at once, so a reader that joins between Start and the first tick does not
// find the stream empty, and then one per interval until ctx ends.
func (mf *MetricsForwarder) run(ctx context.Context) {
	ticker := time.NewTicker(mf.interval)
	defer ticker.Stop()
	mf.publish(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mf.publish(ctx)
		}
	}
}

// publish runs one round: it gathers the registry and publishes every sample. A failed gather is
// counted and skips the round; a sample that does not encode or does not publish is counted and
// the round goes on, since the next round publishes the series again.
func (mf *MetricsForwarder) publish(ctx context.Context) {
	families, err := mf.registry.PrometheusRegistry().Gather()
	if err != nil {
		mf.logger.Debug("failed to gather metrics", "error", err)
		mf.registry.CoreMetrics().RecordError(metricsForwarderServiceName, "metrics_gather")
		return
	}
	for _, sample := range metricSamples(families, mf.includeGoMetrics, mf.includeProcMetrics, time.Now()) {
		data, err := json.Marshal(sample)
		if err == nil {
			err = mf.client.PublishToStream(ctx, sample.Subject, data)
		}
		if err != nil {
			mf.logger.Debug("failed to publish metrics to NATS", "subject", sample.Subject, "error", err)
			mf.registry.CoreMetrics().RecordError(metricsForwarderServiceName, "metrics_publish")
		}
	}
}

// Stop ends the run context, waits for the loop to return and stops the base service. It returns
// nil only once both are done. If ctx ends first it returns that context's error and the next Stop
// waits again. A Stop after one that returned nil returns nil; a Stop while another runs is refused
// as transient.
func (mf *MetricsForwarder) Stop(ctx context.Context) error {
	return mf.lifecycle.Stop(ctx)
}

// cleanup is the one cleanup the lifecycle guard runs for a failed Start and for every Stop.
// Every step can run again, so a Stop after one whose context ended finishes the job.
func (mf *MetricsForwarder) cleanup(ctx context.Context) error {
	if mf.runCancel != nil {
		mf.runCancel()
	}
	if mf.loopDone != nil {
		select {
		case <-mf.loopDone:
		case <-ctx.Done():
			return fmt.Errorf("wait for metrics forwarder loop: %w", ctx.Err())
		}
	}
	if err := mf.BaseService.Stop(ctx); err != nil {
		return fmt.Errorf("stop metrics forwarder base service: %w", err)
	}
	mf.logger.Info("metrics forwarder stopped")
	return nil
}

// metricSample is one forwarded sample: the subject it is published on and the payload's six
// fields.
type metricSample struct {
	Subject   string            `json:"-"`
	Timestamp int64             `json:"timestamp"`
	Name      string            `json:"name"`
	Component string            `json:"component"`
	Type      string            `json:"type"`
	Value     float64           `json:"value"`
	Labels    map[string]string `json:"labels"`
}

// metricSamples maps gathered families to the samples the forwarder publishes, in family order and
// then metric order. A family with no name maps to nothing; a go_ family only with includeGo, a
// process_ family only with includeProc. Each sample's component is its component label, else
// system; its value is the counter, gauge or untyped value, or the sample count of a histogram or
// summary, else 0; its timestamp is now in Unix milliseconds.
func metricSamples(families []*dto.MetricFamily, includeGo, includeProc bool, now time.Time) []metricSample {
	var samples []metricSample
	for _, family := range families {
		if family == nil || family.Name == nil {
			continue
		}
		name := *family.Name
		if strings.HasPrefix(name, "go_") && !includeGo {
			continue
		}
		if strings.HasPrefix(name, "process_") && !includeProc {
			continue
		}
		metricType := metricTypeName(family.Type)
		for _, m := range family.Metric {
			// Never nil: a sample with no labels encodes as {}, not null.
			labels := make(map[string]string)
			for _, label := range m.Label {
				if label.Name != nil && label.Value != nil {
					labels[*label.Name] = *label.Value
				}
			}
			component := "system"
			if c, ok := labels["component"]; ok {
				component = c
			}
			samples = append(samples, metricSample{
				Subject:   "metrics." + component + "." + name,
				Timestamp: now.UnixMilli(),
				Name:      name,
				Component: component,
				Type:      metricType,
				Value:     metricValue(m),
				Labels:    labels,
			})
		}
	}
	return samples
}

// metricValue is the counter, gauge or untyped value, or the sample count of a histogram or
// summary; 0 when the metric carries none.
func metricValue(m *dto.Metric) float64 {
	switch {
	case m.Counter != nil && m.Counter.Value != nil:
		return *m.Counter.Value
	case m.Gauge != nil && m.Gauge.Value != nil:
		return *m.Gauge.Value
	case m.Histogram != nil && m.Histogram.SampleCount != nil:
		return float64(*m.Histogram.SampleCount)
	case m.Summary != nil && m.Summary.SampleCount != nil:
		return float64(*m.Summary.SampleCount)
	case m.Untyped != nil && m.Untyped.Value != nil:
		return *m.Untyped.Value
	default:
		return 0
	}
}

// metricTypeName is the family's type in lower case; untyped when absent or of any other kind.
func metricTypeName(metricType *dto.MetricType) string {
	if metricType == nil {
		return "untyped"
	}
	switch *metricType {
	case dto.MetricType_COUNTER:
		return "counter"
	case dto.MetricType_GAUGE:
		return "gauge"
	case dto.MetricType_HISTOGRAM:
		return "histogram"
	case dto.MetricType_SUMMARY:
		return "summary"
	default:
		return "untyped"
	}
}
