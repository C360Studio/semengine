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
//	│  LoadServerTLSConfigWithMTLS()  │  LoadClientTLSConfigWithMTLS()    │
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
// Server with mTLS (require client certificates):
//
//	tlsConfig, err := tlsutil.LoadServerTLSConfigWithMTLS(
//	    cfg,
//	    security.ServerMTLSConfig{
//	        Enabled:           true,
//	        ClientCAFiles:     []string{"/etc/ssl/client-ca.pem"},
//	        RequireClientCert: true,
//	        AllowedClientCNs:  []string{"service-a"},
//	    },
//	)
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
// Client with mTLS (present client certificate):
//
//	tlsConfig, err := tlsutil.LoadClientTLSConfigWithMTLS(
//	    cfg,
//	    security.ClientMTLSConfig{
//	        Enabled:  true,
//	        CertFile: "/etc/ssl/client.crt",
//	        KeyFile:  "/etc/ssl/client.key",
//	    },
//	)
//
// # Client Certificate Verification
//
// For mTLS servers, optional CN whitelist:
//
//	mtlsCfg := security.ServerMTLSConfig{
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
//   - Invalid errors: a MinVersion other than "1.2", "1.3" or empty
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
