package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/internal/flowgraph"
	"github.com/c360studio/semengine/internal/lifecycleguard"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// NewMessageLoggerService creates the message logger from its JSON configuration
// (MessageLoggerConfig). A key the configuration does not define, or one in another letter case,
// is refused, naming the key. It refuses, as invalid data and naming the input, a nil NATS
// client, a nil payload registry, "*" in monitor_subjects (the default) without a component
// registry, max_entries below 0 or above 100000, and sample_rate below 0. The logger decodes
// typed envelopes through deps.PayloadRegistry and, with "*", reads the subjects of the admitted
// components from deps.ComponentRegistry when it starts.
func NewMessageLoggerService(rawConfig json.RawMessage, deps *Dependencies) (Service, error) {
	var cfg MessageLoggerConfig
	if err := decodeStrictServiceJSON(rawConfig, &cfg); err != nil {
		return nil, errs.WrapInvalid(err, "MessageLogger", "NewMessageLoggerService", "parse message-logger config")
	}
	if err := cfg.Validate(); err != nil {
		return nil, errs.WrapInvalid(err, "MessageLogger", "NewMessageLoggerService", "validate message-logger config")
	}

	// Defaults have one home: here.
	if cfg.MaxEntries == 0 {
		cfg.MaxEntries = 10000
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 1
	}
	if len(cfg.MonitorSubjects) == 0 {
		cfg.MonitorSubjects = []string{"*"}
	}

	if deps == nil || deps.NATSClient == nil {
		return nil, refuseMessageLoggerConfig("message-logger requires a NATS client")
	}
	if deps.PayloadRegistry == nil {
		return nil, refuseMessageLoggerConfig("message-logger requires a payload registry")
	}
	autoDiscover := containsWildcard(cfg.MonitorSubjects)
	if autoDiscover && deps.ComponentRegistry == nil {
		return nil, refuseMessageLoggerConfig(`message-logger with "*" in monitor_subjects requires a component registry`)
	}

	base := NewBaseServiceWithOptions("message-logger", WithLogger(deps.Logger), WithMetrics(deps.MetricsRegistry))
	ml := &MessageLogger{
		BaseService:       base,
		config:            cfg,
		decoder:           message.NewDecoder(deps.PayloadRegistry),
		natsClient:        deps.NATSClient,
		componentRegistry: deps.ComponentRegistry,
		autoDiscover:      autoDiscover,
		entries:           make([]MessageLogEntry, cfg.MaxEntries),
		subscriptions:     make(map[string]*natsclient.Subscription),
		pending:           make(map[string]error),
		subjectMetadata:   make(map[string]portMetadata),
		watchStreams:      make(map[*watchStream]struct{}),
		logger:            base.logger.With("source", "message-logger"),
	}
	ml.stats.startTime = time.Now()
	ml.stats.lastMessageTime.Store(ml.stats.startTime)
	ml.SetHealthCheck(ml.healthCheck)
	return ml, nil
}

func refuseMessageLoggerConfig(reason string) error {
	return errs.WrapInvalid(errors.New(reason), "MessageLogger", "NewMessageLoggerService", "check dependencies")
}

// containsWildcard reports whether subjects holds "*", which asks the logger to monitor the
// subjects of the admitted components. A NATS wildcard inside a subject does not.
func containsWildcard(subjects []string) bool {
	return slices.Contains(subjects, "*")
}

// uniqueStrings returns values without repeats, each kept at its first occurrence.
func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// MessageLoggerConfig holds configuration for the MessageLogger service.
type MessageLoggerConfig struct {
	// MonitorSubjects are the NATS subjects to monitor. "*" adds the NATS, NATS-request and
	// JetStream port subjects of every admitted component. Empty means ["*"].
	// Example: ["*"] or ["*", "debug.>"] or ["raw.udp.messages", "processed.>"]
	MonitorSubjects []string `json:"monitor_subjects"`

	// MaxEntries is how many entries the logger keeps in memory, 0 to 100000; 0 means 10000.
	MaxEntries int `json:"max_entries"`

	// OutputToStdout prints one line per stored entry.
	OutputToStdout bool `json:"output_to_stdout"`

	// SampleRate stores one in every SampleRate messages and counts the rest; 0 and 1 store
	// every message.
	SampleRate int `json:"sample_rate"`
}

