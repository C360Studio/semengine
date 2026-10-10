package component

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"sync"

	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/types"
)

// Factory creates a component instance from configuration following service pattern
// The factory function receives raw JSON configuration and dependencies, parses its own config,
// and returns a properly initialized component that implements the Discoverable interface.
// All I/O operations should be performed in the component's Start() method, not in the factory.
// This pattern matches service constructors: func(rawConfig json.RawMessage, deps Dependencies) (Service, error)
type Factory func(rawConfig json.RawMessage, deps Dependencies) (Discoverable, error)

// PortDeclarer declares a factory's ports as a pure function of the raw
// component configuration and the instance name. It takes no dependencies,
// performs no I/O, and constructs nothing: it is the static fact a
// registration carries beside Schema, evaluated offline by composition
// validation and verified against the constructed component at boot
// admission. The constructor MUST derive its ports from the same function so
// the two cannot drift.
type PortDeclarer func(rawConfig json.RawMessage, instanceName string) (PortConfig, error)

// Dependency identifiers used by Registration.Dependencies. Components
// declare these at registration to opt into framework-driven behavior
// (e.g., restart when a named runtime dependency changes). Kept as
// typed constants so refactors are compiler-checked.
const (
	// DepModelRegistry signals that a component consumes
	// Dependencies.ModelRegistry at construction or run time. Components
	// that declare this consume the model registry selected at boot.
	// Later model_registry writes require process restart.
	DepModelRegistry = "model-registry"
)

// Registration holds factory and metadata for a component type
type Registration struct {
	Name         string       `json:"name"`         // Factory name (e.g., "udp-input")
	Type         string       `json:"type"`         // Component type (input/processor/output/storage)
	Protocol     string       `json:"protocol"`     // Technical protocol (udp, mavlink, websocket, etc.)
	Domain       string       `json:"domain"`       // Business domain (robotics, semantic, network, storage)
	Description  string       `json:"description"`  // Human-readable description
	Version      string       `json:"version"`      // Component version
	Schema       ConfigSchema `json:"schema"`       // Schema as static metadata (Feature 011)
	Factory      Factory      `json:"-"`            // Factory function (not serializable)
	Ports        PortDeclarer `json:"-"`            // Static port declaration (not serializable)
	Dependencies []string     `json:"dependencies"` // Boot dependencies such as DepModelRegistry
}

// RegistrationConfig provides a clean API for component registration.
// This config struct replaces the previous 7-8 parameter function signatures.
// It maps 1:1 to Registration struct fields for simplicity.
type RegistrationConfig struct {
	Name         string       // Component name (e.g., "udp", "websocket", "graph-processor")
	Factory      Factory      // Factory function to create component instances
	Ports        PortDeclarer // Pure port declaration the constructor's ports must equal
	Schema       ConfigSchema // Configuration schema for validation and discovery
	Type         string       // Component type: "input", "processor", "output", "storage"
	Protocol     string       // Technical protocol (udp, tcp, websocket, file, etc.)
	Domain       string       // Business domain (network, storage, processing, robotics, semantic)
	Description  string       // Human-readable description of the component
	Version      string       // Component version (semver recommended)
	Dependencies []string     // Runtime deps declared via constants like DepModelRegistry
}

// Declaration is the immutable port shape of one component instance: what the
// Registry retains for an admitted component, and what Declare produces for a
// configured-but-unconstructed one. It carries no lifecycle, health,
// readiness, grouping, or orchestration state.
type Declaration struct {
	InstanceName       string
	FactoryIdentity    string
	ComponentType      types.ComponentType
	InputPorts         []Port
	OutputPorts        []Port
	InputFacts         []PortFacts
	OutputFacts        []PortFacts
	ExclusiveResources []string
}

// preparedComponent exists only between factory construction and boot
// admission. The Registry retains the immutable declaration, never the live
// component handle.
type preparedComponent struct {
	declaration Declaration
	component   Discoverable
}

// declarationSnapshot is the defensive read view of one admitted component
// declaration, returned by Snapshots for the component manager. The type is
// unexported, so a consumer cannot name it.
type declarationSnapshot struct {
	record Declaration
}

