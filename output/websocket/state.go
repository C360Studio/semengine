package websocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/kvwatch"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/graphview"
)

// viewRecheck is how long the view supervisor waits before it opens ENTITY_STATES again, after the
// bucket was missing or a view ended, as graph-query's community supervisor does
// (processor/graph-query/community.go).
const viewRecheck = 5 * time.Second

// The applied-state readiness, the view supervisor's stuck state. appliedCatchingUp is
// the zero value: no view has caught up since Start.
const (
	appliedCatchingUp int32 = iota
	appliedLive
	appliedLost
)

// The applied-state stuck states Health reports, and the 503 an upgrade gets while one holds.
const (
	stuckCatchingUp = "applied-state view catching up"
	stuckWatchLost  = "applied-state watch lost, reopening"
)

// viewUnavailable is the supervisor's log message for a failed open or an ended view.
const viewUnavailable = "applied-state view unavailable; upgrades are refused with 503 until it opens and catches up again"

// storedState is one ENTITY_STATES value as the view keeps it: the bytes graph-ingest stored,
// checked by graph.UnmarshalEntityState, and the KV write time.
type storedState struct {
	raw     []byte
	created time.Time
}

// stateView is the instance's one view of ENTITY_STATES, shared by its clients (ADR-081).
type stateView = graphview.View[storedState]

// stateSubscription is one client's attachment to the view.
type stateSubscription = graphview.Subscription[storedState]

