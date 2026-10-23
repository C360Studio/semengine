package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"
)

// Construction's composition pass (pin component_manager_rulepack_init_test.go). The pin built a
// struct-literal manager and called its exported Initialize; here NewComponentManager composes the
// boot set and the tests read what it returns and what the factories saw (design D3, D7). Under
// Q-B (owner, 2026-10-21) a component that cannot be built refuses construction, naming every
// failed instance, so the pin's best-effort test for ordinary components is not carried.

type rulePackInitProbe struct {
	baseDiscoverable
	initializeErr error
	startCalls    int
}

func (probe *rulePackInitProbe) Initialize() error {
	return probe.initializeErr
}

func (probe *rulePackInitProbe) Start(context.Context) error {
	probe.startCalls++
	return nil
}

func (*rulePackInitProbe) Stop(context.Context) error {
	return nil
}

// rulePackInitFixture is the configuration a probe factory reads: a non-empty FactoryError makes
// the factory fail, a non-empty InitializeError makes the probe's Initialize fail.
type rulePackInitFixture struct {
	FactoryError    string `json:"factory_error"`
	InitializeError string `json:"initialize_error"`
}

// coldBootFactories records what the probe factories saw: the calls per factory name and every
// probe they returned. Construction builds components one after another (pin :316-352), so the
// fields need no lock.
type coldBootFactories struct {
	calls  map[string]int
	probes []*rulePackInitProbe
}

func (f *coldBootFactories) probeFactory(name string) component.Factory {
	return func(raw json.RawMessage, _ component.Dependencies) (component.Discoverable, error) {
		f.calls[name]++
		var fixture rulePackInitFixture
		if err := json.Unmarshal(raw, &fixture); err != nil {
			return nil, err
		}
		if fixture.FactoryError != "" {
			return nil, errors.New(fixture.FactoryError)
		}
		probe := &rulePackInitProbe{}
		if fixture.InitializeError != "" {
			probe.initializeErr = errors.New(fixture.InitializeError)
		}
		f.probes = append(f.probes, probe)
		return probe, nil
	}
}

// newColdBootDependencies returns the Dependencies the service manager would hand the component
// manager, with a config.Manager holding configs and a registry offering three factories:
// "rule-processor" and "ordinary-processor", which build probes as rulePackInitFixture says, and
// "ordinary-failing", which always fails with "ordinary component failure".
func newColdBootDependencies(t *testing.T, configs config.ComponentConfigs) (*Dependencies, *coldBootFactories) {
	t.Helper()
	registry := component.NewRegistry()
	factories := &coldBootFactories{calls: make(map[string]int)}
	noPorts := func(json.RawMessage, string) (component.PortConfig, error) { return component.PortConfig{}, nil }
	for _, name := range []string{"rule-processor", "ordinary-processor"} {
		require.NoError(t, registry.Register(component.Registration{
			Name: name, Type: types.ComponentTypeProcessor, Factory: factories.probeFactory(name), Ports: noPorts,
		}))
	}
	require.NoError(t, registry.Register(component.Registration{
		Name: "ordinary-failing", Type: types.ComponentTypeProcessor,
		Factory: func(json.RawMessage, component.Dependencies) (component.Discoverable, error) {
			return nil, errors.New("ordinary component failure")
		},
		Ports: noPorts,
	}))

	deps := createTestServiceDependencies()
	deps.NATSClient = newUnconnectedClient(t)
	manager, err := config.NewConfigManager(&config.Config{
		Version:    "1.0.0",
		Platform:   config.PlatformConfig{Org: "acme", ID: "component-manager-test", Type: "test"},
		Services:   make(types.ServiceConfigs),
		Components: configs,
	}, deps.NATSClient, deps.Logger)
	require.NoError(t, err)
	deps.Manager = manager
	deps.ComponentRegistry = registry
	return deps, factories
}

