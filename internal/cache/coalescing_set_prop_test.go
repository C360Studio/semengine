package cache

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"pgregory.net/rapid"
)

// The generated check for CoalescingSet (task 3.6): histories of Add, Remove, RemovePrefix,
// Drain, window ticks and Shutdown, compared after every step with a reference model the test
// owns. The model is written from the set's documented contract (coalescing_set.go doc comments
// and design D7), never from its code:
//
//   - the set holds each pending key once; Add reports whether the key was newly inserted, Remove
//     whether it was pending, RemovePrefix how many pending keys had the prefix, and PendingCount
//     how many keys are pending;
//   - Drain returns every pending key and leaves none, and the callback never sees them;
//   - each window that ends with keys pending delivers them to the callback as one batch, each key
//     once, and leaves none pending; a window that ends with none delivers nothing;
//   - after Shutdown returns nil, no batch is delivered, whatever is added; Add, Remove and Drain
//     still answer from the pending set; a later Shutdown returns nil.
//
// Rapid draws a history of steps; the history then runs inside its own synctest bubble (Rapid
// calls t.Deadline, which a bubble refuses, so the bubble cannot hold rapid.Check). The window is
// the bubble's fake clock: every step takes no time except tick, which moves the clock one window,
// so each tick step meets exactly one tick of the set. Each history builds its own set and shuts
// it down, so the bubble returning proves no history left a goroutine behind. A failed step is
// reported after the bubble returns, and Rapid shrinks the history.
//
// What the example tests still own: a callback that blocks or panics, Shutdown under a blocked
// callback and an ended context, concurrent callers (this model is sequential, docs/testing.md),
// the zero window, and the nil-context refusals.
type propStep struct {
	op  string
	arg string
}

func TestPropCoalescingSetHistory(t *testing.T) {
	// Keys share prefixes so RemovePrefix can match none, some or all of them: "e1\x00" must not
	// match "e10\x00a", and "e2" must not match "a\x00e2", which holds it past the start.
	keys := []string{"e1\x00a", "e1\x00b", "e10\x00a", "e2", "a\x00e2", ""}
	prefixes := []string{"e1\x00", "e1", "e2", "x", ""}
	ops := []string{"add", "add", "add", "remove", "removePrefix", "drain", "tick", "tick", "shutdown"}
	step := rapid.Custom(func(rt *rapid.T) propStep {
		op := rapid.SampledFrom(ops).Draw(rt, "op")
		switch op {
		case "add", "remove":
			return propStep{op, rapid.SampledFrom(keys).Draw(rt, "key")}
		case "removePrefix":
			return propStep{op, rapid.SampledFrom(prefixes).Draw(rt, "prefix")}
		default:
			return propStep{op: op}
		}
	})

	var c propCounts
	rapid.Check(t, func(rt *rapid.T) {
		history := rapid.SliceOfN(step, 1, 40).Draw(rt, "history")
		var failure string
		synctest.Test(t, func(*testing.T) { failure = runCoalescingHistory(history, &c) })
		if failure != "" {
			rt.Fatal(failure)
		}
	})
	t.Logf("assertions run: add %d, remove %d, removePrefix %d, drain %d, pending count %d, "+
		"batch delivered %d, empty window %d, window after shutdown %d, shutdown %d",
		c.adds, c.removes, c.prefixRemoves, c.drains, c.counts, c.batches, c.emptyTicks,
		c.afterShutdownTicks, c.shutdowns)
}

type propCounts struct {
	adds, removes, prefixRemoves, drains, counts, batches, emptyTicks, afterShutdownTicks, shutdowns int
}

