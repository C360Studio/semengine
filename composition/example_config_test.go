package composition_test

import (
	"os"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/composition"
	"github.com/c360studio/semengine/config"
	graphingest "github.com/c360studio/semengine/processor/graph-ingest"
)

// TestValidateExampleConfigHasNoErrorFindings validates the example a developer
// copies first, config/example_config.json, against a catalog built by the
// Register function of each factory it names (owner ruling 4, #9). It loads
// the file through the production loader, so a key the loader or a factory
// refuses fails here too. LoadFile refuses a path outside the working
// directory, so the bytes go through LoadFromBytes, which merges them the
// same way.
func TestValidateExampleConfigHasNoErrorFindings(t *testing.T) {
	data, err := os.ReadFile("../config/example_config.json")
	if err != nil {
		t.Fatalf("read the example: %v", err)
	}
	cfg, err := config.NewLoader().LoadFromBytes(data)
	if err != nil {
		t.Fatalf("load the example: %v", err)
	}

	catalog := component.NewRegistry()
	if err := graphingest.Register(catalog); err != nil {
		t.Fatalf("register graph-ingest: %v", err)
	}

	result := validateRoundTrip(t, catalog, cfg)
	for _, finding := range result.Errors {
		t.Errorf("error finding: %s on %s (port %q): %s", finding.Type, finding.Component, finding.Port, finding.Message)
	}
	// Warnings are allowed: one component on its own has no peer to connect
	// to, so it is reported as disconnected and its ports as orphaned.
	if result.Status == composition.StatusErrors {
		t.Errorf("status = %q, want %q or %q", result.Status, composition.StatusValid, composition.StatusWarnings)
	}

	// An example with no enabled component would pass the checks above with
	// only an empty_composition warning, so require every enabled component
	// to reach the graph: a component refused before analysis never does.
	enabled := 0
	for _, entry := range cfg.Components {
		if entry.Enabled {
			enabled++
		}
	}
	if enabled == 0 {
		t.Fatal("the example enables no component")
	}
	if len(result.Graph.Nodes) != enabled {
		t.Errorf("graph has %d nodes, want one per enabled component (%d)", len(result.Graph.Nodes), enabled)
	}
	for _, finding := range result.Warnings {
		t.Logf("warning: %s on %s (port %q): %s", finding.Type, finding.Component, finding.Port, finding.Message)
	}
}
