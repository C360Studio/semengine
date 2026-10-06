package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semengine/pkg/errs"
)

// Strategy defines the eviction strategy for the cache.
type Strategy string

const (
	// StrategySimple uses no eviction policy.
	StrategySimple Strategy = "simple"

	// StrategyLRU uses Least Recently Used eviction based on size.
	StrategyLRU Strategy = "lru"

	// StrategyTTL uses Time-To-Live eviction based on expiry.
	StrategyTTL Strategy = "ttl"

	// StrategyHybrid uses combined LRU and TTL eviction.
	StrategyHybrid Strategy = "hybrid"
)

// Config contains configuration for cache creation.
type Config struct {
	// Enabled determines if caching is enabled.
	Enabled bool `json:"enabled" schema:"editable,type:bool,description:Enable caching"`

	// Strategy determines the eviction strategy.
	Strategy Strategy `json:"strategy" schema:"editable,type:enum,description:Cache eviction strategy,enum:simple|lru|ttl|hybrid"`

	// MaxSize is the maximum number of entries (for LRU and Hybrid caches).
	MaxSize int `json:"max_size" schema:"editable,type:int,description:Maximum number of cache entries (for LRU and Hybrid),min:1"`

	// TTL is the time-to-live for entries (for TTL and Hybrid caches).
	TTL time.Duration `json:"ttl" schema:"editable,type:string,description:Time-to-live for entries (for TTL and Hybrid)"`

	// CleanupInterval is how often to run background cleanup (for TTL and Hybrid caches).
	CleanupInterval time.Duration `json:"cleanup_interval" schema:"editable,type:string,description:How often to run background cleanup (for TTL and Hybrid)"`
}

// DefaultConfig returns a default cache configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:         true,
		Strategy:        StrategyLRU,
		MaxSize:         1000,
		TTL:             5 * time.Minute,
		CleanupInterval: 1 * time.Minute,
	}
}

// Validate checks if the configuration is valid.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil // No validation needed if disabled
	}

	switch c.Strategy {
	case StrategySimple:
		// No additional validation needed
	case StrategyLRU:
		if c.MaxSize <= 0 {
			return errs.WrapInvalid(errs.ErrInvalidData, "cache", "Validate",
				fmt.Sprintf("max_size must be positive for LRU cache, got %d", c.MaxSize))
		}
	case StrategyTTL:
		if c.TTL <= 0 {
			return errs.WrapInvalid(errs.ErrInvalidData, "cache", "Validate",
				fmt.Sprintf("ttl must be positive for TTL cache, got %v", c.TTL))
		}
		if c.CleanupInterval <= 0 {
			return errs.WrapInvalid(errs.ErrInvalidData, "cache", "Validate",
				fmt.Sprintf("cleanup_interval must be positive for TTL cache, got %v", c.CleanupInterval))
		}
	case StrategyHybrid:
		if c.MaxSize <= 0 {
			return errs.WrapInvalid(errs.ErrInvalidData, "cache", "Validate",
				fmt.Sprintf("max_size must be positive for Hybrid cache, got %d", c.MaxSize))
		}
		if c.TTL <= 0 {
			return errs.WrapInvalid(errs.ErrInvalidData, "cache", "Validate",
				fmt.Sprintf("ttl must be positive for Hybrid cache, got %v", c.TTL))
		}
		if c.CleanupInterval <= 0 {
			return errs.WrapInvalid(errs.ErrInvalidData, "cache", "Validate",
				fmt.Sprintf("cleanup_interval must be positive for Hybrid cache, got %v", c.CleanupInterval))
		}
	default:
		return errs.WrapInvalid(errs.ErrInvalidData, "cache", "Validate",
			fmt.Sprintf("unknown cache strategy: %s", c.Strategy))
	}

	return nil
}

// NewFromConfig creates a cache based on the provided configuration.
// Returns a disabled cache (NoopCache) if config.Enabled is false.
// Additional functional options can be passed to configure metrics, callbacks, etc.
func NewFromConfig[V any](ctx context.Context, config Config, options ...Option[V]) (Cache[V], error) {
	if ctx == nil {
		return nil, errs.WrapInvalid(errors.New("nil context"), "cache", "NewFromConfig", "context is required")
	}
	if err := config.Validate(); err != nil {
		return nil, errs.WrapInvalid(err, "cache", "NewFromConfig", "config validation failed")
	}

	if !config.Enabled {
		return NewNoop[V](), nil
	}

	switch config.Strategy {
	case StrategySimple:
		return NewSimple[V](options...)

	case StrategyLRU:
		return NewLRU[V](config.MaxSize, options...)

	case StrategyTTL:
		return NewTTL[V](ctx, config.TTL, config.CleanupInterval, options...)

	case StrategyHybrid:
		return newHybrid[V](ctx, config.MaxSize, config.TTL, config.CleanupInterval, options...)

	default:
		msg := fmt.Sprintf("unsupported cache strategy: %s", config.Strategy)
		return nil, errs.WrapInvalid(errs.ErrInvalidData, "cache",
			"NewFromConfig", msg)
	}
}

// NewLRU creates a new LRU cache with the specified maximum size.
// Stats are always enabled for observability. Use WithMetrics() to also export as Prometheus metrics.
func NewLRU[V any](maxSize int, options ...Option[V]) (Cache[V], error) {
	if err := (Config{Enabled: true, Strategy: StrategyLRU, MaxSize: maxSize}).Validate(); err != nil {
		return nil, err
	}
	opts := applyOptions(options...)
	c, err := newLRUCache[V](maxSize, opts)
	if err != nil {
		// A nil *lruCache returned as Cache[V] would be a non-nil interface.
		return nil, err
	}
	return c, nil
}

