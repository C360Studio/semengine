package readiness

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
)

// The ComputeIndexStatus tests are ported from pin:graph/index_status_test.go:17-205,
// keeping their oracles. The pin's Revision string field is not ported (#110 item 1):
// where the pin checked it, these tests check IndexedRevision and TargetRevision, the
// one spelling of the revision.

// TestComputeIndexStatus_Staleness pins the age-of-view projection (ADR-083 D3),
// including the presence encoding that keeps an absent staleness from reading as a
// perfectly fresh view in a bounded-staleness gate.
func TestComputeIndexStatus_Staleness(t *testing.T) {
	tests := []struct {
		name string
		in   IndexStatusInputs
		want uint64
	}{
		{
			name: "caught up reports zero staleness",
			in: IndexStatusInputs{
				Indexed: 100, Target: 100,
				IndexedAt: computeBase.Add(-90 * time.Second), Now: computeBase,
			},
			want: 0, // Ready: there is no staleness to report, however old the floor is
		},
		{
			name: "building reports the age of the covered floor",
			in: IndexStatusInputs{
				Indexed: 40, Target: 100,
				IndexedAt: computeBase.Add(-1500 * time.Millisecond), Now: computeBase,
			},
			want: 1500,
		},
		{
			name: "unknown floor commit time stays absent, never zero-as-fresh",
			in: IndexStatusInputs{
				Indexed: 40, Target: 100,
				IndexedAt: time.Time{}, Now: computeBase,
			},
			want: 0,
		},
		{
			name: "sub-millisecond age is clamped to the 1ms presence floor",
			in: IndexStatusInputs{
				Indexed: 40, Target: 100,
				IndexedAt: computeBase.Add(-200 * time.Microsecond), Now: computeBase,
			},
			want: 1,
		},
		{
			name: "a floor committed in the future (clock skew) still reports 1ms, not absent",
			in: IndexStatusInputs{
				Indexed: 40, Target: 100,
				IndexedAt: computeBase.Add(2 * time.Second), Now: computeBase,
			},
			want: 1,
		},
		{
			name: "degraded still carries the age of what it has covered",
			in: IndexStatusInputs{
				Indexed: 40, Target: 100, Stuck: true,
				IndexedAt: computeBase.Add(-45 * time.Second), Now: computeBase,
			},
			want: 45000,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ComputeIndexStatus(tt.in).StalenessMs; got != tt.want {
				t.Errorf("StalenessMs = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestComputeIndexStatus_PreExistingFieldsUnchanged pins Ready, State, Lag and the two
// revisions for each input shape the pin's graph-index produced.
func TestComputeIndexStatus_PreExistingFieldsUnchanged(t *testing.T) {
	tests := []struct {
		name        string
		in          IndexStatusInputs
		wantReady   bool
		wantState   graph.IndexState
		wantLag     uint64
		wantIndexed uint64
		wantTarget  uint64
	}{
		{
			name:      "empty graph is not ready",
			in:        IndexStatusInputs{Indexed: 0, Target: 0},
			wantState: graph.IndexStateBuilding,
		},
		{
			name:        "caught up is ready",
			in:          IndexStatusInputs{Indexed: 100, Target: 100},
			wantReady:   true,
			wantState:   graph.IndexStateReady,
			wantIndexed: 100,
			wantTarget:  100,
		},
		{
			name:        "behind target is building with lag",
			in:          IndexStatusInputs{Indexed: 40, Target: 100},
			wantState:   graph.IndexStateBuilding,
			wantLag:     60,
			wantIndexed: 40,
			wantTarget:  100,
		},
		{
			name:        "stuck and behind is degraded",
			in:          IndexStatusInputs{Indexed: 40, Target: 100, Stuck: true},
			wantState:   graph.IndexStateDegraded,
			wantLag:     60,
			wantIndexed: 40,
			wantTarget:  100,
		},
		{
			name:        "ready wins over stuck",
			in:          IndexStatusInputs{Indexed: 100, Target: 100, Stuck: true},
			wantReady:   true,
			wantState:   graph.IndexStateReady,
			wantIndexed: 100,
			wantTarget:  100,
		},
		{
			// Not a pin case: the spec's "never below zero" for a floor above the
			// target, which the watermark does not produce but the projection must not
			// wrap around on.
			name:        "floor above target has no lag",
			in:          IndexStatusInputs{Indexed: 120, Target: 100},
			wantReady:   true,
			wantState:   graph.IndexStateReady,
			wantIndexed: 120,
			wantTarget:  100,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeIndexStatus(tt.in)
			if got.Ready != tt.wantReady || got.State != tt.wantState || got.Lag != tt.wantLag ||
				got.IndexedRevision != tt.wantIndexed || got.TargetRevision != tt.wantTarget {
				t.Errorf("got Ready=%v State=%q Lag=%d Indexed=%d Target=%d; want %v/%q/%d/%d/%d",
					got.Ready, got.State, got.Lag, got.IndexedRevision, got.TargetRevision,
					tt.wantReady, tt.wantState, tt.wantLag, tt.wantIndexed, tt.wantTarget)
			}
		})
	}
}

// TestComputeIndexStatus_FailedCountDegrades: a failed count above zero projects to
// degraded before "ready wins", so a producer caught up over failures reports degraded
// while Ready stays coverage-accurate (ADR-085). With no failures the failure-detail
// fields stay off the wire.
func TestComputeIndexStatus_FailedCountDegrades(t *testing.T) {
	t.Run("caught up with failures is degraded, Ready still true", func(t *testing.T) {
		got := ComputeIndexStatus(IndexStatusInputs{Indexed: 100, Target: 100, FailedCount: 3})
		if got.State != graph.IndexStateDegraded {
			t.Errorf("State = %q, want %q: a failed count must win over the ready branch", got.State, graph.IndexStateDegraded)
		}
		if !got.Ready {
			t.Error("Ready = false, want true: coverage is complete, only health is degraded")
		}
		if got.FailedCount != 3 {
			t.Errorf("FailedCount = %d, want 3: the input must echo to the envelope", got.FailedCount)
		}
	})

	t.Run("caught up over one failure is degraded", func(t *testing.T) {
		// The spec's scenario, "Caught up over failures".
		got := ComputeIndexStatus(IndexStatusInputs{Indexed: 7, Target: 7, FailedCount: 1})
		if !got.Ready || got.State != graph.IndexStateDegraded {
			t.Errorf("got Ready=%v State=%q, want true/%q", got.Ready, got.State, graph.IndexStateDegraded)
		}
	})

	t.Run("failed with small lag is degraded, not building", func(t *testing.T) {
		got := ComputeIndexStatus(IndexStatusInputs{Indexed: 99, Target: 100, FailedCount: 1})
		if got.State != graph.IndexStateDegraded {
			t.Errorf("State = %q, want %q: a failed entry defers regardless of lag", got.State, graph.IndexStateDegraded)
		}
		if got.Ready {
			t.Error("Ready = true, want false: not caught up")
		}
	})

	t.Run("no failures leaves the failure fields off the wire", func(t *testing.T) {
		for _, in := range []IndexStatusInputs{
			{Indexed: 0, Target: 0},
			{Indexed: 100, Target: 100},
			{Indexed: 40, Target: 100},
			{Indexed: 40, Target: 100, Stuck: true},
			{Indexed: 100, Target: 100, Stuck: true},
		} {
			raw, err := json.Marshal(ComputeIndexStatus(in))
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			for _, key := range []string{"failed_count", "failed_reasons", "first_failure_at"} {
				if _, present := decoded[key]; present {
					t.Errorf("%s present in %s: it must be omitted when there are no failures", key, raw)
				}
			}
		}
	})
}
