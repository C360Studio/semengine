package graph_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

const (
	subject = "c360.platform1.robotics.fleet.drone.001"
	predP   = "robotics.battery.level"
	predQ   = "robotics.battery.voltage"
)

var t0 = time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)

func stmt(pred, source string, at time.Time, object any) message.Triple {
	return message.Triple{Subject: subject, Predicate: pred, Object: object, Source: source, Timestamp: at,
		Confidence: 1}
}

// Requirement: graph-entity-writes/Timestamp orders a replace
func TestReplaceOrderedByTimestamp(t *testing.T) {
	stored := []message.Triple{stmt(predP, "A", t0, 10.0)}

	t.Run("an older set from the same source is not applied", func(t *testing.T) {
		got := graph.ReplaceBySource(stored, []message.Triple{
			stmt(predP, "A", t0.Add(-time.Second), 9.0),
			stmt(predQ, "A", t0.Add(-time.Second), 12.1),
		})
		want := []message.Triple{stmt(predP, "A", t0, 10.0), stmt(predQ, "A", t0.Add(-time.Second), 12.1)}
		requireSameStatements(t, got.Triples, want)
		if !reflect.DeepEqual(got.Stale, []graph.StaleSet{{Subject: subject, Predicate: predP, Source: "A"}}) {
			t.Fatalf("Stale = %+v, want the one (P, A) set", got.Stale)
		}
	})
	t.Run("another source's older set is stored beside it", func(t *testing.T) {
		got := graph.ReplaceBySource(stored, []message.Triple{stmt(predP, "B", t0.Add(-time.Hour), 7.0)})
		requireSameStatements(t, got.Triples, []message.Triple{stmt(predP, "A", t0, 10.0), stmt(predP, "B",
			t0.Add(-time.Hour), 7.0)})
		if len(got.Stale) != 0 {
			t.Fatalf("Stale = %+v, want none", got.Stale)
		}
	})
	t.Run("an equal timestamp applies", func(t *testing.T) {
		got := graph.ReplaceBySource(stored, []message.Triple{stmt(predP, "A", t0, 11.0)})
		requireSameStatements(t, got.Triples, []message.Triple{stmt(predP, "A", t0, 11.0)})
		if len(got.Stale) != 0 {
			t.Fatalf("Stale = %+v, want none", got.Stale)
		}
	})
	t.Run("a newer set replaces the source's whole set", func(t *testing.T) {
		two := []message.Triple{stmt(predP, "A", t0, 1.0), stmt(predP, "A", t0, 2.0), stmt(predP, "B", t0, 3.0)}
		got := graph.ReplaceBySource(two, []message.Triple{stmt(predP, "A", t0.Add(time.Second), 4.0)})
		requireSameStatements(t, got.Triples, []message.Triple{stmt(predP, "B", t0, 3.0),
			stmt(predP, "A", t0.Add(time.Second), 4.0)})
	})
}

// Requirement: graph-entity-writes/Statement metadata is required
func TestConfidenceAndContextNeverOrder(t *testing.T) {
	high := stmt(predP, "A", t0, 10.0)
	high.Confidence = 0.9
	high.Context = "ctx-high"
	newerLow := stmt(predP, "A", t0.Add(time.Minute), 11.0)
	newerLow.Confidence = 0.1

	got := graph.ReplaceBySource([]message.Triple{high}, []message.Triple{newerLow})
	requireSameStatements(t, got.Triples, []message.Triple{newerLow})

	olderHigh := stmt(predP, "A", t0.Add(-time.Minute), 12.0)
	olderHigh.Confidence = 1
	olderHigh.Context = "ctx-higher"
	got = graph.ReplaceBySource([]message.Triple{newerLow}, []message.Triple{olderHigh})
	requireSameStatements(t, got.Triples, []message.Triple{newerLow})
	if len(got.Stale) != 1 {
		t.Fatalf("Stale = %+v, want the older set counted once", got.Stale)
	}
}

