package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
)

// The message logger's shared test helpers (design D7, U-a). The pin's createTestMessageLogger
// (message_logger_test.go:76-96) dialled a fixed address and built the logger through the dropped
// NewMessageLogger; here every logger is built the one production way, NewMessageLoggerService,
// so a test sees only what the constructor composes.

// newTestMessageLogger builds a message logger through NewMessageLoggerService from config, a JSON
// MessageLoggerConfig, over client and logger, with a fresh component registry and payload
// registry, as RegisterAll's composition would hand it.
func newTestMessageLogger(t *testing.T, client *natsclient.Client, config string, logger *slog.Logger) *MessageLogger {
	t.Helper()
	built, err := NewMessageLoggerService(json.RawMessage(config), &Dependencies{
		NATSClient:        client,
		Logger:            logger,
		ComponentRegistry: component.NewRegistry(),
		PayloadRegistry:   payloadregistry.New(),
	})
	require.NoError(t, err)
	ml, ok := built.(*MessageLogger)
	require.True(t, ok, "NewMessageLoggerService returned %T, want *MessageLogger", built)
	return ml
}

// holdingHandler is a slog.Handler a test composes into the logger it builds (design D2's Stop
// proofs, D4's watch overflow). It is enabled from Debug up, because the logger records each
// stored message as a Debug "Message sample" record after storing it (pin message_logger.go:1001,
// :1027), so a held record means its message is stored and its callback has not returned.
//
// It holds the first record whose message is hold until release is called, closing held when that
// record arrives; an empty hold holds nothing. arrived(msg) returns a channel closed once a record
// with message msg has been handled, before or after the call. A held record is also let go when
// the test's context ends, which is before the test's cleanups run, so a callback it holds never
// blocks a cleanup's Stop. It writes nothing.
type holdingHandler struct {
	hold        string
	held        chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
	testDone    <-chan struct{}

	mu       sync.Mutex
	taken    bool
	arrivals map[string]chan struct{}
}

// newHoldingHandler returns a handler that holds the first record whose message is hold.
func newHoldingHandler(t *testing.T, hold string) *holdingHandler {
	t.Helper()
	return &holdingHandler{
		hold:     hold,
		held:     make(chan struct{}),
		release:  make(chan struct{}),
		testDone: t.Context().Done(),
		arrivals: make(map[string]chan struct{}),
	}
}

// heldRecord returns a channel closed when the held record arrives.
func (h *holdingHandler) heldRecord() <-chan struct{} { return h.held }

// releaseHeld lets the held record, and any record that arrives later, go. It may be called more
// than once.
func (h *holdingHandler) releaseHeld() { h.releaseOnce.Do(func() { close(h.release) }) }

// arrived returns a channel closed once a record with message msg has been handled.
func (h *holdingHandler) arrived(msg string) <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.arrivalLocked(msg)
}

func (h *holdingHandler) arrivalLocked(msg string) chan struct{} {
	ch, ok := h.arrivals[msg]
	if !ok {
		ch = make(chan struct{})
		h.arrivals[msg] = ch
	}
	return ch
}

func (h *holdingHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelDebug
}

func (h *holdingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	if ch := h.arrivalLocked(r.Message); !isClosed(ch) {
		close(ch)
	}
	take := h.hold != "" && r.Message == h.hold && !h.taken
	if take {
		h.taken = true
	}
	h.mu.Unlock()
	if !take {
		return nil
	}
	close(h.held)
	select {
	case <-h.release:
	case <-h.testDone:
	}
	return nil
}

// WithAttrs and WithGroup return the handler itself: it reads only a record's message, and every
// logger derived from the one a test composes must share its hold and its arrivals.
func (h *holdingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *holdingHandler) WithGroup(string) slog.Handler      { return h }
