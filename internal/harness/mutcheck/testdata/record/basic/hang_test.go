package basic

import (
	"os"
	"testing"
)

// TestHang blocks, with no assertion, when Value is wrong: the shape of a wrong change that
// hangs a test with no deadline of its own.
func TestHang(t *testing.T) {
	if Value() != 1 {
		<-make(chan struct{})
	}
}

// TestLoggedTimeout passes after printing go test's timeout panic text, as a test that copies a
// child go test's output would.
func TestLoggedTimeout(t *testing.T) {
	_, _ = os.Stdout.WriteString("panic: test timed out after 1s\n")
}
