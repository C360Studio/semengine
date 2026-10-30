package graphview

import (
	"context"
	"fmt"
	"maps"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"pgregory.net/rapid"
)

// The generated history check of design D6 (task 2.3; spec "Snapshot and changes form one
// history", scenario "A generated history"; ADR-081 G1, G2, G4, G6). Rapid draws a replay prefix
// of writes and a live history of writes, attaches of both kinds, unsubscribes and ticks. The
// history runs against a view over the fake watch and against a reference model the test owns:
// the last operation per key, written from the generated operations alone and never read back
// from the view. A put makes the key hold its value at its revision; a poisoned write makes it
// poisoned at its revision; a skipped write, a delete and a purge make it absent. The model also
// keeps each key's last revision, whatever the operation.
//
// What is checked, and what makes each check run:
//
//   - snapshot: a SnapshotAndSubscribe snapshot equals the model at the attach (G1, the snapshot
//     half). Every live history opens with one such attach, so it runs at least once a history.
//   - batch: each batch holds each key at most once, sorted by revision ascending (G4).
//   - revision: each delta's revision is above the subscriber's high-water for its key, which
//     starts at the model's last revision for that key at the attach. A delta at or below it is
//     a change from before the attach (G1, the delta half) or a stale delivery (G2).
//   - state: at every tick, each attached subscriber's state (its snapshot, or the model at the
//     attach for a delta-only subscriber, plus every delta since) equals the model, revisions
//     included (G1, G6: a poison arrives as poison, never as a value). It runs for each
//     subscriber still attached at a tick, so a history whose generated steps unsubscribe both
//     opening subscribers before any tick makes none; the run's aggregate "state" count shows
//     it running.
//   - closed: after Unsubscribe, the subscription's Deltas closes with a nil Err.
//
// Each history runs in its own synctest bubble (Rapid calls t.Deadline, which a bubble refuses,
// so the bubble cannot hold rapid.Check), stops its view, and so leaves no goroutine behind. A
// write is an unbuffered send to Run then synctest.Wait, so the view has applied it before the
// next step. A tick is one tick period of the bubble's clock then synctest.Wait, so Run has fanned
// the window out and each delivery goroutine has sent its batch or blocked before the test reads
// Deltas. A failed step returns its message, with the step index, to the test goroutine, and
// Rapid shrinks the history and prints the seed; replay with -rapid.seed=<seed>.
//
// What this sequential model does not cover: concurrent attaches and writes (the coherence and
// racing-attach example tests), a stalled reader (TestSlowSubscriberBackpressure), a subscription
// context's end (TestSubscriberContextCancelDetaches), the stop and the lost watch.

// historyStep is one generated operation. sub picks a live subscriber for unsubscribe.
type historyStep struct {
	op  string
	key string
	sub int
}

func (s historyStep) String() string {
	switch s.op {
	case "unsubscribe":
		return fmt.Sprintf("unsubscribe #%d", s.sub)
	case "attach", "attachDeltas", "tick":
		return s.op
	default:
		return s.op + " " + s.key
	}
}

// modelValue is the model's state for a key that is not absent.
type modelValue struct {
	poisoned bool
	value    string
	rev      uint64
}

type historyCounts struct {
	histories, steps, snapshots, batches, deltas, states, unsubscribes int
}

