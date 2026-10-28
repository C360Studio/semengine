//go:build integration

package embedding

import (
	"testing"

	"github.com/c360studio/semengine/storage/objectstore"
	"github.com/c360studio/semengine/storage/storeregistry"
)

// TestWorkerResolvesLiveRegistryStatePerFetch (the pin's
// TestIntegration_WorkerResolvesLiveRegistryStatePerFetch, on natsfixture, #195) proves the
// registry lifecycle against a real NATS ObjectStore. A borrowed store works while its exact
// owner is registered, a live deregistration excludes the body without failure, and
// re-registration is observed on the next fetch without a cached handle or retry loop.
func TestWorkerResolvesLiveRegistryStatePerFetch(t *testing.T) {
	client := fixtureClient(t)
	ctx := t.Context()

	store, err := objectstore.NewStoreWithConfig(ctx, client, objectstore.Config{
		BucketName:   "EMBEDDING_RESOLVER_LIFECYCLE",
		InstanceName: "content",
	})
	if err != nil {
		t.Fatalf("NewStoreWithConfig: %v", err)
	}
	if err := store.Put(ctx, "doc/1", []byte("owned body")); err != nil {
		t.Fatalf("Put: %v", err)
	}

	registry := storeregistry.New()
	if err := registry.Register("content", store); err != nil {
		t.Fatalf("Register: %v", err)
	}
	metrics := &countingMetrics{}
	worker := &Worker{
		maxSourceTextLen: 100, metrics: metrics, storeResolver: registry,
		logger: discardLogger(),
	}
	ref := &StorageRef{StorageInstance: "content", Key: "doc/1"}

	body, _, err := worker.fetchTextFromStorage(ctx, ref)
	if err != nil || body != "owned body" {
		t.Fatalf("registered fetch = (%q, %v), want owned body", body, err)
	}

	registry.Deregister("content")
	text, err := worker.getSourceText(ctx, &Record{StorageRef: ref, IdentityText: "inline identity"})
	if err != nil || text != "inline identity" {
		t.Fatalf("deregistered fetch = (%q, %v), want inline identity exclusion", text, err)
	}
	if metrics.unresolved != 1 || metrics.failed != 0 {
		t.Fatalf("deregistered metrics unresolved=%d failed=%d, want 1/0", metrics.unresolved, metrics.failed)
	}

	if err := registry.Register("content", store); err != nil {
		t.Fatalf("re-register: %v", err)
	}
	body, _, err = worker.fetchTextFromStorage(ctx, ref)
	if err != nil || body != "owned body" {
		t.Fatalf("re-registered fetch = (%q, %v), want owned body", body, err)
	}
}
