// Package security provides platform-wide security configuration types.
//
// # Overview
//
// The security package defines configuration structures for TLS, mTLS, and ACME
// across the platform. It provides a unified way to configure secure communications
// for HTTP servers, WebSocket servers, and HTTP clients.
//
// Key features:
//   - Server TLS configuration (manual or ACME-managed certificates)
//   - Client TLS configuration (CA trust, certificate verification)
//   - Mutual TLS (mTLS) for both client and server
//   - ACME integration for automatic certificate management
//   - Comprehensive validation for all configuration options
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
//	│  - Enabled                  │  - Mode (manual/acme)                 │
//	│  - Mode (manual/acme)       │  - CAFiles                            │
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
//	if err := cfg.Validate(); err != nil {
//	    log.Fatal(err)
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
//	        RequireClientCert: true,
//	        AllowedClientCNs:  []string{"service-a", "service-b"},
//	    },
//	}
//
// ACME-managed certificates:
//
//	cfg := security.ServerTLSConfig{
//	    Enabled: true,
//	    Mode:    "acme",
//	    ACME: security.ACMEConfig{
//	        Enabled:       true,
//	        DirectoryURL:  "https://acme-v02.api.letsencrypt.org/directory",
//	        Email:         "admin@example.com",
//	        Domains:       []string{"example.com"},
//	        ChallengeType: "http-01",
//	        StoragePath:   "/etc/ssl/acme",
//	    },
//	}
//
// # Configuration Types
//
// ServerTLSConfig - Server-side TLS:
//   - Enabled: Enable TLS for the server
//   - Mode: "manual" (provide certs) or "acme" (automatic)
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
//   - RequireClientCert: Require vs optional
//   - AllowedClientCNs: Optional CN whitelist
//
// ClientMTLSConfig - Client mTLS (present client cert):
//   - CertFile/KeyFile: Client certificate and key
//
// ACMEConfig - ACME certificate automation:
//   - DirectoryURL: ACME server URL
//   - Email: Contact email
//   - Domains: Certificate domains
//   - ChallengeType: "http-01" or "tls-alpn-01"
//   - RenewBefore: Renewal lead time (e.g., "24h")
//
// # Validation
//
// All config types provide Validate() methods that check:
//   - Required fields are present when features are enabled
//   - File paths exist and are readable
//   - Enum values are valid ("1.2"/"1.3", "manual"/"acme", etc.)
//   - Duration strings parse correctly
//
// # Security Defaults
//
// DefaultConfig() provides secure defaults:
//   - TLS disabled by default (explicit opt-in)
//   - TLS 1.2 minimum version
//   - InsecureSkipVerify false
//   - Manual mode (no ACME by default)
//
// # Thread Safety
//
// Config types are value types and safe to copy. Validation methods
// are safe for concurrent use.
//
// # See Also
//
// Related packages:
//   - [github.com/c360studio/semengine/internal/tlsutil]: Build tls.Config from these types
//   - [github.com/c360studio/semengine/pkg/acme]: ACME client implementation
package security
