//go:build integration

package config

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/c360studio/semengine/types"
)

// arbitrationConfig is a file configuration at version with the named services and components.
func arbitrationConfig(version string, services, components []string) *Config {
	cfg := identityTestConfig("acme", "dep")
	cfg.Version = version
	for _, name := range services {
		cfg.Services[name] = types.ServiceConfig{Enabled: true, Config: json.RawMessage(`{}`)}
	}
	for _, name := range components {
		cfg.Components[name] = types.ComponentConfig{Type: "input", Name: name, Enabled: true}
	}
	return cfg
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// sectionKeys returns the bucket's keys under services., components. and the version key.
func sectionKeys(entries map[string]string) []string {
	var keys []string
	for key := range entries {
		if strings.HasPrefix(key, "services.") || strings.HasPrefix(key, "components.") || key == "version" {
			keys = append(keys, key)
		}
	}
	return keys
}

// TestDeletedFileComponentStaysDeletedAtNextBoot is P4 (#17) as a test: a component the file
// declares, deleted from the bucket at run time, is not brought back by the next boot of the same
// file at the same version, because a selected bucket's components replace the file's (design D3).
//
// spec: config-manager / Version arbitration selects one source for services and components
func TestDeletedFileComponentStaysDeletedAtNextBoot(t *testing.T) {
	tc := fixtureClient(t)
	ctx := testContext(t)
	file := func() *Config { return arbitrationConfig("1.0.0", nil, []string{"a", "b"}) }

	first, err := NewConfigManager(file(), tc, nil)
	require.NoError(t, err)
	require.NoError(t, first.Start(ctx))
	require.NoError(t, first.DeleteComponentFromKV(ctx, "a"))
	stopManager(t, first)

	next, err := NewConfigManager(file(), tc, nil)
	require.NoError(t, err)
	require.NoError(t, next.Start(ctx))
	defer stopManager(t, next)

	components := next.GetConfig().Get().Components
	require.NotContains(t, components, "a", "a component deleted at run time must stay deleted at the next boot (#17)")
	require.Contains(t, components, "b")
}

// TestNameThatIsNotOneKeyTokenIsRefused: a service or component name that is not one NATS KV
// literal token after the space mapping is written as a key the watches and the sync never read
// back. Start refuses such a file name before it acquires the bucket, naming it, and the component
// writers refuse it before any write (design D3).
//
// spec: config-manager / Version arbitration selects one source for services and components
// spec: config-manager / Component writes are next-boot desired state
func TestNameThatIsNotOneKeyTokenIsRefused(t *testing.T) {
	for _, name := range []string{"a.b", "a*", ">"} {
		for _, section := range []string{"service", "component"} {
			t.Run(section+" "+name, func(t *testing.T) {
				tc := fixtureClient(t)
				ctx := testContext(t)
				var cfg *Config
				if section == "service" {
					cfg = arbitrationConfig("1.0.0", []string{name}, nil)
				} else {
					cfg = arbitrationConfig("1.0.0", nil, []string{name})
				}
				manager, err := NewConfigManager(cfg, tc, nil)
				require.NoError(t, err)

				err = manager.Start(ctx)
				require.Error(t, err, "a %s named %q must refuse Start", section, name)
				require.ErrorContains(t, err, name, "the refusal names the %s", section)

				// Refused before the bucket is acquired: no bucket, so no identity record.
				_, bucketErr := tc.GetKeyValueBucket(ctx, manager.bucketName)
				require.Error(t, bucketErr, "a refused name must be refused before the bucket is acquired")
			})
		}
	}

	// Two names that map to one key would overwrite each other in the bucket and in memory.
	for _, section := range []string{"service", "component"} {
		t.Run(section+" names sharing a key", func(t *testing.T) {
			tc := fixtureClient(t)
			ctx := testContext(t)
			names := []string{"a b", "a_b"}
			cfg := arbitrationConfig("1.0.0", nil, names)
			if section == "service" {
				cfg = arbitrationConfig("1.0.0", names, nil)
			}
			manager, err := NewConfigManager(cfg, tc, nil)
			require.NoError(t, err)

			err = manager.Start(ctx)
			require.Error(t, err)
			require.ErrorContains(t, err, `"a b"`)
			require.ErrorContains(t, err, `"a_b"`)
			_, bucketErr := tc.GetKeyValueBucket(ctx, manager.bucketName)
			require.Error(t, bucketErr, "a refused name must be refused before the bucket is acquired")
		})
	}

	t.Run("component writers", func(t *testing.T) {
		tc := fixtureClient(t)
		ctx := testContext(t)
		manager := newIdentityManager(t, tc, "acme", "dep")
		require.NoError(t, manager.Start(ctx))
		defer stopManager(t, manager)
		before := bucketEntries(t, ctx, tc, manager.bucketName)

		for _, name := range []string{"a.b", "a*", ">"} {
			err := manager.PutComponentToKV(ctx, name, types.ComponentConfig{Type: "input", Name: "udp", Enabled: true})
			require.Error(t, err, "PutComponentToKV(%q)", name)
			require.ErrorContains(t, err, name)
			err = manager.DeleteComponentFromKV(ctx, name)
			require.Error(t, err, "DeleteComponentFromKV(%q)", name)
			require.ErrorContains(t, err, name)
		}
		require.Equal(t, before, bucketEntries(t, ctx, tc, manager.bucketName),
			"a refused component name must not reach the bucket")

		// A space is mapped to '_' and is one token after the mapping: it is accepted.
		require.NoError(t, manager.PutComponentToKV(ctx, "a b", types.ComponentConfig{Type: "input", Name: "udp", Enabled: true}))
		_, err := mustStore(t, manager).Get(ctx, "components.a_b")
		require.NoError(t, err)
	})
}

// TestDeletingAFileComponentWithASpaceUpdatesMemory: a file component named with a space is
// stored under the mapped key, and deleting it by its file name removes it from the bucket and from
// GetConfig, under either spelling.
//
// spec: config-manager / Component writes are next-boot desired state
func TestDeletingAFileComponentWithASpaceUpdatesMemory(t *testing.T) {
	tc := fixtureClient(t)
	ctx := testContext(t)
	manager, err := NewConfigManager(arbitrationConfig("1.0.0", nil, []string{"a b"}), tc, nil)
	require.NoError(t, err)
	require.NoError(t, manager.Start(ctx))
	defer stopManager(t, manager)
	// Start names the component as its key does, the name every later boot reads back.
	require.Contains(t, manager.GetConfig().Get().Components, "a_b")

	require.NoError(t, manager.DeleteComponentFromKV(ctx, "a b"))
	_, err = mustStore(t, manager).Get(ctx, "components.a_b")
	require.Error(t, err, "the stored key must be deleted")
	components := manager.GetConfig().Get().Components
	require.NotContains(t, components, "a b", "a successful delete must update desired-state memory")
	require.NotContains(t, components, "a_b", "a successful delete must update desired-state memory")
}

// TestRecoveryFromAnInterruptedPush proves recovery from the leftovers a push interrupted after
// its first write can leave: the identity record and part of the services and components, with no
// version or the bucket's older one, since version is written last (design D3). It seeds that
// state through the KV API; driving the interruption itself would need fault injection.
//
// spec: config-manager / Version arbitration selects one source for services and components
func TestRecoveryFromAnInterruptedPush(t *testing.T) {
	for _, tt := range []struct {
		name          string
		storedVersion string // "" leaves no version key
	}{
		{name: "no version", storedVersion: ""},
		{name: "older version", storedVersion: "0.9.0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tc := fixtureClient(t)
			ctx := testContext(t)
			manager, err := NewConfigManager(arbitrationConfig("1.0.0", []string{"s1", "s2"}, []string{"c1", "c2"}), tc, nil)
			require.NoError(t, err)
			seedDeclaredIdentity(t, ctx, manager)
			putConfigValue(t, ctx, manager, "services.s1", types.ServiceConfig{Enabled: true, Config: json.RawMessage(`{}`)})
			putConfigValue(t, ctx, manager, "components.c1", types.ComponentConfig{Type: "input", Name: "c1", Enabled: true})
			if tt.storedVersion != "" {
				putConfigValue(t, ctx, manager, "version", tt.storedVersion)
			}

			require.NoError(t, manager.Start(ctx))
			defer stopManager(t, manager)

			got := manager.GetConfig().Get()
			require.ElementsMatch(t, []string{"s1", "s2"}, mapNames(got.Services))
			require.ElementsMatch(t, []string{"c1", "c2"}, mapNames(got.Components))

			entries := bucketEntries(t, ctx, tc, manager.bucketName)
			require.ElementsMatch(t,
				[]string{"services.s1", "services.s2", "components.c1", "components.c2", "version"},
				sectionKeys(entries), "the bucket holds the full push")
			require.Equal(t, `"1.0.0"`, entries["version"], "the bucket holds the file's version")
		})
	}
}

