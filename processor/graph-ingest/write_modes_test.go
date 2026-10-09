package graphingest

// The write modes through graph-ingest's lanes (design D15, #100 and #98; task 4.8;
// graph-entity-writes, "One rule per write mode" and "Timestamp orders a replace"). Each test
// writes through the lanes' production entries: the stream lane's consume closure, the canonical
// mutation handlers, and the in-process create. The expected statements are written
// out in each test, never computed by the rule under test.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
	"github.com/nats-io/nats.go/jetstream"
)

// writeModesPredicate is the predicate P of the graph-entity-writes scenarios.
const writeModesPredicate = "test.state.value"

// wmTime is second n of a fixed hour: statements carry their own times, so the order of two
// writes is the test's choice, not the clock's.
func wmTime(n int) time.Time {
	return time.Date(2026, 10, 8, 12, 0, n, 0, time.UTC)
}

// wmStatement is one statement of P about subject. An empty source is left for the stream
// lane to fill from the envelope.
func wmStatement(subject string, object any, source string, at time.Time) message.Triple {
	return message.Triple{Subject: subject, Predicate: writeModesPredicate, Object: object, Source: source, Timestamp: at, Confidence: 1}
}

// heldEntry describes one stored statement as object/source/second.
func heldEntry(object any, source string, at time.Time) string {
	return fmt.Sprintf("%v/%s/%d", object, source, at.Second())
}

// assertHeld fails unless the statements of predicate stored for entityID are exactly want, in
// any order.
func assertHeld(t *testing.T, c *Component, entityID, predicate string, want ...string) {
	t.Helper()
	var got []string
	for _, triple := range storedEntity(t, c, entityID).Triples {
		if triple.Predicate == predicate {
			got = append(got, heldEntry(triple.Object, triple.Source, triple.Timestamp))
		}
	}
	slices.Sort(got)
	want = slices.Sorted(slices.Values(want))
	if !slices.Equal(got, want) {
		t.Errorf("%s of %s holds %v, want %v", predicate, entityID, got, want)
	}
}

// writeModesLane is the stream lane's consume closure and the stream sequence it last used.
type writeModesLane struct {
	t       *testing.T
	c       *Component
	handler func(context.Context, jetstream.Msg)
	seq     uint64
}

func newWriteModesLane(t *testing.T, opts ...testComponentOption) *writeModesLane {
	c, handler := startStreamLane(t, opts...)
	return &writeModesLane{t: t, c: c, handler: handler}
}

// send delivers one message from envelope source carrying triples, and requires it acknowledged.
func (l *writeModesLane) send(entityID, source string, triples ...message.Triple) {
	l.t.Helper()
	l.seq++
	data := streamLaneData(l.t, &mergeTestGraphable{entityID: entityID, triples: triples}, source, message.WithTime(wmTime(0)))
	if got := deliverStreamLane(l.t, l.handler, data, l.seq); got != "ack" {
		l.t.Fatalf("message %d disposition = %s, want ack", l.seq, got)
	}
}

// appendOnMutationLane appends triples through the canonical append and returns each subject's
// outcome.
func appendOnMutationLane(t *testing.T, c *Component, triples ...message.Triple) []graph.MutationOutcome {
	t.Helper()
	data, err := c.handleCanonicalAppend(context.Background(), mustCanonicalJSON(t, graph.AppendTriplesRequest{Triples: triples}))
	if err != nil {
		t.Fatalf("handleCanonicalAppend: %v", err)
	}
	var response graph.AppendTriplesResponse
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode append response: %v", err)
	}
	var outcomes []graph.MutationOutcome
	for _, result := range response.Results {
		outcomes = append(outcomes, result.Outcome)
	}
	return outcomes
}

// reconcileOnMutationLane reconciles P of entityID from source at its stored revision and
// returns the outcome.
func reconcileOnMutationLane(t *testing.T, c *Component, entityID, source string, desired ...message.Triple) graph.MutationOutcome {
	t.Helper()
	_, revision, ok := storedRevision(t, c, entityID)
	if !ok {
		t.Fatalf("entity %s is absent", entityID)
	}
	data, err := c.handleCanonicalReconcile(context.Background(), mustCanonicalJSON(t, graph.ReconcilePredicatesRequest{
		EntityID: entityID, ExpectedRevision: revision, Source: source,
		Predicates: []string{writeModesPredicate}, Desired: append([]message.Triple{}, desired...),
	}))
	if err != nil {
		t.Fatalf("handleCanonicalReconcile: %v", err)
	}
	var response graph.ReconcilePredicatesResponse
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode reconcile response: %v", err)
	}
	return response.Outcome
}

