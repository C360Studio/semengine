//go:build integration

package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/security"
)

// newApplied builds an initialized, unstarted applied-state output on a free loopback port with
// deps' NATS client, registry and security; the logger is the instance's capture.
func newApplied(t *testing.T, deps component.Dependencies) instance {
	t.Helper()
	return newAppliedPinging(t, deps, defaultPingInterval)
}

// newAppliedPinging is newApplied pinging each client every interval.
func newAppliedPinging(t *testing.T, deps component.Dependencies, interval string) instance {
	t.Helper()
	port := freePort(t)
	o := instance{
		listener: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		registry: deps.MetricsRegistry,
		logs:     &logged{},
	}
	deps.Logger = slog.New(o.logs)
	raw, err := json.Marshal(map[string]any{
		"ping_interval": interval,
		"ports": component.PortConfig{
			Inputs:  []component.PortDefinition{entityWatch()},
			Outputs: []component.PortDefinition{server(port)},
		},
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	built, err := create(json.RawMessage(raw), deps)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	o.c = built.(*output)
	if err := o.c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return o
}

// startApplied is newApplied started on client with a registry of its own; the test's cleanup stops
// it.
func startApplied(t *testing.T, client *natsclient.Client) instance {
	t.Helper()
	return started(t, newApplied(t, component.Dependencies{NATSClient: client, MetricsRegistry: metric.NewMetricsRegistry()}))
}

// awaitHealthy waits until o's Health is healthy: a view has caught up.
func awaitHealthy(t *testing.T, o instance) {
	t.Helper()
	await(t, "Health healthy", func(context.Context) (component.HealthStatus, error) {
		return o.c.Health(), nil
	}, func(h component.HealthStatus) bool { return h.Healthy })
}

// requireDegraded fails unless h is degraded naming want.
func requireDegraded(t *testing.T, h component.HealthStatus, want string) {
	t.Helper()
	if h.Healthy || h.Status != "degraded" || h.LastError != want {
		t.Fatalf("Health = %+v; want degraded naming %q", h, want)
	}
}

// entityStates creates ENTITY_STATES as its owner, graph-ingest, would, and returns the write
// handle the test uses in graph-ingest's place.
func entityStates(t *testing.T, client *natsclient.Client) jetstream.KeyValue {
	t.Helper()
	kv, err := kvcatalog.EnsureCatalogBucket(t.Context(), client, graph.BucketEntityStates)
	if err != nil {
		t.Fatalf("ensure %s: %v", graph.BucketEntityStates, err)
	}
	return kv
}

// entityID is the n-th test entity's literal six-part ID.
func entityID(n int) string { return fmt.Sprintf("acme.ops.test.system.widget.%03d", n) }

// storedEntity is a value graph.UnmarshalEntityState accepts for id: one statement whose object is
// value, with its source and timestamp.
func storedEntity(id, value string) []byte {
	return []byte(`{"id":"` + id + `","triples":[{"subject":"` + id + `","predicate":"test.state.value","object":"` +
		value + `","source":"test","timestamp":"2026-11-11T12:00:00Z"}]}`)
}

// notEntity is a value graph.UnmarshalEntityState refuses: it is not JSON.
var notEntity = []byte("not an entity")

func put(t *testing.T, kv jetstream.KeyValue, key string, value []byte) uint64 {
	t.Helper()
	rev, err := kv.Put(t.Context(), key, value)
	if err != nil {
		t.Fatalf("put %s: %v", key, err)
	}
	return rev
}

func del(t *testing.T, kv jetstream.KeyValue, key string) {
	t.Helper()
	if err := kv.Delete(t.Context(), key); err != nil {
		t.Fatalf("delete %s: %v", key, err)
	}
}

// lastWrite is key's last write as the bucket's history records it: its revision and whether it is
// a delete. The history is the oracle, not the frames.
func lastWrite(t *testing.T, kv jetstream.KeyValue, key string) (uint64, bool) {
	t.Helper()
	history, err := kv.History(t.Context(), key)
	if err != nil {
		t.Fatalf("history %s: %v", key, err)
	}
	last := history[len(history)-1]
	return last.Revision(), last.Operation() != jetstream.KeyValuePut
}

// readBack is the bucket's current value of each key, absent when deleted.
func readBack(t *testing.T, kv jetstream.KeyValue, keys ...string) map[string]string {
	t.Helper()
	state := map[string]string{}
	for _, key := range keys {
		entry, err := kv.Get(t.Context(), key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			continue
		}
		if err != nil {
			t.Fatalf("get %s: %v", key, err)
		}
		state[key] = string(entry.Value())
	}
	return state
}

// stateFrameJSON is the test's own reading of a state frame.
type stateFrameJSON struct {
	Type      string          `json:"type"`
	Bucket    string          `json:"bucket"`
	Key       string          `json:"key"`
	Revision  uint64          `json:"revision"`
	Timestamp int64           `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
	Error     string          `json:"error"`
}

// nextState is ws's next frame, which must be a state frame on ENTITY_STATES.
func nextState(t *testing.T, ws *wsClient) stateFrameJSON {
	t.Helper()
	frame := ws.next(t)
	var f stateFrameJSON
	if err := json.Unmarshal(frame, &f); err != nil {
		t.Fatalf("frame %s: %v", frame, err)
	}
	if f.Bucket != graph.BucketEntityStates {
		t.Fatalf("frame %s: bucket %q; want %s", frame, f.Bucket, graph.BucketEntityStates)
	}
	return f
}

// readSnapshot reads ws's frames through synced and returns those before it and synced's revision.
func readSnapshot(t *testing.T, ws *wsClient) ([]stateFrameJSON, uint64) {
	t.Helper()
	var frames []stateFrameJSON
	for {
		f := nextState(t, ws)
		if f.Type == "synced" {
			return frames, f.Revision
		}
		if f.Type != "put" && f.Type != "poisoned" {
			t.Fatalf("snapshot frame %+v before synced; want put or poisoned", f)
		}
		frames = append(frames, f)
	}
}

// readThrough reads ws's frames until one of key at revision rev or above, and returns them all.
// Every change applied before it is in its batch or an earlier one, ordered by revision.
func readThrough(t *testing.T, ws *wsClient, key string, rev uint64) []stateFrameJSON {
	t.Helper()
	var frames []stateFrameJSON
	for {
		f := nextState(t, ws)
		frames = append(frames, f)
		if f.Key == key && f.Revision >= rev {
			return frames
		}
	}
}

// fold applies frames to state as a client does: put sets the key, delete and poisoned remove it.
func fold(t *testing.T, state map[string]string, frames []stateFrameJSON) {
	t.Helper()
	for _, f := range frames {
		switch f.Type {
		case "put":
			state[f.Key] = string(f.Payload)
		case "delete", "poisoned":
			delete(state, f.Key)
		default:
			t.Fatalf("change frame %+v; want put, delete or poisoned", f)
		}
	}
}

// requireConverged fails unless, per key, the frames' revisions rise above synced and the last is
// the key's last write in the bucket, and the fold of every frame equals the bucket read back.
func requireConverged(t *testing.T, name string, kv jetstream.KeyValue, snapshot []stateFrameJSON, synced uint64,
	changes []stateFrameJSON, keys ...string) {
	t.Helper()
	last := map[string]uint64{}
	for _, f := range changes {
		if f.Revision <= synced || f.Revision <= last[f.Key] {
			t.Fatalf("%s: frame %+v after revision %d of its key (synced %d); want each key's revisions rising above synced",
				name, f, last[f.Key], synced)
		}
		last[f.Key] = f.Revision
	}
	for _, key := range keys {
		if want, _ := lastWrite(t, kv, key); want > synced && last[key] != want {
			t.Fatalf("%s: last frame of %s at revision %d; want its last write, %d", name, key, last[key], want)
		}
	}
	got := map[string]string{}
	fold(t, got, snapshot)
	fold(t, got, changes)
	if want := readBack(t, kv, keys...); !maps.Equal(got, want) {
		t.Fatalf("%s: folded state = %v\nwant the bucket read back = %v", name, got, want)
	}
}

// requireUnavailable fails unless a plain GET and a WebSocket dial of o's path are refused with 503,
// the GET's body naming reason.
func requireUnavailable(t *testing.T, o instance, reason string) {
	t.Helper()
	resp, err := http.Get("http://" + o.listener + "/ws")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusServiceUnavailable || !strings.Contains(string(body), reason) {
		t.Fatalf("GET = %d %q, %v; want 503 naming %q", resp.StatusCode, body, err, reason)
	}
	conn, resp, err := websocket.DefaultDialer.DialContext(t.Context(), o.url(), nil)
	if err == nil {
		_ = conn.Close()
		t.Fatalf("dial %s upgraded; want 503", o.url())
	}
	if resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("dial %s: %v, response %v; want 503", o.url(), err, resp)
	}
}

// TestAppliedStateLifecycleSuite runs the lifecycle suite on an applied-state output, whose view
// supervisor runs under the run context Stop ends. The must-fail owner's server TLS asks for mode
// "acme", which Start refuses before it binds.
func TestAppliedStateLifecycleSuite(t *testing.T) {
	client := fixtureClient(t)
	entityStates(t, client)
	factory := func() lifecycletest.Owner {
		return suiteOwner{newApplied(t, component.Dependencies{NATSClient: client}).c}
	}
	mustFail := func() lifecycletest.Owner {
		deps := component.Dependencies{NATSClient: client}
		deps.Security.TLS.Server = security.ServerTLSConfig{Enabled: true, Mode: "acme"}
		return suiteOwner{newApplied(t, deps).c}
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})
}

// TestAppliedStateSnapshotThenChanges: a late client receives a put of the entity written before
// it connected, carrying the stored bytes and the KV write time, then synced at that revision, then
// a put of the second entity and a delete of the first (output-websocket spec,
// "Applied state is a snapshot, a marker, then changes").
func TestAppliedStateSnapshotThenChanges(t *testing.T) {
	client := fixtureClient(t)
	kv := entityStates(t, client)
	first, second := entityID(1), entityID(2)
	stored := storedEntity(first, "a < b && c")
	rev1 := put(t, kv, first, stored)
	entry, err := kv.Get(t.Context(), first)
	if err != nil {
		t.Fatalf("get %s: %v", first, err)
	}
	o := startApplied(t, client)
	awaitHealthy(t, o)

	// The client attaches to the view before its upgrade is answered, so the writes after the dial
	// are changes after its snapshot. Writing them before reading means a missing synced shows as
	// the wrong next frame, not as a wait for a frame that never comes.
	ws := dial(t, o.url(), nil, true)
	rev2 := put(t, kv, second, storedEntity(second, "two"))
	del(t, kv, first)
	rev3, _ := lastWrite(t, kv, first)
	if f := nextState(t, ws); f.Type != "put" || f.Key != first || f.Revision != rev1 || string(f.Payload) != string(stored) ||
		f.Timestamp != entry.Created().UnixMilli() {
		t.Fatalf("first frame %+v; want put of %s at revision %d, written at %d, payload %s unchanged",
			f, first, rev1, entry.Created().UnixMilli(), stored)
	}
	if f := nextState(t, ws); f.Type != "synced" || f.Revision != rev1 {
		t.Fatalf("second frame %+v; want synced at revision %d", f, rev1)
	}

	if f := nextState(t, ws); f.Type != "put" || f.Key != second || f.Revision != rev2 {
		t.Fatalf("frame after synced %+v; want put of %s at revision %d", f, second, rev2)
	}
	if f := nextState(t, ws); f.Type != "delete" || f.Key != first || f.Revision != rev3 {
		t.Fatalf("next frame %+v; want delete of %s at revision %d", f, first, rev3)
	}
}

// TestAppliedStateConverges: back to back, one key is put twice, a second put then deleted, and a
// third put, poisoned and put again; a client connected before the burst and one attached in the
// middle of it each fold their frames to the bucket read back, with each key's revisions rising
// and its last frame at its last write, whichever coalescing window each write fell in
// (output-websocket spec, "Applied state converges to the current state").
func TestAppliedStateConverges(t *testing.T) {
	client := fixtureClient(t)
	kv := entityStates(t, client)
	o := startApplied(t, client)
	awaitHealthy(t, o)
	early := dial(t, o.url(), nil, true)
	earlySnapshot, earlySynced := readSnapshot(t, early)

	twice, gone, healed, end := entityID(1), entityID(2), entityID(3), entityID(9)
	put(t, kv, twice, storedEntity(twice, "first"))
	put(t, kv, twice, storedEntity(twice, "second"))
	put(t, kv, gone, storedEntity(gone, "gone"))
	late := dial(t, o.url(), nil, true)
	del(t, kv, gone)
	put(t, kv, healed, storedEntity(healed, "before"))
	put(t, kv, healed, notEntity)
	put(t, kv, healed, storedEntity(healed, "healed"))
	// The end key's frame is the last change: every write before it arrives in its batch or before.
	endRev := put(t, kv, end, storedEntity(end, "end"))

	lateSnapshot, lateSynced := readSnapshot(t, late)
	keys := []string{twice, gone, healed, end}
	requireConverged(t, "early client", kv, earlySnapshot, earlySynced, readThrough(t, early, end, endRev), keys...)
	requireConverged(t, "late client", kv, lateSnapshot, lateSynced, readThrough(t, late, end, endRev), keys...)
}

// TestNotReadyRefusesUpgrade: while ENTITY_STATES does not exist, an upgrade gets 503 naming why
// and Health is degraded; once the bucket exists and a view has caught up, an upgrade succeeds and
// the client receives synced (output-websocket spec, "No stream before the view is
// caught up").
func TestNotReadyRefusesUpgrade(t *testing.T) {
	client := fixtureClient(t)
	o := startApplied(t, client)
	// Start marks the view catching up before it returns, and only a view that has read the bucket
	// can mark it caught up.
	requireDegraded(t, o.c.Health(), stuckCatchingUp)
	requireUnavailable(t, o, stuckCatchingUp)

	kv := entityStates(t, client)
	rev := put(t, kv, entityID(1), storedEntity(entityID(1), "one"))
	awaitHealthy(t, o)
	snapshot, synced := readSnapshot(t, dial(t, o.url(), nil, true))
	if len(snapshot) != 1 || snapshot[0].Key != entityID(1) || synced != rev {
		t.Fatalf("snapshot %+v, synced %d; want the put of %s and synced at %d", snapshot, synced, entityID(1), rev)
	}
}

// TestPoisonedValueIsAnnounced: a value that does not decode is announced as poisoned with its
// revision and graph_state_reset_required, in the snapshot of a client that connects after it and
// as a change to a client already connected; a later valid write heals the key with a put.
func TestPoisonedValueIsAnnounced(t *testing.T) {
	client := fixtureClient(t)
	kv := entityStates(t, client)
	early, late := entityID(1), entityID(2)
	earlyRev := put(t, kv, early, notEntity)
	o := startApplied(t, client)
	awaitHealthy(t, o)
	ws := dial(t, o.url(), nil, true)
	snapshot, _ := readSnapshot(t, ws)
	if len(snapshot) != 1 || snapshot[0].Type != "poisoned" || snapshot[0].Key != early || snapshot[0].Revision != earlyRev ||
		snapshot[0].Error != graph.ErrorCodeGraphStateResetRequired || snapshot[0].Payload != nil {
		t.Fatalf("snapshot %+v; want poisoned %s at %d with %s and no payload", snapshot, early, earlyRev,
			graph.ErrorCodeGraphStateResetRequired)
	}

	put(t, kv, late, storedEntity(late, "valid"))
	readThrough(t, ws, late, 0)
	lateRev := put(t, kv, late, notEntity)
	if f := nextState(t, ws); f.Type != "poisoned" || f.Key != late || f.Revision != lateRev ||
		f.Error != graph.ErrorCodeGraphStateResetRequired {
		t.Fatalf("frame %+v; want poisoned %s at %d with %s", f, late, lateRev, graph.ErrorCodeGraphStateResetRequired)
	}
	healed := storedEntity(early, "healed")
	healRev := put(t, kv, early, healed)
	if f := nextState(t, ws); f.Type != "put" || f.Key != early || f.Revision != healRev || string(f.Payload) != string(healed) {
		t.Fatalf("frame %+v; want put of %s at %d healing it", f, early, healRev)
	}
}

// TestWatchLossClosesAndRecovers: deleting ENTITY_STATES loses the view's watch: a connected client
// is closed with 1013, Health is degraded and upgrades get 503 naming the loss, until the bucket
// exists again and a new view has caught up; a client then connecting gets a fresh snapshot, which
// holds only what the new bucket holds (output-websocket spec, "No stream before the
// view is caught up").
func TestWatchLossClosesAndRecovers(t *testing.T) {
	client := fixtureClient(t)
	kv := entityStates(t, client)
	put(t, kv, entityID(1), storedEntity(entityID(1), "old"))
	o := startApplied(t, client)
	awaitHealthy(t, o)
	ws := dial(t, o.url(), nil, true)
	readSnapshot(t, ws)

	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	if err := js.DeleteKeyValue(t.Context(), graph.BucketEntityStates); err != nil {
		t.Fatalf("delete %s: %v", graph.BucketEntityStates, err)
	}
	<-ws.ended
	if !websocket.IsCloseError(ws.err, websocket.CloseTryAgainLater) {
		t.Fatalf("client read ended with %v; want close 1013", ws.err)
	}
	// The view reports the loss once every client's changes have closed, so Health turns degraded
	// after the close; from live it turns only to the loss, until a new view catches up.
	requireDegraded(t, await(t, "Health degraded", func(context.Context) (component.HealthStatus, error) {
		return o.c.Health(), nil
	}, func(h component.HealthStatus) bool { return !h.Healthy }), stuckWatchLost)
	requireUnavailable(t, o, stuckWatchLost)
	awaitClients(t, o.c, 0) // detach counts the disconnection before it removes the client
	if n := value(t, o, "client_disconnections_total", "reason", disconnectWatchLost); n != 1 {
		t.Fatalf("watch_lost disconnections = %v; want 1", n)
	}

	kv = entityStates(t, client)
	rev := put(t, kv, entityID(2), storedEntity(entityID(2), "new"))
	awaitHealthy(t, o)
	snapshot, synced := readSnapshot(t, dial(t, o.url(), nil, true))
	if len(snapshot) != 1 || snapshot[0].Key != entityID(2) || synced != rev {
		t.Fatalf("snapshot after recovery %+v, synced %d; want only the put of %s and synced at %d",
			snapshot, synced, entityID(2), rev)
	}
}

// TestWatchLossStopsSnapshotDelivery: a watch lost while a client is still receiving its snapshot
// ends the snapshot there. The client is closed with 1013 and is never sent the lost view's synced
// once Health reports the loss (output-websocket spec, "No stream before the view is
// caught up"). Frames already in the socket buffers may still arrive after the loss; synced, the
// snapshot's last frame, is too far behind them to arrive unless the writer kept sending.
func TestWatchLossStopsSnapshotDelivery(t *testing.T) {
	client := fixtureClient(t)
	kv := entityStates(t, client)
	// Each entity is a 64 KiB frame, so the client's 32 KiB read buffer and the server's send buffer
	// hold only a few. Read one per pace tick, 80 frames take 40s, past the 20s or so the view takes to
	// see the deleted bucket (TestWatchLossClosesAndRecovers runs 25s); once Health reports the loss
	// the client reads without pausing, so the test does not wait out the snapshot.
	const entities = 80
	pad := strings.Repeat("x", 64<<10)
	for i := range entities {
		put(t, kv, entityID(i), storedEntity(entityID(i), pad))
	}
	o := startApplied(t, client)
	awaitHealthy(t, o)
	conn := dialConn(t, websocket.DefaultDialer, o.url(), nil)
	if tcp, ok := conn.UnderlyingConn().(*net.TCPConn); ok {
		if err := tcp.SetReadBuffer(32 << 10); err != nil {
			t.Fatalf("set the client's read buffer: %v", err)
		}
	}
	// The reader's record, read once ended is closed.
	var (
		afterLoss                         int // frames read once Health reported the loss
		syncedBeforeLoss, syncedAfterLoss bool
		readErr                           error
	)
	first := make(chan struct{})
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		pace := time.NewTicker(500 * time.Millisecond)
		defer pace.Stop()
		for n := 0; ; n++ {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				readErr = err
				return
			}
			lost := !o.c.Health().Healthy
			var f stateFrameJSON
			if err := json.Unmarshal(raw, &f); err != nil {
				readErr = fmt.Errorf("frame %.80s: %w", raw, err)
				return
			}
			if n == 0 {
				close(first)
			}
			switch {
			case lost:
				afterLoss++
				syncedAfterLoss = syncedAfterLoss || f.Type == "synced"
			case f.Type == "synced":
				syncedBeforeLoss = true
			default:
				<-pace.C
			}
		}
	}()
	t.Cleanup(func() {
		_ = conn.Close()
		<-ended
	})
	select {
	case <-first:
	case <-ended:
		t.Fatalf("client read ended before its first frame: %v", readErr)
	}

	js, err := client.JetStream()
	if err != nil {
		t.Fatalf("JetStream: %v", err)
	}
	if err := js.DeleteKeyValue(t.Context(), graph.BucketEntityStates); err != nil {
		t.Fatalf("delete %s: %v", graph.BucketEntityStates, err)
	}
	<-ended // orders the reads of the reader's record after its writes
	if syncedBeforeLoss {
		t.Fatalf("synced arrived before Health reported the loss: the snapshot outran the loss, so this run " +
			"did not lose the watch during delivery")
	}
	if syncedAfterLoss {
		t.Fatalf("sent synced from the lost view: %d frames read after Health reported the loss", afterLoss)
	}
	if !websocket.IsCloseError(readErr, websocket.CloseTryAgainLater) {
		t.Fatalf("client read ended with %v after %d frames read once the loss was reported; want close 1013",
			readErr, afterLoss)
	}
}

// stalledAppliedState: while one applied-state client reads nothing, another receives every round
// of writes to the same keys; once the stalled client's changes have merged, the merge is counted
// in changes_coalesced_total and logged once in the minute, naming client_behind, the listener and
// that the latest write still arrives. The stalled client, reading again, folds to the bucket read
// back (output-websocket spec, "A slow client never slows another").
func stalledAppliedState(t *testing.T) {
	client := fixtureClient(t)
	kv := entityStates(t, client)
	o := startApplied(t, client)
	awaitHealthy(t, o)
	stalled := dialConn(t, websocket.DefaultDialer, o.url(), nil)
	live := dial(t, o.url(), nil, true)
	liveSnapshot, liveSynced := readSnapshot(t, live)
	awaitClients(t, o.c, 2)

	// Each round writes 64 KiB to each of 32 keys, 2 MiB, and waits for the live client to receive
	// the round's last write. Once the stalled connection's socket buffers are full, its writer stops
	// taking batches, and a round overwrites changes still pending for it. On the developer's Mac
	// the first merge came in round 4 in each of three runs; maxRounds is ten times that, so a run
	// reaching it found no merge to count. A round bound, not a deadline: a slow machine runs the same
	// rounds more slowly.
	const maxRounds = 40
	pad := strings.Repeat("x", 64<<10)
	keys := make([]string, 32)
	for i := range keys {
		keys[i] = entityID(i)
	}
	var liveChanges []stateFrameJSON
	for round := 0; value(t, o, "changes_coalesced_total") == 0; round++ {
		if round == maxRounds {
			t.Fatalf("changes_coalesced_total = 0 after %d rounds of writes the stalled client did not read", round)
		}
		var last uint64
		for _, key := range keys {
			last = put(t, kv, key, storedEntity(key, strconv.Itoa(round)+pad))
		}
		liveChanges = append(liveChanges, readThrough(t, live, keys[len(keys)-1], last)...)
	}
	requireConverged(t, "live client", kv, liveSnapshot, liveSynced, liveChanges, keys...)

	// Reading again, the stalled client receives each key's latest write.
	endRev := put(t, kv, entityID(99), storedEntity(entityID(99), "end"))
	resumed := reading(t, stalled, true)
	snapshot, synced := readSnapshot(t, resumed)
	requireConverged(t, "stalled client", kv, snapshot, synced, readThrough(t, resumed, entityID(99), endRev),
		append(keys, entityID(99))...)

	// Stop joins the view, so every fan-out hook call has returned.
	stopOutput(t, o.c)
	warnings := o.logs.warnings(reasonClientBehind)
	if len(warnings) != 1 {
		t.Fatalf("client_behind warnings = %v; want one, the later merges within the minute counted only", warnings)
	}
	w := warnings[0]
	if w["listener"] != o.listener || w["dropped"] == "0" || !strings.Contains(w["msg"], "the latest write of each key still arrives") {
		t.Fatalf("client_behind warning = %v; want listener %s, a merged count, and that the latest write still arrives",
			w, o.listener)
	}
}
