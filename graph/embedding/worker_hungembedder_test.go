package embedding

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

// TestHungEmbedderFreesWorkerLane: an embedder that blocks a call until the call's context
// ends holds the lane for the call's 30-second deadline only. The record is then failed
// timeout, whatever error the embedder answers with, and the lane goes on to its next record,
// which the same embedder serves.
//
// Requirement: graph-embedding/An embedder that hangs frees the Worker's lane
func TestHungEmbedderFreesWorkerLane(t *testing.T) {
	cases := []struct {
		name string
		// answer is what the embedder returns once the hung call's context has ended.
		answer func(ctx context.Context) error
	}{
		{"context error", func(ctx context.Context) error { return ctx.Err() }},
		// A product embedder may answer its deadline with an error of its own that wraps no
		// context error and names no timeout.
		{"plain error", func(context.Context) error { return errors.New("embedder gave up") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				const (
					hungID   = "acme.ops.robotics.gcs.document.hung"
					servedID = "acme.ops.robotics.gcs.document.served"
				)
				embedder := ctxEmbedder(func(ctx context.Context, texts []string) ([][]float32, error) {
					if texts[0] == "the hung text" {
						<-ctx.Done()
						return nil, tc.answer(ctx)
					}
					return [][]float32{{1, 2, 3}}, nil
				})
				kv := newWatchKV()
				s := newTestStorage(t, kv, newMemKV())
				events := make(chan terminalEvent, 2)
				w := NewWorker(s, embedder, discardLogger()).
					WithWorkers(1).
					WithOnTerminal(func(_ context.Context, entityID string, revision uint64, outcome TerminalOutcome, reason string) {
						events <- terminalEvent{entityID, revision, outcome, reason}
					})
				stop := runWorker(t, w)
				synctest.Wait() // Run is reading its watch, so the two records arrive in write order

				ctx := t.Context()
				began := time.Now()
				if err := pendingText(ctx, s, hungID, "", "the hung text", 1); err != nil {
					t.Fatal(err)
				}
				if err := pendingText(ctx, s, servedID, "", "the served text", 2); err != nil {
					t.Fatal(err)
				}

				if got, want := <-events, (terminalEvent{hungID, 1, OutcomeFailed, failReasonTimeout}); got != want {
					t.Fatalf("first terminal outcome = %+v, want %+v", got, want)
				}
				if elapsed := time.Since(began); elapsed != 30*time.Second {
					t.Fatalf("the hung call ended after %v, want its 30s deadline", elapsed)
				}
				if got, want := <-events, (terminalEvent{servedID, 2, OutcomeGenerated, ""}); got != want {
					t.Fatalf("second terminal outcome = %+v, want %+v", got, want)
				}
				if elapsed := time.Since(began); elapsed != 30*time.Second {
					t.Fatalf("the second record was generated after %v, want 30s, at once after the first", elapsed)
				}
				// The terminal callback fires after the failed record is written.
				if rec, err := s.GetEmbedding(ctx, hungID); err != nil || rec == nil || rec.Status != StatusFailed {
					t.Fatalf("hung record = %+v, %v; want failed", rec, err)
				}
				stopWorker(t, stop)
			})
		})
	}
}

// ctxEmbedder is an Embedder whose Generate is the function itself, given the call's context.
type ctxEmbedder func(ctx context.Context, texts []string) ([][]float32, error)

func (f ctxEmbedder) Generate(ctx context.Context, texts []string) ([][]float32, error) {
	return f(ctx, texts)
}

func (f ctxEmbedder) GenerateQuery(ctx context.Context, texts []string) ([][]float32, error) {
	return f(ctx, texts)
}

func (ctxEmbedder) Dimensions() int { return 3 }
func (ctxEmbedder) Model() string   { return "m" }
func (ctxEmbedder) Close() error    { return nil }
