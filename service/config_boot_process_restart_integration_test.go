//go:build integration

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"
)

// Carried from the pin's config_boot_process_restart_integration_test.go. The pin's subprocess
// helper was a test that skipped itself unless its role variable was set; here the package's
// TestMain (fixture_client_integration_test.go) runs the role instead of the tests when the
// variable is set, so no test skips (design D7).
const (
	configBootProcessRoleEnv = "SEMENGINE_TEST_CONFIG_BOOT_PROCESS_ROLE"
	configBootProcessURLEnv  = "SEMENGINE_TEST_CONFIG_BOOT_PROCESS_NATS_URL"

	configBootProcessRoleWriter = "writer"
	configBootProcessRoleReader = "reader"

	configBootProcessFactory = "config-boot-process-proof"
	configBootInitialName    = "initial-component"
	configBootDesiredName    = "desired-component"

	// configBootProcessParentBound bounds one child process, its own configBootProcessChildBound
	// included.
	configBootProcessParentBound = 20 * time.Second
	configBootProcessChildBound  = 15 * time.Second
)

// configBootProcessPassed is the line a child prints once its role's assertions hold. The parent
// requires it, so a child that exits zero without running its role (the dispatch broken, and the
// test binary running no test under -test.run=^$) fails the parent.
func configBootProcessPassed(role string) string {
	return "config boot process role " + role + ": passed"
}

type configBootProcessComponent struct {
	baseDiscoverable
	started atomic.Bool
}

func (c *configBootProcessComponent) Health() component.HealthStatus {
	return component.HealthStatus{Healthy: c.started.Load(), LastCheck: time.Now()}
}
func (*configBootProcessComponent) Initialize() error { return nil }
func (c *configBootProcessComponent) Start(context.Context) error {
	c.started.Store(true)
	return nil
}
func (c *configBootProcessComponent) Stop(context.Context) error {
	c.started.Store(false)
	return nil
}

var _ component.LifecycleComponent = (*configBootProcessComponent)(nil)

// TestIntegration_ConfigBootActivationRequiresProcessRestart proves the boot boundary with two
// operating-system processes. The parent holds the package's shared NATS server; a writer and
// then a reader process boot against the same configuration bucket. The writer persists a second
// component and its running composition stays the first alone; the reader, a fresh process, boots
// both. Process exit is the synchronisation: the reader starts only after the writer's
// acknowledged write and its teardown, which closes its connection, have completed.
func TestIntegration_ConfigBootActivationRequiresProcessRestart(t *testing.T) {
	url := fixtureClient(t).URLs()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	for _, role := range []string{configBootProcessRoleWriter, configBootProcessRoleReader} {
		ctx, cancel := context.WithTimeout(t.Context(), configBootProcessParentBound)
		// -test.run=^$ runs no test should the role dispatch not take this process.
		cmd := exec.CommandContext(ctx, executable, "-test.run=^$")
		cmd.Env = append(os.Environ(),
			configBootProcessRoleEnv+"="+role,
			configBootProcessURLEnv+"="+url,
		)
		output, runErr := cmd.CombinedOutput()
		cancel()
		if runErr != nil {
			t.Fatalf("%s process failed: %v\n%s", role, runErr, output)
		}
		if !strings.Contains(string(output), configBootProcessPassed(role)) {
			t.Fatalf("%s process exited zero without running its role:\n%s", role, output)
		}
	}
}

// configBootProcessChild runs role in this process, a child of the parent test, and returns the
// process exit code. TestMain calls it in place of the tests.
func configBootProcessChild(role string) int {
	url := os.Getenv(configBootProcessURLEnv)
	if url == "" {
		fmt.Fprintln(os.Stderr, "missing subprocess NATS URL")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), configBootProcessChildBound)
	defer cancel()
	if err := runConfigBootProcessRole(ctx, url, role); err != nil {
		fmt.Fprintf(os.Stderr, "config boot process role %s: %v\n", role, err)
		return 1
	}
	fmt.Println(configBootProcessPassed(role))
	return 0
}