func (s declarationSnapshot) Name() string { return s.record.InstanceName }

func (s declarationSnapshot) Inputs() []Port {
	return cloneResolvedPorts(s.record.InputPorts)
}

func (s declarationSnapshot) Outputs() []Port {
	return cloneResolvedPorts(s.record.OutputPorts)
}

func (s declarationSnapshot) InputDeclarationFacts() []PortFacts {
	return clonePortFactsSlice(s.record.InputFacts)
}

func (s declarationSnapshot) OutputDeclarationFacts() []PortFacts {
	return clonePortFactsSlice(s.record.OutputFacts)
}

// Declaration returns a defensive clone of the admitted declaration value.
func (s declarationSnapshot) Declaration() Declaration {
	return cloneComponentDeclaration(s.record)
}

// Registry manages component factories and immutable admitted declarations.
// Runtime component handles remain private to ComponentManager.
type Registry struct {
	factories    map[string]*Registration // Factory registry by name
	declarations map[string]Declaration   // Admitted declaration by instance name
	mu           sync.RWMutex             // Protects all registry state

	sealed bool
}

// NewRegistry creates a new empty component registry
func NewRegistry() *Registry {
	return &Registry{
		factories:    make(map[string]*Registration),
		declarations: make(map[string]Declaration),
	}
}

// RegisterFactory registers a component factory with the given name
// Returns an error if a factory with the same name is already registered.
func (r *Registry) RegisterFactory(name string, registration *Registration) error {
	if name == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Registry", "RegisterFactory", "factory name validation")
	}
	if registration == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Registry", "RegisterFactory", "registration validation")
	}
	if registration.Factory == nil {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Registry", "RegisterFactory", "factory function validation")
	}
	if registration.Type == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "Registry", "RegisterFactory", "component type validation")
	}
	if registration.Ports == nil {
		return errs.WrapInvalid(
			fmt.Errorf("factory %q declares no ports: set Registration.Ports to the pure port declarer the constructor derives its ports from", name),
			"Registry", "RegisterFactory", "port declarer validation")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[name]; exists {
		msg := fmt.Errorf("factory '%s' is already registered", name)
		return errs.WrapInvalid(msg, "Registry", "RegisterFactory", "duplicate factory check")
	}

	r.factories[name] = registration
	return nil
}

// CreateComponent is the boot-admission seam. It exists for the component
// manager (service.ComponentManager): a consumer creates and composes
// components through that manager, and a direct call bypasses the manager's
// bookkeeping. prepare must finish all fallible owner-local setup before the
// Registry publishes the immutable declaration.
func (r *Registry) CreateComponent(
	instanceName string,
	config types.ComponentConfig,
	deps Dependencies,
	prepare func(Discoverable) error,
) (Discoverable, error) {
	r.mu.RLock()
	sealed := r.sealed
	r.mu.RUnlock()
	if sealed {
		return nil, errs.WrapInvalid(
			errs.ErrInvalidConfig,
			"Registry", "CreateComponent", "component composition is sealed for this process",
		)
	}
	prepared, err := r.prepareComponent("CreateComponent", instanceName, config, deps)
	if err != nil {
		return nil, err
	}
	if prepare != nil {
		if err := prepare(prepared.component); err != nil {
			return nil, errs.Wrap(err, "Registry", "CreateComponent", "prepare managed component")
		}
	}
	if err := r.admitPrepared(prepared.declaration); err != nil {
		return nil, errs.Wrap(err, "Registry", "CreateComponent", "instance registration")
	}
	return prepared.component, nil
}

