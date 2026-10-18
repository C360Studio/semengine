package graphingest

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// keyedIngestTestMsg is a delivery the unit tests hand processIngest. It records how it was settled
// and counts the in-progress signals sent for it. Its metadata is a first delivery, as the server
// reports one; with noMetadata it reports none, a delivery the settlement helper refuses.
type keyedIngestTestMsg struct {
	ack        atomic.Bool
	nak        atomic.Bool
	term       atomic.Bool
	inProgress atomic.Int32
	// inProgressErr is what InProgress returns.
	inProgressErr error
	noMetadata    bool
	// onAck, when set, runs as the message is acknowledged, and onInProgress as an
	// in-progress signal is sent for it.
	onAck        func()
	onInProgress func()
}

// entity-id-audit:classify intentional-malformed "bad" line=164 column=61 surface=go-field:EntityState.ID entity_id_invalid:arity malformed keyed-ingest state rejection fixture

func (m *keyedIngestTestMsg) Metadata() (*jetstream.MsgMetadata, error) {
	if m.noMetadata {
		return nil, nil
	}
	return &jetstream.MsgMetadata{NumDelivered: 1}, nil
}
func (*keyedIngestTestMsg) Data() []byte         { return nil }
func (*keyedIngestTestMsg) Headers() nats.Header { return nil }
func (*keyedIngestTestMsg) Subject() string      { return "entity.test" }
func (*keyedIngestTestMsg) Reply() string        { return "" }
func (m *keyedIngestTestMsg) Ack() error {
	if m.onAck != nil {
		m.onAck()
	}
	m.ack.Store(true)
	return nil
}
func (*keyedIngestTestMsg) DoubleAck(context.Context) error    { return nil }
func (m *keyedIngestTestMsg) Nak() error                       { m.nak.Store(true); return nil }
func (m *keyedIngestTestMsg) NakWithDelay(time.Duration) error { m.nak.Store(true); return nil }
func (m *keyedIngestTestMsg) InProgress() error {
	m.inProgress.Add(1)
	if m.onInProgress != nil {
		m.onInProgress()
	}
	return m.inProgressErr
}
func (m *keyedIngestTestMsg) Term() error                 { m.term.Store(true); return nil }
func (m *keyedIngestTestMsg) TermWithReason(string) error { m.term.Store(true); return nil }

// testConsumer and testHeartbeat are the consumer configuration and in-progress interval of the
// inputs the unit tests hand processIngest: graph-ingest's defaults for a port that declares neither.
var testConsumer = natsclient.StreamConsumerConfig{StreamName: "ENTITY", AckWait: defaultIngestAckWait}

const testHeartbeat = defaultIngestAckWait / 4

// --- laneGuard (in-memory guard tier, ADR-072) ---

func TestLaneGuard_GetSet(t *testing.T) {
	g := newLaneGuard(8)
	_, ok := g.get("k")
	assert.False(t, ok, "absent key is a miss")

	g.set("k", guardRecord{seq: 5})
	v, ok := g.get("k")
	assert.True(t, ok)
	assert.Equal(t, uint64(5), v.seq)

	g.set("k", guardRecord{seq: 9}) // update in place
	v, _ = g.get("k")
	assert.Equal(t, uint64(9), v.seq)
}

func TestLaneGuard_BoundedEviction(t *testing.T) {
	g := newLaneGuard(2)
	g.set("a", guardRecord{seq: 1})
	g.set("b", guardRecord{seq: 2})
	require.Len(t, g.seq, 2)

	// Inserting a third distinct key evicts one arbitrary entry (durable tier
	// backs correctness, so which one is immaterial) — the map stays bounded.
	g.set("c", guardRecord{seq: 3})
	assert.Len(t, g.seq, 2, "map stays bounded at max")
	v, ok := g.get("c")
	assert.True(t, ok, "the just-inserted key is present")
	assert.Equal(t, uint64(3), v.seq)
}

