package model

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock returns a controllable time source for deterministic
// transition tests. Advance via Advance(); read via Now.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock { return &fakeClock{now: start} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// fail / pass are convenience helpers for test readability.
func fail() Result { return Result{Success: false, Kind: ErrorKindServerError} }
func pass() Result { return Result{Success: true} }

// Untracked endpoints — those with zero observations — must report
// healthy. New endpoints in production start cold; we don't open them
// until they've actually misbehaved.
func TestBreaker_UnknownEndpointIsClosed(t *testing.T) {
	b := NewRollingWindowBreaker(BreakerConfig{})
	if !b.IsHealthy("never-seen") {
		t.Error("untracked endpoint should be healthy")
	}
	if b.EndpointStatus("never-seen") != StatusClosed {
		t.Error("untracked endpoint status should be Closed")
	}
	stats := b.EndpointStats("never-seen")
	if stats.Status != StatusClosed || stats.Successes != 0 || stats.Failures != 0 {
		t.Errorf("untracked stats = %+v, want zero/Closed", stats)
	}
}

// Below MinRequests, the breaker must NOT open even at 100% failure
// rate. Cold endpoints with one or two errors aren't grounds to
// blackhole the endpoint.
func TestBreaker_BelowMinRequestsStaysClosed(t *testing.T) {
	b := NewRollingWindowBreaker(BreakerConfig{
		WindowSize:         20,
		MinRequests:        5,
		ErrorRateThreshold: 0.5,
	})
	for i := 0; i < 4; i++ {
		b.RecordResult("ep", fail())
	}
	if !b.IsHealthy("ep") {
		t.Errorf("breaker opened at 4 failures (below MinRequests=5)")
	}
	if b.EndpointStatus("ep") != StatusClosed {
		t.Errorf("status = %s, want closed", b.EndpointStatus("ep"))
	}
}

// Once MinRequests is reached and the error rate exceeds the
// threshold, the breaker opens. The first failure that crosses the
// threshold flips the state.
func TestBreaker_OpensAtThreshold(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	b := NewRollingWindowBreaker(BreakerConfig{
		WindowSize:         10,
		MinRequests:        5,
		ErrorRateThreshold: 0.5,
		Cooldown:           30 * time.Second,
		Now:                clock.Now,
	})

	// 3 successes, 3 failures = 50% — at threshold, NOT above.
	for i := 0; i < 3; i++ {
		b.RecordResult("ep", pass())
	}
	for i := 0; i < 3; i++ {
		b.RecordResult("ep", fail())
	}
	if !b.IsHealthy("ep") {
		t.Errorf("breaker opened at exactly 50%% error rate; should require above")
	}

	// One more failure pushes us to 4/7 ≈ 57% — opens.
	b.RecordResult("ep", fail())
	if b.IsHealthy("ep") {
		t.Errorf("breaker still closed at 4/7 (~57%%); expected open")
	}
	if b.EndpointStatus("ep") != StatusOpen {
		t.Errorf("status = %s, want open", b.EndpointStatus("ep"))
	}
	stats := b.EndpointStats("ep")
	if stats.LastTransition.IsZero() {
		t.Error("LastTransition not updated on open")
	}
}

// Recovery path: Open → cooldown elapses → HalfOpen → probe succeeds → Closed.
func TestBreaker_RecoveryViaSuccessfulProbe(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	b := NewRollingWindowBreaker(BreakerConfig{
		WindowSize:         10,
		MinRequests:        5,
		ErrorRateThreshold: 0.5,
		Cooldown:           30 * time.Second,
		Now:                clock.Now,
	})

	// Drive to Open.
	for i := 0; i < 6; i++ {
		b.RecordResult("ep", fail())
	}
	if b.EndpointStatus("ep") != StatusOpen {
		t.Fatalf("setup: expected Open, got %s", b.EndpointStatus("ep"))
	}

	// Within cooldown — still Open.
	clock.Advance(15 * time.Second)
	if b.EndpointStatus("ep") != StatusOpen {
		t.Errorf("status before cooldown elapsed = %s, want Open", b.EndpointStatus("ep"))
	}

	// Past cooldown — first reader transitions to HalfOpen.
	clock.Advance(20 * time.Second)
	if b.EndpointStatus("ep") != StatusHalfOpen {
		t.Errorf("status past cooldown = %s, want HalfOpen", b.EndpointStatus("ep"))
	}

	// Probe succeeds → Closed.
	b.RecordResult("ep", pass())
	if b.EndpointStatus("ep") != StatusClosed {
		t.Errorf("status after successful probe = %s, want Closed", b.EndpointStatus("ep"))
	}
}

