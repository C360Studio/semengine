package fusion_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/fusion"
	"github.com/c360studio/semengine/storage"
)

// Tests for construction, metrics and the nil context, and for the declared skip
// (one Warn line per facet per call, and the skipped-read counter).

func discardLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

// counterValue reads one labelled series of a counter family from gatherer, or
// 0 when the family or series is absent.
func counterValue(t *testing.T, gatherer prometheus.Gatherer, family, label, value string) float64 {
	t.Helper()
	mfs, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != family {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == label && l.GetValue() == value {
					return m.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

func TestNewEngineRefusesNilClient(t *testing.T) {
	eng, err := fusion.NewEngine(nil, nil, discardLogger())
	if err == nil || eng != nil {
		t.Fatalf("NewEngine(nil client) = (%v, %v), want no engine and an error", eng, err)
	}
}

func TestNewEngineRefusesNilLogger(t *testing.T) {
	eng, err := fusion.NewEngine(&fakeGraph{}, nil, nil)
	if err == nil || eng != nil {
		t.Fatalf("NewEngine(nil logger) = (%v, %v), want no engine and an error", eng, err)
	}
}

// TestRegistrationRefusalReturned: a registry that already holds a different
// collector under either counter's name refuses the registration, and NewEngine
// returns that refusal and no engine.
func TestRegistrationRefusalReturned(t *testing.T) {
	for _, name := range []string{"body_hydration_failures_total", "skipped_reads_total"} {
		t.Run(name, func(t *testing.T) {
			reg := metric.NewMetricsRegistry()
			squatter := prometheus.NewCounter(prometheus.CounterOpts{Name: "squatter_" + name})
			if _, err := metric.RegisterOrGet(reg, "fusion", name, squatter); err != nil {
				t.Fatalf("pre-register: %v", err)
			}
			eng, err := fusion.NewEngine(&fakeGraph{}, nil, discardLogger(), fusion.WithMetrics(reg))
			if err == nil || eng != nil {
				t.Fatalf("NewEngine = (%v, %v), want the refusal and no engine", eng, err)
			}
		})
	}
}

// bodyFailGraph is one seed whose lens handle points at a store that reports the
// object gone, so every Fuse fails one body with not_found.
func bodyFailGraph() (*fakeGraph, *fusion.BodyResolver) {
	ent := withBodyEntity("acme.ops.code.repo.symbol.Gone", "Gone")
	g := &fakeGraph{seeds: map[string][]string{"q": {ent.ID}}, entities: map[string]*fusion.Entity{ent.ID: ent}}
	body := fusion.NewBodyResolver(fusion.MapStoreResolver{"objectstore": &fakeStore{getErr: storage.ErrObjectNotFound}})
	return g, body
}

// TestCounterOnlyOnInjectedRegistry: an engine built without a registry still
// declares a failed body on the node, and registers nothing in Prometheus'
// default registry; one built with a registry counts there and only there.
func TestCounterOnlyOnInjectedRegistry(t *testing.T) {
	req := fusion.Request{Query: "q", Want: []fusion.Want{fusion.WantBody}}
	hasFusionSeries := func(t *testing.T) bool {
		mfs, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			t.Fatalf("Gather: %v", err)
		}
		for _, mf := range mfs {
			if strings.Contains(mf.GetName(), "fusion") {
				return true
			}
		}
		return false
	}

	g, body := bodyFailGraph()
	resp, err := mustEngine(t, g, body).Fuse(t.Context(), req, refLens{})
	if err != nil {
		t.Fatalf("Fuse: %v", err)
	}
	if resp.Nodes[0].BodyReason != fusion.BodyNotFound {
		t.Errorf("body_reason = %q, want not_found", resp.Nodes[0].BodyReason)
	}
	if hasFusionSeries(t) {
		t.Error("an engine without a registry registered a fusion series in the default registry")
	}

	reg := metric.NewMetricsRegistry()
	g, body = bodyFailGraph()
	if _, err := mustEngine(t, g, body, fusion.WithMetrics(reg)).Fuse(t.Context(), req, refLens{}); err != nil {
		t.Fatalf("Fuse: %v", err)
	}
	if got := bodyFailureCount(t, reg, "not_found"); got != 1 {
		t.Errorf("injected registry body failures = %v, want 1", got)
	}
	if hasFusionSeries(t) {
		t.Error("an engine with a registry registered a fusion series in the default registry")
	}
}

// TestEnginesShareOneSeries: two engines on one registry each fail one body; the
// one series reads 2.
func TestEnginesShareOneSeries(t *testing.T) {
	reg := metric.NewMetricsRegistry()
	req := fusion.Request{Query: "q", Want: []fusion.Want{fusion.WantBody}}
	for range 2 {
		g, body := bodyFailGraph()
		if _, err := mustEngine(t, g, body, fusion.WithMetrics(reg)).Fuse(t.Context(), req, refLens{}); err != nil {
			t.Fatalf("Fuse: %v", err)
		}
	}
	if got := bodyFailureCount(t, reg, "not_found"); got != 2 {
		t.Errorf("body failures = %v, want 2 from two engines on one registry", got)
	}
}

// TestSkippedReadLoggedAndCounted: three relation-target reads fail in one Fuse
// call. One Warn line names facet relations, count 3 and the first error (targets
// are read in sorted order, so a's), no other facet logs, and the relations series
// reads 3.
func TestSkippedReadLoggedAndCounted(t *testing.T) {
	g := targetsGraph([]string{"a", "b", "c"}, nil)
	g.entityErr = map[string]error{
		"a": errors.New("read a failed"), "b": errors.New("read b failed"), "c": errors.New("read c failed"),
	}
	var logs bytes.Buffer
	reg := metric.NewMetricsRegistry()
	eng, err := fusion.NewEngine(g, nil, slog.New(slog.NewJSONHandler(&logs, nil)), fusion.WithMetrics(reg))
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	resp, err := eng.Fuse(t.Context(), fusion.Request{Query: "q", Want: []fusion.Want{fusion.WantRelations}}, refLens{})
	if err != nil {
		t.Fatalf("Fuse: %v", err)
	}
	if !resp.Nodes[0].RelationsTruncated {
		t.Error("relations_truncated must be set")
	}

	type record struct {
		Level      string `json:"level"`
		Facet      string `json:"facet"`
		Skipped    int    `json:"skipped"`
		FirstError string `json:"first_error"`
	}
	var warns []record
	for line := range strings.SplitSeq(strings.TrimSpace(logs.String()), "\n") {
		if line == "" {
			continue // no log output at all
		}
		var r record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("log line %q: %v", line, err)
		}
		if r.Level == "WARN" {
			warns = append(warns, r)
		}
	}
	want := []record{{Level: "WARN", Facet: "relations", Skipped: 3, FirstError: "read a failed"}}
	if len(warns) != 1 || warns[0] != want[0] {
		t.Errorf("warn lines = %+v, want %+v", warns, want)
	}
	if got := counterValue(t, reg.PrometheusRegistry(), "semengine_fusion_skipped_reads_total", "facet", "relations"); got != 3 {
		t.Errorf("skipped reads series = %v, want 3", got)
	}
}

// TestBodyFailureLogged: each body-failure path writes exactly one Warn line
// naming the node's handle, the bounded reason and the error; a hydrated body and
// a body-less entity write none.
func TestBodyFailureLogged(t *testing.T) {
	const id = "acme.ops.code.repo.symbol.Doc"
	req := fusion.Request{Query: "q", Want: []fusion.Want{fusion.WantBody}}
	resolver := func(s *fakeStore) *fusion.BodyResolver {
		return fusion.NewBodyResolver(fusion.MapStoreResolver{"objectstore": s})
	}
	cases := []struct {
		name    string
		ent     *fusion.Entity
		body    *fusion.BodyResolver
		lens    fusion.Lens
		reason  string // "" means no Warn line at all
		errText string // text the logged error must contain
	}{
		{"absent object", withBodyEntity(id, "Doc"), resolver(&fakeStore{getErr: storage.ErrObjectNotFound}),
			refLens{}, "not_found", storage.ErrObjectNotFound.Error()},
		{"store read fault", withBodyEntity(id, "Doc"), resolver(&fakeStore{getErr: errors.New("backend down")}),
			refLens{}, "error", "backend down"},
		{"hydrate fault", withBodyEntity(id, "Doc"), resolver(&fakeStore{}),
			errHydrateLens{}, "error", "hydrate backend fault"},
		{"no body resolver", withBodyEntity(id, "Doc"), nil,
			refLens{}, "error", "no body resolver configured"},
		{"hydrated body", withBodyEntity(id, "Doc"), resolver(&fakeStore{data: map[string][]byte{"k/Doc": []byte("x")}}),
			refLens{}, "", ""},
		{"body-less entity", entity(id, "Doc", "h/Doc.go"), resolver(&fakeStore{}),
			refLens{}, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &fakeGraph{seeds: map[string][]string{"q": {id}}, entities: map[string]*fusion.Entity{id: tc.ent}}
			var logs bytes.Buffer
			eng, err := fusion.NewEngine(g, tc.body, slog.New(slog.NewJSONHandler(&logs, nil)))
			if err != nil {
				t.Fatalf("NewEngine: %v", err)
			}
			if _, err := eng.Fuse(t.Context(), req, tc.lens); err != nil {
				t.Fatalf("Fuse: %v", err)
			}

			type record struct {
				Level  string `json:"level"`
				Msg    string `json:"msg"`
				Handle string `json:"handle"`
				Reason string `json:"reason"`
				Error  string `json:"error"`
			}
			var warns []record
			for line := range strings.SplitSeq(strings.TrimSpace(logs.String()), "\n") {
				if line == "" {
					continue // no log output at all
				}
				var r record
				if err := json.Unmarshal([]byte(line), &r); err != nil {
					t.Fatalf("log line %q: %v", line, err)
				}
				if r.Level == "WARN" {
					warns = append(warns, r)
				}
			}
			if tc.reason == "" {
				if len(warns) != 0 {
					t.Errorf("warn lines = %+v, want none", warns)
				}
				return
			}
			const msg = "fusion could not load a node body; the node ships without it"
			if len(warns) != 1 || warns[0].Msg != msg || warns[0].Handle != id || warns[0].Reason != tc.reason ||
				!strings.Contains(warns[0].Error, tc.errText) {
				t.Errorf("warn lines = %+v, want one {msg %q handle %q reason %q error containing %q}",
					warns, msg, id, tc.reason, tc.errText)
			}
		})
	}
}

// TestFuseRefusesNilContext: a nil context is refused before any read.
func TestFuseRefusesNilContext(t *testing.T) {
	g := &fakeGraph{}
	var nilCtx context.Context
	_, err := mustEngine(t, g, nil).Fuse(nilCtx, fusion.Request{Query: "q"}, refLens{})
	if err == nil {
		t.Fatal("Fuse(nil ctx) returned no error")
	}
	if g.lastResolve.Query != "" {
		t.Errorf("Resolve was called with %+v", g.lastResolve)
	}
}