func TestLaneGuard_UpdateAtCapDoesNotEvict(t *testing.T) {
	g := newLaneGuard(2)
	g.set("a", guardRecord{seq: 1})
	g.set("b", guardRecord{seq: 2})
	// Updating an EXISTING key at capacity must not evict (no new entry).
	g.set("a", guardRecord{seq: 10})
	assert.Len(t, g.seq, 2)
	va, _ := g.get("a")
	vb, okb := g.get("b")
	assert.Equal(t, uint64(10), va.seq)
	assert.True(t, okb, "existing sibling not evicted by an in-place update")
	assert.Equal(t, uint64(2), vb.seq)
}

// TestLaneGuard_RefusedKeysAreBounded: a key is reported new once, again after it is forgotten,
// and the set of refused keys never holds more than the lane's capacity (design D21).
func TestLaneGuard_RefusedKeysAreBounded(t *testing.T) {
	g := newLaneGuard(2)
	if !g.markRefused("a") || g.markRefused("a") {
		t.Fatal("a refused key must be reported new once")
	}
	g.forgetRefused("a")
	if !g.markRefused("a") {
		t.Fatal("a forgotten key must be reported new again")
	}
	for _, key := range []string{"b", "c", "d"} {
		if !g.markRefused(key) {
			t.Errorf("key %s was reported as already refused", key)
		}
	}
	if len(g.refused) != 2 {
		t.Errorf("refused keys = %d, want at most the capacity, 2", len(g.refused))
	}
	if _, ok := g.refused["d"]; !ok {
		t.Error("the key just refused is not held")
	}
}

func TestGuardKey(t *testing.T) {
	assert.Equal(t, "c360.ops.robotics.gcs.drone.001/SENSOR",
		guardKey("c360.ops.robotics.gcs.drone.001", "SENSOR"))
}

// --- ingestGuardStale: in-memory tier + nil-durable (no NATS) ---

func TestIngestGuardStale_MemoryTierAndFirstSeen(t *testing.T) {
	c := createTestComponent(t) // no durable tier in this unit test: ingestGuardBucket is nil
	c.ingestGuardMem = []*laneGuard{newLaneGuard(16)}
	ctx := context.Background()
	work := ingestWork{entityID: "c360.a.b.c.d.001", stream: "SENSOR", seq: 5}

	// First-seen (tier-1 miss + nil durable) → not stale.
	stale, err := c.ingestGuardStale(ctx, 0, work)
	require.NoError(t, err)
	assert.False(t, stale, "an unseen (entity,stream) is not stale")

	// Stamp the in-memory tier at seq 5, then re-check.
	c.ingestGuardMem[0].set(guardKey(work.entityID, work.stream), guardRecord{seq: 5})

	older := work
	older.seq = 3
	stale, err = c.ingestGuardStale(ctx, 0, older)
	require.NoError(t, err)
	assert.True(t, stale, "a lower sequence than last-applied is a stale redelivery")

	same := work
	same.seq = 5
	stale, err = c.ingestGuardStale(ctx, 0, same)
	require.NoError(t, err)
	assert.True(t, stale, "an equal sequence (redelivery of the applied message) is stale")

	newer := work
	newer.seq = 6
	stale, err = c.ingestGuardStale(ctx, 0, newer)
	require.NoError(t, err)
	assert.False(t, stale, "a higher sequence is a fresh update, not stale")
}

// A different stream for the same entity must NOT be judged against another
// stream's sequence (independent per-stream sequence spaces — round-2 fix).
func TestIngestGuardStale_PerStreamIndependence(t *testing.T) {
	c := createTestComponent(t)
	c.ingestGuardMem = []*laneGuard{newLaneGuard(16)}
	ctx := context.Background()
	entity := "c360.a.b.c.d.001"

	// Stream A applied a high sequence.
	c.ingestGuardMem[0].set(guardKey(entity, "STREAM_A"), guardRecord{seq: 1000})

	// A low sequence from stream B is a DIFFERENT key → not stale.
	fromB := ingestWork{entityID: entity, stream: "STREAM_B", seq: 5}
	stale, err := c.ingestGuardStale(ctx, 0, fromB)
	require.NoError(t, err)
	assert.False(t, stale, "a low seq from stream B is not silenced by stream A's high seq")
}

