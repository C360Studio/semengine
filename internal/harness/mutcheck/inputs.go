package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// inputs are the program's flags (mutation-check › "The command and its inputs").
type inputs struct {
	pkg, test, file, mutant string
	expect                  expectation
	seed                    uint64
	timeout                 time.Duration
	runs                    int
}

// refusal is a reason the program starts no run. It names the input and why.
type refusal struct{ input, reason string }

func (r refusal) Error() string { return r.input + ": " + r.reason }

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

var expectedLocation = regexp.MustCompile(`^[^\s:/]+\.go:[1-9][0-9]*$`)

const manual = `the manual procedure of docs/testing.md, "Show that the test can fail", applies to it`

// parseInputs reads the flags. It reads nothing outside its arguments.
func parseInputs(args []string, usage io.Writer) (inputs, error) {
	var in inputs
	var locations, texts listFlag
	fs := flag.NewFlagSet("mutcheck", flag.ContinueOnError)
	fs.SetOutput(usage)
	fs.StringVar(&in.pkg, "pkg", "", "one package, as go test takes it")
	fs.StringVar(&in.test, "test", "", "a top-level test, or Name/Sub for one of its subtests; each part is matched exactly")
	fs.StringVar(&in.file, "file", "", "the Go source file the wrong change is made to")
	fs.StringVar(&in.mutant, "mutant", "", "a copy of that file, outside the repository, with the wrong change made in it")
	fs.Var(&locations, "expect", "a location file.go:N as Go prints it on an expected line (repeatable)")
	fs.Var(&texts, "expect-text", "a fixed string an expected line contains (repeatable)")
	fs.Uint64Var(&in.seed, "seed", 1, "RAPID_SEED for every run")
	fs.DurationVar(&in.timeout, "timeout", 2*time.Minute, "the go test -timeout of each run")
	fs.IntVar(&in.runs, "runs", 3, "the number of baseline runs and of mutant runs")
	if err := fs.Parse(args); err != nil {
		return in, refusal{"flags", err.Error()}
	}
	in.expect = expectation{locations: locations, texts: texts}
	switch {
	case fs.NArg() > 0:
		return in, refusal{"arguments", fmt.Sprintf("%q is not a flag; every input is a flag", fs.Arg(0))}
	case in.pkg == "":
		return in, refusal{"-pkg", "one package is named, and none was"}
	case strings.Contains(in.pkg, "..."):
		return in, refusal{"-pkg", fmt.Sprintf("%q is a pattern; one package is named", in.pkg)}
	case in.test == "" || strings.Count(in.test, "/") > 1 || strings.HasPrefix(in.test, "/") || strings.HasSuffix(in.test, "/"):
		return in, refusal{"-test", fmt.Sprintf("%q is not a top-level test, or a top-level test and one of its subtests written Name/Sub", in.test)}
	case in.file == "":
		return in, refusal{"-file", "the file the wrong change is made to is named, and none was"}
	case in.mutant == "":
		return in, refusal{"-mutant", "the copy with the wrong change is named, and none was"}
	case len(locations) == 0 && len(texts) == 0:
		return in, refusal{"-expect and -expect-text", "at least one is given, naming the assertion the wrong change is meant to make fail"}
	case in.seed == 0:
		return in, refusal{"-seed", `0 is what Rapid takes as "choose a random seed", and the runs of one check would differ`}
	case in.timeout <= 0:
		return in, refusal{"-timeout", "the timeout of a run is positive"}
	case in.runs < 1:
		return in, refusal{"-runs", "at least one baseline run and one mutant run are made"}
	}
	for _, loc := range locations {
		if !expectedLocation.MatchString(loc) {
			return in, refusal{"-expect", fmt.Sprintf("%q is not a location written file.go:N, as Go prints it", loc)}
		}
	}
	for _, text := range texts {
		if text == "" {
			return in, refusal{"-expect-text", "an empty text would match every line"}
		}
	}
	return in, nil
}

