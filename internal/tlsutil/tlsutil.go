// Package tlsutil provides TLS configuration utilities for secure connections.
package tlsutil

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/security"
)

// LoadServerTLSConfig creates a tls.Config for HTTP/WebSocket servers from platform config,
// applying cfg.MTLS when it is enabled. Enabling mTLS while server TLS is disabled is refused
// with errs.ErrInvalidConfig: a plain HTTP server cannot verify a client certificate, so there
// is no config that honours both.
func LoadServerTLSConfig(cfg security.ServerTLSConfig) (*tls.Config, error) {
	if cfg.MTLS.Enabled && !cfg.Enabled {
		return nil, errs.WrapInvalid(errs.ErrInvalidConfig, "tlsutil", "LoadServerTLSConfig",
			"mTLS is enabled but server TLS is not")
	}
	if !cfg.Enabled {
		return nil, nil
	}

	minVersion, err := parseTLSVersion(cfg.MinVersion)
	if err != nil {
		return nil, errs.WrapInvalid(err, "tlsutil", "LoadServerTLSConfig", "parse min_version")
	}

	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, errs.WrapFatal(err, "tlsutil", "LoadServerTLSConfig", "load certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   minVersion,
	}

	if cfg.MTLS.Enabled {
		if err := applyMTLSConfig(tlsConfig, cfg.MTLS); err != nil {
			return nil, err
		}
	}

	return tlsConfig, nil
}

// LoadClientTLSConfig creates a tls.Config for HTTP/WebSocket clients from platform config.
// Always uses system CA bundle first, CAFiles are additional trusted CAs. When cfg.MTLS is
// enabled, its key pair is the client certificate.
func LoadClientTLSConfig(cfg security.ClientTLSConfig) (*tls.Config, error) {
	minVersion, err := parseTLSVersion(cfg.MinVersion)
	if err != nil {
		return nil, errs.WrapInvalid(err, "tlsutil", "LoadClientTLSConfig", "parse min_version")
	}

	tlsConfig := &tls.Config{
		MinVersion: minVersion,
	}

	// Start with system CA pool
	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		// If system pool unavailable, create empty pool
		rootCAs = x509.NewCertPool()
	}

	// Add additional CAs from config
	if err := appendCAFiles(rootCAs, cfg.CAFiles, "LoadClientTLSConfig", "CA"); err != nil {
		return nil, err
	}

	tlsConfig.RootCAs = rootCAs

	// Handle InsecureSkipVerify
	// Note: Setting this is intentional via config - operators know the security implications
	if cfg.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	if cfg.MTLS.Enabled {
		clientCert, err := tls.LoadX509KeyPair(cfg.MTLS.CertFile, cfg.MTLS.KeyFile)
		if err != nil {
			return nil, errs.WrapFatal(err, "tlsutil", "LoadClientTLSConfig", "load client certificate")
		}
		tlsConfig.Certificates = []tls.Certificate{clientCert}
	}

	return tlsConfig, nil
}

// appendCAFiles adds the PEM certificates in files to pool. A file that cannot be read or
// holds no certificate is fatal and named; label ("CA" or "client CA") keeps each caller's text.
func appendCAFiles(pool *x509.CertPool, files []string, method, label string) error {
	for _, caFile := range files {
		caPEM, err := os.ReadFile(caFile)
		if err != nil {
			return errs.WrapFatal(err, "tlsutil", method, fmt.Sprintf("read %s file %s", label, caFile))
		}
		if !pool.AppendCertsFromPEM(caPEM) {
			return errs.WrapFatal(fmt.Errorf("invalid PEM data"), "tlsutil", method,
				fmt.Sprintf("parse %s certificate from %s", label, caFile))
		}
	}
	return nil
}

// applyMTLSConfig applies mTLS settings to existing tls.Config
func applyMTLSConfig(tlsConfig *tls.Config, mtlsCfg security.ServerMTLSConfig) error {
	// Load client CA certificates for validation
	clientCAs := x509.NewCertPool()
	if err := appendCAFiles(clientCAs, mtlsCfg.ClientCAFiles, "applyMTLSConfig", "client CA"); err != nil {
		return err
	}

	tlsConfig.ClientCAs = clientCAs
	// A client certificate is required unless the configuration says otherwise, so the
	// zero value fails closed (owner ruling, #9 comment 5994720412, item 4).
	if mtlsCfg.ClientCertOptional {
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	} else {
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	// Optional: CN whitelist verification
	if len(mtlsCfg.AllowedClientCNs) > 0 {
		tlsConfig.VerifyPeerCertificate = func(_ [][]byte, verifiedChains [][]*x509.Certificate) error {
			return verifyAllowedClientCN(verifiedChains, mtlsCfg.AllowedClientCNs)
		}
	}

	return nil
}

// verifyAllowedClientCN checks if client certificate CN is in whitelist
func verifyAllowedClientCN(chains [][]*x509.Certificate, allowedCNs []string) error {
	if len(chains) == 0 {
		return fmt.Errorf("no verified certificate chains")
	}

	leafCert := chains[0][0]
	for _, allowedCN := range allowedCNs {
		if leafCert.Subject.CommonName == allowedCN {
			return nil
		}
	}

	return fmt.Errorf("client certificate CN '%s' not in allowed list",
		leafCert.Subject.CommonName)
}

// parseTLSVersion converts version string to crypto/tls constant.
// Empty means the default, TLS 1.2. Any other value is refused, as the pin's
// config loader refused it at boot: a typo must not silently become TLS 1.2.
func parseTLSVersion(version string) (uint16, error) {
	switch version {
	case "1.3":
		return tls.VersionTLS13, nil
	case "1.2", "":
		return tls.VersionTLS12, nil
	default:
		return 0, fmt.Errorf("invalid TLS version %q (must be \"1.2\" or \"1.3\")", version)
	}
}
