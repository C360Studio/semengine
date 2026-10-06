package basic

import (
	"syscall"
	"testing"
)

func TestValue(t *testing.T) {
	if got := Value(); got != 1 {
		t.Fatalf("Value() = %d, want 1", got)
	}
}

func TestLogThenFail(t *testing.T) {
	t.Logf("checking Value")
	if got := Value(); got != 1 {
		t.Errorf("Value() = %d, want 1", got)
	}
}

func TestPanicAfter(t *testing.T) {
	if got := Value(); got != 1 {
		t.Errorf("Value() = %d, want 1", got)
	}
	var m map[string]int
	if Value() != 1 {
		m["x"] = 1
	}
}

func TestOuter(t *testing.T) {
	t.Run("inner", func(t *testing.T) {
		if got := Value(); got != 1 {
			t.Errorf("Value() = %d, want 1", got)
		}
	})
}

func TestRace(t *testing.T) {
	x := 0
	done := make(chan struct{})
	go func() {
		x++
		close(done)
	}()
	x++
	<-done
	_ = x
}

func TestTimeout(t *testing.T) {
	if got := Value(); got != 1 {
		t.Errorf("Value() = %d, want 1", got)
		<-make(chan struct{})
	}
}

func TestKilled(t *testing.T) {
	if Value() != 1 {
		_ = syscall.Kill(syscall.Getpid(), syscall.SIGKILL)
	}
}
