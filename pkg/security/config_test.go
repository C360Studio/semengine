package security_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/c360studio/semengine/pkg/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// configDocument sets every key the configuration types declare, each to a value that is not
// the field's zero value. The keys are the configuration contract operators write, so they
// are spelled here by hand rather than derived from the struct tags.
const configDocument = `{
  "tls": {
    "server": {
      "enabled": true,
      "mode": "manual",
      "cert_file": "/etc/ssl/server.crt",
      "key_file": "/etc/ssl/server.key",
      "min_version": "1.3",
      "acme": {
        "enabled": true,
        "directory_url": "https://ca.example/acme/directory",
        "email": "ops@example.com",
        "domains": ["api.example.com"],
        "challenge_type": "tls-alpn-01",
        "renew_before": "8h",
        "storage_path": "/var/lib/acme",
        "ca_bundle": "/etc/ssl/step-ca.pem"
      },
      "mtls": {
        "enabled": true,
        "client_ca_files": ["/etc/ssl/client-ca.pem"],
        "require_client_cert": true,
        "allowed_client_cns": ["service-a"]
      }
    },
    "client": {
      "mode": "acme",
      "ca_files": ["/etc/ssl/internal-ca.pem"],
      "insecure_skip_verify": true,
      "min_version": "1.2",
      "acme": {
        "enabled": true,
        "directory_url": "https://ca.example/acme/client",
        "email": "client@example.com",
        "domains": ["client.example.com"],
        "challenge_type": "http-01",
        "renew_before": "24h",
        "storage_path": "/var/lib/acme-client",
        "ca_bundle": "/etc/ssl/client-step-ca.pem"
      },
      "mtls": {
        "enabled": true,
        "cert_file": "/etc/ssl/client.crt",
        "key_file": "/etc/ssl/client.key"
      }
    }
  }
}`

var wantConfig = security.Config{
	TLS: security.TLSConfig{
		Server: security.ServerTLSConfig{
			Enabled:    true,
			Mode:       "manual",
			CertFile:   "/etc/ssl/server.crt",
			KeyFile:    "/etc/ssl/server.key",
			MinVersion: "1.3",
			ACME: security.ACMEConfig{
				Enabled:       true,
				DirectoryURL:  "https://ca.example/acme/directory",
				Email:         "ops@example.com",
				Domains:       []string{"api.example.com"},
				ChallengeType: "tls-alpn-01",
				RenewBefore:   "8h",
				StoragePath:   "/var/lib/acme",
				CABundle:      "/etc/ssl/step-ca.pem",
			},
			MTLS: security.ServerMTLSConfig{
				Enabled:           true,
				ClientCAFiles:     []string{"/etc/ssl/client-ca.pem"},
				RequireClientCert: true,
				AllowedClientCNs:  []string{"service-a"},
			},
		},
		Client: security.ClientTLSConfig{
			Mode:               "acme",
			CAFiles:            []string{"/etc/ssl/internal-ca.pem"},
			InsecureSkipVerify: true,
			MinVersion:         "1.2",
			ACME: security.ACMEConfig{
				Enabled:       true,
				DirectoryURL:  "https://ca.example/acme/client",
				Email:         "client@example.com",
				Domains:       []string{"client.example.com"},
				ChallengeType: "http-01",
				RenewBefore:   "24h",
				StoragePath:   "/var/lib/acme-client",
				CABundle:      "/etc/ssl/client-step-ca.pem",
			},
			MTLS: security.ClientMTLSConfig{
				Enabled:  true,
				CertFile: "/etc/ssl/client.crt",
				KeyFile:  "/etc/ssl/client.key",
			},
		},
	},
}

// TestConfigDecodesEveryKey holds the JSON keys of the seven configuration types: a renamed
// or dropped key fails the decode (unknown fields are refused) or the comparison.
func TestConfigDecodesEveryKey(t *testing.T) {
	dec := json.NewDecoder(strings.NewReader(configDocument))
	dec.DisallowUnknownFields()

	var got security.Config
	require.NoError(t, dec.Decode(&got))
	assert.Equal(t, wantConfig, got)
}

// TestConfigDocumentCoversEveryField fails when a field is added to a configuration type
// without a key in configDocument, so TestConfigDecodesEveryKey keeps covering every field.
func TestConfigDocumentCoversEveryField(t *testing.T) {
	var walk func(path string, v reflect.Value)
	walk = func(path string, v reflect.Value) {
		if v.Kind() == reflect.Struct {
			for i := range v.NumField() {
				walk(path+"."+v.Type().Field(i).Name, v.Field(i))
			}
			return
		}
		assert.False(t, v.IsZero(), "%s has no non-zero value in configDocument", path)
	}
	walk("Config", reflect.ValueOf(wantConfig))
}
