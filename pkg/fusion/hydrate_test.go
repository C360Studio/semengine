package fusion_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/fusion"
	"github.com/c360studio/semengine/storage/storeregistry"
)

// The store registry is the production StoreResolver (ADR-063): checked at
// compile time.
var _ fusion.StoreResolver = (*storeregistry.Registry)(nil)

// TestBodyResolver_LocationIndependence proves the crux of ADR-062 increment 4:
// ONE BodyResolver derefs handles across DIFFERENT backends, picked by the
// handle's StorageInstance — the engine never assumes one concrete store. A
// handle into "objectstore-primary" and a handle into "filestore-media" both
// resolve through the same resolver to their own backend's bytes.
func TestBodyResolver_LocationIndependence(t *testing.T) {
	objectStore := &fakeStore{data: map[string][]byte{"code/on_event.go": []byte("package handlers")}}
	fileStore := &fakeStore{data: map[string][]byte{"media/diagram.bin": {0x00, 0xff, 0x00}}}

	r := fusion.NewBodyResolver(fusion.MapStoreResolver{
		"objectstore-primary": objectStore,
		"filestore-media":     fileStore,
	})

	// Handle into the object store → that backend's bytes.
	got, err := r.ResolveBody(context.Background(), &message.StorageReference{
		StorageInstance: "objectstore-primary", Key: "code/on_event.go",
	})
	if err != nil {
		t.Fatalf("ResolveBody(objectstore): %v", err)
	}
	if string(got) != "package handlers" {
		t.Errorf("objectstore body = %q, want %q", got, "package handlers")
	}

	// Handle into the filestore → that backend's (binary, non-UTF-8) bytes,
	// byte-exact (no StoredContent envelope corruption).
	got, err = r.ResolveBody(context.Background(), &message.StorageReference{
		StorageInstance: "filestore-media", Key: "media/diagram.bin",
	})
	if err != nil {
		t.Fatalf("ResolveBody(filestore): %v", err)
	}
	if len(got) != 3 || got[1] != 0xff {
		t.Errorf("filestore body = %v, want [0 255 0] byte-exact", got)
	}
}

func TestBodyResolver_NilRefIsNoBody(t *testing.T) {
	r := fusion.NewBodyResolver(fusion.MapStoreResolver{})
	got, err := r.ResolveBody(context.Background(), nil)
	if err != nil || got != nil {
		t.Errorf("ResolveBody(nil) = (%v, %v), want (nil, nil) — no body is not an error", got, err)
	}
}

func TestBodyResolver_FaultPaths(t *testing.T) {
	store := &fakeStore{data: map[string][]byte{"k": []byte("v")}}
	r := fusion.NewBodyResolver(fusion.MapStoreResolver{"inst": store})

	// Empty StorageInstance on a non-nil handle = producer fault.
	if _, err := r.ResolveBody(context.Background(), &message.StorageReference{Key: "k"}); err == nil {
		t.Error("expected error for empty StorageInstance")
	}
	// Empty Key on a non-nil handle = producer fault (explicit, not a backend
	// not-found) — surfaces a clear message the engine can degrade on.
	if _, err := r.ResolveBody(context.Background(), &message.StorageReference{StorageInstance: "inst"}); err == nil {
		t.Error("expected error for empty Key")
	}
	// Unknown instance = wiring fault.
	if _, err := r.ResolveBody(context.Background(), &message.StorageReference{StorageInstance: "nope", Key: "k"}); err == nil {
		t.Error("expected error for unregistered instance")
	}
	// Store Get error propagates.
	failing := fusion.NewBodyResolver(fusion.MapStoreResolver{"inst": &fakeStore{getErr: errors.New("backend down")}})
	if _, err := failing.ResolveBody(context.Background(), &message.StorageReference{StorageInstance: "inst", Key: "k"}); err == nil {
		t.Error("expected the store Get error to propagate")
	}
}

func TestBodyResolver_NilResolverErrorsOnHandle(t *testing.T) {
	r := fusion.NewBodyResolver(nil)
	// nil handle is still no-body (no resolver needed).
	if _, err := r.ResolveBody(context.Background(), nil); err != nil {
		t.Errorf("nil handle with nil resolver should be (nil,nil), got err %v", err)
	}
	// A real handle with no resolver configured is a wiring fault.
	if _, err := r.ResolveBody(context.Background(), &message.StorageReference{StorageInstance: "inst", Key: "k"}); err == nil {
		t.Error("expected error resolving a handle with no resolver configured")
	}
}

// TestBodyResolver_StoreRegistry: a body stored under an instance registered in
// the production store registry resolves byte for byte through it.
func TestBodyResolver_StoreRegistry(t *testing.T) {
	reg := storeregistry.New()
	if err := reg.Register("objectstore-primary", &fakeStore{data: map[string][]byte{"k": {0x00, 0xff, 0x00}}}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := fusion.NewBodyResolver(reg).ResolveBody(t.Context(), &message.StorageReference{
		StorageInstance: "objectstore-primary", Key: "k",
	})
	if err != nil || !bytes.Equal(got, []byte{0x00, 0xff, 0x00}) {
		t.Errorf("ResolveBody = (%v, %v), want [0 255 0]", got, err)
	}
}

// TestResolveBodyRefusesNilContext: a nil context is refused before any store
// is asked, even for a handle that would resolve.
func TestResolveBodyRefusesNilContext(t *testing.T) {
	store := &fakeStore{data: map[string][]byte{"k": []byte("v")}}
	r := fusion.NewBodyResolver(fusion.MapStoreResolver{"inst": store})
	var nilCtx context.Context
	got, err := r.ResolveBody(nilCtx, &message.StorageReference{StorageInstance: "inst", Key: "k"})
	if err == nil || got != nil {
		t.Fatalf("ResolveBody(nil ctx) = (%q, %v), want a refusal", got, err)
	}
	if store.gets != 0 {
		t.Errorf("store asked %d times, want 0", store.gets)
	}
}
