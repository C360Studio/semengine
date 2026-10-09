// Attack tests — permanent CI fixtures.

package inference

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAttack_IsContainerEntity_Concurrent verifies that isContainerEntity
// is safe to call concurrently from multiple goroutines.
func TestAttack_IsContainerEntity_Concurrent(t *testing.T) {
	testCases := []struct {
		entityID        string
		wantIsContainer bool
	}{
		{"c360.semstreams-hierarchy-test.sensor.environmental.temperature.group", true},
		{"c360.semstreams-hierarchy-test.sensor.environmental.group.container", true},
		{"c360.semstreams-hierarchy-test.sensor.group.container.level", true},
		{"c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-001", false},
		{"", false},
		{"a.b.c.d.e", false},
		{"a.b.c.d.e.f.g", false},
	}

	const goroutines = 100
	const iterations = 1000

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				for _, tc := range testCases {
					got := isContainerEntity(tc.entityID)
					if got != tc.wantIsContainer {
						t.Errorf("isContainerEntity(%q) = %v, want %v", tc.entityID, got, tc.wantIsContainer)
					}
				}
			}
		}()
	}
	wg.Wait()
}

// TestAttack_AddToContainers_Concurrent: fifty births at once, of three entities
// and of three IDs that are containers. Every birth succeeds, and a container's
// birth adds it to no container, so the store ends holding the six IDs born and
// the one container no birth names (pressure's type container), and nothing else.
func TestAttack_AddToContainers_Concurrent(t *testing.T) {
	store := newFakeStore()

	hi := newTestInference(t, store)

	entities := []string{
		// Real entities
		"c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-001",
		"c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-002",
		"c360.semstreams-hierarchy-test.sensor.environmental.pressure.press-001",
		// Container entities (should be skipped)
		"c360.semstreams-hierarchy-test.sensor.environmental.temperature.group",
		"c360.semstreams-hierarchy-test.sensor.environmental.group.container",
		"c360.semstreams-hierarchy-test.sensor.group.container.level",
	}

	const goroutines = 50
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			entityID := entities[idx%len(entities)]
			assert.NoError(t, addHierarchy(context.Background(), hi, store, entityID))
		}(g)
	}
	wg.Wait()

	// Seven: the three entities and their four containers, which are the type
	// containers of temperature and of pressure, and the taxonomy and source
	// containers all three share. If a container's birth added it to containers,
	// the store would hold more: the source container's birth alone would add two.
	//
	// The inference's own creates are not counted: a container's birth here puts
	// it in the store as another writer would, racing the inference's create, so
	// how many of the inference's creates commit (one to four) depends on the
	// schedule. What the store holds does not.
	assert.ElementsMatch(t, []string{
		"c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-001",
		"c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-002",
		"c360.semstreams-hierarchy-test.sensor.environmental.pressure.press-001",
		"c360.semstreams-hierarchy-test.sensor.environmental.temperature.group",
		"c360.semstreams-hierarchy-test.sensor.environmental.pressure.group",
		"c360.semstreams-hierarchy-test.sensor.environmental.group.container",
		"c360.semstreams-hierarchy-test.sensor.group.container.level",
	}, store.heldIDs(), "the entities born and their four containers, and nothing else")
}

// TestAttack_CancelledContext verifies that operations respect cancelled context.
func TestAttack_CancelledContext(t *testing.T) {
	store := newFakeStore()

	hi := newTestInference(t, store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Should not hang on a cancelled context; go test -timeout bounds the call.
	_ = addHierarchy(ctx, hi, store, "c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-001")
}

// TestAttack_EdgeCaseInputs verifies handling of unusual input strings.
func TestAttack_EdgeCaseInputs(t *testing.T) {
	testCases := []struct {
		name     string
		entityID string
	}{
		{"empty string", ""},
		{"single character", "a"},
		{"single dot", "."},
		{"leading dot", ".a.b.c.d.e.f"},
		{"trailing dot", "a.b.c.d.e.f."},
		{"multiple trailing dots", "a.b.c.d.e.group..."},
		{"double dots", "a..b.c.d.e.group"},
		{"very long ID", strings.Repeat("a.", 1000) + "group"},
		{"unicode", "c360.semstreams-hierarchy-test.环境.sensor.温度.group"},
		{"special chars", "c360.semstreams-hierarchy-test.env$ironmental.sensor.temp@.group"},
		{"newline", "a.b.c.d.e\n.group"},
		{"null byte", "a.b.c.d.e\x00.group"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Should not panic
			require.NotPanics(t, func() {
				_ = isContainerEntity(tc.entityID)
			}, "isContainerEntity panicked on %q", tc.entityID)

			// Should handle gracefully in AddToContainers
			store := newFakeStore()
			hi := newTestInference(t, store)

			require.NotPanics(t, func() {
				_ = addHierarchy(context.Background(), hi, store, tc.entityID)
			}, "AddToContainers panicked on %q", tc.entityID)
		})
	}
}

