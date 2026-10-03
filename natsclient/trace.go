package natsclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/nats-io/nats.go"
)

// W3C Trace Context headers
const (
	// TraceparentHeader is the W3C standard trace context header
	// Format: 00-{trace_id}-{span_id}-{flags}
	// trace_id: 32 hex chars (16 bytes), span_id: 16 hex chars (8 bytes)
	TraceparentHeader = "traceparent"

	// TraceIDHeader is a simplified header for internal use
	TraceIDHeader = "X-Trace-ID"
	// SpanIDHeader is a simplified header for internal use
	SpanIDHeader = "X-Span-ID"
	// ParentSpanHeader is a simplified header for internal use
	ParentSpanHeader = "X-Parent-Span-ID"
)

// traceContextKey is the context key for trace context
type traceContextKey struct{}

// TraceContext holds trace information propagated through the system
type TraceContext struct {
	TraceID      string // 32 hex chars (16 bytes)
	SpanID       string // 16 hex chars (8 bytes)
	ParentSpanID string // 16 hex chars, empty for root span
	Sampled      bool
}

// TraceContextFromContext extracts trace context from context
func TraceContextFromContext(ctx context.Context) (*TraceContext, bool) {
	tc, ok := ctx.Value(traceContextKey{}).(*TraceContext)
	return tc, ok
}

// ContextWithTrace returns a context with trace information
func ContextWithTrace(ctx context.Context, tc *TraceContext) context.Context {
	return context.WithValue(ctx, traceContextKey{}, tc)
}

// NewTraceContext creates a new trace context with generated IDs
func NewTraceContext() *TraceContext {
	return &TraceContext{
		TraceID: generateTraceID(),
		SpanID:  generateSpanID(),
		Sampled: true,
	}
}

// NewSpan creates a child span from existing trace context
func (tc *TraceContext) NewSpan() *TraceContext {
	return &TraceContext{
		TraceID:      tc.TraceID,
		SpanID:       generateSpanID(),
		ParentSpanID: tc.SpanID,
		Sampled:      tc.Sampled,
	}
}

// InjectTrace adds trace headers to a NATS message from context
func InjectTrace(ctx context.Context, msg *nats.Msg) {
	tc, ok := TraceContextFromContext(ctx)
	if !ok || tc == nil {
		return
	}

	if msg.Header == nil {
		msg.Header = make(nats.Header)
	}

	msg.Header.Set(TraceparentHeader, tc.FormatTraceparent())
	msg.Header.Set(TraceIDHeader, tc.TraceID)
	msg.Header.Set(SpanIDHeader, tc.SpanID)
	if tc.ParentSpanID != "" {
		msg.Header.Set(ParentSpanHeader, tc.ParentSpanID)
	}
}

// ExtractTrace reads trace headers from a NATS message
func ExtractTrace(msg *nats.Msg) *TraceContext {
	if msg == nil || msg.Header == nil {
		return nil
	}

	// Try W3C traceparent first
	if tp := msg.Header.Get(TraceparentHeader); tp != "" {
		tc, err := ParseTraceparent(tp)
		if err == nil {
			return tc
		}
	}

	// Fall back to simplified headers
	traceID := msg.Header.Get(TraceIDHeader)
	if traceID == "" {
		return nil
	}

	return &TraceContext{
		TraceID:      traceID,
		SpanID:       msg.Header.Get(SpanIDHeader),
		ParentSpanID: msg.Header.Get(ParentSpanHeader),
		Sampled:      true,
	}
}

// ExtractTraceFromJetStream reads trace headers from a JetStream message
func ExtractTraceFromJetStream(headers nats.Header) *TraceContext {
	if headers == nil {
		return nil
	}

	// Try W3C traceparent first
	if tp := headers.Get(TraceparentHeader); tp != "" {
		tc, err := ParseTraceparent(tp)
		if err == nil {
			return tc
		}
	}

	// Fall back to simplified headers
	traceID := headers.Get(TraceIDHeader)
	if traceID == "" {
		return nil
	}

	return &TraceContext{
		TraceID:      traceID,
		SpanID:       headers.Get(SpanIDHeader),
		ParentSpanID: headers.Get(ParentSpanHeader),
		Sampled:      true,
	}
}

// ParseTraceparent parses W3C traceparent header
// Format: {version}-{trace_id}-{span_id}-{flags}
// Example: 00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01
func ParseTraceparent(header string) (*TraceContext, error) {
	parts := strings.Split(header, "-")
	if len(parts) != 4 {
		return nil, fmt.Errorf("invalid traceparent format: expected 4 parts, got %d", len(parts))
	}

	version := parts[0]
	if version != "00" {
		return nil, fmt.Errorf("unsupported traceparent version: %s", version)
	}

	traceID := parts[1]
	if len(traceID) != 32 || !isHexString(traceID) {
		return nil, fmt.Errorf("invalid trace ID: must be 32 hex characters")
	}

	spanID := parts[2]
	if len(spanID) != 16 || !isHexString(spanID) {
		return nil, fmt.Errorf("invalid span ID: must be 16 hex characters")
	}

	flags := parts[3]
	sampled := flags == "01"

	return &TraceContext{
		TraceID: traceID,
		SpanID:  spanID,
		Sampled: sampled,
	}, nil
}

// FormatTraceparent formats trace context as W3C traceparent
func (tc *TraceContext) FormatTraceparent() string {
	flags := "00"
	if tc.Sampled {
		flags = "01"
	}
	return fmt.Sprintf("00-%s-%s-%s", tc.TraceID, tc.SpanID, flags)
}

// generateTraceID generates a 32-character hex trace ID (16 bytes)
func generateTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// generateSpanID generates a 16-character hex span ID (8 bytes)
func generateSpanID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// isHexString checks if a string contains only hex characters
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
