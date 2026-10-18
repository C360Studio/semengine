package graphingest

import (
	"testing"
	"time"
)

// TestBootstrapScope_ZeroScopeIsAuthoritativelyNothingToDo covers the distinction the
// field exists for (SemStreams gh#732): a producer whose bind reads found no backlog reports scope 0,
// which a consumer reads, with bootstrap_complete, as "there was nothing to do" rather than
// "I might be early". A later backlog does not make it the initial build.
func TestBootstrapScope_ZeroScopeIsAuthoritativelyNothingToDo(t *testing.T) {
	c := createTestComponent(t)

	if scope := c.bootstrapScope(0, false); scope != 0 {
		t.Errorf("scope = %d, want 0", scope)
	}
	if scope := c.bootstrapScope(120, false); scope != 0 {
		t.Errorf("scope = %d, want 0 — work arriving after a clean bind capture is not the initial build", scope)
	}
}

// TestBootstrapScope_ObservationFailureCapturesNothing is the fail-closed guard. A zero
// read off a FAILED observation is the absence of a measurement, not a measurement of
// absence; it must not replace a partial bind capture.
func TestBootstrapScope_ObservationFailureCapturesNothing(t *testing.T) {
	c := createTestComponent(t)
	c.bootBacklogPartial.Store(true)

	if scope := c.bootstrapScope(0, true); scope != 0 {
		t.Errorf("scope = %d, want 0 — nothing was measured", scope)
	}

	// The failed read must not have consumed the once-only replacement: the first
	// SUCCESSFUL read is what defines the initial build.
	if scope := c.bootstrapScope(700, false); scope != 700 {
		t.Errorf("scope = %d, want 700 — the failed read must not have captured scope", scope)
	}
}

// TestBoundConsumers_DedupedAndDeterministic keeps the backlog sum from
// double-counting a consumer across a rebind, and keeps a degraded verdict naming the
// same consumer run to run.
func TestBoundConsumers_DedupedAndDeterministic(t *testing.T) {
	c := createTestComponent(t)
	c.registerBoundConsumer("SENSOR", "graph-ingest-sensor-all")
	c.registerBoundConsumer("EVENTS", "graph-ingest-events-all")
	c.registerBoundConsumer("SENSOR", "graph-ingest-sensor-all") // rebind

	got := c.boundConsumerSnapshot()
	if len(got) != 2 {
		t.Fatalf("got %d consumers, want 2 (a rebind must not double-count)", len(got))
	}
	if got[0].stream != "EVENTS" || got[1].stream != "SENSOR" {
		t.Errorf("snapshot not in deterministic order: %+v", got)
	}
}

// TestOldestOutstandingAt_PresenceEncoding pins the "0 means no information" contract
// at the producer. A caught-up producer reports no staleness however old its last
// applied message was, and a producer that has applied nothing yet reports absent
// rather than fabricating a fresh view.
func TestOldestOutstandingAt_PresenceEncoding(t *testing.T) {
	applied := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

	t.Run("caught up reports no staleness", func(t *testing.T) {
		c := createTestComponent(t)
		c.recordApplied(applied)
		if got := c.oldestOutstandingAt(0); !got.IsZero() {
			t.Errorf("got %v, want zero — caught up has no staleness to report", got)
		}
	})

	t.Run("backlog ages from the last applied message", func(t *testing.T) {
		c := createTestComponent(t)
		c.recordApplied(applied)
		if got := c.oldestOutstandingAt(5); !got.Equal(applied) {
			t.Errorf("got %v, want %v", got, applied)
		}
	})

	t.Run("nothing applied yet reports absent, not fresh", func(t *testing.T) {
		c := createTestComponent(t)
		if got := c.oldestOutstandingAt(5); !got.IsZero() {
			t.Errorf("got %v, want zero — an unknown age must not read as 0ms fresh", got)
		}
	})

	t.Run("a zero delivery timestamp is not recorded", func(t *testing.T) {
		c := createTestComponent(t)
		c.recordApplied(time.Time{})
		if got := c.oldestOutstandingAt(5); !got.IsZero() {
			t.Errorf("got %v, want zero", got)
		}
	})
}

// The pin's TestLastSyncedRFC3339 is not ported: lastSyncedRFC3339 and the
// envelope's LastSynced go (design D16).

// TestBootstrapScope_PartialBindCaptureIsDiscarded is the Codex-found regression.
//
// The bug: one global bootBacklogKnown flag. With two bound consumers, a SUCCESSFUL
// bind read for A marked the aggregate captured, so a FAILED read for B did not
// "leave the capture unclaimed" — the tick fallback could never fire, and B's initial
// backlog was omitted forever. If A measured zero and B held work, the producer would
// publish `bootstrap_complete && bootstrap_scope == 0`: the contract's "authoritatively
// nothing to replay", asserted over a real initial build.
func TestBootstrapScope_PartialBindCaptureIsDiscarded(t *testing.T) {
	c := createTestComponent(t)

	// Consumer A binds and reads 0. Consumer B's read FAILS.
	c.bootBacklog.Add(0)
	c.bootBacklogPartial.Store(true)

	// First whole observation sees both consumers' real work.
	scope := c.bootstrapScope(700, false)
	if scope != 700 {
		t.Errorf("scope = %d, want 700 — a partial bind capture must be replaced by "+
			"the first whole observation, not kept", scope)
	}

	// And it is still captured once: a later burst must not move it.
	if scope = c.bootstrapScope(9000, false); scope != 700 {
		t.Errorf("scope = %d, want 700 (captured once)", scope)
	}
}

// TestBootstrapScope_CompleteBindCaptureIsKept is the other half: when every bind read
// succeeded, the accumulated total is authoritative and the tick must NOT overwrite it
// with a later, smaller observation (the backlog is draining by then).
func TestBootstrapScope_CompleteBindCaptureIsKept(t *testing.T) {
	c := createTestComponent(t)

	// Two consumers bound cleanly, 900 total at bind.
	c.bootBacklog.Add(400)
	c.bootBacklog.Add(500)

	// By the first tick, 850 have drained.
	scope := c.bootstrapScope(50, false)
	if scope != 900 {
		t.Errorf("scope = %d, want 900 — a clean bind capture is the initial build; "+
			"the tick must not replace it with a drained observation", scope)
	}
}