// TestAttack_ConcurrentBirthsCreateEachContainerOnce: many births of one entity
// at once each ask storage for the entity's three containers, so several can
// find one absent and race to create it. The store's create is atomic and refuses
// every loser with natsclient.ErrKVKeyExists, which the inference takes as the
// container existing: every birth succeeds and each container is created once.
func TestAttack_ConcurrentBirthsCreateEachContainerOnce(t *testing.T) {
	store := newFakeStore()

	hi := newTestInference(t, store)

	// Create same entity from multiple goroutines
	const goroutines = 100
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, addHierarchy(context.Background(), hi, store, "c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-001"))
		}()
	}
	wg.Wait()

	// Each container was created once, by the create that won.
	createdEntities := store.getCreatedEntities()
	assert.Len(t, createdEntities, 3,
		"each of the three containers is created once despite concurrent births")
}

// TestAttack_ContainerEntityVariants verifies all container suffix variants
// are correctly identified under concurrent stress.
func TestAttack_ContainerEntityVariants(t *testing.T) {
	variants := []struct {
		entityID        string
		wantIsContainer bool
	}{
		// Valid containers
		{"a.b.c.d.e.group", true},
		{"a.b.c.d.e.container", true},
		{"a.b.c.d.e.level", true},

		// Invalid: wrong part count
		{"a.b.c.d.group", false},     // 5 parts
		{"a.b.c.d.e.f.group", false}, // 7 parts
		{"group", false},             // 1 part
		{"a.b.c.group", false},       // 4 parts

		// Invalid: suffix in wrong position
		{"a.group.c.d.e.f", false},
		{"a.b.container.d.e.f", false},
		{"a.b.c.level.e.f", false},

		// Invalid: similar but not exact match
		{"a.b.c.d.e.groups", false},
		{"a.b.c.d.e.grouped", false},
		{"a.b.c.d.e.containers", false},
		{"a.b.c.d.e.levels", false},
		{"a.b.c.d.e.GROUP", false}, // Case sensitive
	}

	const goroutines = 50
	const iterations = 100

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				for _, v := range variants {
					got := isContainerEntity(v.entityID)
					if got != v.wantIsContainer {
						t.Errorf("isContainerEntity(%q) = %v, want %v", v.entityID, got, v.wantIsContainer)
					}
				}
			}
		}()
	}

	wg.Wait()
}

// TestAttack_LargeEntityBurst: a thousand births at once, of 26 instances of one
// type. Every birth succeeds and the type's three containers are each created
// once.
func TestAttack_LargeEntityBurst(t *testing.T) {
	store := newFakeStore()

	hi := newTestInference(t, store)

	const entityCount = 1000
	var wg sync.WaitGroup

	start := time.Now()
	for i := 0; i < entityCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// temp-A to temp-Z: 26 instances, all of type temperature
			entityID := "c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-" + string(rune('A'+idx%26))
			assert.NoError(t, addHierarchy(context.Background(), hi, store, entityID))
		}(i)
	}
	wg.Wait()
	t.Logf("Processed %d entities in %v", entityCount, time.Since(start))

	// Three: the 26 instances share one type, so one type container, one
	// taxonomy container and one source container. Births that find one absent
	// race to create it, but the store's create is atomic and refuses every create
	// after the first with natsclient.ErrKVKeyExists, so each commits once.
	assert.Len(t, store.getCreatedEntities(), 3, "one type's three containers, each created once")
}

// TestAttack_ManyBirthsOfOneTypeCreateThreeContainers: ten thousand births of
// one type, one after another, each ask storage for the type's three containers
// through the create; the first creates them and every later birth's create is
// refused because they exist, so three creates commit. The inference keeps
// nothing per birth (#130).
func TestAttack_ManyBirthsOfOneTypeCreateThreeContainers(t *testing.T) {
	store := newFakeStore()

	hi := newTestInference(t, store)

	// 10,000 births of one type: 130 instances, each born many times
	const entityCount = 10000
	for i := 0; i < entityCount; i++ {
		entityID := "c360.semstreams-hierarchy-test.sensor.environmental.temp.instance-" + string(rune('A'+i%26)) + string(rune('0'+i%10))
		require.NoError(t, addHierarchy(context.Background(), hi, store, entityID))
	}

	createdEntities := store.getCreatedEntities()
	assert.Len(t, createdEntities, 3, "one type's three containers, each created once")
	assert.Len(t, store.createCalls(), 3*entityCount, "every birth asked storage for each container")
}

// TestAttack_GoroutineCount checks that AddToContainers leaves no
// goroutine running. synctest.Test returns only once every goroutine started
// in its bubble has exited, and fails the test as a deadlock when one is left
// blocked, so the test needs no wait for cleanup and no goroutine count.
func TestAttack_GoroutineCount(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newFakeStore()

		hi := newTestInference(t, store)

		// Create 100 entities
		for i := 0; i < 100; i++ {
			entityID := "c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-" + string(rune('A'+i%26))
			require.NoError(t, addHierarchy(t.Context(), hi, store, entityID))
		}
	})
}

// heldIDs returns the ID of every entity the store holds.
func (s *fakeStore) heldIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Collect(maps.Keys(s.entities))
}
