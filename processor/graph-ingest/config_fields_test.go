package graphingest

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
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
	if err := graph.UnmarshalEntityState(id, stored.Value, &state); err != nil {
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
	buildHierarchyInference(t, c) // Start's step that reads the field

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
	buildHierarchyInference(t, c)

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

// withEntityStreamTiming returns config with the entity_stream port declaring ackWait and
// interval as its ack_wait and heartbeat_interval; "" declares nothing.
func withEntityStreamTiming(t *testing.T, config Config, ackWait, interval string) Config {
	t.Helper()
	port, ok := config.Ports.Inputs[0].Config.(component.JetStreamPort)
	if !ok || config.Ports.Inputs[0].Name != "entity_stream" {
		t.Fatalf("the first input is %q (%T), want the entity_stream JetStream port",
			config.Ports.Inputs[0].Name, config.Ports.Inputs[0].Config)
	}
	port.AckWait, port.HeartbeatInterval = ackWait, interval
	config.Ports.Inputs[0].Config = port
	return config
}

// bindEntityStream runs c's consumer setup as Start does, with the wait for the stream and
// the consumer's creation stood in for. It returns the configuration the entity_stream
// consumer was created with, whether it was created, and setup's error.
func bindEntityStream(t *testing.T, c *Component) (natsclient.StreamConsumerConfig, bool, error) {
	t.Helper()
	c.waitForStreamInput = func(context.Context, string) (time.Time, error) { return time.Unix(1, 0), nil }
	var bound natsclient.StreamConsumerConfig
	created := false
	c.consumeStream = func(
		_ context.Context,
		_ natsclient.PortConsumerContext,
		cfg natsclient.StreamConsumerConfig,
		_ func(context.Context, jetstream.Msg),
	) (jetstream.ConsumeContext, error) {
		bound, created = cfg, true
		return nil, nil
	}
	err := c.setupSubscriptions(t.Context(), t.Context())
	return bound, created, err
}

// TestConfigAckWaitAndHeartbeatInterval: the entity_stream port's ack_wait reaches its
// consumer and its heartbeat_interval is read, each with graph-ingest's default when the
// port declares none: 30 s, and a quarter of the wait in effect. A heartbeat_interval
// above half the wait fails setup before the consumer is created (design D8, "Long
// apply"). Ignored, the consumer is created with no wait, so natsclient's own fallback
// applies, and no pair is refused. Until an application signals progress, the interval
// reaches nothing setup hands on, so it is read from ingestDeliveryTiming, the function
// setup calls, on the component's own port.
func TestConfigAckWaitAndHeartbeatInterval(t *testing.T) {
	accepted := []struct {
		name                      string
		ackWait, interval         string
		wantAckWait, wantInterval time.Duration
	}{
		{name: "nothing declared", wantAckWait: 30 * time.Second, wantInterval: 7500 * time.Millisecond},
		{name: "ack_wait alone", ackWait: "2s", wantAckWait: 2 * time.Second, wantInterval: 500 * time.Millisecond},
		{name: "both declared", ackWait: "2s", interval: "1s", wantAckWait: 2 * time.Second, wantInterval: time.Second},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfiguredGraphIngest(t, withEntityStreamTiming(t, DefaultConfig(), tc.ackWait, tc.interval))
			bound, created, err := bindEntityStream(t, c)
			if err != nil || !created {
				t.Fatalf("consumer setup: created %v, error %v; want the consumer created", created, err)
			}
			if bound.AckWait != tc.wantAckWait {
				t.Errorf("the consumer's ack wait = %s, want %s", bound.AckWait, tc.wantAckWait)
			}
			declared, err := component.GetConsumerConfig(c.InputPorts()[0])
			if err != nil {
				t.Fatalf("GetConsumerConfig(%s): %v", c.InputPorts()[0].Name, err)
			}
			if _, interval := ingestDeliveryTiming(declared); interval != tc.wantInterval {
				t.Errorf("the heartbeat interval = %s, want %s", interval, tc.wantInterval)
			}
		})
	}

	t.Run("interval above half the wait", func(t *testing.T) {
		c := newConfiguredGraphIngest(t, withEntityStreamTiming(t, DefaultConfig(), "2s", "1500ms"))
		_, created, err := bindEntityStream(t, c)
		if err == nil || !strings.Contains(err.Error(), "heartbeat interval 1.5s exceeds") {
			t.Errorf("consumer setup error = %v, want the 1.5s interval refused", err)
		}
		if created {
			t.Error("the consumer was created for a refused pair")
		}
	})

	// The port's own validation refuses a negative wait when graph-ingest is built, before
	// Start reads it. The port will not encode one, so the JSON is edited as an operator
	// would write it.
	t.Run("negative wait", func(t *testing.T) {
		raw, err := json.Marshal(withEntityStreamTiming(t, DefaultConfig(), "2s", ""))
		if err != nil {
			t.Fatalf("marshal config: %v", err)
		}
		if strings.Count(string(raw), `"ack_wait":"2s"`) != 1 {
			t.Fatalf("the encoded config does not declare ack_wait 2s once: %s", raw)
		}
		raw = []byte(strings.Replace(string(raw), `"ack_wait":"2s"`, `"ack_wait":"-2s"`, 1))
		natsClient, err := natsclient.NewClient("")
		if err != nil {
			t.Fatalf("natsclient.NewClient: %v", err)
		}
		_, err = CreateGraphIngest(raw, testDependencies(t, natsClient))
		if err == nil || !strings.Contains(err.Error(), `"ack_wait"`) {
			t.Errorf("CreateGraphIngest error = %v, want the negative ack_wait refused", err)
		}
	})
}
