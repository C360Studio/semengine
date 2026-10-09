package errs

import (
	"context"
	"fmt"
	"testing"
)

// IsTransient and IsFatal read an error's class and its sentinels, never its
// text (owner ruling N, #91 comment 6080973822, extended to IsFatal in comment
// 6083704900; #146): an error's text can carry an entity ID or a peer's message,
// and a word in it must not decide whether the work is retried or stopped.

// textOnlyError carries text in an unclassified error that wraps no sentinel.
func textOnlyError(text string) error {
	return fmt.Errorf("stored entity c360.ops.robotics.gcs.drone.001 refused: %s", text)
}

func TestIsTransientClassifiesByClassAndSentinel(t *testing.T) {
	typed := []struct {
		name string
		err  error
	}{
		{"classified transient", WrapTransient(fmt.Errorf("publish"), "Component", "Op", "publish")},
		{"ErrConnectionTimeout", ErrConnectionTimeout},
		{"ErrConnectionLost", ErrConnectionLost},
		{"ErrStorageUnavailable", ErrStorageUnavailable},
		{"ErrRateLimited", ErrRateLimited},
		{"ErrCircuitOpen", ErrCircuitOpen},
		{"context.DeadlineExceeded", context.DeadlineExceeded},
		{"context.Canceled", context.Canceled},
	}
	for _, tc := range typed {
		if wrapped := fmt.Errorf("handler: %w", tc.err); !IsTransient(wrapped) {
			t.Errorf("%s wrapped with %%w: IsTransient = false, want true", tc.name)
		}
	}

	// The class decides, whatever the text holds.
	fatal := WrapFatal(fmt.Errorf("connection timeout, network unavailable"), "Component", "Op", "retry")
	if IsTransient(fmt.Errorf("handler: %w", fatal)) {
		t.Errorf("IsTransient(%q) = true, want false: its class is fatal", fatal)
	}

	// Each word the pin's text match read (errs.go:177-193 at 8b99efe9).
	for _, word := range []string{"timeout", "connection", "network", "temporary", "unavailable", "busy", "retry"} {
		if err := textOnlyError(word); IsTransient(err) {
			t.Errorf("IsTransient(%q) = true, want false: a text match decided the class", err)
		}
	}
}

func TestIsFatalClassifiesByClassAndSentinel(t *testing.T) {
	typed := []struct {
		name string
		err  error
	}{
		{"classified fatal", WrapFatal(fmt.Errorf("open store"), "Component", "Op", "open")},
		{"ErrInvalidConfig", ErrInvalidConfig},
		{"ErrMissingConfig", ErrMissingConfig},
		{"ErrDataCorrupted", ErrDataCorrupted},
		{"ErrStorageFull", ErrStorageFull},
		{"ErrResourceExhausted", ErrResourceExhausted},
		{"ErrQuotaExceeded", ErrQuotaExceeded},
	}
	for _, tc := range typed {
		if wrapped := fmt.Errorf("handler: %w", tc.err); !IsFatal(wrapped) {
			t.Errorf("%s wrapped with %%w: IsFatal = false, want true", tc.name)
		}
	}

	// The class decides, whatever the text holds.
	transient := WrapTransient(fmt.Errorf("fatal: data corrupted, disk full"), "Component", "Op", "panic")
	if IsFatal(fmt.Errorf("handler: %w", transient)) {
		t.Errorf("IsFatal(%q) = true, want false: its class is transient", transient)
	}

	// Each word the pin's text match read (errs.go:220-236 at 8b99efe9).
	words := []string{"fatal", "panic", "corrupted", "invalid config", "missing config", "out of memory", "disk full"}
	for _, word := range words {
		if err := textOnlyError(word); IsFatal(err) {
			t.Errorf("IsFatal(%q) = true, want false: a text match decided the class", err)
		}
	}
}
