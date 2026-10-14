package natsfixture

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
)

// stdout runs cmd and returns what it wrote to standard output, trimmed. The docker CLI writes
// progress and warnings to standard error even when it succeeds (on a cold image cache, `docker
// run -d` reports the image pull there and the container ID on standard output), so only standard
// output is data. Standard error is kept for the error when the command fails.
func stdout(cmd *exec.Cmd) (string, error) {
	out, err := cmd.Output()
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		err = fmt.Errorf("%w\n%s", err, exit.Stderr)
	}
	return strings.TrimSpace(string(out)), err
}

// #229: a command that succeeds and writes a warning to standard error, as `docker run -d` does
// when it pulls the image first, still yields only the ID it wrote to standard output.
func TestStdoutIgnoresWarningsOnSuccess(t *testing.T) {
	got, err := stdout(exec.CommandContext(t.Context(), "sh", "-c",
		`echo "Unable to find image 'nats:latest' locally" >&2; echo 4f2a9c`))
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	if got != "4f2a9c" {
		t.Fatalf("stdout returned %q, want only the ID written to standard output, 4f2a9c", got)
	}
}

// A failed command's standard error reaches the error, so a test failure says why docker failed.
func TestStdoutKeepsStderrOnFailure(t *testing.T) {
	_, err := stdout(exec.CommandContext(t.Context(), "sh", "-c", `echo "no such image" >&2; exit 3`))
	if err == nil || !strings.Contains(err.Error(), "no such image") {
		t.Fatalf("stdout of a failed command returned error %v, want one carrying its standard error", err)
	}
}