func mapNames[V any](m map[string]V) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	return names
}

// TestPushWritesVersionLast: after a successful push, the revision of version is greater than the
// revision of every other key the push wrote, so a push interrupted after its first write leaves
// the bucket's old version (design D3).
//
// spec: config-manager / Version arbitration selects one source for services and components
func TestPushWritesVersionLast(t *testing.T) {
	tc := fixtureClient(t)
	ctx := testContext(t)
	manager, err := NewConfigManager(arbitrationConfig("1.0.0", []string{"s1", "s2"}, []string{"c1", "c2"}), tc, nil)
	require.NoError(t, err)
	require.NoError(t, manager.Start(ctx)) // a first boot pushes the file
	defer stopManager(t, manager)

	store := directKVStore(t, ctx, manager)
	keys, err := store.Keys(ctx)
	require.NoError(t, err)
	version, err := store.Get(ctx, "version")
	require.NoError(t, err)
	var compared []string
	for _, key := range keys {
		if key == "version" || key == platformIdentityKVKey {
			continue
		}
		entry, err := store.Get(ctx, key)
		require.NoError(t, err)
		require.Less(t, entry.Revision, version.Revision, "%s was written after version", key)
		compared = append(compared, key)
	}
	// The comparison ran over a full push.
	require.Subset(t, compared, []string{"services.s1", "services.s2", "components.c1", "components.c2", "platform"})
}

