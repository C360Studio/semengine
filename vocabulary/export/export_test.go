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

// sampleTriples returns a set of triples for integration testing.
func sampleTriples() []message.Triple {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	return []message.Triple{
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.battery.level", Object: 85.5},
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.flight.armed", Object: true},
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.status.mode", Object: "hovering"},
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.battery.cells", Object: 4},
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "time.lifecycle.created", Object: ts},
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "graph.rel.contains", Object: "acme.ops.robotics.gcs.battery.001"},
	}
}

func TestSerialize_FormatDispatch(t *testing.T) {
	triples := sampleTriples()

	tests := []struct {
		format Format
		check  func(t *testing.T, output string)
	}{
		{
			format: NTriples,
			check: func(t *testing.T, output string) {
				t.Helper()
				lines := strings.Split(strings.TrimSpace(output), "\n")
				if len(lines) != 6 {
					t.Errorf("N-Triples: expected 6 lines, got %d", len(lines))
				}
				for _, line := range lines {
					if !strings.HasSuffix(line, " .") {
						t.Errorf("N-Triples: line should end with ' .': %s", line)
					}
				}
			},
		},
		{
			format: Turtle,
			check: func(t *testing.T, output string) {
				t.Helper()
				if !strings.Contains(output, "@prefix") {
					t.Error("Turtle: expected prefix declarations")
				}
				if !strings.Contains(output, ";") {
					t.Error("Turtle: expected semicolons for subject grouping")
				}
			},
		},
		{
			format: JSONLD,
			check: func(t *testing.T, output string) {
				t.Helper()
				var doc map[string]any
				if err := json.Unmarshal([]byte(output), &doc); err != nil {
					t.Fatalf("JSON-LD: invalid JSON: %v", err)
				}
				if doc["@id"] == nil {
					t.Error("JSON-LD: expected @id in document")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.format.String(), func(t *testing.T) {
			output, err := SerializeToString(triples, tt.format)
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, output)
		})
	}
}

func TestSerialize_UnsupportedFormat(t *testing.T) {
	err := Serialize(&bytes.Buffer{}, nil, Format(99))
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSerialize_WithBaseIRI(t *testing.T) {
	triples := []message.Triple{
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.battery.level", Object: 85.5},
	}

	output, err := SerializeToString(triples, NTriples, WithBaseIRI("https://example.org"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output, "https://example.org/entities/") {
		t.Errorf("expected custom base in subject IRI, got:\n%s", output)
	}
	if !strings.Contains(output, "https://example.org/predicates/") {
		t.Errorf("expected custom base in predicate IRI, got:\n%s", output)
	}
}

func TestSerialize_WithSubjectIRIFunc(t *testing.T) {
	triples := []message.Triple{
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.battery.level", Object: 85.5},
	}

	customFn := func(subject string) string {
		return "https://custom.example.org/entity/" + subject
	}

	output, err := SerializeToString(triples, NTriples, WithSubjectIRIFunc(customFn))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output, "https://custom.example.org/entity/acme.ops.robotics.gcs.drone.001") {
		t.Errorf("expected custom subject IRI, got:\n%s", output)
	}
}

func TestSerialize_SkipsInvalidTriples(t *testing.T) {
	triples := []message.Triple{
		{Subject: "", Predicate: "robotics.battery.level", Object: 85.5},
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "", Object: 85.5}, // predicate-audit:invalid {"kind":"stored-predicate","value":"","reason":"empty"}
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.battery.level", Object: nil},
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.status.mode", Object: "ok"},
	}

	formats := []Format{Turtle, NTriples, JSONLD}
	for _, format := range formats {
		t.Run(format.String(), func(t *testing.T) {
			output, err := SerializeToString(triples, format)
			if err != nil {
				t.Fatal(err)
			}
			// Only the last valid triple should be serialized
			if output == "" {
				t.Error("expected at least one valid triple in output")
			}
		})
	}
}

func TestSerialize_NonEntitySubjectFallback(t *testing.T) {
	triples := []message.Triple{
		{Subject: "some.arbitrary.subject", Predicate: "robotics.battery.level", Object: 85.5},
	}

	output, err := SerializeToString(triples, NTriples)
	if err != nil {
		t.Fatal(err)
	}

	// Should use subjects/ path for non-entity subjects
	if !strings.Contains(output, "/subjects/some/arbitrary/subject>") {
		t.Errorf("expected fallback subject IRI, got:\n%s", output)
	}
}

func TestSerialize_RegisteredPredicateUsesStandardIRI(t *testing.T) {
	defer vocabulary.SnapshotRegistry()()
	vocabulary.ClearRegistry()

	vocabulary.Register("robotics.battery.level",
		vocabulary.WithIRI("https://schema.org/batteryLevel"))

	triples := []message.Triple{
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.battery.level", Object: 85.5},
	}

	output, err := SerializeToString(triples, NTriples)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(output, "<https://schema.org/batteryLevel>") {
		t.Errorf("expected standard IRI from registry, got:\n%s", output)
	}
}

func TestSerialize_FormatString(t *testing.T) {
	tests := []struct {
		format Format
		want   string
	}{
		{Turtle, "Turtle"},
		{NTriples, "N-Triples"},
		{JSONLD, "JSON-LD"},
		{Format(99), "Format(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.format.String(); got != tt.want {
				t.Errorf("Format.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSerializeToString_Roundtrip(t *testing.T) {
	triples := sampleTriples()

	// Each format should produce non-empty output
	formats := []Format{Turtle, NTriples, JSONLD}
	for _, format := range formats {
		t.Run(format.String(), func(t *testing.T) {
			output, err := SerializeToString(triples, format)
			if err != nil {
				t.Fatal(err)
			}
			if output == "" {
				t.Error("expected non-empty output")
			}
		})
	}
}

func TestSubjectToIRI_EntityID(t *testing.T) {
	base := "https://semengine.semanticstream.ing"
	got := subjectToIRI("acme.ops.robotics.gcs.drone.001", base)
	want := "https://semengine.semanticstream.ing/entities/acme/ops/robotics/gcs/drone/001"
	if got != want {
		t.Errorf("subjectToIRI() = %q, want %q", got, want)
	}
}

func TestSubjectToIRI_NonEntityFallback(t *testing.T) {
	base := "https://semengine.semanticstream.ing"
	got := subjectToIRI("some.topic.name", base)
	want := "https://semengine.semanticstream.ing/subjects/some/topic/name"
	if got != want {
		t.Errorf("subjectToIRI() = %q, want %q", got, want)
	}
}

func TestSubjectToIRI_Empty(t *testing.T) {
	got := subjectToIRI("", "https://example.org")
	if got != "" {
		t.Errorf("expected empty string for empty subject, got %q", got)
	}
}

func TestResolvePredicateIRI_Unregistered(t *testing.T) {
	defer vocabulary.SnapshotRegistry()()
	vocabulary.ClearRegistry()

	base := "https://semengine.semanticstream.ing"
	got := resolvePredicateIRI("robotics.battery.level", base)
	want := "https://semengine.semanticstream.ing/predicates/robotics/battery/level"
	if got != want {
		t.Errorf("resolvePredicateIRI() = %q, want %q", got, want)
	}
}

func TestResolvePredicateIRI_Registered(t *testing.T) {
	defer vocabulary.SnapshotRegistry()()
	vocabulary.ClearRegistry()

	vocabulary.Register("robotics.battery.level",
		vocabulary.WithIRI("https://schema.org/batteryLevel"))

	got := resolvePredicateIRI("robotics.battery.level", "https://semengine.semanticstream.ing")
	if got != "https://schema.org/batteryLevel" {
		t.Errorf("expected registered standard IRI, got %q", got)
	}
}

func TestResolvePredicateIRI_Empty(t *testing.T) {
	got := resolvePredicateIRI("", "https://example.org")
	if got != "" {
		t.Errorf("expected empty string for empty predicate, got %q", got)
	}
}

// TestSubjectToIRIFollowsCanonicalOrder pins the published IRI path
// `…/entities/{org}/{platform}/{system}/{domain}/{type}/{instance}` — the
// canonical order emitted from the named fields (owner item O-10; a
// published-artifact break fresh state does not re-mint).
func TestSubjectToIRIFollowsCanonicalOrder(t *testing.T) {
	base := "https://semengine.semanticstream.ing"
	got := subjectToIRI("acme.dep1.src.git.commit.a1", base)
	want := "https://semengine.semanticstream.ing/entities/acme/dep1/src/git/commit/a1"
	if got != want {
		t.Errorf("subjectToIRI() = %q, want %q", got, want)
	}
	eid, err := message.ParseEntityID("acme.dep1.src.git.commit.a1")
	if err != nil {
		t.Fatal(err)
	}
	byName := base + "/entities/" + eid.Org + "/" + eid.Platform + "/" + eid.System + "/" + eid.Domain + "/" + eid.Type + "/" + eid.Instance
	if got != byName {
		t.Errorf("subjectToIRI() = %q, want the named-field composition %q", got, byName)
	}
}

func TestRegisterPackWithExportPrefixRegistrar(t *testing.T) {
	defer vocabulary.SnapshotRegistry()()
	vocabulary.ClearRegistry()
	t.Cleanup(func() {
		defaultPrefixesMu.Lock()
		delete(defaultPrefixes, "diekm")
		delete(defaultPrefixes, "review235")
		defaultPrefixesMu.Unlock()
	})

	pack, err := vocabulary.LoadPackJSON(strings.NewReader(`{
		"name": "diekm-export-test",
		"version": "1.0.0",
		"prefixes": [
			{"prefix": "diekm", "namespace": "https://confirm.mil/diekm/"}
		],
		"predicates": [
			{
				"name": "uxs.telemetry.altitude",
				"data_type": "float",
				"iri": "https://confirm.mil/diekm/hasAltitude"
			}
		]
	}`))
	if err != nil {
		t.Fatalf("LoadPackJSON() error = %v", err)
	}
	if err := vocabulary.RegisterPack(pack, vocabulary.WithPrefixRegistrar(RegisterPrefixes)); err != nil {
		t.Fatalf("RegisterPack() error = %v", err)
	}

	triples := []message.Triple{
		{Subject: "acme.ops.robotics.uxs.uas.001", Predicate: "uxs.telemetry.altitude", Object: 120.5},
	}
	turtleOut, err := SerializeToString(triples, Turtle)
	if err != nil {
		t.Fatalf("SerializeToString(Turtle) error = %v", err)
	}
	if !strings.Contains(turtleOut, "@prefix diekm: <https://confirm.mil/diekm/> .") {
		t.Errorf("expected registered diekm prefix in Turtle output, got:\n%s", turtleOut)
	}
	if !strings.Contains(turtleOut, "diekm:hasAltitude") {
		t.Errorf("expected compact diekm:hasAltitude predicate in Turtle output, got:\n%s", turtleOut)
	}

	// Late prefix conflict in a pack must not install earlier prefixes or predicates (#237).
	conflictingPack := vocabulary.PackSpec{
		Name: "conflicting-pack",
		Prefixes: []vocabulary.PrefixSpec{
			{Prefix: "review235", Namespace: "https://example.org/first/"},
			{Prefix: "rdf", Namespace: "https://example.org/conflict/"},
		},
		Predicates: []vocabulary.PredicateSpec{
			{Name: "review.pack.first"},
		},
	}
	if err := vocabulary.RegisterPack(conflictingPack, vocabulary.WithPrefixRegistrar(RegisterPrefixes)); err == nil {
		t.Fatalf("expected error from conflicting rdf prefix in pack")
	}
	if got := vocabulary.GetPredicateMetadata("review.pack.first"); got != nil {
		t.Fatalf("expected predicate from rejected pack to remain unregistered, got %+v", got)
	}
	if err := Register("review235", "https://example.org/second/"); err != nil {
		t.Fatalf("expected review235 prefix to be unbound after rejected pack, got error: %v", err)
	}

	// Malformed namespace IRI (such as trailing '>') in a pack is rejected before mutating state (#237).
	malformedNSPack := vocabulary.PackSpec{
		Name: "malformed-ns-pack",
		Prefixes: []vocabulary.PrefixSpec{
			{Prefix: "badns", Namespace: "https://bad.example/>"},
		},
		Predicates: []vocabulary.PredicateSpec{
			{Name: "review.bad.namespace", IRI: "https://bad.example/>term"},
		},
	}
	if err := vocabulary.RegisterPack(malformedNSPack, vocabulary.WithPrefixRegistrar(RegisterPrefixes)); err == nil {
		t.Fatalf("expected error from malformed namespace IRI in pack")
	}
	if got := vocabulary.GetPredicateMetadata("review.bad.namespace"); got != nil {
		t.Fatalf("expected predicate from malformed-ns pack to remain unregistered, got %+v", got)
	}
}

func TestSubjectIRI_MatchesDefaultExporter(t *testing.T) {
	t.Parallel()

	entityID := "acme.ops.robotics.gcs.drone.001"
	wantDefault := "https://semengine.semanticstream.ing/entities/acme/ops/robotics/gcs/drone/001"
	if got := SubjectIRI(entityID, ""); got != wantDefault {
		t.Fatalf("SubjectIRI(%q, \"\") = %q, want %q", entityID, got, wantDefault)
	}
	if got := SubjectIRI(entityID, vocabulary.SemEngineBase+"/"); got != wantDefault {
		t.Fatalf("SubjectIRI(%q, trailing slash) = %q, want %q", entityID, got, wantDefault)
	}

	customBase := "https://example.org///"
	wantCustom := "https://example.org/entities/acme/ops/robotics/gcs/drone/001"
	if got := SubjectIRI(entityID, customBase); got != wantCustom {
		t.Fatalf("SubjectIRI(%q, %q) = %q, want %q", entityID, customBase, got, wantCustom)
	}

	nonEntity := "some.topic.name"
	wantFallback := "https://example.org/subjects/some/topic/name"
	if got := SubjectIRI(nonEntity, "https://example.org/"); got != wantFallback {
		t.Fatalf("SubjectIRI(%q, \"https://example.org/\") = %q, want %q", nonEntity, got, wantFallback)
	}

	if got := SubjectIRI("", "https://example.org"); got != "" {
		t.Fatalf("SubjectIRI(\"\", base) = %q, want \"\"", got)
	}

	// Verify exact match with SerializeToString(NTriples).
	ntOut, err := SerializeToString([]message.Triple{
		{Subject: entityID, Predicate: "robotics.battery.level", Object: 85.5},
	}, NTriples, WithBaseIRI("https://example.org/"))
	if err != nil {
		t.Fatalf("SerializeToString(NTriples) error = %v", err)
	}
	if !strings.HasPrefix(ntOut, "<"+wantCustom+"> ") {
		t.Fatalf("SerializeToString subject prefix mismatch: got %q, want prefix <%s>", ntOut, wantCustom)
	}
}

func FuzzSubjectIRI(f *testing.F) {
	for _, seed := range []struct {
		subject string
		base    string
	}{
		{"acme.ops.robotics.gcs.drone.001", ""},
		{"acme.ops.robotics.gcs.drone.001", "https://example.org/"},
		{"some.topic.name", "https://example.org///"},
		{"", "https://example.org"},
	} {
		f.Add(seed.subject, seed.base)
	}

	f.Fuzz(func(t *testing.T, subject, base string) {
		got := SubjectIRI(subject, base)
		if subject == "" {
			if got != "" {
				t.Fatalf("SubjectIRI(\"\", %q) = %q, want empty", base, got)
			}
			return
		}
		trimmed := strings.TrimRight(base, "/")
		if gotTrimmed := SubjectIRI(subject, trimmed); got != gotTrimmed {
			t.Fatalf("SubjectIRI(%q, %q) = %q != SubjectIRI(%q, %q) = %q", subject, base, got, subject, trimmed, gotTrimmed)
		}
		if trimmed == "" {
			if gotDefault := SubjectIRI(subject, vocabulary.SemEngineBase); got != gotDefault {
				t.Fatalf("SubjectIRI(%q, %q) = %q != default %q", subject, base, got, gotDefault)
			}
		}
		if message.IsValidEntityID(subject) {
			if !strings.Contains(got, "/entities/") {
				t.Fatalf("valid entity ID %q produced non-entity IRI %q", subject, got)
			}
		} else {
			if !strings.Contains(got, "/subjects/") {
				t.Fatalf("non-entity subject %q produced non-fallback IRI %q", subject, got)
			}
		}
	})
}

func TestSerialize_UnfoldedReifiedBlankNodeChain(t *testing.T) {
	defer vocabulary.SnapshotRegistry()()
	vocabulary.ClearRegistry()
	t.Cleanup(func() {
		defaultPrefixesMu.Lock()
		delete(defaultPrefixes, "bfo")
		delete(defaultPrefixes, "cco")
		defaultPrefixesMu.Unlock()
	})

	if err := Register("bfo", "http://purl.obolibrary.org/obo/"); err != nil {
		t.Fatalf("Register(bfo) error = %v", err)
	}
	if err := Register("cco", "http://www.ontologyrepository.com/CommonCoreOntologies/"); err != nil {
		t.Fatalf("Register(cco) error = %v", err)
	}

	vocabulary.Register("uxs.rel.measurement-node",
		vocabulary.WithDataType(vocabulary.DataTypeEntityID),
		vocabulary.WithIRI("http://purl.obolibrary.org/obo/RO_0000056"))

	triples := []message.Triple{
		{
			Subject:   "acme.ops.robotics.gcs.drone.001",
			Predicate: "http://purl.obolibrary.org/obo/RO_0000056",
			Object:    "_:b_1",
		},
		{
			Subject:   "acme.ops.robotics.gcs.drone.001",
			Predicate: "uxs.rel.measurement-node",
			Object:    "_:b_explicit",
			Datatype:  message.EntityReferenceDatatype,
		},
		{
			Subject:   "acme.ops.robotics.gcs.drone.001",
			Predicate: "uxs.rel.measurement-node",
			Object:    "_:b_declared",
		},
		{
			Subject:   "_:b_1",
			Predicate: "http://www.w3.org/1999/02/22-rdf-syntax-ns#type",
			Object:    "http://www.ontologyrepository.com/CommonCoreOntologies/ActOfMeasuring",
		},
		{
			Subject:   "_:b_1",
			Predicate: "http://www.ontologyrepository.com/CommonCoreOntologies/has_decimal_value",
			Object:    85.5,
		},
	}

	t.Run("NTriples", func(t *testing.T) {
		out, err := SerializeToString(triples, NTriples)
		if err != nil {
			t.Fatalf("SerializeToString(NTriples) error = %v", err)
		}
		wantLines := []string{
			`<https://semengine.semanticstream.ing/entities/acme/ops/robotics/gcs/drone/001> <http://purl.obolibrary.org/obo/RO_0000056> _:b_1 .`,
			`<https://semengine.semanticstream.ing/entities/acme/ops/robotics/gcs/drone/001> <http://purl.obolibrary.org/obo/RO_0000056> _:b_explicit .`,
			`<https://semengine.semanticstream.ing/entities/acme/ops/robotics/gcs/drone/001> <http://purl.obolibrary.org/obo/RO_0000056> _:b_declared .`,
			`_:b_1 <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://www.ontologyrepository.com/CommonCoreOntologies/ActOfMeasuring> .`,
			`_:b_1 <http://www.ontologyrepository.com/CommonCoreOntologies/has_decimal_value> "85.5"^^<http://www.w3.org/2001/XMLSchema#double> .`,
		}
		for _, line := range wantLines {
			if !strings.Contains(out, line) {
				t.Errorf("N-Triples missing expected line:\n  want: %s\n  got:\n%s", line, out)
			}
		}
		if strings.Contains(out, "<_:") || strings.Contains(out, "\"_:") || strings.Contains(out, "/subjects/_:") || strings.Contains(out, "/predicates/http") {
			t.Errorf("N-Triples contains improperly escaped blank node or predicate IRI:\n%s", out)
		}
	})

	t.Run("Turtle", func(t *testing.T) {
		out, err := SerializeToString(triples, Turtle)
		if err != nil {
			t.Fatalf("SerializeToString(Turtle) error = %v", err)
		}
		if !strings.Contains(out, "bfo:RO_0000056 _:b_1") {
			t.Errorf("Turtle expected compacted predicate with blank node object, got:\n%s", out)
		}
		if !strings.Contains(out, "\n_:b_1 ") {
			t.Errorf("Turtle expected blank node subject _:b_1, got:\n%s", out)
		}
		if !strings.Contains(out, "cco:has_decimal_value \"85.5\"^^xsd:double") {
			t.Errorf("Turtle expected compacted cco:has_decimal_value on blank node subject, got:\n%s", out)
		}
		if strings.Contains(out, "<_:") || strings.Contains(out, "\"_:") || strings.Contains(out, "/subjects/_:") || strings.Contains(out, "/predicates/http") {
			t.Errorf("Turtle contains improperly escaped blank node or predicate IRI:\n%s", out)
		}
	})

	t.Run("JSONLD", func(t *testing.T) {
		out, err := SerializeToString(triples, JSONLD)
		if err != nil {
			t.Fatalf("SerializeToString(JSONLD) error = %v", err)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("JSON-LD unmarshal error: %v\noutput:\n%s", err, out)
		}
		graph, ok := doc["@graph"].([]any)
		if !ok || len(graph) != 2 {
			t.Fatalf("expected 2 nodes in JSON-LD @graph, got: %#v", doc["@graph"])
		}
		bnode, ok := graph[1].(map[string]any)
		if !ok || bnode["@id"] != "_:b_1" {
			t.Fatalf("expected second node @id to be _:b_1, got: %#v", graph[1])
		}
		if bnode["cco:has_decimal_value"] != 85.5 {
			t.Errorf("expected cco:has_decimal_value = 85.5 on _:b_1, got: %#v", bnode["cco:has_decimal_value"])
		}
	})
}

func TestBlankNodeValidation_MalformedFallback(t *testing.T) {
	t.Parallel()

	for _, bad := range []string{"_:", "_:bad.", "_:has space", "_:bad:colon"} {
		out, err := SerializeToString([]message.Triple{
			{
				Subject:   "acme.ops.robotics.gcs.drone.001",
				Predicate: "robotics.status.mode",
				Object:    bad,
			},
		}, NTriples)
		if err != nil {
			t.Fatalf("SerializeToString(%q) error = %v", bad, err)
		}
		wantLiteral := `"` + bad + `" .`
		if !strings.Contains(out, wantLiteral) {
			t.Errorf("malformed blank-node object %q must fall back to string literal, got:\n%s", bad, out)
		}
	}

	// Valid blank-node string with explicit xsd:string datatype must remain a string literal.
	explicitOut, err := SerializeToString([]message.Triple{
		{
			Subject:   "acme.ops.robotics.gcs.drone.001",
			Predicate: "robotics.status.mode",
			Object:    "_:b_1",
			Datatype:  "xsd:string",
		},
	}, NTriples)
	if err != nil {
		t.Fatalf("SerializeToString(explicit xsd:string) error = %v", err)
	}
	if !strings.Contains(explicitOut, `"_:b_1" .`) {
		t.Errorf("explicit xsd:string blank-node string must remain a string literal, got:\n%s", explicitOut)
	}
}

func TestResolvePredicateIRI_FullIRIPassThrough(t *testing.T) {
	defer vocabulary.SnapshotRegistry()()
	vocabulary.ClearRegistry()

	base := "https://semengine.semanticstream.ing"
	httpPred := "http://purl.obolibrary.org/obo/RO_0000056"
	httpsPred := "https://confirm.mil/diekm/hasAltitude"

	if got := resolvePredicateIRI(httpPred, base); got != httpPred {
		t.Errorf("resolvePredicateIRI(%q) = %q, want unchanged", httpPred, got)
	}
	if got := resolvePredicateIRI(httpsPred, base+"/"); got != httpsPred {
		t.Errorf("resolvePredicateIRI(%q) = %q, want unchanged", httpsPred, got)
	}

	// Malformed HTTP IRI with forbidden IRIREF character falls back to /predicates/.
	badPred := "http://example.org/bad>iri"
	if got := resolvePredicateIRI(badPred, base); got == badPred {
		t.Errorf("resolvePredicateIRI(%q) must not pass through malformed IRIREF unchanged", badPred)
	}
}
