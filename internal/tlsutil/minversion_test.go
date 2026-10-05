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

// The pin's config loader refused a min_version other than "1.2" or "1.3" at boot (pin
// config/config.go:324-326, :348-350, :358-364). SemEngine has no config loader yet, so the
// loaders refuse it themselves rather than build a TLS 1.2 floor nobody asked for (owner
// ruling, #9 comment 5985900154, item 3). The closed set has three accepted spellings, so
// named examples cover it: each accepted one, and refused near-misses of each.
var unknownTLSVersions = []string{"1.1", "1.0", "1.4", "invalid", "1.3 ", " 1.2", "TLS1.3", "tls1.2", "13"}

var acceptedTLSVersions = []struct {
	version string
	want    uint16
}{
	{"", tls.VersionTLS12},
	{"1.2", tls.VersionTLS12},
	{"1.3", tls.VersionTLS13},
}

func requireVersionRefusal(t *testing.T, version string, got *tls.Config, err error) {
	t.Helper()
	require.Error(t, err, "min_version %q must be refused", version)
	assert.Nil(t, got)
	assert.True(t, errs.IsInvalid(err), "refusal must be an invalid-configuration error: %v", err)
	assert.Contains(t, err.Error(), fmt.Sprintf("%q", version), "refusal must name the value")
}

func TestLoadServerTLSConfigRefusesUnknownMinVersion(t *testing.T) {
	certFile, keyFile, caFile, cleanup := setupTestFiles(t)
	defer cleanup()

	for _, version := range unknownTLSVersions {
		t.Run(version, func(t *testing.T) {
			cfg := security.ServerTLSConfig{Enabled: true, CertFile: certFile, KeyFile: keyFile, MinVersion: version}
			got, err := LoadServerTLSConfig(cfg)
			requireVersionRefusal(t, version, got, err)

			mtls := security.ServerMTLSConfig{Enabled: true, ClientCAFiles: []string{caFile}}
			got, err = LoadServerTLSConfigWithMTLS(cfg, mtls)
			requireVersionRefusal(t, version, got, err)
		})
	}
}

func TestLoadClientTLSConfigRefusesUnknownMinVersion(t *testing.T) {
	certFile, keyFile, _, cleanup := setupTestFiles(t)
	defer cleanup()

	for _, version := range unknownTLSVersions {
		t.Run(version, func(t *testing.T) {
			cfg := security.ClientTLSConfig{MinVersion: version}
			got, err := LoadClientTLSConfig(cfg)
			requireVersionRefusal(t, version, got, err)

			mtls := security.ClientMTLSConfig{Enabled: true, CertFile: certFile, KeyFile: keyFile}
			got, err = LoadClientTLSConfigWithMTLS(cfg, mtls)
			requireVersionRefusal(t, version, got, err)
		})
	}
}

func TestLoadersAcceptTheThreeMinVersionSpellings(t *testing.T) {
	certFile, keyFile, _, cleanup := setupTestFiles(t)
	defer cleanup()

	for _, tc := range acceptedTLSVersions {
		t.Run(fmt.Sprintf("%q", tc.version), func(t *testing.T) {
			server, err := LoadServerTLSConfig(security.ServerTLSConfig{
				Enabled: true, CertFile: certFile, KeyFile: keyFile, MinVersion: tc.version,
			})
			require.NoError(t, err)
			assert.Equal(t, tc.want, server.MinVersion)

			client, err := LoadClientTLSConfig(security.ClientTLSConfig{MinVersion: tc.version})
			require.NoError(t, err)
			assert.Equal(t, tc.want, client.MinVersion)
		})
	}
}
