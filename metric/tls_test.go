package metric

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/c360studio/semengine/pkg/security"
	"github.com/stretchr/testify/require"
)

// testIdentity is a self-signed certificate usable as server certificate, client certificate and
// the client CA at once, written to temporary files for the file-based security configuration.
type testIdentity struct {
	certFile, keyFile string
	certPEM           []byte
	pair              tls.Certificate
}

func newTestIdentity(t *testing.T) testIdentity {
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
	id := testIdentity{certFile: filepath.Join(dir, "cert.pem"), keyFile: filepath.Join(dir, "key.pem"), certPEM: certPEM}
	require.NoError(t, os.WriteFile(id.certFile, certPEM, 0600))
	require.NoError(t, os.WriteFile(id.keyFile, keyPEM, 0600))
	id.pair, err = tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	return id
}

// TestServerRequiredMTLSRejectsClientWithoutCertificate: a Server configured to require client
// certificates serves only clients presenting one its client CA signed. At the pin Start loaded
// the server TLS configuration without its MTLS part (handler.go:154), so a client with no
// certificate was served.
func TestServerRequiredMTLSRejectsClientWithoutCertificate(t *testing.T) {
	id := newTestIdentity(t)
	securityCfg := security.Config{TLS: security.TLSConfig{Server: security.ServerTLSConfig{
		Enabled: true, Mode: "manual", CertFile: id.certFile, KeyFile: id.keyFile,
		MTLS: security.ServerMTLSConfig{Enabled: true, ClientCAFiles: []string{id.certFile}},
	}}}
	server := NewServer(9090, "/metrics", NewMetricsRegistry(), securityCfg)
	require.NoError(t, server.StartWithListener(t.Context(), boundServerListener(t)))
	registerServerCleanup(t, server)

	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(id.certPEM))
	clientFor := func(certs ...tls.Certificate) *http.Client {
		transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, Certificates: certs}}
		t.Cleanup(transport.CloseIdleConnections)
		return &http.Client{Timeout: failureBound, Transport: transport}
	}

	response, err := testServerGET(t, server.Address(), clientFor())
	if err == nil {
		_ = response.Body.Close()
	}
	require.Error(t, err, "a client with no certificate must be refused when client certificates are required")

	response, err = testServerGET(t, server.Address(), clientFor(id.pair))
	require.NoError(t, err, "a client presenting a certificate the client CA signed must be served")
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
}

// TestServerRefusesMTLSWithoutTLS: client certificates can only be required over TLS, so a Server
// whose configuration enables mTLS with server TLS disabled refuses to start rather than serving
// plain HTTP to every client. security.Config.Validate does not catch it: a disabled server TLS
// block validates nothing beneath it.
func TestServerRefusesMTLSWithoutTLS(t *testing.T) {
	id := newTestIdentity(t)
	securityCfg := security.Config{TLS: security.TLSConfig{Server: security.ServerTLSConfig{
		MTLS: security.ServerMTLSConfig{Enabled: true, ClientCAFiles: []string{id.certFile}},
	}}}
	server := NewServer(9090, "/metrics", NewMetricsRegistry(), securityCfg)
	listener := boundServerListener(t)
	err := server.StartWithListener(t.Context(), listener)
	if err == nil {
		registerServerCleanup(t, server)
	}
	require.Error(t, err, "mTLS without server TLS must be refused, not served as plain HTTP")
	require.False(t, listener.closed.Load(), "a refused StartWithListener leaves the listener with the caller")
	server.mu.Lock()
	defer server.mu.Unlock()
	require.Nil(t, server.server)
	require.Nil(t, server.listener)
	require.Nil(t, server.serveDone)
}
