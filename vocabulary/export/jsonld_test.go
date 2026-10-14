package export

import (
	"bytes"
	"encoding/json"
	"strings"
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

// expandJSONLDTerm implements the W3C JSON-LD 1.1 Create Term Definition
// (step 14.2.5) and IRI Expansion (§5.2.2) rules as an independent test oracle:
// a simple string term definition only sets _prefix=true when its IRI ends with
// an RFC 3986 gen-delim (':', '/', '?', '#', '[', ']', '@'). An expanded term
// definition map sets _prefix=true only when "@prefix": true is present.
func expandJSONLDTerm(ctx map[string]any, term string) string {
	type termDef struct {
		iri    string
		prefix bool
	}
	defs := make(map[string]termDef, len(ctx))
	for k, raw := range ctx {
		switch v := raw.(type) {
		case string:
			isPrefix := len(v) > 0 && strings.ContainsRune(":/?#[]@", rune(v[len(v)-1]))
			defs[k] = termDef{iri: v, prefix: isPrefix}
		case map[string]any:
			iri, _ := v["@id"].(string)
			isPrefix, _ := v["@prefix"].(bool)
			defs[k] = termDef{iri: iri, prefix: isPrefix}
		}
	}
	if def, ok := defs[term]; ok && def.iri != "" {
		return def.iri
	}
	colon := strings.IndexByte(term, ':')
	if colon <= 0 {
		return term
	}
	prefix := term[:colon]
	suffix := term[colon+1:]
	if strings.HasPrefix(suffix, "//") {
		return term
	}
	if def, ok := defs[prefix]; ok && def.prefix {
		return def.iri + suffix
	}
	return term
}

func TestWriteJSONLD_BFOPredicatePreservesExpandedIRI(t *testing.T) {
	defer vocabulary.SnapshotRegistry()()
	vocabulary.ClearRegistry()

	const bfoNS = "http://purl.obolibrary.org/obo/BFO_"
	const bfoPartOfIRI = "http://purl.obolibrary.org/obo/BFO_0000050"

	// Sensitivity check on the oracle: a bare string context definition ending
	// in '_' leaves _prefix=false and fails to expand "bfo:0000050".
	brokenExpanded := expandJSONLDTerm(map[string]any{"bfo": bfoNS}, "bfo:0000050")
	if brokenExpanded == bfoPartOfIRI {
		t.Fatalf("oracle sensitivity failure: bare string context unexpectedly expanded bfo:0000050 to %q", brokenExpanded)
	}

	if err := Register("bfo", bfoNS); err != nil {
		t.Fatalf("Register bfo: %v", err)
	}
	t.Cleanup(func() {
		defaultPrefixesMu.Lock()
		delete(defaultPrefixes, "bfo")
		defaultPrefixesMu.Unlock()
	})

	vocabulary.Register("review.bfo.part-of", vocabulary.WithIRI(bfoPartOfIRI))
	triples := []message.Triple{
		{Subject: "a.b.c.d.e.f", Predicate: "review.bfo.part-of", Object: "value"},
	}

	jsonldOut, err := SerializeToString(triples, JSONLD)
	if err != nil {
		t.Fatalf("SerializeToString(JSONLD): %v", err)
	}
	ntOut, err := SerializeToString(triples, NTriples)
	if err != nil {
		t.Fatalf("SerializeToString(NTriples): %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(jsonldOut), &doc); err != nil {
		t.Fatalf("json.Unmarshal: %v\n%s", err, jsonldOut)
	}
	ctx, ok := doc["@context"].(map[string]any)
	if !ok {
		t.Fatalf("expected @context map in JSON-LD output:\n%s", jsonldOut)
	}

	var propKey string
	for k := range doc {
		if k != "@context" && k != "@id" && k != "@graph" {
			propKey = k
			break
		}
	}
	if propKey == "" {
		t.Fatalf("missing property key in JSON-LD output:\n%s", jsonldOut)
	}

	expandedPredIRI := expandJSONLDTerm(ctx, propKey)
	if expandedPredIRI != bfoPartOfIRI {
		t.Fatalf("expanded JSON-LD predicate IRI = %q, want %q\nJSON-LD:\n%s", expandedPredIRI, bfoPartOfIRI, jsonldOut)
	}
	if !strings.Contains(ntOut, "<"+expandedPredIRI+">") {
		t.Fatalf("N-Triples output %q does not contain expanded JSON-LD predicate IRI <%s>", ntOut, expandedPredIRI)
	}
}
