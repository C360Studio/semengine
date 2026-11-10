//go:build integration

package graphclustering

import (
	"fmt"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
)

// TestWatchOrderAcrossKeys is the probe #525 asks for (design D4, ADR-112): at the pinned nats.go
// and NATS server image, a watch of every ENTITY_STATES key, the one the follower opens
// (kvcatalog.CatalogReader.WatchAll), delivers revisions in ascending order across keys, in the
// replay of the stored keys and in the live writes after it. It is an observation at those
// versions, recorded in the change's notes/2.3.md, not a promise graph-clustering relies on: the
// follower declares the feed lost on a revision at or below one it has observed, and publication
// stops (TestLostFeedStopsPublication). A version bump that changes the order fails this probe.
func TestWatchOrderAcrossKeys(t *testing.T) {
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	keys := []string{"acme.ops.test.alpha.doc.a1", "acme.ops.test.beta.doc.b1", "acme.ops.test.alpha.doc.a2",
		"acme.ops.test.gamma.doc.c1"}
	// Interleaved rounds before the watch: the replay delivers each key's last write only.
	const rounds = 5
	for round := range rounds {
		for i, key := range keys {
			// The last round writes the keys in reverse, so key order and revision order differ.
			if round == rounds-1 {
				key = keys[len(keys)-1-i]
			}
			put(t, states, key, editedEntity(key, round))
		}
	}

	reader, err := kvcatalog.OpenCatalogReader(t.Context(), client, graph.BucketEntityStates)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketEntityStates, err)
	}
	watcher, err := reader.WatchAll(t.Context())
	if err != nil {
		t.Fatalf("watch %s: %v", graph.BucketEntityStates, err)
	}
	t.Cleanup(func() { _ = watcher.Stop() })

	var replay, live []string
	var revisions []uint64
	record := func(into *[]string, key string, rev uint64) {
		*into = append(*into, fmt.Sprintf("%s@%d", key[strings.LastIndex(key, ".")+1:], rev))
		revisions = append(revisions, rev)
	}
	for entry := range watcher.Updates() {
		if entry == nil {
			break // the end of the replay
		}
		record(&replay, entry.Key(), entry.Revision())
	}
	if len(replay) != len(keys) {
		t.Fatalf("replay = %v; want one entry per key, %d", replay, len(keys))
	}

	// Interleaved live writes, a delete among them.
	var wrote int
	for round := range rounds {
		for i, key := range keys {
			if round%2 == 1 {
				key = keys[len(keys)-1-i]
			}
			if round == 2 && i == 1 {
				if err := states.Delete(t.Context(), key); err != nil {
					t.Fatalf("delete %s: %v", key, err)
				}
			} else {
				put(t, states, key, editedEntity(key, rounds+round))
			}
			wrote++
		}
	}
	for range wrote {
		entry := <-watcher.Updates()
		record(&live, entry.Key(), entry.Revision())
	}

	t.Logf("replay order: %s", strings.Join(replay, " "))
	t.Logf("live order: %s", strings.Join(live, " "))
	for i := 1; i < len(revisions); i++ {
		if revisions[i] <= revisions[i-1] {
			t.Fatalf("revision %d delivered after %d; want ascending across keys (replay %v, live %v)",
				revisions[i], revisions[i-1], replay, live)
		}
	}
}
