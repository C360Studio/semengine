//go:build integration

package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	dto "github.com/prometheus/client_model/go"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/harness/lifecycletest"
	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
)

// shared is this package's NATS server. It starts at the first reset; TestMain removes it after the
// tests.
var shared natsfixture.Shared

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	code := m.Run()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := shared.Stop(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return code
}

func fixtureClient(t *testing.T) *natsclient.Client {
	t.Helper()
	return natsfixture.Open(t, shared.Reset(t), openClient)
}

func openClient(ctx context.Context, url string) (*natsclient.Client, func(context.Context) error, error) {
	client, err := natsclient.NewClient(url,
		natsclient.WithTimeout(15*time.Second),
		natsclient.WithMaxReconnects(0),
		natsclient.WithHealthInterval(0),
	)
	if err != nil {
		return nil, nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	if err := client.WaitForConnection(ctx); err != nil {
		return nil, nil, errors.Join(err, client.Close(ctx))
	}
	return client, client.Close, nil
}

// logged captures the records the component logs through Dependencies.Logger.
type logged struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *logged) Enabled(context.Context, slog.Level) bool { return true }

func (h *logged) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *logged) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *logged) WithGroup(string) slog.Handler      { return h }

// warnings returns the warnings logged with reason, each as its attributes by key with the message
// under "msg".
func (h *logged) warnings(reason string) []map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var found []map[string]string
	for _, r := range h.records {
		if r.Level != slog.LevelWarn {
			continue
		}
		attrs := map[string]string{"msg": r.Message}
		r.Attrs(func(a slog.Attr) bool {
			attrs[a.Key] = a.Value.String()
			return true
		})
		if attrs["reason"] == reason {
			found = append(found, attrs)
		}
	}
	return found
}

// instance is one websocket output under test: the component, the address it listens on, the
// metrics registry it was built with and what it logged.
type instance struct {
	c        *output
	listener string
	registry *metric.MetricsRegistry
	logs     *logged
}

func (o instance) url() string { return "ws://" + o.listener + "/ws" }

// newOutput builds an initialized, unstarted output streaming subject on a free loopback port,
// with a registry of its own.
func newOutput(t *testing.T, client *natsclient.Client, subject, interval string) instance {
	t.Helper()
	return newOutputWith(t, subject, interval, component.Dependencies{
		NATSClient: client, MetricsRegistry: metric.NewMetricsRegistry(),
	})
}

// newOutputWith is newOutput with deps' NATS client, metrics registry and security configuration;
// the logger is the instance's capture.
func newOutputWith(t *testing.T, subject, interval string, deps component.Dependencies) instance {
	t.Helper()
	port := freePort(t)
	o := instance{
		listener: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		registry: deps.MetricsRegistry,
		logs:     &logged{},
	}
	deps.Logger = slog.New(o.logs)
	built, err := create(json.RawMessage(subjectConfig(t, subject, port, interval)), deps)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	o.c = built.(*output)
	if err := o.c.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return o
}

// startOutput is newOutput started; the test's cleanup stops it.
func startOutput(t *testing.T, client *natsclient.Client, subject, interval string) instance {
	t.Helper()
	return started(t, newOutput(t, client, subject, interval))
}

// started starts o; the test's cleanup stops it.
func started(t *testing.T, o instance) instance {
	t.Helper()
	if err := o.c.Start(t.Context()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopOutput(t, o.c) })
	return o
}

// stopBound bounds a cleanup Stop: long enough for any healthy Stop, so the bound never decides a
// test.
const stopBound = time.Hour

func stopOutput(t *testing.T, c *output) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), stopBound)
	defer cancel()
	if err := c.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

// await polls observe until done accepts its value; go test -timeout is the one deadline.
func await[T any](t *testing.T, what string, observe func(context.Context) (T, error), done func(T) bool) T {
	t.Helper()
	v, err := probe.Await(t.Context(), observe, done)
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	return v
}

// awaitClients waits until n clients are attached, so a message published next is sent to each.
// A client's dial returns once the upgrade is answered, which is before its handler attaches it.
func awaitClients(t *testing.T, c *output, n int) {
	t.Helper()
	await(t, fmt.Sprintf("%d clients attached", n), func(context.Context) (int, error) {
		c.clientsMu.Lock()
		defer c.clientsMu.Unlock()
		return len(c.clients), nil
	}, func(got int) bool { return got == n })
}

