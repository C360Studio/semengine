package component

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPortDefinitionExternalRoundTrip(t *testing.T) {
	var definition PortDefinition
	if err := json.Unmarshal([]byte(`{"name":"in","required":true,"external":true,"config":{"kind":"jetstream","stream_name":"USER","subjects":["user.message.>"]}}`), &definition); err != nil {
		t.Fatal(err)
	}
	if !definition.External {
		t.Fatal("decoded definition lost external")
	}
	port, err := definition.Resolve(DirectionInput)
	if err != nil {
		t.Fatal(err)
	}
	if !port.External {
		t.Fatal("resolved port lost external")
	}
	encoded, err := json.Marshal(port)
	if err != nil {
		t.Fatal(err)
	}
	var runtime Port
	if err := json.Unmarshal(encoded, &runtime); err != nil {
		t.Fatal(err)
	}
	if !runtime.External || runtime.Name != "in" || !runtime.Required {
		t.Fatalf("runtime view = %+v, want external required input", runtime)
	}
	if err := json.Unmarshal([]byte(`{"name":"in","externl":true,"config":{"kind":"nats","subject":"x"}}`), &definition); err == nil {
		t.Fatal("strict envelope accepted an unknown field")
	}
	// external is an input statement: an output declaring it is refused, never
	// silently ignored.
	_, err = (PortDefinition{Name: "out", External: true, Config: NATSPort{Subject: "x"}}).Resolve(DirectionOutput)
	if err == nil || !strings.Contains(err.Error(), "external") {
		t.Fatalf("output port with external=true resolved without an error naming the field: %v", err)
	}
	if _, err := (PortDefinition{Name: "in", External: true, Config: NATSPort{Subject: "x"}}).Resolve(DirectionInput); err != nil {
		t.Fatalf("input port with external=true failed to resolve: %v", err)
	}
}
