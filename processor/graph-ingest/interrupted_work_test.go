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
// unsettled, the interrupted-work count under that cause rises by one, one ERROR line names the
// cause, and the next input on the same lane is applied and acknowledged. The interruption adds
// nothing to the error count Health reports (#91 comment 6124130342): where it lands outside the
// entity's write, that count is unchanged; where it lands in the entity's write (its read inside the
// compare-and-set, or a container's birth), the write it cut counts once, at its own site, and logs
// its own ERROR line beside the cause's (Codex B1 on PR #93). The lane is one lane of a keyed pool
// on a context of its own, as Start runs it.
// The last case holds the design's "At Stop": an input that reaches its lane after the lane's
// context has ended is negatively acknowledged, applies nothing, and is logged at WARN.
func TestInterruptedApplicationIsRedeliveredNotAStop(t *testing.T) {
	cases := []struct {
		name  string
		cause string
		// hierarchy turns on hierarchy inference, so the first input's birth creates its containers.
		hierarchy bool
		// writeFailed is set where the interruption fails the entity's write, which counts once in
		// the error count Health reports and logs its own ERROR line, "Failed to merge entity", beside
		// the one naming the cause.
		writeFailed bool
		// arrange interrupts the first input's application, through the applied-sequence bucket,
		// the entity bucket or its message.
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
			check: checkHeartbeatInterruption,
		},
		{
			name:        "in-progress signal fails during the entity write",
			cause:       "heartbeat_failed",
			writeFailed: true,
			arrange: func(it *interruption) {
				it.msg.inProgressErr = errors.New("test: the in-progress signal is refused")
				it.signalled = make(chan struct{})
				it.msg.onInProgress = func() { close(it.signalled) }
				it.entities.getFunc = func(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
					if key == interruptedEntity {
						// The compare-and-set's read, held until the application is cancelled.
						<-ctx.Done()
						it.heldCancelled.Store(true)
						return nil, ctx.Err()
					}
					return nil, jetstream.ErrKeyNotFound
				}
			},
			check: checkHeartbeatInterruption,
		},
		{
			name:        "in-progress signal fails during a container birth",
			cause:       "heartbeat_failed",
			hierarchy:   true,
			writeFailed: true,
			arrange: func(it *interruption) {
				it.msg.inProgressErr = errors.New("test: the in-progress signal is refused")
				it.signalled = make(chan struct{})
				it.msg.onInProgress = func() { close(it.signalled) }
				var held atomic.Bool
				it.entities.createFunc = func(ctx context.Context, key string, value []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
					if held.CompareAndSwap(false, true) {
						// The first create is the first container's birth, which runs before the
						// entity's own write; held until the application is cancelled.
						<-ctx.Done()
						it.heldCancelled.Store(true)
						return 0, ctx.Err()
					}
					it.entities.mu.Lock()
					defer it.entities.mu.Unlock()
					if _, exists := it.entities.data[key]; exists {
						return 0, jetstream.ErrKeyExists
					}
					it.entities.data[key] = mockKVData{value: value, revision: 1}
					return 1, nil
				}
			},
			check: checkHeartbeatInterruption,
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
				if tc.hierarchy {
					c.config.EnableHierarchy = true
					buildHierarchyInference(t, c)
				}
				first := &keyedIngestTestMsg{}
				it := &interruption{guard: guard, entities: entities, msg: first}
				tc.arrange(it)
				errorsBefore := c.Health().ErrorCount

				if err := c.ingestPool.SubmitBlocking(t.Context(), interruptedEntity, interruptedInput(interruptedEntity, 1, first)); err != nil {
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
				// The write the interruption cut, if any: its one count and its one ERROR line.
				cutWrites := 0
				if tc.writeFailed {
					cutWrites = 1
				}
				if got := c.Health().ErrorCount - errorsBefore; got != cutWrites {
					t.Errorf("Health's error count rose by %d, want %d", got, cutWrites)
				}
				errorLines, writeLines := 0, 0
				for _, line := range strings.Split(logs.String(), "\n") {
					if tc.writeFailed && strings.Contains(line, "level=ERROR") &&
						strings.Contains(line, `msg="Failed to merge entity"`) &&
						strings.Contains(line, "entity_id="+interruptedEntity) {
						writeLines++
						continue
					}
					if strings.Contains(line, "level=ERROR") {
						errorLines++
						if !strings.Contains(line, "cause="+tc.cause) || !strings.Contains(line, "entity_id="+interruptedEntity) {
							t.Errorf("ERROR line does not name cause %s and the entity: %s", tc.cause, line)
						}
					}
				}
				if errorLines != 1 {
					t.Errorf("ERROR lines naming the cause = %d, want 1:\n%s", errorLines, logs.String())
				}
				if writeLines != cutWrites {
					t.Errorf("the failed write's ERROR lines = %d, want %d:\n%s", writeLines, cutWrites, logs.String())
				}
				if tc.check != nil {
					tc.check(t, it)
				}

				next := &keyedIngestTestMsg{}
				if err := c.ingestPool.SubmitBlocking(t.Context(), nextEntity, interruptedInput(nextEntity, 2, next)); err != nil {
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

// checkHeartbeatInterruption holds what an in-progress signal that fails leaves behind: the one
// signal sent, which failed, and the held application cancelled.
func checkHeartbeatInterruption(t *testing.T, it *interruption) {
	t.Helper()
	if got := it.msg.inProgress.Load(); got != 1 {
		t.Errorf("in-progress signals = %d, want 1, the one that failed", got)
	}
	if !it.heldCancelled.Load() {
		t.Error("the held application was not cancelled when its in-progress signal failed")
	}
}

// interruption is one case's first input: its message and the applied-sequence and entity buckets
// its application reads and writes, with what the case records of it. heldCancelled records that a
// held application saw its context end; guardReads counts the applied-sequence reads of the
// interrupted entity; when set, signalled is closed as the input's in-progress signal is sent, and
// the test waits for it.
type interruption struct {
	guard         *mockKVBucket
	entities      *mockKVBucket
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

// refuseEntityCreate makes bucket refuse every create with an ordinary error: a write failure that
// is not an interruption, which ends the entity's compare-and-set at once, with no retry.
func refuseEntityCreate(bucket *mockKVBucket) {
	bucket.createFunc = func(context.Context, string, []byte, ...jetstream.KVCreateOpt) (uint64, error) {
		return 0, errors.New("test: the entity bucket refuses the create")
	}
}

// TestOrdinaryLaneWriteErrorCountsOnce holds the other side of TestInterruptedApplicationIsRedeliveredNotAStop
// (Codex B1 on PR #93; #91 comment 6124130342): an entity write that fails on the lane with no
// interruption raises the error count Health reports by one, at the write's own site, counts nothing as
// interrupted work, and its input is negatively acknowledged.
func TestOrdinaryLaneWriteErrorCountsOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, entities, _, _ := interruptedWorkComponent(t)
		refuseEntityCreate(entities)
		errorsBefore := c.Health().ErrorCount
		msg := &keyedIngestTestMsg{}

		if err := c.ingestPool.SubmitBlocking(t.Context(), interruptedEntity, interruptedInput(interruptedEntity, 1, msg)); err != nil {
			t.Fatalf("submit the input: %v", err)
		}
		synctest.Wait()

		if got := settledAs(msg); got != "nak" {
			t.Errorf("the input whose write failed was settled %s, want nak", got)
		}
		if got := c.Health().ErrorCount - errorsBefore; got != 1 {
			t.Errorf("Health's error count rose by %d, want 1", got)
		}
		for _, cause := range interruptedWorkCauses {
			if got := testutil.ToFloat64(c.workInterrupted.WithLabelValues(cause)); got != 0 {
				t.Errorf("work_interrupted_total{cause=%q} = %v, want 0", cause, got)
			}
		}
	})
}

// TestDirectWriteErrorCountsAtOnce holds that a write called outside the delivery helper counts its
// error once in Health's error count before it returns: ingestEntity, the synchronous entry
// handleMessage uses, and the exported CreateEntity.
func TestDirectWriteErrorCountsAtOnce(t *testing.T) {
	t.Run("ingestEntity", func(t *testing.T) {
		c, entities := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
		refuseEntityCreate(entities)
		errorsBefore := c.Health().ErrorCount

		if err := c.ingestEntity(t.Context(), interruptedInput(interruptedEntity, 1, nil).entity, false); err == nil {
			t.Fatal("ingestEntity succeeded, want the refused create's error")
		}
		if got := c.Health().ErrorCount - errorsBefore; got != 1 {
			t.Errorf("Health's error count rose by %d, want 1", got)
		}
	})
	t.Run("CreateEntity", func(t *testing.T) {
		c, entities := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
		refuseEntityCreate(entities)
		errorsBefore := c.Health().ErrorCount

		if err := c.CreateEntity(t.Context(), interruptedInput(interruptedEntity, 1, nil).entity); err == nil {
			t.Fatal("CreateEntity succeeded, want the refused create's error")
		}
		if got := c.Health().ErrorCount - errorsBefore; got != 1 {
			t.Errorf("Health's error count rose by %d, want 1", got)
		}
	})
}
