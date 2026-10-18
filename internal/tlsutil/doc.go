// Package tlsutil provides TLS configuration utilities for secure connections.
//
// # Overview
//
// The tlsutil package bridges security configuration types (pkg/security) and
// Go's crypto/tls package. It creates properly configured tls.Config values
// for both server and client use cases, with support for mTLS.
//
// Key features:
//   - Server TLS configuration
//   - Client TLS configuration (with system CA integration)
//   - Mutual TLS (mTLS) for client certificate validation/provision
//
// # Architecture
//
//	┌─────────────────────────────────────────────────────────────────────┐
//	│                     security.Config                                 │
//	│  (Platform-wide security configuration)                             │
//	└─────────────────────────────────────────────────────────────────────┘
//	                              ↓
//	┌─────────────────────────────────────────────────────────────────────┐
//	│                        tlsutil                                      │
//	│  LoadServerTLSConfig()  │  LoadClientTLSConfig()                    │
//	│  (each applies the MTLS block of its config when it is enabled)     │
//	└─────────────────────────────────────────────────────────────────────┘
//	                              ↓
//	┌─────────────────────────────────────────────────────────────────────┐
//	│                        *tls.Config                                  │
//	│  (Ready to use with http.Server, tls.Dial, etc.)                    │
//	└─────────────────────────────────────────────────────────────────────┘
//
// # Usage
//
// Basic server TLS:
//
//	cfg := security.ServerTLSConfig{
//	    Enabled:  true,
//	    CertFile: "/etc/ssl/server.crt",
//	    KeyFile:  "/etc/ssl/server.key",
//	}
//
//	tlsConfig, err := tlsutil.LoadServerTLSConfig(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	server := &http.Server{
//	    Addr:      ":443",
//	    TLSConfig: tlsConfig,
//	}
//
// Server with mTLS (require client certificates): set the MTLS block and call
// the same loader. Enabling mTLS while Enabled is false is refused with an
// invalid-configuration error, since a plain HTTP server cannot check a client
// certificate.
//
//	cfg.MTLS = security.ServerMTLSConfig{
//	    Enabled:          true,
//	    ClientCAFiles:    []string{"/etc/ssl/client-ca.pem"},
//	    AllowedClientCNs: []string{"service-a"},
//	}
//	tlsConfig, err := tlsutil.LoadServerTLSConfig(cfg)
//
// Client TLS (uses system CAs plus additional):
//
//	cfg := security.ClientTLSConfig{
//	    CAFiles: []string{"/etc/ssl/internal-ca.pem"},
//	}
//
//	tlsConfig, err := tlsutil.LoadClientTLSConfig(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	client := &http.Client{
//	    Transport: &http.Transport{TLSClientConfig: tlsConfig},
//	}
//
// Client with mTLS (present client certificate): set the MTLS block and call
// the same loader; its key pair becomes the client certificate.
//
//	cfg.MTLS = security.ClientMTLSConfig{
//	    Enabled:  true,
//	    CertFile: "/etc/ssl/client.crt",
//	    KeyFile:  "/etc/ssl/client.key",
//	}
//	tlsConfig, err := tlsutil.LoadClientTLSConfig(cfg)
//
// # Client Certificate Verification
//
// For mTLS servers, optional CN whitelist:
//
//	cfg.MTLS = security.ServerMTLSConfig{
//	    Enabled:          true,
//	    ClientCAFiles:    []string{"/etc/ssl/client-ca.pem"},
//	    AllowedClientCNs: []string{"service-a", "service-b"},
//	}
//
// Only certificates with matching Common Names are accepted.
//
// # TLS Version Configuration
//
// Supported MinVersion values:
//   - "1.2" - TLS 1.2 (default, widely compatible)
//   - "1.3" - TLS 1.3 (more secure, modern clients only)
//
// If unspecified, defaults to TLS 1.2. Any other value is refused with an
// invalid-configuration error.
//
// # Error Handling
//
// Errors are classified using the errs package:
//   - Fatal errors: File not found, invalid PEM data
//   - Invalid errors: a MinVersion other than "1.2", "1.3" or empty; server
//     mTLS enabled while server TLS is not
//
// # Thread Safety
//
// All Load* functions are safe for concurrent use.
// The returned tls.Config is thread-safe for concurrent reads.
//
// # See Also
//
// Related packages:
//   - [github.com/c360studio/semengine/pkg/security]: Configuration types
//   - [crypto/tls]: Go standard library TLS package
package tlsutil
