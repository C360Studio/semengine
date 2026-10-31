package clustering

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// fakeKV is an in-memory COMMUNITY_INDEX behind a real natsclient.KVStore. It records every
// write and removal in order and counts reads. Any method it does not define panics through
// the nil embedded interface, which fails the test: the maintainer must call nothing else.
type fakeKV struct {
	jetstream.KeyValue

	mu        sync.Mutex
	data      map[string][]byte
	ops       []string // "put <key>" and "delete <key>", in order
	keysCalls int
	getCalls  int

	// Hooks run before the operation, outside mu; a non-nil error fails the operation.
	beforePut    func(ctx context.Context, key string) error
	beforeDelete func(ctx context.Context, key string) error
	beforeKeys   func(ctx context.Context) error
}

func (f *fakeKV) hook(h *func(context.Context, string) error) func(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return *h
}

func (f *fakeKV) Put(ctx context.Context, key string, value []byte) (uint64, error) {
	if h := f.hook(&f.beforePut); h != nil {
		if err := h(ctx, key); err != nil {
			return 0, err
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data[key] = slices.Clone(value)
	f.ops = append(f.ops, "put "+key)
	return uint64(len(f.ops)), nil
}

func (f *fakeKV) Delete(ctx context.Context, key string, _ ...jetstream.KVDeleteOpt) error {
	if h := f.hook(&f.beforeDelete); h != nil {
		if err := h(ctx, key); err != nil {
			return err
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, key)
	f.ops = append(f.ops, "delete "+key)
	return nil
}

func (f *fakeKV) Keys(ctx context.Context, _ ...jetstream.WatchOpt) ([]string, error) {
	f.mu.Lock()
	h := f.beforeKeys
	f.keysCalls++
	f.mu.Unlock()
	if h != nil {
		if err := h(ctx); err != nil {
			return nil, err
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.data) == 0 {
		return nil, jetstream.ErrNoKeysFound
	}
	return slices.Sorted(maps.Keys(f.data)), nil
}

func (f *fakeKV) Get(context.Context, string) (jetstream.KeyValueEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls++
	return nil, jetstream.ErrKeyNotFound
}

func (f *fakeKV) set(apply func(f *fakeKV)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	apply(f)
}

// takeOps returns the operations since the last call and forgets them.
func (f *fakeKV) takeOps() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	ops := f.ops
	f.ops = nil
	return ops
}

func (f *fakeKV) reads() (keys, gets int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.keysCalls, f.getCalls
}

// records decodes every stored record by key.
func (f *fakeKV) records(t *testing.T) map[string]*Community {
	t.Helper()
	out, err := f.decodeAll()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (f *fakeKV) decodeAll() (map[string]*Community, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]*Community{}
	for key, value := range f.data {
		var c Community
		if err := json.Unmarshal(value, &c); err != nil {
			return nil, fmt.Errorf("decode %s: %w", key, err)
		}
		out[key] = &c
	}
	return out, nil
}

// newTestMaintainer builds a Maintainer as a product does, over a KVStore whose bucket is
// a fakeKV.
func newTestMaintainer(t *testing.T) (*Maintainer, *fakeKV) {
	t.Helper()
	m, kv, err := newFakeMaintainer()
	if err != nil {
		t.Fatal(err)
	}
	return m, kv
}

// newFakeMaintainer is newTestMaintainer for a caller without a *testing.T, such as a
// Rapid check. natsclient.NewClient does not dial.
func newFakeMaintainer() (*Maintainer, *fakeKV, error) {
	client, err := natsclient.NewClient("")
	if err != nil {
		return nil, nil, err
	}
	kv := &fakeKV{data: map[string][]byte{}}
	storage, err := NewNATSCommunityStorage(client.NewKVStore(kv))
	if err != nil {
		return nil, nil, err
	}
	m, err := NewMaintainer(storage)
	return m, kv, err
}

// textState is an entity whose one string value is text; with no text it has a number only.
func textState(id string, text string) *graph.EntityState {
	if text == "" {
		return &graph.EntityState{ID: id, Triples: []message.Triple{{Subject: id, Predicate: "docs.note.size", Object: 42.0}}}
	}
	return &graph.EntityState{ID: id, Triples: []message.Triple{{Subject: id, Predicate: "docs.note.body", Object: text}}}
}

func mustUpsert(t *testing.T, m *Maintainer, id string, revision uint64, text string) {
	t.Helper()
	if err := m.Upsert(id, revision, textState(id, text)); err != nil {
		t.Fatalf("Upsert(%s, %d): %v", id, revision, err)
	}
}

func mustTick(t *testing.T, m *Maintainer) {
	t.Helper()
	if err := m.Tick(t.Context()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
}

// groupKeys are an entity ID's three record keys, split by the test itself.
func groupKeys(id string) []string {
	p := strings.Split(id, ".")
	return []string{
		"0." + strings.Join(p[:4], "."),
		"1." + strings.Join(p[:3], "."),
		"2." + strings.Join(p[:2], "."),
	}
}

func puts(keys ...string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, "put "+k)
	}
	return out
}

func TestNewMaintainerRefusesNil(t *testing.T) {
	m, err := NewMaintainer(nil)
	if m != nil || !errs.IsInvalid(err) {
		t.Fatalf("NewMaintainer(nil) = %v, %v; want nil and an invalid error", m, err)
	}
}

// TestObsoleteDeliveryIgnored: an entity applied at revision 9 is unchanged by an upsert
// or delete at revision 7 or 9.
func TestObsoleteDeliveryIgnored(t *testing.T) {
	m, kv := newTestMaintainer(t)
	id := "acme.dep1.src.docs.note.n1"
	mustUpsert(t, m, id, 9, "alpha beta")
	mustUpsert(t, m, id, 7, "gamma")
	mustUpsert(t, m, id, 9, "gamma")
	for _, rev := range []uint64{7, 9} {
		if err := m.Delete(id, rev); err != nil {
			t.Fatalf("Delete(%d): %v", rev, err)
		}
	}
	m.BootstrapDone()
	mustTick(t, m)

	records := kv.records(t)
	if len(records) != 3 {
		t.Fatalf("records = %v, want the entity's three groups", slices.Sorted(maps.Keys(records)))
	}
	for _, key := range groupKeys(id) {
		r := records[key]
		if r == nil || r.MemberCount != 1 || !slices.Equal(r.Keywords, []string{"alpha", "beta"}) {
			t.Fatalf("record %s = %+v, want one member with keywords [alpha beta]", key, r)
		}
	}
}

// TestRefusedEntityMakesTickIncomplete: refusals name the entity and contribute nothing;
// Tick publishes the rest and returns an *IncompleteError naming every refused ID until
// each is upserted validly or deleted.
func TestRefusedEntityMakesTickIncomplete(t *testing.T) {
	m, kv := newTestMaintainer(t)
	good := "acme.dep1.src.docs.note.n1"
	unparseable := "acme.dep1.src.docs.n2"
	nilState := "acme.dep1.src.docs.note.n3"
	mismatched := "acme.dep1.src.docs.note.n4"
	mustUpsert(t, m, good, 1, "alpha")

	refusals := []struct {
		id    string
		state *graph.EntityState
	}{
		{unparseable, textState(unparseable, "beta")},
		{nilState, nil},
		{mismatched, textState("acme.dep1.src.docs.note.n5", "beta")},
	}
	for _, r := range refusals {
		err := m.Upsert(r.id, 1, r.state)
		if !errs.IsInvalid(err) || !strings.Contains(err.Error(), r.id) {
			t.Fatalf("Upsert(%s) = %v, want an invalid error naming it", r.id, err)
		}
	}

	m.BootstrapDone()
	var incomplete *IncompleteError
	if err := m.Tick(t.Context()); !errors.As(err, &incomplete) {
		t.Fatalf("Tick = %v, want an *IncompleteError", err)
	}
	if want := []string{unparseable, nilState, mismatched}; !slices.Equal(incomplete.Refused, want) {
		t.Fatalf("refused = %q, want %q", incomplete.Refused, want)
	}
	for _, key := range groupKeys(good) {
		if r := kv.records(t)[key]; r == nil || r.MemberCount != 1 {
			t.Fatalf("record %s = %+v, want the one valid member", key, r)
		}
	}

	if err := m.Delete(unparseable, 2); err != nil {
		t.Fatalf("Delete(%s): %v", unparseable, err)
	}
	mustUpsert(t, m, nilState, 2, "beta")
	mustUpsert(t, m, mismatched, 2, "beta")
	mustTick(t, m)
	if r := kv.records(t)["0.acme.dep1.src.docs"]; r.MemberCount != 3 {
		t.Fatalf("level-0 record = %+v, want three members", r)
	}
}

// TestDeleteRefusesUnparseableUnknownID: a delete for an ID the maintainer never held and
// the parser refuses is refused; a valid unknown ID is already gone.
func TestDeleteRefusesUnparseableUnknownID(t *testing.T) {
	m, _ := newTestMaintainer(t)
	if err := m.Delete("acme.dep1", 1); !errs.IsInvalid(err) || !strings.Contains(err.Error(), "acme.dep1") {
		t.Fatalf("Delete(unparseable) = %v, want an invalid error naming it", err)
	}
	if err := m.Delete("acme.dep1.src.docs.note.n1", 1); err != nil {
		t.Fatalf("Delete(unknown) = %v, want nil", err)
	}
}

// TestTickBeforeBootstrap: before BootstrapDone, and with a nil context, Tick refuses and
// touches nothing.
func TestTickBeforeBootstrap(t *testing.T) {
	m, kv := newTestMaintainer(t)
	mustUpsert(t, m, "acme.dep1.src.docs.note.n1", 1, "alpha")
	if err := m.Tick(t.Context()); !errors.Is(err, ErrBootstrapIncomplete) {
		t.Fatalf("Tick before BootstrapDone = %v, want ErrBootstrapIncomplete", err)
	}
	m.BootstrapDone()
	//nolint:staticcheck // SA1012: the nil context is the input under test.
	if err := m.Tick(nil); !errs.IsInvalid(err) {
		t.Fatalf("Tick(nil) = %v, want an invalid error", err)
	}
	if ops := kv.takeOps(); len(ops) != 0 {
		t.Fatalf("operations = %q, want none", ops)
	}
	if keys, gets := kv.reads(); keys != 0 || gets != 0 {
		t.Fatalf("reads: %d listings, %d gets; want none", keys, gets)
	}
	mustTick(t, m)
	if n := len(kv.records(t)); n != 3 {
		t.Fatalf("%d records after BootstrapDone, want 3", n)
	}
}

// TestFailedWriteRetried: a write that fails leaves its record pending; the next tick
// writes it and nothing else.
func TestFailedWriteRetried(t *testing.T) {
	m, kv := newTestMaintainer(t)
	mustUpsert(t, m, "acme.dep1.src.docs.note.n1", 1, "alpha")
	mustUpsert(t, m, "acme.dep1.src.code.file.f1", 1, "beta")
	m.BootstrapDone()
	failing := "2.acme.dep1" // written last: records are written in key order
	kv.set(func(f *fakeKV) {
		f.beforePut = func(_ context.Context, key string) error {
			if key == failing {
				return errors.New("injected put failure")
			}
			return nil
		}
	})
	if err := m.Tick(t.Context()); err == nil {
		t.Fatal("Tick with a failing write = nil, want an error")
	}
	if _, ok := kv.records(t)[failing]; ok {
		t.Fatalf("%s written despite the failure", failing)
	}
	kv.takeOps()

	kv.set(func(f *fakeKV) { f.beforePut = nil })
	mustTick(t, m)
	if r := kv.records(t)[failing]; r == nil || r.MemberCount != 2 {
		t.Fatalf("%s = %+v after the retry, want both members", failing, r)
	}
	// The records the failed tick did write are equal to what it published: not rewritten.
	if ops, want := kv.takeOps(), puts(failing); !slices.Equal(ops, want) {
		t.Fatalf("retry operations = %q, want %q", ops, want)
	}
}

// TestFirstTickFailedPublicationKeepsOldRecords: COMMUNITY_INDEX holds a stale key from an
// earlier run. A failed listing is retried; a failed write on the first tick leaves the
// stale record in place; the next successful tick writes every record and only then
// removes the stale key, listing the keys only once.
func TestFirstTickFailedPublicationKeepsOldRecords(t *testing.T) {
	m, kv := newTestMaintainer(t)
	stale := "0.acme.old.src.docs"
	kv.set(func(f *fakeKV) { f.data[stale] = []byte(`{"id":"acme.old.src.docs","level":0}`) })
	id := "acme.dep1.src.docs.note.n1"
	mustUpsert(t, m, id, 1, "alpha")
	m.BootstrapDone()

	kv.set(func(f *fakeKV) {
		f.beforeKeys = func(context.Context) error { return errors.New("injected listing failure") }
	})
	if err := m.Tick(t.Context()); err == nil {
		t.Fatal("Tick with a failing listing = nil, want an error")
	}
	kv.set(func(f *fakeKV) {
		f.beforeKeys = nil
		f.beforePut = func(_ context.Context, key string) error {
			if key == "2.acme.dep1" {
				return errors.New("injected put failure")
			}
			return nil
		}
	})
	if err := m.Tick(t.Context()); err == nil {
		t.Fatal("Tick with a failing write = nil, want an error")
	}
	if _, ok := kv.records(t)[stale]; !ok {
		t.Fatal("the stale record was removed before every record was written")
	}
	for _, op := range kv.takeOps() {
		if strings.HasPrefix(op, "delete ") {
			t.Fatalf("failed tick removed %q", op)
		}
	}

	kv.set(func(f *fakeKV) { f.beforePut = nil })
	mustTick(t, m)
	records := kv.records(t)
	if _, ok := records[stale]; ok {
		t.Fatal("the stale record survived a successful tick")
	}
	if got, want := slices.Sorted(maps.Keys(records)), slices.Sorted(slices.Values(groupKeys(id))); !slices.Equal(got, want) {
		t.Fatalf("keys = %q, want %q", got, want)
	}
	if ops, want := kv.takeOps(), []string{"put 2.acme.dep1", "delete " + stale}; !slices.Equal(ops, want) {
		t.Fatalf("operations = %q, want the write before the removal: %q", ops, want)
	}
	if keys, _ := kv.reads(); keys != 2 {
		t.Fatalf("%d listings, want 2 (the failed one and its retry)", keys)
	}
}

// TestUpsertDuringBlockedWrite: while a Tick is blocked inside a storage write, Upsert and
// Delete return; the next tick publishes their changes.
func TestUpsertDuringBlockedWrite(t *testing.T) {
	m, kv := newTestMaintainer(t)
	mustUpsert(t, m, "acme.dep1.src.docs.note.n1", 1, "alpha")
	mustUpsert(t, m, "acme.dep1.src.docs.note.n2", 1, "beta")
	m.BootstrapDone()

	// The write waits only on release, not on the KVStore's own timeout, so an Upsert that
	// waited on it would wait past the bound below.
	entered := make(chan struct{})
	release := make(chan struct{})
	var entering, releasing sync.Once
	t.Cleanup(func() { releasing.Do(func() { close(release) }) })
	kv.set(func(f *fakeKV) {
		f.beforePut = func(context.Context, string) error {
			entering.Do(func() { close(entered) })
			<-release
			return nil
		}
	})
	tickDone := make(chan error, 1)
	go func() { tickDone <- m.Tick(t.Context()) }()
	<-entered

	applied := make(chan error, 1)
	go func() {
		err := m.Upsert("acme.dep1.src.docs.note.n3", 1, textState("acme.dep1.src.docs.note.n3", "gamma"))
		applied <- errors.Join(err, m.Delete("acme.dep1.src.docs.note.n2", 2))
	}()
	// The timer only bounds the wait; the receive orders the check after Upsert and Delete.
	select {
	case err := <-applied:
		if err != nil {
			t.Fatalf("Upsert/Delete during a blocked write: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Upsert/Delete waited on the blocked write")
	}

	releasing.Do(func() { close(release) })
	if err := <-tickDone; err != nil {
		t.Fatalf("blocked Tick: %v", err)
	}
	mustTick(t, m)
	r := kv.records(t)["0.acme.dep1.src.docs"]
	if r.MemberCount != 2 || !slices.Equal(r.Keywords, []string{"alpha", "gamma"}) {
		t.Fatalf("record = %+v, want n1 and n3", r)
	}
}

// TestTickReportsCancel: a context ending while a tick writes, or while it removes,
// returns the context's error and leaves the work for the next tick.
func TestTickReportsCancel(t *testing.T) {
	m, kv := newTestMaintainer(t)
	id := "acme.dep1.src.docs.note.n1"
	mustUpsert(t, m, id, 1, "alpha")
	m.BootstrapDone()

	// blockUntilCancel makes the hooked operation wait for the tick's context to end.
	blockUntilCancel := func(entered chan struct{}) func(context.Context, string) error {
		var once sync.Once
		return func(ctx context.Context, _ string) error {
			once.Do(func() { close(entered) })
			<-ctx.Done()
			return ctx.Err()
		}
	}
	cancelledTick := func() error {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		entered := make(chan struct{})
		kv.set(func(f *fakeKV) {
			f.beforePut = blockUntilCancel(entered)
			f.beforeDelete = blockUntilCancel(entered)
		})
		done := make(chan error, 1)
		go func() { done <- m.Tick(ctx) }()
		<-entered
		cancel()
		err := <-done
		kv.set(func(f *fakeKV) { f.beforePut, f.beforeDelete = nil, nil })
		return err
	}

	if err := cancelledTick(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Tick cancelled in a write = %v, want context.Canceled", err)
	}
	mustTick(t, m)
	if n := len(kv.records(t)); n != 3 {
		t.Fatalf("%d records after the retry, want 3", n)
	}

	if err := m.Delete(id, 2); err != nil {
		t.Fatal(err)
	}
	if err := cancelledTick(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Tick cancelled in a removal = %v, want context.Canceled", err)
	}
	if n := len(kv.records(t)); n != 3 {
		t.Fatalf("%d records after the cancelled removal, want the 3 old ones", n)
	}
	mustTick(t, m)
	if n := len(kv.records(t)); n != 0 {
		t.Fatalf("%d records after the retry, want none", n)
	}
}

// TestRepresentativesWithoutScan: representatives are the lexically smallest members with
// words, at every level; members without words never are; when the smallest are deleted
// or lose their words the next ones take their place. 1,100 members insert in reverse
// order and 400 deletions from the front cross the ordered set's chunk boundaries.
func TestRepresentativesWithoutScan(t *testing.T) {
	m, kv := newTestMaintainer(t)
	// Members without words sort before every member with words.
	for i := range 3 {
		mustUpsert(t, m, fmt.Sprintf("acme.dep1.src.docs.note.a%d", i), 1, "")
	}
	var text []string
	for i := 1099; i >= 0; i-- {
		id := fmt.Sprintf("acme.dep1.src.docs.note.n%04d", i)
		mustUpsert(t, m, id, 1, "alpha")
		text = append(text, id)
	}
	slices.Sort(text)
	m.BootstrapDone()

	check := func(step string) {
		t.Helper()
		mustTick(t, m)
		want := text[:MaxRepEntities]
		for _, key := range groupKeys(text[0]) {
			if got := kv.records(t)[key].RepEntities; !slices.Equal(got, want) {
				t.Fatalf("%s: %s representatives = %q, want %q", step, key, got, want)
			}
		}
	}
	check("initial")

	mustUpsert(t, m, text[0], 2, "")
	text = text[1:]
	check("smallest lost its words")

	// Checked after each deletion, so the front of the ordered set holds fewer than
	// MaxRepEntities members at some step.
	for i := range 400 {
		if err := m.Delete(text[0], 3); err != nil {
			t.Fatal(err)
		}
		text = text[1:]
		check(fmt.Sprintf("%d smallest deleted", i+1))
	}

	mustUpsert(t, m, "acme.dep1.src.docs.note.n0000", 4, "beta")
	text = append([]string{"acme.dep1.src.docs.note.n0000"}, text...)
	check("smallest regained words")
}

// TestTickCost: after the first tick, a tick reads nothing from COMMUNITY_INDEX and writes
// only the changed groups' records; an unchanged graph, or a change that leaves every
// record equal, writes nothing.
func TestTickCost(t *testing.T) {
	m, kv := newTestMaintainer(t)
	var ids []string
	for _, src := range []string{"s1", "s2", "s3", "s4"} {
		for _, domain := range []string{"docs", "code"} {
			for i := range 3 {
				id := fmt.Sprintf("acme.dep1.%s.%s.note.n%d", src, domain, i)
				mustUpsert(t, m, id, 1, "alpha "+domain)
				ids = append(ids, id)
			}
		}
	}
	m.BootstrapDone()
	mustTick(t, m)
	if n := len(kv.takeOps()); n != 4*2+4+1 {
		t.Fatalf("first tick wrote %d records, want 13", n)
	}

	expectOps := func(step string, want []string) {
		t.Helper()
		mustTick(t, m)
		if keys, gets := kv.reads(); keys != 1 || gets != 0 {
			t.Fatalf("%s: %d listings and %d gets, want the first tick's one listing", step, keys, gets)
		}
		ops := kv.takeOps()
		slices.Sort(ops)
		slices.Sort(want)
		if !slices.Equal(ops, slices.Compact(want)) {
			t.Fatalf("%s: operations = %q, want %q", step, ops, want)
		}
	}

	// k = 2 new entities, in two sources.
	added := []string{"acme.dep1.s1.docs.note.n9", "acme.dep1.s3.code.note.n9"}
	var want []string
	for _, id := range added {
		mustUpsert(t, m, id, 1, "alpha")
		want = append(want, puts(groupKeys(id)...)...)
	}
	expectOps("two added", want)

	expectOps("unchanged graph", nil)

	mustUpsert(t, m, ids[0], 2, "alpha docs")
	expectOps("same value at a newer revision", nil)

	alone := "acme.dep1.s2.misc.note.n1"
	mustUpsert(t, m, alone, 1, "alpha")
	expectOps("one added", puts(groupKeys(alone)...))
	if err := m.Delete(alone, 2); err != nil {
		t.Fatal(err)
	}
	keys := groupKeys(alone)
	expectOps("one deleted", []string{"delete " + keys[0], "put " + keys[1], "put " + keys[2]})
}
