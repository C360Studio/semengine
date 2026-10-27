package follower

import (
	"context"
	"strings"
	"testing"

	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
)

// TestOpenRefusesMissingInputs: Open refuses each input the entity-state-follower spec names
// ("Open waits for ENTITY_STATES within a bound") before it makes any NATS call, so a zero client
// value is enough to reach every check after the client's.
func TestOpenRefusesMissingInputs(t *testing.T) {
	valid := func() Config {
		return Config{
			Client:  &natsclient.Client{},
			Key:     "graph-index",
			Deliver: func(context.Context, Entry) {},
			Failed:  func() int64 { return 0 },
		}
	}
	cases := []struct {
		name  string
		ctx   context.Context
		edit  func(*Config)
		names string
	}{
		{"nil context", nil, func(*Config) {}, "nil context"},
		{"nil client", t.Context(), func(c *Config) { c.Client = nil }, "nil client"},
		{"empty key", t.Context(), func(c *Config) { c.Key = "" }, "empty status key"},
		{"nil Deliver", t.Context(), func(c *Config) { c.Deliver = nil }, "nil Deliver"},
		{"nil Failed", t.Context(), func(c *Config) { c.Failed = nil }, "nil Failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid()
			tc.edit(&cfg)
			f, err := Open(tc.ctx, cfg)
			if err == nil || f != nil {
				t.Fatalf("Open = %v, %v; want a refusal", f, err)
			}
			if !errs.IsInvalid(err) {
				t.Errorf("Open error %v is not class invalid", err)
			}
			if !strings.Contains(err.Error(), tc.names) {
				t.Errorf("Open error %q does not name %q", err, tc.names)
			}
		})
	}
}
