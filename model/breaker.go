package model

import (
	"sync"
	"time"
)

// BreakerConfig tunes the rolling-window circuit breaker. Zero-value
// defaults are sensible for typical LLM endpoint workloads (tens of
// requests per minute, latency dominated by upstream).
type BreakerConfig struct {
	// WindowSize is the number of recent results to keep per endpoint.
	// Older results slide out as new ones arrive. Must be >= 1.
	// Default: 20.
	WindowSize int

	// MinRequests is the minimum number of observations before the
	// error rate is consulted. Below this, the endpoint stays Closed
	// regardless of failure ratio. Prevents one-or-two-error flukes
	// on cold endpoints from opening the breaker. Must be <= WindowSize.
	// Default: 5.
	MinRequests int

	// ErrorRateThreshold is the failure ratio above which the breaker
	// opens (assuming MinRequests has been met). 0.5 means "open if
	// more than half the recent window failed." Must be in (0, 1].
	// Default: 0.5.
	ErrorRateThreshold float64

	// Cooldown is the wait between Open and HalfOpen — how long the
	// endpoint stays out of rotation before we send a probe. Must be > 0.
	// Default: 30s.
	Cooldown time.Duration

	// Now is the time source. Tests inject a clock; production leaves
	// this nil and time.Now is used.
	Now func() time.Time
}

func (c *BreakerConfig) applyDefaults() {
	if c.WindowSize <= 0 {
		c.WindowSize = 20
	}
	if c.MinRequests <= 0 {
		c.MinRequests = 5
	}
	if c.MinRequests > c.WindowSize {
		c.MinRequests = c.WindowSize
	}
	if c.ErrorRateThreshold <= 0 || c.ErrorRateThreshold > 1 {
		c.ErrorRateThreshold = 0.5
	}
	if c.Cooldown <= 0 {
		c.Cooldown = 30 * time.Second
	}
	if c.Now == nil {
		c.Now = time.Now
	}
}

// RollingWindowBreaker is the default in-process HealthPolicy. It
// keeps a sliding window of the most recent results per endpoint and
// transitions the breaker based on the failure ratio in that window.
//
// State machine:
//
//	Closed   --error rate > threshold (and >= MinRequests)--> Open
//	Open     --cooldown elapsed---------------------------->  HalfOpen
//	HalfOpen --probe succeeds----------------------------->   Closed
//	HalfOpen --probe fails-------------------------------->   Open
//	         (cooldown timer restarts on each Open transition)
//
// HalfOpen lets exactly one probe through: the first IsHealthy caller
// past the cooldown takes the probe slot, and every other IsHealthy
// call returns false until RecordResult reports, preventing
// thundering-herd reopening of a sick endpoint. A probe that has not
// reported within one Cooldown of being admitted counts as lost, and
// the next IsHealthy caller takes the slot, so a dropped request
// cannot hold the endpoint half-open for ever (#114). EndpointStatus
// and EndpointStats reserve nothing.
//
// Safe for concurrent use across many goroutines targeting the same
// or different endpoints. Per-endpoint state is locked independently.
type RollingWindowBreaker struct {
	cfg BreakerConfig

	mu        sync.Mutex
	endpoints map[string]*endpointState
}

type endpointState struct {
	// window stores the most recent WindowSize results, oldest first.
	window []bool
	// successes / failures are running counts of the bools in window;
	// kept incrementally so EndpointStats is O(1).
	successes int
	failures  int

	status         EndpointStatus
	openedAt       time.Time // when status last became Open
	lastFailure    time.Time
	lastTransition time.Time

	// probeInFlight is true while a HalfOpen probe is outstanding,
	// admitted by IsHealthy at probeAdmittedAt. IsHealthy returns false
	// while it is set and younger than one Cooldown, so we don't admit
	// a thundering herd into the half-open state.
	probeInFlight   bool
	probeAdmittedAt time.Time
}

// NewRollingWindowBreaker builds a breaker with the given config.
// Zero-valued config fields fall back to sensible defaults
// (WindowSize=20, MinRequests=5, ErrorRateThreshold=0.5, Cooldown=30s).
func NewRollingWindowBreaker(cfg BreakerConfig) *RollingWindowBreaker {
	cfg.applyDefaults()
	return &RollingWindowBreaker{
		cfg:       cfg,
		endpoints: make(map[string]*endpointState),
	}
}

