//go:build integration

package websocket

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/metric"
	"github.com/c360studio/semengine/pkg/security"
)

// testCA is a certificate authority the test owns. It signs certificates written as PEM files
// under a temporary directory; pool trusts it.
type testCA struct {
	cert     *x509.Certificate
	key      *ecdsa.PrivateKey
	certFile string
	pool     *x509.CertPool
	dir      string
	serial   int64
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	ca := &testCA{dir: t.TempDir(), serial: 1}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(ca.serial),
		Subject:               pkix.Name{CommonName: "websocket test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	ca.key = generateKey(t)
	der, err := x509.CreateCertificate(rand.Reader, template, template, &ca.key.PublicKey, ca.key)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}
	if ca.cert, err = x509.ParseCertificate(der); err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	ca.certFile = writePEM(t, ca.dir, "ca.pem", "CERTIFICATE", der)
	ca.pool = x509.NewCertPool()
	ca.pool.AddCert(ca.cert)
	return ca
}

// issue signs a certificate for name with usage, valid for 127.0.0.1, and returns its certificate
// and key files.
func (ca *testCA) issue(t *testing.T, name string, usage x509.ExtKeyUsage) (certFile, keyFile string) {
	t.Helper()
	ca.serial++
	key := generateKey(t)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(ca.serial),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatalf("create %s certificate: %v", name, err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal %s key: %v", name, err)
	}
	return writePEM(t, ca.dir, name+".pem", "CERTIFICATE", der), writePEM(t, ca.dir, name+"-key.pem", "EC PRIVATE KEY", keyDER)
}

func generateKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func writePEM(t *testing.T, dir, name, blockType string, der []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// requireDialFails fails the test if dialer completes a WebSocket handshake with url.
func requireDialFails(t *testing.T, dialer *websocket.Dialer, url, why string) {
	t.Helper()
	conn, resp, err := dialer.DialContext(t.Context(), url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		_ = conn.Close()
		t.Fatalf("dial %s %s succeeded; want it refused", url, why)
	}
}

// TestTLSServesWSS: with server TLS and mTLS enabled in Dependencies.Security, the output serves
// wss: a client trusting the test's own CA and presenting a certificate it signed completes the
// handshake and receives a frame published afterwards. A client presenting no certificate, and a
// plain ws client, are refused, so the whole loaded configuration is applied and nothing is served
// without TLS.
func TestTLSServesWSS(t *testing.T) {
	ca := newTestCA(t)
	serverCert, serverKey := ca.issue(t, "server", x509.ExtKeyUsageServerAuth)
	clientCert, clientKey := ca.issue(t, "client", x509.ExtKeyUsageClientAuth)
	client := fixtureClient(t)
	o := started(t, newOutputWith(t, "ws.tls", "30s", component.Dependencies{
		NATSClient: client, MetricsRegistry: metric.NewMetricsRegistry(),
		Security: security.Config{TLS: security.TLSConfig{Server: security.ServerTLSConfig{
			Enabled: true, CertFile: serverCert, KeyFile: serverKey,
			MTLS: security.ServerMTLSConfig{Enabled: true, ClientCAFiles: []string{ca.certFile}},
		}}},
	}))
	url := "wss://" + o.listener + "/ws"

	pair, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		t.Fatalf("load client certificate: %v", err)
	}
	trusted := &websocket.Dialer{TLSClientConfig: &tls.Config{RootCAs: ca.pool, Certificates: []tls.Certificate{pair}}}
	ws := reading(t, dialConn(t, trusted, url, nil), true)
	awaitClients(t, o.c, 1)
	publish(t, client, "ws.tls", []byte(`{"over":"tls"}`))
	if got := decodeData(t, ws.next(t)); string(got.Payload) != `{"over":"tls"}` {
		t.Fatalf("payload over wss = %s, want {\"over\":\"tls\"}", got.Payload)
	}

	requireDialFails(t, &websocket.Dialer{TLSClientConfig: &tls.Config{RootCAs: ca.pool}}, url, "with no client certificate")
	requireDialFails(t, websocket.DefaultDialer, "ws://"+o.listener+"/ws", "without TLS")
}
