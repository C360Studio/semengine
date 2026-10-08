//go:build integration

package graphingest

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// The pin ran these tests on one NATS client shared through TestMain
// (natsclient.NewSharedTestClient), terminated with no deadline. natsfixture.Open
// takes the test (design D3), so each test here opens a broker of its own, which its
// cleanup stops under a bounded context. The pin's TestMain also registered four test
// predicates for merge_entity_integration_test.go, which a later stage ports, and
// checked the owner-child request first; TestGraphIngestOwnerChildFixture checks that
// request itself.

// createTestComponentForLifecycle creates a test instance for lifecycle testing.
func createTestComponentForLifecycle(t *testing.T) *Component {
	t.Helper()
	config := DefaultConfig()
	deps := component.Dependencies{
		NATSClient:      newFixtureClient(t, entityStream),
		PayloadRegistry: newTestPayloadRegistry(t),
		Platform:        component.PlatformMeta{Org: testDeploymentOrg, Platform: testDeploymentPlatform},
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	comp, err := CreateGraphIngest(configJSON, deps)
	if err != nil {
		t.Fatalf("create component: %v", err)
	}

	return comp.(*Component)
}

func TestGraphIngest_OneShotLifecycleAgainstNATS(t *testing.T) {
	comp := createTestComponentForLifecycle(t)
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := comp.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := comp.Stop(t.Context()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := comp.Stop(t.Context()); err != nil {
		t.Fatalf("repeated completed Stop: %v", err)
	}
	if err := comp.Start(t.Context()); !errors.Is(err, errs.ErrAlreadyStarted) {
		t.Fatalf("Start after terminal Stop error = %v, want ErrAlreadyStarted", err)
	}
}

type graphIngestTrackingConsumeContext struct {
	jetstream.ConsumeContext
	drains atomic.Int32
}

func (c *graphIngestTrackingConsumeContext) Drain() {
	c.drains.Add(1)
	c.ConsumeContext.Drain()
}

type graphIngestObservedContext struct {
	context.Context
	doneOnce sync.Once
	doneSeen chan struct{}
}

func (c *graphIngestObservedContext) Done() <-chan struct{} {
	c.doneOnce.Do(func() { close(c.doneSeen) })
	return c.Context.Done()
}

func TestGraphIngest_FailedStartRollbackOwnsExactConsumerAndPreservesDurable(t *testing.T) {
	config := DefaultConfig()
	mutation := config.Ports.Inputs[1]
	config.Ports.Inputs = []component.PortDefinition{
		{Name: "entity_one", Config: component.JetStreamPort{StreamName: "ENTITY", Subjects: []string{"entity.one"}, DeliverPolicy: "all"}},
		{Name: "entity_two", Config: component.JetStreamPort{StreamName: "ENTITY", Subjects: []string{"entity.two"}, DeliverPolicy: "all"}},
		mutation,
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	created, err := CreateGraphIngest(configJSON, testDependencies(t, newFixtureClient(t, entityStream)))
	if err != nil {
		t.Fatalf("CreateGraphIngest: %v", err)
	}
	comp := created.(*Component)
	if err := comp.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	secondEntered := make(chan struct{})
	releaseSecond := make(chan struct{})
	sentinel := errors.New("second consumer acquisition failed")
	var calls atomic.Int32
	var first *graphIngestTrackingConsumeContext
	comp.consumeStream = func(
		ctx context.Context,
		owner natsclient.PortConsumerContext,
		cfg natsclient.StreamConsumerConfig,
		handler func(context.Context, jetstream.Msg),
	) (jetstream.ConsumeContext, error) {
		if calls.Add(1) == 1 {
			handle, consumeErr := comp.natsClient.ConsumeStreamWithConfig(ctx, owner, cfg, handler)
			if consumeErr != nil {
				return nil, consumeErr
			}
			first = &graphIngestTrackingConsumeContext{ConsumeContext: handle}
			return first, nil
		}
		close(secondEntered)
		<-releaseSecond
		return nil, sentinel
	}

	startResult := make(chan error, 1)
	go func() { startResult <- comp.Start(t.Context()) }()
	<-secondEntered
	// The pin's lifecycleMu, now handlesMu: it guards the component's handles, not the
	// guard's state, and stays (design D22).
	comp.handlesMu.Lock()
	if len(comp.consumers) != 1 || comp.consumers[0].handle != first {
		comp.handlesMu.Unlock()
		t.Fatal("first exact handle was not published before second acquisition")
	}
	comp.handlesMu.Unlock()

	stopCtx := &graphIngestObservedContext{Context: t.Context(), doneSeen: make(chan struct{})}
	stopResult := make(chan error, 1)
	go func() { stopResult <- comp.Stop(stopCtx) }()
	<-stopCtx.doneSeen
	select {
	case stopErr := <-stopResult:
		t.Fatalf("Stop returned before Start finalized: %v", stopErr)
	default:
	}
	close(releaseSecond)
	if startErr := <-startResult; !errors.Is(startErr, sentinel) {
		t.Fatalf("Start error = %v, want sentinel", startErr)
	}
	if stopErr := <-stopResult; stopErr != nil {
		t.Fatalf("overlapping Stop: %v", stopErr)
	}
	if first == nil || first.drains.Load() != 1 {
		t.Fatalf("first native Drain calls = %v, want 1", first)
	}

	consumerName := "graph-ingest-" + strings.ReplaceAll("entity.one", ".", "-")
	if _, observeErr := comp.observeOutstandingWork(t.Context(), "ENTITY", consumerName); observeErr != nil {
		t.Fatalf("durable consumer was deleted during failed-Start rollback: %v", observeErr)
	}
}
