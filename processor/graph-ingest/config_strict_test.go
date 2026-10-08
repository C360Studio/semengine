package graphingest

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCreateGraphIngestRefusesUnknownKey: a configuration carrying a key graph-ingest
// does not define (the misspelled ingest_lane) is refused at construction, naming the
// key; the same configuration without it builds (design D6, Config (b);
// component-registration, "Misspelled key").
func TestCreateGraphIngestRefusesUnknownKey(t *testing.T) {
	valid, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	if _, err := newUnstartedGraphIngest(t, nil, valid); err != nil {
		t.Fatalf("CreateGraphIngest with the default config: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(valid, &fields); err != nil {
		t.Fatalf("unmarshal default config: %v", err)
	}
	fields["ingest_lane"] = 4
	misspelled, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal misspelled config: %v", err)
	}

	_, err = newUnstartedGraphIngest(t, nil, misspelled)
	if err == nil {
		t.Fatal("CreateGraphIngest accepted a config with the unknown key ingest_lane")
	}
	if !strings.Contains(err.Error(), `"ingest_lane"`) {
		t.Fatalf("CreateGraphIngest error does not name the unknown key ingest_lane: %v", err)
	}
}