// prepared is what the program learned about the tree before its first run.
type prepared struct {
	root      string // the module's root, symlinks resolved
	target    string // the target, relative to root, with slashes
	targetSrc []byte
	mutantSrc []byte
	pkgDir    string // the package's directory, relative to root
	tmpBase   string // where the program's temporary directory is made
	goflags   string // the caller's GOFLAGS, as go env prints it
	goversion string
}

// prepare makes every check of "The command and its inputs" that reads the tree, git or go env,
// and refuses before any run.
func prepare(ctx context.Context, root string, in inputs, environ []string) (prepared, error) {
	var p prepared
	var err error
	if p.root, err = filepath.Abs(root); err == nil {
		p.root, err = filepath.EvalSymlinks(p.root)
	}
	if err != nil {
		return p, refusal{"working directory", err.Error()}
	}
	modData, err := os.ReadFile(filepath.Join(p.root, "go.mod"))
	if err != nil {
		return p, refusal{"working directory", "the program runs from the repository's root, where go.mod is, and " + err.Error()}
	}

	target, err := resolve(p.root, in.file)
	switch {
	case err != nil:
		return p, refusal{"-file " + in.file, "does not exist"}
	case !within(p.root, target):
		return p, refusal{"-file " + in.file, "is outside the module " + p.root}
	case strings.HasSuffix(target, "_test.go"):
		return p, refusal{"-file " + in.file, "is a test file: the test, its inputs and its expectations stay unchanged; " + manual}
	case !strings.HasSuffix(target, ".go"):
		return p, refusal{"-file " + in.file, "only a Go source file that is not a test file can be the target; " + manual}
	}
	rel, _ := filepath.Rel(p.root, target)
	p.target = filepath.ToSlash(rel)
	if p.targetSrc, err = os.ReadFile(target); err != nil {
		return p, refusal{"-file " + in.file, err.Error()}
	}

	mutant, err := resolve(p.root, in.mutant)
	switch {
	case err != nil:
		return p, refusal{"-mutant " + in.mutant, "does not exist"}
	case within(p.root, mutant):
		return p, refusal{"-mutant " + in.mutant, "is inside the module; the copy with the wrong change is kept outside the repository"}
	}
	if p.mutantSrc, err = os.ReadFile(mutant); err != nil {
		return p, refusal{"-mutant " + in.mutant, err.Error()}
	}
	if bytes.Equal(p.mutantSrc, p.targetSrc) {
		return p, refusal{"-mutant " + in.mutant, "has the same content as the target: there is no wrong change"}
	}

	p.tmpBase = os.TempDir()
	if v, ok := lookup(environ, "TMPDIR"); ok && v != "" {
		p.tmpBase = v
	}
	base, err := filepath.EvalSymlinks(p.tmpBase)
	switch {
	case err != nil:
		return p, refusal{"TMPDIR " + p.tmpBase, "the program makes its files in a temporary directory that exists: " + err.Error()}
	case within(p.root, base):
		return p, refusal{"TMPDIR " + p.tmpBase, "is inside the module, so the program's files would be written into the repository; set TMPDIR to a directory outside it"}
	}

	if p.goflags, p.goversion, err = goEnv(ctx, p.root, environ); err != nil {
		return p, refusal{"go env", err.Error()}
	}
	if err := checkGoflags(p.goflags); err != nil {
		return p, err
	}

	if p.pkgDir, err = packageDir(p.root, modulePath(modData), in.pkg); err != nil {
		return p, err
	}
	out, err := runGit(ctx, p.root, environ, "ls-files", "--others", "-z", "--", p.pkgDir+"/testdata/rapid")
	if err != nil {
		return p, refusal{"git ls-files", err.Error()}
	}
	if names := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00"); names[0] != "" {
		return p, refusal{"-pkg " + in.pkg, fmt.Sprintf("a file that git does not track is under %s/testdata/rapid: %s. "+
			"Rapid replays every file there on every run, so a failure file an earlier run left would decide this check; "+
			"move it out of the tree, or commit it as a fixed input", p.pkgDir, strings.Join(names, ", "))}
	}
	return p, nil
}

// resolve makes path absolute against root and resolves its symlinks.
func resolve(root, path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err == nil && !info.Mode().IsRegular() {
		err = errors.New("not a regular file")
	}
	return path, err
}

