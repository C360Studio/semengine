package embedding

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/natsclient"
)

// memKV is an in-memory jetstream.KeyValue sufficient for exercising Storage
// without a NATS server, reached through natsclient.KVStore as production reaches a
// real bucket. Only the operations Storage uses are implemented; everything else
// returns errNotImplemented so a test that strays off the covered surface fails
// loudly instead of silently succeeding on a zero value.
//
// Delete ignores its options, so a revision-fenced delete is not modelled here: the
// fenced removal is proven against a real server (TestRevisionGuardHistoryMatchesModel).
var errNotImplemented = errors.New("memKV: not implemented")

type memKV struct {
	mu   sync.Mutex
	data map[string][]byte
	revs map[string]uint64 // per-key last-write revision, for CAS via Update
	rev  uint64
}

func newMemKV() *memKV {
	return &memKV{data: make(map[string][]byte), revs: make(map[string]uint64)}
}

// newTestStorage builds a Storage over two fake buckets through natsclient's KVStore,
// as production builds it over two real ones.
func newTestStorage(t *testing.T, index, dedup jetstream.KeyValue) *Storage {
	t.Helper()
	return NewStorage(newTestKVStore(t, index), newTestKVStore(t, dedup))
}

func newTestKVStore(t *testing.T, bucket jetstream.KeyValue, opts ...func(*natsclient.KVOptions)) *natsclient.KVStore {
	t.Helper()
	client, err := natsclient.NewClient("")
	if err != nil {
		t.Fatalf("natsclient.NewClient: %v", err)
	}
	return client.NewKVStore(bucket, opts...)
}

type memKVEntry struct {
	key   string
	value []byte
	rev   uint64
	op    jetstream.KeyValueOp
}

func (e *memKVEntry) Key() string                     { return e.key }
func (e *memKVEntry) Value() []byte                   { return e.value }
func (e *memKVEntry) Revision() uint64                { return e.rev }
func (e *memKVEntry) Created() time.Time              { return time.Time{} }
func (e *memKVEntry) Delta() uint64                   { return 0 }
func (e *memKVEntry) Operation() jetstream.KeyValueOp { return e.op }
func (e *memKVEntry) Bucket() string                  { return "MEM" }

func (m *memKV) Put(_ context.Context, key string, value []byte) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rev++
	m.data[key] = append([]byte(nil), value...)
	m.revs[key] = m.rev
	return m.rev, nil
}

func (m *memKV) Get(_ context.Context, key string) (jetstream.KeyValueEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	if !ok {
		return nil, jetstream.ErrKeyNotFound
	}
	// The per-key revision, as a real bucket's entry revision is the key's last-write
	// sequence: the value CAS compares against.
	return &memKVEntry{key: key, value: v, rev: m.revs[key], op: jetstream.KeyValuePut}, nil
}

func (m *memKV) Delete(_ context.Context, key string, _ ...jetstream.KVDeleteOpt) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	delete(m.revs, key)
	m.rev++
	return nil
}

// has reports whether a key is currently present.
func (m *memKV) has(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[key]
	return ok
}

// revision reports a key's current per-key revision, 0 when absent.
func (m *memKV) revision(key string) uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.revs[key]
}

func (m *memKV) Bucket() string { return "MEM" }

func (m *memKV) PutString(ctx context.Context, key string, value string) (uint64, error) {
	return m.Put(ctx, key, []byte(value))
}

func (m *memKV) Purge(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	return m.Delete(ctx, key, opts...)
}

func (m *memKV) GetRevision(_ context.Context, _ string, _ uint64) (jetstream.KeyValueEntry, error) {
	return nil, errNotImplemented
}

// Create succeeds only when the key is absent, returning jetstream.ErrKeyExists
// otherwise. A real bucket's Create also succeeds over a delete marker; memKV removes
// deleted keys entirely, so absence models both cases.
func (m *memKV) Create(_ context.Context, key string, value []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[key]; ok {
		return 0, jetstream.ErrKeyExists
	}
	m.rev++
	m.data[key] = append([]byte(nil), value...)
	m.revs[key] = m.rev
	return m.rev, nil
}

// Update succeeds only if the key's current per-key revision matches the expected
// one. A missing key or a moved revision returns jetstream.ErrKeyExists, the
// wrong-last-sequence sentinel natsclient classifies as a conflict.
func (m *memKV) Update(_ context.Context, key string, value []byte, revision uint64) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.revs[key]
	if !ok || cur != revision {
		return 0, jetstream.ErrKeyExists
	}
	m.rev++
	m.data[key] = append([]byte(nil), value...)
	m.revs[key] = m.rev
	return m.rev, nil
}

func (m *memKV) Watch(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, errNotImplemented
}

func (m *memKV) WatchAll(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, errNotImplemented
}

func (m *memKV) WatchFiltered(_ context.Context, _ []string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, errNotImplemented
}

func (m *memKV) Keys(_ context.Context, _ ...jetstream.WatchOpt) ([]string, error) {
	return nil, errNotImplemented
}

func (m *memKV) ListKeys(_ context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyLister, error) {
	return nil, errNotImplemented
}

func (m *memKV) ListKeysFiltered(_ context.Context, _ ...string) (jetstream.KeyLister, error) {
	return nil, errNotImplemented
}

func (m *memKV) History(_ context.Context, _ string, _ ...jetstream.WatchOpt) ([]jetstream.KeyValueEntry, error) {
	return nil, errNotImplemented
}

func (m *memKV) PurgeDeletes(_ context.Context, _ ...jetstream.KVPurgeOpt) error {
	return errNotImplemented
}

func (m *memKV) Status(_ context.Context) (jetstream.KeyValueStatus, error) {
	return nil, errNotImplemented
}

// replayKV is a memKV whose Watch replays the current key set as the initial snapshot
// (each current value, then the nil end-of-snapshot sentinel), modelling a real
// watch's last-per-subject delivery.
type replayKV struct {
	*memKV
}

func (r *replayKV) Watch(_ context.Context, _ string, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	r.mu.Lock()
	entries := make([]jetstream.KeyValueEntry, 0, len(r.data))
	for k, v := range r.data {
		entries = append(entries, &memKVEntry{
			key: k, value: append([]byte(nil), v...), rev: r.revs[k], op: jetstream.KeyValuePut,
		})
	}
	r.mu.Unlock()

	ch := make(chan jetstream.KeyValueEntry, len(entries)+1)
	for _, e := range entries {
		ch <- e
	}
	ch <- nil // end-of-snapshot sentinel
	return &memWatcher{updates: ch}, nil
}

// memWatcher delivers entries on one Updates channel and closes it on Stop, as
// nats.go closes a stopped watch's channel.
type memWatcher struct {
	updates chan jetstream.KeyValueEntry
	once    sync.Once
}

func (w *memWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }

func (w *memWatcher) Stop() error {
	w.once.Do(func() { close(w.updates) })
	return nil
}