func TestComponentManagerInitializeAggregatesEnabledRulePackFailuresDeterministically(t *testing.T) {
	t.Parallel()
	deps, factories := newColdBootDependencies(t, config.ComponentConfigs{
		"zeta-pack": {
			Type: types.ComponentTypeProcessor, Name: "rule-processor", Enabled: true,
			Config: json.RawMessage(`{"initialize_error":"zeta init failed"}`),
		},
		"alpha-pack": {
			Type: types.ComponentTypeProcessor, Name: "rule-processor", Enabled: true,
			Config: json.RawMessage(`{"factory_error":"alpha factory failed"}`),
		},
		"valid-pack": {
			Type: types.ComponentTypeProcessor, Name: "rule-processor", Enabled: true,
			Config: json.RawMessage(`{}`),
		},
		"disabled-invalid-pack": {
			Type: types.ComponentTypeProcessor, Name: "rule-processor", Enabled: false,
			Config: json.RawMessage(`{"factory_error":"must be ignored"}`),
		},
		"ordinary-failure": {
			Type: types.ComponentTypeProcessor, Name: "ordinary-failing", Enabled: true,
			Config: json.RawMessage(`{}`),
		},
	})

	built, err := NewComponentManager(nil, deps)
	require.Error(t, err)
	require.Nil(t, built)
	require.ErrorContains(t, err, "enabled rule-processor cold-boot initialization failed")
	require.ErrorContains(t, err, "alpha-pack")
	require.ErrorContains(t, err, "alpha factory failed")
	require.ErrorContains(t, err, "zeta-pack")
	require.ErrorContains(t, err, "zeta init failed")
	require.Less(t,
		strings.Index(err.Error(), "alpha-pack"),
		strings.Index(err.Error(), "zeta-pack"),
		err.Error(),
	)
	// Q-B: the ordinary component that failed is named beside the rule packs.
	require.ErrorContains(t, err, "ordinary-failure")
	require.ErrorContains(t, err, "ordinary component failure")
	require.NotContains(t, err.Error(), "valid-pack")
	require.NotContains(t, err.Error(), "must be ignored")

	require.Equal(t, 3, factories.calls["rule-processor"], "disabled rule pack must not call its factory")
	// The creation pass continues after a rule-pack failure: valid-pack's probe and zeta-pack's
	// were both built, whichever order the instances were visited in.
	require.Len(t, factories.probes, 2, "creation pass must continue after a rule-pack failure")
	for _, probe := range factories.probes {
		require.Zero(t, probe.startCalls)
	}
}

// TestComponentManagerRefusesBootWhenAComponentCannotBeBuilt: Q-B (owner, 2026-10-21). An enabled
// component that is not a rule processor and whose factory or Initialize fails refuses
// construction, and the refusal names every failed instance with its cause, not only the first.
// It replaces the pin's TestComponentManagerInitializeOrdinaryFailureRemainsBestEffort (pin :135),
// where construction logged the failure and composed the rest.
func TestComponentManagerRefusesBootWhenAComponentCannotBeBuilt(t *testing.T) {
	t.Parallel()
	deps, factories := newColdBootDependencies(t, config.ComponentConfigs{
		"broken-factory": {
			Type: types.ComponentTypeProcessor, Name: "ordinary-processor", Enabled: true,
			Config: json.RawMessage(`{"factory_error":"broken factory failed"}`),
		},
		"broken-initialize": {
			Type: types.ComponentTypeProcessor, Name: "ordinary-processor", Enabled: true,
			Config: json.RawMessage(`{"initialize_error":"broken initialize failed"}`),
		},
		"always-failing": {
			Type: types.ComponentTypeProcessor, Name: "ordinary-failing", Enabled: true,
			Config: json.RawMessage(`{}`),
		},
		"intact": {
			Type: types.ComponentTypeProcessor, Name: "ordinary-processor", Enabled: true,
			Config: json.RawMessage(`{}`),
		},
	})

	built, err := NewComponentManager(nil, deps)
	require.Error(t, err, "a component that cannot be built must refuse boot")
	require.Nil(t, built)
	for instance, cause := range map[string]string{
		"broken-factory":    "broken factory failed",
		"broken-initialize": "broken initialize failed",
		"always-failing":    "ordinary component failure",
	} {
		require.ErrorContains(t, err, instance)
		require.ErrorContains(t, err, cause)
	}
	require.NotContains(t, err.Error(), "intact")

	require.Equal(t, 3, factories.calls["ordinary-processor"])
	require.Len(t, factories.probes, 2, "every configured component is attempted before the refusal")
	for _, probe := range factories.probes {
		require.Zero(t, probe.startCalls)
	}
}
