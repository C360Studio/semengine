package graphquery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/kvwatch"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/graphview"
)

var (
	_ component.Discoverable       = (*Component)(nil)
	_ component.LifecycleComponent = (*Component)(nil)
)

// TestLifecycleBeforeStart: before Start the component is stopped and unhealthy with nothing
// counted, Initialize may be called again, Start before Initialize and Start without a NATS client
// are refused, and Stop before Start returns nil.
func TestLifecycleBeforeStart(t *testing.T) {
	newComponent := func(t *testing.T) *Component {
		t.Helper()
		c, err := createFromJSON(t, `{}`)
		if err != nil {
			t.Fatalf("CreateGraphQuery: %v", err)
		}
		return c
	}

	t.Run("health and data flow before Start", func(t *testing.T) {
		c := newComponent(t)
		health := c.Health()
		if health.Healthy || health.Status != "stopped" || health.ErrorCount != 0 {
			t.Errorf("Health before Start = %+v, want stopped, not healthy, no errors", health)
		}
		if flow := c.DataFlow(); flow != (component.FlowMetrics{}) {
			t.Errorf("DataFlow before Start = %+v, want zero", flow)
		}
	})

	t.Run("Initialize twice", func(t *testing.T) {
		c := newComponent(t)
		if err := c.Initialize(); err != nil {
			t.Fatalf("first Initialize: %v", err)
		}
		if err := c.Initialize(); err != nil {
			t.Errorf("second Initialize: %v", err)
		}
	})

	t.Run("Start before Initialize", func(t *testing.T) {
		if err := newComponent(t).Start(t.Context()); err == nil {
			t.Error("Start before Initialize returned nil")
		}
	})

	t.Run("Start without a NATS client", func(t *testing.T) {
		c := &Component{}
		if err := c.Initialize(); err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		if err := c.Start(t.Context()); !errors.Is(err, errs.ErrNoConnection) {
			t.Errorf("Start without a client = %v, want ErrNoConnection", err)
		}
	})

	t.Run("Stop before Start", func(t *testing.T) {
		if err := newComponent(t).Stop(t.Context()); err != nil {
			t.Errorf("Stop before Start: %v", err)
		}
	})
}

// TestShutdownFailureKeepsAWatchCleanupFailure: once the run context has ended, the cancellation
// alone, a view that never opened, or a watch lost as it ended is a clean end, and a view's Run that
// joined a failure to stop its watch to the cancellation (graphview.View.Run, kvwatch.Stop) is a
// failure that keeps both errors (#527).
func TestShutdownFailureKeepsAWatchCleanupFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	joined := errors.Join(ctx.Err(), fmt.Errorf("graphview: stop the watch: %w", kvwatch.ErrDeliveryOpen))

	if err := shutdownFailure(ctx, true, ctx.Err()); err != nil {
		t.Errorf("a view ended by the cancellation alone = %v, want nil", err)
	}
	if err := shutdownFailure(ctx, true, nil); err != nil {
		t.Errorf("a view that returned nothing = %v, want nil", err)
	}
	if err := shutdownFailure(ctx, false, fmt.Errorf("open: %w", ctx.Err())); err != nil {
		t.Errorf("an open refused by the cancellation = %v, want nil", err)
	}
	lost := fmt.Errorf("%w: %w", graphview.ErrWatcherLost, errors.New("updates closed"))
	if err := shutdownFailure(ctx, true, lost); err != nil {
		t.Errorf("a watch lost as the run context ended = %v, want nil", err)
	}
	lostOpen := errors.Join(lost, fmt.Errorf("graphview: stop the watch: %w", kvwatch.ErrDeliveryOpen))
	if err := shutdownFailure(ctx, true, lostOpen); !errors.Is(err, kvwatch.ErrDeliveryOpen) {
		t.Errorf("a lost watch whose delivery stayed open = %v, want kvwatch.ErrDeliveryOpen", err)
	}
	err := shutdownFailure(ctx, true, joined)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, kvwatch.ErrDeliveryOpen) {
		t.Errorf("a view that could not stop its watch = %v, want context.Canceled and kvwatch.ErrDeliveryOpen", err)
	}
}

// TestHandlerForNamesAnUnservedVerb: a verb with no handler is a failure naming the verb and its
// subject (design D7), which subscribeQueries returns from Start; a verb with one gets that handler.
func TestHandlerForNamesAnUnservedVerb(t *testing.T) {
	verb := graph.QueryVerb{Name: "unserved", Subject: "graph.query.unserved", Responder: graph.ResponderGraphQuery}

	_, err := handlerFor(verb, map[string]queryHandler{"other": nil})
	if err == nil {
		t.Fatal("a verb with no handler was given one")
	}
	for _, want := range []string{"unserved", "graph.query.unserved"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %s", err, want)
		}
	}

	handler, err := handlerFor(verb, map[string]queryHandler{
		"unserved": func(context.Context, []byte) ([]byte, error) { return []byte("this one"), nil },
	})
	if err != nil {
		t.Fatalf("a verb with a handler: %v", err)
	}
	if got, _ := handler(t.Context(), nil); string(got) != "this one" {
		t.Errorf("handlerFor returned another handler, which answered %q", got)
	}
}

// TestEveryDeclaredVerbHasAHandler: every row the table declares for graph-query has a handler, so
// a row added without one is caught here as well as at Start.
func TestEveryDeclaredVerbHasAHandler(t *testing.T) {
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphQuery: %v", err)
	}
	handlers := c.queryHandlers()
	for _, verb := range graph.QueryVerbs() {
		if verb.Responder != graph.ResponderGraphQuery {
			continue
		}
		if _, err := handlerFor(verb, handlers); err != nil {
			t.Error(err)
		}
	}
}