// NewTTL creates a new TTL cache with the specified TTL and cleanup interval.
// Stats are always enabled for observability. Use WithMetrics() to also export as Prometheus metrics.
func NewTTL[V any](ctx context.Context, ttl, cleanupInterval time.Duration, options ...Option[V]) (Cache[V], error) {
	cfg := Config{Enabled: true, Strategy: StrategyTTL, TTL: ttl, CleanupInterval: cleanupInterval}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	opts := applyOptions(options...)
	c, err := newTTLCache[V](ctx, ttl, cleanupInterval, opts)
	if err != nil {
		// A nil *ttlCache returned as Cache[V] would be a non-nil interface.
		return nil, err
	}
	return c, nil
}

// newHybrid creates a new Hybrid cache combining LRU and TTL eviction.
// Stats are always enabled for observability. Use WithMetrics() to also export as Prometheus metrics.
// This is an internal helper - use NewFromConfig() with StrategyHybrid instead.
func newHybrid[V any](
	ctx context.Context, maxSize int, ttl, cleanupInterval time.Duration,
	options ...Option[V],
) (Cache[V], error) {
	cfg := Config{Enabled: true, Strategy: StrategyHybrid, MaxSize: maxSize, TTL: ttl, CleanupInterval: cleanupInterval}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	opts := applyOptions(options...)
	c, err := newHybridCache[V](ctx, maxSize, ttl, cleanupInterval, opts)
	if err != nil {
		// A nil *hybridCache returned as Cache[V] would be a non-nil interface.
		return nil, err
	}
	return c, nil
}

// NewSimple creates a new Simple cache with no eviction policy.
// Stats are always enabled for observability. Use WithMetrics() to also export as Prometheus metrics.
func NewSimple[V any](options ...Option[V]) (Cache[V], error) {
	opts := applyOptions(options...)
	c, err := newSimpleCache[V](opts)
	if err != nil {
		// A nil *simpleCache returned as Cache[V] would be a non-nil interface.
		return nil, err
	}
	return c, nil
}

// NewNoop creates a cache that does nothing (always returns cache misses).
// This is useful when caching is disabled via configuration.
func NewNoop[V any]() Cache[V] {
	return &noopCache[V]{}
}

// noopCache is a cache implementation that does nothing.
type noopCache[V any] struct{}

func (c *noopCache[V]) Get(_ string) (V, bool) {
	var zero V
	return zero, false
}

func (c *noopCache[V]) Set(_ string, _ V) (bool, error) {
	return false, nil
}

func (c *noopCache[V]) Delete(_ string) (bool, error) {
	return false, nil
}

func (c *noopCache[V]) Clear() error {
	return nil
}

func (c *noopCache[V]) Size() int {
	return 0
}

func (c *noopCache[V]) Keys() []string {
	return nil
}

func (c *noopCache[V]) Stats() *Statistics {
	return nil
}

func (c *noopCache[V]) Close() error {
	return nil
}

// UnmarshalJSON implements custom JSON unmarshaling for Config to support
// duration strings (e.g., "1h", "5m", "30s") in addition to nanosecond integers.
func (c *Config) UnmarshalJSON(data []byte) error {
	// Use an alias to avoid infinite recursion
	type Alias Config

	// Temporary struct that accepts durations as either int64 or string
	aux := &struct {
		TTL             json.RawMessage `json:"ttl,omitempty"`
		CleanupInterval json.RawMessage `json:"cleanup_interval,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(c),
	}

	// A key Config does not have is refused, never silently ignored (owner ruling, #9 comment
	// 5968830525).
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	// Decode into the struct itself, not the pointer to it: a JSON null then leaves aux as it is
	// instead of setting it to nil (at the pin, Unmarshal(&aux) on null panicked below).
	if err := dec.Decode(aux); err != nil {
		return err
	}

	// Parse TTL (supports both int64 nanoseconds and duration strings)
	if len(aux.TTL) > 0 {
		ttl, err := parseDurationField(aux.TTL, "ttl")
		if err != nil {
			return err
		}
		c.TTL = ttl
	}

	// Parse CleanupInterval
	if len(aux.CleanupInterval) > 0 {
		interval, err := parseDurationField(aux.CleanupInterval, "cleanup_interval")
		if err != nil {
			return err
		}
		c.CleanupInterval = interval
	}

	return nil
}

// parseDurationField parses a JSON duration field that can be either:
// - An integer (nanoseconds) for backward compatibility
// - A string (duration like "1h", "5m", "30s")
func parseDurationField(data json.RawMessage, fieldName string) (time.Duration, error) {
	// Try parsing as string first (most common case)
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		duration, err := time.ParseDuration(str)
		if err != nil {
			return 0, fmt.Errorf("invalid duration string for %s: %w", fieldName, err)
		}
		return duration, nil
	}

	// Fall back to integer (nanoseconds) for backward compatibility
	var nsec int64
	if err := json.Unmarshal(data, &nsec); err != nil {
		return 0, fmt.Errorf("field %s must be either a duration string (e.g., '1h') or integer nanoseconds", fieldName)
	}
	return time.Duration(nsec), nil
}
