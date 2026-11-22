package graphingest

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/c360studio/semengine/graph"
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

// TestHealthCountMatchesNamedIDsWhileChanging reads Health while one key is reported and
// repaired over and over, with a second key recorded throughout. With fewer than 10 recorded,
// every message must name exactly as many IDs as its count and claim none more: a count and a
// sample taken from two reads disagree when a report or repair lands between them. The anchor
// key keeps a poison message in every Health, so the assertion runs on each read.
//
// On one processor the writer lands between the two reads only when the scheduler happens to
// switch there, which is rare. The gates run at -cpu 1, so the test raises GOMAXPROCS to at
// least 2 for its own run: writer and reader then run in parallel and the clash shows on most
// runs. It must not call t.Parallel, because GOMAXPROCS is process-wide.
//
// spec: graph-ingest-recovery / graph-ingest reports its poisoned entities
func TestHealthCountMatchesNamedIDsWhileChanging(t *testing.T) {
	prev := runtime.GOMAXPROCS(max(2, runtime.GOMAXPROCS(0)))
	t.Cleanup(func() { runtime.GOMAXPROCS(prev) })
	c, _ := poisonScopingTestComponent(t)
	const (
		anchor = "acme.ops.test.system.widget.anchor"
		id     = "acme.ops.test.system.widget.flicker"
		rounds = 100000
	)
	reason := graph.GraphStateReasonUnreadableEntity
	c.recordEntityPoison(&graph.StateContractError{Reason: reason, EntityID: anchor}, 1)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		<-start
		for i := range rounds {
			c.recordEntityPoison(&graph.StateContractError{Reason: reason, EntityID: id}, uint64(i+1))
			c.dropEntityPoisonRecord(id, uint64(i+1))
		}
	})
	defer wg.Wait() // joins the writer even when an assertion fails
	close(start)
	prefix := graph.ErrorCodeGraphStateResetRequired + ": "
	named := " (" + string(reason) + ")"
	for range rounds {
		msg := c.Health().LastError
		var count int
		if _, err := fmt.Sscanf(msg, prefix+"%d poisoned", &count); err != nil {
			t.Fatalf("Health message %q has no poison count: %v", msg, err)
		}
		if listed := strings.Count(msg, named); listed != count || strings.Contains(msg, "more;") {
			t.Fatalf("Health count %d, %d IDs named: %q", count, listed, msg)
		}
	}
}