// IsHealthy implements HealthPolicy. Past the cooldown it admits one
// probe: the caller that gets true while the endpoint is half-open
// holds the probe slot until RecordResult reports or one Cooldown
// passes.
func (b *RollingWindowBreaker) IsHealthy(endpoint string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	st := b.endpoints[endpoint]
	if st == nil {
		return true
	}
	now := b.cfg.Now()
	b.halfOpenIfCooled(st, now)

	switch st.status {
	case StatusClosed:
		return true
	case StatusHalfOpen:
		// A probe older than one Cooldown counts as lost (#114).
		if st.probeInFlight && now.Sub(st.probeAdmittedAt) < b.cfg.Cooldown {
			return false
		}
		st.probeInFlight = true
		st.probeAdmittedAt = now
		return true
	default:
		return false
	}
}

// EndpointStatus implements HealthPolicy. Drives the cooldown→half-open
// transition lazily on read so we don't need a background goroutine;
// it reserves no probe slot. Returns StatusClosed for endpoints with
// no observations yet.
func (b *RollingWindowBreaker) EndpointStatus(endpoint string) EndpointStatus {
	b.mu.Lock()
	defer b.mu.Unlock()

	st := b.endpoints[endpoint]
	if st == nil {
		return StatusClosed
	}
	b.halfOpenIfCooled(st, b.cfg.Now())
	return st.status
}

// halfOpenIfCooled moves an Open endpoint to HalfOpen once the cooldown
// since it opened has elapsed. The caller holds b.mu.
func (b *RollingWindowBreaker) halfOpenIfCooled(st *endpointState, now time.Time) {
	if st.status == StatusOpen && now.Sub(st.openedAt) >= b.cfg.Cooldown {
		st.status = StatusHalfOpen
		st.lastTransition = now
	}
}

// EndpointStats implements HealthPolicy.
func (b *RollingWindowBreaker) EndpointStats(endpoint string) HealthStats {
	b.mu.Lock()
	defer b.mu.Unlock()

	st := b.endpoints[endpoint]
	if st == nil {
		return HealthStats{Status: StatusClosed}
	}
	total := st.successes + st.failures
	rate := 0.0
	if total > 0 {
		rate = float64(st.failures) / float64(total)
	}
	return HealthStats{
		Status:         st.status,
		Successes:      st.successes,
		Failures:       st.failures,
		ErrorRate:      rate,
		LastFailure:    st.lastFailure,
		LastTransition: st.lastTransition,
	}
}

// RecordResult implements HealthPolicy. Updates the rolling window,
// drives state transitions, and returns. Safe to call concurrently.
func (b *RollingWindowBreaker) RecordResult(endpoint string, result Result) {
	b.mu.Lock()
	defer b.mu.Unlock()

	st, ok := b.endpoints[endpoint]
	if !ok {
		st = &endpointState{
			window:         make([]bool, 0, b.cfg.WindowSize),
			lastTransition: b.cfg.Now(),
		}
		b.endpoints[endpoint] = st
	}

	// Slide the oldest result out if the window is full, fixing up the
	// running counts.
	if len(st.window) == b.cfg.WindowSize {
		oldest := st.window[0]
		st.window = st.window[1:]
		if oldest {
			st.successes--
		} else {
			st.failures--
		}
	}
	st.window = append(st.window, result.Success)
	if result.Success {
		st.successes++
	} else {
		st.failures++
		st.lastFailure = b.cfg.Now()
	}

	// HalfOpen probe outcome closes or re-opens the breaker.
	if st.status == StatusHalfOpen {
		st.probeInFlight = false
		if result.Success {
			st.status = StatusClosed
			st.lastTransition = b.cfg.Now()
		} else {
			st.status = StatusOpen
			st.openedAt = b.cfg.Now()
			st.lastTransition = b.cfg.Now()
		}
		return
	}

	// Closed: open if the window is mature and the failure ratio is
	// above threshold.
	if st.status == StatusClosed {
		total := st.successes + st.failures
		if total >= b.cfg.MinRequests {
			rate := float64(st.failures) / float64(total)
			if rate > b.cfg.ErrorRateThreshold {
				st.status = StatusOpen
				st.openedAt = b.cfg.Now()
				st.lastTransition = b.cfg.Now()
			}
		}
		return
	}

	// Open: a stray RecordResult while open shouldn't change state —
	// the cooldown drives the transition. Only IsHealthy and
	// EndpointStatus observe the elapsed cooldown.
}