func TestProcessIngest_InvalidGraphableTerminatesBeforeGuardIO(t *testing.T) {
	validID := "acme.ops.test.system.widget.001"
	tests := []struct {
		name        string
		entity      *graph.EntityState
		field       string
		reason      string
		tripleIndex int
		predicate   bool
		unmetered   bool
	}{
		{name: "invalid envelope", entity: &graph.EntityState{ID: "bad"}, field: "id", reason: "arity", tripleIndex: -1},
		{name: "cross subject", entity: &graph.EntityState{ID: validID, Triples: []message.Triple{{Subject: "bad", Predicate: "test.state.value"}}}, field: "id", reason: "unknown", tripleIndex: -1, unmetered: true},
		{name: "invalid explicit reference", entity: &graph.EntityState{ID: validID, Triples: []message.Triple{{Subject: validID, Predicate: "test.state.value", Object: 42, Datatype: message.EntityReferenceDatatype}}}, field: "reference", reason: "object_type", tripleIndex: 0},
		{
			name: "invalid predicate",
			entity: &graph.EntityState{ID: validID, Triples: []message.Triple{{
				Subject: validID, Predicate: "bad.two", // predicate-audit:invalid {"kind":"stored-predicate","value":"bad.two","reason":"arity"}
			}}},
			field: "predicate", reason: "arity", tripleIndex: -1,
			predicate: true, // predicate-audit:unrelated {"column":15,"surface":"go-field:predicate","value":"","basis":"reviewed boolean expectation flag, not predicate syntax"}
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			component, _ := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
			var logs bytes.Buffer
			component.logger = slog.New(slog.NewTextHandler(&logs, nil))
			guardBucket := newMockKVBucket()
			var guardGets atomic.Int32
			guardBucket.getFunc = func(context.Context, string) (jetstream.KeyValueEntry, error) {
				guardGets.Add(1)
				return nil, jetstream.ErrKeyNotFound
			}
			component.ingestGuardBucket = component.natsClient.NewKVStore(guardBucket)
			component.ingestGuardMem = []*laneGuard{newLaneGuard(16)}
			msg := &keyedIngestTestMsg{}
			work := ingestWork{entity: tt.entity, msg: msg, consumer: testConsumer, heartbeat: testHeartbeat, entityID: tt.entity.ID, stream: "ENTITY", seq: 1}
			errorsBefore := atomic.LoadInt64(&component.errors)

			var before float64
			if tt.predicate {
				before = testutil.ToFloat64(component.predicateContractRejections.WithLabelValues("graphable", tt.reason))
			} else {
				before = testutil.ToFloat64(component.entityStateContractRejections.WithLabelValues("graphable", tt.field, tt.reason))
			}

			err := component.processIngest(context.Background(), 0, work)
			require.Error(t, err)
			require.True(t, errs.IsInvalid(err), "structural failure must be nonretryable")
			require.Equal(t, int32(0), guardGets.Load(), "invalid candidate reached durable guard Get")
			require.True(t, msg.term.Load(), "immutable invalid message must Term")
			require.False(t, msg.nak.Load(), "immutable invalid message must not redeliver")
			require.False(t, msg.ack.Load(), "terminal rejection is not an ack path")
			require.Equal(t, errorsBefore, atomic.LoadInt64(&component.errors), "pre-guard rejection must not poison sticky error state")

			if !tt.unmetered && tt.predicate {
				assert.InDelta(t, before+1, testutil.ToFloat64(component.predicateContractRejections.WithLabelValues("graphable", tt.reason)), 0.0001)
			} else if !tt.unmetered {
				assert.InDelta(t, before+1, testutil.ToFloat64(component.entityStateContractRejections.WithLabelValues("graphable", tt.field, tt.reason)), 0.0001)
			}
			logLine := logs.String()
			assert.Equal(t, 1, strings.Count(logLine, "level=WARN"), "one rejection warning per message")
			assert.Contains(t, logLine, "lane=graphable")
			assert.Contains(t, logLine, "field="+tt.field)
			assert.Contains(t, logLine, "reason="+tt.reason)
			if tt.tripleIndex >= 0 {
				assert.Contains(t, logLine, "triple_index=0")
			} else {
				assert.NotContains(t, logLine, "triple_index=")
			}
			assert.NotContains(t, logLine, "entity_id=")
			assert.NotContains(t, logLine, "error=")
		})
	}
}

