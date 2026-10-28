package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// harness-boundaries › "Agent setup and rule-index parity": the part of agent-setup parity that
// `ateam doctor --check` (task ateam:doctor) does not check, held by a unit test and its sensitivity
// test: the SemEngine contracts' base-contract pointer, the role adapters, and the adopter skills'
// Claude adapters (existence, link, size). Doctor owns CLAUDE.md, the AGENTS.md size, rule-row parity,
// and ateam's six canonical skills and their adapters. Expected roles, tool
// lists, sandbox modes, model/effort pins, and caps are constants here rather than read from
// .agents/README.md.
const (
	maxRoleAdapterLines = 20
	maxSkillLines       = 25
)

type agentRoleSpec struct {
	name         string
	baseContract string // ateam's base contract the SemEngine contract opens by naming
	readOnly     bool
	claudeModel  string
	claudeEffort string
	codexModel   string
	codexEffort  string
}

var (
	agentRoleSpecs = []agentRoleSpec{
		{name: "semengine-architect", baseContract: "architect.md", readOnly: true, claudeModel: "opus", claudeEffort: "medium", codexModel: "gpt-6-astra", codexEffort: "medium"},
		{name: "semengine-developer", baseContract: "developer.md", readOnly: false, claudeModel: "opus", claudeEffort: "medium", codexModel: "gpt-6-sol", codexEffort: "medium"},
		{name: "semengine-reviewer", baseContract: "reviewer.md", readOnly: true, claudeModel: "opus", claudeEffort: "high", codexModel: "gpt-6-astra", codexEffort: "high"},
		{name: "semengine-technical-writer", baseContract: "developer.md", readOnly: false, claudeModel: "sonnet", claudeEffort: "medium", codexModel: "gpt-6-sol", codexEffort: "medium"},
	}
	readOnlyClaudeTools = []string{"Read", "Bash", "Grep", "Glob", "Skill"}
	writeClaudeTools    = []string{"Read", "Edit", "Write", "Bash", "Grep", "Glob", "Skill"}
	// ateamCanonicalSkills are the six skills ateam init writes and doctor checks in full. Every
	// other directory under .agents/skills/ is an adopter skill, which doctor reports only when an
	// adapter's link dangles: a missing adapter, one pointing at another skill, or one that does not
	// say to read the skill fully passes it, so this test checks them.
	ateamCanonicalSkills = []string{"design", "handoff", "pickup", "ponytail", "preflight", "review-pr"}
)

