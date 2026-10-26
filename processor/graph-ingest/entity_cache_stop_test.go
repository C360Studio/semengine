package graphingest

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/internal/cache"
)

// TestEntityCacheUseDuringStop proves #306's fix (design D7): a batch query and a
// CreateEntity that use the entity query cache while Stop closes and clears it neither
// panic nor race with the clear. The batch query reads the cache (a hit), and on its miss
// reads the bucket and repopulates the cache; CreateEntity invalidates its key. Those are
// the three cache uses D7 lists.
//
// The readers start once Stop is under way (it has drained the consumer and waits for
// it to report closed, so the cache is neither closed nor cleared yet), and the test then
// lets Stop finish while they run. Nothing orders a reader's use of the cache handle
// against Stop's clear, on purpose: a channel between them would be a happens-before
// edge, and the race detector reports only accesses no such edge orders. Every
// interleaving of an unsynchronized read and the clear is a race, so at the base this
// test fails under -race whichever runs first. Run it with -race; without it the test
// can pass at the base.
//
// spec: graph-ingest-recovery / The entity cache stays usable through Stop
func TestEntityCacheUseDuringStop(t *testing.T) {
	c := createTestComponentWithMockKV(t)

	// The cache initStorage builds, with its expiry sweep; Stop's cleanup closes it.
	entityCache, err := cache.NewFromConfig[graph.EntityState](t.Context(), cache.Config{
		Enabled:         true,
		Strategy:        cache.StrategyHybrid,
		MaxSize:         5000,
		TTL:             30 * time.Second,
		CleanupInterval: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("entity cache: %v", err)
	}
	c.entityCache = entityCache

	const (
		cachedID  = "c360.platform.robotics.mav1.drone.001"
		missedID  = "c360.platform.robotics.mav1.drone.002"
		createdID = "c360.platform.robotics.mav1.drone.003"
	)
	for _, id := range []string{cachedID, missedID} {
		if err := c.CreateEntity(t.Context(), newTestEntity(id)); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	entry, err := c.entityBucket.Get(t.Context(), cachedID)
	if err != nil {
		t.Fatalf("read seeded %s: %v", cachedID, err)
	}
	var cachedEntity graph.EntityState
	if err := json.Unmarshal(entry.Value, &cachedEntity); err != nil {
		t.Fatalf("decode seeded %s: %v", cachedID, err)
	}
	if _, err := entityCache.Set(cachedID, cachedEntity); err != nil {
		t.Fatalf("warm the cache: %v", err)
	}

	consumer := &graphIngestLifecycleConsumeContext{
		closed: make(chan struct{}), drainSeen: make(chan struct{}),
	}
	c.consumers = []graphIngestConsumerBinding{{handle: consumer}}
	startGuardAcquiringNothing(t, c)

	stopResult := make(chan error, 1)
	go func() { stopResult <- c.Stop(t.Context()) }()
	// Stop has drained the consumer and waits on its Closed; the cache is still open and set.
	<-consumer.drainSeen

	request, err := json.Marshal(graph.EntityBatchRequest{IDs: []string{cachedID, missedID}})
	if err != nil {
		t.Fatalf("encode batch request: %v", err)
	}
	var (
		readers   sync.WaitGroup
		batch     graph.EntityBatchResponse
		batchErr  error
		createErr error
	)
	readers.Go(func() {
		reply, err := c.handleQueryBatchNATS(t.Context(), request)
		if err != nil {
			batchErr = err
			return
		}
		batchErr = json.Unmarshal(reply, &batch)
	})
	readers.Go(func() {
		createErr = c.CreateEntity(t.Context(), newTestEntity(createdID))
	})
	close(consumer.closed) // Stop now closes and clears the cache while the readers run.

	readers.Wait() // orders the reads of batch, batchErr and createErr below
	if err := <-stopResult; err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if batchErr != nil {
		t.Fatalf("batch query during Stop: %v", batchErr)
	}
	got := map[string]bool{}
	for _, entity := range batch.Entities {
		got[entity.ID] = true
	}
	if len(batch.Entities) != 2 || !got[cachedID] || !got[missedID] || len(batch.Missing) != 0 {
		t.Fatalf("batch query during Stop = %d entities %v, missing %v; want %s and %s, none missing",
			len(batch.Entities), got, batch.Missing, cachedID, missedID)
	}
	if createErr != nil {
		t.Fatalf("CreateEntity during Stop: %v", createErr)
	}
}
