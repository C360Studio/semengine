package composition_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/composition"
	"github.com/c360studio/semengine/config"
	"github.com/c360studio/semengine/types"
)

func marshalValidate(t *testing.T, registry *component.Registry, cfg *config.Config) []byte {
	t.Helper()
	result, err := composition.Validate(registry, cfg)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	return data
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

// TestValidateReadsNoTLSFile is Codex's #347 probe: the result does not depend
// on whether a configured CA file exists on the machine running validation,
// and validation writes nothing to standard error.
func TestValidateReadsNoTLSFile(t *testing.T) {
	registry := fakeRegistry(t)
	cfg := compositionOf(config.ComponentConfigs{})
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	cfg.Security.TLS.Client.CAFiles = []string{caFile}

	var absent, present []byte
	stderr := captureStderr(t, func() {
		absent = marshalValidate(t, registry, cfg)
		if err := os.WriteFile(caFile, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		present = marshalValidate(t, registry, cfg)
		cfg.Security.TLS.Client.InsecureSkipVerify = true
		marshalValidate(t, registry, cfg)
	})
	if !bytes.Equal(absent, present) {
		t.Errorf("result depends on the CA file:\nabsent:  %s\npresent: %s", absent, present)
	}
	if stderr != "" {
		t.Fatalf("Validate wrote to standard error: %q", stderr)
	}
}

// TestValidateConfigInvalidIsDeterministic is Codex's #348 probe: with several
// invalid component entries, the config_invalid finding names the first in
// instance-name order on every run.
func TestValidateConfigInvalidIsDeterministic(t *testing.T) {
	registry := fakeRegistry(t)
	bad := types.ComponentConfig{Name: "src", Type: "bogus", Enabled: true, Config: json.RawMessage(`{}`)}
	cfg := compositionOf(config.ComponentConfigs{"bad3": bad, "bad1": bad, "bad2": bad})
	const want = `component bad1: ComponentType.Validate: invalid component type: "bogus" failed: invalid configuration`

	first := marshalValidate(t, registry, cfg)
	for run := 1; run < 50; run++ {
		if data := marshalValidate(t, registry, cfg); !bytes.Equal(first, data) {
			t.Fatalf("run %d differs from run 0:\n%s\n---\n%s", run, first, data)
		}
	}
	var result composition.Result
	if err := json.Unmarshal(first, &result); err != nil {
		t.Fatal(err)
	}
	invalid := findingsOfType(result.Errors, composition.TypeConfigInvalid)
	if len(invalid) != 1 || invalid[0].Message != want {
		t.Fatalf("config_invalid = %+v, want one with message %q", invalid, want)
	}
}