func TestProcessIngest_DirtyStoredStateNaksAndInventoriesPerEntity(t *testing.T) {
	tests := []struct {
		name        string
		entityID    string
		stored      []byte
		resetReason graph.StateResetReason
	}{
		{
			name:        "noncanonical predicate",
			entityID:    "acme.ops.test.system.widget.dirty-predicate",
			stored:      []byte(`{"id":"acme.ops.test.system.widget.dirty-predicate","triples":[{"subject":"acme.ops.test.system.widget.dirty-predicate","predicate":"bad.two"}]}`), // predicate-audit:invalid {"kind":"stored-predicate","value":"bad.two","reason":"arity"}
			resetReason: graph.GraphStateReasonNoncanonicalPredicate,
		},
		{
			name:        "noncanonical subject",
			entityID:    "acme.ops.test.system.widget.dirty-subject",
			stored:      []byte(`{"id":"acme.ops.test.system.widget.dirty-subject","triples":[{"subject":"bad","predicate":"test.state.value"}]}`),
			resetReason: graph.GraphStateReasonNoncanonicalEntityID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			component, entityBucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
			entityBucket.data[tt.entityID] = mockKVData{value: tt.stored, revision: 1}
			component.ingestGuardMem = []*laneGuard{newLaneGuard(16)}
			guardBucket := newMockKVBucket()
			var guardStamps atomic.Int32
			guardBucket.putFunc = func(context.Context, string, []byte) (uint64, error) {
				guardStamps.Add(1)
				return 1, nil
			}
			component.ingestGuardBucket = component.natsClient.NewKVStore(guardBucket)
			var logs bytes.Buffer
			component.logger = slog.New(slog.NewTextHandler(&logs, nil))
			msg := &keyedIngestTestMsg{}
			work := ingestWork{
				entity: &graph.EntityState{ID: tt.entityID, Triples: withTestMetadata(message.Triple{
					Subject: tt.entityID, Predicate: "test.state.value", Object: "new",
				})},
				msg: msg, consumer: testConsumer, heartbeat: testHeartbeat, entityID: tt.entityID, stream: "ENTITY", seq: 2,
			}
			predicateBefore := testutil.ToFloat64(component.predicateContractRejections.WithLabelValues("graphable", "arity"))
			entityBefore := testutil.ToFloat64(component.entityStateContractRejections.WithLabelValues("graphable", "subject", "arity"))

			err := component.processIngest(context.Background(), 0, work)
			require.Error(t, err)
			var stateErr *graph.StateContractError
			require.True(t, errors.As(err, &stateErr), "dirty persisted state must retain its typed reset error")
			assert.Equal(t, tt.resetReason, stateErr.Reason)
			assert.Equal(t, tt.entityID, stateErr.EntityID, "RMW classification must stamp the entity ID")
			// D8: resident poison is the ENVIRONMENT's fault — Nak so the valid
			// arrival survives the repair window, never Term (permanent loss).
			assert.True(t, msg.nak.Load(), "resident poison must Nak for redelivery after repair")
			assert.False(t, msg.term.Load(), "resident poison must not destroy the valid arrival")
			assert.False(t, msg.ack.Load())
			assert.Equal(t, int32(0), guardStamps.Load(), "failed apply must not stamp the redelivery guard")
			// Per-entity inventory replaces the retired surface-global latch:
			// the entity is inventoried, queries stay READY.
			rec, inventoried := poisonInventoryEntry(component, tt.entityID)
			require.True(t, inventoried, "apply-discovered poison must be inventoried per-entity")
			assert.Equal(t, tt.entityID, rec.contractErr.EntityID)
			assert.Equal(t, predicateBefore, testutil.ToFloat64(component.predicateContractRejections.WithLabelValues("graphable", "arity")), "stored poison is not a producer predicate rejection")
			assert.Equal(t, entityBefore, testutil.ToFloat64(component.entityStateContractRejections.WithLabelValues("graphable", "subject", "arity")), "stored poison is not a producer entity-state rejection")
			assert.Equal(t, 1, strings.Count(logs.String(), "code=graph_state_reset_required"), "first inventory record logs once")
			assert.Contains(t, logs.String(), "reason="+string(tt.resetReason))

			// Redelivery loop dedup: a second delivery re-Naks without re-logging.
			msg2 := &keyedIngestTestMsg{}
			work.msg = msg2
			require.Error(t, component.processIngest(context.Background(), 0, work))
			assert.True(t, msg2.nak.Load())
			assert.Equal(t, 1, strings.Count(logs.String(), "code=graph_state_reset_required"), "redelivery must not re-log an inventoried entity")
		})
	}
}

