package toolcheck

import (
	"bytes"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// harness-boundaries › "ateam template files are the pin's": every file `ateam init` writes into an
// empty directory at the pinned release is byte-identical in this repository, except the files
// SemEngine writes itself. ateam doctor --check does not cover this: at v0.3.3 it checks only that
// developer.md and reviewer.md still hold each ateam paragraph and that skills carry their section
// markers, so an added line, a deleted architect.md or an edited skill passes it.
//
// Init runs into an empty directory, not a copy of the tree: it never overwrites an existing
// file, so a copy of the repository gets nothing written to compare.

// semengineOwned are the paths init writes that SemEngine keeps its own text in, with the reason.
var semengineOwned = map[string]string{
	".gitignore":                        "the repository's own ignore list, with init's two lines appended",
	"AGENTS.md":                         "SemEngine's rules",
	"CLAUDE.md":                         "SemEngine's import of AGENTS.md",
	".agents/README.md":                 "SemEngine's agent profiles",
	".agents/protocol.md":               "SemEngine's shared work protocol",
	".agents/rules-index.md":            "SemEngine's rule index",
	".agents/skills/preflight/SKILL.md": "SemEngine's gate table, with ateam's section markers",
	".claude/skills/preflight/SKILL.md": "the adapter of SemEngine's preflight",
	"openspec/config.yaml":              "SemEngine's OpenSpec configuration",
	"openspec/changes/.gitkeep":         "openspec/changes/ holds SemEngine's changes",
	"openspec/specs/.gitkeep":           "openspec/specs/ holds SemEngine's specs",
}

// ateamPin matches an ateam install line in .github/workflows/ci.yml; every one names the same release.
var ateamPin = regexp.MustCompile(`ateam/cmd/ateam@(v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)`)

func TestAteamTemplatesUnchanged(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "ateam templates", ateamTemplateViolations(root, ateamTemplates(t, root)))
}

func TestAteamTemplatesUnchangedSensitivity(t *testing.T) {
	root := repoRoot(t)
	templates := ateamTemplates(t, root)
	base := make(map[string]string, len(templates))
	for rel := range templates {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		base[rel] = string(data)
	}
	clean, _ := writeTree(t, base)
	requireNoViolations(t, "clean fixture", ateamTemplateViolations(clean, templates))

	for _, tc := range []struct {
		name  string
		edit  func(map[string]string)
		wants []string
	}{
		{"a line added to a base contract", func(m map[string]string) {
			m[".agents/contracts/reviewer.md"] += "A SemEngine rule.\n"
		}, []string{".agents/contracts/reviewer.md", "differs"}},
		{"a base contract deleted", func(m map[string]string) {
			delete(m, ".agents/contracts/architect.md")
		}, []string{".agents/contracts/architect.md", "missing"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := make(map[string]string, len(base))
			for k, v := range base {
				files[k] = v
			}
			tc.edit(files)
			dir, _ := writeTree(t, files)
			requireViolation(t, ateamTemplateViolations(dir, templates), tc.wants...)
		})
	}
}

// ateamTemplates runs the pinned ateam init into an empty directory and returns every file it
// wrote that SemEngine does not own, by slash path. It fails, never skips, when ateam is missing
// or is not the pinned release.
func ateamTemplates(t *testing.T, root string) map[string][]byte {
	t.Helper()
	ci, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var versions []string
	for _, m := range ateamPin.FindAllSubmatch(ci, -1) {
		if v := string(m[1]); !slices.Contains(versions, v) {
			versions = append(versions, v)
		}
	}
	if len(versions) != 1 {
		t.Fatalf(".github/workflows/ci.yml: ateam install lines name versions %v, want one version", versions)
	}
	pin := versions[0]
	install := "GOTOOLCHAIN=auto go install github.com/c360studio/ateam/cmd/ateam@" + pin
	if _, err := exec.LookPath("ateam"); err != nil {
		t.Fatalf("ateam is not on PATH; install the pinned release: %s", install)
	}
	out, err := exec.CommandContext(t.Context(), "ateam", "version").CombinedOutput()
	if err != nil {
		t.Fatalf("ateam version: %v\n%s", err, out)
	}
	if !slices.Contains(strings.Fields(string(out)), pin) {
		t.Fatalf("ateam version reports %q, want the ci.yml pin %s; install it: %s", strings.TrimSpace(string(out)), pin, install)
	}

	dir := t.TempDir()
	if out, err := exec.CommandContext(t.Context(), "ateam", "init", "-dir", dir).CombinedOutput(); err != nil {
		t.Fatalf("ateam init: %v\n%s", err, out)
	}
	templates := map[string][]byte{}
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if _, owned := semengineOwned[rel]; owned {
			return nil
		}
		data, err := os.ReadFile(path)
		templates[rel] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) == 0 {
		t.Fatal("ateam init wrote no template file SemEngine does not own")
	}
	return templates
}

func ateamTemplateViolations(root string, templates map[string][]byte) []string {
	var out []string
	for _, rel := range slices.Sorted(maps.Keys(templates)) {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			out = append(out, fmt.Sprintf("%s: missing; ateam init writes it at the pinned release", rel))
			continue
		}
		if !bytes.Equal(got, templates[rel]) {
			out = append(out, fmt.Sprintf("%s: differs from what ateam init writes at the pinned release; put a SemEngine rule in the matching semengine-* file", rel))
		}
	}
	return out
}