// TestMalformedStoredServiceFailsStart is P5 as a test: a selected bucket holding a services.*
// entry that does not decode fails Start, naming the key, instead of dropping the service from the
// effective configuration. The failed Start arms no writer (design D4).
//
// spec: config-manager / Start fails closed on the bucket it cannot use
func TestMalformedStoredServiceFailsStart(t *testing.T) {
	tc := fixtureClient(t)
	ctx := testContext(t)
	manager, err := NewConfigManager(arbitrationConfig("1.0.0", []string{"metrics"}, nil), tc, nil)
	require.NoError(t, err)
	seedDeclaredIdentity(t, ctx, manager)
	putConfigValue(t, ctx, manager, "version", "1.0.0")
	putConfigValue(t, ctx, manager, "services.metrics", map[string]any{"enabled": true, "name": "m"})

	err = manager.Start(ctx)
	require.Error(t, err, "a stored service that does not decode must fail Start")
	require.ErrorContains(t, err, "services.metrics")
	require.ErrorIs(t, manager.PutComponentToKV(ctx, "c", types.ComponentConfig{Type: "input", Name: "udp", Enabled: true}),
		errBucketNotAcquired, "a failed Start arms no writer")
}

// TestStoredNATSTokenFailsStart: a selected bucket's nats value that sets a key configuration
// loading refuses fails Start, naming it, in any letter case (design D4).
//
// spec: config-manager / Start fails closed on the bucket it cannot use
func TestStoredNATSTokenFailsStart(t *testing.T) {
	for _, key := range []string{"token", "Token"} {
		t.Run(key, func(t *testing.T) {
			tc := fixtureClient(t)
			ctx := testContext(t)
			manager, err := NewConfigManager(arbitrationConfig("1.0.0", nil, nil), tc, nil)
			require.NoError(t, err)
			seedDeclaredIdentity(t, ctx, manager)
			putConfigValue(t, ctx, manager, "version", "1.0.0")
			putConfigValue(t, ctx, manager, "nats", map[string]any{key: "secret"})

			err = manager.Start(ctx)
			require.Error(t, err, "a stored nats %s must fail Start", key)
			require.ErrorContains(t, err, "nats.token")
		})
	}
}

// TestPushEncodingFailureFailsStart: a push whose value cannot be encoded fails Start, naming the
// entry, and writes nothing the push would have written. encoding/json refuses to marshal a
// json.RawMessage that is not valid JSON; this test is the probe for that (design D4). The driver
// is a service, not the component design D4 names: Start validates the whole effective
// configuration when it applies the platform identity, before arbitration, and that validation
// decodes each component's config, so a component whose config is "{" fails Start there and never
// reaches the push (notes/2.1.md). A service's config is not decoded by validation, so it reaches
// the push: at the pin, Start logged the marshal error, returned nil, and left version and the
// services written before it in the bucket.
//
// spec: config-manager / Start fails closed on the bucket it cannot use
func TestPushEncodingFailureFailsStart(t *testing.T) {
	broken := func(version string) *Config {
		cfg := arbitrationConfig(version, []string{"s1"}, []string{"good"})
		cfg.Services["broken"] = types.ServiceConfig{Enabled: true, Config: json.RawMessage("{")}
		return cfg
	}
	_, marshalErr := json.Marshal(broken("1.0.0").Services["broken"])
	require.Error(t, marshalErr, "precondition: encoding/json refuses the broken service")

	t.Run("first boot", func(t *testing.T) {
		tc := fixtureClient(t)
		ctx := testContext(t)
		manager, err := NewConfigManager(broken("1.0.0"), tc, nil)
		require.NoError(t, err)

		err = manager.Start(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "broken")
		// Refused before the bucket is acquired: no bucket, so no identity record either.
		_, bucketErr := tc.GetKeyValueBucket(ctx, manager.bucketName)
		require.Error(t, bucketErr, "a push that cannot encode a value must be refused before the bucket is acquired")
	})

	t.Run("newer file", func(t *testing.T) {
		tc := fixtureClient(t)
		ctx := testContext(t)
		first, err := NewConfigManager(arbitrationConfig("1.0.0", []string{"s1"}, []string{"good"}), tc, nil)
		require.NoError(t, err)
		require.NoError(t, first.Start(ctx))
		stopManager(t, first)
		before := bucketEntries(t, ctx, tc, first.bucketName)

		newer, err := NewConfigManager(broken("2.0.0"), tc, nil)
		require.NoError(t, err)
		err = newer.Start(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "broken")
		require.Equal(t, before, bucketEntries(t, ctx, tc, newer.bucketName),
			"a newer file's push that cannot encode a value leaves the bucket unchanged")
	})
}
