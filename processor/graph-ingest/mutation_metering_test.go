package graphingest

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

// meteringEntityC is a local subject no test creates: an append to it is entity_not_found.
const meteringEntityC = "acme.ops.robotics.gcs.drone.003"

// meteredRoute is the handler graph-ingest subscribes on subject: the route
// canonicalMutationRoutes returns for it, wrapped as setupMutationHandlers wraps it.
func meteredRoute(t *testing.T, c *Component, subject string) mutationHandler {
	t.Helper()
	routes, err := c.canonicalMutationRoutes()
	if err != nil {
		t.Fatalf("canonicalMutationRoutes: %v", err)
	}
	for _, route := range routes {
		if route.subject == subject {
			return c.meteredCanonicalMutation(route)
		}
	}
	t.Fatalf("no route subscribes %q", subject)
	return nil
}

// TestRefusedMutationMetersAndLogsOnce: a refused mutation request counts once on the
// rejection series, once on the outcome series, and logs one WARN line (#303; design D1).
// Every label is spelled out here, not taken from the constants the code uses.
func TestRefusedMutationMetersAndLogsOnce(t *testing.T) {
	staleRevision := func(t *testing.T, c *Component) uint64 {
		t.Helper()
		created := createCanonicalEntity(t, c, canonicalEntityA, []message.Triple{
			canonicalTriple(canonicalEntityA, "test.state.value", "ready"),
		})
		return created.KVRevision + 1
	}
	unregistered := canonicalMutationEntity(canonicalEntityA)
	unregistered.MessageType = message.Type{Domain: "test", Category: "unregistered", Version: "v1"}

	for _, tt := range []struct {
		name      string
		subject   string
		operation string
		request   func(t *testing.T, c *Component) []byte
		reason    string
		outcome   string
		// logged are fragments the one WARN line carries.
		logged []string
		// predicateReasons each rise by one on predicate_contract_rejections_total.
		predicateReasons []string
	}{
		{
			name: "reconcile revision mismatch", subject: "graph.mutation.entity.reconcile", operation: "entity.reconcile",
			request: func(t *testing.T, c *Component) []byte {
				return mustCanonicalJSON(t, graph.ReconcilePredicatesRequest{
					EntityID: canonicalEntityA, ExpectedRevision: staleRevision(t, c), Source: "canonical-test",
					Predicates: []string{"test.state.value"}, Desired: []message.Triple{},
				})
			},
			reason: "revision_mismatch", outcome: "revision_mismatch",
			logged: []string{"graph mutation rejected", "subject=graph.mutation.entity.reconcile",
				"reason=revision_mismatch", "expected_revision"},
		},
		{
			name: "delete revision mismatch", subject: "graph.mutation.entity.delete", operation: "entity.delete",
			request: func(t *testing.T, c *Component) []byte {
				return mustCanonicalJSON(t, graph.DeleteEntityRequest{
					EntityID: canonicalEntityA, ExpectedRevision: staleRevision(t, c),
				})
			},
			reason: "revision_mismatch", outcome: "revision_mismatch",
			logged: []string{"graph mutation rejected", "subject=graph.mutation.entity.delete",
				"reason=revision_mismatch", "expected_revision"},
		},
		{
			name: "create of an existing entity", subject: "graph.mutation.entity.create", operation: "entity.create",
			request: func(t *testing.T, c *Component) []byte {
				staleRevision(t, c)
				return mustCanonicalJSON(t, graph.CreateEntityRequest{
					Entity:  canonicalMutationEntity(canonicalEntityA),
					Triples: []message.Triple{canonicalTriple(canonicalEntityA, "test.state.value", "again")},
				})
			},
			reason: "entity_already_exists", outcome: "entity_already_exists",
			logged: []string{"graph mutation rejected", "reason=entity_already_exists"},
		},
		{
			name: "create with no entity", subject: "graph.mutation.entity.create", operation: "entity.create",
			request: func(t *testing.T, _ *Component) []byte {
				return mustCanonicalJSON(t, graph.CreateEntityRequest{})
			},
			reason: "invalid_request", outcome: "invalid_request",
			logged: []string{"graph mutation rejected", "reason=invalid_request"},
		},
		{
			name: "create of an unregistered message type", subject: "graph.mutation.entity.create", operation: "entity.create",
			request: func(t *testing.T, _ *Component) []byte {
				return mustCanonicalJSON(t, graph.CreateEntityRequest{Entity: unregistered})
			},
			reason: "message_type_unregistered", outcome: "message_type_unregistered",
			logged: []string{"graph mutation rejected", "reason=message_type_unregistered"},
		},
		{
			name: "create with invalid predicates", subject: "graph.mutation.entity.create", operation: "entity.create",
			request: func(t *testing.T, _ *Component) []byte {
				return mustCanonicalJSON(t, graph.CreateEntityRequest{
					Entity: canonicalMutationEntity(canonicalEntityA),
					Triples: withTestMetadata(
						canonicalTriple(canonicalEntityA, "legacy.one", "x"),         // predicate-audit:invalid {"kind":"stored-predicate","value":"legacy.one","reason":"arity"}
						canonicalTriple(canonicalEntityA, "legacy.two", "x"),         // predicate-audit:invalid {"kind":"stored-predicate","value":"legacy.two","reason":"arity"}
						canonicalTriple(canonicalEntityA, "Legacy.valid.shape", "x"), // predicate-audit:invalid {"kind":"stored-predicate","value":"Legacy.valid.shape","reason":"segment_start"}
					),
				})
			},
			reason: "invalid_request", outcome: "invalid_request",
			logged:           []string{"graph mutation rejected", "reason=invalid_request"},
			predicateReasons: []string{"arity", "segment_start"},
		},
		{
			name: "create of a foreign subject", subject: "graph.mutation.entity.create", operation: "entity.create",
			request: func(t *testing.T, _ *Component) []byte {
				return mustCanonicalJSON(t, graph.CreateEntityRequest{
					Entity: canonicalMutationEntity("other.ops.robotics.gcs.drone.001"),
				})
			},
			reason: "authority_foreign", outcome: "entity_id_authority_invalid",
			logged: []string{"graph-ingest: entity authority rejected", "reason=authority_foreign"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := createTestComponentWithMockKV(t, withAuthority("acme", "ops"))
			data := tt.request(t, c)
			handler := meteredRoute(t, c, tt.subject)
			var logs bytes.Buffer
			c.logger = slog.New(slog.NewTextHandler(&logs, nil))

			rejected := c.mutationRejections.WithLabelValues(tt.subject, tt.reason)
			counted := c.mutationOutcomes.WithLabelValues(tt.operation, tt.outcome)
			rejectedBefore, countedBefore := testutil.ToFloat64(rejected), testutil.ToFloat64(counted)
			predicateBefore := map[string]float64{}
			for _, reason := range tt.predicateReasons {
				predicateBefore[reason] = testutil.ToFloat64(c.predicateContractRejections.WithLabelValues(tt.subject, reason))
			}

			if _, err := handler(context.Background(), data); err == nil {
				t.Fatal("the request was accepted")
			}

			if delta := testutil.ToFloat64(rejected) - rejectedBefore; delta != 1 {
				t.Errorf("mutation_rejections_total{subject=%q, reason=%q} delta = %v, want 1", tt.subject, tt.reason, delta)
			}
			if delta := testutil.ToFloat64(counted) - countedBefore; delta != 1 {
				t.Errorf("graph_mutation_outcomes_total{operation=%q, outcome=%q} delta = %v, want 1", tt.operation, tt.outcome, delta)
			}
			for reason, before := range predicateBefore {
				after := testutil.ToFloat64(c.predicateContractRejections.WithLabelValues(tt.subject, reason))
				if after-before != 1 {
					t.Errorf("predicate_contract_rejections_total{lane=%q, reason=%q} delta = %v, want 1", tt.subject, reason, after-before)
				}
			}
			text := logs.String()
			if warnings := strings.Count(text, "level=WARN"); warnings != 1 {
				t.Errorf("WARN lines = %d, want 1:\n%s", warnings, text)
			}
			for _, fragment := range tt.logged {
				if !strings.Contains(text, fragment) {
					t.Errorf("log %q is missing %q", text, fragment)
				}
			}
		})
	}
}

