package export

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/vocabulary"
)

func TestWriteJSONLD_BasicTriple(t *testing.T) {
	triples := []message.Triple{
		{
			Subject:   "acme.ops.robotics.gcs.drone.001",
			Predicate: "robotics.battery.level",
			Object:    85.5,
		},
	}

	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeJSONLD(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, buf.String())
	}

	// Single triple should produce a flat document (no @graph)
	if _, ok := doc["@graph"]; ok {
		t.Errorf("single subject should not produce @graph, got:\n%s", buf.String())
	}
	if doc["@id"] != "https://semengine.semanticstream.ing/entities/acme/ops/robotics/gcs/drone/001" {
		t.Errorf("unexpected @id: %v", doc["@id"])
	}
}

func TestWriteJSONLD_MultipleSubjects(t *testing.T) {
	triples := []message.Triple{
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.battery.level", Object: 85.5},
		{Subject: "acme.ops.robotics.gcs.drone.002", Predicate: "robotics.battery.level", Object: 50.0},
	}

	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeJSONLD(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	graph, ok := doc["@graph"].([]any)
	if !ok {
		t.Fatalf("expected @graph array, got:\n%s", buf.String())
	}
	if len(graph) != 2 {
		t.Errorf("expected 2 nodes in @graph, got %d", len(graph))
	}
}

func TestWriteJSONLD_Context(t *testing.T) {
	// Register a predicate with a standard IRI to trigger prefix usage
	defer vocabulary.SnapshotRegistry()()
	vocabulary.ClearRegistry()

	vocabulary.Register("robotics.identity.same",
		vocabulary.WithIRI("http://www.w3.org/2002/07/owl#sameAs"))

	triples := []message.Triple{
		{
			Subject:   "acme.ops.robotics.gcs.drone.001",
			Predicate: "robotics.identity.same",
			Object:    "acme.ops.robotics.gcs.drone.002",
		},
	}

	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeJSONLD(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	ctx, ok := doc["@context"].(map[string]any)
	if !ok {
		t.Fatalf("expected @context object, got:\n%s", buf.String())
	}
	if ctx["owl"] != "http://www.w3.org/2002/07/owl#" {
		t.Errorf("expected owl namespace in context, got:\n%s", buf.String())
	}
}

func TestWriteJSONLD_EntityReference(t *testing.T) {
	triples := []message.Triple{
		{
			Subject:   "acme.ops.robotics.gcs.drone.001",
			Predicate: "graph.rel.contains",
			Object:    "acme.ops.robotics.gcs.battery.001",
		},
	}

	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeJSONLD(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Find the predicate key
	var predKey string
	for k := range doc {
		if k != "@id" && k != "@context" && k != "@graph" {
			predKey = k
			break
		}
	}

	objMap, ok := doc[predKey].(map[string]any)
	if !ok {
		t.Fatalf("expected object to be a map with @id, got %T: %v", doc[predKey], doc[predKey])
	}
	if !ok || objMap["@id"] == nil {
		t.Errorf("expected entity reference as {@id: ...}, got:\n%s", buf.String())
	}
}

func TestWriteJSONLD_BooleanNativeType(t *testing.T) {
	triples := []message.Triple{
		{
			Subject:   "acme.ops.robotics.gcs.drone.001",
			Predicate: "robotics.flight.armed",
			Object:    true,
		},
	}

	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeJSONLD(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Find the predicate value
	for k, v := range doc {
		if k == "@id" || k == "@context" || k == "@graph" {
			continue
		}
		if b, ok := v.(bool); !ok || b != true {
			t.Errorf("expected native boolean true for key %q, got %T: %v", k, v, v)
		}
	}
}

func TestWriteJSONLD_NumericNativeTypes(t *testing.T) {
	triples := []message.Triple{
		{
			Subject:   "acme.ops.robotics.gcs.drone.001",
			Predicate: "robotics.battery.level",
			Object:    85.5,
		},
	}

	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeJSONLD(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	for k, v := range doc {
		if k == "@id" || k == "@context" || k == "@graph" {
			continue
		}
		num, ok := v.(float64)
		if !ok {
			t.Errorf("expected numeric value for %q, got %T: %v", k, v, v)
			continue
		}
		if num != 85.5 {
			t.Errorf("expected 85.5, got %v", num)
		}
	}
}

func TestWriteJSONLD_DateTimeTypedValue(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	triples := []message.Triple{
		{
			Subject:   "acme.ops.robotics.gcs.drone.001",
			Predicate: "time.lifecycle.created",
			Object:    ts,
		},
	}

	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeJSONLD(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	for k, v := range doc {
		if k == "@id" || k == "@context" || k == "@graph" {
			continue
		}
		typed, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("expected typed value map for dateTime, got %T: %v", v, v)
		}
		if typed["@value"] != "2024-01-15T10:30:00Z" {
			t.Errorf("expected @value with dateTime string, got %v", typed["@value"])
		}
		if typed["@type"] != "xsd:dateTime" {
			t.Errorf("expected @type xsd:dateTime, got %v", typed["@type"])
		}
	}
}

func TestWriteJSONLD_EmptyInput(t *testing.T) {
	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeJSONLD(&buf, nil, &opts)
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Should produce an empty document
	if len(doc) != 0 {
		t.Errorf("expected empty document, got %d keys: %v", len(doc), doc)
	}
}

func TestWriteJSONLD_MultipleSamePredicate(t *testing.T) {
	triples := []message.Triple{
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.tag.label", Object: "alpha"},
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.tag.label", Object: "primary"},
	}

	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeJSONLD(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Find the predicate value
	for k, v := range doc {
		if k == "@id" || k == "@context" || k == "@graph" {
			continue
		}
		arr, ok := v.([]any)
		if !ok {
			t.Fatalf("expected array for repeated predicate %q, got %T: %v", k, v, v)
		}
		if len(arr) != 2 {
			t.Errorf("expected 2 values, got %d", len(arr))
		}
	}
}
