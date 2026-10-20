package config

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/c360studio/semengine/types"
)

func documentConfig() *Config {
	return &Config{Platform: PlatformConfig{Org: "test", ID: "config"}}
}

// captureStderr runs fn with os.Stderr pointed at a temporary file and returns
// what fn wrote there. os.Stderr is process-global, so no test in this package
// runs in parallel with it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	saved := os.Stderr
	os.Stderr = file
	defer func() { os.Stderr = saved }()
	fn()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	written, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(written)
}

// TestValidateDocumentReadsNoFile — a document naming TLS files that do not
// exist passes ValidateDocument, which writes nothing even when certificate
// verification is switched off.
func TestValidateDocumentReadsNoFile(t *testing.T) {
	dir := t.TempDir()
	cfg := documentConfig()
	cfg.Security.TLS.Server.Enabled = true
	cfg.Security.TLS.Server.CertFile = filepath.Join(dir, "server.crt")
	cfg.Security.TLS.Server.KeyFile = filepath.Join(dir, "server.key")
	cfg.Security.TLS.Client.CAFiles = []string{filepath.Join(dir, "ca.pem")}
	cfg.Security.TLS.Client.InsecureSkipVerify = true

	var err error
	stderr := captureStderr(t, func() { err = cfg.ValidateDocument() })
	if err != nil {
		t.Errorf("ValidateDocument read the host's files: %v", err)
	}
	if stderr != "" {
		t.Errorf("ValidateDocument wrote to standard error: %q", stderr)
	}

	// The document checks still run: a missing required field is refused.
	cfg.Security.TLS.Server.KeyFile = ""
	if err := cfg.ValidateDocument(); err == nil ||
		!strings.Contains(err.Error(), "tls.server.key_file is required when TLS is enabled") {
		t.Fatalf("ValidateDocument with no key file = %v, want the required-field error", err)
	}
}

// TestConfigValidateRefusesMissingTLSFiles — Validate still checks that each
// configured TLS file exists, naming the first one missing.
func TestConfigValidateRefusesMissingTLSFiles(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "server.crt")
	key := filepath.Join(dir, "server.key")
	ca := filepath.Join(dir, "ca.pem")
	cfg := documentConfig()
	cfg.Security.TLS.Server.Enabled = true
	cfg.Security.TLS.Server.CertFile = cert
	cfg.Security.TLS.Server.KeyFile = key
	cfg.Security.TLS.Client.CAFiles = []string{ca}

	for _, step := range []struct {
		create string // file created before this step, "" for none
		want   string // the field the error names, "" for no error
	}{
		{create: "", want: "tls.server.cert_file"},
		{create: cert, want: "tls.server.key_file"},
		{create: key, want: "tls.client.ca_files[0]"},
		{create: ca, want: ""},
	} {
		if step.create != "" {
			if err := os.WriteFile(step.create, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		err := cfg.Validate()
		switch {
		case step.want == "" && err != nil:
			t.Fatalf("with every file present, Validate = %v", err)
		case step.want != "" && (err == nil || !strings.HasPrefix(err.Error(), "security configuration: "+step.want+": ")):
			t.Fatalf("Validate = %v, want an error naming %s", err, step.want)
		}
	}
}

// TestConfigValidateNamesFirstInvalidComponentByName — with several invalid
// component entries, every call names the first in instance-name order.
func TestConfigValidateNamesFirstInvalidComponentByName(t *testing.T) {
	bad := types.ComponentConfig{Name: "src", Type: "bogus", Enabled: true, Config: json.RawMessage(`{}`)}
	const want = `component bad1: ComponentType.Validate: invalid component type: "bogus" failed: invalid configuration`
	for run := 0; run < 50; run++ {
		cfg := documentConfig()
		cfg.Components = ComponentConfigs{"bad3": bad, "bad1": bad, "bad2": bad}
		if err := cfg.Validate(); err == nil || err.Error() != want {
			t.Fatalf("run %d: Validate = %v, want %q", run, err, want)
		}
	}
}
