// Package timestamp provides standardized Unix timestamp handling utilities.
//
// # Overview
//
// The timestamp package establishes int64 milliseconds as the canonical timestamp
// format throughout the codebase. This eliminates common timestamp parsing bugs
// and provides consistent behavior across all components.
//
// Design decisions:
//   - All timestamps are Unix milliseconds (ms since epoch, UTC)
//   - Zero value (0) means "not set" or "unknown"
//   - Functions handle zero values gracefully
//   - Automatic detection of seconds vs milliseconds input
//
// # Why Milliseconds?
//
// Milliseconds provide sufficient precision for most applications while avoiding
// the complexity of nanoseconds. They map naturally to JavaScript Date.now() and
// most database timestamp columns.
//
// # Zero Value Semantics
//
// A timestamp of 0 is treated as "unset":
//
//	timestamp.Format(0)      // Returns ""
//	timestamp.FromUnixMs(0)  // Returns time.Time{} (zero time)
//	timestamp.Since(0)       // Returns 0
//	timestamp.Min(0, ts)     // Returns ts (0 treated as "later")
//
// This allows timestamp fields to be optional without using pointers.
//
// # Usage
//
// Getting current time:
//
//	now := timestamp.Now()  // Returns current Unix milliseconds
//
// Converting between types:
//
//	// time.Time → int64
//	ts := timestamp.ToUnixMs(time.Now())
//
//	// int64 → time.Time
//	t := timestamp.FromUnixMs(ts)
//	t := timestamp.ToTime(ts)  // Alias for readability
//
// Formatting for display:
//
//	s := timestamp.Format(ts)  // Returns "2024-01-15T10:30:00Z"
//
// Parsing various inputs:
//
//	// RFC3339 string
//	ts := timestamp.Parse("2024-01-15T10:30:00Z")
//
//	// Unix seconds (auto-detected)
//	ts := timestamp.Parse(1705315800)
//
//	// Unix milliseconds (auto-detected)
//	ts := timestamp.Parse(1705315800000)
//
//	// time.Time
//	ts := timestamp.Parse(time.Now())
//
// Time arithmetic:
//
//	// Add duration
//	future := timestamp.Add(ts, 1*time.Hour)
//
//	// Subtract duration
//	past := timestamp.Sub(ts, 30*time.Minute)
//
//	// Duration between timestamps
//	elapsed := timestamp.Between(start, end)
//
//	// Duration since timestamp
//	age := timestamp.Since(ts)
//
// Comparison:
//
//	earlier := timestamp.Min(ts1, ts2)
//	later := timestamp.Max(ts1, ts2)
//	timestamp.IsZero(ts)  // true if 0
//
// Validation:
//
//	if err := timestamp.Validate(ts); err != nil {
//	    // Invalid: negative or unreasonably far in future
//	}
//
// # Migration Guide
//
// Replace common patterns:
//
//	// Old: time.Now().Unix()
//	// New:
//	now := timestamp.Now()
//
//	// Old: time.Unix(sec, 0)
//	// New:
//	t := timestamp.FromUnixMs(sec * 1000)
//
//	// Old: time.Parse(time.RFC3339, s)
//	// New:
//	ts := timestamp.Parse(s)
//
//	// Old: t.Format(time.RFC3339)
//	// New:
//	s := timestamp.Format(ts)
//
// # Auto-Detection Logic
//
// Parse() automatically detects seconds vs milliseconds:
//   - Values > 1e12 (year ~2001 in ms) are treated as milliseconds
//   - Values ≤ 1e12 are treated as seconds and converted
//
// This handles both JavaScript (milliseconds) and Unix (seconds) conventions.
//
// # Thread Safety
//
// All functions are pure (no shared state) and safe for concurrent use.
//
// # See Also
//
// Related packages:
//   - [time]: Go standard library time package
//   - [github.com/c360studio/semengine/graph]: Uses int64 timestamps for EntityState
package timestamp
