package service

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/composition"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/internal/lifecycleguard"
	"github.com/c360studio/semengine/model"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/payloadregistry"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/security"
	"github.com/c360studio/semengine/storage/storeregistry"
	"github.com/c360studio/semengine/types"
)

// ComponentManager owns the boot composition's components (inputs, processors,
// outputs, storage) for the life of the process.
//
// NewComponentManager composes the set once, from the configuration manager's
// boot configuration: it builds every enabled component, analyses the
// composition, and seals the component registry. The set is fixed from then on.
// Start starts it once, store providers first; Stop is the only stop, and stops
// the components in reverse start order (ADR-113).
type ComponentManager struct {
	*BaseService

	registry        *component.Registry
	payloadRegistry *payloadregistry.Registry
	natsClient      *natsclient.Client
	platform        types.PlatformMeta

	// The boot configuration, read once by the constructor (ADR-096 ¶4). Later
	// writes to the configuration manager are desired state for the next boot.
	// componentConfigs holds every configured component, disabled ones included.
	componentConfigs  config.ComponentConfigs
	bootSecurity      security.Config
	bootModelRegistry model.RegistryReader
	bootStreams       config.StreamConfigs // explicit `streams` of the boot configuration, for composition analysis

	// bootFindings is the composition analysis of the admitted boot set, computed
	// once by the constructor before the registry seals (ADR-100). The validate,
	// flowgraph and paths projections serve it as is. Nothing writes it after
	// construction, so it is read without a lock.
	bootFindings *composition.Result

	// components is the admitted set in the fixed start order (providers by name,
	// then the rest by name), and componentsByName the same records by instance
	// name. Both are built by the constructor and never written again (design D2,
	// "State shape"), so they are read without a lock.
	components       []*componentRecord
	componentsByName map[string]*componentRecord

	// storeRegistry is the shared {StorageInstance -> store} resolver (ADR-063):
	// filled from each provider's ProvidedStores after its Start and cleared
	// before its Stop, and handed to every component as
	// component.Dependencies.StoreRegistry.
	storeRegistry *storeregistry.Registry

	// overrideExpiry re-reports stream migration-override expiry against the LIVE
	// configuration while the process runs. It is hosted here, not by an optional
	// service, because the component manager is the one service no composition
	// can disable (MandatoryServiceDisabledError): an operator who declares a
	// bridge should not also have to declare the service that says it lapsed.
	// The constructor always sets it.
	overrideExpiry *streamOverrideExpiryReporter

	// lifecycle admits one Start and makes Stop the only stop (ADR-113): a failed
	// Start's rollback and every Stop run the one stop sequence through it.
	lifecycle lifecycleguard.Guard

	// stopBegun is set when a Stop begins its stop sequence: from then
	// startupStopBegun reports true and the service manager's /readyz answers
	// NOT READY.
	stopBegun atomic.Bool

	// The supervisor (the health publisher and the override-expiry reporter): its
	// run context's cancel, and the channel closed once both loops returned.
	// Start's function in the lifecycle guard writes them and the stop sequence
	// reads them; the guard orders the two, so neither needs a lock.
	supervisorCancel context.CancelFunc
	supervisorDone   chan struct{}
}

// componentRecord is one admitted component. The constructor fills every field
// but state, and nothing writes them again.
//
// Everything about the component that changes (its lifecycle state, its last
// error, its startup counts, the stores it registered, whether its Stop returned
// nil) is one componentState value behind state, replaced whole by update and
// never edited in place, so a reader loads it without waiting on anything.
//
// One goroutine at a time may replace a record's value, and two orderings are
// the only guard of that: each start phase joins every component's Start (whose
// goroutine is the one writer while it runs) before the next phase or the failed
// Start's rollback runs; and the lifecycle guard runs the rollback and the Stop
// sequence one after the other, refuses a concurrent Stop, and the sequence calls
// each component's Stop on the caller's stack, so a Stop cut short leaves no
// writer behind. Nothing else enforces it, and the race detector cannot see a
// breach: two stores to one atomic.Pointer are synchronised, so a lost update
// passes -race silently (design D2, "State shape").
type componentRecord struct {
	name      string
	component component.Discoverable
	// config is the exact boot configuration the instance was built from.
	config types.ComponentConfig
	// provider is whether the component is a component.StoreProvider, which puts
	// it in the first start phase.
	provider bool
	// startOrder is the record's position in ComponentManager.components.
	startOrder int

	state atomic.Pointer[componentState]
}

