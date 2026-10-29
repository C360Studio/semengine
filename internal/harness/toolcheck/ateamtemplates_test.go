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
// Init runs into an empty directory holding only the repository's review policy, not a copy of the
// tree: it never overwrites an existing file, so a copy of the repository gets nothing written to
// compare.

// semengineOwned are the paths init writes that SemEngine keeps its own text in, with the reason. The
// five client configurations are ateam's to register into but the adopter's to edit: init never
// replaces them, since they "may contain permissions, secrets, or other servers" (ateam mcp.go,
// isClientConfig), and doctor checks the hook and MCP registrations inside them.
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
	".claude/settings.json":             "client configuration; doctor checks its hooks",
	".codex/config.toml":                "client configuration; doctor checks its MCP registration",
	".codex/hooks.json":                 "client configuration; doctor checks its hooks",
	".gemini/settings.json":             "client configuration; doctor checks its hooks and MCP registration",
	".mcp.json":                         "client configuration; doctor checks its MCP registration",
}

// ateamPin matches an ateam install line in .github/workflows/ci.yml and merge-check.yml; every one
// names the same release.
var ateamPin = regexp.MustCompile(`ateam/cmd/ateam@(v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)`)

// ateamPins returns the one release every ateam install line in the workflow files names (keyed by
// file name), or a violation naming each file's versions when they are not one, or when a file has
// no install line (harness-boundaries › "ateam doctor gates verify"; merge-gate › "A contract test
// guards both workflows").
func ateamPins(files map[string][]byte) (string, []string) {
	var all, per, violations []string
	for _, name := range slices.Sorted(maps.Keys(files)) {
		var versions []string
		for _, m := range ateamPin.FindAllSubmatch(files[name], -1) {
			v := string(m[1])
			if !slices.Contains(versions, v) {
				versions = append(versions, v)
			}
			if !slices.Contains(all, v) {
				all = append(all, v)
			}
		}
		if len(versions) == 0 {
			violations = append(violations, fmt.Sprintf(".github/workflows/%s: no ateam install line; every workflow that runs ateam installs the pinned release", name))
		}
		per = append(per, fmt.Sprintf("%s %v", name, versions))
	}
	if len(all) > 1 {
		violations = append(violations, fmt.Sprintf(".github/workflows: ateam install lines name versions %s; want one version", strings.Join(per, ", ")))
	}
	if len(violations) > 0 {
		return "", violations
	}
	return all[0], nil
}

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
	workflows := map[string][]byte{}
	for _, name := range []string{"ci.yml", "merge-check.yml"} {
		data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", name))
		if err != nil {
			t.Fatal(err)
		}
		workflows[name] = data
	}
	pin, violations := ateamPins(workflows)
	requireNoViolations(t, "ateam pin", violations)
	install := "GOTOOLCHAIN=auto go install github.com/c360studio/ateam/cmd/ateam@" + pin
	if _, err := exec.LookPath("ateam"); err != nil {
		t.Fatalf("ateam is not on PATH; install the pinned release: %s", install)
	}
	out, err := exec.CommandContext(t.Context(), "ateam", "version").CombinedOutput()
	if err != nil {
		t.Fatalf("ateam version: %v\n%s", err, out)
	}
	if !slices.Contains(strings.Fields(string(out)), pin) {
		t.Fatalf("ateam version reports %q, want the workflows' pin %s; install it and put $(go env GOPATH)/bin first on PATH: %s", strings.TrimSpace(string(out)), pin, install)
	}

	// init reads the review policy (review_record: aip writes a different reviewer.md and
	// review-pr skill, ateam reviewrecord.go), so the empty directory gets the repository's.
	dir := t.TempDir()
	policy, err := os.ReadFile(filepath.Join(root, ".agents", "review-policy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agents", "review-policy.json"), policy, 0o644); err != nil {
		t.Fatal(err)
	}
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
