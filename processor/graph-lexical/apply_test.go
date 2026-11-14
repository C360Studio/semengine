package graphlexical

import (
	"cmp"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/entitytext"
	"github.com/c360studio/semengine/graph/follower"
	"github.com/c360studio/semengine/graph/lexical"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/metric"
)

// failer is what the shared helpers need from *testing.T and *rapid.T.
type failer interface {
	Helper()
	Fatalf(format string, args ...any)
}

// text is what one delivered entity says: its name, the inline text that is not its name, and
// whether its body is offloaded to a store. An offloaded body is under instance, "objectstore"
// when empty, at bodyKey, the entity's key and "/body" when empty.
type text struct {
	name, inline      string
	offloaded         bool
	instance, bodyKey string
}

// stateEntry is the follower entry for key at revision, carrying an entity state that says t:
// name under a .name predicate, inline under a .description predicate, and a storage reference
// when the body is offloaded. Every value is a canonical ENTITY_STATES value.
func stateEntry(f failer, key string, revision uint64, t text) follower.Entry {
	f.Helper()
	at := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	state := graph.EntityState{ID: key, UpdatedAt: at}
	for _, pv := range [][2]string{{"test.doc.name", t.name}, {"test.doc.description", t.inline}} {
		if pv[1] != "" {
			state.Triples = append(state.Triples, message.Triple{Subject: key, Predicate: pv[0], Object: pv[1],
				Source: "test", Timestamp: at})
		}
	}
	if t.offloaded {
		state.StorageRef = &message.StorageReference{StorageInstance: cmp.Or(t.instance, "objectstore"),
			Key: cmp.Or(t.bodyKey, key+"/body")}
	}
	value, err := json.Marshal(state)
	if err != nil {
		f.Fatalf("marshal %s: %v", key, err)
	}
	return follower.Entry{Key: key, Revision: revision, Value: value, Bytes: len(value)}
}

// deleteEntry is the follower entry for a delete of key at revision.
func deleteEntry(key string, revision uint64) follower.Entry {
	return follower.Entry{Key: key, Revision: revision, Deleted: true}
}

// holders is the IDs of the entities a search for word finds, in ID order.
func holders(f failer, ix *lexical.Index, word string) []string {
	f.Helper()
	q, err := lexical.ParseQuery(word)
	if err != nil {
		f.Fatalf("ParseQuery(%q): %v", word, err)
	}
	result, err := ix.Search(q, lexical.Options{Limit: 100})
	if err != nil {
		f.Fatalf("Search(%q): %v", word, err)
	}
	var ids []string
	for _, hit := range result.Hits {
		ids = append(ids, hit.EntityID)
	}
	slices.Sort(ids)
	return ids
}

// requireHeld fails unless a search for each word finds exactly ids.
func requireHeld(f failer, ix *lexical.Index, ids []string, words ...string) {
	f.Helper()
	for _, word := range words {
		if got := holders(f, ix, word); !slices.Equal(got, ids) {
			f.Fatalf("search %q found %v; want %v", word, got, ids)
		}
	}
}

// newTestApplier is an applier with graph-lexical's default rule and gauges on a registry of its
// own, completing revisions through complete.
func newTestApplier(f failer, complete func(key string, revision uint64)) *applier {
	f.Helper()
	metrics, err := newLexicalMetrics(metric.NewMetricsRegistry())
	if err != nil {
		f.Fatalf("newLexicalMetrics: %v", err)
	}
	return newApplier(entitytext.New(nil, defaultMaxTextLen), defaultMaxTextLen, complete, metrics, slog.New(slog.DiscardHandler))
}

// mustDeliver delivers e and fails on a refusal; it returns the body read the delivery asks for.
func mustDeliver(f failer, a *applier, e follower.Entry) (bodyRead, bool) {
	f.Helper()
	read, ok, err := a.deliver(e)
	if err != nil {
		f.Fatalf("deliver %s at %d: %v", e.Key, e.Revision, err)
	}
	return read, ok
}

const (
	pump  = "c360.lex.test.doc.pump.a"
	valve = "c360.lex.test.doc.valve.b"
)

