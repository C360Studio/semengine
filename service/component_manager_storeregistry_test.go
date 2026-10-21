package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/storage"
)

// The store registration tests, from the pin's component_manager_storeregistry_test.go:1-271
// (design D3, "Stores"). The pin built struct-literal managers and set a component's state, or
// its storeProvided tracking, by hand before calling registerProvidedStores; here the manager
// comes from NewComponentManager with the providers composed, and which stores a provider
// registered is read from the store registry. The pin's TestRegisterProvidedStores_SkipsWhenNotLive
// (:96-106) is not carried: its liveness guard existed only for a Stop that cancels a Start in
// flight, which the port's Stop cannot do (sweep C5; design D2, Q-A option A).

// fakeStreamable is a minimal StreamableStore for manager-wiring tests.
type fakeStreamable struct{ id string }

func (f *fakeStreamable) Put(context.Context, string, []byte) error      { return nil }
func (f *fakeStreamable) Get(context.Context, string) ([]byte, error)    { return []byte(f.id), nil }
func (f *fakeStreamable) List(context.Context, string) ([]string, error) { return nil, nil }
func (f *fakeStreamable) Delete(context.Context, string) error           { return nil }
func (f *fakeStreamable) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader([]byte(f.id))), nil
}

// storeProviderComponent is a Discoverable that also owns stores (ADR-063).
type storeProviderComponent struct {
	mockDiscoverableComponent
	provided map[string]storage.StreamableStore
}

func (s *storeProviderComponent) ProvidedStores() map[string]storage.StreamableStore {
	return s.provided
}

var _ component.StoreProvider = (*storeProviderComponent)(nil)

// stopObservingProvider is a barrierStoreProvider whose Stop runs onStop and records that it ran.
type stopObservingProvider struct {
	*barrierStoreProvider
	onStop  func() error
	stopped bool
}

func (p *stopObservingProvider) Stop(context.Context) error {
	p.stopped = true
	if p.onStop == nil {
		return nil
	}
	return p.onStop()
}

// provider returns a store provider owning one store named instance.
func provider(instance string) *storeProviderComponent {
	return &storeProviderComponent{
		provided: map[string]storage.StreamableStore{instance: &fakeStreamable{id: instance}},
	}
}

func TestRegisterProvidedStores_PopulatesRegistry(t *testing.T) {
	objectstore := provider("objectstore")
	plain := &mockDiscoverableComponent{}
	cm := newTestComponentManager(t,
		testComponent{name: "objectstore", instance: objectstore},
		testComponent{name: "plain", instance: plain},
	)

	if err := cm.registerProvidedStores("objectstore", objectstore); err != nil {
		t.Fatalf("register provided store: %v", err)
	}
	if got, ok := cm.storeRegistry.Streamable("objectstore"); !ok || got != objectstore.provided["objectstore"] {
		t.Fatalf("store not registered: (%v, %v)", got, ok)
	}
	// A component that is not a store provider is a silent no-op: not every component owns a store.
	if err := cm.registerProvidedStores("plain", plain); err != nil {
		t.Fatalf("register non-provider: %v", err)
	}
	if _, ok := cm.storeRegistry.Streamable("objectstore"); !ok {
		t.Fatal("registering a non-provider removed the provider's store")
	}
}

func TestDeregisterProvidedStores_ClearsRegistry(t *testing.T) {
	objectstore := provider("objectstore")
	cm := newTestComponentManager(t, testComponent{name: "objectstore", instance: objectstore})
	if err := cm.registerProvidedStores("objectstore", objectstore); err != nil {
		t.Fatalf("register provided store: %v", err)
	}
	cm.deregisterProvidedStores("objectstore")

	if _, ok := cm.storeRegistry.Streamable("objectstore"); ok {
		t.Fatal("store still registered after stop")
	}
	// Tracking is cleared, so a redundant deregister is a no-op.
	cm.deregisterProvidedStores("objectstore")
}

func TestStopLifecycleComponent_DeregistersBeforeProviderStop(t *testing.T) {
	observed := &stopObservingProvider{
		barrierStoreProvider: &barrierStoreProvider{
			barrierTestComponent: newBarrierTestComponent("objectstore"),
			provided: map[string]storage.StreamableStore{
				"objectstore": &fakeStreamable{id: "objectstore"},
			},
		},
	}
	cm := newTestComponentManager(t, testComponent{name: "objectstore", instance: observed})
	observed.onStop = func() error {
		if _, ok := cm.storeRegistry.Streamable("objectstore"); ok {
			return errors.New("store remained registered when provider Stop began")
		}
		return nil
	}

	if err := cm.Start(t.Context()); err != nil {
		t.Fatalf("start component manager: %v", err)
	}
	if _, ok := cm.storeRegistry.Streamable("objectstore"); !ok {
		t.Fatal("the provider's store was not registered after Start")
	}
	if err := cm.Stop(stopContext(t)); err != nil {
		t.Fatalf("stop component manager: %v", err)
	}
	if !observed.stopped {
		t.Fatal("Stop did not call the provider's Stop")
	}
}