// sample reads semengine_websocket_<name> for o's listener and the label pairs in labels (name,
// value, ...) from o's registry by name, or nil when it has no such sample.
func sample(o instance, name string, labels ...string) (*dto.Metric, error) {
	families, err := o.registry.PrometheusRegistry().Gather()
	if err != nil {
		return nil, err
	}
	want := map[string]string{"listener": o.listener}
	for i := 0; i+1 < len(labels); i += 2 {
		want[labels[i]] = labels[i+1]
	}
	for _, family := range families {
		if family.GetName() != "semengine_websocket_"+name {
			continue
		}
		for _, m := range family.GetMetric() {
			got := map[string]string{}
			for _, pair := range m.GetLabel() {
				got[pair.GetName()] = pair.GetValue()
			}
			if maps.Equal(got, want) {
				return m, nil
			}
		}
	}
	return nil, nil
}

// value is the counter or gauge value of sample, 0 when there is no sample; a failed gather fails
// the test.
func value(t *testing.T, o instance, name string, labels ...string) float64 {
	t.Helper()
	m, err := sample(o, name, labels...)
	if err != nil {
		t.Fatalf("gather %s: %v", name, err)
	}
	if m == nil {
		return 0
	}
	if m.Counter != nil {
		return m.GetCounter().GetValue()
	}
	return m.GetGauge().GetValue()
}

func publish(t *testing.T, client *natsclient.Client, subject string, data []byte) {
	t.Helper()
	if err := client.Publish(t.Context(), subject, data); err != nil {
		t.Fatalf("publish %s: %v", subject, err)
	}
}

// wsClient is a test client reading every frame into frames until its read fails, when err is set
// and ended closes. Each ping it receives is signalled on pings; it answers them when told to.
type wsClient struct {
	conn   *websocket.Conn
	frames chan []byte
	pings  chan struct{}
	ended  chan struct{}
	err    error
}

// dial connects a reading client to url with header; its cleanup closes it and joins its reader.
func dial(t *testing.T, url string, header http.Header, answerPings bool) *wsClient {
	t.Helper()
	return reading(t, dialConn(t, websocket.DefaultDialer, url, header), answerPings)
}

// reading reads every frame conn receives into a wsClient; its cleanup closes conn and joins its
// reader.
func reading(t *testing.T, conn *websocket.Conn, answerPings bool) *wsClient {
	t.Helper()
	c := &wsClient{conn: conn, frames: make(chan []byte, 1024), pings: make(chan struct{}, 1024), ended: make(chan struct{})}
	answer := conn.PingHandler()
	conn.SetPingHandler(func(data string) error {
		select {
		case c.pings <- struct{}{}:
		default:
		}
		if !answerPings {
			return nil
		}
		return answer(data)
	})
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
	t.Cleanup(func() {
		_ = conn.Close()
		for {
			select {
			case <-c.ended:
				return
			case <-c.frames:
			}
		}
	})
	return c
}