// runCoalescingHistory runs history against a new set and the model, inside the caller's bubble,
// and returns the first disagreement, or "" when there is none.
func runCoalescingHistory(history []propStep, c *propCounts) string {
	const window = 50 * time.Millisecond
	var mu sync.Mutex
	var delivered [][]string
	set, err := NewCoalescingSet(context.Background(), window, func(batch []string) {
		mu.Lock()
		delivered = append(delivered, batch)
		mu.Unlock()
	})
	if err != nil {
		return "NewCoalescingSet: " + err.Error()
	}
	defer func() { _ = set.Shutdown(context.Background()) }()
	takeDelivered := func() [][]string {
		mu.Lock()
		defer mu.Unlock()
		out := delivered
		delivered = nil
		return out
	}

	pending := map[string]bool{} // the model
	shutDown := false
	sortedPending := func() []string {
		out := make([]string, 0, len(pending))
		for k := range pending {
			out = append(out, k)
		}
		slices.Sort(out)
		return out
	}

	for i, s := range history {
		switch s.op {
		case "add":
			want := !pending[s.arg]
			pending[s.arg] = true
			if got := set.Add(s.arg); got != want {
				return fmt.Sprintf("step %d: Add(%q) = %v, model says %v", i, s.arg, got, want)
			}
			c.adds++
		case "remove":
			want := pending[s.arg]
			delete(pending, s.arg)
			if got := set.Remove(s.arg); got != want {
				return fmt.Sprintf("step %d: Remove(%q) = %v, model says %v", i, s.arg, got, want)
			}
			c.removes++
		case "removePrefix":
			want := 0
			for k := range pending {
				if strings.HasPrefix(k, s.arg) {
					delete(pending, k)
					want++
				}
			}
			if got := set.RemovePrefix(s.arg); got != want {
				return fmt.Sprintf("step %d: RemovePrefix(%q) = %d, model says %d", i, s.arg, got, want)
			}
			c.prefixRemoves++
		case "drain":
			want := sortedPending()
			clear(pending)
			got := set.Drain()
			slices.Sort(got)
			if !slices.Equal(got, want) {
				return fmt.Sprintf("step %d: Drain() = %q, model says %q", i, got, want)
			}
			c.drains++
		case "tick":
			<-time.After(window)
			synctest.Wait()
			got := takeDelivered()
			switch {
			case shutDown:
				if len(got) != 0 {
					return fmt.Sprintf("step %d: after Shutdown a window delivered %q", i, got)
				}
				c.afterShutdownTicks++
			case len(pending) == 0:
				if len(got) != 0 {
					return fmt.Sprintf("step %d: a window with nothing pending delivered %q", i, got)
				}
				c.emptyTicks++
			default:
				want := sortedPending()
				clear(pending)
				if len(got) != 1 {
					return fmt.Sprintf("step %d: a window with %q pending delivered %d batches: %q", i, want, len(got), got)
				}
				batch := slices.Clone(got[0])
				slices.Sort(batch)
				if !slices.Equal(batch, want) {
					return fmt.Sprintf("step %d: batch = %q, model says %q", i, batch, want)
				}
				c.batches++
			}
		case "shutdown":
			if err := set.Shutdown(context.Background()); err != nil {
				return fmt.Sprintf("step %d: Shutdown = %v, want nil (the callback never blocks)", i, err)
			}
			shutDown = true
			c.shutdowns++
		}
		if got, want := set.PendingCount(), len(pending); got != want {
			return fmt.Sprintf("step %d (%s): PendingCount() = %d, model says %d", i, s.op, got, want)
		}
		c.counts++
	}
	return ""
}

// TestCoalescingSetRemovePrefixMatchesOnlyAtTheStart owns the boundary the generated check reaches
// only by chance: a key that holds the prefix past its start is not removed.
func TestCoalescingSetRemovePrefixMatchesOnlyAtTheStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		set := newTestCoalescingSet(context.Background(), t, time.Hour, func([]string) {})
		set.Add("e2\x00a")
		set.Add("a\x00e2")
		if got := set.RemovePrefix("e2"); got != 1 {
			t.Errorf("RemovePrefix(%q) = %d, want 1", "e2", got)
		}
		if got := set.Drain(); !slices.Equal(got, []string{"a\x00e2"}) {
			t.Errorf("Drain() = %q, want the key holding the prefix past its start", got)
		}
		if err := set.Shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
	})
}