// Validate checks the configuration's bounds.
func (c MessageLoggerConfig) Validate() error {
	if c.MaxEntries < 0 {
		return errors.New("max_entries cannot be negative")
	}
	if c.MaxEntries > 100000 {
		return errors.New("max_entries cannot exceed 100000")
	}
	if c.SampleRate < 0 {
		return errors.New("sample_rate cannot be negative")
	}
	return nil
}

// MessageLogEntry is one stored message. RawData is the payload as published when it is JSON,
// and the payload as a JSON string when it is not.
type MessageLogEntry struct {
	Sequence    uint64          `json:"sequence"` // Monotonic sequence; owns the entry's buffer slot
	Timestamp   time.Time       `json:"timestamp"`
	Subject     string          `json:"subject"`
	MessageType string          `json:"message_type,omitempty"`
	MessageID   string          `json:"message_id,omitempty"`
	TraceID     string          `json:"trace_id,omitempty"` // W3C trace ID (32 hex chars)
	SpanID      string          `json:"span_id,omitempty"`  // W3C span ID (16 hex chars)
	Summary     string          `json:"summary"`
	RawData     json.RawMessage `json:"raw_data,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
}

// portMetadata describes the port that declares a monitored subject, for the logger's lines.
type portMetadata struct {
	Component string // Component instance name
	PortName  string // Port name
	PortType  string // Port kind (e.g. "jetstream", "nats")
	Interface string // Interface contract (e.g. "core.json.v1")
}

// subjectOverlap reports a desired subject the logger does not subscribe because another
// desired subject, Broader, covers it.
type subjectOverlap struct {
	Broader    string `json:"broader"`
	Covered    string `json:"covered"`
	Resolution string `json:"resolution"`
}

// coveredSubjectOmitted is the one Resolution a subjectOverlap carries.
const coveredSubjectOmitted = "covered subscription omitted"

// subscribeRetryInterval is how often the logger retries the subjects it failed to subscribe.
const subscribeRetryInterval = 100 * time.Millisecond

// watchStream is one open KV watch stream, registered by openWatchStream and removed by
// closeWatchStream. The logger's Stop closes stop to end the stream and waits for done, which
// closeWatchStream closes once the stream's KV watcher has stopped.
type watchStream struct {
	stop chan struct{}
	done chan struct{}
}

// MessageLogger observes the messages published on its subjects and keeps the newest in memory
// for its HTTP routes. Start subscribes; Stop is the only stop (ADR-113).
type MessageLogger struct {
	*BaseService

	config            MessageLoggerConfig
	decoder           *message.Decoder
	natsClient        *natsclient.Client
	componentRegistry *component.Registry
	autoDiscover      bool
	logger            *slog.Logger

	// lifecycle admits one Start and makes Stop the only stop (ADR-113).
	lifecycle lifecycleguard.Guard

	// lifecycleMu guards the fields below it. It is never held across a call that can block:
	// a subscribe, a drain, a join or a log line.
	lifecycleMu sync.Mutex
	// runCancel ends the run context Start creates, which every subscription runs under.
	runCancel context.CancelFunc
	// subscriptions holds each subscribed subject's subscription until a Stop drained it.
	subscriptions map[string]*natsclient.Subscription
	// pending holds each subject whose subscribe failed, with its last error, until the retry
	// subscribes it. While it is not empty the logger is degraded and unhealthy.
	pending map[string]error
	// retryStop, closed by Stop, ends the retry; retryDone is closed when the retry has returned,
	// and is nil when no retry was started or Stop has joined it.
	retryStop chan struct{}
	retryDone chan struct{}
	// overlaps reports the desired subjects not subscribed because another covers them.
	overlaps []subjectOverlap
	// subjectMetadata names the declaring port of each subject the registry contributed.
	subjectMetadata map[string]portMetadata
	// watchesClosed is set when a Stop begins; openWatchStream then refuses.
	watchesClosed bool
	// watchStreams holds the open KV watch streams.
	watchStreams map[*watchStream]struct{}

	// Message storage (circular buffer); a slot belongs to the entry with the highest sequence.
	entries      []MessageLogEntry
	entriesMu    sync.RWMutex
	nextSequence atomic.Uint64

	// messageCount counts messages for sampling.
	messageCount atomic.Uint64

	stats struct {
		totalMessages   atomic.Int64
		validMessages   atomic.Int64
		invalidMessages atomic.Int64
		sampledMessages atomic.Int64
		startTime       time.Time
		lastMessageTime atomic.Value // time.Time
	}
}

// Start computes the subjects to monitor, refuses a malformed one, starts the base service and
// subscribes to each subject in sorted order. It refuses a nil or ended context, a second Start
// and a Start after any Stop (errs.ErrAlreadyStarted); a refused Start holds nothing.
//
// The subjects are the explicit monitor_subjects and, with "*", the NATS, NATS-request and
// JetStream port subjects of every admitted component (skipping "*", ">" and _INBOX). Every one
// is checked with flowgraph.ValidSubject before anything else happens; the first malformed one
// refuses Start as invalid data, naming the subject and where it came from. A subject another
// covers is not subscribed and is reported in the statistics' subject_overlaps.
//
// A subscribe that fails leaves the logger running: the subject is retried every 100 ms until it
// subscribes, and meanwhile the statistics report the logger degraded and its health check fails,
// naming the subject. ctx bounds Start only; ending it after Start returned stops nothing. ctx is
// checked before each subscribe, but a subscribe already in progress runs to its own end.
func (ml *MessageLogger) Start(ctx context.Context) error {
	return ml.lifecycle.Start(ctx, ml.start, ml.cleanup)
}

// start is Start's work, run by the lifecycle guard; a failure is rolled back through cleanup.
func (ml *MessageLogger) start(ctx context.Context) error {
	desired, metadata, err := ml.desiredSubjects()
	if err != nil {
		return err
	}
	subjects, overlaps := resolveLoggerSubjects(desired)

	if err := ml.BaseService.Start(ctx); err != nil {
		return err
	}
	ml.logger.Info("MessageLogger starting")

	// The run context: Start's values without its cancellation, ended only by Stop. natsclient
	// makes it the parent of every message's context.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	ml.lifecycleMu.Lock()
	ml.runCancel = cancel
	ml.overlaps = overlaps
	ml.subjectMetadata = metadata
	ml.lifecycleMu.Unlock()

	failed := 0
	for _, subject := range subjects {
		if err := ctx.Err(); err != nil {
			return errs.WrapTransient(err, "MessageLogger", "Start", "context ended before subscribe "+subject)
		}
		if ml.subscribeSubject(runCtx, subject) != nil {
			failed++
		}
	}
	if failed > 0 {
		stop, done := make(chan struct{}), make(chan struct{})
		ml.lifecycleMu.Lock()
		ml.retryStop, ml.retryDone = stop, done
		ml.lifecycleMu.Unlock()
		go ml.retrySubscribes(runCtx, stop, done)
	}

	ml.logger.Info("MessageLogger started",
		"monitored_subjects", len(subjects)-failed,
		"failed_subjects", failed,
		"max_entries", ml.config.MaxEntries,
		"output_to_stdout", ml.config.OutputToStdout)
	return nil
}

// desiredSubjects computes the subjects to monitor and checks each one, before any is removed as
// a duplicate or as covered. It returns the set and the declaring port of each subject the
// registry contributed.
func (ml *MessageLogger) desiredSubjects() (map[string]struct{}, map[string]portMetadata, error) {
	desired := make(map[string]struct{}, len(ml.config.MonitorSubjects))
	metadata := make(map[string]portMetadata)
	for _, subject := range ml.config.MonitorSubjects {
		if subject == "*" {
			continue
		}
		if !flowgraph.ValidSubject(subject) {
			return nil, nil, malformedSubject(fmt.Errorf("subject %q in monitor_subjects is not a valid NATS subject", subject))
		}
	}
	for _, subject := range uniqueStrings(ml.config.MonitorSubjects) {
		if subject != "*" {
			desired[subject] = struct{}{}
		}
	}
	if !ml.autoDiscover {
		return desired, metadata, nil
	}
	for _, declaration := range ml.componentRegistry.Snapshots() {
		name := declaration.InstanceName
		if err := collectDeclaredSubjects(name, declaration.InputPorts, declaration.InputFacts, desired, metadata); err != nil {
			return nil, nil, err
		}
		if err := collectDeclaredSubjects(name, declaration.OutputPorts, declaration.OutputFacts, desired, metadata); err != nil {
			return nil, nil, err
		}
	}
	return desired, metadata, nil
}

func malformedSubject(err error) error {
	return errs.WrapInvalid(err, "MessageLogger", "Start", "validate monitored subjects")
}

// collectDeclaredSubjects adds the NATS, NATS-request and JetStream subjects of one component's
// ports to desired, refusing a malformed one. facts[i] describes ports[i] (component.Declaration).
func collectDeclaredSubjects(
	componentName string,
	ports []component.Port,
	facts []component.PortFacts,
	desired map[string]struct{},
	metadata map[string]portMetadata,
) error {
	for index, port := range ports {
		portFacts := facts[index]
		switch portFacts.Kind() {
		case component.PortKindNATS, component.PortKindNATSRequest, component.PortKindJetStream:
		default:
			continue
		}
		interfaceType := ""
		if contract, ok := portFacts.Interface(); ok {
			interfaceType = contract.Type
		}
		for _, subject := range portFacts.NATSSubjects() {
			if subject == "*" || subject == ">" || subject == "_INBOX" || strings.HasPrefix(subject, "_INBOX.") {
				continue
			}
			if !flowgraph.ValidSubject(subject) {
				return malformedSubject(fmt.Errorf("subject %q declared by component %q port %q is not a valid NATS subject",
					subject, componentName, port.Name))
			}
			desired[subject] = struct{}{}
			if _, exists := metadata[subject]; !exists {
				metadata[subject] = portMetadata{
					Component: componentName,
					PortName:  port.Name,
					PortType:  string(portFacts.Kind()),
					Interface: interfaceType,
				}
			}
		}
	}
	return nil
}

// resolveLoggerSubjects turns a validated set of desired subjects into the sorted subjects to
// subscribe and the omissions: a subject another desired subject covers (flowgraph.SubjectCovers)
// is not subscribed, so every desired subject is covered by a subscribed one and no subscribed
// subject covers another. Each omission is reported once, naming a subscribed subject that covers
// it. Two subscribed subjects can still overlap without either covering the other.
func resolveLoggerSubjects(desired map[string]struct{}) ([]string, []subjectOverlap) {
	all := slices.Sorted(maps.Keys(desired))
	// covered reports whether another subject in all covers s; of two subjects that cover each
	// other the smaller is kept, so one of them is always subscribed.
	covered := func(s string) bool {
		for _, other := range all {
			if other != s && flowgraph.SubjectCovers(other, s) && (!flowgraph.SubjectCovers(s, other) || other < s) {
				return true
			}
		}
		return false
	}

	var subjects, omitted []string
	for _, s := range all {
		if covered(s) {
			omitted = append(omitted, s)
		} else {
			subjects = append(subjects, s)
		}
	}

	var overlaps []subjectOverlap
	for _, s := range omitted {
		broader := ""
		for _, candidates := range [][]string{subjects, all} {
			if i := slices.IndexFunc(candidates, func(c string) bool {
				return c != s && flowgraph.SubjectCovers(c, s)
			}); i >= 0 {
				broader = candidates[i]
				break
			}
		}
		overlaps = append(overlaps, subjectOverlap{Broader: broader, Covered: s, Resolution: coveredSubjectOmitted})
	}
	if subjects == nil {
		subjects = []string{}
	}
	return subjects, overlaps
}

// subscribeSubject subscribes to subject under runCtx and records the outcome: the subscription,
// or the subject as pending with its error, logged.
func (ml *MessageLogger) subscribeSubject(runCtx context.Context, subject string) error {
	subscription, err := ml.natsClient.Subscribe(runCtx, subject, func(msgCtx context.Context, msg *nats.Msg) {
		ml.handleMessage(msgCtx, msg.Subject, msg.Data)
	})
	ml.lifecycleMu.Lock()
	if err != nil {
		ml.pending[subject] = err
	} else {
		delete(ml.pending, subject)
		ml.subscriptions[subject] = subscription
	}
	ml.lifecycleMu.Unlock()
	if err != nil {
		ml.logger.Error("Failed to subscribe to subject; retrying", "subject", subject, "error", err)
		return err
	}
	ml.logger.Debug("Subscribed to subject", "subject", subject)
	return nil
}

// retrySubscribes retries the pending subjects every subscribeRetryInterval until none is left or
// stop is closed. A subscribe in progress when stop closes runs to its own end. It closes done
// when it returns.
func (ml *MessageLogger) retrySubscribes(runCtx context.Context, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(subscribeRetryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		ml.lifecycleMu.Lock()
		pending := slices.Sorted(maps.Keys(ml.pending))
		ml.lifecycleMu.Unlock()
		if len(pending) == 0 {
			return
		}
		for _, subject := range pending {
			select {
			case <-stop:
				return
			default:
			}
			_ = ml.subscribeSubject(runCtx, subject) // recorded and logged there
		}
	}
}

// healthCheck fails while a subject is not subscribed, naming the subjects.
func (ml *MessageLogger) healthCheck() error {
	ml.lifecycleMu.Lock()
	pending := slices.Sorted(maps.Keys(ml.pending))
	ml.lifecycleMu.Unlock()
	if len(pending) > 0 {
		return fmt.Errorf("message logger has not subscribed to %s", strings.Join(pending, ", "))
	}
	return nil
}

// Stop ends the subscribe retry and waits for it, drains every subscription so a message being
// handled is stored, then ends the run context, ends every open KV watch stream and waits for its
// watcher to stop, and stops the base service. It returns nil only once all are released. If ctx
// ends first, or a drain fails, it returns that error and leaves what is not yet released for the
// next Stop, which runs the same steps. A Stop after one that returned nil returns nil; a Stop
// while another runs is refused as transient. A watch requested once Stop has begun is refused.
func (ml *MessageLogger) Stop(ctx context.Context) error {
	if ctx != nil && ctx.Err() == nil {
		ml.lifecycleMu.Lock()
		ml.watchesClosed = true
		ml.lifecycleMu.Unlock()
	}
	if err := ml.lifecycle.Stop(ctx); err != nil {
		return err
	}
	// A watch opened before any Start: the guard runs no cleanup for a Stop before Start.
	return ml.endWatchStreams(ctx)
}

// cleanup is the one cleanup the lifecycle guard runs for a failed Start and for every Stop. Each
// step keeps what it did not release, so running it again finishes the job.
func (ml *MessageLogger) cleanup(ctx context.Context) error {
	if err := ml.joinRetry(ctx); err != nil {
		return err
	}
	if err := ml.drainSubscriptions(ctx); err != nil {
		return err
	}

	ml.lifecycleMu.Lock()
	cancel := ml.runCancel
	ml.runCancel = nil
	ml.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}

	if err := ml.endWatchStreams(ctx); err != nil {
		return err
	}
	if err := ml.BaseService.Stop(ctx); err != nil {
		return fmt.Errorf("stop message-logger base service: %w", err)
	}

	ml.lifecycleMu.Lock()
	clear(ml.pending)
	ml.overlaps = nil
	ml.lifecycleMu.Unlock()
	ml.logger.Info("MessageLogger stopped")
	return nil
}

// joinRetry ends the subscribe retry and waits for it to return.
func (ml *MessageLogger) joinRetry(ctx context.Context) error {
	ml.lifecycleMu.Lock()
	if ml.retryStop != nil {
		close(ml.retryStop)
		ml.retryStop = nil
	}
	done := ml.retryDone
	ml.lifecycleMu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
	case <-ctx.Done():
		return fmt.Errorf("wait for message-logger subscribe retry: %w", ctx.Err())
	}
	ml.lifecycleMu.Lock()
	ml.retryDone = nil
	ml.lifecycleMu.Unlock()
	return nil
}

// drainSubscriptions drains every subscription, in subject order, forgetting each one drained.
func (ml *MessageLogger) drainSubscriptions(ctx context.Context) error {
	ml.lifecycleMu.Lock()
	subscriptions := maps.Clone(ml.subscriptions)
	ml.lifecycleMu.Unlock()

	var drainErrs []error
	for _, subject := range slices.Sorted(maps.Keys(subscriptions)) {
		if err := subscriptions[subject].Drain(ctx); err != nil {
			ml.logger.Warn("Failed to drain subscription", "subject", subject, "error", err)
			drainErrs = append(drainErrs, fmt.Errorf("drain %s: %w", subject, err))
			continue
		}
		ml.lifecycleMu.Lock()
		delete(ml.subscriptions, subject)
		ml.lifecycleMu.Unlock()
	}
	return errors.Join(drainErrs...)
}

// openWatchStream registers a KV watch stream. It returns false once a Stop has begun; the
// caller then opens nothing. Otherwise the caller must call closeWatchStream once the stream's
// KV watcher has stopped, and must end the stream when stop is closed.
func (ml *MessageLogger) openWatchStream() (*watchStream, bool) {
	ml.lifecycleMu.Lock()
	defer ml.lifecycleMu.Unlock()
	if ml.watchesClosed {
		return nil, false
	}
	stream := &watchStream{stop: make(chan struct{}), done: make(chan struct{})}
	ml.watchStreams[stream] = struct{}{}
	return stream, true
}

// closeWatchStream removes a stream openWatchStream registered and tells a waiting Stop it ended.
func (ml *MessageLogger) closeWatchStream(stream *watchStream) {
	ml.lifecycleMu.Lock()
	delete(ml.watchStreams, stream)
	ml.lifecycleMu.Unlock()
	close(stream.done)
}

// endWatchStreams tells every open watch stream to end and waits until each has closed.
func (ml *MessageLogger) endWatchStreams(ctx context.Context) error {
	ml.lifecycleMu.Lock()
	streams := make([]*watchStream, 0, len(ml.watchStreams))
	for stream := range ml.watchStreams {
		// Every close of stop happens here, under lifecycleMu, so it closes once.
		select {
		case <-stream.stop:
		default:
			close(stream.stop)
		}
		streams = append(streams, stream)
	}
	ml.lifecycleMu.Unlock()

	for _, stream := range streams {
		select {
		case <-stream.done:
		case <-ctx.Done():
			return fmt.Errorf("wait for KV watch streams to end: %w", ctx.Err())
		}
	}
	return nil
}

// shouldSample reports whether this message is stored under the sample rate.
func (ml *MessageLogger) shouldSample() bool {
	if ml.config.SampleRate <= 1 {
		return true
	}
	return ml.messageCount.Add(1)%uint64(ml.config.SampleRate) == 0
}

// handleMessage counts a message and, when sampled, stores it. A message that decodes as a typed
// envelope is counted valid; any other is counted invalid and stored as "raw". A payload that is
// not JSON is stored as a JSON string, so every stored entry encodes.
func (ml *MessageLogger) handleMessage(ctx context.Context, subject string, data []byte) {
	if ctx.Err() != nil {
		return
	}

	ml.stats.totalMessages.Add(1)
	ml.stats.lastMessageTime.Store(time.Now())
	if !ml.shouldSample() {
		return
	}
	ml.stats.sampledMessages.Add(1)

	// Graph request/reply families use raw JSON without a BaseMessage envelope; they are still
	// logged, as "raw".
	var msgType, summary string
	if msg, err := ml.decoder.Decode(data); err == nil {
		ml.stats.validMessages.Add(1)
		msgType = msg.Type().String()
		summary = generateSummary(msg)
	} else {
		ml.stats.invalidMessages.Add(1)
		msgType = "raw"
		summary = generateRawSummary(subject, data)
	}

	var traceID, spanID string
	if tc, ok := natsclient.TraceContextFromContext(ctx); ok && tc != nil {
		traceID = tc.TraceID
		spanID = tc.SpanID
	}

	entry := MessageLogEntry{
		Sequence:    ml.nextSequence.Add(1),
		Timestamp:   time.Now(),
		Subject:     subject,
		MessageType: msgType,
		TraceID:     traceID,
		SpanID:      spanID,
		Summary:     summary,
		RawData:     rawPayload(data),
	}
	ml.storeEntry(entry)

	logArgs := []any{"subject", subject, "size", len(data)}
	ml.lifecycleMu.Lock()
	meta, hasMetadata := ml.subjectMetadata[subject]
	ml.lifecycleMu.Unlock()
	if hasMetadata {
		logArgs = append(logArgs, "component", meta.Component, "port", meta.PortName)
		if meta.PortType != "" {
			logArgs = append(logArgs, "port_type", meta.PortType)
		}
		if meta.Interface != "" {
			logArgs = append(logArgs, "interface", meta.Interface)
		}
	}
	// Logged after the entry is stored.
	ml.logger.Debug("Message sample", logArgs...)

	if ml.config.OutputToStdout {
		fmt.Printf("[%s] %s: %s\n", entry.Timestamp.Format("15:04:05.000"), entry.Subject, entry.Summary)
	}
}

// rawPayload returns data as stored JSON: itself when it is JSON, else a JSON string holding it.
func rawPayload(data []byte) json.RawMessage {
	if json.Valid(data) {
		return json.RawMessage(data)
	}
	quoted, _ := json.Marshal(string(data)) // marshalling a string cannot fail
	return quoted
}

// generateSummary describes a typed envelope.
func generateSummary(msg *message.BaseMessage) string {
	summary := fmt.Sprintf("Type: %s", msg.Type())
	if payload := msg.Payload(); payload != nil {
		summary += fmt.Sprintf(", Payload: %T", payload)
	}
	return summary
}

// generateRawSummary describes a payload with no envelope: its top-level keys when it is a JSON
// object, else its size.
func generateRawSummary(subject string, data []byte) string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Sprintf("Subject: %s (%d bytes)", subject, len(data))
	}
	return fmt.Sprintf("Subject: %s, Keys: %v", subject, slices.Sorted(maps.Keys(obj)))
}

// storeEntry stores an entry in the circular buffer.
func (ml *MessageLogger) storeEntry(entry MessageLogEntry) {
	ml.entriesMu.Lock()
	defer ml.entriesMu.Unlock()

	// Sequence owns the slot. Callback goroutines may reach this lock long after newer callbacks
	// have wrapped onto the same slot, so an older sequence never replaces a newer entry there.
	index := int((entry.Sequence - 1) % uint64(len(ml.entries)))
	if ml.entries[index].Sequence >= entry.Sequence {
		return
	}
	ml.entries[index] = entry
}

// GetEntriesByTrace returns the held entries of the trace traceID, oldest first. It scans the
// buffer, so an entry the buffer has overwritten is gone.
func (ml *MessageLogger) GetEntriesByTrace(traceID string) []MessageLogEntry {
	if traceID == "" {
		return nil
	}
	ml.entriesMu.RLock()
	var results []MessageLogEntry
	for _, entry := range ml.entries {
		if entry.Sequence != 0 && entry.TraceID == traceID {
			results = append(results, entry)
		}
	}
	ml.entriesMu.RUnlock()
	sort.Slice(results, func(i, j int) bool { return results[i].Sequence < results[j].Sequence })
	return results
}

// GetLogEntries returns the newest stored entries, newest first, at most limit of them; a limit
// of 0 or less returns every stored entry.
func (ml *MessageLogger) GetLogEntries(limit int) []MessageLogEntry {
	ml.entriesMu.RLock()
	defer ml.entriesMu.RUnlock()

	if limit <= 0 || limit > len(ml.entries) {
		limit = len(ml.entries)
	}
	return newestEntries(ml.entries, limit)
}

func newestEntries(entries []MessageLogEntry, limit int) []MessageLogEntry {
	result := make([]MessageLogEntry, 0, min(limit, len(entries)))
	for _, entry := range entries {
		if entry.Sequence != 0 {
			result = append(result, entry)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Sequence > result[j].Sequence })
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

// GetStatistics returns the logger's counts and subject state. valid_messages counts the messages
// decoded as typed envelopes, invalid_messages the rest. subject_reconciliation_degraded is true
// while a subject is not subscribed, and subject_reconciliation_error names each one with its
// last subscribe error.
func (ml *MessageLogger) GetStatistics() map[string]any {
	lastMessageTime, _ := ml.stats.lastMessageTime.Load().(time.Time)
	subjects, overlaps := ml.subjectInspection()

	ml.lifecycleMu.Lock()
	pendingErrs := make([]string, 0, len(ml.pending))
	for _, subject := range slices.Sorted(maps.Keys(ml.pending)) {
		pendingErrs = append(pendingErrs, fmt.Sprintf("subscribe %s: %v", subject, ml.pending[subject]))
	}
	ml.lifecycleMu.Unlock()

	return map[string]any{
		"total_messages":                  ml.stats.totalMessages.Load(),
		"sampled_messages":                ml.stats.sampledMessages.Load(),
		"valid_messages":                  ml.stats.validMessages.Load(),
		"invalid_messages":                ml.stats.invalidMessages.Load(),
		"sample_rate":                     ml.config.SampleRate,
		"start_time":                      ml.stats.startTime,
		"last_message_time":               lastMessageTime,
		"uptime_seconds":                  time.Since(ml.stats.startTime).Seconds(),
		"monitored_subjects":              subjects,
		"subject_overlaps":                overlaps,
		"subject_reconciliation_degraded": len(pendingErrs) > 0,
		"subject_reconciliation_error":    strings.Join(pendingErrs, "; "),
		"max_entries":                     ml.config.MaxEntries,
	}
}

// subjectInspection returns the subscribed subjects, sorted and never nil, and the omitted
// covered subjects. Both are empty once a Stop has released the subscriptions.
func (ml *MessageLogger) subjectInspection() ([]string, []subjectOverlap) {
	ml.lifecycleMu.Lock()
	defer ml.lifecycleMu.Unlock()
	subjects := slices.Sorted(maps.Keys(ml.subscriptions))
	if subjects == nil {
		subjects = []string{}
	}
	return subjects, slices.Clone(ml.overlaps)
}
