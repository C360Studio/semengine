package service

import (
	"encoding/json"
	"log/slog"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/types"
)

// Dependencies provides the standard dependencies that all services receive.
// Services should use HTTP or NATS RPC for inter-service communication.
type Dependencies struct {
	NATSClient        *natsclient.Client
	MetricsRegistry   *metric.MetricsRegistry
	Logger            *slog.Logger
	Platform          types.PlatformMeta        // Platform identity
	Manager           *config.Manager           // Centralized configuration management
	ComponentRegistry *component.Registry       // Component registry for ComponentManager
	PayloadRegistry   *payloadregistry.Registry // Shared payload registry plumbed to component deps
	ServiceManager    *Manager                  // The manager that constructs the service
}

// Constructor defines the standard constructor signature for all services.
// Every service must have a constructor that follows this pattern.
// The constructor receives raw JSON config and must handle its own parsing.
type Constructor func(rawConfig json.RawMessage, deps *Dependencies) (Service, error)
