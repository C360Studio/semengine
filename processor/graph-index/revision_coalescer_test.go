package graphindex

import (
	"context"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// TestCoalescerHandsOverTheGreatestRevisionPerKey: inside one window the coalescer keeps each
// key's greatest revision, whatever order the revisions arrive in, drops a removed key, and hands
// the set over once per window; run returns when its context ends.
func TestCoalescerHandsOverTheGreatestRevisionPerKey(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var batches [][]coalescedEntity
		c := newRevisionCoalescer(time.Second, func(_ context.Context, batch []coalescedEntity) {
			slices.SortFunc(batch, func(a, b coalescedEntity) int { return strings.Compare(a.entityID, b.entityID) })
			batches = append(batches, batch)
		})
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() { c.run(ctx); close(done) }()

		c.Add("a", 3)
		c.Add("a", 7)
		c.Add("a", 5) // lower than the pending 7: kept at 7
		c.Add("b", 2)
		c.Add("gone", 9)
		c.Remove("gone")
		<-time.After(time.Second) // the bubble's fake clock: one window
		synctest.Wait()           // the callback has returned
		want := []coalescedEntity{{"a", 7}, {"b", 2}}
		if len(batches) != 1 || !slices.Equal(batches[0], want) {
			t.Fatalf("batches after one window = %v, want [%v]", batches, want)
		}

		<-time.After(time.Second) // an empty window hands over nothing
		synctest.Wait()
		if len(batches) != 1 {
			t.Fatalf("an empty window handed over %v", batches[1:])
		}

		cancel()
		<-done
	})
}
