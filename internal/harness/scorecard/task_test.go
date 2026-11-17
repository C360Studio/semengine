package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// `task -x scorecard:tier1` exits with the program's own status. A refused run is the cheap
// failure to reach through the real Taskfile: it starts nothing and must exit 2, where `go run`
// would have flattened it to 1, a floor miss. Exit 1 through the built program is
// TestFloorMissExitsOne.
func TestTaskPassesTheStatusThrough(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("task", "-x", "scorecard:tier1")
	cmd.Dir = root
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("task -x scorecard:tier1 with no flags: %v, want exit status 2\n%s", err, out)
	}
	if !strings.Contains(string(out), "-url is required") {
		t.Errorf("output does not name the refusal:\n%s", out)
	}
}
