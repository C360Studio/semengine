package graphingest

// Statement metadata on the mutation and in-process lanes (design D15, #98; task 4.8): a write
// carrying a statement without a Source or a Timestamp is refused, a create or stream merge
// carrying no statement is refused, and a reconcile names its source and carries no other. Each
// refusal is invalid_request and stores nothing. The writes go through the lanes' production
// entries: the canonical mutation handlers, and the CreateEntity and AddTriple hierarchy
// inference calls; the stream lane's merge, where the rule is a second line, is called directly.

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

// bucketContents copies the value and revision of every key the in-memory entity bucket holds.
func bucketContents(bucket *mockKVBucket) map[string]mockKVData {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	contents := make(map[string]mockKVData, len(bucket.data))
	for key, entry := range bucket.data {
		contents[key] = mockKVData{value: slices.Clone(entry.value), revision: entry.revision}
	}
	return contents
}

// assertNothingStored fails when the bucket's contents differ from before in any key, value
// or revision.
func assertNothingStored(t *testing.T, bucket *mockKVBucket, before map[string]mockKVData) {
	t.Helper()
	after := bucketContents(bucket)
	if !maps.EqualFunc(before, after, func(a, b mockKVData) bool {
		return a.revision == b.revision && bytes.Equal(a.value, b.value)
	}) {
		t.Errorf("a refused write changed the bucket: keys %v -> %v", slices.Sorted(maps.Keys(before)), slices.Sorted(maps.Keys(after)))
	}
}

// assertRefused checks that a write was refused as invalid_request naming each of names. An
// accepted write fails the test without stopping it, so the caller still reads what it stored.
func assertRefused(t *testing.T, err error, names ...string) {
	t.Helper()
	if err == nil {
		t.Error("the write was accepted; want an invalid_request refusal")
		return
	}
	assertInvalidRequest(t, err, names...)
}

func TestWriteRefusesStatementWithoutSourceOrTimestamp(t *testing.T) {
	at := time.Date(2026, 9, 21, 14, 13, 20, 0, time.UTC)
	statement := func(subject, predicate string, object any) message.Triple {
		return message.Triple{Subject: subject, Predicate: predicate, Object: object, Source: "metadata-writer", Timestamp: at}
	}
	noSource := func(tr message.Triple) message.Triple { tr.Source = ""; return tr }
	noTimestamp := func(tr message.Triple) message.Triple { tr.Timestamp = time.Time{}; return tr }

	const inProcessID = "acme.ops.robotics.gcs.drone.003"
	statementA := statement(canonicalEntityA, "test.state.value", "ready")
	statementB := statement(canonicalEntityB, "test.state.value", "idle")
	inProcess := statement(inProcessID, "test.state.value", "armed")

	mutationCreate := func(triples ...message.Triple) func(*testing.T, *Component) error {
		return func(t *testing.T, c *Component) error {
			_, err := c.handleCanonicalCreate(context.Background(), mustCanonicalJSON(t, graph.CreateEntityRequest{
				Entity: canonicalMutationEntity(inProcessID), Triples: append([]message.Triple{}, triples...),
			}))
			return err
		}
	}
	mutationAppend := func(triples ...message.Triple) func(*testing.T, *Component) error {
		return func(t *testing.T, c *Component) error {
			_, err := c.handleCanonicalAppend(context.Background(), mustCanonicalJSON(t, graph.AppendTriplesRequest{Triples: triples}))
			return err
		}
	}
	mutationReconcile := func(desired ...message.Triple) func(*testing.T, *Component) error {
		return func(t *testing.T, c *Component) error {
			_, revision, ok := storedRevision(t, c, canonicalEntityA)
			if !ok {
				t.Fatalf("seed entity %s is absent", canonicalEntityA)
			}
			_, err := c.handleCanonicalReconcile(context.Background(), mustCanonicalJSON(t, graph.ReconcilePredicatesRequest{
				EntityID: canonicalEntityA, ExpectedRevision: revision, Source: "metadata-writer",
				Predicates: []string{"test.state.value"}, Desired: desired,
			}))
			return err
		}
	}
	inProcessCreate := func(triples ...message.Triple) func(*testing.T, *Component) error {
		return func(_ *testing.T, c *Component) error {
			entity := canonicalMutationEntity(inProcessID)
			entity.Triples = triples
			_, err := (&entityManagerAdapter{component: c}).CreateEntity(context.Background(), entity)
			return err
		}
	}
	// The stream lane stamps a Graphable's statements from the envelope before its merge, so its
	// merge refuses as a second line: these cases call the merge directly.
	streamMerge := func(triples ...message.Triple) func(*testing.T, *Component) error {
		return func(_ *testing.T, c *Component) error {
			entity := canonicalMutationEntity(inProcessID)
			entity.Triples = triples
			return c.mergeEntityOnLane(context.Background(), entity, false)
		}
	}
	inProcessAppend := func(triple message.Triple) func(*testing.T, *Component) error {
		return func(_ *testing.T, c *Component) error {
			return (&tripleAdderAdapter{component: c}).AddTriple(context.Background(), triple)
		}
	}

	tests := []struct {
		name  string
		write func(*testing.T, *Component) error
		names []string // what the refusal must name; an empty create names no statement
	}{
		{"mutation create without source", mutationCreate(inProcess, noSource(inProcess)), []string{"triple[1]", "source"}},
		{"mutation create without timestamp", mutationCreate(inProcess, noTimestamp(inProcess)), []string{"triple[1]", "timestamp"}},
		{"mutation create with no statement", mutationCreate(), nil},
		// Statement 0 is for an entity the append could commit, so a refusal that came only
		// with statement 1's subject would leave statement 0 stored.
		{"mutation append without source", mutationAppend(statementA, noSource(statementB)), []string{"triple[1]", "source"}},
		{"mutation append without timestamp", mutationAppend(statementA, noTimestamp(statementB)), []string{"triple[1]", "timestamp"}},
		{"mutation reconcile without source", mutationReconcile(statementA, noSource(statementA)), []string{"triple[1]", "source"}},
		{"mutation reconcile without timestamp", mutationReconcile(statementA, noTimestamp(statementA)), []string{"triple[1]", "timestamp"}},
		{"in-process create without source", inProcessCreate(inProcess, noSource(inProcess)), []string{"triple[1]", "source"}},
		{"in-process create without timestamp", inProcessCreate(inProcess, noTimestamp(inProcess)), []string{"triple[1]", "timestamp"}},
		{"in-process create with no statement", inProcessCreate(), nil},
		{"in-process append without source", inProcessAppend(noSource(statementB)), []string{"triple[0]", "source"}},
		{"in-process append without timestamp", inProcessAppend(noTimestamp(statementB)), []string{"triple[0]", "timestamp"}},
		{"stream merge without source", streamMerge(inProcess, noSource(inProcess)), []string{"triple[1]", "source"}},
		{"stream merge without timestamp", streamMerge(inProcess, noTimestamp(inProcess)), []string{"triple[1]", "timestamp"}},
		{"stream merge with no statement", streamMerge(), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
			createCanonicalEntity(t, c, canonicalEntityA, []message.Triple{statement(canonicalEntityA, "test.state.value", "seeded")})
			createCanonicalEntity(t, c, canonicalEntityB, []message.Triple{statement(canonicalEntityB, "test.state.value", "seeded")})
			// Hierarchy on: an in-process create infers hierarchy before its own write, and the
			// inference creates containers and appends edges, so a refusal found after it would
			// leave them stored.
			c.config.EnableHierarchy = true
			c.initHierarchyInference()
			before := bucketContents(bucket)

			assertRefused(t, tt.write(t, c), tt.names...)
			assertNothingStored(t, bucket, before)
		})
	}
}

