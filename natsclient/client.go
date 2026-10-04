// Package natsclient provides a client for managing NATS connections with circuit breaker pattern.
package natsclient

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/internal/resource"
	"github.com/c360studio/semengine/pkg/errs"
)

// ConnectionStatus represents the state of the NATS connection
type ConnectionStatus int

// Possible connection statuses
const (
	StatusDisconnected ConnectionStatus = iota
	StatusConnecting
	StatusConnected
	StatusReconnecting
	StatusCircuitOpen
)

// String returns the string representation of ConnectionStatus
func (s ConnectionStatus) String() string {
	switch s {
	case StatusDisconnected:
		return "disconnected"
	case StatusConnecting:
		return "connecting"
	case StatusConnected:
		return "connected"
	case StatusReconnecting:
		return "reconnecting"
	case StatusCircuitOpen:
		return "circuit_open"
	default:
		return "unknown"
	}
}

// Error messages
var (
	ErrNotConnected = stderrors.New("not connected to NATS")
	ErrCircuitOpen  = stderrors.New("circuit breaker is open")
)

// Status holds runtime status information for the NATS manager
type Status struct {
	Status          ConnectionStatus
	FailureCount    int32
	LastFailureTime time.Time
	Reconnects      int32
	RTT             time.Duration
}

// Client manages NATS connections with circuit breaker pattern
type Client struct {
	urls     string       // comma-separated NATS server URLs for clustering support
	status   atomic.Value // stores ConnectionStatus
	failures atomic.Int32
	logger   *slog.Logger

	// NATS connection
	conn *nats.Conn
	js   jetstream.JetStream

	// Native-handle consumer claims reject duplicate fixed durable ownership
	// without retaining lifecycle handles or giving Client.Close child authority.
	internalClaimsMu sync.Mutex
	internalClaims   map[internalConsumerIdentity]*internalConsumerClaim

	// Circuit breaker
	lastFailure      atomic.Value // stores time.Time
	backoff          atomic.Value // stores time.Duration
	circuitFailures  atomic.Int32 // failures in current circuit round
	circuitThreshold int32        // failures before opening circuit
	maxBackoff       time.Duration

	// Connection options
	maxReconnects int
	reconnectWait time.Duration
	pingInterval  time.Duration
	timeout       time.Duration
	drainTimeout  time.Duration

	// requestHandlerTimeout bounds a single SubscribeForRequests handler
	// invocation. Defaults to DefaultRequestHandlerTimeout (30s); raised per
	// deployment via WithRequestHandlerTimeout or the
	// SEMSTREAMS_NATS_REQUEST_HANDLER_TIMEOUT env var for slow-by-design
	// handlers (e.g. LLM answer synthesis on the globalSearch path).
	requestHandlerTimeout time.Duration

	// Authentication - sensitive fields cleared on close
	username string
	password string // WARNING: Consider using JWT/NKey authentication instead

	// Client identification
	clientName string

	// Metrics
	jsMetrics       *jetstreamMetrics
	metricsCancel   context.CancelFunc
	metricsInterval time.Duration

	// Callbacks
	onConnectionLost func(error)

	// Connection-loss watchdog: when set, onConnectionLost fires once the
	// connection has been continuously down for at least connectionLossTimeout.
	// Reconnecting before the timeout cancels it. Useful for callers that
	// want to bound how long the process tolerates an absent broker (e.g.
	// trigger graceful shutdown so the supervisor can restart with a fresh
	// connection instead of hot-looping in degraded mode forever).
	connectionLossTimeout time.Duration
	lossTimer             *time.Timer

	// circuitTimer runs the circuit test once the breaker's backoff expires.
	circuitTimer *time.Timer

	// timersMu guards lossTimer and circuitTimer. Lock order: timersMu, then mu.
	timersMu sync.Mutex

	// Health monitoring
	healthTicker   *time.Ticker
	healthInterval time.Duration
	healthDone     chan struct{} // Signal to stop health monitoring goroutine

	// Synchronization
	mu      sync.RWMutex
	closeMu sync.Mutex // serializes Close's setup with Connect's admission

	// Lifecycle (design D3). closing is set once, by the first Close, under mu.
	// startBackground refuses work once it is set and otherwise adds to
	// background under the same lock, so no Add follows the Wait that the
	// first Close's joiner runs; joined is closed when that Wait returns.
	// running counts the admitted work per kind, for the lifecycle adapter.
	closing    bool
	background sync.WaitGroup
	running    map[string]int
	joined     chan struct{}

	// opHook, when set, is told of each external operation the client
	// performs ("dial", "drain"); the lifecycle adapter counts them.
	opHook func(op string)
}

// Kinds of background work, the six sites of design D3. Each starts through
// startBackground; the name is what its drop log and the lifecycle adapter report.
const (
	workHealthMonitor  = "health monitor"
	workMetricsPoller  = "metrics poller"
	workClaimRelease   = "claim release"
	workConnectionLost = "connection-loss timer"
	workCircuitTest    = "circuit-test timer"
)

// startBackground is the one way the client starts work that outlives the call
// that started it. Under mu it refuses once Close has begun, and otherwise adds
// to the background group before the goroutine starts. Refused work is dropped,
// never run inline: the drop is logged at debug level and false is returned.
func (m *Client) startBackground(kind string, fn func()) bool {
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		m.logDropped(kind)
		return false
	}
	m.background.Add(1)
	if m.running == nil {
		m.running = make(map[string]int)
	}
	m.running[kind]++
	m.mu.Unlock()
	go func() {
		defer func() {
			m.mu.Lock()
			if m.running[kind]--; m.running[kind] == 0 {
				delete(m.running, kind)
			}
			m.mu.Unlock()
			m.background.Done()
		}()
		fn()
	}()
	return true
}

// logDropped records work refused because Close has begun. Continuing is safe:
// no callback runs once Close has begun, and Close joins whatever was admitted.
func (m *Client) logDropped(kind string) {
	m.logger.Debug("NATS client closing; background work dropped", slog.String("work", kind))
}

// isClosing reports whether Close has begun.
func (m *Client) isClosing() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closing
}

func (m *Client) recordOp(op string) {
	if m.opHook != nil {
		m.opHook(op)
	}
}

// NewClient creates a new NATS client with optional configuration.
// The urls parameter accepts comma-separated NATS server URLs for clustering support
// (e.g., "nats://server1:4222,nats://server2:4222").
func NewClient(urls string, opts ...ClientOption) (*Client, error) {
	c := &Client{
		urls:   urls,
		logger: slog.Default(),
		// Sensible defaults
		maxReconnects:    -1, // infinite by default
		reconnectWait:    2 * time.Second,
		pingInterval:     30 * time.Second,
		healthInterval:   10 * time.Second,
		circuitThreshold: 15, // Increased from 5 for resilience to transient failures
		maxBackoff:       time.Minute,
		timeout:          5 * time.Second,
		drainTimeout:     30 * time.Second,
		metricsInterval:  30 * time.Second, // Poll JetStream stats every 30s
		// Env-resolved default (30s unless SEMSTREAMS_NATS_REQUEST_HANDLER_TIMEOUT
		// overrides). Applied here so an explicit WithRequestHandlerTimeout option
		// below still wins (option > env > framework default).
		requestHandlerTimeout: resolveRequestHandlerTimeoutFromEnv(),
	}

	// Apply options
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, errs.WrapInvalid(err, "Client", "NewClient", "apply option")
		}
	}

	c.status.Store(StatusDisconnected)
	c.backoff.Store(time.Second)
	c.lastFailure.Store(time.Time{})

	c.logger.Debug("Created NATS client", slog.String("urls", urls))

	return c, nil
}

