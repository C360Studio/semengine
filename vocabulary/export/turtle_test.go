package export

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/vocabulary"
)

func TestWriteTurtle_BasicTriple(t *testing.T) {
	triples := []message.Triple{
		{
			Subject:   "acme.ops.robotics.gcs.drone.001",
			Predicate: "robotics.battery.level",
			Object:    85.5,
		},
	}

	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeTurtle(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	// Should have xsd prefix for the double datatype
	if !strings.Contains(got, "@prefix xsd:") {
		t.Errorf("expected xsd prefix declaration, got:\n%s", got)
	}
	// Should have the triple with xsd:double
	if !strings.Contains(got, `"85.5"^^xsd:double`) {
		t.Errorf("expected typed double literal, got:\n%s", got)
	}
	// Should end with .
	if !strings.Contains(got, " .") {
		t.Errorf("expected triple to end with dot, got:\n%s", got)
	}
}

func TestWriteTurtle_SubjectGrouping(t *testing.T) {
	triples := []message.Triple{
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.battery.level", Object: 85.5},
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.flight.armed", Object: true},
	}

	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeTurtle(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	// Should use semicolons for same subject
	if !strings.Contains(got, ";") {
		t.Errorf("expected semicolons for subject grouping, got:\n%s", got)
	}
	// Should only have one subject IRI reference
	subjectIRI := "https://semengine.semanticstream.ing/entities/acme/ops/robotics/gcs/drone/001"
	count := strings.Count(got, subjectIRI)
	if count != 1 {
		t.Errorf("expected subject IRI to appear once (grouped), appeared %d times:\n%s", count, got)
	}
}

func TestWriteTurtle_BareIntegerAndBoolean(t *testing.T) {
	triples := []message.Triple{
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.battery.cells", Object: 4},
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.flight.armed", Object: false},
	}

	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeTurtle(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	// Integers and booleans should be bare (not quoted) in Turtle
	if strings.Contains(got, `"4"`) {
		t.Errorf("integer should be bare, not quoted, got:\n%s", got)
	}
	if strings.Contains(got, `"false"`) {
		t.Errorf("boolean should be bare, not quoted, got:\n%s", got)
	}
}

func TestWriteTurtle_EntityReference(t *testing.T) {
	triples := []message.Triple{
		{
			Subject:   "acme.ops.robotics.gcs.drone.001",
			Predicate: "graph.rel.contains",
			Object:    "acme.ops.robotics.gcs.battery.001",
		},
	}

	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeTurtle(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	// Object should be an IRI reference, not a literal
	if strings.Contains(got, `"acme.ops.robotics.gcs.battery.001"`) {
		t.Errorf("entity reference should be IRI, not literal, got:\n%s", got)
	}
	if !strings.Contains(got, "entities/acme/ops/robotics/gcs/battery/001>") {
		t.Errorf("expected entity IRI in output, got:\n%s", got)
	}
}

func TestWriteTurtle_TimeLiteral(t *testing.T) {
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
	err := writeTurtle(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, `"2024-01-15T10:30:00Z"^^xsd:dateTime`) {
		t.Errorf("expected typed dateTime, got:\n%s", got)
	}
}

func TestWriteTurtle_OnlyUsedPrefixes(t *testing.T) {
	triples := []message.Triple{
		{
			Subject:   "acme.ops.robotics.gcs.drone.001",
			Predicate: "robotics.battery.level",
			Object:    85.5,
		},
	}

	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeTurtle(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	// Should have xsd prefix (used by double)
	if !strings.Contains(got, "@prefix xsd:") {
		t.Errorf("expected xsd prefix, got:\n%s", got)
	}
	// Should NOT have owl prefix (not used)
	if strings.Contains(got, "@prefix owl:") {
		t.Errorf("unexpected owl prefix (not used), got:\n%s", got)
	}
	// Should NOT have prov prefix (not used)
	if strings.Contains(got, "@prefix prov:") {
		t.Errorf("unexpected prov prefix (not used), got:\n%s", got)
	}
}

func TestWriteTurtle_RegisteredPredicateUsesStandardIRI(t *testing.T) {
	// Register a predicate with a standard IRI
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
	err := writeTurtle(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "owl:sameAs") {
		t.Errorf("expected owl:sameAs from registry, got:\n%s", got)
	}
	if !strings.Contains(got, "@prefix owl:") {
		t.Errorf("expected owl prefix declaration, got:\n%s", got)
	}
}

func TestWriteTurtle_EmptyInput(t *testing.T) {
	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeTurtle(&buf, nil, &opts)
	if err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

func TestWriteTurtle_MultipleSubjects(t *testing.T) {
	triples := []message.Triple{
		{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: "robotics.battery.level", Object: 85.5},
		{Subject: "acme.ops.robotics.gcs.drone.002", Predicate: "robotics.battery.level", Object: 50.0},
	}

	var buf bytes.Buffer
	opts := defaultOptions()
	err := writeTurtle(&buf, triples, &opts)
	if err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	// Both subjects should appear
	if !strings.Contains(got, "drone/001>") {
		t.Errorf("expected drone.001 subject, got:\n%s", got)
	}
	if !strings.Contains(got, "drone/002>") {
		t.Errorf("expected drone.002 subject, got:\n%s", got)
	}
	// Should have blank line between subjects
	if !strings.Contains(got, " .\n\n") {
		t.Errorf("expected blank line between subjects, got:\n%s", got)
	}
}

// parseSingleTripleTurtleResourceObject extracts the expanded object IRI from a
// single-triple Turtle document, stripping '#' comments outside '<...>' and
// '"..."' and expanding '@prefix' declarations.
func parseSingleTripleTurtleResourceObject(t *testing.T, turtleDoc string) string {
	t.Helper()
	prefixes := make(map[string]string)
	var stmtLines []string
	for _, rawLine := range strings.Split(turtleDoc, "\n") {
		line := stripTurtleComment(rawLine)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "@prefix ") {
			rest := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "@prefix "), "."))
			colon := strings.IndexByte(rest, ':')
			openIdx := strings.IndexByte(rest, '<')
			closeIdx := strings.LastIndexByte(rest, '>')
			if colon <= 0 || openIdx <= colon || closeIdx <= openIdx {
				t.Fatalf("malformed @prefix declaration: %q", rawLine)
			}
			prefixes[strings.TrimSpace(rest[:colon])] = rest[openIdx+1 : closeIdx]
			continue
		}
		stmtLines = append(stmtLines, line)
	}
	stmt := strings.TrimSpace(strings.Join(stmtLines, " "))
	if !strings.HasSuffix(stmt, " .") {
		t.Fatalf("Turtle statement does not end with ' .': %q", stmt)
	}
	stmt = strings.TrimSpace(strings.TrimSuffix(stmt, "."))
	fields := strings.Fields(stmt)
	if len(fields) != 3 {
		t.Fatalf("expected 3 Turtle terms, got %d in %q", len(fields), stmt)
	}
	objTerm := fields[2]
	if strings.HasPrefix(objTerm, "<") && strings.HasSuffix(objTerm, ">") {
		return objTerm[1 : len(objTerm)-1]
	}
	colon := strings.IndexByte(objTerm, ':')
	if colon <= 0 {
		t.Fatalf("unexpected Turtle object term %q", objTerm)
	}
	pfx := objTerm[:colon]
	local := objTerm[colon+1:]
	if !isValidPNLocal(local) {
		t.Fatalf("Turtle object term %q has invalid PN_LOCAL %q", objTerm, local)
	}
	ns, ok := prefixes[pfx]
	if !ok {
		t.Fatalf("Turtle object term %q uses undeclared prefix %q", objTerm, pfx)
	}
	return ns + local
}

func stripTurtleComment(line string) string {
	inIRI := false
	inQuotes := false
	escaped := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if escaped {
			escaped = false
			continue
		}
		if inQuotes {
			if c == '\\' {
				escaped = true
			} else if c == '"' {
				inQuotes = false
			}
			continue
		}
		if inIRI {
			if c == '>' {
				inIRI = false
			}
			continue
		}
		switch c {
		case '<':
			inIRI = true
		case '"':
			inQuotes = true
		case '#':
			return line[:i]
		}
	}
	return line
}

func TestWriteTurtle_ResourceObjectsWithQueryFragmentOrTrailingDotStayFullIRIs(t *testing.T) {
	if err := Register("review", "https://example.org/"); err != nil {
		t.Fatalf("Register review: %v", err)
	}
	t.Cleanup(func() {
		defaultPrefixesMu.Lock()
		delete(defaultPrefixes, "review")
		defaultPrefixesMu.Unlock()
	})

	for _, iri := range []string{
		"https://example.org/item?revision=1",
		"https://example.org/item#section",
		"https://example.org/item.",
	} {
		triple := []message.Triple{{
			Subject:   "a.b.c.d.e.f",
			Predicate: "review.link.value",
			Object:    iri,
		}}
		turtleOut, err := SerializeToString(triple, Turtle)
		if err != nil {
			t.Fatalf("SerializeToString(Turtle) for %q: %v", iri, err)
		}
		ntOut, err := SerializeToString(triple, NTriples)
		if err != nil {
			t.Fatalf("SerializeToString(NTriples) for %q: %v", iri, err)
		}

		parsedTurtleObj := parseSingleTripleTurtleResourceObject(t, turtleOut)
		if parsedTurtleObj != iri {
			t.Fatalf("parsed Turtle object IRI = %q, want %q\nTurtle:\n%s", parsedTurtleObj, iri, turtleOut)
		}
		if !strings.Contains(ntOut, "<"+parsedTurtleObj+">") {
			t.Fatalf("N-Triples output %q disagrees with Turtle object IRI <%s>", ntOut, parsedTurtleObj)
		}
	}
}