// Failed probe restarts the cooldown — endpoint goes back to Open.
func TestBreaker_FailedProbeReopens(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	b := NewRollingWindowBreaker(BreakerConfig{
		MinRequests:        3,
		ErrorRateThreshold: 0.5,
		Cooldown:           30 * time.Second,
		Now:                clock.Now,
	})

	for i := 0; i < 4; i++ {
		b.RecordResult("ep", fail())
	}
	if b.EndpointStatus("ep") != StatusOpen {
		t.Fatalf("setup: expected Open, got %s", b.EndpointStatus("ep"))
	}

	clock.Advance(31 * time.Second)
	if b.EndpointStatus("ep") != StatusHalfOpen {
		t.Fatalf("setup: expected HalfOpen, got %s", b.EndpointStatus("ep"))
	}

	// Probe fails — re-open with fresh cooldown.
	b.RecordResult("ep", fail())
	if b.EndpointStatus("ep") != StatusOpen {
		t.Errorf("status after failed probe = %s, want Open", b.EndpointStatus("ep"))
	}

	// Cooldown should be measured from the re-open, not the original.
	clock.Advance(15 * time.Second)
	if b.EndpointStatus("ep") != StatusOpen {
		t.Errorf("status 15s after re-open = %s, want Open (cooldown not yet elapsed)", b.EndpointStatus("ep"))
	}
	clock.Advance(20 * time.Second) // total 35s past re-open
	if b.EndpointStatus("ep") != StatusHalfOpen {
		t.Errorf("status 35s after re-open = %s, want HalfOpen", b.EndpointStatus("ep"))
	}
}

// openBreaker returns a breaker whose endpoint "ep" has just opened,
// with a 30s cooldown on a fake clock.
func openBreaker(t *testing.T) (*RollingWindowBreaker, *fakeClock) {
	t.Helper()
	clock := newFakeClock(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	b := NewRollingWindowBreaker(BreakerConfig{
		MinRequests:        3,
		ErrorRateThreshold: 0.5,
		Cooldown:           30 * time.Second,
		Now:                clock.Now,
	})
	for i := 0; i < 4; i++ {
		b.RecordResult("ep", fail())
	}
	if b.IsHealthy("ep") {
		t.Fatal("setup: expected the breaker to be open")
	}
	return b, clock
}

// Past the cooldown, half-open admits one probe; every other caller is
// refused while that probe has not reported (#114).
func TestBreaker_HalfOpenAdmitsOneProbe(t *testing.T) {
	b, clock := openBreaker(t)
	clock.Advance(30 * time.Second)

	if !b.IsHealthy("ep") {
		t.Fatal("first caller past the cooldown was refused; it should take the probe slot")
	}
	for i := 0; i < 3; i++ {
		if b.IsHealthy("ep") {
			t.Fatalf("caller %d admitted while the probe is outstanding", i+2)
		}
	}
	if got := b.EndpointStatus("ep"); got != StatusHalfOpen {
		t.Errorf("status while the probe is outstanding = %s, want half_open", got)
	}
}

// A failed probe reopens the breaker; once the new cooldown passes the
// slot is free again for one caller, and a passing probe closes the
// breaker so every caller is admitted.
func TestBreaker_ProbeSlotAfterFailedAndPassingProbe(t *testing.T) {
	b, clock := openBreaker(t)
	clock.Advance(30 * time.Second)
	if !b.IsHealthy("ep") {
		t.Fatal("setup: first probe refused")
	}

	b.RecordResult("ep", fail())
	if b.IsHealthy("ep") {
		t.Fatal("caller admitted right after a failed probe; the breaker should be open")
	}

	clock.Advance(30 * time.Second)
	if !b.IsHealthy("ep") {
		t.Fatal("caller refused after the failed probe's cooldown; the slot should be free")
	}
	if b.IsHealthy("ep") {
		t.Fatal("second caller admitted while the new probe is outstanding")
	}

	b.RecordResult("ep", pass())
	for i := 0; i < 3; i++ {
		if !b.IsHealthy("ep") {
			t.Fatalf("caller %d refused after a passing probe; the breaker should be closed", i+1)
		}
	}
	if got := b.EndpointStatus("ep"); got != StatusClosed {
		t.Errorf("status after a passing probe = %s, want closed", got)
	}
}

// A probe that has not reported within one Cooldown of being admitted
// counts as lost: the next caller takes the slot, and not before
// (owner ruling, #114 comment 6722829046).
func TestBreaker_LostProbeFreesSlotAfterOneCooldown(t *testing.T) {
	b, clock := openBreaker(t)
	clock.Advance(30 * time.Second)
	if !b.IsHealthy("ep") {
		t.Fatal("setup: first probe refused")
	}

	clock.Advance(30*time.Second - time.Nanosecond)
	if b.IsHealthy("ep") {
		t.Fatal("caller admitted before the outstanding probe was one Cooldown old")
	}

	clock.Advance(time.Nanosecond)
	if !b.IsHealthy("ep") {
		t.Fatal("caller refused one Cooldown after the probe was admitted; the probe should count as lost")
	}
	if b.IsHealthy("ep") {
		t.Fatal("second caller admitted while the replacement probe is outstanding")
	}
}

// Reading the status or the stats past the cooldown reserves nothing:
// the next IsHealthy caller still takes the probe slot, and only it.
func TestBreaker_StatusReadsReserveNothing(t *testing.T) {
	reads := map[string]func(b *RollingWindowBreaker){
		"EndpointStatus": func(b *RollingWindowBreaker) {
			if got := b.EndpointStatus("ep"); got != StatusHalfOpen {
				t.Errorf("status past the cooldown = %s, want half_open", got)
			}
		},
		"EndpointStats": func(b *RollingWindowBreaker) { _ = b.EndpointStats("ep") },
	}
	for name, read := range reads {
		t.Run(name, func(t *testing.T) {
			b, clock := openBreaker(t)
			clock.Advance(30 * time.Second)

			read(b)
			read(b)
			if !b.IsHealthy("ep") {
				t.Fatalf("IsHealthy refused after %s reads; a read must not take the probe slot", name)
			}
			if b.IsHealthy("ep") {
				t.Fatal("second caller admitted while the probe is outstanding")
			}
		})
	}
}

// Concurrent IsHealthy callers past the cooldown: exactly one is
// admitted. Run under -race.
func TestBreaker_ConcurrentProbeAdmitsExactlyOne(t *testing.T) {
	b, clock := openBreaker(t)
	clock.Advance(30 * time.Second)

	const callers = 32
	var admitted atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if b.IsHealthy("ep") {
				admitted.Add(1)
			}
		}()
	}
	close(start)
	// wg.Wait orders the read of admitted after every caller's write.
	wg.Wait()

	if got := admitted.Load(); got != 1 {
		t.Errorf("admitted %d of %d concurrent callers past the cooldown, want exactly 1", got, callers)
	}
}

