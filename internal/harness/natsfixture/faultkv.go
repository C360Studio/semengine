package natsfixture

import (
	"context"
	"sync"

	"github.com/nats-io/nats.go/jetstream"
)

// KVOp names a key-value write FaultKV can fail.
type KVOp string

// The four writes FaultKV can fail.
const (
	KVPut    KVOp = "put"
	KVCreate KVOp = "create"
	KVUpdate KVOp = "update"
	KVDelete KVOp = "delete"
)

// realKV is the embedded bucket. It is an alias so the embedded field is unexported: callers of
// FaultKV reach the real bucket only through the wrapper.
type realKV = jetstream.KeyValue

// FaultKV wraps a real bucket so that Put, Create, Update and Delete can fail before the real call
// (nothing happens on the server) or after it (the write stands and the caller still sees an
// error, the "server applied it, client saw an error" case). PutString is a Put. Every other method,
// Purge included, reaches the real bucket unchanged. A fault stays set until cleared with a nil
// error. It is typed on jetstream.KeyValue only, so natsfixture imports no package of this module
// outside internal/harness.
type FaultKV struct {
	realKV

	mu     sync.Mutex
	faults map[KVOp]fault
	calls  map[KVOp]int
}

type fault struct {
	err   error
	after bool
}

var _ jetstream.KeyValue = (*FaultKV)(nil)

// NewFaultKV wraps bucket with no fault set.
func NewFaultKV(bucket jetstream.KeyValue) *FaultKV {
	return &FaultKV{realKV: bucket, faults: map[KVOp]fault{}, calls: map[KVOp]int{}}
}

// FailBefore makes op return err without calling the real bucket. A nil err clears op's fault.
func (k *FaultKV) FailBefore(op KVOp, err error) { k.set(op, fault{err: err}) }

// FailAfter makes op call the real bucket, discard its result, and return err. A nil err clears
// op's fault.
func (k *FaultKV) FailAfter(op KVOp, err error) { k.set(op, fault{err: err, after: true}) }

func (k *FaultKV) set(op KVOp, f fault) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if f.err == nil {
		delete(k.faults, op)
		return
	}
	k.faults[op] = f
}

// Calls reports how many real calls each operation made, in a map the caller owns.
func (k *FaultKV) Calls() map[KVOp]int {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make(map[KVOp]int, len(k.calls))
	for op, n := range k.calls {
		out[op] = n
	}
	return out
}

// do applies op's fault around call, counting the real call when it is made.
func (k *FaultKV) do(op KVOp, call func() error) error {
	k.mu.Lock()
	f, faulted := k.faults[op]
	k.mu.Unlock()
	if faulted && !f.after {
		return f.err
	}
	err := call()
	k.mu.Lock()
	k.calls[op]++
	k.mu.Unlock()
	if faulted {
		return f.err
	}
	return err
}

// Put writes through to the real bucket unless KVPut is faulted.
func (k *FaultKV) Put(ctx context.Context, key string, value []byte) (uint64, error) {
	var rev uint64
	err := k.do(KVPut, func() (err error) {
		rev, err = k.realKV.Put(ctx, key, value)
		return err
	})
	if err != nil {
		return 0, err
	}
	return rev, nil
}

// PutString is Put, so a KVPut fault applies to it too.
func (k *FaultKV) PutString(ctx context.Context, key string, value string) (uint64, error) {
	return k.Put(ctx, key, []byte(value))
}

// Create writes through to the real bucket unless KVCreate is faulted.
func (k *FaultKV) Create(ctx context.Context, key string, value []byte, opts ...jetstream.KVCreateOpt) (uint64, error) {
	var rev uint64
	err := k.do(KVCreate, func() (err error) {
		rev, err = k.realKV.Create(ctx, key, value, opts...)
		return err
	})
	if err != nil {
		return 0, err
	}
	return rev, nil
}

// Update writes through to the real bucket unless KVUpdate is faulted.
func (k *FaultKV) Update(ctx context.Context, key string, value []byte, revision uint64) (uint64, error) {
	var rev uint64
	err := k.do(KVUpdate, func() (err error) {
		rev, err = k.realKV.Update(ctx, key, value, revision)
		return err
	})
	if err != nil {
		return 0, err
	}
	return rev, nil
}

// Delete writes through to the real bucket unless KVDelete is faulted.
func (k *FaultKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	return k.do(KVDelete, func() error { return k.realKV.Delete(ctx, key, opts...) })
}
