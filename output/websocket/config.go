package websocket

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
)

const (
	defaultPath         = "/ws"
	defaultPingInterval = "30s"
	defaultPort         = 8081
)

// removedKeys are keys an earlier websocket configuration may carry, each with what replaces
// it. A configuration carrying one is refused with that guidance, where strict decoding alone
// would only say the key is unknown.
var removedKeys = map[string]string{
	"delivery_mode": "every frame is sent at most once; the at-least-once mode and its acknowledgements are gone",
	"ack_timeout":   "nothing is acknowledged, so there is no timeout to wait for",
	"passthrough": "a data frame's payload is always the message bytes unchanged, " +
		"with subject and timestamp beside it in the frame",
}

// config is the websocket configuration as written. Decoding is strict: a key it does not define
// is refused naming it, and a removed key is refused with what replaces it.
type config struct {
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:One input (a kv-watch of every ENTITY_STATES key or a core NATS subject) and one http network output,category:basic"`
	// Path is the path-only HTTP ServeMux pattern clients upgrade on.
	Path string `json:"path" schema:"type:string,description:WebSocket upgrade path-only ServeMux pattern,category:basic,default:/ws"`
	// PingInterval is the time between pings to each client, a Go duration such as 30s; a pong
	// extends the client's read deadline to twice this.
	PingInterval string `json:"ping_interval" schema:"type:string,description:Interval between pings to each client (e.g. 30s); a client answering none is closed after twice this,category:advanced,default:30s"`
}

// schema is the configuration schema Register publishes.
var schema = component.GenerateConfigSchema(reflect.TypeOf(config{}))

// settings is a configuration resolved and checked: what the factory builds from and the port
// declarer reports.
type settings struct {
	path         string
	pingInterval time.Duration
	// input is the one input port; subject is its subject for a core NATS input, empty for the
	// applied-state kv-watch of ENTITY_STATES.
	input   component.Port
	subject string
	// output is the one http network port the server listens on.
	output component.Port
}

// defaultPorts are the ports of a configuration without a ports block: applied state from every
// ENTITY_STATES key, served on 0.0.0.0:8081.
func defaultPorts() *component.PortConfig {
	return &component.PortConfig{
		Inputs: []component.PortDefinition{
			{Name: "entity_watch", Config: component.KVWatchPort{Bucket: graph.BucketEntityStates}},
		},
		Outputs: []component.PortDefinition{{
			Name:        "websocket_server",
			Config:      component.NetworkPort{Protocol: "http", Host: "0.0.0.0", Port: defaultPort},
			Description: "WebSocket server endpoint",
		}},
	}
}

// resolveConfig decodes rawConfig, applies defaults and checks every key and port. It is the one
// derivation the factory and the port declarer share; it needs no dependencies.
func resolveConfig(rawConfig json.RawMessage) (settings, error) {
	if err := refuseRemovedKeys(rawConfig); err != nil {
		return settings{}, err
	}
	// Scalars start at their defaults so an explicit empty value is refused, not defaulted. A ports
	// block that is present is used as written, never merged with the default ports, so Ports starts
	// nil.
	cfg := config{Path: defaultPath, PingInterval: defaultPingInterval}
	if err := component.SafeUnmarshal(rawConfig, &cfg); err != nil {
		return settings{}, errs.WrapInvalid(err, "websocket", "factory", "config unmarshal")
	}
	if cfg.Ports == nil {
		cfg.Ports = defaultPorts()
	}

	var s settings
	if err := validatePath(cfg.Path); err != nil {
		return settings{}, errs.WrapInvalid(err, "websocket", "factory", "validate path")
	}
	s.path = cfg.Path

	interval, err := time.ParseDuration(cfg.PingInterval)
	if err != nil || interval <= 0 {
		return settings{}, errs.WrapInvalid(errs.ErrInvalidConfig, "websocket", "factory",
			fmt.Sprintf("ping_interval %q must be a duration above zero, such as 30s", cfg.PingInterval))
	}
	s.pingInterval = interval

	if s.input, s.subject, err = resolveInput(cfg.Ports.Inputs); err != nil {
		return settings{}, err
	}
	if s.output, err = resolveOutput(cfg.Ports.Outputs); err != nil {
		return settings{}, err
	}
	return s, nil
}

// refuseRemovedKeys refuses every removed key rawConfig carries, naming each with its guidance.
// A value that is not one object is left to the strict decoder, which names its fault.
func refuseRemovedKeys(rawConfig json.RawMessage) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(rawConfig, &fields) != nil {
		return nil
	}
	var refused []string
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if guidance, ok := removedKeys[key]; ok {
			refused = append(refused, fmt.Sprintf("%q was removed: %s", key, guidance))
		}
	}
	if len(refused) == 0 {
		return nil
	}
	return errs.WrapInvalid(errors.New(strings.Join(refused, "; ")), "websocket", "factory", "config unmarshal")
}