// dialConn connects to url with header through dialer and does not read; its cleanup closes it.
func dialConn(t *testing.T, dialer *websocket.Dialer, url string, header http.Header) *websocket.Conn {
	t.Helper()
	conn, resp, err := dialer.DialContext(t.Context(), url, header)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// next returns the client's next frame, failing the test if its read has failed instead.
func (c *wsClient) next(t *testing.T) []byte {
	t.Helper()
	select {
	case frame := <-c.frames:
		return frame
	case <-c.ended:
		t.Fatalf("client read ended before the next frame: %v", c.err)
		return nil
	}
}

// dataFrameJSON is the test's own reading of a data frame.
type dataFrameJSON struct {
	Type      string          `json:"type"`
	Subject   string          `json:"subject"`
	Timestamp int64           `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

func decodeData(t *testing.T, frame []byte) dataFrameJSON {
	t.Helper()
	var f dataFrameJSON
	if err := json.Unmarshal(frame, &f); err != nil {
		t.Fatalf("frame %s: %v", frame, err)
	}
	if f.Type != "data" {
		t.Fatalf("frame type = %q, want data: %s", f.Type, frame)
	}
	return f
}

// requireDialRefused fails the test unless a dial of address is refused.
func requireDialRefused(t *testing.T, address string) {
	t.Helper()
	if conn, err := net.Dial("tcp4", address); err == nil {
		_ = conn.Close()
		t.Fatalf("dial %s connected; want it refused", address)
	}
}

// TestWebsocketLifecycleSuite runs the lifecycle suite on the websocket output. The must-fail
// owner's NATS client never connected, so its Start fails at the subscribe after binding its port.
// The component is one-shot, so no restart is promised.
func TestWebsocketLifecycleSuite(t *testing.T) {
	client := fixtureClient(t)
	factory := func() lifecycletest.Owner { return suiteOwner{newOutput(t, client, "ws.suite", "30s").c} }
	mustFail := func() lifecycletest.Owner {
		unconnected, err := natsclient.NewClient("")
		if err != nil {
			t.Fatalf("natsclient.NewClient: %v", err)
		}
		return suiteOwner{newOutput(t, unconnected, "ws.suite", "30s").c}
	}
	lifecycletest.Run(t, factory, mustFail, lifecycletest.Promise{})
}

// TestStartContextEndingStopsNothing: ending Start's context after Start returned stops nothing: a
// client connects and receives a message published afterwards. Stop then closes the client with
// 1001, releases the port and leaves no client attached, with clients_connected 0
// (output-websocket spec, "Start bounds start; Stop is the only stop").
func TestStartContextEndingStopsNothing(t *testing.T) {
	client := fixtureClient(t)
	o := newOutput(t, client, "ws.start", "30s")
	startCtx, cancelStart := context.WithCancel(t.Context())
	if err := o.c.Start(startCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { stopOutput(t, o.c) })
	cancelStart()

	ws := dial(t, o.url(), nil, true)
	awaitClients(t, o.c, 1)
	// attach sets clients_connected under the lock awaitClients reads the clients under.
	if n := value(t, o, "clients_connected"); n != 1 {
		t.Fatalf("clients_connected = %v with one client attached; want 1", n)
	}
	publish(t, client, "ws.start", []byte(`{"after":"start"}`))
	if got := decodeData(t, ws.next(t)); string(got.Payload) != `{"after":"start"}` {
		t.Fatalf("payload = %s, want the message published after Start's context ended", got.Payload)
	}

	stopOutput(t, o.c)
	requireClosedByStop(t, ws)
	requireDialRefused(t, o.listener)
	if held := (suiteOwner{o.c}).Observe().Unresolved; len(held) != 0 {
		t.Fatalf("after Stop the output holds %v; want nothing", held)
	}
	if n := value(t, o, "clients_connected"); n != 0 {
		t.Fatalf("clients_connected = %v after Stop; want 0", n)
	}
}

// requireClosedByStop fails the test unless ws's read ended with Stop's close 1001 and no frame
// was left unread before it. ws's reader fills frames before it closes ended, so after ended the
// frames left are final.
func requireClosedByStop(t *testing.T, ws *wsClient) {
	t.Helper()
	<-ws.ended // Stop has returned, so the server has closed the connection
	if !websocket.IsCloseError(ws.err, websocket.CloseGoingAway) {
		t.Fatalf("client read ended with %v; want close 1001", ws.err)
	}
	if extra := len(ws.frames); extra != 0 {
		t.Fatalf("client received %d frames more than expected: the first is %s", extra, <-ws.frames)
	}
}

// TestSubjectFanOut: a JSON message reaches every client as one data frame whose payload is the
// published bytes unchanged; a message that is not JSON is sent to none, counted under not_json and
// logged once, naming the reason, the listener, the subject and that later messages still flow. A
// client sending any Origin is accepted (output-websocket spec, "A subject is delivered
// at most once, unchanged").
func TestSubjectFanOut(t *testing.T) {
	client := fixtureClient(t)
	o := startOutput(t, client, "ws.fanout", "30s")
	clients := []*wsClient{
		dial(t, o.url(), nil, true),
		dial(t, o.url(), http.Header{"Origin": {"https://elsewhere.example"}}, true),
	}
	awaitClients(t, o.c, 2)

	// Spaces, an escaped é, <, & and a trailing zero: a re-encode of the payload would change each.
	payload := []byte(`{"text": "a < b && c > d", "name": "caf\u00e9", "n": 1.50}`)
	before := time.Now().UnixMilli()
	publish(t, client, "ws.fanout", []byte("not json"))
	publish(t, client, "ws.fanout", payload)
	for i, ws := range clients {
		got := decodeData(t, ws.next(t))
		if got.Subject != "ws.fanout" || string(got.Payload) != string(payload) {
			t.Fatalf("client %d frame subject %q payload %s; want ws.fanout and %s unchanged", i, got.Subject, got.Payload, payload)
		}
		if after := time.Now().UnixMilli(); got.Timestamp < before || got.Timestamp > after {
			t.Fatalf("client %d frame timestamp %d; want the receive time, within [%d, %d]", i, got.Timestamp, before, after)
		}
	}

	// Stop drains the subscription, so both messages have been handled once it returns; each client
	// then reads Stop's close frame, with nothing sent for the message that is not JSON.
	stopOutput(t, o.c)
	for _, ws := range clients {
		requireClosedByStop(t, ws)
	}
	if n := value(t, o, "messages_dropped_total", "reason", reasonNotJSON); n != 1 {
		t.Fatalf("not_json dropped = %v; want 1", n)
	}
	warnings := o.logs.warnings(reasonNotJSON)
	if len(warnings) != 1 {
		t.Fatalf("not_json warnings = %v; want one", warnings)
	}
	w := warnings[0]
	if w["listener"] != o.listener || w["subject"] != "ws.fanout" || w["dropped"] != "1" ||
		!strings.Contains(w["msg"], "later messages still flow") {
		t.Fatalf("not_json warning = %v; want listener %s, subject ws.fanout, dropped 1, and that later messages still flow",
			w, o.listener)
	}
}

// fullestQueue is the most frames any attached client has queued. The test reads it only to know
// when the stalled client's queue is full; what it asserts is what clients, the counter and the log
// show.
func fullestQueue(c *output) int {
	c.clientsMu.Lock()
	defer c.clientsMu.Unlock()
	most := 0
	for cl := range c.clients {
		cl.mu.Lock()
		most = max(most, len(cl.frames))
		cl.mu.Unlock()
	}
	return most
}

// TestStalledClientDoesNotSlowOthers: a client that stops reading delays no other, on either input
// (output-websocket spec, "A slow client never slows another").
func TestStalledClientDoesNotSlowOthers(t *testing.T) {
	t.Run("subject", stalledSubject)
	t.Run("applied_state", stalledAppliedState)
}

// stalledSubject: while one client reads nothing, another receives every frame in order, each
// before the next is published, through the stalled client's queue filling and two frames more;
// the shed is counted under client_queue_full and logged once in the minute, naming the reason, the
// listener and that other clients are unaffected.
func stalledSubject(t *testing.T) {
	client := fixtureClient(t)
	o := startOutput(t, client, "ws.stall", "30s")
	stalled := dialConn(t, websocket.DefaultDialer, o.url(), nil)
	live := dial(t, o.url(), nil, true)
	awaitClients(t, o.c, 2)

	// Frames of 16 KiB fill the stalled connection's socket buffers, then its queue. The live
	// client's queue never holds more than the one frame it is about to read.
	pad := strings.Repeat("x", 16<<10)
	send := func(seq int) {
		publish(t, client, "ws.stall", []byte(fmt.Sprintf(`{"seq":%d,"pad":"%s"}`, seq, pad)))
		var got struct {
			Seq int `json:"seq"`
		}
		if err := json.Unmarshal(decodeData(t, live.next(t)).Payload, &got); err != nil || got.Seq != seq {
			t.Fatalf("live client frame seq %d, %v; want %d", got.Seq, err, seq)
		}
	}
	seq := 0
	for ; fullestQueue(o.c) < clientQueueFrames; seq++ {
		send(seq)
	}
	// Each of these drops the stalled client's oldest queued frame.
	send(seq)
	send(seq + 1)

	// Closing the stalled client ends the write it is blocked in; Stop then drains the
	// subscription, so every drop has been counted and logged once it returns.
	if err := stalled.Close(); err != nil {
		t.Fatalf("close the stalled client: %v", err)
	}
	stopOutput(t, o.c)
	if n := value(t, o, "messages_dropped_total", "reason", reasonClientQueueFull); n < 2 {
		t.Fatalf("client_queue_full dropped = %v; want at least 2", n)
	}
	warnings := o.logs.warnings(reasonClientQueueFull)
	if len(warnings) != 1 {
		t.Fatalf("client_queue_full warnings = %v; want one, the later drops within the minute counted only", warnings)
	}
	w := warnings[0]
	if w["listener"] != o.listener || w["dropped"] != "1" || !strings.Contains(w["msg"], "other clients and later frames are unaffected") {
		t.Fatalf("client_queue_full warning = %v; want listener %s, dropped 1, and that other clients are unaffected", w, o.listener)
	}
}

// TestQuietClientStaysConnected: a client that answers pings and sends nothing stays connected
// across three read deadlines (six pings at 300ms, each deadline twice that) and still receives
// the next frame (output-websocket spec, "Keep-alive"). Without the pong extending
// the read deadline the server closes it at the first deadline.
func TestQuietClientStaysConnected(t *testing.T) {
	client := fixtureClient(t)
	o := startOutput(t, client, "ws.quiet", "300ms")
	ws := dial(t, o.url(), nil, true)
	awaitClients(t, o.c, 1)
	for pings := 0; pings < 6; {
		select {
		case <-ws.pings:
			pings++
		case <-ws.ended:
			t.Fatalf("client closed after %d pings: %v", pings, ws.err)
		}
	}
	publish(t, client, "ws.quiet", []byte(`{"still":"here"}`))
	if got := decodeData(t, ws.next(t)); string(got.Payload) != `{"still":"here"}` {
		t.Fatalf("payload = %s, want {\"still\":\"here\"}", got.Payload)
	}
}

// TestBusyClientKeepsPings: a client that answers pings and reads a frame every 5ms stays connected
// for six pings at 200ms while the 160 frames of 256 KiB of its applied-state snapshot are written
// to it (output-websocket spec, "Keep-alive"). The 5ms ticker is the client's
// processing time, not an ordering: it keeps the writer busy for 800ms, twice the read deadline. A
// writer that writes every frame it has before it pings sends no ping meanwhile, and the server
// closes the client at its read deadline. Snapshot, subject
// queue and change batches share that loop (drain).
//
// The writer's frames must be ready faster than the client reads them, or the writer runs out and
// pings between them whether or not drain does. A snapshot is: it is in memory before the client
// connects. Subject frames are not: under -race the subscription took about 28 MB/s, so only small
// frames back up behind a client reading every 5ms. A ping also reaches the client behind every
// frame already in the server's socket send buffer, which the test cannot size: on Linux it held up
// to 1.3 MB, 39 frames of 32 KiB, about 200ms of reading, and the server closed the client for
// read_timeout (CI run 43076963073). In 256 KiB frames that is five frames, 25ms of reading; even a
// 4 MiB buffer, Linux's default tcp_wmem maximum, is 80ms.
func TestBusyClientKeepsPings(t *testing.T) {
	client := fixtureClient(t)
	kv := entityStates(t, client)
	pad := strings.Repeat("x", 256<<10)
	for n := range 160 {
		put(t, kv, entityID(n), storedEntity(entityID(n), pad))
	}
	o := started(t, newAppliedPinging(t,
		component.Dependencies{NATSClient: client, MetricsRegistry: metric.NewMetricsRegistry()}, "200ms"))
	// Healthy means the view has caught up, so the client's snapshot holds every entity.
	awaitHealthy(t, o)
	// A small receive buffer, set before the upgrade, keeps the snapshot in the writer rather than
	// in the client's socket.
	dialer := &websocket.Dialer{NetDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		if err := conn.(*net.TCPConn).SetReadBuffer(32 << 10); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return conn, nil
	}}
	conn := dialConn(t, dialer, o.url(), nil)
	pings := make(chan struct{}, 64)
	answer := conn.PingHandler()
	conn.SetPingHandler(func(data string) error {
		select {
		case pings <- struct{}{}:
		default:
		}
		return answer(data)
	})
	type result struct {
		frames int
		err    error
	}
	ended := make(chan result, 1)
	quit := make(chan struct{})
	go func() {
		pace := time.NewTicker(5 * time.Millisecond)
		defer pace.Stop()
		frames := 0
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				ended <- result{frames, err}
				return
			}
			frames++
			select {
			case <-pace.C:
			case <-quit:
				ended <- result{frames, nil}
				return
			}
		}
	}()
	t.Cleanup(func() {
		close(quit)
		_ = conn.Close()
		<-ended
	})
	for i := 0; i < 6; i++ {
		select {
		case <-pings:
		case r := <-ended:
			ended <- r // for the cleanup's join
			t.Fatalf("client closed after %d pings and %d frames: %v", i, r.frames, r.err)
		}
	}
}

// TestSilentClientIsClosed: a client that answers no pings is closed once its read deadline, twice
// ping_interval, passes, without the close frame Stop sends, is no longer attached, and is counted
// under client_disconnections_total{reason="read_timeout"} (output-websocket spec,
// "Keep-alive"). Still connected after six pings, three deadlines, fails.
func TestSilentClientIsClosed(t *testing.T) {
	client := fixtureClient(t)
	o := startOutput(t, client, "ws.silent", "100ms")
	ws := dial(t, o.url(), nil, false)
	awaitClients(t, o.c, 1)
	for pings, ended := 0, false; !ended; {
		select {
		case <-ws.pings:
			if pings++; pings == 6 {
				t.Fatalf("client still connected after %d pings it did not answer", pings)
			}
		case <-ws.ended:
			ended = true
		}
	}
	if websocket.IsCloseError(ws.err, websocket.CloseGoingAway) {
		t.Fatalf("client read ended with %v; want the connection closed for its read deadline, not a stop", ws.err)
	}
	// detach counts the disconnection before it removes the client awaitClients waits on.
	awaitClients(t, o.c, 0)
	if n := value(t, o, "client_disconnections_total", "reason", "read_timeout"); n != 1 {
		t.Fatalf("read_timeout disconnections = %v; want 1", n)
	}
}

// TestStopWithEndedContextClosesClientsAndCancelsSupervisorThenRetries checks that Stop handed
// an already-expired context still closes connected WebSocket clients with 1001 and cancels the
// applied-state view supervisor before a retry Stop joins cleanly.
func TestStopWithEndedContextClosesClientsAndCancelsSupervisorThenRetries(t *testing.T) {
	t.Run("subject_closes_clients", func(t *testing.T) {
		client := fixtureClient(t)
		o := startOutput(t, client, "ws.ended", "30s")
		ws := dial(t, o.url(), nil, true)
		awaitClients(t, o.c, 1)

		expired, cancelExpired := context.WithDeadline(t.Context(), time.Unix(1, 0))
		defer cancelExpired()
		if err := o.c.Stop(expired); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Stop(expired) = %v, want context.DeadlineExceeded", err)
		}

		waitBound, cancelWait := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancelWait()
		select {
		case <-ws.ended:
		case <-waitBound.Done():
			t.Fatal("Stop(expired) did not close connected client")
		}
		requireClosedByStop(t, ws)

		stopOutput(t, o.c)
		if held := (suiteOwner{o.c}).Observe().Unresolved; len(held) != 0 {
			t.Fatalf("after retry Stop the output holds %v; want nothing", held)
		}
	})

	t.Run("applied_state_cancels_supervisor", func(t *testing.T) {
		client := fixtureClient(t)
		entityStates(t, client)
		o := startApplied(t, client)
		awaitHealthy(t, o)
		ws := dial(t, o.url(), nil, true)
		readSnapshot(t, ws)
		awaitClients(t, o.c, 1)
		supervised := o.c.supervised

		expired, cancelExpired := context.WithDeadline(t.Context(), time.Unix(1, 0))
		defer cancelExpired()
		if err := o.c.Stop(expired); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Stop(expired) = %v, want context.DeadlineExceeded", err)
		}

		waitBound, cancelWait := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancelWait()
		select {
		case <-supervised:
		case <-waitBound.Done():
			t.Fatal("Stop(expired) did not cancel applied-state view supervisor")
		}
		select {
		case <-ws.ended:
		case <-waitBound.Done():
			t.Fatal("Stop(expired) did not close applied-state client")
		}
		requireClosedByStop(t, ws)

		stopOutput(t, o.c)
		if held := (suiteOwner{o.c}).Observe().Unresolved; len(held) != 0 {
			t.Fatalf("after retry Stop the output holds %v; want nothing", held)
		}
	})
}
