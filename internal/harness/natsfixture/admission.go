package natsfixture

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// admission is what the integration runner hands a test process: proof it holds the host lock,
// where to write evidence, and the one image pin.
type admission struct {
	evidenceDir string
	image       string
}

var digestImage = regexp.MustCompile(`^nats(:[A-Za-z0-9][A-Za-z0-9._-]*)?@sha256:[0-9a-f]{64}$`)

// admit reads the runner's environment and requires its token in the lock owner file, and that
// owner to be live by its identity_utc, or by its identity when identity_utc is absent or empty
// (ownerLive). The environment alone is never trusted: a stale shell export must not admit a
// direct `go test`, nor one whose runner died and left its pid to another process. If ctx ends
// before ps can read the owner's start time, admit returns ctx's error. It makes no Docker call.
func admit(ctx context.Context) (admission, error) {
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
	fields := map[string]string{}
	scanner := bufio.NewScanner(owner)
	for scanner.Scan() {
		if scanner.Text() == "token="+token {
			held = true
		}
		if k, v, ok := strings.Cut(scanner.Text(), "="); ok {
			fields[k] = v
		}
	}
	if err := scanner.Err(); err != nil {
		return admission{}, fmt.Errorf("%w (read lock owner file: %v)", ErrNotAdmitted, err)
	}
	if !held {
		return admission{}, fmt.Errorf("%w (SEMENGINE_DOCKER_ADMISSION_TOKEN is not in %s/owner; the lock is not held for this run)",
			ErrNotAdmitted, lockDir)
	}
	if err := ownerLive(ctx, fields["host"], fields["pid"], fields["identity"], fields["identity_utc"]); err != nil {
		// ownerLive returns ctx's own error when ctx ended before ps answered: the caller was
		// cancelled, not refused.
		if errors.Is(err, ctx.Err()) {
			return admission{}, err
		}
		return admission{}, fmt.Errorf("%w (%s/owner: %v)", ErrNotAdmitted, lockDir, err)
	}
	return a, nil
}

// ownerLive requires the recorded owner to be live as the integration runner's spec defines it
// ("Shared host lock"): on this host, with ps printing the recorded start time for its pid. A
// record whose identity_utc is not empty is judged by it, read with TZ=UTC and LC_ALL=C as the
// runner wrote it, so a test process in another time zone or locale reads a live owner as live.
// Otherwise the record is judged by its identity, read in this process's environment. Only
// SemEngine's runner exports the admission token, so that fallback serves a SemEngine runner older
// than identity_utc, met by a direct `go test` from a newer worktree with a copied token, and the
// six-key records tests plant.
//
// A runner killed with SIGKILL never runs its EXIT trap, so its owner file, token included,
// outlives it, and its pid may be given to another process, whose start time is not the record's;
// the next runner will quarantine that lock. An `unknown` value is never live, since ps never
// prints it. The runner and its tests always share a host, so an owner elsewhere admits nothing.
func ownerLive(ctx context.Context, host, pid, identity, identityUTC string) error {
	here, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("hostname: %w", err)
	}
	if host != here {
		return fmt.Errorf("owner host %q is another host (this is %q)", host, here)
	}
	n, err := strconv.Atoi(pid)
	if err != nil || n <= 0 {
		return fmt.Errorf("owner pid %q is not a process id", pid)
	}
	key, recorded := "identity", identity
	var env []string // this process's environment, unchanged
	if identityUTC != "" {
		key, recorded = "identity_utc", identityUTC
		env = []string{"TZ=UTC", "LC_ALL=C"}
	}
	started, err := startTime(ctx, n, env...)
	if err != nil {
		// A ps that exec killed because ctx ended fails with "signal: killed", which does not say
		// why: the caller is told its context ended, not that the owner is dead.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// %v, not %w, so a context error reaches admit only through the ctx.Err() check above.
		return fmt.Errorf("owner pid %d is not live: read its start time: %v", n, err)
	}
	// An empty read proves nothing live, and would equal a record that has no identity.
	if started == "" {
		return fmt.Errorf("owner pid %d is not live: ps printed no start time", n)
	}
	if started != recorded {
		return fmt.Errorf("owner pid %d started at %q, not at the recorded %s %q", n, started, key, recorded)
	}
	return nil
}

// startTime reads pid's start time as the runner records its own (scripts/test-integration.sh's
// owner_identity, or owner_identity_utc when env is TZ=UTC and LC_ALL=C): ps's lstart column, read
// with env added to this process's environment, with its newline and leading blanks removed and
// any trailing blanks kept, since macOS's ps pads the column and the record keeps the padding.
func startTime(ctx context.Context, pid int, env ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	// os/exec keeps the last value of a repeated key, so env overrides this process's own.
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimLeft(strings.TrimSuffix(string(out), "\n"), " \t"), nil
}
