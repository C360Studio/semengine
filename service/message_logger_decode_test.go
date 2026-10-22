package service

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/payloadregistry"
)

// decodeProbePayload is a payload only this file registers, so the logger can decode it only
// through the payload registry its Dependencies hand it (design D1, ADR-093).
type decodeProbePayload struct {
	Note string `json:"note"`
}

func (p *decodeProbePayload) Schema() message.Type {
	return message.Type{Domain: "test", Category: "logger_probe", Version: "v1"}
}

func (p *decodeProbePayload) Validate() error {
	if p.Note == "" {
		return errors.New("note is required")
	}
	return nil
}

func (p *decodeProbePayload) MarshalJSON() ([]byte, error) {
	type plain decodeProbePayload
	return json.Marshal((*plain)(p))
}

func (p *decodeProbePayload) UnmarshalJSON(data []byte) error {
	type plain decodeProbePayload
	return json.Unmarshal(data, (*plain)(p))
}

// TestMessageLoggerDecodesPayloadsFromItsRegistry: a message logger built with a payload registry
// that holds a payload type logs a BaseMessage of that type under the type's registered name and
// summarises it as the concrete Go type, not as raw JSON (pin message_logger_user_response_test.go
// :16-49, with a test payload in place of agentic.UserResponse; design D1, D7 U-g).
//
// The logger is built through NewMessageLoggerService directly, not newTestMessageLogger, because
// that helper hands every logger an empty payload registry.
func TestMessageLoggerDecodesPayloadsFromItsRegistry(t *testing.T) {
	registry := payloadregistry.New()
	require.NoError(t, registry.Register(&payloadregistry.Registration{
		Domain: "test", Category: "logger_probe", Version: "v1",
		Description: "message logger decode probe",
		Factory:     func() any { return &decodeProbePayload{} },
	}))
	created, err := NewMessageLoggerService(json.RawMessage(`{
		"monitor_subjects":["probe.logger.>"],
		"max_entries":10,
		"sample_rate":1
	}`), &Dependencies{
		NATSClient:        newUnconnectedClient(t),
		ComponentRegistry: component.NewRegistry(),
		PayloadRegistry:   registry,
	})
	require.NoError(t, err)
	logger, ok := created.(*MessageLogger)
	require.True(t, ok, "NewMessageLoggerService returned %T, want *MessageLogger", created)

	probe := &decodeProbePayload{Note: "observed"}
	wire, err := json.Marshal(message.NewBaseMessage(probe.Schema(), probe, "decode-probe-test"))
	require.NoError(t, err)

	logger.handleMessage(t.Context(), "probe.logger.one", wire)

	entries := logger.GetLogEntries(1)
	require.Len(t, entries, 1)
	require.Equal(t, "probe.logger.one", entries[0].Subject)
	require.Equal(t, "test.logger_probe.v1", entries[0].MessageType)
	require.Contains(t, entries[0].Summary, "*service.decodeProbePayload")
}