func TestProcessIngest_GenericFatalApplyErrorTerminatesWithoutPoison(t *testing.T) {
	const entityID = "acme.ops.test.system.widget.storage-fatal"
	component, entityBucket := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
	entityBucket.getFunc = func(context.Context, string) (jetstream.KeyValueEntry, error) {
		return nil, errs.WrapFatal(errs.ErrStorageFull, "test", "entityGet", "storage exhausted")
	}
	component.entityBucket = component.natsClient.NewKVStore(entityBucket, func(options *natsclient.KVOptions) {
		options.MaxRetries = 0
	})
	component.ingestGuardMem = []*laneGuard{newLaneGuard(16)}
	var logs bytes.Buffer
	component.logger = slog.New(slog.NewTextHandler(&logs, nil))
	msg := &keyedIngestTestMsg{}

	err := component.processIngest(context.Background(), 0, ingestWork{
		entity: &graph.EntityState{ID: entityID, Triples: withTestMetadata(message.Triple{
			Subject: entityID, Predicate: "test.state.value", Object: "new",
		})},
		msg: msg, consumer: testConsumer, heartbeat: testHeartbeat, entityID: entityID, stream: "ENTITY", seq: 1,
	})

	require.Error(t, err)
	require.True(t, errs.IsFatal(err), "classified fatal apply error must remain fatal through merge wrappers")
	assert.True(t, msg.term.Load(), "generic fatal apply failure must Term")
	assert.False(t, msg.nak.Load(), "generic fatal apply failure must not redeliver")
	assert.False(t, msg.ack.Load())
	assert.Equal(t, int64(0), component.entityPoisonSize.Load(), "generic fatal failure must not masquerade as graph-state poison")
	assert.Contains(t, logs.String(), "fatal ingest failure; terminating")
	assert.NotContains(t, logs.String(), "code=graph_state_reset_required")
}

func TestProcessIngest_FillsEmptyFactSubjectBeforeStaleGuard(t *testing.T) {
	component, _ := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
	component.ingestGuardMem = []*laneGuard{newLaneGuard(16)}
	validID := "acme.ops.test.system.widget.001"
	component.ingestGuardMem[0].set(guardKey(validID, "ENTITY"), guardRecord{seq: 1})
	entity := &graph.EntityState{ID: validID, Triples: []message.Triple{{Subject: "", Predicate: "test.state.value"}}}
	msg := &keyedIngestTestMsg{}

	err := component.processIngest(context.Background(), 0, ingestWork{
		entity: entity, msg: msg, consumer: testConsumer, heartbeat: testHeartbeat, entityID: validID, stream: "ENTITY", seq: 1,
	})
	require.NoError(t, err)
	require.Equal(t, validID, entity.Triples[0].Subject, "fact projection fill must precede stale-drop guard")
	require.True(t, msg.ack.Load())
	require.False(t, msg.term.Load())
	require.False(t, msg.nak.Load())
}

// settledAs names how msg was settled: "ack", "nak" or "term", "none", or "mixed" for more than one.
func settledAs(msg *keyedIngestTestMsg) string {
	ack, nak, term := msg.ack.Load(), msg.nak.Load(), msg.term.Load()
	switch {
	case ack && !nak && !term:
		return "ack"
	case nak && !ack && !term:
		return "nak"
	case term && !ack && !nak:
		return "term"
	case !ack && !nak && !term:
		return "none"
	}
	return "mixed"
}

