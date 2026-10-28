package objectstore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/types"
)

// instanceName is the one StorageInstance the Component stamps into every reference and offers
// its Store under, and the instance of its store-provide port. The factory receives no instance
// name, and consumers resolve references by this literal; a second Component in one process is
// refused at Start by the store registry (design D7, owner ruling 3).
const instanceName = "objectstore"

// storedPort is the one output the Component publishes on: a StoredMessage per stored body.
const storedPort = "stored"

// componentConfig is the Component's configuration. It is decoded strictly: any other key is
// refused, naming it.
type componentConfig struct {
	// BucketName is the NATS ObjectStore bucket. Empty means "MESSAGES".
	BucketName string `json:"bucket_name" schema:"type:string,description:NATS ObjectStore bucket name,default:MESSAGES,category:basic"`

	// Ports are the input and output ports. Absent means the default write input and stored
	// output; present means exactly the ports written.
	Ports *component.PortConfig `json:"ports" schema:"type:ports,description:Port configuration for inputs and outputs,category:basic"`
}

// schema is the configuration schema Register declares.
var schema = component.GenerateConfigSchema(reflect.TypeOf(componentConfig{}))

// defaultPorts returns the ports of a configuration with no ports key: the write input on core
// NATS and the stored output carrying storage.stored.v1.
func defaultPorts() *component.PortConfig {
	return &component.PortConfig{
		Inputs: []component.PortDefinition{{
			Name:        "write",
			Config:      component.NATSPort{Subject: "storage.objectstore.write"},
			Description: "Messages whose ContentStorable body is stored",
		}},
		Outputs: []component.PortDefinition{{
			Name: storedPort,
			Config: component.NATSPort{
				Subject:   "storage.objectstore.stored",
				Interface: &component.InterfaceContract{Type: "storage.stored.v1"},
			},
			Description: "A StoredMessage carrying each stored body's reference",
		}},
	}
}

// Component stores the bodies of the messages on its inputs in a NATS ObjectStore bucket and
// publishes a StoredMessage carrying each body's reference on its stored output (ADR-063).
//
// This unit carries construction only (tasks 3.1, 3.2): configuration, ports and registration.
// Start, Stop and the write lane come with tasks 3.3 and 3.4.
type Component struct {
	config  componentConfig
	inputs  []component.Port
	outputs []component.Port

	// lifecycle is the lifecycle state, Health and DataFlow (component.Lifecycle, ADR-108).
	lifecycle component.Lifecycle
}

var _ component.Discoverable = (*Component)(nil)

// Register registers the objectstore factory with registry.
func Register(registry *component.Registry) error {
	return registry.Register(component.Registration{
		Name:        "objectstore",
		Type:        types.ComponentTypeStorage,
		Protocol:    "objectstore",
		Domain:      "storage",
		Description: "Stores message bodies in a NATS ObjectStore bucket and publishes their references",
		Version:     "1.0.0",
		Schema:      schema,
		Factory:     newComponent,
		Ports:       declarePorts,
	})
}

// declarePorts is the component.PortDeclarer for objectstore: the ports newComponent builds for
// rawConfig. The instance name is not used: the store-provide port always names instanceName.
func declarePorts(rawConfig json.RawMessage, _ string) (component.PortConfig, error) {
	_, inputs, outputs, err := resolveConfig(rawConfig)
	if err != nil {
		return component.PortConfig{}, err
	}
	return component.PortConfigFrom(inputs, outputs), nil
}

// newComponent is the objectstore factory. It validates the configuration and resolves the
// ports; it opens no connection.
func newComponent(rawConfig json.RawMessage, _ component.Dependencies) (component.Discoverable, error) {
	config, inputs, outputs, err := resolveConfig(rawConfig)
	if err != nil {
		return nil, err
	}
	return &Component{config: config, inputs: inputs, outputs: outputs}, nil
}