func withMetricsRegistry(registry *metric.MetricsRegistry) testComponentOption {
	return func(deps *component.Dependencies) { deps.MetricsRegistry = registry }
}

// staleSetsTotal reads semengine_graph_ingest_stale_sets_total from registry. A registry that
// does not gather it fails the test and reads as zero, so the rest of the test still runs.
func staleSetsTotal(t *testing.T, registry *metric.MetricsRegistry) float64 {
	t.Helper()
	family, ok := gatherFamilies(t, registry.PrometheusRegistry())["semengine_graph_ingest_stale_sets_total"]
	if !ok || len(family.GetMetric()) != 1 {
		t.Error("the registry does not gather semengine_graph_ingest_stale_sets_total")
		return 0
	}
	return family.GetMetric()[0].GetCounter().GetValue()
}

func TestWriteModesAgreeAcrossLanes(t *testing.T) {
	t.Run("birth through all three lanes", func(t *testing.T) {
		lane := newWriteModesLane(t)
		const viaMutation, viaInProcess, viaStream = "c360.test.write.birth.mutation.001",
			"c360.test.write.birth.process.001", "c360.test.write.birth.stream.001"
		statements := func(subject string) []message.Triple {
			return []message.Triple{wmStatement(subject, "v", "source-a", wmTime(1)), wmStatement(subject, "w", "source-a", wmTime(1))}
		}

		createCanonicalEntity(t, lane.c, viaMutation, statements(viaMutation))
		inProcess := canonicalMutationEntity(viaInProcess)
		inProcess.Triples = statements(viaInProcess)
		if err := lane.c.CreateEntity(context.Background(), inProcess); err != nil {
			t.Fatalf("CreateEntity: %v", err)
		}
		lane.send(viaStream, "source-a", statements(viaStream)...)

		for _, id := range []string{viaMutation, viaInProcess, viaStream} {
			assertHeld(t, lane.c, id, writeModesPredicate, "v/source-a/1", "w/source-a/1")
		}
	})

	// Append has one lane, the mutation request: ruling M (#91 comment 6080973822) removed the
	// in-process append.
	t.Run("append through the mutation lane", func(t *testing.T) {
		lane := newWriteModesLane(t)
		const viaMutation = "c360.test.write.append.mutation.001"
		createCanonicalEntity(t, lane.c, viaMutation, []message.Triple{canonicalTriple(viaMutation, "test.state.other", "seed")})

		for attempt := 1; attempt <= 2; attempt++ {
			outcomes := appendOnMutationLane(t, lane.c, wmStatement(viaMutation, "v", "source-a", wmTime(1)))
			wantOutcome := graph.MutationApplied
			if attempt == 2 {
				wantOutcome = graph.MutationUnchanged
			}
			if !slices.Equal(outcomes, []graph.MutationOutcome{wantOutcome}) {
				t.Errorf("append %d on the mutation lane: outcomes %v, want [%s]", attempt, outcomes, wantOutcome)
			}
			assertHeld(t, lane.c, viaMutation, writeModesPredicate, "v/source-a/1")
		}
	})

	// P holds a statement from source-a and one from source-b; the same statements from
	// source-a replace source-a's set through the stream lane and through a reconcile.
	t.Run("replace and conditional replace", func(t *testing.T) {
		lane := newWriteModesLane(t)
		const viaStream, viaReconcile = "c360.test.write.replace.stream.001", "c360.test.write.replace.reconcile.001"
		for _, id := range []string{viaStream, viaReconcile} {
			createCanonicalEntity(t, lane.c, id, []message.Triple{
				wmStatement(id, "a0", "source-a", wmTime(1)), wmStatement(id, "b0", "source-b", wmTime(1)),
			})
		}
		incoming := func(subject string) []message.Triple {
			return []message.Triple{wmStatement(subject, "a1", "source-a", wmTime(2)), wmStatement(subject, "a2", "source-a", wmTime(2))}
		}

		lane.send(viaStream, "source-a", incoming(viaStream)...)
		if got := reconcileOnMutationLane(t, lane.c, viaReconcile, "source-a", incoming(viaReconcile)...); got != graph.MutationApplied {
			t.Errorf("reconcile outcome = %s, want applied", got)
		}

		for _, id := range []string{viaStream, viaReconcile} {
			assertHeld(t, lane.c, id, writeModesPredicate, "a1/source-a/2", "a2/source-a/2", "b0/source-b/1")
		}
	})
}

