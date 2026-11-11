// Package websocket is the websocket output component: a server that streams applied ENTITY_STATES,
// or one core NATS subject, to WebSocket clients. Register is its only export. Until the server
// exists, its factory checks a configuration and builds nothing.
package websocket

import (
	"encoding/json"
	"errors"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/pkg/errs"
)

// key names the websocket output once: its registration and its logger.
const key = "websocket"

// errNotBuilt refuses construction of a configuration that passed every check, because the
// component that would serve it does not exist yet.
//
// shortcut: the factory checks the configuration and builds nothing, so no composition admits an
// instance that never serves; task 3.1 of alpha4-output-websocket replaces this with the server.
var errNotBuilt = errors.New("websocket output: the server is not built yet (alpha4-output-websocket task 3.1)")

// declarePorts is the component.PortDeclarer for the websocket output: the ports the factory
// resolves for rawConfig, computed without dependencies.
func declarePorts(rawConfig json.RawMessage, _ string) (component.PortConfig, error) {
	s, err := resolveConfig(rawConfig)
	if err != nil {
		return component.PortConfig{}, err
	}
	return component.PortConfigFrom([]component.Port{s.input}, []component.Port{s.output}), nil
}

// newSettings is the factory's derivation: the resolved configuration, refusing a nil NATS client.
func newSettings(rawConfig json.RawMessage, deps component.Dependencies) (settings, error) {
	if deps.NATSClient == nil {
		return settings{}, errs.WrapInvalid(errs.ErrInvalidConfig, key, "factory", "NATS client required")
	}
	return resolveConfig(rawConfig)
}

// create is the factory function for websocket outputs. It opens no connection.
func create(rawConfig json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	if _, err := newSettings(rawConfig, deps); err != nil {
		return nil, err
	}
	return nil, errs.Wrap(errNotBuilt, key, "factory", "build")
}

// Register registers the websocket output factory with the component registry.
func Register(registry *component.Registry) error {
	return registry.Register(component.Registration{
		Name:        key,
		Type:        "output",
		Protocol:    "websocket",
		Domain:      "network",
		Description: "WebSocket server streaming applied ENTITY_STATES or a core NATS subject to clients",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     create,
		Ports:       declarePorts,
	})
}
