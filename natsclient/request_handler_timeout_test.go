package natsclient

import (
	"testing"
	"time"
)

// The per-message request-handler context timeout (applied inside
// SubscribeForRequests) was a hardcoded 30s. Slow LLM handlers on the
// globalSearch path (8B answer synthesis) exceed it and get cancelled
// mid-generation. These tests pin the configurable replacement: default
// stays 30s (unchanged for CI), the env var can raise it.

func TestRequestHandlerTimeout_DefaultIs30s(t *testing.T) {
	c, err := NewClient("nats://unused")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := c.requestHandlerTimeout; got != DefaultRequestHandlerTimeout {
		t.Fatalf("default requestHandlerTimeout = %v, want %v", got, DefaultRequestHandlerTimeout)
	}
	if DefaultRequestHandlerTimeout != 30*time.Second {
		t.Fatalf("DefaultRequestHandlerTimeout = %v, want 30s (CI default must not change)", DefaultRequestHandlerTimeout)
	}
}

func TestRequestHandlerTimeout_EnvOverride(t *testing.T) {
	t.Setenv("SEMENGINE_NATS_REQUEST_HANDLER_TIMEOUT", "150s")
	c, err := NewClient("nats://unused")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := c.requestHandlerTimeout; got != 150*time.Second {
		t.Fatalf("env override: requestHandlerTimeout = %v, want 150s", got)
	}
}

func TestRequestHandlerTimeout_EnvInvalidFallsBackToDefault(t *testing.T) {
	t.Setenv("SEMENGINE_NATS_REQUEST_HANDLER_TIMEOUT", "not-a-duration")
	c, err := NewClient("nats://unused")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if got := c.requestHandlerTimeout; got != DefaultRequestHandlerTimeout {
		t.Fatalf("invalid env: requestHandlerTimeout = %v, want default %v", got, DefaultRequestHandlerTimeout)
	}
}
