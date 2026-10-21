package service

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/types"
)

// The component type catalog, from the pin's component_catalog_test.go. BuildComponentTypeCatalog
// is framework API (design D1): a consumer adapter builds its own catalog from it, so the tests
// call it as a product would. The pin's handler test built a struct-literal manager; here the
// manager is built through NewComponentManager over the catalog's registry (design D7).

// stubFactory is a factory no test calls: no catalog factory is named by a configuration.
func stubFactory(json.RawMessage, component.Dependencies) (component.Discoverable, error) {
	return nil, errors.New("stub: not meant to be called")
}

// registerCatalogFactory registers a factory with the catalog metadata given, failing the test on
// error.
func registerCatalogFactory(
	t *testing.T, reg *component.Registry, name string, compType types.ComponentType,
	protocol, domain, desc, version string,
) {
	t.Helper()
	err := reg.Register(component.Registration{
		Name:        name,
		Type:        compType,
		Protocol:    protocol,
		Domain:      domain,
		Description: desc,
		Version:     version,
		Factory:     stubFactory,
		Ports:       func(json.RawMessage, string) (component.PortConfig, error) { return component.PortConfig{}, nil },
	})
	if err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
}

// setupCatalogRegistry creates a registry with two fake factories for catalog tests.
func setupCatalogRegistry(t *testing.T) *component.Registry {
	t.Helper()
	reg := component.NewRegistry()
	registerCatalogFactory(t, reg, "alpha-input", types.ComponentTypeInput, "udp", "network",
		"Alpha UDP input for tests", "1.0.0")
	registerCatalogFactory(t, reg, "beta-processor", types.ComponentTypeProcessor, "internal", "semantic",
		"Beta semantic processor for tests", "2.0.0")
	return reg
}

// TestBuildComponentTypeCatalog_Shape verifies the builder produces entries
// with the expected keys and correct values for each factory.
func TestBuildComponentTypeCatalog_Shape(t *testing.T) {
	t.Parallel()

	reg := setupCatalogRegistry(t)
	logger := slog.Default()

	catalog := BuildComponentTypeCatalog(reg, logger)

	if len(catalog) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(catalog))
	}

	// Index by id, so the assertions do not depend on the catalog's order.
	byID := make(map[string]map[string]any, len(catalog))
	for _, entry := range catalog {
		id, _ := entry["id"].(string)
		byID[id] = entry
	}

	requiredKeys := []string{"id", "name", "type", "protocol", "domain", "description", "version", "category", "schema"}

	for _, id := range []string{"alpha-input", "beta-processor"} {
		entry, ok := byID[id]
		if !ok {
			t.Fatalf("missing catalog entry for %q", id)
		}
		for _, k := range requiredKeys {
			if _, present := entry[k]; !present {
				t.Errorf("entry %q missing key %q", id, k)
			}
		}
	}

	// Spot-check field values.
	alpha := byID["alpha-input"]
	if alpha["type"] != "input" {
		t.Errorf("alpha-input type: want 'input', got %v", alpha["type"])
	}
	if alpha["protocol"] != "udp" {
		t.Errorf("alpha-input protocol: want 'udp', got %v", alpha["protocol"])
	}
	if alpha["domain"] != "network" {
		t.Errorf("alpha-input domain: want 'network', got %v", alpha["domain"])
	}
	// category mirrors type per the builder contract.
	if alpha["category"] != alpha["type"] {
		t.Errorf("alpha-input category should equal type; got category=%v type=%v", alpha["category"], alpha["type"])
	}

	beta := byID["beta-processor"]
	if beta["type"] != "processor" {
		t.Errorf("beta-processor type: want 'processor', got %v", beta["type"])
	}
	if beta["version"] != "2.0.0" {
		t.Errorf("beta-processor version: want '2.0.0', got %v", beta["version"])
	}
}

// TestBuildComponentTypeCatalog_EmptyRegistry confirms an empty registry
// returns an empty (non-nil) slice, matching the handler's Encode behaviour.
func TestBuildComponentTypeCatalog_EmptyRegistry(t *testing.T) {
	t.Parallel()

	reg := component.NewRegistry()
	catalog := BuildComponentTypeCatalog(reg, slog.Default())

	if catalog == nil {
		t.Fatal("expected non-nil slice for empty registry")
	}
	if len(catalog) != 0 {
		t.Fatalf("expected 0 entries for empty registry, got %d", len(catalog))
	}
}

// TestHandleComponentTypes_DelegatesToBuilder confirms the HTTP handler encodes the same data
// BuildComponentTypeCatalog returns for the manager's registry, which lists every registered
// factory whether or not the configuration names it. Guards against the handler growing its own
// copy of the shape: the pin compared entry counts; here the whole answer is compared.
func TestHandleComponentTypes_DelegatesToBuilder(t *testing.T) {
	t.Parallel()

	reg := setupCatalogRegistry(t)
	cm := newProjectionComponentManager(t, reg)

	req := httptest.NewRequest(http.MethodGet, "/components/types", nil)
	w := httptest.NewRecorder()
	cm.handleComponentTypes(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var served []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &served); err != nil {
		t.Fatalf("response is not valid JSON array: %v", err)
	}

	if len(served) != 2 {
		t.Fatalf("expected 2 component types, got %d", len(served))
	}

	builderOutput, err := json.Marshal(BuildComponentTypeCatalog(reg, slog.Default()))
	require.NoError(t, err)
	require.JSONEq(t, string(builderOutput), w.Body.String(), "the handler's answer is not the builder's catalog")
}
