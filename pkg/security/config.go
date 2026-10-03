// Package security provides platform-wide security configuration types
package security

import (
	"fmt"
	"os"
	"time"
)

// Config holds platform-wide security configuration.
type Config struct {
	TLS TLSConfig `json:"tls,omitempty" schema:"type:object,description:TLS configuration for servers and clients,category:security"`
}

// DefaultConfig returns a security configuration with sensible defaults.
// TLS is disabled by default - enable explicitly for production.
func DefaultConfig() Config {
	return Config{
		TLS: DefaultTLSConfig(),
	}
}

// Validate checks if the configuration is valid.
func (c Config) Validate() error {
	return c.TLS.Validate()
}

// TLSConfig holds TLS configuration for HTTP/WebSocket servers and clients.
type TLSConfig struct {
	Server ServerTLSConfig `json:"server,omitempty" schema:"type:object,description:Server TLS configuration,category:security"`
	Client ClientTLSConfig `json:"client,omitempty" schema:"type:object,description:Client TLS configuration,category:security"`
}

// DefaultTLSConfig returns default TLS configuration with TLS disabled.
func DefaultTLSConfig() TLSConfig {
	return TLSConfig{
		Server: DefaultServerTLSConfig(),
		Client: DefaultClientTLSConfig(),
	}
}

// Validate checks if the TLS configuration is valid.
func (c TLSConfig) Validate() error {
	if err := c.Server.Validate(); err != nil {
		return fmt.Errorf("server TLS: %w", err)
	}
	if err := c.Client.Validate(); err != nil {
		return fmt.Errorf("client TLS: %w", err)
	}
	return nil
}

// ACMEConfig holds ACME client configuration for automated certificate management.
type ACMEConfig struct {
	Enabled       bool     `json:"enabled" schema:"type:boolean,description:Enable ACME certificate management,default:false,category:security"`
	DirectoryURL  string   `json:"directory_url,omitempty" schema:"type:string,description:ACME directory URL (e.g. step-ca),category:security"`
	Email         string   `json:"email,omitempty" schema:"type:string,description:Contact email for ACME account,category:security"`
	Domains       []string `json:"domains,omitempty" schema:"type:array,description:Domains for certificate,category:security"`
	ChallengeType string   `json:"challenge_type,omitempty" schema:"type:string,description:Challenge type (http-01 or tls-alpn-01),default:http-01,category:security"`
	RenewBefore   string   `json:"renew_before,omitempty" schema:"type:string,description:Renew certificate before expiry (e.g. 8h),default:24h,category:security"`
	StoragePath   string   `json:"storage_path,omitempty" schema:"type:string,description:Certificate storage path,category:security"`
	CABundle      string   `json:"ca_bundle,omitempty" schema:"type:string,description:CA bundle for step-ca validation,category:security"`
}

// Validate checks if the ACME configuration is valid.
func (c ACMEConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.DirectoryURL == "" {
		return fmt.Errorf("directory_url is required when ACME is enabled")
	}
	if len(c.Domains) == 0 {
		return fmt.Errorf("at least one domain is required when ACME is enabled")
	}
	if c.ChallengeType != "" && c.ChallengeType != "http-01" && c.ChallengeType != "tls-alpn-01" {
		return fmt.Errorf("challenge_type must be 'http-01' or 'tls-alpn-01', got %q", c.ChallengeType)
	}
	if c.RenewBefore != "" {
		if _, err := time.ParseDuration(c.RenewBefore); err != nil {
			return fmt.Errorf("renew_before must be a valid duration: %w", err)
		}
	}
	return nil
}

// ServerMTLSConfig holds mTLS configuration for servers (client certificate validation).
type ServerMTLSConfig struct {
	Enabled           bool     `json:"enabled" schema:"type:boolean,description:Enable mTLS client certificate validation,default:false,category:security"`
	ClientCAFiles     []string `json:"client_ca_files,omitempty" schema:"type:array,description:CA certificates to trust for client validation,category:security"`
	RequireClientCert bool     `json:"require_client_cert,omitempty" schema:"type:boolean,description:Require client certificate (vs optional),default:true,category:security"`
	AllowedClientCNs  []string `json:"allowed_client_cns,omitempty" schema:"type:array,description:Allowed client certificate Common Names (empty=any),category:security"`
}

// Validate checks if the server mTLS configuration is valid.
func (c ServerMTLSConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if len(c.ClientCAFiles) == 0 {
		return fmt.Errorf("client_ca_files is required when mTLS is enabled")
	}
	for _, f := range c.ClientCAFiles {
		if _, err := os.Stat(f); err != nil {
			return fmt.Errorf("client CA file %q: %w", f, err)
		}
	}
	return nil
}

// ServerTLSConfig holds TLS configuration for HTTP/WebSocket servers.
type ServerTLSConfig struct {
	Enabled    bool   `json:"enabled" schema:"type:boolean,description:Enable TLS for server,default:false,category:security"`
	Mode       string `json:"mode,omitempty" schema:"type:string,description:TLS mode (manual or acme),default:manual,category:security"`
	CertFile   string `json:"cert_file,omitempty" schema:"type:string,description:Path to server certificate file,category:security"`
	KeyFile    string `json:"key_file,omitempty" schema:"type:string,description:Path to server private key file,category:security"`
	MinVersion string `json:"min_version,omitempty" schema:"type:string,description:Minimum TLS version (1.2 or 1.3),default:1.2,category:security"`

	// ACME mode (Tier 3)
	ACME ACMEConfig `json:"acme,omitempty" schema:"type:object,description:ACME configuration for automatic certificates,category:security"`

	// mTLS support (both modes)
	MTLS ServerMTLSConfig `json:"mtls,omitempty" schema:"type:object,description:mTLS configuration for client certificate validation,category:security"`
}

