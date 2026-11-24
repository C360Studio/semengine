//go:build integration

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/types"
)

// TestComponentManagerRefusesAPlatformOtherThanTheEstablishedOne: a started config.Manager mints
// the platform ID from the declared stem, and the component manager refuses any deps.Platform
// with a field set that is not that minted pair, before a component factory runs. Each of the two
// fields is empty, equal to the effective value or different, so the nine cases are every
// combination.
//
// spec: service-lifecycle / A platform the configuration manager did not set is refused
func TestComponentManagerRefusesAPlatformOtherThanTheEstablishedOne(t *testing.T) {
	const declaredOrg, declaredStem = "o", "s"
	client := fixtureClient(t)
	logger := createTestServiceDependencies().Logger

	// The caller's configuration keeps the stem; Start mints the effective ID into a clone.
	configManager, err := config.NewConfigManager(&config.Config{
		Version:  "1.0.0",
		Platform: config.PlatformConfig{Org: declaredOrg, ID: declaredStem, Type: "test"},
		Services: make(types.ServiceConfigs),
		Components: config.ComponentConfigs{"worker": {
			Name: "counting", Type: types.ComponentTypeProcessor, Enabled: true,
			Config: json.RawMessage(`{}`),
		}},
	}, client, logger)
	require.NoError(t, err)
	require.NoError(t, configManager.Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), stopBound)
		defer cancel()
		require.NoError(t, configManager.Stop(ctx))
	})

	effective := configManager.GetConfig().Get().Platform
	require.Equal(t, declaredOrg, effective.Org)
	require.True(t, strings.HasPrefix(effective.ID, declaredStem+"-"),
		"effective platform ID %q is not minted from stem %q", effective.ID, declaredStem)

	orgs := map[string]string{"empty": "", "equal": effective.Org, "different": "other"}
	platforms := map[string]string{"empty": "", "equal": effective.ID, "different": declaredStem}
	for _, orgCase := range []string{"empty", "equal", "different"} {
		for _, platformCase := range []string{"empty", "equal", "different"} {
			given := types.PlatformMeta{Org: orgs[orgCase], Platform: platforms[platformCase]}
			accepted := given == types.PlatformMeta{} ||
				(given.Org == effective.Org && given.Platform == effective.ID)

			t.Run(fmt.Sprintf("org %s, platform %s", orgCase, platformCase), func(t *testing.T) {
				factoryCalls := 0
				components := component.NewRegistry()
				require.NoError(t, components.Register(component.Registration{
					Name: "counting", Type: types.ComponentTypeProcessor,
					Factory: func(json.RawMessage, component.Dependencies) (component.Discoverable, error) {
						factoryCalls++
						return &mockDiscoverableComponent{}, nil
					},
					Ports: func(json.RawMessage, string) (component.PortConfig, error) {
						return component.PortConfig{}, nil
					},
				}))

				services := NewServiceRegistry()
				require.NoError(t, services.Register(componentManagerName, NewComponentManager))
				manager := NewServiceManager(services)
				deps := &Dependencies{
					NATSClient: client, Logger: logger, MetricsRegistry: createTestServiceDependencies().MetricsRegistry,
					Platform: given, Manager: configManager, ComponentRegistry: components,
				}
				// metrics is disabled: it is not registered here, and its default port is fixed.
				err := manager.ConfigureFromServices(types.ServiceConfigs{
					"metrics": {Enabled: false, Config: json.RawMessage(`{}`)},
				}, deps)
				built, composed := manager.GetService(componentManagerName)

				if accepted {
					require.NoError(t, err)
					require.Equal(t, 1, factoryCalls, "the worker factory ran %d times, want once", factoryCalls)
					require.True(t, composed, "the component manager is not composed")
					require.IsType(t, (*ComponentManager)(nil), built)
					return
				}
				require.Error(t, err, "deps.Platform %+v was accepted", given)
				require.True(t, errs.IsInvalid(err), "the refusal is not classified invalid: %v", err)
				require.Contains(t, err.Error(), fmt.Sprintf("(org %q, platform %q)", given.Org, given.Platform),
					"the refusal does not name the given pair")
				require.Contains(t, err.Error(), fmt.Sprintf("(org %q, id %q)", effective.Org, effective.ID),
					"the refusal does not name the effective pair")
				require.Zero(t, factoryCalls, "a component factory ran before the refusal")
				require.False(t, composed, "a refused composition holds a component manager")
			})
		}
	}
}
