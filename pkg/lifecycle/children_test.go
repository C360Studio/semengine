package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/nats-io/nats.go"
)

const (
	childrenParentID      = "c360.platform1.gcs.lifecycle.mission.children-parent"
	childrenSubtaskPrefix = "c360.platform1.gcs.lifecycle.subtask."
)

// countingExactReader counts the exact reads the manager makes and fails the
// read of one entity with failErr, so a test can bound Children's reads and
// drive a child read failure.
type countingExactReader struct {
	inner   exactEntityReader
	mu      sync.Mutex
	reads   int
	failID  string
	failErr error
}

func (r *countingExactReader) ReadExactEntity(ctx context.Context, entityID string) (*graph.ExactEntity, error) {
	r.mu.Lock()
	r.reads++
	failID, failErr := r.failID, r.failErr
	r.mu.Unlock()
	if entityID == failID {
		return nil, failErr
	}
	return r.inner.ReadExactEntity(ctx, entityID)
}

func (r *countingExactReader) take() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := r.reads
	r.reads = 0
	return n
}

// newChildrenManager registers a parent workflow whose children are subtasks
// linked by mission.child.subtask (plus any extra child specs), creates the
// parent, and links it to every ID in links. Only the IDs in stored exist.
func newChildrenManager(t *testing.T, links, stored []string, extra ...ChildSpec) (*Manager, *countingExactReader, *fakeBucket) {
	t.Helper()
	bucket := newFakeBucket()
	mgr := newManagerForTest(slog.New(slog.DiscardHandler), &fakeEmitter{bucket: bucket}, bucket)
	ctx := t.Context()
	parent := lifecycle{}.fixtureWorkflow()
	parent.ChildWorkflows = append([]ChildSpec{{Workflow: "subtask", LinkPredicate: "mission.child.subtask"}}, extra...)
	if err := mgr.Register(parent); err != nil {
		t.Fatalf("Register parent: %v", err)
	}
	if err := mgr.Register(subtaskWorkflow()); err != nil {
		t.Fatalf("Register subtask: %v", err)
	}
	for _, id := range stored {
		if err := mgr.Create(ctx, &subtask{ID: id, PhaseF: "queued"}); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}
	if err := mgr.Create(ctx, &fixtureMission{ID: childrenParentID, PhaseF: "planning"}); err != nil {
		t.Fatalf("Create parent: %v", err)
	}
	// Links are stored in reverse, so the result order is Children's sort.
	for _, id := range slices.Backward(links) {
		linkChild(bucket, "mission.child.subtask", id)
	}
	reader := &countingExactReader{inner: mgr.exactReader}
	mgr.exactReader = reader
	return mgr, reader, bucket
}

// linkChild adds a stamped link from the parent to childID, as an upstream
// producer's write would: lifecycle itself writes no child link.
func linkChild(bucket *fakeBucket, predicate, childID string) {
	state := bucket.get(childrenParentID)
	state.Triples = append(state.Triples, message.Triple{
		Subject: childrenParentID, Predicate: predicate, Object: childID,
		Source: fixtureSource, Timestamp: fixtureTime,
	})
	bucket.put(childrenParentID, state)
}

// callChildren reports a panic as an error, so a negative Offset on the base
// commit fails one case instead of the whole run.
func callChildren(ctx context.Context, mgr *Manager, opts ChildOptions) (got []ChildResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Children panicked: %v", r)
		}
	}()
	return mgr.Children(ctx, childrenParentID, opts)
}

// TestChildrenPagesKeptChildrenWithinBoundedReads is design D6's exhaustive
// small-scope check. Links a..f with b and d absent leave kept = a, c, e, f.
// For every Offset and Limit in -1..7 the page equals an oracle written from
// List's slice rule, not from Children: start = max(Offset, 0) capped at
// len(kept); end = len(kept) when Limit <= 0, else min(start+Limit, len(kept));
// want kept[start:end], as a non-nil slice. The child reads made stay within
// ADR-049's bound: Offset + Limit + dropped when Limit > 0, every link when it
// is not.
func TestChildrenPagesKeptChildrenWithinBoundedReads(t *testing.T) {
	t.Parallel()
	var links, stored []string
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		links = append(links, childrenSubtaskPrefix+name)
		if name != "b" && name != "d" {
			stored = append(stored, childrenSubtaskPrefix+name)
		}
	}
	kept := stored
	const dropped = 2
	mgr, reader, _ := newChildrenManager(t, links, stored)
	ctx := t.Context()

	cases := 0
	for offset := -1; offset <= 7; offset++ {
		for limit := -1; limit <= 7; limit++ {
			start := min(max(offset, 0), len(kept))
			end := len(kept)
			if limit > 0 {
				end = min(start+limit, len(kept))
			}
			want := kept[start:end]

			reader.take()
			got, err := callChildren(ctx, mgr, ChildOptions{Offset: offset, Limit: limit})
			childReads := reader.take() - 1 // the parent's read
			if err != nil {
				t.Errorf("Offset=%d Limit=%d: %v", offset, limit, err)
				continue
			}
			cases++
			if got == nil {
				t.Errorf("Offset=%d Limit=%d: nil result, want a non-nil slice", offset, limit)
			}
			ids := make([]string, 0, len(got))
			for _, r := range got {
				ids = append(ids, r.State.EntityID())
			}
			if !slices.Equal(ids, want) {
				t.Errorf("Offset=%d Limit=%d: got %q, want %q", offset, limit, ids, want)
			}
			bound := len(links)
			if limit > 0 {
				bound = min(bound, max(offset, 0)+limit+dropped)
			}
			if childReads > bound || (limit <= 0 && childReads != len(links)) {
				t.Errorf("Offset=%d Limit=%d: %d child reads, want at most %d (all %d when Limit <= 0)",
					offset, limit, childReads, bound, len(links))
			}
		}
	}
	if cases != 81 {
		t.Errorf("checked %d of 81 cases", cases)
	}
}

// TestChildrenFailsOnAnUnreadableChild: a child read that fails for any reason
// other than absence or no lifecycle phase fails the call, as List does (owner
// question 3): a read timeout, and a link into a child workflow nobody
// registered.
func TestChildrenFailsOnAnUnreadableChild(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	t.Run("a child read times out", func(t *testing.T) {
		t.Parallel()
		ids := []string{childrenSubtaskPrefix + "a", childrenSubtaskPrefix + "b"}
		mgr, reader, _ := newChildrenManager(t, ids, ids)
		reader.mu.Lock()
		reader.failID, reader.failErr = childrenSubtaskPrefix+"b", nats.ErrTimeout
		reader.mu.Unlock()
		got, err := mgr.Children(ctx, childrenParentID, ChildOptions{})
		if !errors.Is(err, nats.ErrTimeout) {
			t.Fatalf("Children = %v, %v; want an error matching nats.ErrTimeout", got, err)
		}
		if got != nil {
			t.Errorf("Children returned %d results beside its error", len(got))
		}
	})

	t.Run("a link into an unregistered child workflow", func(t *testing.T) {
		t.Parallel()
		ids := []string{childrenSubtaskPrefix + "a"}
		mgr, _, bucket := newChildrenManager(t, ids, ids,
			ChildSpec{Workflow: "unregistered", LinkPredicate: "mission.child.orphan"})
		linkChild(bucket, "mission.child.orphan", childrenSubtaskPrefix+"orphan")
		got, err := mgr.Children(ctx, childrenParentID, ChildOptions{})
		if !errors.Is(err, ErrWorkflowNotRegistered) {
			t.Fatalf("Children = %v, %v; want an error matching ErrWorkflowNotRegistered", got, err)
		}
	})
}
