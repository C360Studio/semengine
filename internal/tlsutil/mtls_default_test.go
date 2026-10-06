package tlsutil

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semengine/pkg/security"
	"github.com/stretchr/testify/require"
)

// TestServerMTLSRequiresClientCertificateByDefault: with mTLS enabled, neither the optional
// flag set nor a CN allowlist, a client with no certificate is refused and a client whose
// certificate the client CA signed is served, in a real handshake on TLS 1.2 and on TLS 1.3.
// Before owner ruling #9 comment 5994720412 item 4 the zero value of the pin's
// RequireClientCert made client certificates optional, so the first client was served.
func TestServerMTLSRequiresClientCertificateByDefault(t *testing.T) {
	versions := []struct {
		name    string
		config  string
		version uint16
	}{
		{"TLS1.2", "1.2", tls.VersionTLS12},
		{"TLS1.3", "1.3", tls.VersionTLS13},
	}
	for _, v := range versions {
		t.Run(v.name, func(t *testing.T) {
			dir := t.TempDir()
			serverCertPEM, serverKeyPEM := generateTestCert(t)
			clientCertPEM, clientKeyPEM := generateTestCertWithCN(t, "test-client")
			serverCertFile := filepath.Join(dir, "server-cert.pem")
			serverKeyFile := filepath.Join(dir, "server-key.pem")
			clientCAFile := filepath.Join(dir, "client-ca.pem")
			require.NoError(t, os.WriteFile(serverCertFile, serverCertPEM, 0600))
			require.NoError(t, os.WriteFile(serverKeyFile, serverKeyPEM, 0600))
			require.NoError(t, os.WriteFile(clientCAFile, clientCertPEM, 0600))

			serverTLS, err := LoadServerTLSConfigWithMTLS(
				security.ServerTLSConfig{Enabled: true, CertFile: serverCertFile, KeyFile: serverKeyFile, MinVersion: v.config},
				security.ServerMTLSConfig{Enabled: true, ClientCAFiles: []string{clientCAFile}},
			)
			require.NoError(t, err)

			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			server.TLS = serverTLS
			server.StartTLS()
			t.Cleanup(server.Close)

			get := func(certs ...tls.Certificate) error {
				transport := &http.Transport{TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, // the test checks the server's view of the client, not the reverse
					MinVersion:         v.version,
					MaxVersion:         v.version,
					Certificates:       certs,
				}}
				t.Cleanup(transport.CloseIdleConnections)
				client := &http.Client{Timeout: 10 * time.Second, Transport: transport}
				req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
				require.NoError(t, err)
				resp, err := client.Do(req)
				if err != nil {
					return err
				}
				defer resp.Body.Close()
				require.Equal(t, http.StatusOK, resp.StatusCode)
				return nil
			}

			require.Error(t, get(), "a client with no certificate must be refused when ClientCertOptional is unset")

			pair, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
			require.NoError(t, err)
			require.NoError(t, get(pair), "a client whose certificate the client CA signed must be served")
		})
	}
}
