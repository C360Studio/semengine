package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// pinStore is a bare repository in a temporary directory outside the repository, holding the
// commits fetched for one run.
type pinStore struct{ dir string }

// gitEnv runs git without the user's or the system's configuration, without inherited GIT_*
// variables, and without a terminal prompt: the SHA fixes what is read, so nothing configured
// on the host may change where or how it is read.
func gitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0")
}

func (p pinStore) git(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", p.dir, "--literal-pathspecs"}, args...)...)
	cmd.Env = gitEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// fetchPins fetches each sha from remote into a new store under tmp. All the fetches share one
// bound: once it is spent, no further fetch starts. It returns one problem per sha that could
// not be fetched.
func fetchPins(ctx context.Context, remote string, bound time.Duration, tmp string, shas []string) (pinStore, []string) {
	store := pinStore{dir: filepath.Join(tmp, "pin.git")}
	if _, err := (pinStore{dir: tmp}).git(ctx, "init", "--quiet", "--bare", store.dir); err != nil {
		return store, []string{err.Error()}
	}
	fetchCtx, cancel := context.WithTimeout(ctx, bound)
	defer cancel()
	var problems []string
	for _, sha := range shas {
		if ctx.Err() != nil {
			problems = append(problems, fmt.Sprintf("%s: not fetched: interrupted", sha))
			continue
		}
		if fetchCtx.Err() != nil {
			problems = append(problems, fmt.Sprintf("%s: not fetched: the fetch bound of %s for the whole run was spent", sha, bound))
			continue
		}
		if err := store.fetch(fetchCtx, remote, sha, tmp); err != nil {
			switch {
			case errors.Is(fetchCtx.Err(), context.DeadlineExceeded):
				problems = append(problems, fmt.Sprintf("%s: the fetch from %s did not finish within the fetch bound of %s", sha, remote, bound))
			default:
				problems = append(problems, fmt.Sprintf("%s: %v", sha, err))
			}
		}
	}
	return store, problems
}

// fetch runs one shallow fetch. git runs in a process group of its own, and the whole group is
// killed when the bound cuts the fetch off and again once git has exited: a transport helper git
// started (git-remote-https) does not die with git and would otherwise outlive the program.
func (p pinStore) fetch(ctx context.Context, remote, sha, tmp string) error {
	errFile, err := os.CreateTemp(tmp, "fetch-*.err")
	if err != nil {
		return err
	}
	defer errFile.Close()
	cmd := exec.CommandContext(ctx, "git", "-C", p.dir, "fetch", "--quiet", "--no-tags", "--depth", "1", "--end-of-options", remote, sha)
	cmd.Env = gitEnv()
	cmd.Stderr = errFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	err = cmd.Run()
	if cmd.Process != nil {
		// The group is git's pid; ESRCH means nothing in it is left.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if err != nil {
		msg, _ := os.ReadFile(errFile.Name())
		return fmt.Errorf("git fetch %s %s: %w: %s", remote, sha, err, strings.TrimSpace(string(msg)))
	}
	return nil
}

type pinObject struct {
	kind string // "blob" or "tree"
	oid  string
	path string // full path at the pin
}

// list runs ls-tree at sha for one path and parses its entries.
func (p pinStore) list(ctx context.Context, sha string, recursive bool, path string) ([]pinObject, error) {
	args := []string{"ls-tree", "-z"}
	if recursive {
		args = append(args, "-r")
	}
	out, err := p.git(ctx, append(args, "--end-of-options", sha, "--", path)...)
	if err != nil {
		return nil, err
	}
	var objects []pinObject
	for _, rec := range strings.Split(string(out), "\x00") {
		if rec == "" {
			continue
		}
		meta, name, ok := strings.Cut(rec, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			return nil, fmt.Errorf("git ls-tree %s %s: unexpected record %q", sha, path, rec)
		}
		objects = append(objects, pinObject{kind: fields[1], oid: fields[2], path: name})
	}
	return objects, nil
}

// stat reports what path is at sha; its kind is "" when it does not exist.
func (p pinStore) stat(ctx context.Context, sha, path string) (pinObject, error) {
	objects, err := p.list(ctx, sha, false, path)
	if err != nil {
		return pinObject{}, err
	}
	for _, o := range objects {
		if o.path == path {
			return o, nil
		}
	}
	return pinObject{path: path}, nil
}

// covered lists the files a directory entry covers at sha, keyed by path relative to the
// directory: the .go files directly in it and every file under its testdata directory.
func (p pinStore) covered(ctx context.Context, sha, dir string) (map[string]pinObject, error) {
	files := map[string]pinObject{}
	direct, err := p.list(ctx, sha, false, dir+"/")
	if err != nil {
		return nil, err
	}
	for _, o := range direct {
		if o.kind == "blob" && coveredName(strings.TrimPrefix(o.path, dir+"/")) {
			files[strings.TrimPrefix(o.path, dir+"/")] = o
		}
	}
	testdata, err := p.list(ctx, sha, true, dir+"/testdata/")
	if err != nil {
		return nil, err
	}
	for _, o := range testdata {
		if o.kind == "blob" {
			files[strings.TrimPrefix(o.path, dir+"/")] = o
		}
	}
	return files, nil
}

func (p pinStore) read(ctx context.Context, o pinObject) ([]byte, error) {
	return p.git(ctx, "cat-file", "blob", o.oid)
}