// URLs returns the NATS server URLs (comma-separated for clustering)
func (m *Client) URLs() string {
	return m.urls
}

// Status returns the current connection status
func (m *Client) Status() ConnectionStatus {
	val := m.status.Load()
	if val == nil {
		return StatusDisconnected
	}
	return val.(ConnectionStatus)
}

// GetConnection returns the current NATS connection
func (m *Client) GetConnection() *nats.Conn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.conn
}

// MaxPayload observes the maximum payload reported by the active NATS
// connection. The value is diagnostic and may change after it is returned;
// callers must treat the result of an actual publish as authoritative.
func (m *Client) MaxPayload() (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.conn == nil || !m.conn.IsConnected() {
		return 0, ErrNotConnected
	}
	return m.conn.MaxPayload(), nil
}

// SetConnection sets the NATS connection (for testing)
func (m *Client) SetConnection(conn *nats.Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conn = conn
	if conn != nil && conn.IsConnected() {
		m.setStatus(StatusConnected)
	}
}

// setStatus updates the connection status
func (m *Client) setStatus(status ConnectionStatus) {
	m.status.Store(status)
}

// IsHealthy returns true if the connection is healthy
func (m *Client) IsHealthy() bool {
	return m.Status() == StatusConnected
}

// Failures returns the current failure count
func (m *Client) Failures() int32 {
	return m.failures.Load()
}

// Backoff returns the current backoff duration
func (m *Client) Backoff() time.Duration {
	return m.backoff.Load().(time.Duration)
}

// recordFailure records a connection failure and manages circuit breaker
func (m *Client) recordFailure() {
	// Track total failures for metrics
	totalFailures := m.failures.Add(1)
	m.lastFailure.Store(time.Now())

	// Track circuit breaker failures separately
	circuitFailures := m.circuitFailures.Add(1)

	m.logger.Debug("Recorded failure", slog.Int64("total_failures", int64(totalFailures)), slog.Int64("circuit_failures", int64(circuitFailures)))

	// Open circuit after threshold failures in this round
	if circuitFailures >= m.circuitThreshold {
		currentStatus := m.Status()

		// We need to open or update the circuit breaker
		if currentStatus != StatusCircuitOpen {
			// Try to transition to open state (only one goroutine will succeed)
			if m.status.CompareAndSwap(currentStatus, StatusCircuitOpen) {
				// We successfully opened the circuit
				currentBackoff := m.backoff.Load().(time.Duration)
				newBackoff := currentBackoff * 2
				if newBackoff > m.maxBackoff {
					newBackoff = m.maxBackoff
				}
				m.backoff.Store(newBackoff)

				m.logger.Info("Circuit breaker opened",
					slog.Int64("circuit_failures", int64(circuitFailures)),
					slog.Duration("backoff", currentBackoff),
				)

				// Reset circuit failures for next round
				m.circuitFailures.Store(0)

				// Schedule circuit test after backoff
				m.armCircuitTimer(currentBackoff)
			}
		} else {
			// Circuit already open - may need to increase backoff for consecutive failures
			// This handles the case where failures continue while circuit is open
			currentBackoff := m.backoff.Load().(time.Duration)
			newBackoff := currentBackoff * 2
			if newBackoff > m.maxBackoff {
				newBackoff = m.maxBackoff
			}
			m.backoff.Store(newBackoff)

			m.logger.Info("Circuit breaker still open, increased backoff", slog.Duration("backoff", newBackoff))

			// Reset circuit failures for next round
			m.circuitFailures.Store(0)
		}
	}
}

// recordStreamPublishFailure accounts a failed JetStream publish against the
// connection circuit unless the server's typed PubAck says only that the target
// stream reached one of its configured admission ceilings. Capacity refusal is
// a healthy connection reporting durable-resource state, not connection loss.
// The exact typed API error remains caller-visible at each publish seam.
func (m *Client) recordStreamPublishFailure(err error) {
	if isCircuitNeutralStreamCapacityError(err) {
		return
	}
	m.recordFailure()
}

func isCircuitNeutralStreamCapacityError(err error) bool {
	var apiErr *jetstream.APIError
	if !stderrors.As(err, &apiErr) || apiErr == nil || apiErr.ErrorCode != jetstream.ErrorCode(10077) {
		return false
	}
	switch apiErr.Description {
	case "maximum bytes exceeded",
		"maximum messages exceeded",
		"maximum messages per subject exceeded":
		return true
	default:
		return false
	}
}

// resetCircuit resets the circuit breaker state
func (m *Client) resetCircuit() {
	m.failures.Store(0)
	m.circuitFailures.Store(0)
	m.backoff.Store(time.Second)
	m.lastFailure.Store(time.Time{})

	// Don't change status if we're connected
	if m.Status() == StatusCircuitOpen {
		m.setStatus(StatusDisconnected)
	}
}

// armCircuitTimer schedules the circuit test after backoff. Once Close has
// begun it arms nothing, and logs the drop. A test from an earlier round that is
// still pending is stopped: this round's backoff supersedes it, and an untracked
// timer could outlive Close.
func (m *Client) armCircuitTimer(backoff time.Duration) {
	m.timersMu.Lock()
	defer m.timersMu.Unlock()
	if m.isClosing() {
		m.logDropped(workCircuitTest)
		return
	}
	if m.circuitTimer != nil {
		m.circuitTimer.Stop()
	}
	var timer *time.Timer
	// timer is read only under timersMu, where it was written.
	timer = time.AfterFunc(backoff, func() { m.circuitTimerFired(func() bool { return m.circuitTimer == timer }) })
	m.circuitTimer = timer
}

// circuitTimerFired is the circuit timer's body. It enters through
// startBackground before doing anything, so once Close has begun the test is
// dropped, not run. current, called under timersMu, reports whether the timer
// that fired is still the armed one.
func (m *Client) circuitTimerFired(current func() bool) {
	m.startBackground(workCircuitTest, func() {
		m.timersMu.Lock()
		if current() {
			m.circuitTimer = nil
		}
		m.timersMu.Unlock()
		m.testCircuit()
	})
}

// testCircuit runs when the circuit breaker's backoff expires. It only moves the
// status from open to disconnected; it does not reconnect. The status returns to
// connected through handleReconnect or the health monitor.
func (m *Client) testCircuit() {
	m.logger.Debug("Testing circuit breaker - attempting to close circuit")

	if m.Status() == StatusCircuitOpen {
		m.logger.Debug("Circuit breaker test: moving from open to disconnected")
		m.setStatus(StatusDisconnected)
	}
}