// TestOlderDeliveryIgnored: once a key's revision R is applied, a delivery of the key at R or
// below changes nothing in the index and is completed.
func TestOlderDeliveryIgnored(t *testing.T) {
	var completed []uint64
	a := newTestApplier(t, func(_ string, revision uint64) { completed = append(completed, revision) })
	mustDeliver(t, a, stateEntry(t, pump, 7, text{name: "current"}))
	for _, revision := range []uint64{7, 3} {
		if _, ok := mustDeliver(t, a, stateEntry(t, pump, revision, text{name: "older", offloaded: true})); ok {
			t.Fatalf("an ignored delivery at %d asked for a body read", revision)
		}
	}
	if _, ok := mustDeliver(t, a, deleteEntry(pump, 6)); ok {
		t.Fatal("an ignored delete asked for a body read")
	}
	requireHeld(t, a.index, []string{pump}, "current")
	requireHeld(t, a.index, nil, "older")
	if want := []uint64{7, 7, 3, 6}; !slices.Equal(completed, want) {
		t.Fatalf("completed %v; want %v, every delivery completed", completed, want)
	}
}

// TestOlderDeliveryAfterDelete: once a key is deleted at revision D, a delivery of it at D or
// below is still ignored and completed: a delete is the revision the index last applied for the
// key.
func TestOlderDeliveryAfterDelete(t *testing.T) {
	var completed []uint64
	a := newTestApplier(t, func(_ string, revision uint64) { completed = append(completed, revision) })
	mustDeliver(t, a, stateEntry(t, pump, 1, text{name: "oldword"}))
	mustDeliver(t, a, deleteEntry(pump, 2))
	for _, revision := range []uint64{1, 2} {
		if _, ok := mustDeliver(t, a, stateEntry(t, pump, revision, text{name: "oldword", offloaded: true})); ok {
			t.Fatalf("a delivery at %d after the delete at 2 asked for a body read", revision)
		}
	}
	requireHeld(t, a.index, nil, "oldword")
	if n := a.index.Len(); n != 0 {
		t.Fatalf("index holds %d entities after the delete; want 0", n)
	}
	if want := []uint64{1, 2, 1, 2}; !slices.Equal(completed, want) {
		t.Fatalf("completed %v; want %v, every delivery completed", completed, want)
	}
}

// TestDeleteRemovesEntity: an entity deleted at revision D is found by no search once D is
// complete, and the other entities are untouched.
func TestDeleteRemovesEntity(t *testing.T) {
	var deleted bool
	var a *applier
	a = newTestApplier(t, func(key string, revision uint64) {
		if key == pump && revision == 3 {
			deleted = true
			requireHeld(t, a.index, nil, "pumpname")
			requireHeld(t, a.index, []string{valve}, "shared")
		}
	})
	mustDeliver(t, a, stateEntry(t, pump, 1, text{name: "pumpname", inline: "shared"}))
	mustDeliver(t, a, stateEntry(t, valve, 2, text{name: "valvename", inline: "shared"}))
	requireHeld(t, a.index, []string{pump, valve}, "shared")
	mustDeliver(t, a, deleteEntry(pump, 3))
	if !deleted {
		t.Fatal("the delete's revision was not completed")
	}
	requireHeld(t, a.index, []string{valve}, "shared", "valvename")
	if n := a.index.Len(); n != 1 {
		t.Fatalf("index holds %d entities; want 1", n)
	}
}

// TestUnreadableEntityRefused: a value that is not a canonical entity state is refused with the
// state contract's reason and its revision is not completed, so the follower's reset, not a
// silent skip, answers for it; the index keeps what it held.
func TestUnreadableEntityRefused(t *testing.T) {
	var completed []uint64
	a := newTestApplier(t, func(_ string, revision uint64) { completed = append(completed, revision) })
	mustDeliver(t, a, stateEntry(t, pump, 1, text{name: "pumpname"}))
	_, _, err := a.deliver(follower.Entry{Key: pump, Revision: 2, Value: []byte("not an entity state")})
	var stateErr *graph.StateContractError
	if !errors.As(err, &stateErr) || stateErr.Reason != graph.GraphStateReasonUnreadableEntity {
		t.Fatalf("deliver of an unreadable value = %v; want a state contract error, reason %s", err,
			graph.GraphStateReasonUnreadableEntity)
	}
	if !slices.Equal(completed, []uint64{1}) {
		t.Fatalf("completed %v; want only revision 1", completed)
	}
	requireHeld(t, a.index, []string{pump}, "pumpname")
}