// TestMutationOutcomeCountedAtTheDecision: each successful mutation request counts its
// reply's outcome once, and an append counts once per subject (design D1). The wanted counts
// come from the requests the test sends, and every label is spelled out.
func TestMutationOutcomeCountedAtTheDecision(t *testing.T) {
	c := createTestComponentWithMockKV(t, withAuthority("acme", "ops"))
	ctx := context.Background()
	call := func(subject string, request any) []byte {
		t.Helper()
		reply, err := meteredRoute(t, c, subject)(ctx, mustCanonicalJSON(t, request))
		if err != nil {
			t.Fatalf("%s: %v", subject, err)
		}
		return reply
	}
	entityReply := func(subject string, request any, want graph.MutationOutcome) graph.EntityMutationResponse {
		t.Helper()
		var response graph.EntityMutationResponse
		if err := json.Unmarshal(call(subject, request), &response); err != nil {
			t.Fatalf("decode %s reply: %v", subject, err)
		}
		if response.Outcome != want {
			t.Fatalf("%s outcome = %q, want %q", subject, response.Outcome, want)
		}
		return response
	}

	series := []struct {
		operation, outcome string
		want               float64
	}{
		{"entity.create", "applied", 2},
		{"entity.reconcile", "unchanged", 1},
		{"entity.reconcile", "applied", 1},
		{"triple.append", "applied", 1},
		{"triple.append", "unchanged", 1},
		{"triple.append", "entity_not_found", 1},
		{"entity.delete", "applied", 1},
	}
	before := make([]float64, len(series))
	for i, s := range series {
		before[i] = testutil.ToFloat64(c.mutationOutcomes.WithLabelValues(s.operation, s.outcome))
	}

	held := canonicalTriple(canonicalEntityB, "test.state.value", "ready")
	a := entityReply("graph.mutation.entity.create", graph.CreateEntityRequest{
		Entity:  canonicalMutationEntity(canonicalEntityA),
		Triples: []message.Triple{canonicalTriple(canonicalEntityA, "test.state.value", "ready")},
	}, graph.MutationApplied)
	b := entityReply("graph.mutation.entity.create", graph.CreateEntityRequest{
		Entity: canonicalMutationEntity(canonicalEntityB), Triples: []message.Triple{held},
	}, graph.MutationApplied)
	entityReply("graph.mutation.entity.reconcile", graph.ReconcilePredicatesRequest{
		EntityID: canonicalEntityA, ExpectedRevision: a.KVRevision, Source: "canonical-test",
		Predicates: []string{"test.state.value"},
		Desired:    []message.Triple{canonicalTriple(canonicalEntityA, "test.state.value", "ready")},
	}, graph.MutationUnchanged)
	entityReply("graph.mutation.entity.reconcile", graph.ReconcilePredicatesRequest{
		EntityID: canonicalEntityA, ExpectedRevision: a.KVRevision, Source: "canonical-test",
		Predicates: []string{"test.state.value"},
		Desired:    []message.Triple{canonicalTriple(canonicalEntityA, "test.state.value", "changed")},
	}, graph.MutationApplied)

	var appended graph.AppendTriplesResponse
	if err := json.Unmarshal(call("graph.mutation.triple.append", graph.AppendTriplesRequest{Triples: []message.Triple{
		canonicalTriple(canonicalEntityA, "test.evidence.value", "new"),
		held,
		canonicalTriple(meteringEntityC, "test.evidence.value", "absent"),
	}}), &appended); err != nil {
		t.Fatalf("decode append reply: %v", err)
	}
	wantAppend := map[string]graph.MutationOutcome{
		canonicalEntityA: graph.MutationApplied,
		canonicalEntityB: graph.MutationUnchanged,
		meteringEntityC:  graph.MutationEntityNotFound,
	}
	if len(appended.Results) != len(wantAppend) {
		t.Fatalf("append results = %#v", appended.Results)
	}
	for _, result := range appended.Results {
		if result.Outcome != wantAppend[result.EntityID] {
			t.Fatalf("append result for %q = %q, want %q", result.EntityID, result.Outcome, wantAppend[result.EntityID])
		}
	}

	var deleted graph.DeleteEntityResponse
	if err := json.Unmarshal(call("graph.mutation.entity.delete", graph.DeleteEntityRequest{
		EntityID: canonicalEntityB, ExpectedRevision: b.KVRevision,
	}), &deleted); err != nil {
		t.Fatalf("decode delete reply: %v", err)
	}
	if deleted.Outcome != graph.MutationApplied {
		t.Fatalf("delete outcome = %q, want applied", deleted.Outcome)
	}

	for i, s := range series {
		if delta := testutil.ToFloat64(c.mutationOutcomes.WithLabelValues(s.operation, s.outcome)) - before[i]; delta != s.want {
			t.Errorf("graph_mutation_outcomes_total{operation=%q, outcome=%q} delta = %v, want %v",
				s.operation, s.outcome, delta, s.want)
		}
	}
}
