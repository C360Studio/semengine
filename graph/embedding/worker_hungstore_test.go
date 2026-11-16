package embedding

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/c360studio/semengine/storage"
)

// TestHungStoreFreesWorkerLane: a body store that blocks a read until the read's context ends,
// then answers with an error of its own that wraps no context error (as the NATS object store
// does at its deadline), holds the lane for the read's 10-second deadline only. The record is
// then failed content_error, as any body read failure is, and the lane goes on to its next
// record, whose body the same store serves.
//
// Requirement: graph-embedding/A body store that hangs frees the Worker's lane
func TestHungStoreFreesWorkerLane(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			hungID   = "acme.ops.robotics.gcs.document.hung"
			servedID = "acme.ops.robotics.gcs.document.served"
		)
		store := readerFunc(func(ctx context.Context, key string) (io.ReadCloser, error) {
			if key == "body/hung" {
				<-ctx.Done()
				return nil, errors.New("nats: timeout")
			}
			return io.NopCloser(strings.NewReader("the served body")), nil
		})
		kv := newWatchKV()
		s := newTestStorage(t, kv, newMemKV())
		events := make(chan terminalEvent, 2)
		w := NewWorker(s, fixedEmbedder(), discardLogger()).
			WithWorkers(1).
			WithStoreResolver(fakeResolver{stores: map[string]storage.StreamableStore{"objstore": store}}).
			WithOnTerminal(func(_ context.Context, entityID string, revision uint64, outcome TerminalOutcome, reason string) {
				events <- terminalEvent{entityID, revision, outcome, reason}
			})
		stop := runWorker(t, w)
		synctest.Wait() // Run is reading its watch, so the two records arrive in write order

		ctx := t.Context()
		began := time.Now()
		if err := pendingRef(ctx, s, hungID, "", "", &StorageRef{StorageInstance: "objstore", Key: "body/hung"}, 1); err != nil {
			t.Fatal(err)
		}
		if err := pendingRef(ctx, s, servedID, "", "", &StorageRef{StorageInstance: "objstore", Key: "body/served"}, 2); err != nil {
			t.Fatal(err)
		}

		if got, want := <-events, (terminalEvent{hungID, 1, OutcomeFailed, failReasonContentError}); got != want {
			t.Fatalf("first terminal outcome = %+v, want %+v", got, want)
		}
		if elapsed := time.Since(began); elapsed != 10*time.Second {
			t.Fatalf("the hung read ended after %v, want its 10s deadline", elapsed)
		}
		if got, want := <-events, (terminalEvent{servedID, 2, OutcomeGenerated, ""}); got != want {
			t.Fatalf("second terminal outcome = %+v, want %+v", got, want)
		}
		if elapsed := time.Since(began); elapsed != 10*time.Second {
			t.Fatalf("the second record was generated after %v, want 10s, at once after the first", elapsed)
		}
		if rec, err := s.GetEmbedding(ctx, hungID); err != nil || rec == nil || rec.Status != StatusFailed {
			t.Fatalf("hung record = %+v, %v; want failed", rec, err)
		}
		stopWorker(t, stop)
	})
}

// readerFunc is a streamable store whose Open is the function itself; nothing else is read.
type readerFunc func(ctx context.Context, key string) (io.ReadCloser, error)

func (readerFunc) Put(context.Context, string, []byte) error      { return nil }
func (readerFunc) Get(context.Context, string) ([]byte, error)    { return nil, errors.New("not read") }
func (readerFunc) List(context.Context, string) ([]string, error) { return nil, nil }
func (readerFunc) Delete(context.Context, string) error           { return nil }
func (f readerFunc) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return f(ctx, key)
}