// resolveInput accepts exactly one input: a kv-watch of every ENTITY_STATES key with no history
// (applied state), or a core NATS subject without a queue group. It returns the port
// and, for a subject input, its subject.
func resolveInput(definitions []component.PortDefinition) (component.Port, string, error) {
	if len(definitions) != 1 {
		return component.Port{}, "", errs.WrapInvalid(errs.ErrInvalidConfig, "websocket", "factory",
			fmt.Sprintf("exactly one input port is required, a kv-watch of every %s key or a core NATS subject; got %d",
				graph.BucketEntityStates, len(definitions)))
	}
	port, err := definitions[0].Resolve(component.DirectionInput)
	if err != nil {
		return component.Port{}, "", errs.WrapInvalid(err, "websocket", "factory", "resolve input port")
	}
	switch c := port.Config.(type) {
	case component.KVWatchPort:
		if c.Bucket == graph.BucketEntityStates && len(c.Keys) == 0 && !c.History {
			return port, "", nil
		}
		return component.Port{}, "", errs.WrapInvalid(errs.ErrInvalidConfig, "websocket", "factory",
			fmt.Sprintf("input port %q must be a kv-watch of every %s key with no history", port.Name, graph.BucketEntityStates))
	case component.NATSPort:
		if c.Queue == "" {
			return port, c.Subject, nil
		}
		// Every instance sends every message to its clients; a queue group would split them.
		return component.Port{}, "", errs.WrapInvalid(errs.ErrInvalidConfig, "websocket", "factory",
			fmt.Sprintf("input port %q sets queue %q; a subject input takes every message, so remove the queue", port.Name, c.Queue))
	case component.JetStreamPort:
		return component.Port{}, "", errs.WrapInvalid(errs.ErrInvalidConfig, "websocket", "factory",
			fmt.Sprintf("input port %q is a jetstream input, which was removed; use a kv-watch of every %s key "+
				"to stream applied state, or a core NATS subject", port.Name, graph.BucketEntityStates))
	default:
		return component.Port{}, "", errs.WrapInvalid(errs.ErrInvalidConfig, "websocket", "factory",
			fmt.Sprintf("input port %q is a %s port; the input is a kv-watch of every %s key or a core NATS subject",
				port.Name, port.Config.Kind(), graph.BucketEntityStates))
	}
}

// resolveOutput accepts exactly one http network output on a port from 1024 to 65535;
// port 0 is refused because the address a client dials must be known before Start.
func resolveOutput(definitions []component.PortDefinition) (component.Port, error) {
	if len(definitions) != 1 {
		return component.Port{}, errs.WrapInvalid(errs.ErrInvalidConfig, "websocket", "factory",
			fmt.Sprintf("exactly one output port is required, the http network port the server listens on; got %d",
				len(definitions)))
	}
	port, err := definitions[0].Resolve(component.DirectionOutput)
	if err != nil {
		return component.Port{}, errs.WrapInvalid(err, "websocket", "factory", "resolve output port")
	}
	network, ok := port.Config.(component.NetworkPort)
	if !ok {
		return component.Port{}, errs.WrapInvalid(errs.ErrInvalidConfig, "websocket", "factory",
			fmt.Sprintf("output port %q is a %s port; it must be a network port", port.Name, port.Config.Kind()))
	}
	if network.Protocol != "http" {
		return component.Port{}, errs.WrapInvalid(errs.ErrInvalidConfig, "websocket", "factory",
			fmt.Sprintf("output port %q protocol %q is not http", port.Name, network.Protocol))
	}
	if network.Port < 1024 || network.Port > 65535 {
		return component.Port{}, errs.WrapInvalid(errs.ErrInvalidConfig, "websocket", "factory",
			fmt.Sprintf("output port %q: port %d is out of range 1024-65535", port.Name, network.Port))
	}
	return port, nil
}

// validatePath refuses a path that is not a path-only ServeMux pattern.
func validatePath(path string) (err error) {
	if path == "" {
		return errors.New("path cannot be empty")
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("path %q must begin with /", path)
	}
	for index := 0; index < len(path); index++ {
		if path[index] <= ' ' || path[index] == 0x7f {
			return fmt.Errorf("path %q contains ASCII whitespace or a control character at byte %d", path, index)
		}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("path %q is not a valid HTTP ServeMux pattern: %v", path, recovered)
		}
	}()
	http.NewServeMux().HandleFunc(path, func(http.ResponseWriter, *http.Request) {})
	return nil
}