// The window slides — old failures fall out, allowing recovery as
// fresh successes accumulate.
func TestBreaker_WindowSlides(t *testing.T) {
	b := NewRollingWindowBreaker(BreakerConfig{
		WindowSize:         5,
		MinRequests:        3,
		ErrorRateThreshold: 0.5,
	})

	// Fill window with failures — opens.
	for i := 0; i < 5; i++ {
		b.RecordResult("ep", fail())
	}
	if b.IsHealthy("ep") {
		t.Fatal("setup: expected open")
	}

	stats := b.EndpointStats("ep")
	if stats.Failures != 5 || stats.Successes != 0 {
		t.Errorf("setup stats = %+v, want 5/0", stats)
	}

	// 5 successes in a row should slide all failures out and bring
	// counts to 5/0. The breaker stays Open until the cooldown drives
	// it to HalfOpen, but EndpointStats reflects the fresh window.
	for i := 0; i < 5; i++ {
		b.RecordResult("ep", pass())
	}
	stats = b.EndpointStats("ep")
	if stats.Successes != 5 || stats.Failures != 0 {
		t.Errorf("after slide stats = %+v, want 5 successes 0 failures", stats)
	}
}

// Concurrent recorders + readers across many endpoints must not race.
// Validated under -race; the assertions are spot-checks that the run
// produced sensible counts.
func TestBreaker_ConcurrentSafe(t *testing.T) {
	b := NewRollingWindowBreaker(BreakerConfig{
		WindowSize:         100,
		MinRequests:        10,
		ErrorRateThreshold: 0.9,
	})

	const goroutines = 8
	const perGoroutine = 500

	var totalRecorded atomic.Int64
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			endpoint := "ep-shared"
			for i := 0; i < perGoroutine; i++ {
				if i%3 == 0 {
					b.RecordResult(endpoint, fail())
				} else {
					b.RecordResult(endpoint, pass())
				}
				_ = b.IsHealthy(endpoint)
				_ = b.EndpointStats(endpoint)
				totalRecorded.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := totalRecorded.Load(); got != goroutines*perGoroutine {
		t.Fatalf("recorded = %d, want %d", got, goroutines*perGoroutine)
	}
	stats := b.EndpointStats("ep-shared")
	if stats.Successes+stats.Failures != b.cfg.WindowSize {
		t.Errorf("window not full: stats = %+v, want sum=%d", stats, b.cfg.WindowSize)
	}
}

// AlwaysHealthyPolicy returns true for every endpoint and ignores
// RecordResult — the no-op fallback for tests / disabled deployments.
func TestAlwaysHealthyPolicy(t *testing.T) {
	p := NewAlwaysHealthyPolicy()
	if !p.IsHealthy("anything") {
		t.Error("IsHealthy should always return true")
	}
	if p.EndpointStatus("anything") != StatusClosed {
		t.Error("EndpointStatus should be Closed")
	}
	// Lots of failures should not open the breaker.
	for i := 0; i < 1000; i++ {
		p.RecordResult("anything", fail())
	}
	if !p.IsHealthy("anything") {
		t.Error("AlwaysHealthy should ignore failures")
	}
}

// Compile-time assertion: both built-in policies satisfy HealthPolicy.
var (
	_ HealthPolicy = (*RollingWindowBreaker)(nil)
	_ HealthPolicy = alwaysHealthyPolicy{}
)
