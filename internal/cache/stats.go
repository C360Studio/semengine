package cache

import (
	"sync/atomic"
	"time"
)

// Statistics tracks cache performance metrics using lock-free atomic counters.
type Statistics struct {
	hits        atomic.Int64
	misses      atomic.Int64
	sets        atomic.Int64
	deletes     atomic.Int64
	evictions   atomic.Int64
	currentSize atomic.Int64
	maxSize     atomic.Int64
	startTime   atomic.Pointer[time.Time]
}

// NewStatistics creates a new statistics tracker.
func NewStatistics() *Statistics {
	s := &Statistics{}
	now := time.Now()
	s.startTime.Store(&now)
	return s
}

// Hit records a cache hit.
func (s *Statistics) Hit() {
	s.hits.Add(1)
}

// Miss records a cache miss.
func (s *Statistics) Miss() {
	s.misses.Add(1)
}

// Set records a cache set operation.
func (s *Statistics) Set() {
	s.sets.Add(1)
}

// Delete records a cache delete operation.
func (s *Statistics) Delete() {
	s.deletes.Add(1)
}

// Eviction records a cache eviction.
func (s *Statistics) Eviction() {
	s.evictions.Add(1)
}

// UpdateSize updates the current cache size and advances MaxSize when size exceeds it.
func (s *Statistics) UpdateSize(size int64) {
	s.currentSize.Store(size)
	for {
		prev := s.maxSize.Load()
		if size <= prev || s.maxSize.CompareAndSwap(prev, size) {
			return
		}
	}
}

// Hits returns the total number of cache hits.
func (s *Statistics) Hits() int64 {
	return s.hits.Load()
}

// Misses returns the total number of cache misses.
func (s *Statistics) Misses() int64 {
	return s.misses.Load()
}

// Sets returns the total number of set operations.
func (s *Statistics) Sets() int64 {
	return s.sets.Load()
}

// Deletes returns the total number of delete operations.
func (s *Statistics) Deletes() int64 {
	return s.deletes.Load()
}

// Evictions returns the total number of evictions.
func (s *Statistics) Evictions() int64 {
	return s.evictions.Load()
}

// CurrentSize returns the current number of entries in the cache.
func (s *Statistics) CurrentSize() int64 {
	return s.currentSize.Load()
}

// MaxSize returns the maximum number of entries the cache has held.
func (s *Statistics) MaxSize() int64 {
	return s.maxSize.Load()
}

// HitRatio returns the cache hit ratio as a percentage (0.0 to 1.0).
func (s *Statistics) HitRatio() float64 {
	hits := s.Hits()
	misses := s.Misses()
	total := hits + misses

	if total == 0 {
		return 0.0
	}

	return float64(hits) / float64(total)
}

// MissRatio returns the cache miss ratio as a percentage (0.0 to 1.0).
func (s *Statistics) MissRatio() float64 {
	return 1.0 - s.HitRatio()
}

// RequestsPerSecond returns the average number of requests (hits + misses) per second.
func (s *Statistics) RequestsPerSecond() float64 {
	elapsed := s.Uptime()
	if elapsed <= 0 {
		return 0.0
	}

	totalRequests := s.Hits() + s.Misses()
	return float64(totalRequests) / elapsed.Seconds()
}

// Uptime returns how long the cache has been running using the stored start time's monotonic clock reading.
func (s *Statistics) Uptime() time.Duration {
	start := s.startTime.Load()
	if start == nil {
		return 0
	}
	return time.Since(*start)
}

// Reset resets all statistics to zero. Callers must ensure cache mutations (UpdateSize) are quiescent
// when calling Reset; Reset does not provide an atomic multi-field reset across concurrent size updates.
func (s *Statistics) Reset() {
	s.hits.Store(0)
	s.misses.Store(0)
	s.sets.Store(0)
	s.deletes.Store(0)
	s.evictions.Store(0)
	now := time.Now()
	s.startTime.Store(&now)
	s.currentSize.Store(0)
	s.maxSize.Store(0)
}

// StatsSummary returns a snapshot of all statistics.
type StatsSummary struct {
	Hits              int64         `json:"hits"`
	Misses            int64         `json:"misses"`
	Sets              int64         `json:"sets"`
	Deletes           int64         `json:"deletes"`
	Evictions         int64         `json:"evictions"`
	CurrentSize       int64         `json:"current_size"`
	MaxSize           int64         `json:"max_size"`
	HitRatio          float64       `json:"hit_ratio"`
	MissRatio         float64       `json:"miss_ratio"`
	RequestsPerSecond float64       `json:"requests_per_second"`
	Uptime            time.Duration `json:"uptime"`
}

// Summary returns a snapshot of all statistics.
func (s *Statistics) Summary() StatsSummary {
	return StatsSummary{
		Hits:              s.Hits(),
		Misses:            s.Misses(),
		Sets:              s.Sets(),
		Deletes:           s.Deletes(),
		Evictions:         s.Evictions(),
		CurrentSize:       s.CurrentSize(),
		MaxSize:           s.MaxSize(),
		HitRatio:          s.HitRatio(),
		MissRatio:         s.MissRatio(),
		RequestsPerSecond: s.RequestsPerSecond(),
		Uptime:            s.Uptime(),
	}
}