// WaitForConnection waits for the connection to be established
func (m *Client) WaitForConnection(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("connection timeout: %w", ctx.Err())
		case <-ticker.C:
			if m.IsHealthy() {
				return nil
			}
		}
	}
}

// buildConnectionOptions builds NATS connection options from client configuration
func (m *Client) buildConnectionOptions() []nats.Option {
	opts := []nats.Option{
		nats.MaxReconnects(m.maxReconnects),
		nats.ReconnectWait(m.reconnectWait),
		nats.PingInterval(m.pingInterval),
		nats.Timeout(m.timeout),
		nats.DrainTimeout(m.drainTimeout),
		nats.DisconnectErrHandler(m.handleDisconnect),
		nats.ReconnectHandler(m.handleReconnect),
		nats.ClosedHandler(m.handleClosed),
		nats.ErrorHandler(m.handleError),
	}

	// Add authentication if configured
	if m.username != "" && m.password != "" {
		opts = append(opts, nats.UserInfo(m.username, m.password))
	}

	// Add client name if configured
	if m.clientName != "" {
		opts = append(opts, nats.Name(m.clientName))
	}

	return opts
}

// GetStatus returns current status information
func (m *Client) GetStatus() *Status {
	lastFailure := m.lastFailure.Load().(time.Time)

	status := &Status{
		Status:          m.Status(),
		FailureCount:    m.failures.Load(),
		LastFailureTime: lastFailure,
	}

	// Add RTT if connected
	if m.conn != nil && m.conn.IsConnected() {
		if rtt, err := m.conn.RTT(); err == nil {
			status.RTT = rtt
		}
	}

	return status
}

// Connect establishes connection to NATS server. It refuses a nil context and a
// client that is already connected, with an error and before any dial. Connect
// returns an error when Close has begun before it finished; the connection it
// dialled is then closed and nothing it would have started runs.
func (m *Client) Connect(ctx context.Context) error {
	return m.connectWith(ctx, nats.Connect)
}

// connectWith keeps the native connection candidate private until Connect wins
// admission against terminal Close. dial is a test seam; production uses
// nats.Connect synchronously with the timeout in buildConnectionOptions.
func (m *Client) connectWith(
	ctx context.Context,
	dial func(string, ...nats.Option) (*nats.Conn, error),
) error {
	if ctx == nil {
		return errs.WrapInvalid(stderrors.New("nil context"), "Client", "Connect", "missing context")
	}
	m.mu.RLock()
	started := m.conn != nil
	m.mu.RUnlock()
	if started {
		return errs.WrapInvalid(errs.ErrAlreadyStarted, "Client", "Connect", "already connected")
	}

	// Check circuit breaker first
	if m.Status() == StatusCircuitOpen {
		m.logger.Debug("Circuit breaker is open, skipping connection attempt")
		return ErrCircuitOpen
	}

	m.setStatus(StatusConnecting)
	m.logger.Info("Connecting to NATS", slog.String("urls", m.urls))

	// Build connection options
	opts := m.buildConnectionOptions()

	m.recordOp("dial")
	conn, err := dial(m.urls, opts...)
	if ctxErr := ctx.Err(); ctxErr != nil {
		if conn != nil {
			conn.Close()
		}
		m.recordFailure()
		if m.Status() != StatusCircuitOpen {
			m.setStatus(StatusDisconnected)
		}
		return errs.WrapTransient(ctxErr, "Client", "Connect", "connection cancelled")
	}
	if err != nil {
		m.recordFailure()

		// Only set to disconnected if circuit didn't open
		if m.Status() != StatusCircuitOpen {
			m.setStatus(StatusDisconnected)
		}

		// Check if circuit opened after this failure
		if m.Status() == StatusCircuitOpen {
			return ErrCircuitOpen
		}

		return errs.WrapTransient(err, "Client", "Connect", "establish connection")
	}

	// Initialize JetStream with new API. The async publish error handler
	// bridges failed async acks into the circuit breaker so a broken ack
	// path opens the breaker exactly as a failed synchronous publish does
	// (see publishToStreamAsync). Keep both candidates local until admission.
	// nats.go v1.54.0 fails jetstream.New only when an option does, and this
	// option never does; the error is still returned, never dropped.
	js, err := jetstream.New(conn, jetstream.WithPublishAsyncErrHandler(m.asyncPublishErrHandler))
	if err != nil {
		conn.Close()
		m.setStatus(StatusDisconnected)
		return errs.Wrap(err, "Client", "Connect", "initialize JetStream")
	}

	// Close owns terminal admission. Once Close has begun, no native
	// connection produced by an in-flight dial may become Client state.
	m.closeMu.Lock()
	if ctxErr := ctx.Err(); ctxErr != nil {
		conn.Close()
		m.recordFailure()
		if m.Status() != StatusCircuitOpen {
			m.setStatus(StatusDisconnected)
		}
		m.closeMu.Unlock()
		return errs.WrapTransient(ctxErr, "Client", "Connect", "connection cancelled")
	}

	m.mu.Lock()
	switch {
	case m.closing:
		m.mu.Unlock()
		conn.Close()
		m.setStatus(StatusDisconnected)
		m.closeMu.Unlock()
		return errs.Wrap(nats.ErrConnectionClosed, "Client", "Connect", "admit connection")
	case m.conn != nil:
		// A concurrent Connect won admission; this one leaves its connection in place.
		m.mu.Unlock()
		conn.Close()
		m.closeMu.Unlock()
		return errs.WrapInvalid(errs.ErrAlreadyStarted, "Client", "Connect", "already connected")
	}
	m.conn = conn
	m.js = js
	m.mu.Unlock()

	m.setStatus(StatusConnected)
	m.resetCircuit()
	m.closeMu.Unlock()

	m.logger.Info("Successfully connected to NATS", slog.String("urls", m.urls))

	// The monitor and the poller start through startBackground, so a Close that
	// has begun since admission refuses them.
	refused := false
	if m.healthInterval > 0 {
		m.logger.Debug("Starting health monitoring", slog.Duration("interval", m.healthInterval))
		refused = !m.startHealthMonitoring()
	}

	if m.jsMetrics != nil && m.metricsInterval > 0 {
		m.logger.Debug("Starting JetStream metrics polling", slog.Duration("interval", m.metricsInterval))
		refused = !m.startMetricsPoller() || refused
	}

	if refused {
		return errs.Wrap(nats.ErrConnectionClosed, "Client", "Connect", "closed while starting")
	}
	return nil
}

// startMetricsPoller starts the JetStream metrics poller. It reports false when
// Close has begun, in which case nothing is left running.
//
// The poller's context is a root this client owns (task 3.7c triage of the
// pin's client.go:566): it outlives Connect's dial-scoped context on purpose,
// and Close cancels the poller's in-flight work and joins it. The cancel is
// published before the goroutine is admitted, so a Close that begins after
// admission finds it.
func (m *Client) startMetricsPoller() bool {
	pollCtx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.metricsCancel = cancel
	m.mu.Unlock()
	interval := m.metricsInterval
	if m.startBackground(workMetricsPoller, func() { m.jsMetrics.runPoller(pollCtx, interval) }) {
		return true
	}
	cancel()
	m.mu.Lock()
	m.metricsCancel = nil
	m.mu.Unlock()
	return false
}

