package component

import (
	"log/slog"

	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/model"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/pkg/security"
	"github.com/c360studio/semengine/storage"
	"github.com/c360studio/semengine/storage/storeregistry"
	"github.com/c360studio/semengine/types"
)

// PlatformMeta provides platform identity to components.
// Type alias to avoid import cycles while maintaining compatibility.
type PlatformMeta = types.PlatformMeta

// StoreProvider is implemented by storage components that own one or more stores
// addressable by StorageInstance name (ADR-063). For the boot-composed component
// set, ComponentManager reads this AFTER a component Starts to populate the
// shared StoreRegistry and clears those entries when the component Stops. A
// desired-state write does not swap a handle in the running process.
//
// The map is keyed by the StorageInstance name each store STAMPS into refs
// (store.InstanceName()), so the registry key, the store-provide port token, and
// the ref's StorageInstance are the same value by construction. Returns nil/empty
// before Start (no store yet) or for a component that provides no store.
type StoreProvider interface {
	ProvidedStores() map[string]storage.StreamableStore
}

// Dependencies provides all external dependencies needed by components.
//
// It carries no lifecycle manager and no tool registry. A component that needs
// the lifecycle manager receives it from its host when it is registered or
// constructed, so importing component reaches no graph or agentic package.
//
// PayloadRegistry uses the concrete *payloadregistry.Registry rather than an
// interface on purpose: payloadregistry is a leaf package this package already
// imports, so there's no cycle to dodge, and message.NewDecoder also
// requires the concrete type. An interface here would force callers
// to type-assert at every Decoder construction site — pure friction
// for no abstraction win.
type Dependencies struct {
	NATSClient      *natsclient.Client        // NATS client for messaging
	MetricsRegistry *metric.MetricsRegistry   // Metrics registry for Prometheus (can be nil)
	Logger          *slog.Logger              // Structured logger (can be nil, defaults to slog.Default())
	Platform        PlatformMeta              // Platform identity (organization and platform)
	Security        security.Config           // Platform-wide security configuration
	ModelRegistry   model.RegistryReader      // Boot-selected model registry (can be nil)
	PayloadRegistry *payloadregistry.Registry // Shared payload registry (can be nil; components unmarshaling BaseMessage require it)

	// StoreRegistry is the shared {StorageInstance → storage.StreamableStore}
	// resolver (ADR-063). The ComponentManager populates it from the boot-composed
	// storage components' store-provide ports at Start and clears entries at Stop;
	// content-fetch consumers (graph-embedding, fusion) resolve a StorageRef's
	// StorageInstance through it, lazily per-fetch. Concrete framework-leaf type
	// per the PayloadRegistry precedent.
	//
	// Can be nil — deployments with no offloaded-content fetch pay zero cost. A
	// consumer that receives a StorageReference without this exact-name authority
	// reports content-unresolved and excludes the body; it never selects another
	// store or degrades solely because the name is unresolved.
	StoreRegistry *storeregistry.Registry
}

// GetLogger returns the configured logger or a default logger if none is provided
func (d *Dependencies) GetLogger() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}

// GetLoggerWithComponent returns a logger configured with component context
func (d *Dependencies) GetLoggerWithComponent(componentName string) *slog.Logger {
	return d.GetLogger().With("component", componentName)
}