// TestProcessIngest_DecisionsSettleAsBefore holds design D8, "Long apply": an input's application
// returns a decision, and the delivery helper settles it once, as processIngest settled each branch
// before it ran through the helper. An applied input has aged the readiness view by the time it is
// acknowledged; a stale redelivery, and an input left for redelivery, do not age it. The branches
// that terminate, and resident poison, are held by the tests above.
func TestProcessIngest_DecisionsSettleAsBefore(t *testing.T) {
	const entityID = "acme.ops.test.system.widget.decision"
	deliveredAt := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	unavailable := errors.New("test: bucket unavailable")
	tests := []struct {
		name    string
		arrange func(c *Component, entities, guard *mockKVBucket)
		want    string
		aged    bool
		dropped float64
	}{
		{name: "applied", want: "ack", aged: true},
		{
			name: "stale redelivery",
			arrange: func(c *Component, _, _ *mockKVBucket) {
				c.ingestGuardMem[0].set(guardKey(entityID, "ENTITY"), guardRecord{seq: 2})
			},
			want: "ack", dropped: 1,
		},
		{
			name: "guard read fails",
			arrange: func(_ *Component, _, guard *mockKVBucket) {
				guard.getFunc = func(context.Context, string) (jetstream.KeyValueEntry, error) { return nil, unavailable }
			},
			want: "nak",
		},
		{
			name: "apply fails transiently",
			arrange: func(c *Component, entities, _ *mockKVBucket) {
				entities.getFunc = func(context.Context, string) (jetstream.KeyValueEntry, error) {
					return nil, errs.WrapTransient(unavailable, "test", "entityGet", "bucket unavailable")
				}
				c.entityBucket = c.natsClient.NewKVStore(entities, func(options *natsclient.KVOptions) {
					options.MaxRetries = 0
				})
			},
			want: "nak",
		},
		{
			name: "durable stamp fails",
			arrange: func(_ *Component, _, guard *mockKVBucket) {
				guard.putFunc = func(context.Context, string, []byte) (uint64, error) { return 0, unavailable }
			},
			want: "nak",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, entities := createTestComponentWithMockKVBucket(t, withAuthority("acme", "ops"))
			guard := newMockKVBucket()
			c.ingestGuardBucket = c.natsClient.NewKVStore(guard)
			c.ingestGuardMem = []*laneGuard{newLaneGuard(16)}
			if tt.arrange != nil {
				tt.arrange(c, entities, guard)
			}
			msg := &keyedIngestTestMsg{}
			var agedAtAck time.Time
			msg.onAck = func() { agedAtAck = c.oldestOutstandingAt(1) }
			droppedBefore := testutil.ToFloat64(c.redeliveriesDropped)

			_ = c.processIngest(t.Context(), 0, ingestWork{
				entity: &graph.EntityState{ID: entityID, MessageType: testEntityType(), Triples: withTestMetadata(message.Triple{
					Subject: entityID, Predicate: "test.state.value", Object: "decided", Timestamp: fixtureTime, Confidence: 1.0,
				})},
				msg: msg, consumer: testConsumer, heartbeat: testHeartbeat,
				entityID: entityID, stream: "ENTITY", seq: 2, deliveredAt: deliveredAt,
			})

			if got := settledAs(msg); got != tt.want {
				t.Errorf("settlement = %s, want %s", got, tt.want)
			}
			aged := c.oldestOutstandingAt(1)
			if tt.aged {
				if !aged.Equal(deliveredAt) {
					t.Errorf("readiness view aged from %v after the input applied, want its delivery time %v", aged, deliveredAt)
				}
				if !agedAtAck.Equal(deliveredAt) {
					t.Errorf("readiness view at the acknowledgement aged from %v, want the applied input's %v", agedAtAck, deliveredAt)
				}
			} else if !aged.IsZero() {
				t.Errorf("readiness view aged from %v by an input that was not applied, want no age", aged)
			}
			if got := testutil.ToFloat64(c.redeliveriesDropped) - droppedBefore; got != tt.dropped {
				t.Errorf("stale redeliveries dropped = %v, want %v", got, tt.dropped)
			}
		})
	}
}
