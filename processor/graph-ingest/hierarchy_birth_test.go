package graphingest

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/inference"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// The entity under test and its three containers, written out from the README's container
// rule (type: 5-part prefix + .group; taxonomy: 4-part prefix + .group.container; source:
// 3-part prefix + .group.container.level), not computed by the inference.
const (
	hierarchyBirthID        = "acme.ops.robotics.gcs.drone.001"
	hierarchyBirthType      = "acme.ops.robotics.gcs.drone.group"
	hierarchyBirthTaxonomy  = "acme.ops.robotics.gcs.group.container"
	hierarchyBirthSource    = "acme.ops.robotics.group.container.level"
	hierarchyBirthPredicate = "test.state.value"
)

var errInjectedRefusal = errors.New("injected refusal")

// refuseFirstEdge is the inference's store: graph-ingest's own adapter, except that the first
// inverse edge it is asked to write is refused, once, with an error classified invalid. A
// refusal the store classifies invalid must still leave the birth retryable (design D21).
type refuseFirstEdge struct {
	inference.EntityStore
	refused atomic.Bool
}

func (s *refuseFirstEdge) AddTriple(ctx context.Context, triple message.Triple) error {
	if s.refused.CompareAndSwap(false, true) {
		return errs.WrapInvalid(errInjectedRefusal, "refuseFirstEdge", "AddTriple", "write inverse edge")
	}
	return s.EntityStore.AddTriple(ctx, triple)
}

// hierarchyBirthComponent is graph-ingest over a mock bucket with hierarchy on, its inference
// built by the production constructor over the production adapter. With refuse set, the
// adapter refuses the first inverse edge once.
func hierarchyBirthComponent(t *testing.T, refuse bool) (*Component, *mockKVBucket) {
	t.Helper()
	c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
	c.config.EnableHierarchy = true
	var store inference.EntityStore = &hierarchyStore{component: c}
	if refuse {
		store = &refuseFirstEdge{EntityStore: store}
	}
	hi, err := inference.NewHierarchyInference(store, c.platform, c.logger)
	if err != nil {
		t.Fatalf("NewHierarchyInference: %v", err)
	}
	c.hierarchyInference = hi
	c.ingestGuardMem = []*laneGuard{newLaneGuard(16)}
	return c, bucket
}

// hierarchyBirth is a fresh arrival for the entity, as a redelivery decodes it again.
func hierarchyBirth() *graph.EntityState {
	return &graph.EntityState{
		ID:          hierarchyBirthID,
		MessageType: testEntityType(),
		Triples: withTestMetadata(message.Triple{
			Subject: hierarchyBirthID, Predicate: hierarchyBirthPredicate, Object: "ready",
			Timestamp: time.Now(), Confidence: 1.0,
		}),
	}
}

func hierarchyBirthWork(msg *keyedIngestTestMsg) ingestWork {
	return ingestWork{entity: hierarchyBirth(), msg: msg, entityID: hierarchyBirthID, stream: "ENTITY", seq: 1}
}

func assertEntityAbsent(t *testing.T, bucket *mockKVBucket) {
	t.Helper()
	bucket.mu.Lock()
	_, stored := bucket.data[hierarchyBirthID]
	bucket.mu.Unlock()
	if stored {
		t.Fatal("the entity was stored although its hierarchy inference failed")
	}
}

// assertBornWithContainers checks the entity carries one membership edge to each of its three
// containers, and each container one inverse edge back: a repeat from the failed attempt was
// suppressed, not stored twice.
func assertBornWithContainers(t *testing.T, c *Component) {
	t.Helper()
	stored := storedEntity(t, c, hierarchyBirthID)
	if got := storedStatement(t, stored, hierarchyBirthPredicate); got.Object != "ready" {
		t.Errorf("stored %s = %v, want ready", hierarchyBirthPredicate, got.Object)
	}
	want := map[string]string{
		"hierarchy.type.member":   hierarchyBirthType,
		"hierarchy.system.member": hierarchyBirthTaxonomy,
		"hierarchy.domain.member": hierarchyBirthSource,
	}
	got := map[string][]any{}
	for _, tr := range stored.Triples {
		if isHierarchyPredicate(tr.Predicate) {
			got[tr.Predicate] = append(got[tr.Predicate], tr.Object)
		}
	}
	if len(got) != len(want) {
		t.Errorf("membership edges = %v, want one to each of %v", got, want)
	}
	for predicate, container := range want {
		if objects := got[predicate]; len(objects) != 1 || objects[0] != container {
			t.Errorf("%s edges = %v, want exactly [%s]", predicate, objects, container)
		}
	}
	inverse := map[string]string{
		hierarchyBirthType:     "hierarchy.type.contains",
		hierarchyBirthTaxonomy: "hierarchy.system.contains",
		hierarchyBirthSource:   "hierarchy.domain.contains",
	}
	for container, predicate := range inverse {
		edges := 0
		for _, tr := range storedEntity(t, c, container).Triples {
			if tr.Predicate == predicate && tr.Object == hierarchyBirthID {
				edges++
			}
		}
		if edges != 1 {
			t.Errorf("container %s holds %d %s edges to the entity, want 1", container, edges, predicate)
		}
	}
}