func TestReconcileRefusesForeignSourceStatement(t *testing.T) {
	at := time.Date(2026, 9, 21, 14, 13, 20, 0, time.UTC)
	from := func(source string, object any) message.Triple {
		return message.Triple{Subject: canonicalEntityA, Predicate: "test.state.value", Object: object, Source: source, Timestamp: at}
	}
	// request is a reconcile from source-a at the entity's revision, encoded as on the wire;
	// edit changes its decoded fields before it is encoded again.
	request := func(t *testing.T, c *Component, desired []message.Triple, edit func(map[string]any)) []byte {
		t.Helper()
		_, revision, ok := storedRevision(t, c, canonicalEntityA)
		if !ok {
			t.Fatalf("seed entity %s is absent", canonicalEntityA)
		}
		data := mustCanonicalJSON(t, graph.ReconcilePredicatesRequest{
			EntityID: canonicalEntityA, ExpectedRevision: revision, Source: "source-a",
			Predicates: []string{"test.state.value"}, Desired: desired,
		})
		var fields map[string]any
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		edit(fields)
		return mustCanonicalJSON(t, fields)
	}

	tests := []struct {
		name    string
		desired []message.Triple
		edit    func(map[string]any)
		names   []string
	}{
		{
			name:    "a desired statement from another source",
			desired: []message.Triple{from("source-a", "kept"), from("source-b", "foreign")},
			edit:    func(map[string]any) {},
			names:   []string{"triple[1]"},
		},
		{
			name:    "no source on the wire",
			desired: []message.Triple{from("source-a", "kept")},
			edit:    func(fields map[string]any) { delete(fields, "source") },
			names:   []string{"source"},
		},
		{
			// A Go caller that leaves Source unset sends "source": "".
			name:    "an empty source",
			desired: []message.Triple{from("source-a", "kept")},
			edit:    func(fields map[string]any) { fields["source"] = "" },
			names:   []string{"source"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
			createCanonicalEntity(t, c, canonicalEntityA, []message.Triple{from("source-a", "seeded"), from("source-b", "seeded")})
			before := bucketContents(bucket)

			_, err := c.handleCanonicalReconcile(context.Background(), request(t, c, tt.desired, tt.edit))
			assertRefused(t, err, tt.names...)
			assertNothingStored(t, bucket, before)
		})
	}
}