func runConfigBootProcessRole(ctx context.Context, url, role string) (result error) {
	// Each stop gets a fresh bounded context, so a role whose ctx ended still tears down.
	stopCtx := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	}
	client, closeClient, err := openClient(ctx, url)
	if err != nil {
		return fmt.Errorf("connect NATS client: %w", err)
	}
	defer func() {
		closeCtx, cancel := stopCtx()
		defer cancel()
		result = errors.Join(result, closeClient(closeCtx))
	}()

	deps := createTestServiceDependencies()
	configManager, err := config.NewConfigManager(configBootInitialConfig(), client, deps.Logger)
	if err != nil {
		return fmt.Errorf("construct Config Manager: %w", err)
	}
	if err := configManager.Start(ctx); err != nil {
		return fmt.Errorf("start Config Manager: %w", err)
	}
	defer func() {
		closeCtx, cancel := stopCtx()
		defer cancel()
		result = errors.Join(result, configManager.Stop(closeCtx))
	}()

	registry := component.NewRegistry()
	if err := registry.Register(component.Registration{
		Name: configBootProcessFactory,
		Type: types.ComponentTypeProcessor,
		Factory: func(raw json.RawMessage, _ component.Dependencies) (component.Discoverable, error) {
			var cfg struct {
				Marker string `json:"marker"`
			}
			if err := json.Unmarshal(raw, &cfg); err != nil {
				return nil, fmt.Errorf("decode proof component config: %w", err)
			}
			if cfg.Marker == "" {
				return nil, errors.New("proof component marker is required")
			}
			return &configBootProcessComponent{}, nil
		},
		Ports: func(json.RawMessage, string) (component.PortConfig, error) {
			return component.PortConfigFrom(nil, nil), nil
		},
	}); err != nil {
		return fmt.Errorf("register proof component: %w", err)
	}

	deps.NATSClient = client
	deps.Manager = configManager
	deps.ComponentRegistry = registry
	deps.Platform = types.PlatformMeta{Org: "test", Platform: "config-boot-process-proof"}
	serviceValue, err := NewComponentManager(json.RawMessage(`{}`), deps)
	if err != nil {
		return fmt.Errorf("construct ComponentManager: %w", err)
	}
	componentManager, ok := serviceValue.(*ComponentManager)
	if !ok {
		return fmt.Errorf("NewComponentManager returned %T, want *ComponentManager", serviceValue)
	}
	if err := componentManager.Start(ctx); err != nil {
		return fmt.Errorf("start ComponentManager: %w", err)
	}
	defer func() {
		closeCtx, cancel := stopCtx()
		defer cancel()
		result = errors.Join(result, componentManager.Stop(closeCtx))
	}()

	switch role {
	case configBootProcessRoleWriter:
		if err := assertConfigBootComposition(componentManager, configBootInitialName); err != nil {
			return fmt.Errorf("writer initial composition: %w", err)
		}
		if err := configManager.PutComponentToKV(ctx, configBootDesiredName,
			configBootComponentConfig("desired")); err != nil {
			return fmt.Errorf("persist desired component: %w", err)
		}
		if _, ok := configManager.GetConfig().Get().Components[configBootDesiredName]; !ok {
			return errors.New("Config Manager did not expose acknowledged desired component write")
		}
		if err := assertConfigBootComposition(componentManager, configBootInitialName); err != nil {
			return fmt.Errorf("writer composition after desired write: %w", err)
		}
	case configBootProcessRoleReader:
		if _, ok := configManager.GetConfig().Get().Components[configBootDesiredName]; !ok {
			return errors.New("fresh Config Manager did not load persisted desired component")
		}
		if err := assertConfigBootComposition(
			componentManager, configBootInitialName, configBootDesiredName,
		); err != nil {
			return fmt.Errorf("reader reboot composition: %w", err)
		}
	default:
		return fmt.Errorf("unknown subprocess role %q", role)
	}
	return nil
}

func configBootInitialConfig() *config.Config {
	return &config.Config{
		Version: "1.0.0",
		Platform: config.PlatformConfig{
			Org: "test", ID: "config-boot-process-proof", Environment: "test",
		},
		Components: config.ComponentConfigs{
			configBootInitialName: configBootComponentConfig("initial"),
		},
	}
}

func configBootComponentConfig(marker string) types.ComponentConfig {
	raw, err := json.Marshal(struct {
		Marker string `json:"marker"`
	}{Marker: marker})
	if err != nil {
		panic(err)
	}
	return types.ComponentConfig{
		Type: types.ComponentTypeProcessor, Name: configBootProcessFactory, Enabled: true, Config: raw,
	}
}

func assertConfigBootComposition(manager *ComponentManager, expected ...string) error {
	status := manager.GetComponentStatus()
	for name, componentStatus := range status {
		if componentStatus.State != component.StateStarted {
			return fmt.Errorf("component %q state = %s, want started", name, componentStatus.State)
		}
	}
	actual := slices.Sorted(maps.Keys(status))
	slices.Sort(expected)
	if !slices.Equal(actual, expected) {
		return fmt.Errorf("running composition = %v, want %v", actual, expected)
	}
	return nil
}
