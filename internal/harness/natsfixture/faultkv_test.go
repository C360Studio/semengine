package natsfixture

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

// memKV is an in-memory bucket with the revision rules FaultKV's tests rely on: every write takes
// the next revision, Create refuses an existing key, Update refuses a stale revision. Methods it
// does not implement panic through the nil embedded interface.
type memKV struct {
	jetstream.KeyValue
	rev  uint64
	keys map[string]memEntry
}

type memEntry struct {
	jetstream.KeyValueEntry
	key   string
	value []byte
	rev   uint64
}

func (e memEntry) Key() string      { return e.key }
func (e memEntry) Value() []byte    { return e.value }
func (e memEntry) Revision() uint64 { return e.rev }

var errWrongRevision = errors.New("memKV: wrong last revision")

func newMemKV() *memKV { return &memKV{keys: map[string]memEntry{}} }

func (m *memKV) write(key string, value []byte) uint64 {
	m.rev++
	m.keys[key] = memEntry{key: key, value: value, rev: m.rev}
	return m.rev
}

func (m *memKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	e, ok := m.keys[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}
	return e, nil
}

func (m *memKV) Put(_ context.Context, key string, value []byte) (uint64, error) {
	return m.write(key, value), nil
}

func (m *memKV) Create(_ context.Context, key string, value []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	if _, ok := m.keys[key]; ok {
		return 0, jetstream.ErrKeyExists
	}
	return m.write(key, value), nil
}

func (m *memKV) Update(_ context.Context, key string, value []byte, revision uint64) (uint64, error) {
	if e, ok := m.keys[key]; !ok || e.rev != revision {
		return 0, errWrongRevision
	}
	return m.write(key, value), nil
}

func (m *memKV) Delete(_ context.Context, key string, _ ...jetstream.KVDeleteOpt) error {
	delete(m.keys, key)
	return nil
}

// nats-fixture › "Fail after a bucket update": the caller sees the injected error, the write stands
// at the next revision, and one bucket Update was made.
func TestFaultKVFailAfterUpdate(t *testing.T) {
	ctx := t.Context()
	bucket := newMemKV()
	kv := NewFaultKV(bucket)
	rev, err := kv.Create(ctx, "k", []byte("v1"))
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("timeout after the server applied it")
	kv.FailAfter(KVUpdate, injected)
	if _, err := kv.Update(ctx, "k", []byte("v2"), rev); !errors.Is(err, injected) {
		t.Fatalf("Update = %v, want the injected error", err)
	}
	e, err := bucket.Get(ctx, "k")
	if err != nil || string(e.Value()) != "v2" || e.Revision() != rev+1 {
		t.Fatalf("fresh read after a fail-after Update: %v %v, want v2 at revision %d", e, err, rev+1)
	}
	if calls := kv.Calls(); calls[KVUpdate] != 1 || calls[KVCreate] != 1 {
		t.Fatalf("Calls() = %v, want one bucket Create and one bucket Update", calls)
	}
}

// nats-fixture › "Fail before a bucket create": the caller sees the injected error, the key does
// not exist, and no bucket Create was made.
func TestFaultKVFailBeforeCreate(t *testing.T) {
	ctx := t.Context()
	bucket := newMemKV()
	kv := NewFaultKV(bucket)
	injected := errors.New("refused before the server saw it")
	kv.FailBefore(KVCreate, injected)
	if _, err := kv.Create(ctx, "k", []byte("v")); !errors.Is(err, injected) {
		t.Fatalf("Create = %v, want the injected error", err)
	}
	if _, err := bucket.Get(ctx, "k"); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("key after a fail-before Create: %v, want ErrKeyNotFound", err)
	}
	if n := kv.Calls()[KVCreate]; n != 0 {
		t.Fatalf("Calls()[KVCreate] = %d, want 0", n)
	}
}

// Each of the four operations is faulted independently, before and after; a nil error clears the
// fault; PutString is a Put; every other method reaches the bucket bucket untouched.
func TestFaultKVEachOperation(t *testing.T) {
	ctx := t.Context()
	injected := errors.New("injected")
	ops := map[KVOp]func(kv *FaultKV, bucket *memKV) error{
		KVPut: func(kv *FaultKV, _ *memKV) error { _, err := kv.Put(ctx, "k", []byte("put")); return err },
		KVCreate: func(kv *FaultKV, _ *memKV) error {
			_, err := kv.Create(ctx, "fresh", []byte("create"))
			return err
		},
		KVUpdate: func(kv *FaultKV, bucket *memKV) error {
			_, err := kv.Update(ctx, "k", []byte("update"), bucket.keys["k"].rev)
			return err
		},
		KVDelete: func(kv *FaultKV, _ *memKV) error { return kv.Delete(ctx, "k") },
	}
	for op, call := range ops {
		for _, after := range []bool{false, true} {
			bucket := newMemKV()
			bucket.write("k", []byte("v0"))
			kv := NewFaultKV(bucket)
			fail := kv.FailBefore
			want := 0
			if after {
				fail, want = kv.FailAfter, 1
			}
			fail(op, injected)
			if err := call(kv, bucket); !errors.Is(err, injected) {
				t.Errorf("%s (after %t) = %v, want the injected error", op, after, err)
			}
			if n := kv.Calls()[op]; n != want {
				t.Errorf("%s (after %t): Calls() = %d, want %d", op, after, n, want)
			}
			for other := range ops {
				if other != op && kv.Calls()[other] != 0 {
					t.Errorf("%s (after %t): a call counted against %s", op, after, other)
				}
			}
			fail(op, nil)
			bucket.write("k", []byte("v1"))
			delete(bucket.keys, "fresh")
			if err := call(kv, bucket); err != nil || kv.Calls()[op] != want+1 {
				t.Errorf("%s (after %t) once cleared = %v, Calls() = %v; want the bucket result", op, after, err, kv.Calls())
			}
		}
	}

	bucket := newMemKV()
	kv := NewFaultKV(bucket)
	kv.FailBefore(KVPut, injected)
	if _, err := kv.PutString(ctx, "k", "v"); !errors.Is(err, injected) {
		t.Fatalf("PutString = %v, want the Put fault", err)
	}
	kv.FailBefore(KVPut, nil)
	if _, err := kv.PutString(ctx, "k", "v"); err != nil || kv.Calls()[KVPut] != 1 {
		t.Fatalf("PutString = %v, Calls() = %v; want one bucket Put", err, kv.Calls())
	}
	if e, err := kv.Get(ctx, "k"); err != nil || string(e.Value()) != "v" {
		t.Fatalf("Get through the wrapper = %v %v", e, err)
	}
	calls := kv.Calls()
	calls[KVPut] = 99
	if kv.Calls()[KVPut] != 1 {
		t.Fatal("Calls() returned the wrapper's own map")
	}
}

// Faults may be set while another goroutine writes through the wrapper.
func TestFaultKVConcurrentUse(t *testing.T) {
	ctx := t.Context()
	kv := NewFaultKV(newMemKV())
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			kv.FailAfter(KVDelete, errors.New("x"))
			kv.FailBefore(KVDelete, nil)
			_ = kv.Calls()
		}
	}()
	for range 100 {
		if _, err := kv.Put(ctx, "k", []byte("v")); err != nil {
			t.Error(err)
		}
	}
	<-done
}
