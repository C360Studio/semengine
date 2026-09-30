package natsfixture

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// admissionEnv lists every variable the runner exports to the fixture. Tests here set them
// explicitly with t.Setenv, so they cannot run in parallel and do not depend on how they were run.
var admissionEnv = []string{
	"SEMENGINE_DOCKER_ADMISSION_TOKEN", "SEMENGINE_DOCKER_ADMISSION_LOCK_DIR", "SEMENGINE_EVIDENCE_DIR", "SEMENGINE_NATS_IMAGE",
}

// plantLock writes a lock owner record holding token and points the fixture's environment at it.
func plantLock(t *testing.T, ownerToken, envToken string) {
	t.Helper()
	lock := filepath.Join(t.TempDir(), "lock")
	if err := os.Mkdir(lock, 0o755); err != nil {
		t.Fatal(err)
	}
	owner := "host=h\npid=1\nstarted=1\nidentity=x\ntoken=" + ownerToken + "\ncommand=semengine /x/scripts/test-integration.sh\n"
	if err := os.WriteFile(filepath.Join(lock, "owner"), []byte(owner), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SEMENGINE_DOCKER_ADMISSION_TOKEN", envToken)
	t.Setenv("SEMENGINE_DOCKER_ADMISSION_LOCK_DIR", lock)
	t.Setenv("SEMENGINE_EVIDENCE_DIR", t.TempDir())
	t.Setenv("SEMENGINE_NATS_IMAGE", "nats"+":2.14.7-alpine@sha256:"+strings.Repeat("0", 64))
}

// S1-9: Start refuses without admission and makes no Docker or NATS call. The environment
// variable alone is never trusted: its token must be in the live owner file.
func TestS1_9AdmissionRefusal(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T)
		want  string
	}{
		{"no runner environment", func(t *testing.T) {
			for _, v := range admissionEnv {
				t.Setenv(v, "")
			}
		}, "SEMENGINE_DOCKER_ADMISSION_TOKEN"},
		{"token absent from the owner file", func(t *testing.T) { plantLock(t, "runner-token", "forged-token") }, "not in"},
		{"token is a prefix of the owner's", func(t *testing.T) { plantLock(t, "runner-token-2", "runner-token") }, "not in"},
		{"no owner file", func(t *testing.T) {
			plantLock(t, "tok", "tok")
			if err := os.Remove(filepath.Join(os.Getenv("SEMENGINE_DOCKER_ADMISSION_LOCK_DIR"), "owner")); err != nil {
				t.Fatal(err)
			}
		}, "owner"},
		{"no evidence directory", func(t *testing.T) {
			plantLock(t, "tok", "tok")
			t.Setenv("SEMENGINE_EVIDENCE_DIR", "")
		}, "SEMENGINE_EVIDENCE_DIR"},
		{"no image", func(t *testing.T) {
			plantLock(t, "tok", "tok")
			t.Setenv("SEMENGINE_NATS_IMAGE", "")
		}, "SEMENGINE_NATS_IMAGE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)
			f := New(t)
			err := f.Start(t.Context())
			if !errors.Is(err, ErrNotAdmitted) {
				t.Fatalf("Start = %v, want ErrNotAdmitted", err)
			}
			for _, want := range []string{tc.want, "task test:integration -- "} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err, want)
				}
			}
			if calls := f.totalCalls(); calls != 0 {
				t.Errorf("refused Start made %d dependency call(s): %v", calls, f.callCounts())
			}
			if rem := f.remaining(); len(rem) != 0 {
				t.Errorf("refused Start retained %v", rem)
			}
			// A refusal before acting consumes nothing: Stop is a no-op, and the fixture can
			// still be started once admitted.
			if err := f.Stop(t.Context()); err != nil {
				t.Errorf("Stop after refusal = %v", err)
			}
		})
	}
}