// Close drains and closes the NATS connection, stops the timers, and waits for
// every goroutine the client started: the health monitor, the metrics poller,
// the claim releases and the timer callbacks. It returns nil only once all of
// them have returned. When ctx ends first, or has already ended, Close returns
// ctx.Err() and the join stays pending; a later Close waits on the same join.
// No callback runs once Close has begun: work offered after that is dropped and
// logged at debug level. A Close called from inside one of the client's
// callbacks waits on a join that includes its own goroutine, so it returns
// ctx.Err() when its context ends and never returns nil. Close refuses a nil
// context with an error and touches nothing.
func (m *Client) Close(ctx context.Context) error {
	if ctx == nil {
		return errs.WrapInvalid(stderrors.New("nil context"), "Client", "Close", "missing context")
	}

	// Only the first Close runs the setup, under closeMu; every Close then
	// waits on the same join, outside any lock.
	m.closeMu.Lock()
	m.mu.Lock()
	first := !m.closing
	if first {
		m.closing = true
		m.joined = make(chan struct{})
	}
	joined := m.joined
	conn := m.conn
	drainTimeout := m.drainTimeout
	cancelPoller := m.metricsCancel
	m.metricsCancel = nil
	m.mu.Unlock()

	var closeErr error
	if first {
		m.stopHealthMonitoring()
		m.stopTimers()
		if cancelPoller != nil {
			cancelPoller()
		}

		closeErr = m.drainAndCloseConnection(ctx, conn, drainTimeout)

		m.mu.Lock()
		if m.conn == conn {
			m.conn = nil
		}
		m.js = nil

		// Clear sensitive credentials from memory
		m.username = ""
		m.password = ""
		m.mu.Unlock()

		m.setStatus(StatusDisconnected)

		// closing is set, so startBackground adds nothing from here on.
		go func() {
			m.background.Wait()
			close(joined)
		}()
	}
	m.closeMu.Unlock()

	select {
	case <-joined:
		// An ended context is reported even when the join has also finished,
		// so a nil return always means the caller's context was live.
		if err := ctx.Err(); err != nil {
			return err
		}
		return closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

// guardedConsumer serializes Info() on one consumer handle.
//
// jetstream.Consumer.Info() is NOT safe for concurrent use on the same handle: it
// assigns the fetched ConsumerInfo to the consumer's own cache field with no
// synchronization (nats.go jetstream/consumer.go — `p.info = resp.ConsumerInfo`). Two
// callers reading the same bound consumer at once race inside the library, and the
// detector reports it against OUR call site. Confirmed by the graph-ingest lifecycle
// stress test the moment a second reader existed.
//
// THE GUARD IS ON THE HANDLE, NOT ON A CALL SITE, and that is the point. The same
// *jetstream.Consumer is read by the acquisition path and the metrics observer, so a
// lock around only one reader leaves the other racing the identical object. Wrapping
// once at creation means every present and future caller in this package is covered by
// construction.
//
// Embedding keeps it a jetstream.Consumer; only Info is overridden, and its signature
// must stay EXACTLY `Info(context.Context) (*jetstream.ConsumerInfo, error)` — a
// divergent signature would silently stop overriding and reopen the race.
//
// STILL PRESENT UPSTREAM as of nats.go v1.52.0 (checked 2026-07-30; we pin v1.48.0).
// The assignment is byte-identical across both, so a dependency bump does NOT retire
// this guard — do not delete it on upgrade. Filing it upstream is tracked in the
// change's follow-ups.
type guardedConsumer struct {
	jetstream.Consumer
	infoMu sync.Mutex
}

// Info serializes the underlying call. The lock is held across the network round trip
// because the unsynchronized write happens at the END of the library's Info().
func (g *guardedConsumer) Info(ctx context.Context) (*jetstream.ConsumerInfo, error) {
	g.infoMu.Lock()
	defer g.infoMu.Unlock()
	return g.Consumer.Info(ctx)
}

// CachedInfo serializes the READ of the same field Info writes.
//
// Overriding it is not optional. jetstream.Consumer includes CachedInfo, implemented as
// a bare `return p.info` — the exact field Info assigns. Guarding only Info would leave
// the promoted CachedInfo racing the guarded writer, so the "every caller is covered"
// claim above would be false for it. No caller in this repo uses Consumer.CachedInfo
// today; this closes the hole rather than waiting for one to open it.
func (g *guardedConsumer) CachedInfo() *jetstream.ConsumerInfo {
	g.infoMu.Lock()
	defer g.infoMu.Unlock()
	return g.Consumer.CachedInfo()
}

// isBenignDrainError reports whether Drain reached the desired terminal state
// before this client asked it to. Other failures remain observable so cleanup
// cannot silently hide deadline, cancellation, or transport errors.
func isBenignDrainError(err error) bool {
	return stderrors.Is(err, nats.ErrConnectionClosed)
}

// drainAndCloseConnection drains and closes the NATS connection.
func (m *Client) drainAndCloseConnection(ctx context.Context, conn *nats.Conn, drainTimeout time.Duration) error {
	if conn == nil {
		return nil
	}

	// Use context deadline for drain timeout if available
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < drainTimeout {
			drainTimeout = remaining
		}
	}

	closed := conn.StatusChanged(nats.CLOSED)
	defer conn.RemoveStatusListener(closed)

	m.recordOp("drain")
	if err := conn.Drain(); err != nil {
		if isBenignDrainError(err) {
			return nil
		}
		drainErr := errs.Wrap(err, "Client", "Close", "drain connection")
		m.logger.Error("Drain error", slog.Any("error", err))
		if !conn.IsClosed() {
			conn.Close()
		}
		return drainErr
	}

	drainTimer := time.NewTimer(drainTimeout)
	defer drainTimer.Stop()

	select {
	case <-closed:
		return nil
	case <-drainTimer.C:
		drainErr := errs.WrapTransient(
			fmt.Errorf("drain timeout after %v", drainTimeout),
			"Client", "Close", "drain timeout",
		)
		m.logger.Error("Drain timeout, force closing", slog.Duration("drain_timeout", drainTimeout))
		if !conn.IsClosed() {
			conn.Close()
		}
		return drainErr
	case <-ctx.Done():
		drainErr := errs.Wrap(ctx.Err(), "Client", "Close", "context cancelled during drain")
		m.logger.Error("Context cancelled during drain, force closing")
		if !conn.IsClosed() {
			conn.Close()
		}
		return drainErr
	}
}

// RTT returns the round-trip time to the NATS server
func (m *Client) RTT() (time.Duration, error) {
	m.mu.RLock()
	conn := m.conn
	m.mu.RUnlock()

	if conn == nil || !conn.IsConnected() {
		return 0, ErrNotConnected
	}

	return conn.RTT()
}

type nativeSubscription interface {
	Drain() error
	IsValid() bool
	SetClosedHandler(func(subject string))
	Unsubscribe() error
}

// Subscription wraps a NATS subscription for lifecycle management.
type Subscription struct {
	sub nativeSubscription

	drainOnce sync.Once
	drainErr  error
	done      chan struct{}
	doneOnce  sync.Once
}

func newSubscription(sub nativeSubscription) *Subscription {
	s := &Subscription{sub: sub, done: make(chan struct{})}
	// nats.go calls the closed handler once, after the delivery goroutine
	// exits, on drain, unsubscribe, and connection close alike.
	sub.SetClosedHandler(func(string) { s.closeDone() })
	if !sub.IsValid() {
		// Closed before the handler was set, so it may never fire; doneOnce
		// makes a late fire harmless.
		s.closeDone()
	}
	return s
}

// closeDone runs on the nats.go delivery goroutine, so it must not block.
func (s *Subscription) closeDone() {
	s.doneOnce.Do(func() { close(s.done) })
}

// Unsubscribe unsubscribes from the subject
func (s *Subscription) Unsubscribe() error {
	if s == nil || s.sub == nil {
		return nil
	}
	return s.sub.Unsubscribe()
}

// Drain stops new deliveries and returns once the subscription's delivery
// goroutine has exited, so every callback has returned. If the connection
// closes before or during the drain, Drain still returns nil after the
// in-flight callback returns; messages queued but not yet delivered are
// discarded, because core NATS is at-most-once. If ctx ends first, Drain
// returns ctx's error and a later call rejoins the same native drain; it never
// starts a second one.
func (s *Subscription) Drain(ctx context.Context) error {
	if ctx == nil {
		return stderrors.New("natsclient: nil Subscription.Drain context")
	}
	if s == nil || s.sub == nil {
		return nil
	}
	s.drainOnce.Do(func() {
		// A closed connection ends the subscription; done still joins the
		// in-flight callback.
		if err := s.sub.Drain(); !stderrors.Is(err, nats.ErrConnectionClosed) {
			s.drainErr = err
		}
	})
	if s.drainErr != nil {
		return s.drainErr
	}
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Subscribe subscribes to a NATS subject with context propagation.
// Each message handler receives the full *nats.Msg to access Subject, Data, Headers, etc.
// This is essential for wildcard subscriptions where the actual subject differs from the pattern.
// The context is derived from the parent context with a 30-second timeout for message processing.
// Returns a Subscription handle that can be used to unsubscribe.
func (m *Client) Subscribe(ctx context.Context, subject string, handler func(context.Context, *nats.Msg)) (*Subscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil || !m.conn.IsConnected() {
		return nil, ErrNotConnected
	}

	sub, err := m.conn.Subscribe(subject, func(msg *nats.Msg) {
		// Create per-message context with timeout
		msgCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		// Extract trace context from message headers
		if tc := ExtractTrace(msg); tc != nil {
			msgCtx = ContextWithTrace(msgCtx, tc)
		}

		handler(msgCtx, msg)
	})
	if err != nil {
		return nil, err
	}

	return newSubscription(sub), nil
}

// Publish publishes a message to a NATS subject
func (m *Client) Publish(ctx context.Context, subject string, data []byte) error {
	m.mu.RLock()
	conn := m.conn
	m.mu.RUnlock()

	if conn == nil || !conn.IsConnected() {
		return ErrNotConnected
	}

	// Auto-generate trace if none exists
	if _, ok := TraceContextFromContext(ctx); !ok {
		ctx = ContextWithTrace(ctx, NewTraceContext())
	}

	msg := &nats.Msg{Subject: subject, Data: data}
	InjectTrace(ctx, msg)

	return conn.PublishMsg(msg)
}

// JetStream returns the JetStream context
func (m *Client) JetStream() (jetstream.JetStream, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.js == nil {
		return nil, errs.WrapTransient(
			fmt.Errorf("JetStream not initialized"),
			"Client", "JetStream", "get JetStream context")
	}

	return m.js, nil
}

// CreateStream creates a JetStream stream
func (m *Client) CreateStream(ctx context.Context, cfg jetstream.StreamConfig) (jetstream.Stream, error) {
	// Fail closed on a KV/ObjectStore backing-stream name before anything else,
	// so the refusal does not depend on connection or circuit state.
	if err := CheckOrdinaryStreamName(cfg.Name, "natsclient.Client.CreateStream"); err != nil {
		return nil, errs.WrapFatal(err, "Client", "CreateStream",
			"validate stream name "+cfg.Name)
	}

	// Bounds are checked HERE, unconditionally, unlike on EnsureStream where the
	// same check sits inside the not-found branch. This seam only ever CREATES, so
	// there is no bind path to protect and no reason to wait for the server to
	// tell us which act this is.
	if err := CheckStreamBounds(cfg, "natsclient.Client.CreateStream"); err != nil {
		return nil, errs.WrapFatal(err, "Client", "CreateStream",
			"validate stream bounds for "+cfg.Name)
	}

	// Check circuit breaker first
	if m.Status() == StatusCircuitOpen {
		return nil, ErrCircuitOpen
	}

	if m.Status() != StatusConnected {
		return nil, ErrNotConnected
	}

	js, err := m.JetStream()
	if err != nil {
		m.recordFailure()
		return nil, err
	}

	stream, err := js.CreateStream(ctx, cfg)
	if err != nil {
		m.recordFailure()
		m.jsMetrics.recordError("create_stream")
		return nil, err
	}

	m.resetCircuit()

	// Track stream for metrics collection
	m.jsMetrics.trackStream(cfg.Name, stream)

	return stream, nil
}

// PublishToStream publishes to a JetStream stream with automatic trace context propagation.
// If no trace context exists in ctx, one is auto-generated for distributed tracing.
func (m *Client) PublishToStream(ctx context.Context, subject string, data []byte) error {
	return m.publishToStream(ctx, subject, data, "")
}

// PublishToStreamWithMsgID publishes to a JetStream stream stamping the
// Nats-Msg-Id header so the server's duplicate-detection window collapses
// re-publishes/redeliveries of the same logical event to a single store.
//
// This is the producer half of the at-least-once idempotency contract
// (ADR-055 §5, "T1"): graph-ingest's stream consumer is at-least-once and
// MergeEntity APPENDS triples on merge, so a redelivered born-once entity
// payload would double-apply its triples without dedup. Callers pass a
// DETERMINISTIC msgID for the logical event (e.g. "<loopID>:spawn",
// "<entityID>:v<n>") so a retry/redelivery carries the same ID.
//
// Scope of the guarantee: dedup only holds WITHIN the stream's configured
// duplicate window (config.StreamConfig.Duplicates; the NATS server default
// is 2m when unset). Redelivery outside that window — e.g. DeliverPolicy:all
// replay on consumer recreation — can still re-append; see ADR-055 Open
// Question #1. An empty msgID is equivalent to PublishToStream (no dedup),
// so this is a safe drop-in.
func (m *Client) PublishToStreamWithMsgID(ctx context.Context, subject string, data []byte, msgID string) error {
	return m.publishToStream(ctx, subject, data, msgID)
}

// publishToStream is the shared publish path for PublishToStream and
// PublishToStreamWithMsgID. A non-empty msgID is stamped as the Nats-Msg-Id
// header for server-side duplicate detection.
func (m *Client) publishToStream(ctx context.Context, subject string, data []byte, msgID string) error {
	// Check circuit breaker first
	if m.Status() == StatusCircuitOpen {
		return ErrCircuitOpen
	}

	if m.Status() != StatusConnected {
		return ErrNotConnected
	}

	js, err := m.JetStream()
	if err != nil {
		m.recordFailure()
		return err
	}

	// Auto-generate trace context if none exists
	if _, ok := TraceContextFromContext(ctx); !ok {
		ctx = ContextWithTrace(ctx, NewTraceContext())
	}

	// Build message with headers for trace propagation
	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
	}
	if msgID != "" {
		// Initialize the header here (rather than relying on InjectTrace,
		// which early-returns when no trace context is present) so the
		// dedup ID is always carried.
		msg.Header = make(nats.Header)
		msg.Header.Set(nats.MsgIdHdr, msgID)
	}
	InjectTrace(ctx, msg)

	_, err = js.PublishMsg(ctx, msg)
	if err != nil {
		m.recordStreamPublishFailure(err)
		return err
	}

	m.resetCircuit()
	return nil
}

// asyncPublishErrHandler is the connection-level handler jetstream-go invokes for
// every failed async publish ack. It applies the same failure accounting as a
// failed synchronous publish; a typed target-stream capacity refusal remains
// circuit-neutral. The reset side lives on the enqueue path (successful enqueue
// = connection healthy); this handler never resets the circuit.
func (m *Client) asyncPublishErrHandler(_ jetstream.JetStream, msg *nats.Msg, err error) {
	m.recordStreamPublishFailure(err)
	if m.jsMetrics != nil {
		m.jsMetrics.recordError("publish_async")
	}
	subject := ""
	if msg != nil {
		subject = msg.Subject
	}
	m.logger.Debug("Async publish ack failed",
		slog.String("subject", subject),
		slog.Any("error", err),
	)
}

// publishToStreamAsync is the async publish path behind PublishBatchToStream.
// It publishes to a JetStream stream WITHOUT blocking on the PubAck, returning a
// jetstream.PubAckFuture whose Ok()/Err() carry the eventual server
// acknowledgement. It mirrors publishToStream's pre-checks and trace stamping,
// then enqueues via PublishMsgAsync.
//
// The enqueue itself is synchronous: an open circuit returns ErrCircuitOpen, a
// disconnected client returns ErrNotConnected, a cancelled context returns its
// error, and a full in-flight window past the stall wait returns jetstream's
// ErrTooManyStalledMsgs — in all of which the returned future is nil. A
// successful enqueue resets the circuit breaker (the connection-health signal);
// an enqueue error records a failure. Failed acks are delivered on the future's
// Err() channel and accounted by the connection-level async error handler; only
// the exact classified capacity set is circuit-neutral.
func (m *Client) publishToStreamAsync(ctx context.Context, subject string, data []byte) (jetstream.PubAckFuture, error) {
	// Check circuit breaker first (the breaker is the outermost gate, as on the
	// sync path).
	if m.Status() == StatusCircuitOpen {
		return nil, ErrCircuitOpen
	}

	if m.Status() != StatusConnected {
		return nil, ErrNotConnected
	}

	// Honor a cancelled context before enqueuing (PublishMsgAsync takes no ctx,
	// so this is the only cancellation point on the async path). A cancelled ctx
	// is caller intent, not a connection fault, so it does NOT record a failure.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	js, err := m.JetStream()
	if err != nil {
		m.recordFailure()
		return nil, err
	}

	// Auto-generate trace context if none exists
	if _, ok := TraceContextFromContext(ctx); !ok {
		ctx = ContextWithTrace(ctx, NewTraceContext())
	}

	// Build message with headers for trace propagation
	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
	}
	InjectTrace(ctx, msg)

	future, err := js.PublishMsgAsync(msg)
	if err != nil {
		m.recordFailure()
		return nil, err
	}

	// Enqueue succeeded: the connection is up and JetStream accepted the message
	// onto the wire. On the async path the breaker is a CONNECTION-LIVENESS gate:
	// a successful enqueue proves the connection is healthy, so we reset here
	// rather than at ack time. The exact classified target-stream capacity set is
	// surfaced through the future/batch and remains circuit-neutral. Every other
	// ack failure still contributes through asyncPublishErrHandler; connection
	// outage also makes subsequent enqueues fail, which trips the breaker. See
	// the nats-streaming capability contract.
	m.resetCircuit()
	return future, nil
}