func TestReplaceKeepsOtherSourcesStatements(t *testing.T) {
	lane := newWriteModesLane(t)
	const id = "c360.test.write.sources.entity.001"
	// The entity is born with another predicate, so the append has an entity to add to.
	lane.send(id, "seed-producer", message.Triple{Subject: id, Predicate: "test.state.other", Object: "seed", Timestamp: wmTime(0), Confidence: 1})
	appendOnMutationLane(t, lane.c, wmStatement(id, "from-a", "source-a", wmTime(1)))

	lane.send(id, "source-b", wmStatement(id, "from-b", "", wmTime(2)))
	assertHeld(t, lane.c, id, writeModesPredicate, "from-a/source-a/1", "from-b/source-b/2")

	lane.send(id, "source-b", wmStatement(id, "from-b-again", "", wmTime(3)))
	assertHeld(t, lane.c, id, writeModesPredicate, "from-a/source-a/1", "from-b-again/source-b/3")
}

func TestStreamLaneGroupsByStampedSource(t *testing.T) {
	lane := newWriteModesLane(t)
	const id = "c360.test.write.grouping.entity.001"
	lane.send(id, "producer-e",
		wmStatement(id, "e1", "", wmTime(1)), wmStatement(id, "x1", "relay-x", wmTime(1)), wmStatement(id, "y1", "relay-y", wmTime(1)))
	assertHeld(t, lane.c, id, writeModesPredicate, "e1/producer-e/1", "x1/relay-x/1", "y1/relay-y/1")

	// Statements without a source replace the set stored under the envelope's source.
	lane.send(id, "producer-e", wmStatement(id, "e2", "", wmTime(2)))
	assertHeld(t, lane.c, id, writeModesPredicate, "e2/producer-e/2", "x1/relay-x/1", "y1/relay-y/1")

	// Statements naming two sources replace those two sets and no other, the envelope's
	// included.
	lane.send(id, "producer-e", wmStatement(id, "x3", "relay-x", wmTime(3)), wmStatement(id, "y3", "relay-y", wmTime(3)))
	assertHeld(t, lane.c, id, writeModesPredicate, "e2/producer-e/2", "x3/relay-x/3", "y3/relay-y/3")
}

func TestReconcileReplacesOnlyItsSource(t *testing.T) {
	c, _ := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
	id := canonicalEntityA
	a1 := wmStatement(id, "a1", "source-a", wmTime(1))
	createCanonicalEntity(t, c, id, []message.Triple{a1, wmStatement(id, "b1", "source-b", wmTime(1))})

	// "Unchanged" compares source-a's statements only.
	if got := reconcileOnMutationLane(t, c, id, "source-a", a1); got != graph.MutationUnchanged {
		t.Errorf("reconcile of source-a's stored statements: outcome %s, want unchanged", got)
	}
	assertHeld(t, c, id, writeModesPredicate, "a1/source-a/1", "b1/source-b/1")

	if got := reconcileOnMutationLane(t, c, id, "source-a", wmStatement(id, "a2", "source-a", wmTime(2))); got != graph.MutationApplied {
		t.Errorf("reconcile to a2: outcome %s, want applied", got)
	}
	assertHeld(t, c, id, writeModesPredicate, "a2/source-a/2", "b1/source-b/1")

	// An empty set clears source-a's statements of P only.
	if got := reconcileOnMutationLane(t, c, id, "source-a"); got != graph.MutationApplied {
		t.Errorf("empty reconcile: outcome %s, want applied", got)
	}
	assertHeld(t, c, id, writeModesPredicate, "b1/source-b/1")
}

