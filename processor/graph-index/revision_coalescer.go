package graphindex

import (
	"context"
	"sync"
	"time"
)

type coalescedEntity struct {
	entityID string
	revision uint64
}

// revisionCoalescer keeps the greatest delivered revision of every pending key and hands
// the pending set to its callback once per window. Keeping the revision with the key lets
// the lane complete exactly the revisions a batch stands for, never a newer one delivered
// after the batch was taken.
//
// It is background work in the Run(ctx) shape (background-work spec): run holds the
// ticker on the caller's goroutine and returns when ctx ends. Keys still pending then are
// not handed over; their revisions stay incomplete in the watermark.
type revisionCoalescer struct {
	window   time.Duration
	callback func(context.Context, []coalescedEntity)

	mu      sync.Mutex
	pending map[string]uint64
}

func newRevisionCoalescer(window time.Duration, callback func(context.Context, []coalescedEntity)) *revisionCoalescer {
	return &revisionCoalescer{window: window, callback: callback, pending: make(map[string]uint64)}
}

// Add records revision for key, keeping the greater of it and any pending revision.
func (c *revisionCoalescer) Add(key string, revision uint64) {
	c.mu.Lock()
	if revision > c.pending[key] {
		c.pending[key] = revision
	}
	c.mu.Unlock()
}

// Remove drops key's pending revision: a delete supersedes it.
func (c *revisionCoalescer) Remove(key string) {
	c.mu.Lock()
	delete(c.pending, key)
	c.mu.Unlock()
}

// run hands the pending set to the callback once per window until ctx ends.
func (c *revisionCoalescer) run(ctx context.Context) {
	ticker := time.NewTicker(c.window)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.fire(ctx)
		}
	}
}

func (c *revisionCoalescer) fire(ctx context.Context) {
	c.mu.Lock()
	if len(c.pending) == 0 {
		c.mu.Unlock()
		return
	}
	pending := c.pending
	c.pending = make(map[string]uint64)
	c.mu.Unlock()

	batch := make([]coalescedEntity, 0, len(pending))
	for key, revision := range pending {
		batch = append(batch, coalescedEntity{entityID: key, revision: revision})
	}
	c.callback(ctx, batch)
}
