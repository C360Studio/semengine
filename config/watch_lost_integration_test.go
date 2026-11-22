//go:build integration

package config

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

// Stop stops each watch on a real server before it ends the run context, so a
// watch loop can see its updates end while that context is still live. That is
// still Stop, not a lost watch: nothing is logged and Health stays healthy.
//
// spec: config-manager / A watch that ends outside Stop is reported
func TestStoppedManagerReportsNoLostWatch(t *testing.T) {
	tc := fixtureClient(t)
	family, err := NewKeyFamily("rules", func(KeyFamilyEntry) {})
	require.NoError(t, err)
	logs := &errorRecords{}
	manager, err := NewConfigManager(identityTestConfig("acme", "dep"), tc, slog.New(logs), WithKeyFamily(family))
	require.NoError(t, err)
	require.NoError(t, manager.Start(t.Context()))
	require.True(t, manager.Health().Healthy, "%+v", manager.Health())

	// Stop has joined every watch loop, so all they logged is in place.
	stopManager(t, manager)

	require.Empty(t, logs.watchNames(), "a watch ended by Stop is not lost")
	require.True(t, manager.Health().Healthy, "%+v", manager.Health())
}
