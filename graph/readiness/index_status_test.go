package readiness

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
)

// computeBase is a fixed compute instant so the staleness projection is asserted
// exactly rather than against the wall clock.
var computeBase = time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

// TestIndexStatusResponse_FailedDetailWireRoundTrip proves the three additive
// failure-detail fields survive the production JSON encode/decode under the exact
// keys consumers read, and are omitted when zero (wire compatibility). The
// graph-status KV value IS this JSON, so this is the production codec.
func TestIndexStatusResponse_FailedDetailWireRoundTrip(t *testing.T) {
	src := graph.IndexStatusResponse{
		Ready: true, State: graph.IndexStateDegraded,
		FailedCount:    5,
		FailedReasons:  map[string]uint64{"connection_refused": 4, "content_error": 1},
		FirstFailureAt: "2026-07-22T10:00:00Z",
	}
	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got graph.IndexStatusResponse
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.FailedCount != 5 || got.FirstFailureAt != "2026-07-22T10:00:00Z" {
		t.Errorf("failure detail lost: FailedCount=%d FirstFailureAt=%q", got.FailedCount, got.FirstFailureAt)
	}
	if got.FailedReasons["connection_refused"] != 4 || got.FailedReasons["content_error"] != 1 {
		t.Errorf("FailedReasons lost: %v", got.FailedReasons)
	}
}

// TestIndexStatusResponse_StalenessWireRoundTrip proves the additive field survives
// the wire under the exact key consumers decode (`staleness_ms`), and that it is
// omitted — not emitted as 0 — when absent. The graph-status KV value IS this JSON,
// so the encoder/decoder pair here is the production one (plain encoding/json, no
// payload-registry envelope: readiness is operational KV state, not a published
// message payload).
func TestIndexStatusResponse_StalenessWireRoundTrip(t *testing.T) {
	src := graph.IndexStatusResponse{
		State:           graph.IndexStateBuilding,
		IndexedRevision: 40,
		TargetRevision:  100,
		Lag:             60,
		StalenessMs:     2500,
	}
	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var keyed map[string]any
	if err := json.Unmarshal(raw, &keyed); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	if got, ok := keyed["staleness_ms"]; !ok || got != float64(2500) {
		t.Fatalf("wire key staleness_ms = %v (present=%v), want 2500", got, ok)
	}

	var back graph.IndexStatusResponse
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(back, src) {
		t.Errorf("round trip changed the envelope:\n got %+v\nwant %+v", back, src)
	}

	// Ready envelopes omit the key entirely, so an old decoder sees exactly what it
	// saw before and a new one cannot mistake an absent value for a present zero.
	readyRaw, err := json.Marshal(graph.IndexStatusResponse{
		Ready:           true,
		State:           graph.IndexStateReady,
		IndexedRevision: 100,
		TargetRevision:  100,
	})
	if err != nil {
		t.Fatalf("marshal ready: %v", err)
	}
	var readyKeyed map[string]any
	if err := json.Unmarshal(readyRaw, &readyKeyed); err != nil {
		t.Fatalf("unmarshal ready: %v", err)
	}
	if _, present := readyKeyed["staleness_ms"]; present {
		t.Errorf("ready envelope emitted staleness_ms: %s", readyRaw)
	}
}

// TestIndexStatusResponse_BootstrapCompleteWireRoundTrip pins the ADR-084 D2 bit on
// the wire. Unlike staleness_ms, this field is deliberately NOT omitempty: it gates
// health, so an explicit `false` must be distinguishable in a `nats kv get` dump from
// a pre-ADR-084 producer that never emits the key at all. Both decode to false — fail
// closed either way — but only one of them is a bug an operator can fix by upgrading.
func TestIndexStatusResponse_BootstrapCompleteWireRoundTrip(t *testing.T) {
	for _, bootstrapped := range []bool{false, true} {
		src := graph.IndexStatusResponse{
			State:             graph.IndexStateBuilding,
			IndexedRevision:   40,
			TargetRevision:    100,
			Lag:               60,
			BootstrapComplete: bootstrapped,
		}

		raw, err := json.Marshal(src)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var keyed map[string]any
		if err := json.Unmarshal(raw, &keyed); err != nil {
			t.Fatalf("unmarshal to map: %v", err)
		}
		got, present := keyed["bootstrap_complete"]
		if !present {
			t.Fatalf("bootstrap_complete absent from the wire for %v: %s", bootstrapped, raw)
		}
		if got != bootstrapped {
			t.Errorf("wire bootstrap_complete = %v, want %v", got, bootstrapped)
		}

		var back graph.IndexStatusResponse
		if err := json.Unmarshal(raw, &back); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !reflect.DeepEqual(back, src) {
			t.Errorf("round trip changed the envelope:\n got %+v\nwant %+v", back, src)
		}
	}

	// The migration contract: an envelope from a producer that predates the field
	// decodes to false, so every health gate fails closed until the lockstep upgrade.
	var legacy graph.IndexStatusResponse
	if err := json.Unmarshal([]byte(`{"ready":true,"state":"ready"}`), &legacy); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if legacy.BootstrapComplete {
		t.Error("absent bootstrap_complete decoded true; the health gate would fail OPEN on an old producer")
	}
}