// within reports whether path is root or lies under it; both are absolute and resolved.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func lookup(environ []string, key string) (string, bool) {
	for i := len(environ) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(environ[i], key+"="); ok {
			return v, true
		}
	}
	return "", false
}

// goEnv reads the caller's GOFLAGS as go env prints it in the program's environment, so a value
// set with go env -w counts (design D15, P22), and the Go version.
func goEnv(ctx context.Context, root string, environ []string) (string, string, error) {
	cmd := command(ctx, root, environ, "go", "env", "-json", "GOFLAGS", "GOVERSION")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var v struct{ GOFLAGS, GOVERSION string }
	if err := json.Unmarshal(out, &v); err != nil {
		return "", "", err
	}
	return v.GOFLAGS, v.GOVERSION, nil
}

// checkGoflags refuses a GOFLAGS that sets an overlay, which the program's own would conflict
// with, or coverage, under which Go builds the target from the file on disk (design P16, P17).
func checkGoflags(goflags string) error {
	fields, err := splitQuoted(goflags)
	if err != nil {
		return refusal{"GOFLAGS", fmt.Sprintf("%q cannot be read: %v", goflags, err)}
	}
	for _, f := range fields {
		name, _, _ := strings.Cut(strings.TrimLeft(f, "-"), "=")
		switch name {
		case "overlay":
			return refusal{"GOFLAGS", fmt.Sprintf("%q sets -overlay; the program applies the wrong change with an overlay of its own", goflags)}
		case "cover", "coverpkg", "covermode", "coverprofile":
			return refusal{"GOFLAGS", fmt.Sprintf("%q sets -%s; under coverage Go would build the target from the file on disk, and the wrong change would not run", goflags, name)}
		}
	}
	return nil
}

// splitQuoted splits a GOFLAGS value into fields as the go command does: on white space, with a
// field allowed to be wrapped in single or double quotes.
func splitQuoted(s string) ([]string, error) {
	var fields []string
	for s = strings.TrimSpace(s); s != ""; s = strings.TrimSpace(s) {
		if q := s[0]; q == '"' || q == '\'' {
			end := strings.IndexByte(s[1:], q)
			if end < 0 {
				return nil, errors.New("a quote is not closed")
			}
			fields, s = append(fields, s[1:1+end]), s[2+end:]
			continue
		}
		end := strings.IndexAny(s, " \t\n\r")
		if end < 0 {
			end = len(s)
		}
		fields, s = append(fields, s[:end]), s[end:]
	}
	return fields, nil
}

// quoteFlag quotes one GOFLAGS field when it holds white space or a quote, as splitQuoted reads it.
func quoteFlag(f string) (string, error) {
	switch {
	case !strings.ContainsAny(f, " \t\n\r'\""):
		return f, nil
	case !strings.Contains(f, "'"):
		return "'" + f + "'", nil
	case !strings.Contains(f, `"`):
		return `"` + f + `"`, nil
	}
	return "", fmt.Errorf("%q cannot be quoted in GOFLAGS", f)
}

// modulePath reads the module path from go.mod's module line.
func modulePath(gomod []byte) string {
	for _, line := range strings.Split(string(gomod), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			return strings.Trim(fields[1], "\"`")
		}
	}
	return ""
}

// packageDir maps -pkg, a relative path or an import path of the module, to the package's
// directory relative to root.
func packageDir(root, module, pkg string) (string, error) {
	var rel string
	switch {
	case pkg == "." || strings.HasPrefix(pkg, "./") || strings.HasPrefix(pkg, "../"):
		dir := filepath.Join(root, pkg)
		if !within(root, dir) {
			return "", refusal{"-pkg " + pkg, "is outside the module " + root}
		}
		rel, _ = filepath.Rel(root, dir)
	case module != "" && pkg == module:
		rel = "."
	case module != "" && strings.HasPrefix(pkg, module+"/"):
		rel = strings.TrimPrefix(pkg, module+"/")
	default:
		return "", refusal{"-pkg " + pkg, fmt.Sprintf("is not a package of the module %q; name it as ./<dir> or by its import path", module)}
	}
	return filepath.ToSlash(rel), nil
}
