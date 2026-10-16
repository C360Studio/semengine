package tlsutil

import (
	"crypto/tls"
	"fmt"
	"testing"

	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadersRefuseSettingsNotApplied: SemEngine has no ACME loader yet, so both loaders refuse
// a mode other than empty or "manual", and an ACME block that is enabled, rather than build a
// configuration from certificate files the operator did not ask for (security-config spec,
// "Settings no SemEngine code applies yet are refused"). The other ACME settings act only under
// acme.enabled, so refusing it covers them.
func TestLoadersRefuseSettingsNotApplied(t *testing.T) {
	certFile, keyFile, _, cleanup := setupTestFiles(t)
	defer cleanup()

	type settings struct {
		mode string
		acme security.ACMEConfig
	}
	server := func(s settings) (*tls.Config, error) {
		return LoadServerTLSConfig(security.ServerTLSConfig{
			Enabled: true, Mode: s.mode, CertFile: certFile, KeyFile: keyFile, ACME: s.acme,
		})
	}
	client := func(s settings) (*tls.Config, error) {
		return LoadClientTLSConfig(security.ClientTLSConfig{Mode: s.mode, ACME: s.acme})
	}
	loaders := map[string]func(settings) (*tls.Config, error){"server": server, "client": client}

	refused := []struct {
		name     string
		settings settings
		key      string
	}{
		{"mode acme", settings{mode: "acme"}, `mode "acme"`},
		{"mode amce", settings{mode: "amce"}, `mode "amce"`},
		{"acme enabled", settings{acme: security.ACMEConfig{Enabled: true}}, "acme.enabled"},
		{"acme enabled in manual mode", settings{mode: "manual", acme: security.ACMEConfig{Enabled: true}}, "acme.enabled"},
	}
	for loaderName, load := range loaders {
		for _, tc := range refused {
			t.Run(loaderName+"/"+tc.name, func(t *testing.T) {
				got, err := load(tc.settings)
				require.Error(t, err, "%s must be refused", tc.key)
				assert.Nil(t, got)
				assert.True(t, errs.IsInvalid(err), "refusal must be an invalid-configuration error: %v", err)
				assert.Contains(t, err.Error(), tc.key, "refusal must name the key")
			})
		}
		for _, mode := range []string{"", "manual"} {
			t.Run(fmt.Sprintf("%s/mode %q accepted", loaderName, mode), func(t *testing.T) {
				got, err := load(settings{mode: mode})
				require.NoError(t, err)
				assert.NotNil(t, got)
			})
		}
	}
}
