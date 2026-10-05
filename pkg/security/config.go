// Package security provides platform-wide security configuration types
package security

// Config holds platform-wide security configuration.
type Config struct {
	TLS TLSConfig `json:"tls,omitempty" schema:"type:object,description:TLS configuration for servers and clients,category:security"`
}

// TLSConfig holds TLS configuration for HTTP/WebSocket servers and clients.
type TLSConfig struct {
	Server ServerTLSConfig `json:"server,omitempty" schema:"type:object,description:Server TLS configuration,category:security"`
	Client ClientTLSConfig `json:"client,omitempty" schema:"type:object,description:Client TLS configuration,category:security"`
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

// ServerMTLSConfig holds mTLS configuration for servers (client certificate validation).
type ServerMTLSConfig struct {
	Enabled           bool     `json:"enabled" schema:"type:boolean,description:Enable mTLS client certificate validation,default:false,category:security"`
	ClientCAFiles     []string `json:"client_ca_files,omitempty" schema:"type:array,description:CA certificates to trust for client validation,category:security"`
	RequireClientCert bool     `json:"require_client_cert,omitempty" schema:"type:boolean,description:Require client certificate (vs optional),default:true,category:security"`
	AllowedClientCNs  []string `json:"allowed_client_cns,omitempty" schema:"type:array,description:Allowed client certificate Common Names (empty=any),category:security"`
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

// ClientMTLSConfig holds mTLS configuration for clients (client certificate provision).
type ClientMTLSConfig struct {
	Enabled  bool   `json:"enabled" schema:"type:boolean,description:Enable mTLS client certificate,default:false,category:security"`
	CertFile string `json:"cert_file,omitempty" schema:"type:string,description:Path to client certificate file,category:security"`
	KeyFile  string `json:"key_file,omitempty" schema:"type:string,description:Path to client private key file,category:security"`
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