// adopterSkills lists the skill directories under .agents/skills/ that are not ateam's.
func adopterSkills(root string) []string {
	entries, err := os.ReadDir(filepath.Join(root, ".agents", "skills"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && !slices.Contains(ateamCanonicalSkills, e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestAgentSetupParity(t *testing.T) {
	requireNoViolations(t, "agent setup parity", agentSetupViolations(repoRoot(t)))
}

func TestAgentSetupParitySensitivity(t *testing.T) {
	base := copyAgentSetupFixture(t, repoRoot(t))
	edit := func(rel string, fn func(string) string) map[string]string {
		m := make(map[string]string, len(base))
		for k, v := range base {
			m[k] = v
		}
		if fn == nil {
			delete(m, rel)
		} else {
			m[rel] = fn(m[rel])
		}
		return m
	}
	root, _ := writeTree(t, base)
	requireNoViolations(t, "clean fixture", agentSetupViolations(root))

	for _, tc := range []struct {
		name  string
		files map[string]string
		wants []string
	}{
		{"missing canonical contract", edit(".agents/contracts/semengine-architect.md", nil),
			[]string{".agents/contracts/semengine-architect.md", "missing"}},
		{"contract drops its base-contract pointer", edit(".agents/contracts/semengine-reviewer.md", func(s string) string {
			return strings.Replace(s, "Read `.agents/contracts/reviewer.md`, ateam's base contract, first; this contract adds SemEngine rules on top.\n\n", "", 1)
		}), []string{".agents/contracts/semengine-reviewer.md", "`.agents/contracts/reviewer.md`"}},
		{"technical writer points at the reviewer base", edit(".agents/contracts/semengine-technical-writer.md", func(s string) string {
			return strings.Replace(s, "`.agents/contracts/developer.md`", "`.agents/contracts/reviewer.md`", 1)
		}), []string{".agents/contracts/semengine-technical-writer.md", "`.agents/contracts/developer.md`"}},
		{"missing codex adapter", edit(".codex/agents/semengine-reviewer.toml", nil),
			[]string{".codex/agents/semengine-reviewer.toml", "missing"}},
		{"wrong contract path in claude adapter", edit(".claude/agents/semengine-reviewer.md", func(s string) string {
			return strings.ReplaceAll(s, "semengine-reviewer.md", "semengine-developer.md")
		}), []string{".claude/agents/semengine-reviewer.md", ".agents/contracts/semengine-reviewer.md"}},
		{"read-only claude adapter granted Edit", edit(".claude/agents/semengine-architect.md", func(s string) string {
			return strings.Replace(s, "tools: Read, Bash, Grep, Glob, Skill", "tools: Read, Edit, Bash, Grep, Glob, Skill", 1)
		}), []string{".claude/agents/semengine-architect.md", "tools"}},
		{"claude adapter lists LSP tool", edit(".claude/agents/semengine-developer.md", func(s string) string {
			return strings.Replace(s, "Skill", "Skill, LSP", 1)
		}), []string{".claude/agents/semengine-developer.md", "tools"}},
		{"read-only codex adapter missing sandbox_mode", edit(".codex/agents/semengine-reviewer.toml", func(s string) string {
			return strings.Replace(s, `sandbox_mode = "read-only"`+"\n", "", 1)
		}), []string{".codex/agents/semengine-reviewer.toml", "sandbox_mode"}},
		{"write codex adapter sets sandbox_mode", edit(".codex/agents/semengine-developer.toml", func(s string) string {
			return s + "sandbox_mode = \"read-only\"\n"
		}), []string{".codex/agents/semengine-developer.toml", "sandbox_mode"}},
		{"write codex adapter sets literal-string sandbox_mode", edit(".codex/agents/semengine-developer.toml", func(s string) string {
			return s + "sandbox_mode = 'read-only'\n"
		}), []string{".codex/agents/semengine-developer.toml", "sandbox_mode"}},
		{"write codex adapter sets sandbox_mode with inline comment", edit(".codex/agents/semengine-developer.toml", func(s string) string {
			return s + "sandbox_mode = \"read-only\" # override\n"
		}), []string{".codex/agents/semengine-developer.toml", "sandbox_mode"}},
		{"role adapter contains checkbox", edit(".claude/agents/semengine-developer.md", func(s string) string {
			return s + "- [ ] copied checklist item\n"
		}), []string{".claude/agents/semengine-developer.md", "checkbox"}},
		{"role adapter exceeds line limit", edit(".codex/agents/semengine-developer.toml", func(s string) string {
			return s + strings.Repeat("# extra line\n", 15)
		}), []string{".codex/agents/semengine-developer.toml", "lines"}},
		{"claude model mismatch", edit(".claude/agents/semengine-technical-writer.md", func(s string) string {
			return strings.Replace(s, "model: sonnet", "model: opus", 1)
		}), []string{".claude/agents/semengine-technical-writer.md", "model", "sonnet"}},
		{"adopter skill adapter points to another skill", edit(".claude/skills/query-pattern/SKILL.md", func(s string) string {
			return strings.ReplaceAll(s, "skills/query-pattern/SKILL.md", "skills/kv-or-stream/SKILL.md")
		}), []string{".claude/skills/query-pattern/SKILL.md", "../../../.agents/skills/query-pattern/SKILL.md"}},
		{"adopter skill adapter does not say fully", edit(".claude/skills/new-payload/SKILL.md", func(s string) string {
			return strings.ReplaceAll(s, "fully", "")
		}), []string{".claude/skills/new-payload/SKILL.md", "read it fully"}},
		{"new adopter skill missing claude adapter", edit(".agents/skills/new-review-skill/SKILL.md", func(string) string {
			return "# New Skill\n"
		}), []string{".claude/skills/new-review-skill/SKILL.md", "missing Claude skill adapter"}},
		{"claude effort mismatch", edit(".claude/agents/semengine-reviewer.md", func(s string) string {
			return strings.Replace(s, "effort: high", "effort: medium", 1)
		}), []string{".claude/agents/semengine-reviewer.md", "effort", "high"}},
		{"adopter skill adapter contains checkbox", edit(".claude/skills/query-pattern/SKILL.md", func(s string) string {
			return s + "- [ ] copied checklist item\n"
		}), []string{".claude/skills/query-pattern/SKILL.md", "checkbox"}},
		{"adopter skill adapter exceeds line limit", edit(".claude/skills/semengine-status/SKILL.md", func(s string) string {
			return s + strings.Repeat("extra line\n", maxSkillLines)
		}), []string{".claude/skills/semengine-status/SKILL.md", "lines"}},
		{"codex effort mismatch", edit(".codex/agents/semengine-developer.toml", func(s string) string {
			return strings.Replace(s, `model_reasoning_effort = "medium"`, `model_reasoning_effort = "high"`, 1)
		}), []string{".codex/agents/semengine-developer.toml", "model_reasoning_effort", "medium"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, _ := writeTree(t, tc.files)
			requireViolation(t, agentSetupViolations(dir), tc.wants...)
		})
	}
}

func copyAgentSetupFixture(t *testing.T, root string) map[string]string {
	t.Helper()
	var rels []string
	for _, r := range agentRoleSpecs {
		rels = append(rels,
			".agents/contracts/"+r.name+".md",
			".claude/agents/"+r.name+".md",
			".codex/agents/"+r.name+".toml",
		)
	}
	for _, s := range adopterSkills(root) {
		rels = append(rels, ".agents/skills/"+s+"/SKILL.md", ".claude/skills/"+s+"/SKILL.md")
	}
	files := make(map[string]string, len(rels))
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		files[rel] = string(data)
	}
	return files
}

func agentSetupViolations(root string) []string {
	var out []string
	out = append(out, checkRequiredAgentFiles(root)...)
	for _, role := range agentRoleSpecs {
		out = append(out, checkContractBasePointer(root, role)...)
		out = append(out, checkClaudeRoleAdapter(root, role)...)
		out = append(out, checkCodexRoleAdapter(root, role)...)
	}
	out = append(out, checkAdopterSkillAdapters(root)...)
	return out
}

func checkRequiredAgentFiles(root string) []string {
	var out []string
	var required []string
	for _, r := range agentRoleSpecs {
		required = append(required,
			".agents/contracts/"+r.name+".md",
			".claude/agents/"+r.name+".md",
			".codex/agents/"+r.name+".toml",
		)
	}
	for _, rel := range required {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || info.IsDir() || info.Size() == 0 {
			out = append(out, fmt.Sprintf("%s: missing or empty required agent file", rel))
		}
	}
	return out
}

// checkContractBasePointer: the first paragraph after a SemEngine contract's title names the ateam
// base contract to read first (harness-boundaries › "SemEngine contracts sit on the base contracts").
func checkContractBasePointer(root string, role agentRoleSpec) []string {
	rel := ".agents/contracts/" + role.name + ".md"
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	var para []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "# ") || (strings.TrimSpace(line) == "" && len(para) == 0) {
			continue
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		para = append(para, line)
	}
	want := "`.agents/contracts/" + role.baseContract + "`"
	if !strings.Contains(strings.Join(para, " "), want) {
		return []string{fmt.Sprintf("%s: first paragraph must name the base contract %s to read first", rel, want)}
	}
	return nil
}

var contractPathPattern = regexp.MustCompile(`\.agents/contracts/[A-Za-z0-9_.-]+\.md`)

func checkClaudeRoleAdapter(root string, role agentRoleSpec) []string {
	rel := ".claude/agents/" + role.name + ".md"
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	text := string(data)
	var out []string
	out = append(out, checkAdapterCommon(rel, role.name, text)...)

	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return append(out, fmt.Sprintf("%s: missing YAML frontmatter", rel))
	}
	fmText, _, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return append(out, fmt.Sprintf("%s: unterminated YAML frontmatter", rel))
	}
	var fm struct {
		Name   string `yaml:"name"`
		Tools  string `yaml:"tools"`
		Model  string `yaml:"model"`
		Effort string `yaml:"effort"`
	}
	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		return append(out, fmt.Sprintf("%s: invalid frontmatter: %v", rel, err))
	}
	if fm.Name != role.name {
		out = append(out, fmt.Sprintf("%s: frontmatter name = %q, want %q", rel, fm.Name, role.name))
	}
	if fm.Model != role.claudeModel {
		out = append(out, fmt.Sprintf("%s: frontmatter model = %q, want %q", rel, fm.Model, role.claudeModel))
	}
	if fm.Effort != role.claudeEffort {
		out = append(out, fmt.Sprintf("%s: frontmatter effort = %q, want %q", rel, fm.Effort, role.claudeEffort))
	}
	var gotTools []string
	for _, tok := range strings.Split(fm.Tools, ",") {
		if t := strings.TrimSpace(tok); t != "" {
			gotTools = append(gotTools, t)
		}
	}
	wantTools := writeClaudeTools
	if role.readOnly {
		wantTools = readOnlyClaudeTools
	}
	if !slices.Equal(gotTools, wantTools) {
		out = append(out, fmt.Sprintf("%s: frontmatter tools = %v, want %v", rel, gotTools, wantTools))
	}
	return out
}

