package graphingest

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCreateGraphIngestRefusesUnknownKey: a configuration carrying a key graph-ingest
// does not define is refused at construction, naming the key; the same configuration
// without it builds (design D6, Config (b); component-registration, "Misspelled key").
// The keys are a misspelling (ingest_lane) and a key graph-ingest no longer has
// (enable_type_siblings, dropped with sibling edges by ruling G, #91 comment 6062681355).
func TestCreateGraphIngestRefusesUnknownKey(t *testing.T) {
	valid, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	if _, err := newUnstartedGraphIngest(t, nil, valid); err != nil {
		t.Fatalf("CreateGraphIngest with the default config: %v", err)
	}

	for key, value := range map[string]any{"ingest_lane": 4, "enable_type_siblings": false} {
		t.Run(key, func(t *testing.T) {
			var fields map[string]any
			if err := json.Unmarshal(valid, &fields); err != nil {
				t.Fatalf("unmarshal default config: %v", err)
			}
			fields[key] = value
			config, err := json.Marshal(fields)
			if err != nil {
				t.Fatalf("marshal config with %s: %v", key, err)
			}

			_, err = newUnstartedGraphIngest(t, nil, config)
			if err == nil {
				t.Fatalf("CreateGraphIngest accepted a config with the unknown key %s", key)
			}
			if !strings.Contains(err.Error(), `"`+key+`"`) {
				t.Fatalf("CreateGraphIngest error does not name the unknown key %s: %v", key, err)
			}
		})
	}
}
