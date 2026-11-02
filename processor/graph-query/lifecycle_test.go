package graphquery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
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