// PublishBatchToStream publishes every message in msgs to one subject via the
// async path, waits for all acks (bounded by ctx), and returns a single aggregate
// error. Per-subject ordering from this single calling goroutine is preserved
// (jetstream-go async is in-order per connection, absent a NoResponders retry —
// see design.md §1). It is the convenience path for bursty producers that do not
// need per-message futures (gh#470).
//
// The drain waits on THIS batch's own futures, not the JetStream context's
// connection-global PublishAsyncComplete, so a concurrent async producer on the same Client cannot
// make this batch over-wait. If ctx is cancelled before all acks arrive, it
// returns the context error rather than hanging; the already-enqueued publishes
// still resolve in the background (and feed the circuit breaker via the async
// error handler on a connection fault). An enqueue error stops further enqueuing
// but already-enqueued messages are still drained. Ack failures are accounted
// once by asyncPublishErrHandler — this loop only collects them for the returned
// error (accounting here too would double-count).
func (m *Client) PublishBatchToStream(ctx context.Context, subject string, msgs [][]byte) error {
	if len(msgs) == 0 {
		return nil
	}
	// Fail fast on an already-cancelled context so nothing is enqueued.
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("PublishBatchToStream: context already cancelled: %w", err)
	}

	futures := make([]jetstream.PubAckFuture, 0, len(msgs))
	var enqueueErr error
	for _, data := range msgs {
		future, err := m.publishToStreamAsync(ctx, subject, data)
		if err != nil {
			enqueueErr = err
			break
		}
		futures = append(futures, future)
	}

	// Drain this batch's own futures, honoring ctx. Each future is already
	// in-flight, so waiting in order costs only the slowest ack, not the sum.
	errsList := make([]error, 0)
	if enqueueErr != nil {
		errsList = append(errsList, fmt.Errorf("enqueue stopped after %d of %d messages: %w",
			len(futures), len(msgs), enqueueErr))
	}
	ackFailures := 0
	for i, future := range futures {
		select {
		case <-future.Ok():
		case ackErr := <-future.Err():
			ackFailures++
			errsList = append(errsList, ackErr)
		case <-ctx.Done():
			// ctx.Done() and this future's completion can be ready in the same
			// instant; select then picks at random. Re-check the future
			// non-blocking so a publish that actually resolved is counted, not
			// spuriously reported cancelled — this preserves the "a batch that
			// finished draining before the cancel returns success" guarantee
			// (feedback_select_race_on_pre_cancelled_ctx).
			select {
			case <-future.Ok():
			case ackErr := <-future.Err():
				ackFailures++
				errsList = append(errsList, ackErr)
			default:
				return fmt.Errorf("PublishBatchToStream: context cancelled while draining "+
					"(%d of %d resolved, %d still pending): %w",
					i, len(futures), len(futures)-i, ctx.Err())
			}
		}
	}
	if len(errsList) == 0 {
		return nil
	}
	// Failed publishes = messages that never enqueued + messages whose ack failed.
	failed := (len(msgs) - len(futures)) + ackFailures
	return fmt.Errorf("PublishBatchToStream: %d of %d publishes failed (%d ack failures): %w",
		failed, len(msgs), ackFailures, stderrors.Join(errsList...))
}

