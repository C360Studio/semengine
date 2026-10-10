package storeregistry_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/c360studio/semengine/storage"
	"github.com/c360studio/semengine/storage/storeregistry"
)

// fakeStore is a minimal StreamableStore for registry tests. Pointer-backed, so
// two fakes are distinct instances.
type fakeStore struct{ id string }

func (f *fakeStore) Put(context.Context, string, []byte) error      { return nil }
func (f *fakeStore) Get(context.Context, string) ([]byte, error)    { return []byte(f.id), nil }
func (f *fakeStore) List(context.Context, string) ([]string, error) { return nil, nil }
func (f *fakeStore) Delete(context.Context, string) error           { return nil }
func (f *fakeStore) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader([]byte(f.id))), nil
}

var _ storage.StreamableStore = (*fakeStore)(nil)

func TestRegisterAndResolve(t *testing.T) {
	r := storeregistry.New()
	s := &fakeStore{id: "objectstore"}

	if err := r.Register("objectstore", s); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Streamable resolves the same handle.
	got, ok := r.Streamable("objectstore")
	if !ok || got != s {
		t.Fatalf("Streamable: got (%v,%v), want (%v,true)", got, ok, s)
	}

	// Store() (fusion path) resolves the same underlying store.
	gotStore, ok := r.Store("objectstore")
	if !ok || gotStore != storage.Store(s) {
		t.Fatalf("Store: got (%v,%v), want the same store", gotStore, ok)
	}

	// Unknown instance resolves to (nil,false), not a panic.
	if _, ok := r.Streamable("nope"); ok {
		t.Fatal("Streamable(unknown) returned ok=true")
	}
}

func TestRegisterDuplicateOwnershipErrors(t *testing.T) {
	r := storeregistry.New()
	if err := r.Register("objectstore", &fakeStore{id: "a"}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	// A second, different component claiming the same instance is a wiring fault.
	err := r.Register("objectstore", &fakeStore{id: "b"})
	if err == nil {
		t.Fatal("duplicate ownership did not error")
	}
	// The first owner is unchanged (no silent clobber).
	got, _ := r.Streamable("objectstore")
	if fs, _ := got.(*fakeStore); fs == nil || fs.id != "a" {
		t.Fatalf("duplicate register clobbered the entry: %v", got)
	}
}

func TestDeregisterThenReregister_Reconfig(t *testing.T) {
	r := storeregistry.New()
	old := &fakeStore{id: "old"}
	if err := r.Register("objectstore", old); err != nil {
		t.Fatalf("Register old: %v", err)
	}

	// Reconfig: Stop deregisters, Start of the fresh instance re-registers. No
	// collision, and the new handle wins.
	r.Deregister("objectstore")
	fresh := &fakeStore{id: "fresh"}
	if err := r.Register("objectstore", fresh); err != nil {
		t.Fatalf("re-register after deregister must succeed, got: %v", err)
	}
	got, _ := r.Streamable("objectstore")
	if got != fresh {
		t.Fatalf("post-reconfig resolve got %v, want the fresh handle", got)
	}
}

func TestDeregisterIsIdempotentAndDoesNotClose(t *testing.T) {
	r := storeregistry.New()
	// Deregistering an absent instance is a no-op, not a panic.
	r.Deregister("absent")
	if s, ok := r.Streamable("absent"); ok {
		t.Fatalf("no-op deregister left an entry: Streamable(%q) = (%v, true)", "absent", s)
	}
	// The registry stays usable after a no-op deregister.
	if err := r.Register("x", &fakeStore{id: "x"}); err != nil {
		t.Fatalf("Register after no-op deregister: %v", err)
	}
}

func TestRegisterGuards(t *testing.T) {
	r := storeregistry.New()
	if err := r.Register("", &fakeStore{}); err == nil {
		t.Fatal("empty instance name should error")
	}
	if err := r.Register("x", nil); err == nil {
		t.Fatal("nil store should error")
	}
}

// A second registration under another name leaves the first in place: both resolve, each to
// its own store.
func TestDistinctInstancesResolveTheirOwnStores(t *testing.T) {
	r := storeregistry.New()
	stores := map[string]*fakeStore{"a": {id: "a"}, "b": {id: "b"}}
	for _, name := range []string{"a", "b"} {
		if err := r.Register(name, stores[name]); err != nil {
			t.Fatalf("Register(%q): %v", name, err)
		}
	}
	for name, want := range stores {
		if got, ok := r.Streamable(name); !ok || got != want {
			t.Fatalf("Streamable(%q) = (%v, %v), want (%v, true)", name, got, ok, want)
		}
	}
}
