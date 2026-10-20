package config

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/c360studio/semengine/types"
	"github.com/stretchr/testify/require"
)

// newReconcileTestManager builds a NATS-free Manager for exercising the
// in-memory handleUpdate / notification path (no KV, no watcher goroutine).
func newReconcileTestManager() *Manager {
	cfg := &Config{
		Version:    "1.0.0",
		Platform:   PlatformConfig{Org: "c360", ID: "reconcile-test", Type: "test"},
		Services:   make(types.ServiceConfigs),
		Components: make(ComponentConfigs),
	}
	return &Manager{
		config:      NewSafeConfig(cfg),
		subscribers: make(map[string][]chan update),
		appliedRev:  make(map[string]uint64),
		logger:      slog.Default(),
	}
}

func componentJSON(t *testing.T, name string) []byte {
	t.Helper()
	data, err := json.Marshal(types.ComponentConfig{Type: "input", Name: name, Enabled: true})
	require.NoError(t, err)
	return data
}

// TestHandleUpdate_EngineOwnedRevisionStillNotifies is the gh#388 regression:
// an engine-owned revision (at or below the revision applied for its key) must skip the
// in-memory RE-APPLY but STILL notify subscribers, so a runtime add/remove
// drives a reconcile. Before the fix, handleUpdate returned before notifying.
func TestHandleUpdate_EngineOwnedRevisionStillNotifies(t *testing.T) {
	cm := newReconcileTestManager()
	cm.appliedRev["components.doc-source-003"] = 100

	ch := cm.onChange("components.*")
	<-ch // drain the initial-config send from OnChange

	// Engine-owned event: revision 50 <= the key's applied revision 100.
	cm.handleUpdate("components.doc-source-003", componentJSON(t, "doc-source-003"), 50)

	select {
	case up := <-ch:
		require.Equal(t, "components.doc-source-003", up.Path,
			"engine-owned revision must still notify subscribers (gh#388)")
	case <-time.After(time.Second):
		t.Fatal("engine-owned revision did not notify subscriber (gh#388)")
	}

	// The in-memory config must NOT be re-applied from the event — the engine
	// owns the apply, so the component is absent until the engine applies it.
	_, present := cm.config.Get().Components["doc-source-003"]
	require.False(t, present, "engine-owned event must not re-apply in memory")
}

// TestHandleUpdate_ExternalRevisionAppliesAndNotifies confirms the external
// path is unchanged: a revision above the key's applied revision applies in
// memory AND notifies.
func TestHandleUpdate_ExternalRevisionAppliesAndNotifies(t *testing.T) {
	cm := newReconcileTestManager()
	cm.appliedRev["components.ext-comp"] = 100

	ch := cm.onChange("components.*")
	<-ch

	// External event: revision 150 > the key's applied revision 100.
	cm.handleUpdate("components.ext-comp", componentJSON(t, "ext-comp"), 150)

	select {
	case up := <-ch:
		require.Equal(t, "components.ext-comp", up.Path)
	case <-time.After(time.Second):
		t.Fatal("external revision did not notify subscriber")
	}

	_, present := cm.config.Get().Components["ext-comp"]
	require.True(t, present, "external event must apply in memory")
}