// Requirement: graph-entity-writes/A single-value read picks one statement the same way every time
func TestSingleValueReadPicksLatestAcrossSources(t *testing.T) {
	older := stmt(predP, "B", t0, "from-b")
	later := stmt(predP, "A", t0.Add(time.Minute), "from-a")
	for _, order := range [][]message.Triple{{older, later}, {later, older}} {
		es := &graph.EntityState{ID: subject, Triples: order}
		if got := es.GetTriple(predP); got == nil || got.Source != "A" {
			t.Fatalf("GetTriple over %v = %+v, want source A's later statement", sources(order), got)
		}
		if v, ok := es.GetPropertyValue(predP); !ok || v != "from-a" {
			t.Fatalf("GetPropertyValue over %v = %v, %v; want from-a", sources(order), v, ok)
		}
	}

	tieB := stmt(predP, "B", t0, "tie-b")
	tieA := stmt(predP, "A", t0, "tie-a")
	es := &graph.EntityState{ID: subject, Triples: []message.Triple{tieB, tieA}}
	if got := es.GetTriple(predP); got == nil || got.Source != "A" {
		t.Fatalf("GetTriple on equal timestamps = %+v, want the source that sorts first (A)", got)
	}

	first := stmt(predP, "A", t0, "first")
	second := stmt(predP, "A", t0, "second")
	es = &graph.EntityState{ID: subject, Triples: []message.Triple{first, second}}
	if got := es.GetTriple(predP); got == nil || got.Object != "first" {
		t.Fatalf("GetTriple on equal timestamp and source = %+v, want the first stored", got)
	}
	if got := (&graph.EntityState{ID: subject}).GetTriple(predP); got != nil {
		t.Fatalf("GetTriple on an absent predicate = %+v, want nil", got)
	}
}

// Requirement: graph-entity-writes/Statement metadata is required
func TestIsReservedSourceNamesAll(t *testing.T) {
	for _, name := range []string{"graph-ingest-indexing-profile", "graph-ingest-hierarchy", "semengine-lifecycle"} {
		if !graph.IsReservedSource(name) {
			t.Errorf("IsReservedSource(%q) = false, want true", name)
		}
	}
	if graph.SourceIndexingProfile != "graph-ingest-indexing-profile" || graph.SourceHierarchy !=
		"graph-ingest-hierarchy" || graph.SourceLifecycle != "semengine-lifecycle" {
		t.Errorf("reserved source constants = %q, %q, %q", graph.SourceIndexingProfile, graph.SourceHierarchy,
			graph.SourceLifecycle)
	}
	for _, name := range []string{"", "graph-ingest", "semengine-lifecycle ", "Graph-Ingest-Hierarchy", "sensor"} {
		if graph.IsReservedSource(name) {
			t.Errorf("IsReservedSource(%q) = true, want false", name)
		}
	}
}

// Requirement: graph-entity-writes/The revision is the only fence
func TestStoredEntityHasNoVersion(t *testing.T) {
	es := graph.EntityState{ID: subject, Triples: []message.Triple{stmt(predP, "A", t0, 1.0)}, UpdatedAt: t0}
	raw, err := json.Marshal(es)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := keys["version"]; ok {
		t.Fatalf("stored JSON %s has a version key", raw)
	}

	legacy := `{"id":"` + subject + `","triples":[],"message_type":{},"version":7,"updated_at":"2026-10-07T12:00:00Z"}`
	var decoded graph.EntityState
	if err := json.Unmarshal([]byte(legacy), &decoded); err != nil {
		t.Fatalf("a value with a version key does not decode: %v", err)
	}
	if decoded.ID != subject {
		t.Fatalf("decoded ID = %q, want %q", decoded.ID, subject)
	}
}

// Requirement: graph-transport-boundary/The readiness envelope carries its publish time and no legacy fields
func TestIndexStatusResponseHasNoLegacyFields(t *testing.T) {
	typ := reflect.TypeOf(graph.IndexStatusResponse{})
	tags := map[string]bool{}
	for i := range typ.NumField() {
		tags[strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]] = true
	}
	for _, legacy := range []string{"phase", "revision", "last_synced"} {
		if tags[legacy] {
			t.Errorf("IndexStatusResponse carries the legacy field %q", legacy)
		}
	}
	if !tags["published_at"] || !tags["indexed_revision"] {
		t.Errorf("IndexStatusResponse fields %v lack published_at or indexed_revision", tags)
	}
}

func requireSameStatements(t *testing.T, got, want []message.Triple) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("statements = %+v, want %+v", got, want)
	}
	left := append([]message.Triple(nil), want...)
	for _, g := range got {
		found := -1
		for i, w := range left {
			if reflect.DeepEqual(g, w) {
				found = i
				break
			}
		}
		if found < 0 {
			t.Fatalf("statements = %+v, want %+v (unexpected %+v)", got, want, g)
		}
		left = append(left[:found], left[found+1:]...)
	}
}

func sources(ts []message.Triple) []string {
	out := make([]string, len(ts))
	for i, tr := range ts {
		out[i] = tr.Source
	}
	return out
}
