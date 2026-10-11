package graphingest

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/pkg/errs"
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

// TestConfigRefusesOutputsGraphIngestDoesNotWrite: graph-ingest writes entity state to
// ENTITY_STATES only, a bucket initStorage opens by its fixed name, and publishes to no
// subject, stream, connection, file or store. An output port that names any other
// destination would be reported by DeclarePorts and OutputPorts while nothing is written
// there, so configuration validation refuses it as invalid, naming the port, whether it
// replaces the ENTITY_STATES output or sits beside it (checkpoint 3 H2, PR #93 comment
// 6099921473). Every kind an output port may have is listed; the one accepted is the
// default, a kv-write port on ENTITY_STATES.
func TestConfigRefusesOutputsGraphIngestDoesNotWrite(t *testing.T) {
	unwritten := map[string]component.Portable{
		"kv-write on another bucket": component.KVWritePort{Bucket: "OTHER"},
		"jetstream":                  component.JetStreamPort{StreamName: "GRAPH", Subjects: []string{"graph.entity.created"}},
		"nats":                       component.NATSPort{Subject: "graph.entity.events"},
		"nats-request":               component.NATSRequestPort{Subject: "graph.entity.ask"},
		"network":                    component.NetworkPort{Protocol: "tcp", Host: "example.test", Port: 443},
		"file":                       component.FilePort{Path: "entities.jsonl"},
		"store-provide":              component.StoreProvidePort{Instance: "entities"},
	}
	placements := map[string]func(config *Config, output component.PortDefinition){
		"in place of entity_states": func(config *Config, output component.PortDefinition) {
			config.Ports.Outputs = []component.PortDefinition{output}
		},
		"beside entity_states": func(config *Config, output component.PortDefinition) {
			config.Ports.Outputs = append(config.Ports.Outputs, output)
		},
	}
	for kind, portConfig := range unwritten {
		for placement, place := range placements {
			t.Run(kind+" "+placement, func(t *testing.T) {
				config := DefaultConfig()
				place(&config, component.PortDefinition{Name: "unwritten_output", Config: portConfig})
				raw, err := json.Marshal(config)
				if err != nil {
					t.Fatalf("marshal config: %v", err)
				}

				ports, err := DeclarePorts(raw, "graph-ingest")
				if err == nil {
					t.Fatalf("DeclarePorts accepted an output graph-ingest never writes: outputs %+v", ports.Outputs)
				}
				var classified *errs.ClassifiedError
				if !errors.As(err, &classified) || classified.Class != errs.ErrorInvalid {
					t.Fatalf("DeclarePorts error is not classified invalid: %v", err)
				}
				if !strings.Contains(err.Error(), `"unwritten_output"`) {
					t.Fatalf("DeclarePorts error does not name the refused port: %v", err)
				}
				if built, err := newUnstartedGraphIngest(t, nil, raw); err == nil {
					t.Fatalf("CreateGraphIngest accepted an output graph-ingest never writes: OutputPorts %+v", built.OutputPorts())
				}
				if err := config.Validate(); err == nil {
					t.Fatal("Validate accepted an output graph-ingest never writes")
				}
			})
		}
	}

	valid, err := json.Marshal(DefaultConfig())
	if err != nil {
		t.Fatalf("marshal default config: %v", err)
	}
	if _, err := DeclarePorts(valid, "graph-ingest"); err != nil {
		t.Fatalf("DeclarePorts refused the default kv-write output on ENTITY_STATES: %v", err)
	}
}