func checkCodexRoleAdapter(root string, role agentRoleSpec) []string {
	rel := ".codex/agents/" + role.name + ".toml"
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	text := string(data)
	var out []string
	out = append(out, checkAdapterCommon(rel, role.name, text)...)

	var cfg struct {
		Name                 string `toml:"name"`
		Model                string `toml:"model"`
		ModelReasoningEffort string `toml:"model_reasoning_effort"`
		SandboxMode          string `toml:"sandbox_mode"`
	}
	md, err := toml.Decode(text, &cfg)
	if err != nil {
		return append(out, fmt.Sprintf("%s: invalid TOML: %v", rel, err))
	}
	if cfg.Name != role.name {
		out = append(out, fmt.Sprintf("%s: name = %q, want %q", rel, cfg.Name, role.name))
	}
	if cfg.Model != role.codexModel {
		out = append(out, fmt.Sprintf("%s: model = %q, want %q", rel, cfg.Model, role.codexModel))
	}
	if cfg.ModelReasoningEffort != role.codexEffort {
		out = append(out, fmt.Sprintf("%s: model_reasoning_effort = %q, want %q", rel, cfg.ModelReasoningEffort, role.codexEffort))
	}
	if role.readOnly && cfg.SandboxMode != "read-only" {
		out = append(out, fmt.Sprintf("%s: read-only role must set sandbox_mode = \"read-only\" (got %q)", rel, cfg.SandboxMode))
	}
	if !role.readOnly && md.IsDefined("sandbox_mode") {
		out = append(out, fmt.Sprintf("%s: write role must not set sandbox_mode (got %q)", rel, cfg.SandboxMode))
	}
	return out
}