// GetStream gets an existing JetStream stream.
//
// Its jetstream.ErrStreamNotFound means "not on this connection, now" — it is a
// point-in-time probe, not durable evidence of absence, because a clustered node
// that has not applied the meta assignment answers it for a stream that exists.
// A caller deciding something for the process lifetime wants ErrStreamNotVisible
// out of the guarded consumer setup instead; see that sentinel and the package doc.
func (m *Client) GetStream(ctx context.Context, name string) (jetstream.Stream, error) {
	// Check circuit breaker first
	if m.Status() == StatusCircuitOpen {
		return nil, ErrCircuitOpen
	}

	if m.Status() != StatusConnected {
		return nil, ErrNotConnected
	}

	js, err := m.JetStream()
	if err != nil {
		m.recordFailure()
		return nil, err
	}

	stream, err := js.Stream(ctx, name)
	if err != nil {
		// ErrStreamNotFound is a successful probe result (the stream is absent on
		// this connection right now), not a transport or availability failure.
		// Counting it as a failure would trip the circuit breaker on legitimate
		// existence probes — a component checking whether an optional stream is
		// present, a diagnostic sweep, a caller deciding whether to provision.
		// Only genuine failures (timeout, no-responders, etc.) should move the
		// breaker.
		//
		// The exemption is why this seam is deliberately NOT wired to the
		// consumer setup path's stream-visibility wait: a cheap probe stays
		// cheap. It is also why its answer is not evidence of absence — see the
		// package doc, and ErrStreamNotVisible for the answer that is.
		if !stderrors.Is(err, jetstream.ErrStreamNotFound) {
			m.recordFailure()
			m.jsMetrics.recordError("get_stream")
		}
		return nil, err
	}

	m.resetCircuit()

	// Track stream for metrics collection
	m.jsMetrics.trackStream(name, stream)

	return stream, nil
}

