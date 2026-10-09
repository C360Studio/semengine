// Attack tests — permanent CI fixtures.

package inference

import (
	"context"
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

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("timeout: concurrent isContainerEntity calls hung")
	}
}

// TestAttack_AddToContainers_Concurrent verifies that AddToContainers
// is safe to call concurrently with container and real entities.
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
			_ = addHierarchy(context.Background(), hi, store, entityID)
		}(g)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - verify no panic and reasonable results
		createdEntities := store.getCreatedEntities()

		// Real entities create containers, so we should have some
		// But containers themselves should NOT create additional containers
		// With 3 real entities (2 temp, 1 pressure) in 50 goroutines:
		// - temp-001 and temp-002 share same type container (temperature.group)
		// - press-001 has its own type container (pressure.group)
		// - Each real entity creates 3 levels (type, system, domain)
		// - Max containers: 4 unique type containers + 1 system + 1 domain = 6
		assert.LessOrEqual(t, len(createdEntities), 6,
			"Should create bounded containers, got %d", len(createdEntities))

		// The key test: verify that if we processed 50 goroutines with some being
		// container entities, those container entities did NOT create additional containers.
		// We can verify this by checking the container count is bounded and matches
		// what real entities would create (not exponential growth).

	case <-time.After(10 * time.Second):
		t.Fatal("timeout: concurrent AddToContainers calls hung")
	}
}

// TestAttack_CancelledContext verifies that operations respect cancelled context.
func TestAttack_CancelledContext(t *testing.T) {
	store := newFakeStore()

	hi := newTestInference(t, store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	done := make(chan struct{})
	go func() {
		defer close(done)
		// Should not hang on cancelled context
		_ = addHierarchy(ctx, hi, store, "c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-001")
	}()

	select {
	case <-done:
		// Success - operation completed without hanging
	case <-time.After(5 * time.Second):
		t.Fatal("AddToContainers hung on cancelled context")
	}
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

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Each container was created once, by the create that won.
		createdEntities := store.getCreatedEntities()
		assert.Len(t, createdEntities, 3,
			"each of the three containers is created once despite concurrent births")

	case <-time.After(10 * time.Second):
		t.Fatal("timeout: concurrent births of one entity hung")
	}
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

// TestAttack_LargeEntityBurst verifies handling of many entities at once.
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
			// Different types to create different containers
			entityID := "c360.semstreams-hierarchy-test.sensor.environmental.temperature.temp-" + string(rune('A'+idx%26))
			_ = addHierarchy(context.Background(), hi, store, entityID)
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		elapsed := time.Since(start)
		t.Logf("Processed %d entities in %v", entityCount, elapsed)

		// Verify bounded container creation
		createdEntities := store.getCreatedEntities()
		assert.LessOrEqual(t, len(createdEntities), 78, // 26 types * 3 levels
			"Should create bounded containers, got %d", len(createdEntities))

	case <-time.After(30 * time.Second):
		t.Fatal("timeout: large entity burst took too long")
	}
}

// TestAttack_ManyBirthsOfOneTypeCreateThreeContainers: ten thousand births of
// one type, one after another, each ask storage for the type's three containers;
// the first creates them and every later birth finds them, so the store saw three
// creates. The inference keeps nothing per birth (#130).
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
	assert.Len(t, store.createCalls(), 3, "no birth after the first asked for a create")
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
