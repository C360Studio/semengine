package cache

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestConfig_UnmarshalJSON_DurationStrings(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
		want     Config
		wantErr  bool
	}{
		{
			name: "duration strings",
			jsonData: `{
				"enabled": true,
				"strategy": "hybrid",
				"max_size": 1000,
				"ttl": "1h",
				"cleanup_interval": "5m"
			}`,
			want: Config{
				Enabled:         true,
				Strategy:        StrategyHybrid,
				MaxSize:         1000,
				TTL:             1 * time.Hour,
				CleanupInterval: 5 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "integer nanoseconds (backward compatibility)",
			jsonData: `{
				"enabled": true,
				"strategy": "ttl",
				"ttl": 3600000000000,
				"cleanup_interval": 300000000000
			}`,
			want: Config{
				Enabled:         true,
				Strategy:        StrategyTTL,
				TTL:             1 * time.Hour,
				CleanupInterval: 5 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "mixed formats",
			jsonData: `{
				"enabled": true,
				"strategy": "hybrid",
				"max_size": 500,
				"ttl": "2h30m",
				"cleanup_interval": 60000000000
			}`,
			want: Config{
				Enabled:         true,
				Strategy:        StrategyHybrid,
				MaxSize:         500,
				TTL:             2*time.Hour + 30*time.Minute,
				CleanupInterval: 1 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "invalid duration string",
			jsonData: `{
				"enabled": true,
				"ttl": "invalid"
			}`,
			wantErr: true,
		},
		{
			name: "minimal config",
			jsonData: `{
				"enabled": false
			}`,
			want: Config{
				Enabled: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Config
			err := json.Unmarshal([]byte(tt.jsonData), &got)

			if (err != nil) != tt.wantErr {
				t.Errorf("Config.UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if got.Enabled != tt.want.Enabled {
					t.Errorf("Enabled = %v, want %v", got.Enabled, tt.want.Enabled)
				}
				if got.Strategy != tt.want.Strategy {
					t.Errorf("Strategy = %v, want %v", got.Strategy, tt.want.Strategy)
				}
				if got.MaxSize != tt.want.MaxSize {
					t.Errorf("MaxSize = %v, want %v", got.MaxSize, tt.want.MaxSize)
				}
				if got.TTL != tt.want.TTL {
					t.Errorf("TTL = %v, want %v", got.TTL, tt.want.TTL)
				}
				if got.CleanupInterval != tt.want.CleanupInterval {
					t.Errorf("CleanupInterval = %v, want %v", got.CleanupInterval, tt.want.CleanupInterval)
				}
			}
		})
	}
}

func TestConfig_UnmarshalJSON_RealWorldExample(t *testing.T) {
	// Test with a real-world objectstore config
	jsonData := `{
		"enabled": true,
		"strategy": "hybrid",
		"max_size": 5000,
		"ttl": "1h",
		"cleanup_interval": "5m"
	}`

	var cfg Config
	if err := json.Unmarshal([]byte(jsonData), &cfg); err != nil {
		t.Fatalf("UnmarshalJSON() failed: %v", err)
	}

	if cfg.TTL != 1*time.Hour {
		t.Errorf("TTL = %v, want 1h", cfg.TTL)
	}

	if cfg.CleanupInterval != 5*time.Minute {
		t.Errorf("CleanupInterval = %v, want 5m", cfg.CleanupInterval)
	}

	// Verify it validates correctly
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() failed: %v", err)
	}
}

// TestConfig_UnmarshalJSON_RefusesUnknownKeys: a key Config does not have is an error, never
// silently ignored (owner ruling, #9 comment 5968830525, rule 2(b); Codex F5). The oracle is the
// error and its naming the key.
func TestConfig_UnmarshalJSON_RefusesUnknownKeys(t *testing.T) {
	for _, data := range []string{
		`{"enabled": true, "strategy": "lru", "max_size": 10, "max_entries": 10}`,
		`{"enabled": true, "strategy": "ttl", "ttl": "1m", "cleanup_interval": "1s", "TTL_typo": "1m"}`,
	} {
		var cfg Config
		err := json.Unmarshal([]byte(data), &cfg)
		if err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Errorf("Unmarshal(%s) error = %v, want an unknown-field error", data, err)
		}
	}
}

// TestConfig_UnmarshalJSON_RefusesStatsInterval: stats_interval was parsed, defaulted and
// validated but read by no behaviour, so it is removed (owner ruling, #9 comment 5968830525;
// Codex F4), and a configuration that still sets it fails loudly instead of being ignored.
func TestConfig_UnmarshalJSON_RefusesStatsInterval(t *testing.T) {
	var cfg Config
	err := json.Unmarshal([]byte(`{"enabled": true, "strategy": "lru", "max_size": 10, "stats_interval": "30s"}`), &cfg)
	if err == nil || !strings.Contains(err.Error(), `"stats_interval"`) {
		t.Fatalf("error = %v, want an unknown-field error naming stats_interval", err)
	}
}
