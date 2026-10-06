package natsclient

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTraceparent(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		wantErr bool
		traceID string
		spanID  string
		sampled bool
	}{
		{
			name:    "valid sampled",
			header:  "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			wantErr: false,
			traceID: "0af7651916cd43dd8448eb211c80319c",
			spanID:  "b7ad6b7169203331",
			sampled: true,
		},
		{
			name:    "valid not sampled",
			header:  "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00",
			wantErr: false,
			traceID: "0af7651916cd43dd8448eb211c80319c",
			spanID:  "b7ad6b7169203331",
			sampled: false,
		},
		{
			name:    "invalid version",
			header:  "01-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
			wantErr: true,
		},
		{
			name:    "missing parts",
			header:  "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331",
			wantErr: true,
		},
		{
			name:    "invalid trace ID length",
			header:  "00-0af7651916cd43dd-b7ad6b7169203331-01",
			wantErr: true,
		},
		{
			name:    "invalid span ID length",
			header:  "00-0af7651916cd43dd8448eb211c80319c-b7ad6b71-01",
			wantErr: true,
		},
		{
			name:    "invalid hex in trace ID",
			header:  "00-0af7651916cd43dd8448eb211c80319Z-b7ad6b7169203331-01",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc, err := ParseTraceparent(tt.header)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.traceID, tc.TraceID)
			assert.Equal(t, tt.spanID, tc.SpanID)
			assert.Equal(t, tt.sampled, tc.Sampled)
		})
	}
}

func TestFormatTraceparent(t *testing.T) {
	tc := &TraceContext{
		TraceID: "0af7651916cd43dd8448eb211c80319c",
		SpanID:  "b7ad6b7169203331",
		Sampled: true,
	}

	result := tc.FormatTraceparent()
	assert.Equal(t, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", result)

	// Test not sampled
	tc.Sampled = false
	result = tc.FormatTraceparent()
	assert.Equal(t, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-00", result)
}

func TestRoundTrip(t *testing.T) {
	original := "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"
	tc, err := ParseTraceparent(original)
	require.NoError(t, err)

	formatted := tc.FormatTraceparent()
	assert.Equal(t, original, formatted)
}

func TestNewTraceContext(t *testing.T) {
	tc := NewTraceContext()
	assert.Len(t, tc.TraceID, 32, "trace ID should be 32 hex chars")
	assert.Len(t, tc.SpanID, 16, "span ID should be 16 hex chars")
	assert.True(t, tc.Sampled, "new trace should be sampled by default")
	assert.Empty(t, tc.ParentSpanID, "new trace should have no parent")
}

func TestNewSpan(t *testing.T) {
	parent := NewTraceContext()
	child := parent.NewSpan()

	assert.Equal(t, parent.TraceID, child.TraceID, "child should inherit trace ID")
	assert.NotEqual(t, parent.SpanID, child.SpanID, "child should have new span ID")
	assert.Equal(t, parent.SpanID, child.ParentSpanID, "child's parent should be parent's span")
	assert.Equal(t, parent.Sampled, child.Sampled, "child should inherit sampled flag")
}

func TestContextWithTrace(t *testing.T) {
	tc := NewTraceContext()
	ctx := ContextWithTrace(context.Background(), tc)

	extracted, ok := TraceContextFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, tc, extracted)
}

func TestTraceContextFromContext_NotSet(t *testing.T) {
	ctx := context.Background()
	tc, ok := TraceContextFromContext(ctx)
	assert.False(t, ok)
	assert.Nil(t, tc)
}

func TestInjectAndExtractTrace(t *testing.T) {
	tc := &TraceContext{
		TraceID:      "0af7651916cd43dd8448eb211c80319c",
		SpanID:       "b7ad6b7169203331",
		ParentSpanID: "1234567890abcdef",
		Sampled:      true,
	}

	ctx := ContextWithTrace(context.Background(), tc)
	msg := &nats.Msg{}

	InjectTrace(ctx, msg)

	assert.Equal(t, "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01", msg.Header.Get(TraceparentHeader))
	assert.Equal(t, tc.TraceID, msg.Header.Get(TraceIDHeader))
	assert.Equal(t, tc.SpanID, msg.Header.Get(SpanIDHeader))
	assert.Equal(t, tc.ParentSpanID, msg.Header.Get(ParentSpanHeader))

	// Extract and verify
	extracted := ExtractTrace(msg)
	require.NotNil(t, extracted)
	assert.Equal(t, tc.TraceID, extracted.TraceID)
	assert.Equal(t, tc.SpanID, extracted.SpanID)
	assert.Equal(t, tc.Sampled, extracted.Sampled)
}

func TestInjectTrace_NoContext(t *testing.T) {
	ctx := context.Background()
	msg := &nats.Msg{}

	InjectTrace(ctx, msg)

	// Should not crash and should not add headers
	assert.Nil(t, msg.Header)
}

func TestExtractTrace_NoHeaders(t *testing.T) {
	msg := &nats.Msg{}
	tc := ExtractTrace(msg)
	assert.Nil(t, tc)
}

func TestExtractTrace_FallbackToSimpleHeaders(t *testing.T) {
	msg := &nats.Msg{
		Header: make(nats.Header),
	}
	msg.Header.Set(TraceIDHeader, "0af7651916cd43dd8448eb211c80319c")
	msg.Header.Set(SpanIDHeader, "b7ad6b7169203331")

	tc := ExtractTrace(msg)
	require.NotNil(t, tc)
	assert.Equal(t, "0af7651916cd43dd8448eb211c80319c", tc.TraceID)
	assert.Equal(t, "b7ad6b7169203331", tc.SpanID)
}

func TestIsHexString(t *testing.T) {
	assert.True(t, isHexString("0123456789abcdef"))
	assert.True(t, isHexString("0123456789ABCDEF"))
	assert.True(t, isHexString(""))
	assert.False(t, isHexString("0123456789abcdefg"))
	assert.False(t, isHexString("ghijklmnop"))
}