// assertNakOnly checks the stream lane's settlement: not acknowledged and not terminated, so the
// message is delivered again.
func assertNakOnly(t *testing.T, msg *keyedIngestTestMsg) {
	t.Helper()
	if !msg.nak.Load() || msg.ack.Load() || msg.term.Load() {
		t.Fatalf("settlement after a failed hierarchy birth: nak %v, ack %v, term %v; want only a negative acknowledgement",
			msg.nak.Load(), msg.ack.Load(), msg.term.Load())
	}
}

// TestHierarchyFailureFailsTheBirth holds graph-entity-writes, "Birth with hierarchy fails
// closed" (design D21): with hierarchy on, an entity is born with its container edges or not at
// all. When the inference's store fails once, the entity is absent and the failure is transient;
// on the stream lane the message is not acknowledged, and its redelivery births the entity with
// its container edges. On the in-process lane the caller gets the transient error, and its retry
// births the entity.
func TestHierarchyFailureFailsTheBirth(t *testing.T) {
	t.Run("stream lane", func(t *testing.T) {
		c, bucket := hierarchyBirthComponent(t, true)

		first := &keyedIngestTestMsg{}
		if err := c.processIngest(t.Context(), 0, hierarchyBirthWork(first)); err == nil {
			t.Fatal("processIngest returned nil for a birth whose hierarchy inference failed")
		}
		assertNakOnly(t, first)
		assertEntityAbsent(t, bucket)

		redelivery := &keyedIngestTestMsg{}
		if err := c.processIngest(t.Context(), 0, hierarchyBirthWork(redelivery)); err != nil {
			t.Fatalf("redelivery: %v", err)
		}
		if !redelivery.ack.Load() {
			t.Fatal("the redelivery was not acknowledged")
		}
		assertBornWithContainers(t, c)
	})

	t.Run("in-process lane", func(t *testing.T) {
		c, bucket := hierarchyBirthComponent(t, true)

		err := c.CreateEntity(t.Context(), hierarchyBirth())
		if err == nil {
			t.Fatal("CreateEntity returned nil for a birth whose hierarchy inference failed")
		}
		if !errs.IsTransient(err) || errs.IsInvalid(err) || errs.IsFatal(err) {
			t.Fatalf("CreateEntity error is not classified transient: %v", err)
		}
		if !errors.Is(err, errInjectedRefusal) {
			t.Errorf("CreateEntity error does not carry the store's refusal: %v", err)
		}
		assertEntityAbsent(t, bucket)

		if err := c.CreateEntity(t.Context(), hierarchyBirth()); err != nil {
			t.Fatalf("CreateEntity retry: %v", err)
		}
		assertBornWithContainers(t, c)
	})

	// The stream lane asks storage whether the arrival is a birth before it runs the inference.
	// When that read fails, graph-ingest cannot tell a birth from an update, so it refuses the
	// arrival rather than risk a birth without hierarchy.
	t.Run("stream lane, the birth check cannot read", func(t *testing.T) {
		c, bucket := hierarchyBirthComponent(t, false)
		var failed atomic.Bool
		bucket.getFunc = func(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
			if key == hierarchyBirthID && failed.CompareAndSwap(false, true) {
				return nil, errInjectedRefusal
			}
			bucket.mu.Lock()
			defer bucket.mu.Unlock()
			if data, ok := bucket.data[key]; ok {
				return &mockKVEntry{data: data.value, revision: data.revision, key: key}, nil
			}
			return nil, jetstream.ErrKeyNotFound
		}

		first := &keyedIngestTestMsg{}
		if err := c.processIngest(t.Context(), 0, hierarchyBirthWork(first)); err == nil {
			t.Fatal("processIngest returned nil although the birth check could not read")
		}
		assertNakOnly(t, first)
		assertEntityAbsent(t, bucket)

		redelivery := &keyedIngestTestMsg{}
		if err := c.processIngest(t.Context(), 0, hierarchyBirthWork(redelivery)); err != nil {
			t.Fatalf("redelivery: %v", err)
		}
		if !redelivery.ack.Load() {
			t.Fatal("the redelivery was not acknowledged")
		}
		assertBornWithContainers(t, c)
	})
}