// DefaultServerTLSConfig returns default server TLS configuration (disabled).
func DefaultServerTLSConfig() ServerTLSConfig {
	return ServerTLSConfig{
		Enabled:    false,
		Mode:       "manual",
		MinVersion: "1.2",
	}
}

// Validate checks if the server TLS configuration is valid.
func (c ServerTLSConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.Mode != "" && c.Mode != "manual" && c.Mode != "acme" {
		return fmt.Errorf("mode must be 'manual' or 'acme', got %q", c.Mode)
	}
	if c.MinVersion != "" && c.MinVersion != "1.2" && c.MinVersion != "1.3" {
		return fmt.Errorf("min_version must be '1.2' or '1.3', got %q", c.MinVersion)
	}
	if c.Mode == "acme" || (c.Mode == "" && c.ACME.Enabled) {
		if err := c.ACME.Validate(); err != nil {
			return fmt.Errorf("ACME: %w", err)
		}
	} else {
		// Manual mode - require cert and key files
		if c.CertFile == "" {
			return fmt.Errorf("cert_file is required when TLS is enabled in manual mode")
		}
		if c.KeyFile == "" {
			return fmt.Errorf("key_file is required when TLS is enabled in manual mode")
		}
		if _, err := os.Stat(c.CertFile); err != nil {
			return fmt.Errorf("cert_file %q: %w", c.CertFile, err)
		}
		if _, err := os.Stat(c.KeyFile); err != nil {
			return fmt.Errorf("key_file %q: %w", c.KeyFile, err)
		}
	}
	if err := c.MTLS.Validate(); err != nil {
		return fmt.Errorf("mTLS: %w", err)
	}
	return nil
}

// ClientMTLSConfig holds mTLS configuration for clients (client certificate provision).
type ClientMTLSConfig struct {
	Enabled  bool   `json:"enabled" schema:"type:boolean,description:Enable mTLS client certificate,default:false,category:security"`
	CertFile string `json:"cert_file,omitempty" schema:"type:string,description:Path to client certificate file,category:security"`
	KeyFile  string `json:"key_file,omitempty" schema:"type:string,description:Path to client private key file,category:security"`
}

// Validate checks if the client mTLS configuration is valid.
func (c ClientMTLSConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.CertFile == "" {
		return fmt.Errorf("cert_file is required when client mTLS is enabled")
	}
	if c.KeyFile == "" {
		return fmt.Errorf("key_file is required when client mTLS is enabled")
	}
	if _, err := os.Stat(c.CertFile); err != nil {
		return fmt.Errorf("cert_file %q: %w", c.CertFile, err)
	}
	if _, err := os.Stat(c.KeyFile); err != nil {
		return fmt.Errorf("key_file %q: %w", c.KeyFile, err)
	}
	return nil
}

// ClientTLSConfig holds TLS configuration for HTTP/WebSocket clients.
// Always uses system CA bundle first, CAFiles are ADDITIONAL trusted CAs.
type ClientTLSConfig struct {
	Mode               string   `json:"mode,omitempty" schema:"type:string,description:TLS mode (manual or acme),default:manual,category:security"`
	CAFiles            []string `json:"ca_files,omitempty" schema:"type:array,description:Additional CA certificates to trust,category:security"`
	InsecureSkipVerify bool     `json:"insecure_skip_verify,omitempty" schema:"type:boolean,description:Skip certificate verification (DEV/TEST ONLY),default:false,category:security"`
	MinVersion         string   `json:"min_version,omitempty" schema:"type:string,description:Minimum TLS version (1.2 or 1.3),default:1.2,category:security"`

	// ACME mode (Tier 3)
	ACME ACMEConfig `json:"acme,omitempty" schema:"type:object,description:ACME configuration for automatic client certificates,category:security"`

	// mTLS support (both modes)
	MTLS ClientMTLSConfig `json:"mtls,omitempty" schema:"type:object,description:mTLS configuration for client certificate provision,category:security"`
}

// DefaultClientTLSConfig returns default client TLS configuration.
func DefaultClientTLSConfig() ClientTLSConfig {
	return ClientTLSConfig{
		Mode:               "manual",
		InsecureSkipVerify: false,
		MinVersion:         "1.2",
	}
}

// Validate checks if the client TLS configuration is valid.
func (c ClientTLSConfig) Validate() error {
	if c.Mode != "" && c.Mode != "manual" && c.Mode != "acme" {
		return fmt.Errorf("mode must be 'manual' or 'acme', got %q", c.Mode)
	}
	if c.MinVersion != "" && c.MinVersion != "1.2" && c.MinVersion != "1.3" {
		return fmt.Errorf("min_version must be '1.2' or '1.3', got %q", c.MinVersion)
	}
	for _, f := range c.CAFiles {
		if _, err := os.Stat(f); err != nil {
			return fmt.Errorf("CA file %q: %w", f, err)
		}
	}
	if c.ACME.Enabled {
		if err := c.ACME.Validate(); err != nil {
			return fmt.Errorf("ACME: %w", err)
		}
	}
	if err := c.MTLS.Validate(); err != nil {
		return fmt.Errorf("mTLS: %w", err)
	}
	return nil
}