func TestViewHistoryMatchesModel(t *testing.T) {
	// Three keys, so writes to one key meet within a window and across attaches.
	keys := []string{"k0", "k1", "k2"}
	writeOps := []string{"put", "put", "put", "poison", "skip", "delete", "purge"}
	write := rapid.Custom(func(rt *rapid.T) historyStep {
		return historyStep{
			op:  rapid.SampledFrom(writeOps).Draw(rt, "op"),
			key: rapid.SampledFrom(keys).Draw(rt, "key"),
		}
	})
	liveOps := []string{"write", "write", "write", "write", "attach", "attachDeltas", "unsubscribe", "tick", "tick"}
	live := rapid.Custom(func(rt *rapid.T) historyStep {
		switch op := rapid.SampledFrom(liveOps).Draw(rt, "kind"); op {
		case "write":
			return write.Draw(rt, "write")
		case "unsubscribe":
			return historyStep{op: op, sub: rapid.IntRange(0, 7).Draw(rt, "sub")}
		default:
			return historyStep{op: op}
		}
	})

	var c historyCounts
	rapid.Check(t, func(rt *rapid.T) {
		replay := rapid.SliceOfN(write, 0, 8).Draw(rt, "replay")
		history := []historyStep{{op: "attach"}, {op: "attachDeltas"}}
		history = append(history, rapid.SliceOfN(live, 8, 60).Draw(rt, "live")...)
		history = append(history, historyStep{op: "tick"})
		var failure string
		synctest.Test(t, func(bt *testing.T) { failure = runViewHistory(bt.Context(), replay, history, &c) })
		if failure != "" {
			rt.Fatal(failure)
		}
	})
	t.Logf("checks run: histories %d, steps %d, snapshot %d, batch %d, revision %d, state %d, closed %d",
		c.histories, c.steps, c.snapshots, c.batches, c.deltas, c.states, c.unsubscribes)
}

// historySub is one attached subscriber as the test sees it: its state built from its snapshot
// (or the model at the attach) and its deltas, and its per-key revision high-water.
type historySub struct {
	sub   *Subscription[string]
	name  string
	state map[string]modelValue
	high  map[string]uint64
}

// runViewHistory runs the replay writes, the end of the replay and history against a new view and
// the model, inside the caller's bubble, and returns the first disagreement, or "" when there is
// none.
func runViewHistory(ctx context.Context, replay, history []historyStep, c *historyCounts) string {
	c.histories++
	w := newFakeWatcher()
	view, err := New[string](&fakeSource{queue: []*fakeWatcher{w}}, decodeTest, WithTickInterval(testTick))
	if err != nil {
		return "New: " + err.Error()
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- view.Run(runCtx) }()
	defer func() {
		cancel()
		<-done // Run stops every subscription before it returns
	}()
	synctest.Wait() // Run has opened the watch

	model := map[string]modelValue{}
	lastRev := map[string]uint64{}
	var rev uint64
	apply := func(s historyStep) {
		rev++
		lastRev[s.key] = rev
		e := fakeEntry{key: s.key, rev: rev, op: jetstream.KeyValuePut}
		switch s.op {
		case "put":
			e.value = []byte(fmt.Sprintf("v%d", rev))
			model[s.key] = modelValue{value: string(e.value), rev: rev}
		case "poison":
			e.value = []byte("POISON")
			model[s.key] = modelValue{poisoned: true, rev: rev}
		case "skip":
			e.value = []byte("SKIP")
			delete(model, s.key)
		case "delete":
			e.op = jetstream.KeyValueDelete
			delete(model, s.key)
		case "purge":
			e.op = jetstream.KeyValuePurge
			delete(model, s.key)
		}
		w.updates <- e
		synctest.Wait() // Run has applied the write
	}

	for _, s := range replay {
		apply(s)
	}
	w.updates <- nil // the end-of-replay marker
	if err := view.WaitCaughtUp(ctx); err != nil {
		return "WaitCaughtUp after the replay: " + err.Error()
	}

	var subs []*historySub
	attached := 0
	for i, s := range history {
		c.steps++
		at := fmt.Sprintf("step %d (%s)", i, s)
		switch s.op {
		case "attach":
			snap, sub, err := view.SnapshotAndSubscribe(ctx)
			if err != nil {
				return at + ": SnapshotAndSubscribe: " + err.Error()
			}
			got, msg := snapshotState(snap)
			if msg != "" {
				return at + ": " + msg
			}
			if !maps.Equal(got, model) {
				return fmt.Sprintf("%s: snapshot %v, model %v", at, got, model)
			}
			c.snapshots++
			subs = append(subs, &historySub{sub: sub, name: fmt.Sprintf("subscriber %d (snapshot)", attached),
				state: got, high: maps.Clone(lastRev)})
			attached++
		case "attachDeltas":
			sub, err := view.Subscribe(ctx)
			if err != nil {
				return at + ": Subscribe: " + err.Error()
			}
			subs = append(subs, &historySub{sub: sub, name: fmt.Sprintf("subscriber %d (deltas only)", attached),
				state: maps.Clone(model), high: maps.Clone(lastRev)})
			attached++
		case "unsubscribe":
			if len(subs) == 0 {
				continue
			}
			j := s.sub % len(subs)
			hs := subs[j]
			subs = append(subs[:j], subs[j+1:]...)
			if msg := hs.unsubscribe(c); msg != "" {
				return at + ": " + msg
			}
		case "tick":
			<-time.After(testTick)
			synctest.Wait() // Run has fanned the window out; each delivery goroutine has sent or blocked
			for _, hs := range subs {
				if msg := hs.afterTick(model, c); msg != "" {
					return at + ": " + msg
				}
			}
		default:
			apply(s)
		}
	}
	return ""
}