func (r *Registry) prepareComponent(
	operation, instanceName string, config types.ComponentConfig, deps Dependencies,
) (preparedComponent, error) {
	// Security: Validate instance name
	if err := ValidateComponentName(instanceName); err != nil {
		return preparedComponent{}, errs.Wrap(err, "Registry", operation, "instance name validation")
	}
	if !config.Enabled {
		return preparedComponent{}, errs.WrapInvalid(
			errs.ErrInvalidConfig, "Registry", operation, "disabled component admission")
	}
	if config.Type == "" {
		return preparedComponent{}, errs.WrapInvalid(
			errs.ErrInvalidConfig, "Registry", operation, "component type validation")
	}
	// Security: Validate factory name
	if err := ValidateComponentName(config.Name); err != nil {
		return preparedComponent{}, errs.Wrap(err, "Registry", operation, "factory name validation")
	}
	if deps.NATSClient == nil {
		return preparedComponent{}, errs.WrapInvalid(errs.ErrInvalidConfig, "Registry", operation, "NATS client validation")
	}

	// CRITICAL SECURITY: Comprehensive validation before factory execution
	// This prevents injection attacks, resource exhaustion, and malformed input
	if err := ValidateFactoryConfig(config.Config); err != nil {
		return preparedComponent{}, errs.Wrap(err, "Registry", operation, "config security validation")
	}

	// Look up factory by the component/factory name (e.g., "udp", "websocket")
	r.mu.RLock()
	registration, exists := r.factories[config.Name]
	r.mu.RUnlock()

	if !exists {
		msg := fmt.Errorf("unknown component factory '%s'", config.Name)
		return preparedComponent{}, errs.WrapInvalid(msg, "Registry", operation, "factory lookup")
	}

	// Validate that the factory type matches the requested type
	if registration.Type != string(config.Type) {
		msg := fmt.Errorf("component '%s' is type '%s', not '%s'",
			config.Name, registration.Type, config.Type)
		return preparedComponent{}, errs.WrapInvalid(msg, "Registry", operation, "type validation")
	}

	// Create the component using the factory with service pattern
	// Pass the component-specific config (config.Config) to the factory
	component, err := registration.Factory(config.Config, deps)
	if err != nil {
		return preparedComponent{}, errs.Wrap(err, "Registry", operation, "factory execution")
	}

	// Defensive check: factory should never return (nil, nil)
	if component == nil {
		return preparedComponent{}, errs.WrapInvalid(errs.ErrInvalidConfig, "Registry", operation,
			"factory returned nil component without error")
	}

	prepared, err := captureComponentDeclaration(instanceName, config, component)
	if err != nil {
		return preparedComponent{}, errs.Wrap(err, "Registry", operation, "capture declaration")
	}
	// P1 parity: the static declaration must equal what the constructed
	// component reports, port for port. A disagreement is a lying declarer or
	// a drifted constructor; either fails admission here, naming the factory,
	// the instance, and the first differing port.
	declared, err := declare(registration, instanceName, config)
	if err != nil {
		return preparedComponent{}, errs.WrapInvalid(
			fmt.Errorf("factory %q instance %q: port declarer failed: %w", config.Name, instanceName, err),
			"Registry", operation, "port declaration parity")
	}
	if err := comparePortDeclarations(declared, prepared); err != nil {
		return preparedComponent{}, errs.WrapInvalid(
			fmt.Errorf("factory %q instance %q: %w", config.Name, instanceName, err),
			"Registry", operation, "port declaration parity")
	}
	return preparedComponent{declaration: prepared, component: component}, nil
}

// Declare evaluates the named factory's port declarer for one configured
// instance and resolves the result through the canonical port resolver. It
// constructs nothing and needs no dependencies: this is the offline half of
// the declaration the Registry verifies at admission.
func (r *Registry) Declare(instanceName string, config types.ComponentConfig) (Declaration, error) {
	if err := ValidateComponentName(instanceName); err != nil {
		return Declaration{}, errs.Wrap(err, "Registry", "Declare", "instance name validation")
	}
	if config.Type == "" {
		return Declaration{}, errs.WrapInvalid(errs.ErrInvalidConfig, "Registry", "Declare", "component type validation")
	}
	if err := ValidateComponentName(config.Name); err != nil {
		return Declaration{}, errs.Wrap(err, "Registry", "Declare", "factory name validation")
	}
	if err := ValidateFactoryConfig(config.Config); err != nil {
		return Declaration{}, errs.Wrap(err, "Registry", "Declare", "config security validation")
	}
	r.mu.RLock()
	registration, exists := r.factories[config.Name]
	r.mu.RUnlock()
	if !exists {
		return Declaration{}, errs.WrapInvalid(
			fmt.Errorf("unknown component factory '%s'", config.Name), "Registry", "Declare", "factory lookup")
	}
	if registration.Type != string(config.Type) {
		return Declaration{}, errs.WrapInvalid(
			fmt.Errorf("component '%s' is type '%s', not '%s'", config.Name, registration.Type, config.Type),
			"Registry", "Declare", "type validation")
	}
	declared, err := declare(registration, instanceName, config)
	if err != nil {
		return Declaration{}, errs.WrapInvalid(err, "Registry", "Declare", "port declaration")
	}
	return declared, nil
}

