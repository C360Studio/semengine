//go:build integration

package graphingest

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// pauseOnRecord is a component logger handler, given through deps.Logger, that holds the goroutine logging one record
// (fix-301 design D2, "The pause"). On each record whose message is msg it closes entered (on the first) and blocks
// until Release; every other record passes at once and is dropped. graph-ingest logs the Debug record "Entity
// ingested" after the entity write and before the applied-sequence record and the acknowledgement, on the delivery
// work goroutine, so pausing on it holds an input applied and unacknowledged (probe 1.1, notes/1.1.md). It logs
// "query handlers registered" and "mutation handlers registered" on Start's goroutine once those subscriptions are
// live and before Start completes. A Stop while the pause is held waits for the held work, so a test releases the
// pause before Stop.
type pauseOnRecord struct {
	msg         string
	enterOnce   sync.Once
	entered     chan struct{}
	releaseOnce sync.Once
	release     chan struct{}
}

func newPauseOnRecord(msg string) *pauseOnRecord {
	return &pauseOnRecord{msg: msg, entered: make(chan struct{}), release: make(chan struct{})}
}

func (p *pauseOnRecord) Enabled(context.Context, slog.Level) bool { return true }

func (p *pauseOnRecord) Handle(_ context.Context, record slog.Record) error {
	if record.Message != p.msg {
		return nil
	}
	p.enterOnce.Do(func() { close(p.entered) })
	<-p.release
	return nil
}

func (p *pauseOnRecord) WithAttrs([]slog.Attr) slog.Handler { return p }
func (p *pauseOnRecord) WithGroup(string) slog.Handler      { return p }

// Release lets every held and later record through. It may be called more than once.
func (p *pauseOnRecord) Release() { p.releaseOnce.Do(func() { close(p.release) }) }

// awaitEntered waits until a goroutine is held on the record, failing the test when bound passes first.
func (p *pauseOnRecord) awaitEntered(ctx context.Context, t *testing.T, bound time.Duration) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	select {
	case <-p.entered:
	case <-waitCtx.Done():
		t.Fatalf("graph-ingest logged no %q within %s", p.msg, bound)
	}
}
