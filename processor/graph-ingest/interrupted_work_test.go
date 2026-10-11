package graphingest

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
)

// interruptedEntity is the entity whose application the interrupted-work tests interrupt, and
// nextEntity the one the same lane takes next, under the acme.ops authority of their component.
const (
	interruptedEntity = "acme.ops.test.system.widget.interrupted"
	nextEntity        = "acme.ops.test.system.widget.next"
)

// interruptedWorkCauses are the causes work_interrupted_total counts under.
var interruptedWorkCauses = []string{"panic", "heartbeat_failed", "invalid"}

// interruptedInput is an input for id at seq, delivered as msg, from a consumer with a 2-second
// acknowledgement wait, so its in-progress interval is half a second.
func interruptedInput(id string, seq uint64, msg *keyedIngestTestMsg) ingestWork {
	return ingestWork{
		entity: &graph.EntityState{ID: id, MessageType: testEntityType(), Triples: withTestMetadata(message.Triple{
			Subject: id, Predicate: "test.state.value", Object: "applied", Timestamp: fixtureTime, Confidence: 1.0,
		})},
		msg:       msg,
		consumer:  natsclient.StreamConsumerConfig{StreamName: "ENTITY", AckWait: 2 * time.Second},
		heartbeat: 500 * time.Millisecond,
		entityID:  id,
		stream:    "ENTITY",
		seq:       seq,
	}
}

// entityStored reports whether bucket holds a key for id.
func entityStored(bucket *mockKVBucket, id string) bool {
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	_, ok := bucket.data[id]
	return ok
}

// TestInterruptedApplicationIsRedeliveredNotAStop holds graph-ingest-recovery, "An interrupted
// application is redelivered, not a stop" (design D8, "Long apply"; #91 comment 6109278240, item 1).
// When the delivery helper reports that the owner must stop (the application panicked, an
// in-progress signal could not be sent, or the delivery has no metadata), the input is left
// unsettled, the error count Health reads and the interrupted-work count under that cause each rise
// by one, one ERROR line names the cause, and the next input on the same lane is applied and
// acknowledged. The lane is one lane of a keyed pool on a context of its own, as Start runs it.
// The last case holds the design's "At Stop": an input that reaches its lane after the lane's
// context has ended is negatively acknowledged, applies nothing, and is logged at WARN.
func TestInterruptedApplicationIsRedeliveredNotAStop(t *testing.T) {
	cases := []struct {
		name  string
		cause string
		// arrange interrupts the first input's application, through the applied-sequence bucket or
		// its message.
		arrange func(it *interruption)
		// check holds what else the cause leaves behind, once the first input has been handled.
		check func(t *testing.T, it *interruption)
	}{
		{
			name:  "application panics",
			cause: "panic",
			arrange: func(it *interruption) {
				it.guard.getFunc = func(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
					if key == guardKey(interruptedEntity, "ENTITY") {
						panic("test: the application panics")
					}
					return nil, jetstream.ErrKeyNotFound
				}
			},
		},
		{
			name:  "in-progress signal fails",
			cause: "heartbeat_failed",
			arrange: func(it *interruption) {
				it.msg.inProgressErr = errors.New("test: the in-progress signal is refused")
				it.signalled = make(chan struct{})
				it.msg.onInProgress = func() { close(it.signalled) }
				it.guard.getFunc = func(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
					if key == guardKey(interruptedEntity, "ENTITY") {
						// Held until the application is cancelled; the in-progress interval elapses first.
						<-ctx.Done()
						it.heldCancelled.Store(true)
						return nil, ctx.Err()
					}
					return nil, jetstream.ErrKeyNotFound
				}
			},
			check: func(t *testing.T, it *interruption) {
				if got := it.msg.inProgress.Load(); got != 1 {
					t.Errorf("in-progress signals = %d, want 1, the one that failed", got)
				}
				if !it.heldCancelled.Load() {
					t.Error("the held application was not cancelled when its in-progress signal failed")
				}
			},
		},
		{
			name:  "delivery has no metadata",
			cause: "invalid",
			arrange: func(it *interruption) {
				it.msg.noMetadata = true
				it.guard.getFunc = func(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
					if key == guardKey(interruptedEntity, "ENTITY") {
						it.guardReads.Add(1)
					}
					return nil, jetstream.ErrKeyNotFound
				}
			},
			check: func(t *testing.T, it *interruption) {
				if got := it.guardReads.Load(); got != 0 {
					t.Errorf("the input with no metadata was applied: %d applied-sequence reads, want 0", got)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				c, entities, guard, logs := interruptedWorkComponent(t)
				first := &keyedIngestTestMsg{}
				it := &interruption{guard: guard, msg: first}
				tc.arrange(it)
				errorsBefore := c.Health().ErrorCount

				if err := c.ingestPool.SubmitBlocking(t.Context(), interruptedInput(interruptedEntity, 1, first)); err != nil {
					t.Fatalf("submit the first input: %v", err)
				}
				if it.signalled != nil {
					// Blocked here, not in synctest.Wait, so the bubble's clock reaches the interval.
					<-it.signalled
				}
				synctest.Wait()

				if got := settledAs(first); got != "none" {
					t.Errorf("the interrupted input was settled (%s), want it left for redelivery", got)
				}
				if entityStored(entities, interruptedEntity) {
					t.Error("the interrupted input's entity was written")
				}
				for _, cause := range interruptedWorkCauses {
					want := 0.0
					if cause == tc.cause {
						want = 1
					}
					if got := testutil.ToFloat64(c.workInterrupted.WithLabelValues(cause)); got != want {
						t.Errorf("work_interrupted_total{cause=%q} = %v, want %v", cause, got, want)
					}
				}
				if got := c.Health().ErrorCount - errorsBefore; got != 1 {
					t.Errorf("Health's error count rose by %d, want 1", got)
				}
				errorLines := 0
				for _, line := range strings.Split(logs.String(), "\n") {
					if strings.Contains(line, "level=ERROR") {
						errorLines++
						if !strings.Contains(line, "cause="+tc.cause) || !strings.Contains(line, "entity_id="+interruptedEntity) {
							t.Errorf("ERROR line does not name cause %s and the entity: %s", tc.cause, line)
						}
					}
				}
				if errorLines != 1 {
					t.Errorf("ERROR lines = %d, want 1:\n%s", errorLines, logs.String())
				}
				if tc.check != nil {
					tc.check(t, it)
				}

				next := &keyedIngestTestMsg{}
				if err := c.ingestPool.SubmitBlocking(t.Context(), interruptedInput(nextEntity, 2, next)); err != nil {
					t.Fatalf("submit the next input: %v", err)
				}
				synctest.Wait()
				if got := settledAs(next); got != "ack" {
					t.Errorf("the lane's next input was settled %s, want ack: the lane must keep taking input", got)
				}
				if !entityStored(entities, nextEntity) {
					t.Error("the lane's next input was not applied")
				}
			})
		})
	}

	t.Run("lane context ended", func(t *testing.T) {
		c, entities, guard, logs := interruptedWorkComponent(t)
		var guardReads atomic.Int32
		guard.getFunc = func(context.Context, string) (jetstream.KeyValueEntry, error) {
			guardReads.Add(1)
			return nil, jetstream.ErrKeyNotFound
		}
		errorsBefore := c.Health().ErrorCount
		laneCtx, endLane := context.WithCancel(t.Context())
		endLane()
		msg := &keyedIngestTestMsg{}

		_ = c.processIngest(laneCtx, 0, interruptedInput(interruptedEntity, 1, msg))

		if got := settledAs(msg); got != "nak" {
			t.Errorf("an input reaching its lane after the lane's context ended was settled %s, want nak", got)
		}
		if guardReads.Load() != 0 || keyCount(entities) != 0 || keyCount(guard) != 0 {
			t.Errorf("the input was applied: %d applied-sequence reads, %d entity keys, %d applied-sequence keys; want none",
				guardReads.Load(), keyCount(entities), keyCount(guard))
		}
		warned := false
		for _, line := range strings.Split(logs.String(), "\n") {
			if strings.Contains(line, "level=WARN") && strings.Contains(line, "entity_id="+interruptedEntity) {
				warned = true
			}
		}
		if !warned {
			t.Errorf("no WARN line names the entity:\n%s", logs.String())
		}
		for _, cause := range interruptedWorkCauses {
			if got := testutil.ToFloat64(c.workInterrupted.WithLabelValues(cause)); got != 0 {
				t.Errorf("work_interrupted_total{cause=%q} = %v at Stop, want 0", cause, got)
			}
		}
		if got := c.Health().ErrorCount - errorsBefore; got != 0 {
			t.Errorf("Health's error count rose by %d at Stop, want 0", got)
		}
	})
}