// declare runs a registration's declarer and resolves every definition through
// resolveAndProjectPort, the single interpreter of a port declaration.
func declare(registration *Registration, instanceName string, config types.ComponentConfig) (Declaration, error) {
	ports, err := registration.Ports(config.Config, instanceName)
	if err != nil {
		return Declaration{}, err
	}
	inputs, inputFacts, err := resolveDefinitions(ports.Inputs, DirectionInput)
	if err != nil {
		return Declaration{}, fmt.Errorf("declared input ports: %w", err)
	}
	outputs, outputFacts, err := resolveDefinitions(ports.Outputs, DirectionOutput)
	if err != nil {
		return Declaration{}, fmt.Errorf("declared output ports: %w", err)
	}
	return Declaration{
		InstanceName:       instanceName,
		FactoryIdentity:    config.Name,
		ComponentType:      config.Type,
		InputPorts:         inputs,
		OutputPorts:        outputs,
		InputFacts:         inputFacts,
		OutputFacts:        outputFacts,
		ExclusiveResources: exclusiveResources(inputFacts, outputFacts),
	}, nil
}

func resolveDefinitions(definitions []PortDefinition, direction Direction) ([]Port, []PortFacts, error) {
	ports := make([]Port, len(definitions))
	facts := make([]PortFacts, len(definitions))
	for index, definition := range definitions {
		resolved, projected, err := resolveAndProjectPort(definition, direction)
		if err != nil {
			return nil, nil, err
		}
		ports[index] = resolved
		facts[index] = projected
	}
	return ports, facts, nil
}

// comparePortDeclarations reports the first port on which the declared and
// the constructed declarations disagree: name, direction, required, kind,
// resource identity, NATS subjects, or interface contract, in order.
func comparePortDeclarations(declared, constructed Declaration) error {
	if err := comparePortLane(DirectionInput, declared.InputPorts, declared.InputFacts, constructed.InputPorts, constructed.InputFacts); err != nil {
		return err
	}
	return comparePortLane(DirectionOutput, declared.OutputPorts, declared.OutputFacts, constructed.OutputPorts, constructed.OutputFacts)
}

func comparePortLane(
	direction Direction,
	declared []Port, declaredFacts []PortFacts,
	constructed []Port, constructedFacts []PortFacts,
) error {
	for index := 0; index < len(declared) || index < len(constructed); index++ {
		switch {
		case index >= len(declared):
			return fmt.Errorf("port %q: constructed %s port not declared (declared %d %s ports, constructed %d)",
				constructed[index].Name, direction, len(declared), direction, len(constructed))
		case index >= len(constructed):
			return fmt.Errorf("port %q: declared %s port not constructed (declared %d %s ports, constructed %d)",
				declared[index].Name, direction, len(declared), direction, len(constructed))
		}
		if reason := portDifference(declared[index], declaredFacts[index], constructed[index], constructedFacts[index]); reason != "" {
			return fmt.Errorf("port %q: %s", declared[index].Name, reason)
		}
	}
	return nil
}

