// Package security provides platform-wide security configuration types.
//
// # Overview
//
// The security package defines configuration structures for TLS, mTLS, and ACME
// across the platform. It provides a unified way to configure secure communications
// for HTTP servers, WebSocket servers, and HTTP clients.
//
// The contract is the security-config spec (openspec/specs/security-config/spec.md).
// A product's component factory receives a Config as component.Dependencies.Security.
// No SemEngine code fills that field yet, so it holds what the code building the
// Dependencies put there, and the zero value otherwise.
//
// Key features:
//   - Server TLS configuration from certificate and key files
//   - Client TLS configuration (CA trust, certificate verification)
//   - Mutual TLS (mTLS) for both client and server
//   - ACME settings, which nothing applies yet and the TLS loaders refuse
//
// # Architecture
//
//	┌─────────────────────────────────────────────────────────────────────┐
//	│                        security.Config                              │
//	└─────────────────────────────────────────────────────────────────────┘
//	                              ↓
//	┌─────────────────────────────────────────────────────────────────────┐
//	│                        TLSConfig                                    │
//	├─────────────────────────────┬───────────────────────────────────────┤
//	│       ServerTLSConfig       │          ClientTLSConfig              │
//	│  - Enabled                  │  - Mode                               │
//	│  - Mode                     │  - CAFiles                            │
//	│  - CertFile, KeyFile        │  - InsecureSkipVerify                 │
//	│  - MinVersion               │  - MinVersion                         │
//	│  - ACMEConfig               │  - ACMEConfig                         │
//	│  - ServerMTLSConfig         │  - ClientMTLSConfig                   │
//	└─────────────────────────────┴───────────────────────────────────────┘
//
// # Usage
//
// Basic server TLS configuration:
//
//	cfg := security.ServerTLSConfig{
//	    Enabled:    true,
//	    Mode:       "manual",
//	    CertFile:   "/etc/ssl/server.crt",
//	    KeyFile:    "/etc/ssl/server.key",
//	    MinVersion: "1.2",
//	}
//
// Client TLS with additional CAs:
//
//	cfg := security.ClientTLSConfig{
//	    CAFiles:    []string{"/etc/ssl/internal-ca.pem"},
//	    MinVersion: "1.3",
//	}
//
// Server with mTLS (client certificate validation):
//
//	cfg := security.ServerTLSConfig{
//	    Enabled:  true,
//	    CertFile: "/etc/ssl/server.crt",
//	    KeyFile:  "/etc/ssl/server.key",
//	    MTLS: security.ServerMTLSConfig{
//	        Enabled:           true,
//	        ClientCAFiles:     []string{"/etc/ssl/client-ca.pem"},
//	        AllowedClientCNs:  []string{"service-a", "service-b"},
//	    },
//	}
//
// # Configuration Types
//
// ServerTLSConfig - Server-side TLS:
//   - Enabled: Enable TLS for the server
//   - Mode: empty or "manual"; any other value is refused (see ACMEConfig)
//   - CertFile/KeyFile: Certificate and key paths (manual mode)
//   - MinVersion: "1.2" or "1.3"
//
// ClientTLSConfig - Client-side TLS:
//   - CAFiles: Additional CAs to trust (system CAs always included)
//   - InsecureSkipVerify: Skip verification (DEV/TEST ONLY)
//   - MinVersion: "1.2" or "1.3"
//
// ServerMTLSConfig - Server mTLS (validate client certs):
//   - ClientCAFiles: CAs to trust for client certs
//   - ClientCertOptional: Admit clients without a certificate (default: required)
//   - AllowedClientCNs: Optional list of allowed client certificate Common Names.
//     When it is set, a client presenting no certificate is refused even with
//     ClientCertOptional.
//
// ClientMTLSConfig - Client mTLS (present client cert):
//   - CertFile/KeyFile: Client certificate and key
//
// ACMEConfig - settings for automatic certificates from an ACME server:
//   - SemEngine does not apply these yet. Until its ACME support is ported, the TLS
//     loaders refuse a Mode other than empty or "manual" and an ACME block with
//     Enabled true, with an error naming the key. For the server block this applies
//     when server TLS is enabled.
//
// # Validation
//
// These types carry no validation of their own. A configuration is checked when a
// TLS configuration is built from it: the certificate, key and CA files are read, and
// a MinVersion other than "1.2", "1.3" or empty is refused, as are the ACME settings
// above.
//
// # Security Defaults
//
// The zero value of each type provides these defaults:
//   - TLS disabled by default (explicit opt-in)
//   - TLS 1.2 minimum version
//   - InsecureSkipVerify false
//   - Mode empty, which means certificate and key files
//   - With server mTLS on, a client certificate is required
//
// # Thread Safety
//
// Config types are value types and safe to copy.
//
// # See Also
//
// Related:
//   - [github.com/c360studio/semengine/component.Dependencies]: Security is how a
//     component factory receives this configuration
//   - [github.com/c360studio/semengine/metric.NewServer]: applies TLS.Server to the
//     metrics server. No exported SemEngine call applies TLS.Client yet.
package security