// superviseView runs views of ENTITY_STATES until ctx, the run context, ends: it opens the bucket
// through the catalog's reader, which never creates it, runs one view until it returns, waits
// viewRecheck and opens again, as graph-query does for COMMUNITY_INDEX. A missing bucket and a lost
// watch are the same case: upgrades are refused until a new view has caught up.
//
// Each failure is logged, at Warn when it is the first, differs from the last one logged, or follows
// a view that opened, and at Debug otherwise. Once ctx ends it returns nil, or the error of the
// view's Run when that says more than the cancellation, which cleanup hands to Stop's caller.
func (c *output) superviseView(ctx context.Context) error {
	var last string
	for {
		opened, err := c.runView(ctx)
		if ctx.Err() != nil {
			return shutdownFailure(ctx, opened, err)
		}
		if err == nil { // Run ends a live view only with an error; this keeps the log honest if not
			err = errors.New("the applied-state view returned")
		}
		level := slog.LevelDebug
		if opened || err.Error() != last {
			level = slog.LevelWarn
		}
		last = err.Error()
		c.logger.Log(ctx, level, viewUnavailable, slog.String("listener", c.listener),
			slog.Any("error", err), slog.Duration("retry_in", viewRecheck))
		timer := time.NewTimer(viewRecheck)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

// shutdownFailure is what a view's Run returned once ctx has ended: nil for a view that never
// opened, one Run ended with the cancellation alone, or one whose watch was lost as ctx ended, and
// Run's error otherwise, which is a failure to stop the watch (graphview.View.Run, kvwatch.Stop).
// It is graph-query's rule (processor/graph-query/community.go) for the same Run.
func shutdownFailure(ctx context.Context, opened bool, err error) error {
	if !opened || err == nil || err == ctx.Err() {
		return nil
	}
	if errors.Is(err, graphview.ErrWatcherLost) && !errors.Is(err, kvwatch.ErrDeliveryOpen) {
		return nil
	}
	return err
}

// runView opens ENTITY_STATES, publishes a view of it for the upgrade handler and runs the view
// until ctx ends or its watch is lost, reporting whether a view was opened. Once Run has returned
// the view attaches nothing, and it is withdrawn so the handler sees none.
func (c *output) runView(ctx context.Context) (bool, error) {
	reader, err := kvcatalog.OpenCatalogReader(ctx, c.natsClient, graph.BucketEntityStates)
	if err != nil {
		return false, err
	}
	view, err := graphview.New(reader, c.decodeState, graphview.WithHooks(graphview.Hooks{
		OnCaughtUp:    func() { c.applied.Store(appliedLive) },
		OnWatcherLost: func(error) { c.applied.Store(appliedLost) },
		OnFanOut:      c.countCoalesced,
	}))
	if err != nil {
		return false, err
	}
	c.view.Store(view)
	defer c.view.Store(nil)
	return true, view.Run(ctx)
}

// decodeState is the view's decode: every key is an entity, and a value graph.UnmarshalEntityState
// refuses poisons that key until a valid write replaces it. The stored bytes are
// kept as they are, so a put frame carries what graph-ingest stored.
func (c *output) decodeState(key string, value []byte, meta graphview.EntryMeta) (storedState, bool, error) {
	c.lifecycle.CountInput(len(value))
	c.metrics.received.Inc()
	var entity graph.EntityState
	if err := graph.UnmarshalEntityState(key, value, &entity); err != nil {
		return storedState{}, false, err
	}
	return storedState{raw: bytes.Clone(value), created: meta.Created}, true, nil
}

// countCoalesced is the view's fan-out hook: changes overwritten in a client's pending buffer before
// its writer took them are counted and logged as client_behind.
// Merging inside the view's own window is the contract, and graphview does not count it.
func (c *output) countCoalesced(overwritten, _ int) {
	if overwritten == 0 {
		return
	}
	c.drops.record(reasonClientBehind, overwritten,
		"a client is behind and its pending changes were merged per key; the latest write of each key still arrives, "+
			"and other clients are unaffected")
}

// appliedStuck is the applied-state stuck state, or "" once a view has caught up.
func (c *output) appliedStuck() string {
	switch c.applied.Load() {
	case appliedLive:
		return ""
	case appliedLost:
		return stuckWatchLost
	default:
		return stuckCatchingUp
	}
}

// subscribeState attaches a client to the current view: its snapshot and the changes after it. When
// the view is not caught up, or its watch is lost, it returns the reason the upgrade is refused.
// ctx is the upgrade request's: the subscription ends with it.
func (c *output) subscribeState(ctx context.Context) (graphview.Snapshot[storedState], *stateSubscription, string) {
	view := c.view.Load()
	if view == nil {
		return graphview.Snapshot[storedState]{}, nil, c.notReady()
	}
	snapshot, sub, err := view.SnapshotAndSubscribe(ctx)
	if err != nil {
		if errors.Is(err, graphview.ErrWatcherLost) {
			return graphview.Snapshot[storedState]{}, nil, stuckWatchLost
		}
		return graphview.Snapshot[storedState]{}, nil, c.notReady()
	}
	return snapshot, sub, ""
}

// notReady is the reason an upgrade is refused while no caught-up view attaches it.
func (c *output) notReady() string {
	if stuck := c.appliedStuck(); stuck != "" {
		return stuck
	}
	// The view caught up and has just ended; the hook naming why has not run yet.
	return stuckCatchingUp
}

// frames is a sequence of frames a writer sends one at a time, building each as it goes.
type frames func() (frame []byte, ok bool)

// snapshotFrames are a client's first frames: a put per entity, by key, a poisoned per
// poisoned key, by key, then synced at the snapshot's revision.
func snapshotFrames(s graphview.Snapshot[storedState]) frames {
	keys := slices.Sorted(maps.Keys(s.Entries))
	poisoned := slices.Sorted(maps.Keys(s.Poisoned))
	i := 0
	return func() ([]byte, bool) {
		defer func() { i++ }()
		switch {
		case i < len(keys):
			entry := s.Entries[keys[i]]
			return putFrame(keys[i], entry.Revision, entry.Value), true
		case i < len(keys)+len(poisoned):
			p := s.Poisoned[poisoned[i-len(keys)]]
			return poisonedFrame(p.Key, p.Revision, p), true
		case i == len(keys)+len(poisoned):
			return syncedFrame(s.Revision), true
		}
		return nil, false
	}
}

// batchFrames are the frames of one batch of changes, in the batch's revision order.
func batchFrames(batch []graphview.Delta[storedState]) frames {
	i := 0
	return func() ([]byte, bool) {
		if i == len(batch) {
			return nil, false
		}
		d := batch[i]
		i++
		switch d.Op {
		case graphview.DeltaUpsert:
			return putFrame(d.Key, d.Revision, d.Value), true
		case graphview.DeltaDelete:
			return deleteFrame(d.Key, d.Revision, d.Created), true
		default:
			return poisonedFrame(d.Key, d.Revision, d.Err), true
		}
	}
}

// stateFrame starts a frame of type kind on ENTITY_STATES.
func stateFrame(kind string, size int) []byte {
	frame := make([]byte, 0, size+128)
	frame = append(frame, `{"type":"`...)
	frame = append(frame, kind...)
	return append(frame, `","bucket":"`+graph.BucketEntityStates+`"`...)
}

// appendKeyRevision appends the key and revision fields.
func appendKeyRevision(frame []byte, key string, revision uint64) []byte {
	quoted, _ := json.Marshal(key) // a string always marshals
	frame = append(frame, `,"key":`...)
	frame = append(frame, quoted...)
	frame = append(frame, `,"revision":`...)
	return strconv.AppendUint(frame, revision, 10)
}

// putFrame is the put frame of key at revision: its KV write time in Unix milliseconds and the
// stored bytes unchanged as the payload, placed by hand as dataFrame places a subject message.
func putFrame(key string, revision uint64, state storedState) []byte {
	frame := appendKeyRevision(stateFrame("put", len(state.raw)+len(key)), key, revision)
	frame = append(frame, `,"timestamp":`...)
	frame = strconv.AppendInt(frame, state.created.UnixMilli(), 10)
	frame = append(frame, `,"payload":`...)
	frame = append(frame, state.raw...)
	return append(frame, '}')
}

// deleteFrame is the delete frame of key at revision, with the delete's KV write time.
func deleteFrame(key string, revision uint64, created time.Time) []byte {
	frame := appendKeyRevision(stateFrame("delete", len(key)), key, revision)
	frame = append(frame, `,"timestamp":`...)
	frame = strconv.AppendInt(frame, created.UnixMilli(), 10)
	return append(frame, '}')
}

// poisonedFrame is the poisoned frame of key at revision: the value stored there does not decode,
// and error is its classification code, graph_state_reset_required for every refusal of
// graph.UnmarshalEntityState, the view's one decoder.
func poisonedFrame(key string, revision uint64, err error) []byte {
	code := graph.ErrorCodeGraphStateResetRequired
	var classified *errs.ClassifiedError
	if errors.As(err, &classified) && classified.Code != "" {
		code = classified.Code
	}
	quoted, _ := json.Marshal(code)
	frame := appendKeyRevision(stateFrame("poisoned", len(key)), key, revision)
	frame = append(frame, `,"error":`...)
	frame = append(frame, quoted...)
	return append(frame, '}')
}

// syncedFrame is the marker after a client's snapshot: every frame before it is the state at
// revision, and every frame after it a change above it.
func syncedFrame(revision uint64) []byte {
	frame := append(stateFrame("synced", 0), `,"revision":`...)
	frame = strconv.AppendUint(frame, revision, 10)
	return append(frame, '}')
}
