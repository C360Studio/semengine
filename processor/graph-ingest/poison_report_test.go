package graphingest

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestHealthNamesTenPoisonedEntities records 11 poisoned entities through the query path,
// in an order unlike key order, and checks that Health names the count, the first 10 IDs in
// sorted order with their reasons and 1 more, and that the debug status lists all 11 sorted.
//
// spec: graph-ingest-recovery / graph-ingest reports its poisoned entities
func TestHealthNamesTenPoisonedEntities(t *testing.T) {
	c, bucket := poisonScopingTestComponent(t)
	// Suffixes chosen so insertion order, numeric order and key order all differ.
	suffixes := []string{"k", "c", "j", "a", "e", "i", "b", "h", "d", "g", "f"}
	for i, s := range suffixes {
		id := "acme.ops.test.system.widget.p" + s
		seedPoisonBytes(bucket, id, uint64(i+1))
		if _, err := queryEntity(t, c, id); err == nil {
			t.Fatalf("query of poisoned %s succeeded, want a refusal", id)
		}
	}

	sortedIDs := make([]string, 0, len(suffixes))
	for _, s := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"} {
		sortedIDs = append(sortedIDs, "acme.ops.test.system.widget.p"+s)
	}
	named := make([]string, 0, 10)
	for _, id := range sortedIDs[:10] {
		named = append(named, id+" (noncanonical_predicate)")
	}
	want := graph.ErrorCodeGraphStateResetRequired +
		": 11 poisoned entities (per-entity refusal; repair = delete + recreate): " +
		strings.Join(named, ", ") + " (+1 more; enumerate via debug status)"
	health := c.Health()
	if health.Healthy || health.Status != "degraded" {
		t.Fatalf("Health = healthy %v status %q, want degraded", health.Healthy, health.Status)
	}
	if !strings.Contains(health.LastError, want) {
		t.Fatalf("Health message = %q, want it to contain %q", health.LastError, want)
	}

	status, ok := c.DebugStatus().(entityPoisonDebugStatus)
	if !ok {
		t.Fatalf("DebugStatus() = %T, want entityPoisonDebugStatus", c.DebugStatus())
	}
	if status.PoisonedEntityCount != 11 {
		t.Fatalf("debug count = %d, want 11", status.PoisonedEntityCount)
	}
	listed := make([]string, 0, len(status.PoisonedEntities))
	for _, e := range status.PoisonedEntities {
		listed = append(listed, e.EntityID)
		if e.Reason != "noncanonical_predicate" || e.Error == "" {
			t.Fatalf("debug entry %+v, want reason noncanonical_predicate and an error", e)
		}
	}
	if !slices.Equal(listed, sortedIDs) {
		t.Fatalf("debug listing = %v, want %v", listed, sortedIDs)
	}
	if e := status.PoisonedEntities[0]; e.Revision != 4 {
		t.Fatalf("debug entry for %s at revision %d, want 4", e.EntityID, e.Revision)
	}
}

// TestGaugeFollowsConcurrentRecordAndClear reports and clears distinct keys from many
// goroutines at once. Once every goroutine has returned, the gauge must equal the number of
// keys left recorded, counted here from what each goroutine did: a count written late from
// an older size would leave the gauge wrong. Run it under -race.
//
// spec: graph-ingest-recovery / graph-ingest reports its poisoned entities
func TestGaugeFollowsConcurrentRecordAndClear(t *testing.T) {
	c, _ := poisonScopingTestComponent(t)
	const workers, perWorker = 16, 200
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range perWorker {
				id := fmt.Sprintf("acme.ops.test.system.widget.w%02d%04d", w, i)
				c.recordEntityPoison(&graph.StateContractError{
					Reason: graph.GraphStateReasonUnreadableEntity, EntityID: id,
				}, 5)
				if i%3 != 0 {
					c.dropEntityPoisonRecord(id, 5)
				}
			}
		})
	}
	wg.Wait() // every record and clear has returned
	left := 0
	for range workers {
		for i := range perWorker {
			if i%3 == 0 {
				left++
			}
		}
	}
	if got := testutil.ToFloat64(c.poisonedEntities); got != float64(left) {
		t.Fatalf("gauge = %v after the join, want %d keys left recorded", got, left)
	}
}
