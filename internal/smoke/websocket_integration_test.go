//go:build integration

package smoke

import (
	"context"
	"encoding/json"
	"maps"
	"net"
	"slices"
	"strconv"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/c360studio/semengine/graph"
)

// wsClient is a client of the stack's websocket output, reading every frame into frames until its
// read fails, when err is set and ended closes. gorilla's default ping handler answers the
// output's pings while it reads.
type wsClient struct {
	conn   *websocket.Conn
	frames chan []byte
	ended  chan struct{}
	err    error
}

// dialWebsocket connects a reading client to the stack's websocket output on 127.0.0.1, where it
// listens. Until the output's view of ENTITY_STATES is ready, an upgrade is refused with 503, so
// the dial is retried until one is accepted. The cleanup closes the connection and joins its
// reader.
func dialWebsocket(t *testing.T, s stack) *wsClient {
	t.Helper()
	url := "ws://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(s.websocketPort)) + "/ws"
	conn := await(t, s, "the websocket output to accept a client at "+url,
		func(ctx context.Context) (*websocket.Conn, error) {
			conn, response, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
			if response != nil {
				_ = response.Body.Close()
			}
			return conn, err
		}, func(conn *websocket.Conn) bool { return conn != nil })
	c := &wsClient{conn: conn, frames: make(chan []byte, 1024), ended: make(chan struct{})}
	go func() {
		defer close(c.ended)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				c.err = err
				return
			}
			c.frames <- data
		}
	}()
	t.Cleanup(c.close)
	return c
}

// close closes the client's connection and joins its reader; a second close does nothing more.
func (c *wsClient) close() {
	_ = c.conn.Close()
	for {
		select {
		case <-c.ended:
			return
		case <-c.frames:
		}
	}
}

// wsFrame is the test's own reading of a state frame.
type wsFrame struct {
	Type     string          `json:"type"`
	Bucket   string          `json:"bucket"`
	Key      string          `json:"key"`
	Revision uint64          `json:"revision"`
	Payload  json.RawMessage `json:"payload"`
}

// next returns the client's next frame, which must be a state frame on ENTITY_STATES.
func (c *wsClient) next(t *testing.T) wsFrame {
	t.Helper()
	var data []byte
	select {
	case data = <-c.frames:
	case <-c.ended:
		t.Fatalf("websocket client read ended before the next frame: %v", c.err)
	}
	var f wsFrame
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("websocket frame %s: %v", data, err)
	}
	if f.Bucket != graph.BucketEntityStates {
		t.Fatalf("websocket frame %s: bucket %q, want %s", data, f.Bucket, graph.BucketEntityStates)
	}
	return f
}

// stored is one key's state: the revision and the value, as the bucket holds it or as frames fold.
type stored struct {
	revision uint64
	value    string
}

// readSnapshot reads the client's snapshot through synced, folding each put into state, and
// returns synced's revision.
func (c *wsClient) readSnapshot(t *testing.T, state map[string]stored) uint64 {
	t.Helper()
	for {
		f := c.next(t)
		if f.Type == "synced" {
			return f.Revision
		}
		if f.Type != "put" {
			t.Fatalf("snapshot frame %+v before synced, want put", f)
		}
		state[f.Key] = stored{revision: f.Revision, value: string(f.Payload)}
	}
}

// readThrough reads change frames, folding each into state as a client does, until one at revision
// or above. Frames arrive in revision order, so every change at or below revision is folded then.
func (c *wsClient) readThrough(t *testing.T, state map[string]stored, revision uint64) {
	t.Helper()
	for {
		f := c.next(t)
		switch f.Type {
		case "put":
			state[f.Key] = stored{revision: f.Revision, value: string(f.Payload)}
		case "delete":
			delete(state, f.Key)
		default:
			t.Fatalf("change frame %+v, want put or delete", f)
		}
		if f.Revision >= revision {
			return
		}
	}
}

// readEntityStates reads every key of ENTITY_STATES back, with its revision and value.
func (o observer) readEntityStates(t *testing.T) map[string]stored {
	t.Helper()
	kv, err := o.js.KeyValue(t.Context(), graph.BucketEntityStates)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketEntityStates, err)
	}
	keys, err := kv.ListKeys(t.Context())
	if err != nil {
		t.Fatalf("list %s: %v", graph.BucketEntityStates, err)
	}
	state := map[string]stored{}
	for key := range keys.Keys() {
		entry, err := kv.Get(t.Context(), key)
		if err != nil {
			t.Fatalf("read %s/%s: %v", graph.BucketEntityStates, key, err)
		}
		state[key] = stored{revision: entry.Revision(), value: string(entry.Value())}
	}
	return state
}

// checkWebsocket reads c's change frames from folded, the revision state already holds, through
// revision, folding them into state, and fails unless the fold equals ENTITY_STATES read back,
// revision and value per key, and holds exactly w's entities. revision is the bucket's revision
// with nothing written after it, so the read-back is the state the frames must reach.
func checkWebsocket(t *testing.T, o observer, c *wsClient, state map[string]stored, folded, revision uint64, w want) {
	t.Helper()
	if folded < revision {
		c.readThrough(t, state, revision)
	}
	bucket := o.readEntityStates(t)
	keys := append(slices.Collect(maps.Keys(state)), slices.Collect(maps.Keys(bucket))...)
	slices.Sort(keys)
	for _, key := range slices.Compact(keys) {
		folded, inFold := state[key]
		read, inBucket := bucket[key]
		if inFold != inBucket || folded != read {
			t.Errorf("websocket client's folded %s: held %v, revision %d, value %s; want %s read back: held %v, revision %d, value %s",
				key, inFold, folded.revision, folded.value, graph.BucketEntityStates, inBucket, read.revision, read.value)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	if got, wantIDs := slices.Sorted(maps.Keys(state)), slices.Sorted(maps.Keys(w)); !slices.Equal(got, wantIDs) {
		t.Fatalf("websocket client holds %v, want the entities %v", got, wantIDs)
	}
}

// awaitStreamed waits until the websocket output's view has applied ENTITY_STATES through revision:
// a client it accepts reads its snapshot, and the changes after it, through that revision. It is
// the output's ordering event, as an index's GRAPH_STATUS key at the revision is the index's: its
// health reads degraded until its view is caught up, and an upgrade is refused until then.
func awaitStreamed(t *testing.T, s stack, revision uint64) {
	t.Helper()
	c := dialWebsocket(t, s)
	defer c.close()
	state := map[string]stored{}
	if synced := c.readSnapshot(t, state); synced < revision {
		c.readThrough(t, state, revision)
	}
}
