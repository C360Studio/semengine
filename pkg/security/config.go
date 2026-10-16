// Package security provides platform-wide security configuration types
package security

// Config holds platform-wide security configuration.
type Config struct {
	TLS TLSConfig `json:"tls,omitempty"`
}

// TLSConfig holds TLS configuration for HTTP/WebSocket servers and clients.
type TLSConfig struct {
	Server ServerTLSConfig `json:"server,omitempty"`
	Client ClientTLSConfig `json:"client,omitempty"`
}

// ACMEConfig holds ACME client configuration for automated certificate management.
type ACMEConfig struct {
	Enabled       bool     `json:"enabled"`
	DirectoryURL  string   `json:"directory_url,omitempty"`
	Email         string   `json:"email,omitempty"`
	Domains       []string `json:"domains,omitempty"`
	ChallengeType string   `json:"challenge_type,omitempty"`
	RenewBefore   string   `json:"renew_before,omitempty"`
	StoragePath   string   `json:"storage_path,omitempty"`
	CABundle      string   `json:"ca_bundle,omitempty"`
}

// ServerMTLSConfig holds mTLS configuration for servers (client certificate validation).
type ServerMTLSConfig struct {
	Enabled            bool     `json:"enabled"`
	ClientCAFiles      []string `json:"client_ca_files,omitempty"`
	ClientCertOptional bool     `json:"client_cert_optional,omitempty"`
	AllowedClientCNs   []string `json:"allowed_client_cns,omitempty"`
}

// ServerTLSConfig holds TLS configuration for HTTP/WebSocket servers.
type ServerTLSConfig struct {
	Enabled    bool   `json:"enabled"`
	Mode       string `json:"mode,omitempty"`
	CertFile   string `json:"cert_file,omitempty"`
	KeyFile    string `json:"key_file,omitempty"`
	MinVersion string `json:"min_version,omitempty"`

	// ACME mode (Tier 3)
	ACME ACMEConfig `json:"acme,omitempty"`

	// mTLS support (both modes)
	MTLS ServerMTLSConfig `json:"mtls,omitempty"`
}

// ClientMTLSConfig holds mTLS configuration for clients (client certificate provision).
type ClientMTLSConfig struct {
	Enabled  bool   `json:"enabled"`
	CertFile string `json:"cert_file,omitempty"`
	KeyFile  string `json:"key_file,omitempty"`
}

// ClientTLSConfig holds TLS configuration for HTTP/WebSocket clients.
// Always uses system CA bundle first, CAFiles are ADDITIONAL trusted CAs.
type ClientTLSConfig struct {
	Mode               string   `json:"mode,omitempty"`
	CAFiles            []string `json:"ca_files,omitempty"`
	InsecureSkipVerify bool     `json:"insecure_skip_verify,omitempty"`
	MinVersion         string   `json:"min_version,omitempty"`

	// ACME mode (Tier 3)
	ACME ACMEConfig `json:"acme,omitempty"`

	// mTLS support (both modes)
	MTLS ClientMTLSConfig `json:"mtls,omitempty"`
}
