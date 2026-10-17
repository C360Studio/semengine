package graph

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

// TestIndexStatesReturnsACopy holds design D6: IndexStates lists the four wire
// states of the readiness envelope, and a caller that changes the slice it got
// back cannot change what the next caller gets. The readiness gate's allow-list
// and the one-hot state gauge both read this list.
func TestIndexStatesReturnsACopy(t *testing.T) {
	want := []IndexState{"building", "ready", "degraded", "reset_required"}

	first := IndexStates()
	if !slices.Equal(first, want) {
		t.Fatalf("IndexStates() = %q, want %q", first, want)
	}

	first[0] = "tampered"
	_ = append(first[:1], "appended")

	if second := IndexStates(); !slices.Equal(second, want) {
		t.Fatalf("IndexStates() after a caller changed its copy = %q, want %q", second, want)
	}
}

// TestIndexStatusResponseWire holds the readiness envelope's JSON, which is the
// GRAPH_STATUS value itself: plain encoding/json, with no payload-registry
// envelope, because readiness is KV state, not a published message.
//
// Each case decodes a wire value, checks the decoded envelope, encodes it again
// and checks which keys the encoding carries. A case that names an envelope
// instead of a wire value starts from that envelope's own encoding, so it also
// proves the envelope survives a round trip. Decoding a wire value first is what
// lets a case show that keys the type does not define (the pin's legacy fields)
// do not come back out.
//
// Requirement: graph-transport-boundary/The readiness envelope carries its publish time and no legacy fields
func TestIndexStatusResponseWire(t *testing.T) {
	tests := []struct {
		name     string
		envelope IndexStatusResponse // encoded to get the wire value when wire is empty
		wire     string              // decoded first when set
		want     IndexStatusResponse // the decoded envelope; the envelope itself when wire is empty
		keys     map[string]any      // keys the re-encoding carries, with their JSON values
		absent   []string            // keys the re-encoding leaves out
	}{
		{
			// A degraded producer's failure detail travels under the keys
			// operators and consumers read.
			name: "failure detail",
			envelope: IndexStatusResponse{
				Ready: true, State: IndexStateDegraded,
				FailedCount:    5,
				FailedReasons:  map[string]uint64{"connection_refused": 4, "content_error": 1},
				FirstFailureAt: "2026-07-22T10:00:00Z",
			},
			keys: map[string]any{
				"failed_count":     float64(5),
				"failed_reasons":   map[string]any{"connection_refused": float64(4), "content_error": float64(1)},
				"first_failure_at": "2026-07-22T10:00:00Z",
			},
		},
		{
			name: "no failure detail when there are no failures",
			envelope: IndexStatusResponse{
				Ready: true, State: IndexStateReady, IndexedRevision: 100, TargetRevision: 100,
			},
			absent: []string{"failed_count", "failed_reasons", "first_failure_at"},
		},
		{
			name: "staleness when computed",
			envelope: IndexStatusResponse{
				State: IndexStateBuilding, IndexedRevision: 40, TargetRevision: 100, Lag: 60, StalenessMs: 2500,
			},
			keys: map[string]any{"staleness_ms": float64(2500)},
		},
		{
			// Zero staleness means "no information", so it is left out rather
			// than sent as a 0 a reader could take for "0 ms behind".
			name: "no staleness on a ready envelope",
			envelope: IndexStatusResponse{
				Ready: true, State: IndexStateReady, IndexedRevision: 100, TargetRevision: 100,
			},
			absent: []string{"staleness_ms"},
		},
		{
			// bootstrap_complete is never left out: an explicit false tells an
			// operator reading the key that the producer is still bootstrapping,
			// where a missing key means a producer older than the field.
			name: "bootstrap_complete false is on the wire",
			envelope: IndexStatusResponse{
				State: IndexStateBuilding, IndexedRevision: 40, TargetRevision: 100, Lag: 60,
			},
			keys: map[string]any{"bootstrap_complete": false},
		},
		{
			name: "bootstrap_complete true is on the wire",
			envelope: IndexStatusResponse{
				State: IndexStateBuilding, IndexedRevision: 40, TargetRevision: 100, Lag: 60,
				BootstrapComplete: true,
			},
			keys: map[string]any{"bootstrap_complete": true},
		},
		{
			// A producer older than the field reads as not bootstrapped, so
			// every health gate fails closed on it.
			name: "missing bootstrap_complete decodes false",
			wire: `{"ready":true,"state":"ready"}`,
			want: IndexStatusResponse{Ready: true, State: IndexStateReady},
			keys: map[string]any{"bootstrap_complete": false},
		},
		{
			name:     "bootstrap_scope when set",
			envelope: IndexStatusResponse{State: IndexStateReady, BootstrapScope: 12345},
			keys:     map[string]any{"bootstrap_scope": float64(12345)},
		},
		{
			// Producers that do not report a scope keep it off their envelope.
			name:     "no bootstrap_scope at zero",
			envelope: IndexStatusResponse{State: IndexStateReady},
			absent:   []string{"bootstrap_scope"},
		},
		{
			// A complete bootstrap with no scope reads as "the initial build
			// was empty", the one distinction the field exists for.
			name: "complete bootstrap without a scope reads as an empty build",
			wire: `{"state":"ready","bootstrap_complete":true}`,
			want: IndexStatusResponse{State: IndexStateReady, BootstrapComplete: true},
		},
		{
			name: "legacy fields do not come back out",
			wire: `{"state":"ready","phase":"ready","revision":7,"last_synced":"2026-10-07T12:00:00Z",` +
				`"indexed_revision":7,"published_at":"2026-10-07T12:00:01Z"}`,
			want: IndexStatusResponse{
				State: IndexStateReady, IndexedRevision: 7, PublishedAt: "2026-10-07T12:00:01Z",
			},
			keys: map[string]any{
				"indexed_revision": float64(7),
				"published_at":     "2026-10-07T12:00:01Z",
			},
			absent: []string{"phase", "revision", "last_synced"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire, want := []byte(tt.wire), tt.want
			if tt.wire == "" {
				var err error
				if wire, err = json.Marshal(tt.envelope); err != nil {
					t.Fatalf("marshal: %v", err)
				}
				want = tt.envelope
			}

			var got IndexStatusResponse
			if err := json.Unmarshal(wire, &got); err != nil {
				t.Fatalf("unmarshal %s: %v", wire, err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("decoded %s as\n %+v\nwant %+v", wire, got, want)
			}

			again, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal again: %v", err)
			}
			var keyed map[string]any
			if err := json.Unmarshal(again, &keyed); err != nil {
				t.Fatalf("unmarshal to map: %v", err)
			}
			for key, value := range tt.keys {
				if v, ok := keyed[key]; !ok || !reflect.DeepEqual(v, value) {
					t.Errorf("%s: %s = %v (present %v), want %v", again, key, v, ok, value)
				}
			}
			for _, key := range tt.absent {
				if _, ok := keyed[key]; ok {
					t.Errorf("%s carries %s", again, key)
				}
			}
		})
	}
}
