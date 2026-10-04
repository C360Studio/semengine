package early

import (
	"os"
	"syscall"
	"testing"
)

// TestMain kills its own process before any test starts.
func TestMain(m *testing.M) {
	_ = syscall.Kill(syscall.Getpid(), syscall.SIGKILL)
	os.Exit(m.Run())
}

func TestEarly(t *testing.T) {}
