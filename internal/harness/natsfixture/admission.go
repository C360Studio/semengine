package natsfixture

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// admission is what the integration runner hands a test process: proof it holds the host lock,
// where to write evidence, and the one image pin.
type admission struct {
	evidenceDir string
	image       string
}

var digestImage = regexp.MustCompile(`^nats(:[A-Za-z0-9][A-Za-z0-9._-]*)?@sha256:[0-9a-f]{64}$`)

// admit reads the runner's environment and requires its token in the live lock owner file. The
// environment alone is never trusted: a stale shell export must not admit a direct `go test`.
// It makes no Docker call.
func admit() (admission, error) {
	var missing []string
	get := func(name string) string {
		v := os.Getenv(name)
		if v == "" {
			missing = append(missing, name)
		}
		return v
	}
	token := get("SEMENGINE_DOCKER_ADMISSION_TOKEN")
	lockDir := get("SEMENGINE_DOCKER_ADMISSION_LOCK_DIR")
	a := admission{evidenceDir: get("SEMENGINE_EVIDENCE_DIR"), image: get("SEMENGINE_NATS_IMAGE")}
	if len(missing) > 0 {
		return admission{}, fmt.Errorf("%w (unset: %v)", ErrNotAdmitted, missing)
	}
	owner, err := os.Open(filepath.Join(lockDir, "owner"))
	if err != nil {
		return admission{}, fmt.Errorf("%w (no live lock owner file: %v)", ErrNotAdmitted, err)
	}
	defer func() { _ = owner.Close() }()
	held := false
	scanner := bufio.NewScanner(owner)
	for scanner.Scan() {
		if scanner.Text() == "token="+token {
			held = true
		}
	}
	if err := scanner.Err(); err != nil {
		return admission{}, fmt.Errorf("%w (read lock owner file: %v)", ErrNotAdmitted, err)
	}
	if !held {
		return admission{}, fmt.Errorf("%w (SEMENGINE_DOCKER_ADMISSION_TOKEN is not in %s/owner; the lock is not held for this run)",
			ErrNotAdmitted, lockDir)
	}
	return a, nil
}
