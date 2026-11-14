package graphlexical

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/c360studio/semengine/graph/entitytext"
)

// repairInterval is how often a repair pass reads again the bodies left unread or with no store.
const repairInterval = 30 * time.Second

// bodyPath reads every offloaded body on one goroutine, so the follower never waits for a store.
// A read that fails on a store, other than with a missing object, skips that store until the next
// repair pass: the bodies queued on it are left unread without a read, so a store that blocks
// costs one read deadline per pass, whatever the number of bodies on it.
type bodyPath struct {
	apply  *applier
	stores entitytext.Stores // nil when no store is registered anywhere
	maxLen int
	logger *slog.Logger

	mu    sync.Mutex
	queue []bodyRead
	wake  chan struct{} // holds one signal while the queue has reads run has not taken
}

// newBodyPath returns a body path that reads from stores, up to maxLen characters, and gives each
// outcome to apply.
func newBodyPath(apply *applier, stores entitytext.Stores, maxLen int, logger *slog.Logger) *bodyPath {
	return &bodyPath{apply: apply, stores: stores, maxLen: maxLen, logger: logger, wake: make(chan struct{}, 1)}
}

// enqueue queues reads for run and returns at once.
func (b *bodyPath) enqueue(reads ...bodyRead) {
	if len(reads) == 0 {
		return
	}
	b.mu.Lock()
	b.queue = append(b.queue, reads...)
	b.mu.Unlock()
	select {
	case b.wake <- struct{}{}:
	default:
	}
}

// run reads queued bodies until ctx ends, and every repairInterval queues again the bodies left
// unread or with no store. A pass ends before the next begins, since one goroutine runs both.
// A read in flight when ctx ends returns by its own deadline at the latest, and its outcome is
// discarded.
func (b *bodyPath) run(ctx context.Context) {
	repair := time.NewTicker(repairInterval)
	defer repair.Stop()
	// skipped holds each store a read failed on since the last repair pass, with that failure.
	skipped := make(map[string]error)
	for {
		select {
		case <-ctx.Done():
			return
		case <-b.wake:
		case <-repair.C:
			clear(skipped)
			b.enqueue(b.apply.retries()...)
		}
		b.read(ctx, skipped)
	}
}

// read makes every read queued now, in order. A read for a revision the key has passed is not
// made; a read on a skipped store is not made and leaves the body unread.
func (b *bodyPath) read(ctx context.Context, skipped map[string]error) {
	b.mu.Lock()
	batch := b.queue
	b.queue = nil
	b.mu.Unlock()
	for _, read := range batch {
		if ctx.Err() != nil {
			return
		}
		if !b.apply.wanted(read) {
			continue
		}
		instance := read.ref.StorageInstance
		if cause, ok := skipped[instance]; ok {
			b.apply.bodyDone(read, "", cause)
			continue
		}
		body, err := entitytext.ReadBody(ctx, b.stores, read.ref, b.maxLen)
		if ctx.Err() != nil {
			return
		}
		if readState(err) == bodyUnread {
			skipped[instance] = err
			b.logger.Warn("body read failed; its store is skipped until the next repair pass",
				slog.String("storage_instance", instance), slog.String("entity", read.key), slog.Any("error", err))
		}
		b.apply.bodyDone(read, body, err)
	}
}