// resolveConfig decodes rawConfig strictly, applies the defaults and resolves the ports. It is the
// one derivation declarePorts and newComponent share.
func resolveConfig(rawConfig json.RawMessage) (componentConfig, []component.Port, []component.Port, error) {
	var config componentConfig
	if len(rawConfig) > 0 {
		// A port definition decodes through its own UnmarshalJSON, which this setting does not
		// reach; it refuses unknown keys itself.
		decoder := json.NewDecoder(bytes.NewReader(rawConfig))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&config); err != nil {
			return componentConfig{}, nil, nil, errs.WrapInvalid(err, "objectstore", "newComponent", "config unmarshal")
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return componentConfig{}, nil, nil, errs.WrapInvalid(errors.New("data after the configuration object"),
				"objectstore", "newComponent", "config unmarshal")
		}
	}
	if config.BucketName == "" {
		config.BucketName = defaultBucketName
	}
	if config.Ports == nil {
		config.Ports = defaultPorts()
	}
	inputs, outputs, err := resolvePorts(*config.Ports)
	if err != nil {
		return componentConfig{}, nil, nil, errs.WrapInvalid(err, "objectstore", "newComponent", "resolve ports")
	}
	return config, inputs, outputs, nil
}

// resolvePorts resolves the configured ports and appends the store-provide output. Each port is
// core NATS or JetStream on one subject, and names are unique. The only output is stored, and an
// input needs it: a body stored with no reference published is addressable by nobody (design D3).
func resolvePorts(ports component.PortConfig) ([]component.Port, []component.Port, error) {
	seen := make(map[string]bool, len(ports.Inputs)+len(ports.Outputs))
	resolve := func(definitions []component.PortDefinition, direction component.Direction) ([]component.Port, error) {
		resolved := make([]component.Port, 0, len(definitions))
		for _, definition := range definitions {
			if direction == component.DirectionInput && definition.Name == "api" {
				return nil, errors.New(`input "api" was removed; read through the registered Store`)
			}
			if direction == component.DirectionOutput && definition.Name != storedPort {
				return nil, fmt.Errorf("output %q is not supported: the only output is %q", definition.Name, storedPort)
			}
			port, err := definition.Resolve(direction)
			if err != nil {
				return nil, err
			}
			if seen[port.Name] {
				return nil, fmt.Errorf("duplicate port name %q", port.Name)
			}
			seen[port.Name] = true
			facts, err := port.Facts()
			if err != nil {
				return nil, err
			}
			if kind := facts.Kind(); kind != component.PortKindNATS && kind != component.PortKindJetStream {
				return nil, fmt.Errorf("port %q kind %q is not supported", port.Name, kind)
			}
			if subjects := facts.NATSSubjects(); len(subjects) != 1 {
				return nil, fmt.Errorf("port %q declares %d subjects, want one", port.Name, len(subjects))
			}
			resolved = append(resolved, port)
		}
		return resolved, nil
	}
	inputs, err := resolve(ports.Inputs, component.DirectionInput)
	if err != nil {
		return nil, nil, err
	}
	outputs, err := resolve(ports.Outputs, component.DirectionOutput)
	if err != nil {
		return nil, nil, err
	}
	if len(inputs) > 0 && len(outputs) == 0 {
		return nil, nil, fmt.Errorf("inputs need the %q output, which publishes each stored body's reference", storedPort)
	}
	provider, err := component.PortDefinition{
		Name:        "store-provide",
		Description: "Owns the store addressable as StorageInstance=" + instanceName,
		Config:      component.StoreProvidePort{Instance: instanceName},
	}.Resolve(component.DirectionOutput)
	if err != nil {
		return nil, nil, err
	}
	return inputs, append(outputs, provider), nil
}

// InputPorts returns the input ports, fixed at construction.
func (c *Component) InputPorts() []component.Port {
	return slices.Clone(c.inputs)
}

// OutputPorts returns the output ports, fixed at construction: stored, if configured, and
// store-provide.
func (c *Component) OutputPorts() []component.Port {
	return slices.Clone(c.outputs)
}

// Health reports the lifecycle shell's verdict.
func (c *Component) Health() component.HealthStatus {
	return c.lifecycle.Health()
}

// DataFlow reports the lifecycle shell's counts.
func (c *Component) DataFlow() component.FlowMetrics {
	return c.lifecycle.DataFlow()
}