func checkAdapterCommon(rel, roleName, text string) []string {
	var out []string
	wantContract := ".agents/contracts/" + roleName + ".md"
	matches := contractPathPattern.FindAllString(text, -1)
	if len(matches) == 0 || !slices.Equal(matches, []string{wantContract}) {
		out = append(out, fmt.Sprintf("%s: contract references %v, want exactly [%s]", rel, matches, wantContract))
	}
	wantReadFirst := fmt.Sprintf("Your first action is to read `%s` fully", wantContract)
	if !strings.Contains(text, wantReadFirst) {
		out = append(out, fmt.Sprintf("%s: missing %q", rel, wantReadFirst))
	}
	lines := strings.Count(strings.TrimRight(text, "\n"), "\n") + 1
	if lines > maxRoleAdapterLines {
		out = append(out, fmt.Sprintf("%s: %d lines exceeds max %d lines", rel, lines, maxRoleAdapterLines))
	}
	if strings.Contains(text, "- [ ]") || strings.Contains(text, "- [x]") {
		out = append(out, fmt.Sprintf("%s: contains copied checklist checkbox", rel))
	}
	return out
}

func checkAdopterSkillAdapters(root string) []string {
	var out []string
	for _, s := range adopterSkills(root) {
		rel := ".claude/skills/" + s + "/SKILL.md"
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			out = append(out, fmt.Sprintf("%s: missing Claude skill adapter", rel))
			continue
		}
		text := string(data)
		wantLink := "../../../.agents/skills/" + s + "/SKILL.md"
		if !strings.Contains(text, wantLink) || !strings.Contains(text, "fully") {
			out = append(out, fmt.Sprintf("%s: must link to %s and instruct to read it fully", rel, wantLink))
		}
		lines := strings.Count(strings.TrimRight(text, "\n"), "\n") + 1
		if lines > maxSkillLines {
			out = append(out, fmt.Sprintf("%s: %d lines exceeds max %d lines", rel, lines, maxSkillLines))
		}
		if strings.Contains(text, "- [ ]") || strings.Contains(text, "- [x]") {
			out = append(out, fmt.Sprintf("%s: contains copied checklist checkbox", rel))
		}
	}
	return out
}
