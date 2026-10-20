package composition_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/composition"
)

// TestCatalogCarriesDefaultPortsOrRequiresConfig — every catalog entry
// carries either default_ports (resolved inputs and outputs) or
// ports_require_config with the declarer's error text, never both and never
// neither. One factory declares ports for {} and one refuses {}.
func TestCatalogCarriesDefaultPortsOrRequiresConfig(t *testing.T) {
	registry := fakeRegistry(t,
		fakeSpec{name: "src", typ: "input", outputs: []component.PortDefinition{natsOut("out", "data.raw", nil)}},
		fakeSpec{name: "needs-config", typ: "processor", declareErr: errors.New("subject is required")},
	)
	entries := composition.Catalog(registry)
	if got, want := len(entries), len(registry.ListFactories()); got != want {
		t.Fatalf("catalog has %d entries, registry has %d factories", got, want)
	}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	withDefaults, requiring := 0, 0
	for _, entry := range decoded {
		id, _ := entry["id"].(string)
		_, hasDefaults := entry["default_ports"]
		requires, _ := entry["ports_require_config"].(bool)
		reason, _ := entry["ports_error"].(string)
		switch {
		case hasDefaults && !requires && reason == "":
			withDefaults++
		case !hasDefaults && requires && reason != "":
			requiring++
			if id != "needs-config" || !strings.Contains(reason, "subject is required") {
				t.Errorf("entry %s requires configuration with %q, want only needs-config with its refusal", id, reason)
			}
		default:
			t.Errorf("entry %s: default_ports=%v ports_require_config=%v ports_error=%q — want exactly one shape", id, hasDefaults, requires, reason)
		}
		for _, key := range []string{"id", "name", "type", "protocol", "domain", "description", "version", "category", "schema"} {
			if _, ok := entry[key]; !ok {
				t.Errorf("entry %s lacks %s", id, key)
			}
		}
	}
	if withDefaults != 1 || requiring != 1 {
		t.Fatalf("catalog: %d entries with default_ports, %d requiring configuration; want 1 and 1", withDefaults, requiring)
	}
}