// snapshotState is a snapshot as the model spells it, or a message when it holds a key both as a
// value and as poisoned.
func snapshotState(snap Snapshot[string]) (map[string]modelValue, string) {
	got := map[string]modelValue{}
	for k, e := range snap.Entries {
		got[k] = modelValue{value: e.Value, rev: e.Revision}
	}
	for k, p := range snap.Poisoned {
		if _, both := got[k]; both {
			return nil, fmt.Sprintf("snapshot holds %s both as a value and as poisoned", k)
		}
		got[k] = modelValue{poisoned: true, rev: p.Revision}
	}
	return got, ""
}

// afterTick takes the batch the subscriber's delivery goroutine has ready, if any, and compares
// the subscriber's state with the model. The caller has waited for the tick's fan-out.
func (hs *historySub) afterTick(model map[string]modelValue, c *historyCounts) string {
	select {
	case batch, open := <-hs.sub.Deltas():
		if !open {
			return fmt.Sprintf("%s: Deltas closed (Err %v)", hs.name, hs.sub.Err())
		}
		if msg := hs.applyBatch(batch, c); msg != "" {
			return msg
		}
	default:
	}
	if !maps.Equal(hs.state, model) {
		return fmt.Sprintf("%s: state %v, model %v", hs.name, hs.state, model)
	}
	c.states++
	return ""
}

// unsubscribe detaches the subscriber and drains its Deltas until it closes, checking any batch
// taken before the detach, then checks Err.
func (hs *historySub) unsubscribe(c *historyCounts) string {
	hs.sub.Unsubscribe()
	for {
		synctest.Wait() // the delivery goroutine has sent or closed, or is blocked
		var batch []Delta[string]
		var open bool
		select {
		case batch, open = <-hs.sub.Deltas():
		default:
			return hs.name + ": Deltas not closed after Unsubscribe"
		}
		if !open {
			break
		}
		if msg := hs.applyBatch(batch, c); msg != "" {
			return msg
		}
	}
	if err := hs.sub.Err(); err != nil {
		return fmt.Sprintf("%s: Err after Unsubscribe = %v, want nil", hs.name, err)
	}
	c.unsubscribes++
	return ""
}

// applyBatch checks one batch and applies it to the subscriber's state, returning the first
// violation, or "" when there is none.
func (hs *historySub) applyBatch(batch []Delta[string], c *historyCounts) string {
	seen := map[string]bool{}
	for i, d := range batch {
		if seen[d.Key] {
			return fmt.Sprintf("%s: batch %v holds key %s twice", hs.name, batch, d.Key)
		}
		seen[d.Key] = true
		if i > 0 && d.Revision <= batch[i-1].Revision {
			return fmt.Sprintf("%s: batch %v is not sorted by revision", hs.name, batch)
		}
		if d.Revision <= hs.high[d.Key] {
			return fmt.Sprintf("%s: delta %s %s@%d is not above the key's high-water %d (a change from "+
				"before the attach, or a stale one)", hs.name, d.Op, d.Key, d.Revision, hs.high[d.Key])
		}
		hs.high[d.Key] = d.Revision
		switch d.Op {
		case DeltaUpsert:
			hs.state[d.Key] = modelValue{value: d.Value, rev: d.Revision}
		case DeltaPoison:
			hs.state[d.Key] = modelValue{poisoned: true, rev: d.Revision}
		case DeltaDelete:
			delete(hs.state, d.Key)
		default:
			return fmt.Sprintf("%s: delta %v has an unknown op", hs.name, d)
		}
		c.deltas++
	}
	c.batches++
	return ""
}
