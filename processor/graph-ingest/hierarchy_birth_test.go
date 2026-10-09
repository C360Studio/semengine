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

// refuseFirstContainer is the inference's store: graph-ingest's own adapter, except that the
// first container it is asked to create is refused, once, with an error classified invalid. A
// refusal the store classifies invalid must still leave the birth retryable (design D21).
type refuseFirstContainer struct {
	inference.EntityStore
	refused atomic.Bool
}

func (s *refuseFirstContainer) CreateEntity(ctx context.Context, entity *graph.EntityState) error {
	if s.refused.CompareAndSwap(false, true) {
		return errs.WrapInvalid(errInjectedRefusal, "refuseFirstContainer", "CreateEntity", "create container")
	}
	return s.EntityStore.CreateEntity(ctx, entity)
}

// hierarchyBirthComponent is graph-ingest over a mock bucket with hierarchy on, its inference
// built by the production constructor over the production adapter. With refuse set, the
// adapter refuses the first container's birth once.
func hierarchyBirthComponent(t *testing.T, refuse bool) (*Component, *mockKVBucket) {
	t.Helper()
	c, bucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
	c.config.EnableHierarchy = true
	var store inference.EntityStore = &hierarchyStore{component: c}
	if refuse {
		store = &refuseFirstContainer{EntityStore: store}
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
	return hierarchyBirthOf(hierarchyBirthID)
}

// hierarchyBirthOf is a fresh arrival for the entity id, a member of the same three containers
// when id differs from hierarchyBirthID only in its instance.
func hierarchyBirthOf(id string) *graph.EntityState {
	return &graph.EntityState{
		ID:          id,
		MessageType: testEntityType(),
		Triples: withTestMetadata(message.Triple{
			Subject: id, Predicate: hierarchyBirthPredicate, Object: "ready",
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
// containers, and each container is stored with no statement naming the entity: a birth writes
// no inverse edge (ruling M, #91 comment 6080973822).
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
	for _, container := range want {
		for _, tr := range storedEntity(t, c, container).Triples {
			if tr.Object == hierarchyBirthID {
				t.Errorf("container %s holds %s %v: a birth writes no inverse edge", container, tr.Predicate, tr.Object)
			}
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

	// The birth check finds the entity, so the arrival is an update and no inference runs; the
	// entity is then deleted before the write reads the key. A birth there would have no
	// hierarchy, so the write stores nothing and the arrival is delivered again; the redelivery's
	// birth check finds no entity, and the birth carries its container edges.
	t.Run("stream lane, the entity is deleted after the birth check", func(t *testing.T) {
		c, bucket := hierarchyBirthComponent(t, false)
		seedEntityState(t, c, hierarchyBirth())
		// The first read of the entity's key answers from the stored entity, and the key is
		// deleted before that read returns, so every later read finds it absent.
		var deleted atomic.Bool
		bucket.getFunc = func(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
			bucket.mu.Lock()
			defer bucket.mu.Unlock()
			data, ok := bucket.data[key]
			if !ok {
				return nil, jetstream.ErrKeyNotFound
			}
			if key == hierarchyBirthID && deleted.CompareAndSwap(false, true) {
				delete(bucket.data, key)
			}
			return &mockKVEntry{data: data.value, revision: data.revision, key: key}, nil
		}

		first := &keyedIngestTestMsg{}
		err := c.processIngest(t.Context(), 0, hierarchyBirthWork(first))
		if err == nil {
			t.Fatal("processIngest returned nil for an arrival whose entity was deleted after the birth check")
		}
		// The class is read from the error, not with errs.IsTransient, which also matches an
		// unclassified error by its text ("non-retryable" contains "retry").
		var classified *errs.ClassifiedError
		if !errors.As(err, &classified) || classified.Class != errs.ErrorTransient {
			t.Fatalf("processIngest error is not classified transient: %v", err)
		}
		assertNakOnly(t, first)
		bucket.mu.Lock()
		keys := len(bucket.data)
		bucket.mu.Unlock()
		if keys != 0 {
			t.Fatalf("the bucket holds %d keys after the refused write, want none", keys)
		}

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

// containerRevisions reads the stored revision of each container; each must be stored.
func containerRevisions(t *testing.T, bucket *mockKVBucket, containers []string) map[string]uint64 {
	t.Helper()
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	revisions := make(map[string]uint64, len(containers))
	for _, container := range containers {
		data, stored := bucket.data[container]
		if !stored {
			t.Fatalf("container %s is not stored", container)
		}
		revisions[container] = data.revision
	}
	return revisions
}

// TestMemberBirthKeepsContainerRevisions holds design D21's hierarchy paragraph (ruled M, #91
// comment 6080973822; #145): a birth writes no inverse edge into its containers, so an existing
// container's stored value does not change when a member is born. The first member's birth
// creates the three containers; the second member's birth, on each lane that infers hierarchy,
// leaves each container at the revision it had. Before ruling M each birth appended a contains
// edge to each container, so a container's value grew with every member until the bucket's value
// limit refused the append and every later birth under that container failed.
func TestMemberBirthKeepsContainerRevisions(t *testing.T) {
	const secondID = "acme.ops.robotics.gcs.drone.002"
	births := map[string]func(*testing.T, *Component){
		"stream lane": func(t *testing.T, c *Component) {
			msg := &keyedIngestTestMsg{}
			work := ingestWork{entity: hierarchyBirthOf(secondID), msg: msg, entityID: secondID, stream: "ENTITY", seq: 2}
			if err := c.processIngest(t.Context(), 0, work); err != nil {
				t.Fatalf("processIngest: %v", err)
			}
			if !msg.ack.Load() {
				t.Fatal("the second member's arrival was not acknowledged")
			}
		},
		"in-process lane": func(t *testing.T, c *Component) {
			if err := c.CreateEntity(t.Context(), hierarchyBirthOf(secondID)); err != nil {
				t.Fatalf("CreateEntity: %v", err)
			}
		},
	}
	containers := []string{hierarchyBirthType, hierarchyBirthTaxonomy, hierarchyBirthSource}

	for lane, birth := range births {
		t.Run(lane, func(t *testing.T) {
			c, bucket := hierarchyBirthComponent(t, false)
			if err := c.CreateEntity(t.Context(), hierarchyBirth()); err != nil {
				t.Fatalf("the first member's birth: %v", err)
			}
			before := containerRevisions(t, bucket, containers)

			birth(t, c)

			// The second member carries its membership edge to the type container, so the
			// inference ran for its birth.
			members := 0
			for _, tr := range storedEntity(t, c, secondID).Triples {
				if tr.Predicate == "hierarchy.type.member" && tr.Object == hierarchyBirthType {
					members++
				}
			}
			if members != 1 {
				t.Fatalf("the second member holds %d hierarchy.type.member edges to %s, want 1", members, hierarchyBirthType)
			}
			after := containerRevisions(t, bucket, containers)
			for _, container := range containers {
				if after[container] != before[container] {
					t.Errorf("container %s is at revision %d after a member's birth, want %d: the birth wrote to it",
						container, after[container], before[container])
				}
			}
		})
	}
}
