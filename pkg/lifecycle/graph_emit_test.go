package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

// TestManagerWithoutClientRefusesEmit: a manager built with no NATS client has no graph mutation
// client, and each of its three writes (create, reconcile, delete) is refused with ErrEmitFailed
// naming the missing client (design D17). The reads are given a fixture bucket, since
// NewManager(nil, ...) has none, so each call reaches its write.
func TestManagerWithoutClientRefusesEmit(t *testing.T) {
	t.Parallel()
	mgr := NewManager(nil, nil)
	if err := mgr.Register(lifecycle{}.fixtureWorkflow()); err != nil {
		t.Fatalf("Register: %v", err)
	}
	bucket := newFakeBucket()
	mgr.exactReader = bucketExactEntityReader{bucket: bucket}
	ctx := context.Background()

	const attached = "c360.platform1.gcs.lifecycle.mission.no-client-attach"
	bucket.put(attached, &graph.EntityState{ID: attached, Triples: []message.Triple{{
		Subject: attached, Predicate: "mission.identity.owner-org-id", Object: "acme", Source: "another-producer",
	}}})
	const reclaimed = "c360.platform1.gcs.lifecycle.mission.no-client-reclaim"
	bucket.put(reclaimed, &graph.EntityState{ID: reclaimed, Triples: []message.Triple{{
		Subject: reclaimed, Predicate: "mission.lifecycle.phase", Object: "completed", Source: graph.SourceLifecycle,
	}}})

	writes := []struct {
		name  string
		write func() error
	}{
		{"create", func() error {
			return mgr.Create(ctx, &fixtureMission{ID: "c360.platform1.gcs.lifecycle.mission.no-client-born", PhaseF: "planning"})
		}},
		{"reconcile", func() error {
			return mgr.Create(ctx, &fixtureMission{ID: attached, PhaseF: "planning"})
		}},
		{"delete", func() error { return mgr.Despawn(ctx, "fixture", reclaimed) }},
	}
	for _, w := range writes {
		err := w.write()
		if !errors.Is(err, ErrEmitFailed) {
			t.Errorf("%s: error = %v, want ErrEmitFailed", w.name, err)
			continue
		}
		if !strings.Contains(err.Error(), "no NATS client") {
			t.Errorf("%s: error %q does not name the missing NATS client", w.name, err)
		}
	}
}
