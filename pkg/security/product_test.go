package security_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requestBound bounds each request and the cleanup Stop, so a server that never answers fails
// the test instead of hanging it.
const requestBound = 10 * time.Second

// product is the smallest component a product built on SemEngine writes against the
// security-config contract: its factory takes the security configuration from the
// Dependencies it is called with and builds the metrics server from it.
type product struct {
	security security.Config
	server   *metric.Server
}

func (*product) InputPorts() []component.Port    { return nil }
func (*product) OutputPorts() []component.Port   { return nil }
func (*product) Health() component.HealthStatus  { return component.HealthStatus{Healthy: true} }
func (*product) DataFlow() component.FlowMetrics { return component.FlowMetrics{} }

var factory component.Factory = func(_ json.RawMessage, deps component.Dependencies) (component.Discoverable, error) {
	return &product{
		security: deps.Security,
		server:   metric.NewServer(0, "/metrics", metric.NewMetricsRegistry(), deps.Security),
	}, nil
}

// newProduct calls the factory as the host does, with a Dependencies whose Security an operator
// document describes, decoded with unknown keys refused.
func newProduct(t *testing.T, document string) *product {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(document))
	dec.DisallowUnknownFields()
	var cfg security.Config
	require.NoError(t, dec.Decode(&cfg))

	built, err := factory(nil, component.Dependencies{Security: cfg})
	require.NoError(t, err)
	p, ok := built.(*product)
	require.True(t, ok, "factory returned %T", built)
	return p
}

// startProduct starts the product's server on an ephemeral loopback listener and stops it, under
// a bounded context, when the test ends. It returns Start's error, stopping nothing if Start failed.
func startProduct(t *testing.T, p *product) error {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	if err := p.server.StartWithListener(t.Context(), listener); err != nil {
		return err
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestBound)
		defer cancel()
		require.NoError(t, p.server.Stop(ctx))
	})
	return nil
}

// identity is a self-signed certificate used as the server certificate, the client CA and the
// signed client certificate at once.
type identity struct {
	certFile, keyFile string
	roots             *x509.CertPool
	pair              tls.Certificate
}

func newIdentity(t *testing.T) identity {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IsCA:        true, BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	dir := t.TempDir()
	id := identity{certFile: filepath.Join(dir, "cert.pem"), keyFile: filepath.Join(dir, "key.pem"), roots: x509.NewCertPool()}
	require.NoError(t, os.WriteFile(id.certFile, certPEM, 0600))
	require.NoError(t, os.WriteFile(id.keyFile, keyPEM, 0600))
	require.True(t, id.roots.AppendCertsFromPEM(certPEM))
	id.pair, err = tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	return id
}

// operatorDocument is the security document an operator writes for a TLS server that requires
// client certificates signed by id. serverExtra and mtlsExtra add keys to the server and mtls
// blocks; each is empty or ends in a comma.
func operatorDocument(t *testing.T, id identity, serverExtra, mtlsExtra string) string {
	t.Helper()
	quote := func(s string) string {
		b, err := json.Marshal(s)
		require.NoError(t, err)
		return string(b)
	}
	return fmt.Sprintf(`{
  "tls": {
    "server": {
      %s
      "enabled": true,
      "cert_file": %s,
      "key_file": %s,
      "mtls": {
        %s
        "enabled": true,
        "client_ca_files": [%s]
      }
    }
  }
}`, serverExtra, quote(id.certFile), quote(id.keyFile), mtlsExtra, quote(id.certFile))
}

// get requests address with a crypto/tls client that trusts id and presents certs.
func get(t *testing.T, id identity, address string, certs ...tls.Certificate) (*http.Response, error) {
	t.Helper()
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: id.roots, Certificates: certs}}
	t.Cleanup(transport.CloseIdleConnections)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, address, nil)
	require.NoError(t, err)
	return (&http.Client{Timeout: requestBound, Transport: transport}).Do(request)
}

// TestProductServesTLSFromDependenciesSecurity drives the security-config contract the way a
// product does: a component factory receives the operator's security configuration as
// Dependencies.Security and builds metric.NewServer from it. The oracle is a crypto/tls client
// and the spec's scenarios, independent of internal/tlsutil.
func TestProductServesTLSFromDependenciesSecurity(t *testing.T) {
	t.Run("every setting arrives unchanged", func(t *testing.T) {
		p := newProduct(t, configDocument)
		assert.Equal(t, wantConfig, p.security)
	})

	id := newIdentity(t)

	t.Run("client certificate required", func(t *testing.T) {
		p := newProduct(t, operatorDocument(t, id, "", ""))
		require.NoError(t, startProduct(t, p))

		response, err := get(t, id, p.server.Address())
		if err == nil {
			_ = response.Body.Close()
		}
		require.Error(t, err, "a client with no certificate must be refused when client certificates are required")

		response, err = get(t, id, p.server.Address(), id.pair)
		require.NoError(t, err, "a client presenting a certificate the client CA signed must be served")
		require.NoError(t, response.Body.Close())
		require.Equal(t, http.StatusOK, response.StatusCode)
	})

	t.Run("client certificate optional", func(t *testing.T) {
		p := newProduct(t, operatorDocument(t, id, "", `"client_cert_optional": true,`))
		require.NoError(t, startProduct(t, p))

		response, err := get(t, id, p.server.Address())
		require.NoError(t, err, "with client_cert_optional a client with no certificate must be served")
		require.NoError(t, response.Body.Close())
		require.Equal(t, http.StatusOK, response.StatusCode)
	})

	t.Run("client certificate optional under a CN allowlist", func(t *testing.T) {
		p := newProduct(t, operatorDocument(t, id, "", `"client_cert_optional": true, "allowed_client_cns": ["service-a"],`))
		require.NoError(t, startProduct(t, p))

		response, err := get(t, id, p.server.Address())
		if err == nil {
			_ = response.Body.Close()
		}
		require.Error(t, err, "with a CN allowlist a client with no certificate must be refused, even with client_cert_optional")
	})

	t.Run("misspelled TLS version", func(t *testing.T) {
		p := newProduct(t, operatorDocument(t, id, `"min_version": "1.4",`, ""))
		require.ErrorContains(t, startProduct(t, p), "min_version")
	})
}