func portDifference(declared Port, declaredFacts PortFacts, constructed Port, constructedFacts PortFacts) string {
	switch {
	case declared.Name != constructed.Name:
		return fmt.Sprintf("declared name %q, constructed %q", declared.Name, constructed.Name)
	case declared.Direction != constructed.Direction:
		return fmt.Sprintf("declared direction %q, constructed %q", declared.Direction, constructed.Direction)
	case declared.Required != constructed.Required:
		return fmt.Sprintf("declared required=%v, constructed %v", declared.Required, constructed.Required)
	case declared.External != constructed.External:
		return fmt.Sprintf("declared external=%v, constructed %v", declared.External, constructed.External)
	case declaredFacts.Kind() != constructedFacts.Kind():
		return fmt.Sprintf("declared kind %q, constructed %q", declaredFacts.Kind(), constructedFacts.Kind())
	case declaredFacts.ResourceID() != constructedFacts.ResourceID():
		return fmt.Sprintf("declared resource %q, constructed %q", declaredFacts.ResourceID(), constructedFacts.ResourceID())
	case !slices.Equal(declaredFacts.NATSSubjects(), constructedFacts.NATSSubjects()):
		return fmt.Sprintf("declared subjects %v, constructed %v", declaredFacts.NATSSubjects(), constructedFacts.NATSSubjects())
	}
	declaredContract, declaredHas := declaredFacts.Interface()
	constructedContract, constructedHas := constructedFacts.Interface()
	if declaredHas != constructedHas || declaredContract.Type != constructedContract.Type ||
		declaredContract.Version != constructedContract.Version ||
		!slices.Equal(declaredContract.Compatible, constructedContract.Compatible) {
		return fmt.Sprintf("declared interface %v %+v, constructed %v %+v", declaredHas, declaredContract, constructedHas, constructedContract)
	}
	return ""
}

func (r *Registry) admitPrepared(prepared Declaration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed {
		return errs.WrapInvalid(
			errs.ErrInvalidConfig,
			"Registry", "admitPrepared", "component composition is sealed for this process",
		)
	}

	_, exists := r.declarations[prepared.InstanceName]
	if exists {
		return errs.WrapInvalid(
			fmt.Errorf("instance '%s' is already registered", prepared.InstanceName),
			"Registry", "admitPrepared", "duplicate instance check")
	}
	if err := r.checkResourceConflictsLocked(prepared.InstanceName, prepared.ExclusiveResources); err != nil {
		return errs.Wrap(err, "Registry", "admitPrepared", "resource conflict check")
	}
	r.declarations[prepared.InstanceName] = prepared
	return nil
}

// SealComposition closes boot admission for the current process. It exists for
// the component manager (service.ComponentManager), which seals the composition
// once boot admission is complete; a direct call bypasses the manager's
// bookkeeping.
func (r *Registry) SealComposition() {
	r.mu.Lock()
	r.sealed = true
	r.mu.Unlock()
}

// ListFactories returns all registered component factories
// This provides information about what types of components can be created.
func (r *Registry) ListFactories() map[string]*Registration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]*Registration, len(r.factories))
	for name, registration := range r.factories {
		// Create a copy of the registration without the factory function
		// to avoid potential issues with function pointers
		result[name] = &Registration{
			Name:         registration.Name,
			Type:         registration.Type,
			Protocol:     registration.Protocol,
			Domain:       registration.Domain,
			Description:  registration.Description,
			Version:      registration.Version,
			Schema:       cloneConfigSchema(registration.Schema),
			Ports:        registration.Ports, // pure function; no handle to leak
			Dependencies: append([]string(nil), registration.Dependencies...),
			// Factory is intentionally not copied for safety
		}
	}

	return result
}

func cloneConfigSchema(schema ConfigSchema) ConfigSchema {
	clone := ConfigSchema{Required: append([]string(nil), schema.Required...)}
	if schema.Properties != nil {
		clone.Properties = make(map[string]PropertySchema, len(schema.Properties))
		for name, property := range schema.Properties {
			clone.Properties[name] = clonePropertySchema(property)
		}
	}
	return clone
}