// CreateKeyValueBucket creates or gets a KV bucket with configuration
func (m *Client) CreateKeyValueBucket(ctx context.Context, cfg jetstream.KeyValueConfig) (jetstream.KeyValue, error) {
	// Check circuit breaker first
	if m.Status() == StatusCircuitOpen {
		return nil, ErrCircuitOpen
	}

	if m.Status() != StatusConnected {
		return nil, ErrNotConnected
	}

	js, err := m.JetStream()
	if err != nil {
		m.recordFailure()
		return nil, err
	}

	// Try to get existing bucket first
	bucket, err := js.KeyValue(ctx, cfg.Bucket)
	if err == nil {
		// Bucket already exists, use it
		m.logger.Info("Using existing KV bucket", slog.String("bucket", cfg.Bucket))
		m.resetCircuit()
		return bucket, nil
	}

	// Bucket doesn't exist, try to create it
	bucket, err = js.CreateKeyValue(ctx, cfg)
	if err != nil {
		// Check if error is "already exists" (race condition)
		if isAlreadyExistsError(err) {
			m.logger.Info("KV bucket already exists (race condition), attempting to get existing bucket",
				slog.String("bucket", cfg.Bucket),
			)
			// Try to get the existing bucket
			bucket, err = js.KeyValue(ctx, cfg.Bucket)
			if err != nil {
				m.recordFailure()
				return nil, errs.Wrap(err, "Client", "CreateKeyValueBucket",
					fmt.Sprintf("access existing bucket %s", cfg.Bucket))
			}
			m.logger.Info("Successfully accessed existing KV bucket", slog.String("bucket", cfg.Bucket))
			m.resetCircuit()
			return bucket, nil
		}
		// Real error, record failure
		m.recordFailure()
		return nil, err
	}

	// Successfully created new bucket
	m.logger.Info("Created new KV bucket", slog.String("bucket", cfg.Bucket))
	m.resetCircuit()
	return bucket, nil
}

// GetKeyValueBucket gets an existing KV bucket
func (m *Client) GetKeyValueBucket(ctx context.Context, name string) (jetstream.KeyValue, error) {
	// Check circuit breaker first
	if m.Status() == StatusCircuitOpen {
		return nil, ErrCircuitOpen
	}

	if m.Status() != StatusConnected {
		return nil, ErrNotConnected
	}

	js, err := m.JetStream()
	if err != nil {
		m.recordFailure()
		return nil, err
	}

	bucket, err := js.KeyValue(ctx, name)
	if err != nil {
		// ErrBucketNotFound is a successful probe result (the bucket is absent),
		// not a transport or availability failure — the same reasoning as
		// GetStream above, which has exempted ErrStreamNotFound since gh#248.
		// Counting it would trip the shared circuit breaker on legitimate
		// existence probes, and several callers poll for a legitimately absent
		// bucket: graph-query's resource.Watcher on COMMUNITY_INDEX rechecks
		// every 60s on any deployment without community detection, the readiness
		// watcher rebinds on every retry, and WaitForBucket polls at 500ms by
		// design. At a threshold of 15 those reach it on their own.
		if !stderrors.Is(err, jetstream.ErrBucketNotFound) {
			m.recordFailure()
		}
		return nil, err
	}

	m.resetCircuit()
	return bucket, nil
}

// WaitForBucket waits for a KV bucket to become available, retrying until
// the timeout expires or the context is cancelled. Use this when a component
// depends on a bucket created by another component with unpredictable startup timing.
//
// For more advanced patterns (background recovery, loss detection), use
// pkg/resource.Watcher directly.
func (m *Client) WaitForBucket(ctx context.Context, name string, timeout time.Duration) (jetstream.KeyValue, error) {
	// Try immediately first
	if bucket, err := m.GetKeyValueBucket(ctx, name); err == nil {
		return bucket, nil
	}

	// Calculate retry attempts from timeout
	interval := 500 * time.Millisecond
	attempts := int(timeout / interval)
	if attempts < 1 {
		attempts = 1
	}

	// Use resource.Watcher for structured retry with logging
	var result jetstream.KeyValue
	watcher := resource.NewWatcher(name, func(checkCtx context.Context) error {
		bucket, err := m.GetKeyValueBucket(checkCtx, name)
		if err != nil {
			return err
		}
		result = bucket
		return nil
	}, resource.Config{
		StartupAttempts: attempts,
		StartupInterval: interval,
		Logger:          m.logger,
	})

	if !watcher.WaitForStartup(ctx) {
		return nil, fmt.Errorf("bucket %q not available after %s", name, timeout)
	}

	return result, nil
}

// DeleteKeyValueBucket deletes a KV bucket
func (m *Client) DeleteKeyValueBucket(ctx context.Context, name string) error {
	// Check circuit breaker first
	if m.Status() == StatusCircuitOpen {
		return ErrCircuitOpen
	}

	if m.Status() != StatusConnected {
		return ErrNotConnected
	}

	js, err := m.JetStream()
	if err != nil {
		m.recordFailure()
		return err
	}

	err = js.DeleteKeyValue(ctx, name)
	if err != nil {
		m.recordFailure()
		return err
	}

	m.resetCircuit()
	return nil
}

