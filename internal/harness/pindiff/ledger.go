package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const ledgerPath = "docs/admission-ledger.yaml"

// entry holds the four ledger fields the comparison reads. The schema, and the other six fields,
// belong to ledgerViolations in internal/harness/contract; this program does not validate.
type entry struct {
	SourcePath  string `yaml:"source_path"`
	SourceSHA   string `yaml:"source_sha"`
	Destination string `yaml:"destination"`
	Disposition string `yaml:"disposition"`
}

func readLedger(root string) ([]entry, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ledgerPath)))
	if err != nil {
		return nil, err
	}
	var entries []entry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	for i := range entries {
		e := &entries[i]
		e.SourcePath, e.SourceSHA = strings.TrimSpace(e.SourcePath), strings.TrimSpace(e.SourceSHA)
		e.Destination, e.Disposition = strings.TrimSpace(e.Destination), strings.TrimSpace(e.Disposition)
	}
	return entries, nil
}

// destPrefix is the destination path: the leading run of letters, digits, `/`, `.`, `_` and `-`;
// anything after it is a note.
var destPrefix = regexp.MustCompile(`^[A-Za-z0-9/._-]*`)

// treeSide is where an entry lands in the tree.
type treeSide struct {
	path    string // the destination path, relative to the repository root
	abs     string // its absolute path; empty when it names nothing in the repository
	isDir   bool
	problem string // why it names nothing; empty when abs is set
}

// resolveDestination finds an entry's destination path in the tree. One that is empty, starts
// with `/`, has a `..` segment, or resolves outside the repository counts as a path that does not
// exist.
func resolveDestination(root, destination string) treeSide {
	p := strings.TrimRight(destPrefix.FindString(destination), "/")
	side := treeSide{path: p}
	if p == "" || strings.HasPrefix(destination, "/") || slices.Contains(strings.Split(p, "/"), "..") {
		side.problem = fmt.Sprintf("destination path %q is not a path inside this repository", p)
		return side
	}
	rootAbs, err := filepath.Abs(root)
	if err == nil {
		rootAbs, err = filepath.EvalSymlinks(rootAbs)
	}
	if err != nil {
		side.problem = fmt.Sprintf("the repository root could not be resolved: %v", err)
		return side
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(rootAbs, filepath.FromSlash(p)))
	if err != nil {
		side.problem = fmt.Sprintf("destination path %q does not exist in this repository", p)
		return side
	}
	if rel, err := filepath.Rel(rootAbs, resolved); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		side.problem = fmt.Sprintf("destination path %q resolves outside this repository", p)
		return side
	}
	info, err := os.Stat(resolved)
	if err != nil {
		side.problem = fmt.Sprintf("destination path %q could not be read: %v", p, err)
		return side
	}
	side.abs, side.isDir = resolved, info.IsDir()
	return side
}

// movedPackages is the set the rewrite moves for files at sha: every carry or adapt entry at that
// sha whose source_path is a directory at the pin and whose destination path is a directory in
// the tree that differs from source_path. No other disposition moves anything. pinIsDir answers
// whether a path is a directory at sha.
func movedPackages(root string, entries []entry, sha string, pinIsDir func(path string) (bool, error)) (map[string]string, error) {
	moved := map[string]string{}
	for _, e := range entries {
		if e.SourceSHA != sha || (e.Disposition != "carry" && e.Disposition != "adapt") {
			continue
		}
		side := resolveDestination(root, e.Destination)
		if side.abs == "" || !side.isDir || side.path == e.SourcePath {
			continue
		}
		isDir, err := pinIsDir(e.SourcePath)
		if err != nil {
			return nil, err
		}
		if isDir {
			moved[e.SourcePath] = side.path
		}
	}
	return moved, nil
}
