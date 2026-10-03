package cache

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

// FuzzConfigUnmarshalJSON: Config.UnmarshalJSON decodes outside bytes (task 3.6a, Codex F5). The
// invariants come from the decoder's documented contract (a duration is a duration string or
// integer nanoseconds; an unknown key is refused), checked with the standard library, never with
// the decoder's own helpers:
//
//   - it never panics;
//   - an accepted document holds only Config's keys (enabled, strategy, max_size, ttl,
//     cleanup_interval, matched as encoding/json matches them, ignoring case);
//   - an accepted document gives each duration the value the test derives itself from that key's
//     raw JSON: time.ParseDuration of a string, or time.Duration of an integer; an absent key
//     gives zero;
//   - an accepted Config survives json.Marshal and a second decode unchanged.
//
// A document holding a duration key twice, in any letter case, is decoded by encoding/json's own
// last-match rule; the duration invariant is not asserted for it (the round trip still is).
func FuzzConfigUnmarshalJSON(f *testing.F) {
	for _, seed := range []string{
		// accepted
		`{"enabled": true, "strategy": "hybrid", "max_size": 1000, "ttl": "1h", "cleanup_interval": "5m"}`,
		`{"enabled": true, "strategy": "ttl", "ttl": 3600000000000, "cleanup_interval": 300000000000}`,
		`{"enabled": true, "strategy": "ttl", "ttl": "1h30m", "cleanup_interval": 1000000000}`,
		`{"strategy": "lru", "max_size": 10}`,
		`{"ttl": "-5s"}`,
		`{"TTL": "2m"}`,
		`{}`,
		`null`,
		// refused
		`{"enabled": true, "stats_interval": "30s"}`,
		`{"max_entries": 10}`,
		`{"ttl": "5"}`,
		`{"ttl": "forever"}`,
		`{"ttl": 1.5e9}`,
		`{"ttl": null}`,
		`{"ttl": true}`,
		`{"max_size": "ten"}`,
		`[1, 2]`,
		`"ttl"`,
		``,
		`{"ttl": "1m"`,
		`{"ttl": "1m", "ttl": 7}`,
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var got Config
		if err := json.Unmarshal(data, &got); err != nil {
			return // refused; the only invariant on a refusal is that it did not panic
		}

		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) == nil {
			for k := range fields {
				known := false
				for _, want := range []string{"enabled", "strategy", "max_size", "ttl", "cleanup_interval"} {
					known = known || strings.EqualFold(k, want)
				}
				if !known {
					t.Fatalf("accepted %q with the unknown key %q", data, k)
				}
			}
			for key, want := range map[string]time.Duration{"ttl": got.TTL, "cleanup_interval": got.CleanupInterval} {
				raw, n := foldLookup(fields, key)
				if n > 1 {
					continue
				}
				expected, ok := oracleDuration(raw)
				if !ok {
					t.Fatalf("accepted %q, but %s's raw value %s is neither a duration string nor an integer", data, key, raw)
				}
				if want != expected {
					t.Fatalf("decoding %q: %s = %v, want %v from %s", data, key, want, expected, raw)
				}
			}
		}

		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("marshal %+v: %v", got, err)
		}
		var again Config
		if err := json.Unmarshal(encoded, &again); err != nil {
			t.Fatalf("decoding the marshalled %s: %v", encoded, err)
		}
		if again != got {
			t.Fatalf("round trip of %q: %+v, then %+v", data, got, again)
		}
	})
}

// foldLookup finds key in fields with encoding/json's case-insensitive match, and reports how
// many keys matched.
func foldLookup(fields map[string]json.RawMessage, key string) (json.RawMessage, int) {
	var raw json.RawMessage
	n := 0
	for k, v := range fields {
		if strings.EqualFold(k, key) {
			raw = v
			n++
		}
	}
	return raw, n
}

// oracleDuration derives a duration from its raw JSON: absent is zero, a string is
// time.ParseDuration, an integer literal is nanoseconds.
func oracleDuration(raw json.RawMessage) (time.Duration, bool) {
	if raw == nil {
		return 0, true
	}
	trimmed := bytes.TrimSpace(raw)
	var s string
	if len(trimmed) > 0 && trimmed[0] == '"' && json.Unmarshal(trimmed, &s) == nil {
		d, err := time.ParseDuration(s)
		return d, err == nil
	}
	n, err := strconv.ParseInt(string(trimmed), 10, 64)
	return time.Duration(n), err == nil
}