// ListKeyValueBuckets lists all KV buckets
func (m *Client) ListKeyValueBuckets(ctx context.Context) ([]string, error) {
	// Check circuit breaker first
	if m.Status() == StatusCircuitOpen {
		return nil, ErrCircuitOpen
	}

	if m.Status() != StatusConnected {
		return nil, ErrNotConnected
	}

	js, err := m.JetStream()
	if err != nil {
		m.recordFailure()
		return nil, err
	}

	// KeyValue stores are implemented as JetStream streams with "KV_" prefix
	names := []string{}
	streamsCh := js.ListStreams(ctx)

	// StreamInfoLister is actually a channel of *StreamInfo
	for stream := range streamsCh.Info() {
		if stream != nil {
			// KV buckets are streams with "KV_" prefix
			if len(stream.Config.Name) > 3 && stream.Config.Name[:3] == "KV_" {
				bucketName := stream.Config.Name[3:] // Remove "KV_" prefix
				names = append(names, bucketName)
			}
		}
	}

	if err := streamsCh.Err(); err != nil {
		m.recordFailure()
		return nil, err
	}

	m.resetCircuit()
	return names, nil
}

// Event handlers for NATS connection
func (m *Client) handleDisconnect(_ *nats.Conn, err error) {
	m.setStatus(StatusReconnecting)

	m.armConnectionLossTimer(err)
}

func (m *Client) handleReconnect(_ *nats.Conn) {
	m.setStatus(StatusConnected)
	m.resetCircuit()
	m.cancelConnectionLossTimer()
}

// armConnectionLossTimer starts the connection-loss watchdog if it is
// configured and not already armed. Idempotent across repeated disconnects:
// a second disconnect without an intervening reconnect reuses the original
// timer so the elapsed grace measures from the *first* loss of contact.
func (m *Client) armConnectionLossTimer(disconnectErr error) {
	if m.connectionLossTimeout <= 0 {
		return
	}

	m.mu.RLock()
	cb := m.onConnectionLost
	m.mu.RUnlock()
	if cb == nil {
		return
	}

	m.timersMu.Lock()
	defer m.timersMu.Unlock()
	if m.isClosing() {
		// No callback runs once Close has begun, so there is nothing to arm.
		m.logDropped(workConnectionLost)
		return
	}
	if m.lossTimer != nil {
		return
	}
	var timer *time.Timer
	// timer is read only under timersMu, where it was written.
	timer = time.AfterFunc(m.connectionLossTimeout, func() {
		m.connectionLossFired(func() bool { return m.lossTimer == timer }, disconnectErr)
	})
	m.lossTimer = timer
}

// connectionLossFired is the connection-loss timer's body. It enters through
// startBackground before doing anything, so once Close has begun the callback
// is dropped, not run, and Close joins a callback that was admitted. current,
// called under timersMu, reports whether the timer that fired is still the
// armed one.
func (m *Client) connectionLossFired(current func() bool, disconnectErr error) {
	m.startBackground(workConnectionLost, func() {
		m.timersMu.Lock()
		if current() {
			m.lossTimer = nil
		}
		m.timersMu.Unlock()

		// Re-read the callback under the main lock so a concurrent option
		// change doesn't race us into firing on a stale handle.
		m.mu.RLock()
		fire := m.onConnectionLost
		m.mu.RUnlock()
		if fire != nil {
			fire(disconnectErr)
		}
	})
}

// cancelConnectionLossTimer stops the watchdog if armed. Safe to call when
// no timer is pending.
func (m *Client) cancelConnectionLossTimer() {
	m.timersMu.Lock()
	defer m.timersMu.Unlock()
	if m.lossTimer != nil {
		m.lossTimer.Stop()
		m.lossTimer = nil
	}
}

// stopTimers stops both timers. A timer that has already fired enters
// startBackground, which Close has closed by then.
func (m *Client) stopTimers() {
	m.timersMu.Lock()
	defer m.timersMu.Unlock()
	if m.lossTimer != nil {
		m.lossTimer.Stop()
		m.lossTimer = nil
	}
	if m.circuitTimer != nil {
		m.circuitTimer.Stop()
		m.circuitTimer = nil
	}
}

func (m *Client) handleClosed(_ *nats.Conn) {
	m.setStatus(StatusDisconnected)
}

func (m *Client) handleError(_ *nats.Conn, sub *nats.Subscription, err error) {
	attrs := []any{slog.Any("error", err)}
	if sub != nil {
		attrs = append(attrs, slog.String("subject", sub.Subject))
		if sub.Queue != "" {
			attrs = append(attrs, slog.String("queue", sub.Queue))
		}
		if stderrors.Is(err, nats.ErrSlowConsumer) {
			if dropped, droppedErr := sub.Dropped(); droppedErr == nil {
				attrs = append(attrs, slog.Int("dropped", dropped))
			} else {
				attrs = append(attrs, slog.Bool("dropped_available", false))
			}
		}
	}

	// Log error for debugging
	m.logger.Error("NATS error", attrs...)
	// Don't record failure here as it may be called for non-connection errors
}

// startHealthMonitoring starts periodic health checks. It reports false when
// Close has begun, in which case nothing is left running.
func (m *Client) startHealthMonitoring() bool {
	// Stop any existing health monitoring
	m.stopHealthMonitoring()

	// The ticker and done channel are published before the goroutine is
	// admitted, so a Close that begins after admission finds done to close.
	m.mu.Lock()
	m.healthTicker = time.NewTicker(m.healthInterval)
	m.healthDone = make(chan struct{})
	ticker := m.healthTicker
	done := m.healthDone
	m.mu.Unlock()

	started := m.startBackground(workHealthMonitor, func() {
		defer ticker.Stop() // Ensure ticker is stopped when goroutine exits

		for {
			select {
			case <-done:
				// Exit goroutine cleanly
				return
			case <-ticker.C:
				m.mu.RLock()
				conn := m.conn
				m.mu.RUnlock()

				if conn == nil {
					continue
				}

				healthy := conn.IsConnected()
				if _, err := conn.RTT(); err != nil {
					healthy = false
				}

				// Update status based on health
				if healthy && m.Status() != StatusConnected {
					m.setStatus(StatusConnected)
				} else if !healthy && m.Status() == StatusConnected {
					m.setStatus(StatusReconnecting)
				}
			}
		}
	})
	if !started {
		m.stopHealthMonitoring()
	}
	return started
}

// stopHealthMonitoring stops health monitoring goroutine
func (m *Client) stopHealthMonitoring() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.healthTicker != nil {
		m.healthTicker.Stop()
		m.healthTicker = nil
	}
	if m.healthDone != nil {
		close(m.healthDone)
		m.healthDone = nil
	}
}

// isAlreadyExistsError checks if an error indicates a KV bucket already exists
func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "bucket name already in use") ||
		strings.Contains(errStr, "already exists") ||
		strings.Contains(errStr, "stream name already in use")
}