// interruption is one case's first input: its message and the applied-sequence bucket its
// application reads, with what the case records of it. heldCancelled records that a held application
// saw its context end; guardReads counts the applied-sequence reads of the interrupted entity; when
// set, signalled is closed as the input's in-progress signal is sent, and the test waits for it.
type interruption struct {
	guard         *mockKVBucket
	msg           *keyedIngestTestMsg
	heldCancelled atomic.Bool
	guardReads    atomic.Int32
	signalled     chan struct{}
}

// interruptedWorkComponent builds, inside the calling bubble or test, a component over mock entity
// and applied-sequence buckets, logging into the returned buffer, with a one-lane ingest pool on a
// context of its own whose cancel is the component's, as Start builds it. The pool is shut down
// before the test ends.
func interruptedWorkComponent(t *testing.T) (*Component, *mockKVBucket, *mockKVBucket, *ingestGuardLogBuffer) {
	t.Helper()
	c, entities := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
	logs := newIngestGuardLogBuffer()
	c.logger = slog.New(slog.NewTextHandler(logs, nil))
	guard := newMockKVBucket()
	c.ingestGuardBucket = c.natsClient.NewKVStore(guard)
	c.config.IngestLanes = 1
	poolCtx, poolCancel := context.WithCancel(t.Context())
	c.ingestPoolCancel = poolCancel
	if err := c.buildIngestPool(poolCtx); err != nil {
		t.Fatalf("buildIngestPool: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := c.ingestPool.Shutdown(ctx); err != nil {
			t.Errorf("ingest pool Shutdown: %v", err)
		}
		poolCancel()
	})
	return c, entities, guard, logs
}
