package cache

import (
	"testing"
	"time"
)

// TestCacheUsableAfterClose proves that Close stops only background work: every implementation
// NewFromConfig builds answers Get, Set and Delete after Close exactly as it did before. graph-ingest
// relies on this when a query that loaded the cache handle before Stop cleared it runs after Close
// (design D7 of setup-04a-04-graph-roots-processor-shell, #306).
//
// spec: graph-ingest-recovery / The entity cache stays usable through Stop (scenario: A cache after Close)
func TestCacheUsableAfterClose(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
		// stores is false for the disabled cache, which misses on every Get before and after Close.
		stores bool
	}{
		{"simple", Config{Enabled: true, Strategy: StrategySimple}, true},
		{"lru", Config{Enabled: true, Strategy: StrategyLRU, MaxSize: 8}, true},
		// The TTL outlasts the test, so a miss below is never an expiry.
		{"ttl", Config{Enabled: true, Strategy: StrategyTTL, TTL: time.Hour, CleanupInterval: time.Minute}, true},
		{"hybrid", Config{
			Enabled: true, Strategy: StrategyHybrid, MaxSize: 8, TTL: time.Hour, CleanupInterval: time.Minute,
		}, true},
		{"noop", Config{Enabled: false}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewFromConfig[string](t.Context(), tc.cfg)
			if err != nil {
				t.Fatalf("NewFromConfig: %v", err)
			}
			if _, err := c.Set("before", "v0"); err != nil {
				t.Fatalf("Set before Close: %v", err)
			}
			if err := c.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			// Every call below runs after Close returned.
			wantGet := func(key, want string) {
				t.Helper()
				if !tc.stores {
					want = ""
				}
				got, ok := c.Get(key)
				if got != want || ok != (want != "") {
					t.Fatalf("Get(%q) after Close = (%q, %t), want (%q, %t)", key, got, ok, want, want != "")
				}
			}

			wantGet("before", "v0")

			isNew, err := c.Set("after", "v1")
			if err != nil || isNew != tc.stores {
				t.Fatalf("Set(new key) after Close = (%t, %v), want (%t, nil)", isNew, err, tc.stores)
			}
			wantGet("after", "v1")

			isNew, err = c.Set("before", "v2")
			if err != nil || isNew {
				t.Fatalf("Set(existing key) after Close = (%t, %v), want (false, nil)", isNew, err)
			}
			wantGet("before", "v2")

			deleted, err := c.Delete("before")
			if err != nil || deleted != tc.stores {
				t.Fatalf("Delete after Close = (%t, %v), want (%t, nil)", deleted, err, tc.stores)
			}
			wantGet("before", "")
			wantGet("after", "v1")
		})
	}
}