func clonePropertySchema(property PropertySchema) PropertySchema {
	clone := property
	clone.Enum = append([]string(nil), property.Enum...)
	clone.Required = append([]string(nil), property.Required...)
	if property.Minimum != nil {
		value := *property.Minimum
		clone.Minimum = &value
	}
	if property.Maximum != nil {
		value := *property.Maximum
		clone.Maximum = &value
	}
	if property.MinLength != nil {
		value := *property.MinLength
		clone.MinLength = &value
	}
	if property.MaxLength != nil {
		value := *property.MaxLength
		clone.MaxLength = &value
	}
	if property.AdditionalProperties != nil {
		value := *property.AdditionalProperties
		clone.AdditionalProperties = &value
	}
	if property.Items != nil {
		value := clonePropertySchema(*property.Items)
		clone.Items = &value
	}
	if property.Properties != nil {
		clone.Properties = make(map[string]PropertySchema, len(property.Properties))
		for name, nested := range property.Properties {
			clone.Properties[name] = clonePropertySchema(nested)
		}
	}
	return clone
}

// RegisterWithConfig registers a component using a configuration struct.
// This is the recommended registration method that replaces the multi-parameter functions.
//
// Example usage:
//
//	registry.RegisterWithConfig(component.RegistrationConfig{
//	    Name:        "udp",
//	    Factory:     CreateUDPInput,
//	    Schema:      udpSchema,
//	    Type:        "input",
//	    Protocol:    "udp",
//	    Domain:      "network",
//	    Description: "UDP input component for receiving network data",
//	    Version:     "1.0.0",
//	})
func (r *Registry) RegisterWithConfig(config RegistrationConfig) error {
	registration := &Registration{
		Name:         config.Name,
		Factory:      config.Factory,
		Ports:        config.Ports,
		Schema:       config.Schema,
		Type:         config.Type,
		Protocol:     config.Protocol,
		Domain:       config.Domain,
		Description:  config.Description,
		Version:      config.Version,
		Dependencies: config.Dependencies,
	}

	return r.RegisterFactory(config.Name, registration)
}

// Config validation constants - security limits
const (
	MaxStringLength = 1024        // Maximum length for string values
	MaxJSONSize     = 1024 * 1024 // Maximum JSON size (1MB)
)

// ValidateComponentName validates component/instance names for security
func ValidateComponentName(name string) error {
	if name == "" {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "ConfigValidator", "ValidateComponentName", "empty name")
	}
	if len(name) > MaxStringLength {
		return errs.WrapInvalid(errs.ErrInvalidConfig, "ConfigValidator", "ValidateComponentName", "name too long")
	}
	// Check for potentially dangerous characters - allow alphanumeric, dash, underscore , dot
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return errs.WrapInvalid(
				errs.ErrInvalidConfig, "ConfigValidator", "ValidateComponentName",
				"invalid name characters")
		}
	}
	return nil
}

func (r *Registry) checkResourceConflictsLocked(instanceName string, resources []string) error {
	for existingName, declaration := range r.declarations {
		if existingName == instanceName {
			continue
		}
		for _, existingResource := range declaration.ExclusiveResources {
			for _, resource := range resources {
				if resource == existingResource {
					msg := fmt.Errorf("resource conflict: %s already used by component '%s'", resource, existingName)
					return errs.WrapInvalid(msg, "Registry", "checkResourceConflicts", "exclusive resource check")
				}
			}
		}
	}
	return nil
}

func captureComponentDeclaration(
	instanceName string, config types.ComponentConfig, discoverable Discoverable,
) (Declaration, error) {
	inputs, inputFacts, err := cloneAndProjectPorts(discoverable.InputPorts())
	if err != nil {
		return Declaration{}, err
	}
	outputs, outputFacts, err := cloneAndProjectPorts(discoverable.OutputPorts())
	if err != nil {
		return Declaration{}, err
	}
	return Declaration{
		InstanceName:       instanceName,
		FactoryIdentity:    config.Name,
		ComponentType:      config.Type,
		InputPorts:         inputs,
		OutputPorts:        outputs,
		InputFacts:         inputFacts,
		OutputFacts:        outputFacts,
		ExclusiveResources: exclusiveResources(inputFacts, outputFacts),
	}, nil
}

func exclusiveResources(inputFacts, outputFacts []PortFacts) []string {
	resources := make([]string, 0, len(inputFacts)+len(outputFacts))
	for _, facts := range append(append([]PortFacts(nil), inputFacts...), outputFacts...) {
		if facts.IsExclusive() {
			resources = append(resources, facts.ResourceID())
		}
	}
	sort.Strings(resources)
	return compactStrings(resources)
}