// componentState is the part of a component's record that changes after
// construction. A value is never edited once stored: componentRecord.update
// stores a changed copy.
type componentState struct {
	state     component.State
	lastError error

	// The component's startup counts, which startupSnapshot reports.
	startInvoked   bool
	startCompleted bool
	startFailed    bool

	// stores are the StorageInstance names registerProvidedStores registered
	// for the component, which deregisterProvidedStores removes.
	stores []string

	// stopped is set when the component's Stop returned nil; a later Stop does
	// not call it again.
	stopped bool
}

// current returns the record's state value. The caller must not edit it.
func (r *componentRecord) current() componentState {
	return *r.state.Load()
}

// update stores a copy of the record's state with change applied. Only the
// record's one writer calls it (see componentRecord).
func (r *componentRecord) update(change func(*componentState)) {
	next := r.current()
	next.stores = slices.Clone(next.stores)
	change(&next)
	r.state.Store(&next)
}

// managed returns a fresh component.ManagedComponent describing the record, so
// nothing a caller writes to it reaches the manager.
func (r *componentRecord) managed() *component.ManagedComponent {
	current := r.current()
	return &component.ManagedComponent{
		Component:  r.component,
		State:      current.state,
		Config:     r.config,
		StartOrder: r.startOrder,
		LastError:  current.lastError,
	}
}

// NewComponentManager decodes the component manager's configuration, which
// defines no key, and composes the boot component set (design D3). It refuses a
// configuration key, a missing configuration manager or component registry, a
// component that cannot be built (naming every one) and a composition with an
// error-severity finding; a refusal returns a nil Service.
func NewComponentManager(rawConfig json.RawMessage, deps *Dependencies) (Service, error) {
	var cfg ComponentManagerConfig
	if err := decodeStrictServiceJSON(rawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("parse component-manager config: %w", err)
	}

	if deps == nil || deps.Manager == nil {
		return nil, fmt.Errorf("component-manager requires config manager")
	}
	if deps.ComponentRegistry == nil {
		return nil, fmt.Errorf("component-manager requires a component registry")
	}

	// Runtime construction reads one snapshot of the configuration. Later
	// desired writes remain configuration-manager state for the next process.
	currentConfig := deps.Manager.GetConfig()
	if currentConfig == nil {
		return nil, fmt.Errorf("component-manager config manager has no config")
	}
	bootConfig := currentConfig.Get()
	componentsConfig := bootConfig.Components
	if componentsConfig == nil {
		componentsConfig = make(config.ComponentConfigs)
	}
	// Assigned only when the configuration holds a registry: a nil
	// *model.Registry in the interface would read as a registry to components.
	var bootModelRegistry model.RegistryReader
	if bootConfig.ModelRegistry != nil {
		bootModelRegistry = bootConfig.ModelRegistry
	}

	var opts []Option
	if deps.Logger != nil {
		opts = append(opts, WithLogger(deps.Logger))
	}
	if deps.MetricsRegistry != nil {
		opts = append(opts, WithMetrics(deps.MetricsRegistry))
	}

	cm := &ComponentManager{
		BaseService:       NewBaseServiceWithOptions(componentManagerName, opts...),
		registry:          deps.ComponentRegistry,
		payloadRegistry:   deps.PayloadRegistry,
		natsClient:        deps.NATSClient,
		platform:          deps.Platform,
		componentConfigs:  componentsConfig,
		bootSecurity:      bootConfig.Security,
		bootModelRegistry: bootModelRegistry,
		bootStreams:       bootConfig.Streams,
		storeRegistry:     storeregistry.New(),
	}

	// Stream migration-override expiry reporting (ADR-100). The reporter reads
	// currentConfig on every evaluation rather than capturing bootConfig: an
	// operator may extend or remove an override without restarting, and a
	// reporter that kept warning about a renewed bridge would be worse than one
	// that said nothing. It is report-only, never composition authority.
	cm.overrideExpiry = newStreamOverrideExpiryReporter(
		currentConfig.Get,
		cm.logger.With("source", "component-manager.stream-overrides"),
	)
	if deps.MetricsRegistry != nil {
		// Registered here, on the registry the /metrics endpoint scrapes: nothing
		// in the framework calls the Service interface's RegisterMetrics.
		if err := cm.overrideExpiry.register(deps.MetricsRegistry); err != nil {
			return nil, fmt.Errorf("register stream override expiry metrics: %w", err)
		}
	}
	// Evaluate once at composition so every declared bridge has a series before
	// it matters. An alert on a series that first appears at the moment of
	// failure cannot be tested in advance.
	cm.overrideExpiry.evaluate(time.Now())

	cm.SetHealthCheck(cm.healthCheck)

	if err := cm.initialize(); err != nil {
		return nil, fmt.Errorf("initialize component manager: %w", err)
	}
	return cm, nil
}