func TestDuplicateOwnership_RefusedNotClobbered(t *testing.T) {
	first := provider("objectstore")
	second := provider("objectstore")
	cm := newTestComponentManager(t,
		testComponent{name: "owner-a", instance: first},
		testComponent{name: "owner-b", instance: second},
	)
	if err := cm.registerProvidedStores("owner-a", first); err != nil {
		t.Fatalf("register incumbent: %v", err)
	}

	// A second boot-composed component claims the same instance: refused, incumbent kept.
	if err := cm.registerProvidedStores("owner-b", second); err == nil {
		t.Fatal("duplicate ownership did not return a provider startup error")
	}

	got, _ := cm.storeRegistry.Streamable("objectstore")
	if fs, _ := got.(*fakeStreamable); fs == nil || fs != first.provided["objectstore"] {
		t.Fatalf("duplicate ownership clobbered the incumbent store: %v", got)
	}
	// owner-b registered nothing, so deregistering it must not evict owner-a's store.
	cm.deregisterProvidedStores("owner-b")
	if _, ok := cm.storeRegistry.Streamable("objectstore"); !ok {
		t.Fatal("deregistering the refused owner evicted the incumbent")
	}
}

func TestDuplicateOwnership_RollsBackOnlyRivalRegistrations(t *testing.T) {
	incumbent := provider("shared")
	// Instances register in sorted order, so the rival's "a-rival-only" registers before its
	// "shared" claim is refused, and must then be rolled back.
	rival := &storeProviderComponent{provided: map[string]storage.StreamableStore{
		"a-rival-only": &fakeStreamable{id: "rival-only"},
		"shared":       &fakeStreamable{id: "rival-shared"},
	}}
	cm := newTestComponentManager(t,
		testComponent{name: "owner-a", instance: incumbent},
		testComponent{name: "owner-b", instance: rival},
	)
	if err := cm.registerProvidedStores("owner-a", incumbent); err != nil {
		t.Fatalf("register incumbent: %v", err)
	}

	if err := cm.registerProvidedStores("owner-b", rival); err == nil {
		t.Fatal("duplicate ownership did not return an error")
	}
	if _, ok := cm.storeRegistry.Streamable("a-rival-only"); ok {
		t.Fatal("failed rival left an earlier successful claim registered")
	}
	got, ok := cm.storeRegistry.Streamable("shared")
	if !ok || got != incumbent.provided["shared"] {
		t.Fatalf("rival rollback removed or replaced incumbent: (%v, %v)", got, ok)
	}
}

func TestSequentialComponentLifetimesCanReuseStorageInstance(t *testing.T) {
	oldComp := provider("objectstore")
	cm := newTestComponentManager(t, testComponent{name: "objectstore", instance: oldComp})
	if err := cm.registerProvidedStores("objectstore", oldComp); err != nil {
		t.Fatalf("register old store: %v", err)
	}

	// Once the first lifetime's stores are deregistered, a later lifetime may reuse the same
	// StorageInstance, and the registry resolves it to the fresh handle.
	cm.deregisterProvidedStores("objectstore")
	freshComp := provider("objectstore")
	if err := cm.registerProvidedStores("objectstore", freshComp); err != nil {
		t.Fatalf("register fresh store: %v", err)
	}

	got, _ := cm.storeRegistry.Streamable("objectstore")
	if got != freshComp.provided["objectstore"] {
		t.Fatal("later component lifetime did not resolve to the fresh handle")
	}
}

func TestRegisterProvidedStores_InvalidClaimFailsAtomically(t *testing.T) {
	invalid := &storeProviderComponent{provided: map[string]storage.StreamableStore{
		"valid": &fakeStreamable{id: "valid"},
		"":      &fakeStreamable{id: "invalid"},
	}}
	other := provider("valid")
	cm := newTestComponentManager(t,
		testComponent{name: "invalid-provider", instance: invalid},
		testComponent{name: "other-provider", instance: other},
	)

	if err := cm.registerProvidedStores("invalid-provider", invalid); err == nil {
		t.Fatal("empty StorageInstance did not return an error")
	}
	if _, ok := cm.storeRegistry.Streamable("valid"); ok {
		t.Fatal("invalid provider left a partially registered store")
	}
	// The refused provider tracks nothing: once another provider holds "valid", deregistering
	// the refused one leaves that store registered.
	if err := cm.registerProvidedStores("other-provider", other); err != nil {
		t.Fatalf("register the other provider: %v", err)
	}
	cm.deregisterProvidedStores("invalid-provider")
	if got, ok := cm.storeRegistry.Streamable("valid"); !ok || got != other.provided["valid"] {
		t.Fatalf("invalid provider left registration tracking that evicted another's store: (%v, %v)", got, ok)
	}
}

func TestRegisterProvidedStores_NilClaimFails(t *testing.T) {
	var typedNil *fakeStreamable
	for _, tc := range []struct {
		name  string
		store storage.StreamableStore
	}{
		{name: "nil interface", store: nil},
		{name: "typed nil", store: typedNil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp := &storeProviderComponent{
				provided: map[string]storage.StreamableStore{"nil-store": tc.store},
			}
			cm := newTestComponentManager(t, testComponent{name: "nil-provider", instance: comp})

			if err := cm.registerProvidedStores("nil-provider", comp); err == nil {
				t.Fatal("nil store did not return an error")
			}
			if _, ok := cm.storeRegistry.Streamable("nil-store"); ok {
				t.Fatal("nil store was registered")
			}
		})
	}
}
