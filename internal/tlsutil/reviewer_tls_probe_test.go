package tlsutil

import (
	"testing"

	"github.com/c360studio/semengine/pkg/security"
)

func TestReviewerMTLSWithoutTLSDoesNotPanic(t *testing.T) {
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("configuration panicked instead of refusing: %v", p)
		}
	}()
	cfg, err := LoadServerTLSConfig(security.ServerTLSConfig{MTLS: security.ServerMTLSConfig{Enabled: true}})
	if err == nil {
		t.Errorf("mTLS without TLS accepted: cfg=%v", cfg)
	}
}