func cloneAndProjectPorts(ports []Port) ([]Port, []PortFacts, error) {
	cloned := make([]Port, len(ports))
	facts := make([]PortFacts, len(ports))
	for index, port := range ports {
		resolved, projected, err := resolveAndProjectPort(definitionFromPort(port), port.Direction)
		if err != nil {
			return nil, nil, err
		}
		cloned[index] = resolved
		facts[index] = projected
	}
	return cloned, facts, nil
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	write := 1
	for read := 1; read < len(values); read++ {
		if values[read] == values[write-1] {
			continue
		}
		values[write] = values[read]
		write++
	}
	return values[:write]
}

// declaration returns a defensive clone of one admitted component declaration.
func (r *Registry) declaration(instanceName string) (Declaration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	declaration, ok := r.declarations[instanceName]
	if !ok {
		return Declaration{}, false
	}
	return cloneComponentDeclaration(declaration), true
}

// declarationsSnapshot returns a deterministic defensive clone of the complete
// boot admission set.
func (r *Registry) declarationsSnapshot() []Declaration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.declarationsLocked()
}

// Snapshots returns a deterministic defensive copy of the complete admission
// set. It exists for the component manager (service.ComponentManager): a
// consumer reads admitted components through that manager, and a direct call
// bypasses the manager's bookkeeping.
//
//revive:disable:unexported-return The component manager ranges this opaque snapshot set; its record is not consumer surface.
func (r *Registry) Snapshots() []declarationSnapshot {
	declarations := r.declarationsSnapshot()
	result := make([]declarationSnapshot, len(declarations))
	for index, declaration := range declarations {
		result[index] = declarationSnapshot{record: declaration}
	}
	return result
}

//revive:enable:unexported-return

func (r *Registry) declarationsLocked() []Declaration {
	names := make([]string, 0, len(r.declarations))
	for name := range r.declarations {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]Declaration, 0, len(names))
	for _, name := range names {
		result = append(result, cloneComponentDeclaration(r.declarations[name]))
	}
	return result
}

func cloneComponentDeclaration(declaration Declaration) Declaration {
	clone := declaration
	clone.InputPorts = cloneResolvedPorts(declaration.InputPorts)
	clone.OutputPorts = cloneResolvedPorts(declaration.OutputPorts)
	clone.InputFacts = clonePortFactsSlice(declaration.InputFacts)
	clone.OutputFacts = clonePortFactsSlice(declaration.OutputFacts)
	clone.ExclusiveResources = append([]string(nil), declaration.ExclusiveResources...)
	return clone
}

func cloneResolvedPorts(ports []Port) []Port {
	cloned := make([]Port, len(ports))
	for index, port := range ports {
		resolved, _, err := resolveAndProjectPort(definitionFromPort(port), port.Direction)
		if err != nil {
			panic(fmt.Sprintf("clone retained port %q: %v", port.Name, err))
		}
		cloned[index] = resolved
	}
	return cloned
}

func clonePortFactsSlice(facts []PortFacts) []PortFacts {
	cloned := make([]PortFacts, len(facts))
	for index, fact := range facts {
		cloned[index] = clonePortFacts(fact)
	}
	return cloned
}

func clonePortFacts(facts PortFacts) PortFacts {
	clone := facts
	clone.interfaceContract = cloneInterfaceContract(facts.interfaceContract)
	clone.connectionIDs = append([]string(nil), facts.connectionIDs...)
	clone.natsSubjects = append([]string(nil), facts.natsSubjects...)
	if facts.stream != nil {
		stream := *facts.stream
		stream.subjects = append([]string(nil), facts.stream.subjects...)
		clone.stream = &stream
	}
	if facts.network != nil {
		network := *facts.network
		clone.network = &network
	}
	return clone
}

// Note: Component registration functions have been removed.
// Components now use explicit Register(*Registry) methods for registration.
//
// Payload registration moved to the payloadregistry package
// (alongside PayloadRegistration, PayloadRegistry, the global
// singleton, and Register/Create/Build helpers). Migration is a
// straightforward import-path swap; the surface is preserved.
