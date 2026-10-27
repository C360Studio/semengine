package service

import (
	"log/slog"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/metric"
)

// The supervisor's tests new in this port (design D4): the health publisher counts the publish it
// could not make, and a failed Start launches no publisher.

// healthPublishErrors is the error count's labels for a health publish the component manager
// could not make.
var healthPublishErrors = map[string]string{"service": "component-manager", "type": "health_publish"}

// TestComponentHealthPublishFailureIsCounted: over a NATS client that is not connected, the health
// publisher's publish for each admitted component fails once when Start returns and again every 5
// seconds; each failure increments semengine_errors_total{service="component-manager",
// type="health_publish"} and logs at Debug naming the subject, the loop goes on, and nothing is
// published after Stop (design D4). The bubble's fake clock makes each count exact.
//
// spec: service-lifecycle / Component health is published and its drops counted
func TestComponentHealthPublishFailureIsCounted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		logs := &syncBuffer{}
		logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		metrics := metric.NewMetricsRegistry()
		cm := composeComponentManager(t, logger, metrics,
			testComponent{name: "publish-a", instance: &mockDiscoverableComponent{}},
			testComponent{name: "publish-b", instance: &mockDiscoverableComponent{}})

		before, _ := gaugeValue(t, metrics, "semengine_errors_total", healthPublishErrors)
		require.Zero(t, before, "construction published health")

		require.NoError(t, cm.Start(t.Context()))
		synctest.Wait()
		require.Equal(t, 2.0, requireGauge(t, metrics, "semengine_errors_total", healthPublishErrors),
			"the publish of each of the two components' health when Start returned was not counted")
		require.Contains(t, logs.String(), "health.component.publish-a", "the failed publish's log names its subject")
		require.Contains(t, logs.String(), "health.component.publish-b", "the failed publish's log names its subject")

		<-time.After(5 * time.Second)
		synctest.Wait()
		require.Equal(t, 4.0, requireGauge(t, metrics, "semengine_errors_total", healthPublishErrors),
			"the publisher did not go on after a failed publish: no second round 5 s after Start")

		require.NoError(t, cm.Stop(stopContext(t)))
		<-time.After(5 * time.Second)
		synctest.Wait()
		require.Equal(t, 4.0, requireGauge(t, metrics, "semengine_errors_total", healthPublishErrors),
			"the publisher published after Stop returned")
	})
}

// TestComponentManagerFailedStartLaunchesNoPublisher: a Start that fails, here because a
// component's Start fails, leaves no supervisor behind: no health was published, and the done
// channel is nil or closed (design D4). The pin failed Start on a struct-literal manager it had not
// initialized; the bubble would also fail the test if any goroutine the Start launched were left
// running.
//
// spec: service-lifecycle / Component health is published and its drops counted
func TestComponentManagerFailedStartLaunchesNoPublisher(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		metrics := metric.NewMetricsRegistry()
		failing := &suiteComponent{name: "supervisor-fails", startErr: errSuiteComponentStart}
		cm := composeComponentManager(t, discardLogger, metrics,
			testComponent{name: failing.name, instance: failing})

		require.ErrorContains(t, cm.Start(t.Context()), `component "supervisor-fails"`)
		synctest.Wait()

		done := cm.supervisorDone
		require.True(t, done == nil || isClosed(done), "a failed Start left a supervisor to join")
		published, _ := gaugeValue(t, metrics, "semengine_errors_total", healthPublishErrors)
		require.Zero(t, published, "a failed Start launched the health publisher anyway")
	})
}

// composeComponentManager builds a component manager through NewComponentManager, the
// production composition path, over a configuration that enables components, logging to logger
// and registering its metrics on metrics. Called inside a synctest bubble it builds everything in
// the bubble.
func composeComponentManager(
	t *testing.T, logger *slog.Logger, metrics *metric.MetricsRegistry, components ...testComponent,
) *ComponentManager {
	t.Helper()
	registry := component.NewRegistry()
	names := make([]string, 0, len(components))
	for _, c := range components {
		admitTestRegistryComponent(t, registry, c.name, c.instance)
		names = append(names, c.name)
	}
	deps := testComponentDependencies(t, registry, names...)
	deps.Logger = logger
	deps.MetricsRegistry = metrics

	built, err := NewComponentManager(nil, deps)
	require.NoError(t, err)
	cm, ok := built.(*ComponentManager)
	require.True(t, ok, "NewComponentManager returned %T, want *ComponentManager", built)
	return cm
}

// syncBuffer is a log sink the manager's goroutines write while the test reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
