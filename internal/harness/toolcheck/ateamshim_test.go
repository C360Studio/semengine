package toolcheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// harness-boundaries › "scripts/ateam resolves the main checkout's .tools/bin/ateam":
// scripts/ateam resolves <main>/.tools/bin/ateam from a linked worktree's .git file without
// invoking git, falls back to <worktree>/.tools/bin/ateam and then PATH, and exits 127 naming
// task tools:ateam when none is found.
func TestAteamShimResolution(t *testing.T) {
	root := repoRoot(t)
	shimPath := filepath.Join(root, "scripts", "ateam")
	shimInfo, err := os.Stat(shimPath)
	if err != nil {
		t.Fatalf("stat scripts/ateam: %v", err)
	}
	if shimInfo.Mode()&0o111 == 0 {
		t.Fatalf("scripts/ateam mode = %v, want executable bit set (0755)", shimInfo.Mode())
	}
	shimBytes, err := os.ReadFile(shimPath)
	if err != nil {
		t.Fatalf("read scripts/ateam: %v", err)
	}

	writeExec := func(t *testing.T, path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	writeFakeAteam := func(t *testing.T, path, tag string) {
		t.Helper()
		writeExec(t, path, "#!/bin/sh\nprintf '%s:%s\\n' '"+tag+"' \"$*\"\n")
	}

	// Build a clean isolated system bin with NO `git` and NO `ateam`, proving
	// the shim resolves both primary checkouts and linked worktrees without forking `git`.
	makeIsolatedPath := func(t *testing.T, extraDirs ...string) string {
		t.Helper()
		sysBin := filepath.Join(t.TempDir(), "sys-bin")
		if err := os.MkdirAll(sysBin, 0o755); err != nil {
			t.Fatalf("mkdir sys-bin: %v", err)
		}
		for _, tool := range []string{"sh", "printf"} {
			if p, lookErr := exec.LookPath(tool); lookErr == nil {
				_ = os.Symlink(p, filepath.Join(sysBin, tool))
			}
		}
		dirs := append([]string{}, extraDirs...)
		dirs = append(dirs, sysBin)
		return strings.Join(dirs, string(os.PathListSeparator))
	}

	setupFixture := func(t *testing.T, shimContent []byte) (mainDir, wtDir, pathBinDir string) {
		t.Helper()
		base := t.TempDir()
		mainDir = filepath.Join(base, "main")
		wtDir = filepath.Join(mainDir, ".worktrees", "feature-x")
		pathBinDir = filepath.Join(base, "path-bin")

		if err := os.MkdirAll(filepath.Join(mainDir, ".git", "worktrees", "feature-x"), 0o755); err != nil {
			t.Fatalf("mkdir main .git: %v", err)
		}
		if err := os.MkdirAll(wtDir, 0o755); err != nil {
			t.Fatalf("mkdir wtDir: %v", err)
		}
		gitFile := "gitdir: " + filepath.Join(mainDir, ".git", "worktrees", "feature-x") + "\n"
		if err := os.WriteFile(filepath.Join(wtDir, ".git"), []byte(gitFile), 0o644); err != nil {
			t.Fatalf("write wt .git: %v", err)
		}
		writeExec(t, filepath.Join(mainDir, "scripts", "ateam"), string(shimContent))
		writeExec(t, filepath.Join(wtDir, "scripts", "ateam"), string(shimContent))
		return mainDir, wtDir, pathBinDir
	}

	runShim := func(dir, pathEnv string, args ...string) (string, int) {
		cmd := exec.CommandContext(t.Context(), filepath.Join(dir, "scripts", "ateam"), args...)
		cmd.Dir = dir
		cmd.Env = []string{"PATH=" + pathEnv}
		out, runErr := cmd.CombinedOutput()
		code := 0
		if runErr != nil {
			if exitErr, ok := runErr.(*exec.ExitError); ok {
				code = exitErr.ExitCode()
			} else {
				code = -1
			}
		}
		return strings.TrimSpace(string(out)), code
	}

	t.Run("primary_and_worktree_prefer_main_tools_bin_over_worktree_and_path_without_git", func(t *testing.T) {
		mainDir, wtDir, pathBinDir := setupFixture(t, shimBytes)
		writeFakeAteam(t, filepath.Join(mainDir, ".tools", "bin", "ateam"), "MAIN")
		writeFakeAteam(t, filepath.Join(wtDir, ".tools", "bin", "ateam"), "WT")
		writeFakeAteam(t, filepath.Join(pathBinDir, "ateam"), "PATH")

		gotMain, codeMain := runShim(mainDir, makeIsolatedPath(t, pathBinDir), "version")
		if codeMain != 0 || gotMain != "MAIN:version" {
			t.Fatalf("primary checkout got (%q, exit %d), want (\"MAIN:version\", exit 0)", gotMain, codeMain)
		}

		gotWT, codeWT := runShim(wtDir, makeIsolatedPath(t, pathBinDir), "version")
		if codeWT != 0 || gotWT != "MAIN:version" {
			t.Fatalf("worktree got (%q, exit %d), want (\"MAIN:version\", exit 0)", gotWT, codeWT)
		}
	})

	t.Run("worktree_falls_back_to_worktree_tools_bin_then_path", func(t *testing.T) {
		_, wtDir, pathBinDir := setupFixture(t, shimBytes)
		writeFakeAteam(t, filepath.Join(wtDir, ".tools", "bin", "ateam"), "WT")
		writeFakeAteam(t, filepath.Join(pathBinDir, "ateam"), "PATH")

		got, code := runShim(wtDir, makeIsolatedPath(t, pathBinDir), "mcp")
		if code != 0 || got != "WT:mcp" {
			t.Fatalf("got (%q, exit %d), want (\"WT:mcp\", exit 0)", got, code)
		}

		if err := os.Remove(filepath.Join(wtDir, ".tools", "bin", "ateam")); err != nil {
			t.Fatalf("remove wt ateam: %v", err)
		}
		got, code = runShim(wtDir, makeIsolatedPath(t, pathBinDir), "hook", "--event", "turn-end")
		if code != 0 || got != "PATH:hook --event turn-end" {
			t.Fatalf("got (%q, exit %d), want (\"PATH:hook --event turn-end\", exit 0)", got, code)
		}
	})

	t.Run("missing_everywhere_exits_127_with_install_hint", func(t *testing.T) {
		_, wtDir, _ := setupFixture(t, shimBytes)
		got, code := runShim(wtDir, makeIsolatedPath(t), "version")
		if code != 127 || !strings.Contains(got, "task tools:ateam") {
			t.Fatalf("got (%q, exit %d), want exit 127 mentioning 'task tools:ateam'", got, code)
		}
	})

	t.Run("sensitivity_swapping_search_order_is_detected", func(t *testing.T) {
		mutated := strings.Replace(
			string(shimBytes),
			`for bin in "$main/.tools/bin/ateam" "$here/.tools/bin/ateam"; do`,
			`for bin in "$here/.tools/bin/ateam" "$main/.tools/bin/ateam"; do`,
			1,
		)
		if mutated == string(shimBytes) {
			t.Fatal("search-order mutation did not apply to scripts/ateam")
		}
		mainDir, wtDir, pathBinDir := setupFixture(t, []byte(mutated))
		writeFakeAteam(t, filepath.Join(mainDir, ".tools", "bin", "ateam"), "MAIN")
		writeFakeAteam(t, filepath.Join(wtDir, ".tools", "bin", "ateam"), "WT")
		writeFakeAteam(t, filepath.Join(pathBinDir, "ateam"), "PATH")

		got, code := runShim(wtDir, makeIsolatedPath(t, pathBinDir), "version")
		if code == 0 && got == "MAIN:version" {
			t.Fatal("expected mutated search order to prefer WT over MAIN, proving test sensitivity")
		}
	})

	t.Run("sensitivity_dropping_primary_checkout_else_branch_is_detected", func(t *testing.T) {
		mutated := strings.Replace(
			string(shimBytes),
			"else\n\tmain=$here\nfi",
			"else\n\tmain=/nonexistent\nfi",
			1,
		)
		if mutated == string(shimBytes) {
			t.Fatal("primary-checkout else-branch mutation did not apply to scripts/ateam")
		}
		mainDir, _, pathBinDir := setupFixture(t, []byte(mutated))
		writeFakeAteam(t, filepath.Join(mainDir, ".tools", "bin", "ateam"), "MAIN")
		writeFakeAteam(t, filepath.Join(pathBinDir, "ateam"), "PATH")
		// Remove scripts/ateam's second fallback ($here/.tools/bin/ateam) so primary-checkout $main is tested directly.
		mutatedNoHere := strings.Replace(mutated, ` "$here/.tools/bin/ateam"`, "", 1)
		writeExec(t, filepath.Join(mainDir, "scripts", "ateam"), mutatedNoHere)

		got, code := runShim(mainDir, makeIsolatedPath(t, pathBinDir), "version")
		if code == 0 && got == "MAIN:version" {
			t.Fatal("expected broken primary-checkout main=$here branch to miss MAIN, proving test sensitivity")
		}
	})
}