func TestReplaceOrderedByTimestamp(t *testing.T) {
	const id = "c360.test.write.ordered.entity.001"
	const other = "test.state.other"

	t.Run("stream lane", func(t *testing.T) {
		registry := metric.NewMetricsRegistry()
		lane := newWriteModesLane(t, withMetricsRegistry(registry))
		var logs bytes.Buffer
		lane.c.logger = slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
		lane.send(id, "source-a", wmStatement(id, "a2", "", wmTime(2)))
		before := staleSetsTotal(t, registry)

		// An older set from the same source is not applied; the message's newer set is.
		lane.send(id, "source-a", wmStatement(id, "a1", "", wmTime(1)),
			message.Triple{Subject: id, Predicate: other, Object: "q3", Timestamp: wmTime(3), Confidence: 1})
		assertHeld(t, lane.c, id, writeModesPredicate, "a2/source-a/2")
		assertHeld(t, lane.c, id, other, "q3/source-a/3")
		if got := staleSetsTotal(t, registry) - before; got != 1 {
			t.Errorf("stale_sets_total rose by %v after an older set from the same source, want 1", got)
		}
		var staleLine string
		for line := range strings.Lines(logs.String()) {
			if strings.Contains(line, "level=DEBUG") && strings.Contains(line, "source=source-a") {
				staleLine = line
			}
		}
		for _, want := range []string{"entity_id=" + id, "predicate=" + writeModesPredicate, "source=source-a"} {
			if !strings.Contains(staleLine, want) {
				t.Errorf("no debug line names the stale set's %q; last candidate %q", want, staleLine)
			}
		}

		// An older set from another source is stored and counts nothing.
		lane.send(id, "source-b", wmStatement(id, "b0", "", wmTime(0)))
		assertHeld(t, lane.c, id, writeModesPredicate, "a2/source-a/2", "b0/source-b/0")

		// An equal timestamp applies.
		lane.send(id, "source-a", wmStatement(id, "a2-again", "", wmTime(2)))
		assertHeld(t, lane.c, id, writeModesPredicate, "a2-again/source-a/2", "b0/source-b/0")
		if got := staleSetsTotal(t, registry) - before; got != 1 {
			t.Errorf("stale_sets_total rose by %v in all, want 1", got)
		}
	})

	// The entity's own fields take the arrival's values only when none of its sets was skipped.
	t.Run("message type and storage reference", func(t *testing.T) {
		c, _ := createTestComponentWithMockKVBucket(t, withAuthority("c360", "test"))
		first := message.Type{Domain: "test", Category: "merge", Version: "v1"}
		later := message.Type{Domain: "test", Category: "fixture", Version: "v1"}
		arrive := func(mt message.Type, ref string, triples ...message.Triple) {
			t.Helper()
			entity := &graph.EntityState{ID: id, MessageType: mt, Triples: triples,
				StorageRef: &message.StorageReference{StorageInstance: "store", Key: ref}}
			if err := c.mergeEntityOnLane(context.Background(), entity, false); err != nil {
				t.Fatalf("mergeEntityOnLane: %v", err)
			}
		}
		assertFields := func(mt message.Type, ref string) {
			t.Helper()
			stored := storedEntity(t, c, id)
			if stored.MessageType != mt || stored.StorageRef == nil || stored.StorageRef.Key != ref {
				t.Errorf("stored message type %v, storage reference %+v; want %v, %s", stored.MessageType, stored.StorageRef, mt, ref)
			}
		}

		arrive(first, "ref-1", wmStatement(id, "a2", "source-a", wmTime(2)))
		arrive(later, "ref-2", wmStatement(id, "a1", "source-a", wmTime(1)),
			message.Triple{Subject: id, Predicate: other, Object: "q3", Source: "source-a", Timestamp: wmTime(3), Confidence: 1})
		assertFields(first, "ref-1")

		arrive(later, "ref-3", wmStatement(id, "a3", "source-a", wmTime(3)))
		assertFields(later, "ref-3")
	})
}

func TestConfidenceAndContextNeverOrder(t *testing.T) {
	lane := newWriteModesLane(t)
	const id = "c360.test.write.confidence.entity.001"
	statement := func(object string, at time.Time, confidence float64, context string) message.Triple {
		triple := wmStatement(id, object, "", at)
		triple.Confidence, triple.Context = confidence, context
		return triple
	}
	lane.send(id, "source-a", statement("first", wmTime(1), 0.9, "context-1"))

	lane.send(id, "source-a", statement("newer-lower", wmTime(2), 0.1, "context-2"))
	assertHeld(t, lane.c, id, writeModesPredicate, "newer-lower/source-a/2")

	lane.send(id, "source-a", statement("older-higher", wmTime(1), 1, "context-1"))
	assertHeld(t, lane.c, id, writeModesPredicate, "newer-lower/source-a/2")
}
