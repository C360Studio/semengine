package graphingest

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/vocabulary"
)

// Each of graph-ingest's three configuration fields has a test here that fails when the
// field is ignored, that is, when the component runs on the field's default instead of
// the value the configuration gave it (design D6, Config (b)). Each test builds the
// component through CreateGraphIngest from JSON, so the value goes through the strict
// decoder, ApplyDefaults and Validate as an operator's would.

// newConfiguredGraphIngest builds graph-ingest through CreateGraphIngest from config and
// gives it the in-memory entity bucket, as createTestComponentWithMockKV does.
func newConfiguredGraphIngest(t *testing.T, config Config) *Component {
	t.Helper()
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	natsClient, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	built, err := CreateGraphIngest(raw, testDependencies(t, natsClient))
	if err != nil {
		t.Fatalf("CreateGraphIngest: %v", err)
	}
	c := built.(*Component)
	c.entityBucket = natsClient.NewKVStore(newMockKVBucket())
	return c
}

// createDrone creates one entity of the c360.platform.robotics.mav1.drone type through
// the in-process create path and returns its stored triples. The statement carries a
// Timestamp, which the hierarchy inference takes as the triggering time.
func createDrone(t *testing.T, c *Component, instance string) []message.Triple {
	t.Helper()
	id := "c360.platform.robotics.mav1.drone." + instance
	now := time.Now()
	entity := &graph.EntityState{
		ID:          id,
		MessageType: testEntityType(),
		Triples: withTestMetadata(
			message.Triple{Subject: id, Predicate: "robotics.status.armed", Object: true, Timestamp: now},
		),
		UpdatedAt: now,
	}
	if err := c.CreateEntity(t.Context(), entity); err != nil {
		t.Fatalf("CreateEntity %s: %v", id, err)
	}
	stored, err := c.entityBucket.Get(t.Context(), id)
	if err != nil {
		t.Fatalf("read %s back: %v", id, err)
	}
	var state graph.EntityState
	if err := graph.UnmarshalEntityState(stored.Value, &state); err != nil {
		t.Fatalf("decode %s: %v", id, err)
	}
	return state.Triples
}

func hasPredicate(triples []message.Triple, predicate string) bool {
	return slices.ContainsFunc(triples, func(tr message.Triple) bool { return tr.Predicate == predicate })
}

// TestConfigPortsAreTheComponentsPorts: an input port the configuration renames is
// the port the component declares. Ignored, the default entity_stream port is declared.
func TestConfigPortsAreTheComponentsPorts(t *testing.T) {
	config := DefaultConfig()
	config.Ports.Inputs[0].Name = "configured_entity_stream"

	c := newConfiguredGraphIngest(t, config)

	var names []string
	for _, port := range c.InputPorts() {
		names = append(names, port.Name)
	}
	if !slices.Contains(names, "configured_entity_stream") || slices.Contains(names, "entity_stream") {
		t.Fatalf("input ports = %v, want configured_entity_stream in place of entity_stream", names)
	}
}

// TestConfigEnableHierarchyAddsHierarchyStatements: with enable_hierarchy, a birth
// carries its type-membership statement. Ignored, hierarchy is off and it carries none.
func TestConfigEnableHierarchyAddsHierarchyStatements(t *testing.T) {
	config := DefaultConfig()
	config.EnableHierarchy = true

	c := newConfiguredGraphIngest(t, config)
	c.initHierarchyInference() // Start's step that reads the field

	if triples := createDrone(t, c, "001"); !hasPredicate(triples, vocabulary.HierarchyTypeMember) {
		t.Fatalf("birth with enable_hierarchy has no %s statement: %v", vocabulary.HierarchyTypeMember, triples)
	}
}

// TestHierarchyBirthWritesNoSiblingEdge: with enable_hierarchy, a second birth of one
// type carries its type-membership statement and no hierarchy.type.sibling statement,
// and the entity born first gains none (ruling G, #91 comment 6062681355).
func TestHierarchyBirthWritesNoSiblingEdge(t *testing.T) {
	config := DefaultConfig()
	config.EnableHierarchy = true

	c := newConfiguredGraphIngest(t, config)
	c.initHierarchyInference()

	createDrone(t, c, "001")
	second := createDrone(t, c, "002")
	if !hasPredicate(second, vocabulary.HierarchyTypeMember) {
		t.Fatalf("second birth has no %s statement, so hierarchy did not run: %v", vocabulary.HierarchyTypeMember, second)
	}
	first := storedEntity(t, c, "c360.platform.robotics.mav1.drone.001").Triples
	for name, triples := range map[string][]message.Triple{"second birth": second, "first entity": first} {
		if hasPredicate(triples, vocabulary.HierarchyTypeSibling) {
			t.Errorf("%s carries a %s statement: %v", name, vocabulary.HierarchyTypeSibling, triples)
		}
	}
}

// TestConfigIngestLanesSetsTheLaneCount: ingest_lanes 3 builds three ingest lanes,
// each with its in-memory redelivery guard. Ignored, the default eight are built. (0
// and a negative value: TestConfigIngestLanesBelowOne.)
func TestConfigIngestLanesSetsTheLaneCount(t *testing.T) {
	config := DefaultConfig()
	config.IngestLanes = 3

	c := newConfiguredGraphIngest(t, config)
	if err := c.buildIngestPool(t.Context()); err != nil {
		t.Fatalf("buildIngestPool: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := c.ingestPool.Shutdown(ctx); err != nil {
			t.Errorf("ingest pool Shutdown: %v", err)
		}
	})

	if got := len(c.ingestGuardMem); got != 3 {
		t.Fatalf("ingest lanes = %d, want 3", got)
	}
}

// TestConfigIngestLanesBelowOne: as at the pin, an explicit ingest_lanes 0 is read as
// unset and builds the default eight lanes, and a negative value is clamped to one lane
// (owner ruling, #91 comment 6059144952). The field has no omitempty, so the JSON the
// factory decodes carries the 0.
func TestConfigIngestLanesBelowOne(t *testing.T) {
	cases := []struct {
		name  string
		lanes int
		want  int
	}{
		{name: "zero", lanes: 0, want: 8},
		{name: "negative", lanes: -1, want: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := DefaultConfig()
			config.IngestLanes = tc.lanes

			c := newConfiguredGraphIngest(t, config)
			if err := c.buildIngestPool(t.Context()); err != nil {
				t.Fatalf("buildIngestPool: %v", err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := c.ingestPool.Shutdown(ctx); err != nil {
					t.Errorf("ingest pool Shutdown: %v", err)
				}
			})

			if got := len(c.ingestGuardMem); got != tc.want {
				t.Fatalf("ingest_lanes %d built %d lanes, want %d", tc.lanes, got, tc.want)
			}
		})
	}
}