// initialize builds every enabled component in instance-name order, refuses
// boot when any cannot be built (naming each, the rule packs grouped and
// sorted), fixes the start order, analyses the composition and seals the
// registry. Only the constructor calls it.
func (cm *ComponentManager) initialize() error {
	cm.logger.Debug("ComponentManager.initialize: creating components from config",
		"count", len(cm.componentConfigs))

	var (
		records          []*componentRecord
		rulePackFailures []error
		otherFailures    []error
	)
	for _, instanceName := range slices.Sorted(maps.Keys(cm.componentConfigs)) {
		componentConfig := cm.componentConfigs[instanceName]
		if !componentConfig.Enabled {
			cm.logger.Debug("ComponentManager.initialize: skipping disabled component",
				"instance", instanceName)
			continue
		}

		record, err := cm.createComponent(instanceName, componentConfig, cm.buildComponentDependencies())
		if err != nil {
			cm.logger.Error("Failed to create component from config",
				"instance", instanceName,
				"factory", componentConfig.Name,
				"type", componentConfig.Type,
				"error", err)
			// Every enabled component the manager cannot build refuses boot (Q-B,
			// owner 2026-10-21). The creation pass still finishes, so the refusal
			// names every failure. An enabled rule pack dropped here would also
			// hide it from ProjectionBinders; its failures keep the pin's own text.
			if componentConfig.Name == "rule-processor" {
				rulePackFailures = append(rulePackFailures,
					fmt.Errorf("rule processor component %q: %w", instanceName, err))
			} else {
				otherFailures = append(otherFailures, fmt.Errorf("component %q: %w", instanceName, err))
			}
			continue
		}
		records = append(records, record)

		cm.logger.Debug("Component created from config",
			"instance", instanceName,
			"factory", componentConfig.Name,
			"type", componentConfig.Type)
	}

	var failures []error
	if len(rulePackFailures) > 0 {
		failures = append(failures, fmt.Errorf(
			"enabled rule-processor cold-boot initialization failed: %w",
			errors.Join(rulePackFailures...),
		))
	}
	failures = append(failures, otherFailures...)
	if len(failures) > 0 {
		return errors.Join(failures...)
	}

	// The fixed start order: store providers by name, then the rest by name.
	slices.SortFunc(records, func(a, b *componentRecord) int {
		if a.provider != b.provider {
			if a.provider {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.name, b.name)
	})
	cm.components = records
	cm.componentsByName = make(map[string]*componentRecord, len(records))
	for position, record := range records {
		record.startOrder = position
		cm.componentsByName[record.name] = record
	}
	cm.logger.Debug("ComponentManager.initialize: finished creating components",
		"created", len(records))

	// ADR-100: validate the admitted composition at the real boundary with the
	// same interpreter the offline verb runs, over what was actually admitted;
	// an error-severity finding refuses boot before the seal.
	if err := cm.analyzeBootComposition(); err != nil {
		return err
	}

	cm.registry.SealComposition()
	return nil
}

// analyzeBootComposition runs composition.Analyze over the registry's admitted
// declarations and the boot configuration's explicit streams, logs every
// finding, retains the result for the HTTP projections, and returns an error,
// refusing boot, when the result has an error-severity finding.
func (cm *ComponentManager) analyzeBootComposition() error {
	result := composition.Analyze(cm.registry.Snapshots(), cm.bootStreams)
	cm.bootFindings = result

	for _, finding := range result.Warnings {
		cm.logger.Warn("composition finding",
			"type", finding.Type, "component", finding.Component, "port", finding.Port, "message", finding.Message)
	}
	for _, finding := range result.Errors {
		cm.logger.Error("composition finding",
			"type", finding.Type, "component", finding.Component, "port", finding.Port, "message", finding.Message)
	}
	cm.logger.Info("composition validated at boot",
		"status", result.Status, "errors", len(result.Errors), "warnings", len(result.Warnings),
		"components", len(result.Graph.Nodes), "edges", len(result.Graph.Edges))

	if len(result.Errors) == 0 {
		return nil
	}
	descriptions := make([]string, 0, len(result.Errors))
	for _, finding := range result.Errors {
		description := fmt.Sprintf("%s on %s/%s: %s", finding.Type, finding.Component, finding.Port, finding.Message)
		if len(finding.Suggestions) > 0 {
			description += " (" + strings.Join(finding.Suggestions, "; ") + ")"
		}
		descriptions = append(descriptions, description)
	}
	return fmt.Errorf("composition validation refused boot with %d error finding(s): %s",
		len(result.Errors), strings.Join(descriptions, " | "))
}

// bootCompositionResult returns the composition result kept at construction.
func (cm *ComponentManager) bootCompositionResult() *composition.Result {
	return cm.bootFindings
}

// createComponent builds one instance through the registry, running the
// component's Initialize before the registry publishes its declaration, and
// returns its record. The caller fixes the record's start order.
func (cm *ComponentManager) createComponent(
	instanceName string, cfg types.ComponentConfig, deps component.Dependencies,
) (*componentRecord, error) {
	if instanceName == "" {
		return nil, fmt.Errorf("instance name cannot be empty")
	}
	if cfg.Name == "" {
		return nil, fmt.Errorf("component factory name cannot be empty")
	}
	if cfg.Type == "" {
		return nil, fmt.Errorf("component type cannot be empty")
	}

	state := component.StateCreated
	comp, err := cm.registry.CreateComponent(
		instanceName, cfg, deps,
		func(comp component.Discoverable) error {
			if lifecycle, ok := component.AsLifecycleComponent(comp); ok {
				if err := lifecycle.Initialize(); err != nil {
					return fmt.Errorf("initialize component %q: %w", instanceName, err)
				}
				state = component.StateInitialized
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	_, provider := comp.(component.StoreProvider)
	record := &componentRecord{
		name:      instanceName,
		component: comp,
		config:    cfg,
		provider:  provider,
	}
	record.state.Store(&componentState{state: state})
	return record, nil
}

// healthCheck is the check BaseService runs while it runs. It loads every
// component's record every time and reports each component in StateFailed, in
// name order, with its last error; otherwise the manager is healthy. It waits on
// nothing (design D4, "The manager's own health").
func (cm *ComponentManager) healthCheck() error {
	var failures []error
	for _, name := range slices.Sorted(maps.Keys(cm.componentsByName)) {
		current := cm.componentsByName[name].current()
		if current.state != component.StateFailed {
			continue
		}
		if current.lastError != nil {
			failures = append(failures, fmt.Errorf("component %q failed: %w", name, current.lastError))
		} else {
			failures = append(failures, fmt.Errorf("component %q failed", name))
		}
	}
	return errors.Join(failures...)
}

// buildComponentDependencies returns the dependencies every component is built
// with.
func (cm *ComponentManager) buildComponentDependencies() component.Dependencies {
	return component.Dependencies{
		NATSClient:      cm.natsClient,
		MetricsRegistry: cm.metricsRegistry,
		Logger:          cm.logger,
		Platform:        cm.platform,
		Security:        cm.bootSecurity,
		ModelRegistry:   cm.bootModelRegistry,
		PayloadRegistry: cm.payloadRegistry,
		StoreRegistry:   cm.storeRegistry,
	}
}

// registerProvidedStores registers a just-started component's provided stores in
// the shared store registry (ADR-063), in instance order. A component that is
// not a store provider, or provides none, registers nothing. An empty instance
// name, a nil store and an instance another component holds are refused; a
// refusal removes this call's own registrations and leaves the holder's in
// place. The registered names are kept on the component's record, so
// deregisterProvidedStores never re-reads a store that may already be closed.
// The component is called with nothing held.
func (cm *ComponentManager) registerProvidedStores(name string, comp component.Discoverable) error {
	provider, ok := comp.(component.StoreProvider)
	if !ok {
		return nil
	}
	record, ok := cm.componentsByName[name]
	if !ok {
		return fmt.Errorf("store registry: component %q is not admitted", name)
	}
	provided := provider.ProvidedStores()
	if len(provided) == 0 {
		return nil
	}

	instances := make([]string, 0, len(provided))
	for instance, store := range provided {
		if strings.TrimSpace(instance) == "" {
			return fmt.Errorf("store registry: component %q claimed an empty StorageInstance", name)
		}
		storeIsNil := store == nil
		if !storeIsNil {
			storeValue := reflect.ValueOf(store)
			switch storeValue.Kind() {
			case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
				storeIsNil = storeValue.IsNil()
			}
		}
		if storeIsNil {
			return fmt.Errorf("store registry: component %q claimed nil store for instance %q", name, instance)
		}
		instances = append(instances, instance)
	}
	slices.Sort(instances)

	registered := make([]string, 0, len(instances))
	for _, instance := range instances {
		if err := cm.storeRegistry.Register(instance, provided[instance]); err != nil {
			for _, registeredInstance := range registered {
				cm.storeRegistry.Deregister(registeredInstance)
			}
			return fmt.Errorf("store registry: component %q instance %q: %w", name, instance, err)
		}
		registered = append(registered, instance)
		cm.logger.Debug("store registry: registered store", "component", name, "instance", instance)
	}
	record.update(func(s *componentState) {
		s.stores = append(s.stores, registered...)
	})
	return nil
}

// deregisterProvidedStores removes a stopping component's stores from the shared
// store registry (ADR-063), by the names its record kept. Call it before the
// component's Stop closes them, so the registry never points at a closing
// handle. A second call, or one for a component that registered nothing, does
// nothing.
func (cm *ComponentManager) deregisterProvidedStores(name string) {
	record, ok := cm.componentsByName[name]
	if !ok {
		return
	}
	instances := record.current().stores
	if len(instances) == 0 {
		return
	}
	record.update(func(s *componentState) {
		s.stores = nil
	})
	for _, instance := range instances {
		cm.storeRegistry.Deregister(instance)
		cm.logger.Debug("store registry: deregistered store", "component", name, "instance", instance)
	}
}

// GetComponentHealth returns each admitted component's own health answer. It
// asks every component, before, during and after Start and Stop, with nothing
// held (design D2, "No wait for readers").
func (cm *ComponentManager) GetComponentHealth() map[string]component.HealthStatus {
	result := make(map[string]component.HealthStatus, len(cm.components))
	for _, record := range cm.components {
		result[record.name] = record.component.Health()
	}
	return result
}

// GetComponentStatus returns each admitted component's lifecycle state and last
// error, read from its record, beside its own health and flow answers, asked
// with nothing held.
func (cm *ComponentManager) GetComponentStatus() map[string]ComponentStatus {
	result := make(map[string]ComponentStatus, len(cm.components))
	for _, record := range cm.components {
		current := record.current()
		result[record.name] = ComponentStatus{
			Name:      record.name,
			State:     current.state,
			Health:    record.component.Health(),
			DataFlow:  record.component.DataFlow(),
			LastError: current.lastError,
		}
	}
	return result
}

// ComponentStatus combines lifecycle state with health and flow metrics
type ComponentStatus struct {
	Name      string                 `json:"name"`
	State     component.State        `json:"state"`
	Health    component.HealthStatus `json:"health"`
	DataFlow  component.FlowMetrics  `json:"data_flow"`
	LastError error                  `json:"last_error,omitempty"`
}

// =============================================================================
// Composition projections
// =============================================================================

// GetFlowPaths reports, for each input component of the boot composition, every
// component reachable from it.
//
// It reads the graph the composition result keeps (ADR-100), and does not
// rebuild one from the registry, so it cannot disagree with the result
// `<components>/validate` and `<components>/flowgraph` serve: one interpreter
// stands behind every answer about a composition. This derives no severity: it
// is a projection, not a judgment.
func (cm *ComponentManager) GetFlowPaths() (map[string][]string, error) {
	result := cm.bootCompositionResult()
	if result == nil {
		return nil, errs.WrapTransient(
			errors.New("composition result unavailable"),
			"ComponentManager", "GetFlowPaths", "composition not initialized")
	}

	adjacency := make(map[string][]string, len(result.Graph.Nodes))
	for _, edge := range result.Graph.Edges {
		adjacency[edge.From] = append(adjacency[edge.From], edge.To)
	}

	paths := make(map[string][]string)
	for _, node := range result.Graph.Nodes {
		if !isInputNode(node) {
			continue
		}
		paths[node.Instance] = reachableFrom(node.Instance, adjacency)
	}
	return paths, nil
}

// isInputNode reports whether a projected node originates data for the graph.
// Both facts come off the projection itself: the declared component type, and
// an input port whose interaction pattern reaches outside the composition —
// PatternNetwork binds a local listener, PatternHTTPClient initiates an
// outbound poll. Either makes the component a data source.
func isInputNode(node composition.Node) bool {
	if node.Type == string(types.ComponentTypeInput) {
		return true
	}
	for _, port := range node.Inputs {
		switch component.InteractionPattern(port.Pattern) {
		case component.PatternNetwork, component.PatternHTTPClient:
			return true
		}
	}
	return false
}

// reachableFrom returns start followed by every node reachable from it, in
// depth-first order. Edges arrive sorted from the projection, so the order is
// stable across calls.
func reachableFrom(start string, adjacency map[string][]string) []string {
	visited := make(map[string]bool)
	var order []string

	var visit func(string)
	visit = func(node string) {
		if visited[node] {
			return
		}
		visited[node] = true
		order = append(order, node)
		for _, next := range adjacency[node] {
			visit(next)
		}
	}
	visit(start)
	return order
}
