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
// alone, a view that never opened, or a watch lost as it ended is a clean end, and a view's Run
// that joined a failure to stop its watch to the cancellation (graphview.View.Run, kvwatch.Stop) is
// a failure that keeps both errors.
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
// subject, which subscribeQueries returns from Start; a verb with one gets that handler.
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

// TestEveryDeclaredVerbHasAHandler: every row the table declares for graph-query has a handler, with
// embedding_enabled on or off, so a row added without one is caught here as well as at Start.
func TestEveryDeclaredVerbHasAHandler(t *testing.T) {
	for _, raw := range []string{`{}`, `{"embedding_enabled":false}`} {
		c, err := createFromJSON(t, raw)
		if err != nil {
			t.Fatalf("CreateGraphQuery(%s): %v", raw, err)
		}
		handlers := c.queryHandlers()
		for _, verb := range graph.QueryVerbs() {
			if verb.Responder != graph.ResponderGraphQuery {
				continue
			}
			if _, err := handlerFor(verb, handlers); err != nil {
				t.Errorf("%s: %v", raw, err)
			}
		}
	}
}

// TestEmbeddingVerbsRefuseWhenNotEnabled: with embedding_enabled false, semantic and similar refuse
// every request invalid not_enabled, naming the verb, whatever its bytes; each counts as one input
// and no error. Left out, the same calls forward: the client is never connected, so the forward's
// own transient internal refusal names the downstream subject (graph-query spec, "Embedding verbs
// can be turned off").
func TestEmbeddingVerbsRefuseWhenNotEnabled(t *testing.T) {
	downstream := map[string]string{"semantic": "search", "similar": "similar"}
	requests := []string{`{"query":"widget"}`, `{"entity_id":"c360.test.a.b.c.d"}`, `not json`, ``}

	off, err := createFromJSON(t, `{"embedding_enabled":false}`)
	if err != nil {
		t.Fatalf("CreateGraphQuery: %v", err)
	}
	handlers := off.queryHandlers()
	var messages, bytes int64
	for verb := range downstream {
		for _, request := range requests {
			reply, err := handlers[verb](t.Context(), []byte(request))
			if err == nil {
				t.Fatalf("%s(%q) with embedding_enabled false answered %q", verb, request, reply)
			}
			var refusal *errs.ClassifiedError
			if !errors.As(err, &refusal) || refusal.Class != errs.ErrorInvalid || refusal.Code != "not_enabled" {
				t.Errorf("%s(%q) refused %v, want invalid not_enabled", verb, request, err)
			}
			if !strings.Contains(err.Error(), verb+" is not enabled") {
				t.Errorf("%s(%q) refusal %q does not name the verb", verb, request, err)
			}
			messages++
			bytes += int64(len(request))
		}
	}
	// Each handler counts before it returns.
	if flow := off.DataFlow(); flow.Messages != messages || flow.Bytes != bytes {
		t.Errorf("DataFlow = %+v, want %d messages of %d bytes", flow, messages, bytes)
	}
	if got := off.Health().ErrorCount; got != 0 {
		t.Errorf("ErrorCount = %d, want 0: not_enabled is not graph-query's error", got)
	}

	on, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatalf("CreateGraphQuery: %v", err)
	}
	handlers = on.queryHandlers()
	for verb, name := range downstream {
		var subject string
		for _, row := range graph.QueryVerbs() {
			if row.Responder == graph.ResponderGraphEmbedding && row.Name == name {
				subject = row.Subject
			}
		}
		reply, err := handlers[verb](t.Context(), []byte(requests[0]))
		if err == nil {
			t.Fatalf("%s with no connection answered %q", verb, reply)
		}
		var refusal *errs.ClassifiedError
		if !errors.As(err, &refusal) || refusal.Class != errs.ErrorTransient || refusal.Code != graph.ErrorCodeInternal {
			t.Errorf("%s left on refused %v, want transient internal", verb, err)
		}
		if subject == "" || !strings.Contains(err.Error(), subject) {
			t.Errorf("%s left on: refusal %q does not name the downstream subject %q", verb, err, subject)
		}
	}
}
