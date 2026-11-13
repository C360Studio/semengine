package embedding

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/c360studio/semengine/internal/harness/probe"
	"github.com/c360studio/semengine/natsclient"
)

// benchRecords and benchDimensions size the cache benchmarks: 50,000 generated records
// of the BM25 embedder's default 384 dimensions.
const (
	benchRecords    = 50_000
	benchDimensions = 384
)

// benchCache returns a cache over benchRecords generated records whose Run has applied the
// snapshot, and the query vector. The vectors come from a fixed seed, so every run scores the
// same data.
func benchCache(b *testing.B) (*VectorCache, []float32) {
	b.Helper()
	rng := rand.New(rand.NewPCG(1, 2))
	vector := func() []float32 {
		v := make([]float32, benchDimensions)
		for i := range v {
			v[i] = rng.Float32()
		}
		return v
	}
	kv := &replayKV{memKV: newMemKV()}
	for n := range benchRecords {
		id := fmt.Sprintf("acme.ops.robotics.gcs.drone.d%05d", n)
		data, err := json.Marshal(&Record{EntityID: id, Vector: vector(), Status: StatusGenerated})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := kv.Put(b.Context(), id, data); err != nil {
			b.Fatal(err)
		}
	}
	client, err := natsclient.NewClient("")
	if err != nil {
		b.Fatal(err)
	}
	cache := NewVectorCache(NewStorage(client.NewKVStore(kv), nil))
	ctx, cancel := context.WithCancel(b.Context())
	done := make(chan error, 1)
	go func() { done <- cache.Run(ctx) }()
	b.Cleanup(func() { cancel(); <-done })
	if _, err := probe.Await(b.Context(), func(context.Context) (error, error) {
		_, _, err := cache.FindSimilar("", nil, nil, 0)
		return err, nil
	}, func(err error) bool { return err == nil }); err != nil {
		b.Fatalf("the cache never answered: %v", err)
	}
	return cache, vector()
}

// BenchmarkFindSimilar50k is the call time of one similar query, limit 10, over 50,000 vectors:
// the read under the view's lock, then scoring, sorting and the cut outside it.
func BenchmarkFindSimilar50k(b *testing.B) {
	cache, query := benchCache(b)
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := cache.FindSimilar("", query, nil, 10); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVectorCacheLockHold50k is the time FindSimilar holds the view's lock over 50,000
// vectors: graphview.View.ListCounted runs its whole body, the copy and the sort of every key,
// under the view's one mutex (pkg/graphview/view.go ListCounted), so its uncontended call time is
// the hold time plus one uncontended lock and unlock. While it holds the lock the view applies no
// write.
func BenchmarkVectorCacheLockHold50k(b *testing.B) {
	cache, _ := benchCache(b)
	view := cache.view.Load()
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := view.ListCounted(""); err != nil {
			b.Fatal(err)
		}
	}
}
