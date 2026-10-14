package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// harness-boundaries › "Agent setup and rule-index parity": Steps 1–8 of .agents/README.md held by
// a unit test and its sensitivity test. Expected roles, tool lists, sandbox modes, model/effort
// pins, and caps are constants here rather than read from .agents/README.md.
const (
	maxAgentsMDBytes    = 24000
	maxClaudeMDLines    = 10
	maxRoleAdapterLines = 20
	maxSkillLines       = 25
)

type agentRoleSpec struct {
	name        string
	readOnly    bool
	claudeModel string
	codexModel  string
	codexEffort string
}

var (
	agentRoleSpecs = []agentRoleSpec{
		{name: "semengine-architect", readOnly: true, claudeModel: "opus", codexModel: "gpt-6-astra", codexEffort: "medium"},
		{name: "semengine-developer", readOnly: false, claudeModel: "opus", codexModel: "gpt-6-sol", codexEffort: "medium"},
		{name: "semengine-reviewer", readOnly: true, claudeModel: "opus", codexModel: "gpt-6-astra", codexEffort: "high"},
		{name: "semengine-technical-writer", readOnly: false, claudeModel: "sonnet", codexModel: "gpt-6-sol", codexEffort: "medium"},
	}
	readOnlyClaudeTools = []string{"Read", "Bash", "Grep", "Glob", "Skill"}
	writeClaudeTools    = []string{"Read", "Edit", "Write", "Bash", "Grep", "Glob", "Skill"}
	sharedSkills        = []struct{ canonical, claude string }{
		{"entity-or-bucket", "entity-or-bucket"},
		{"kv-or-stream", "kv-or-stream"},
		{"new-payload", "new-payload"},
		{"query-pattern", "query-pattern"},
		{"semengine-handoff", "semengine-handoff"},
		{"semengine-pickup", "semengine-pickup"},
		{"semengine-preflight", "preflight"},
	}
	expectedRuleDomains = []string{
		"1. Go, state, and component lifecycle",
		"2. Graph, identity, and NATS storage",
		"3. Testing and verification",
		"4. Porting and architecture",
		"5. Workflow, review, and merge gates",
	}
)

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
		{"CLAUDE.md drops @AGENTS.md import", edit("CLAUDE.md", func(string) string {
			return "# SemEngine\n\nCustom rules here.\n"
		}), []string{"CLAUDE.md", "@AGENTS.md"}},
		{"CLAUDE.md adds section heading", edit("CLAUDE.md", func(s string) string {
			return s + "\n## Local Rule\n"
		}), []string{"CLAUDE.md", "section heading"}},
		{"AGENTS.md reaches 24000 bytes", edit("AGENTS.md", func(s string) string {
			return s + strings.Repeat("x", maxAgentsMDBytes)
		}), []string{"AGENTS.md", "24000"}},
		{"AGENTS.md and rules-index.md row count mismatch", edit("AGENTS.md", func(s string) string {
			return strings.Replace(s, "| `verify` | No bare `select {}` | `harness-boundaries` spec |\n", "", 1)
		}), []string{"AGENTS.md", ".agents/rules-index.md", "1. Go, state, and component lifecycle"}},
		{"role adapter contains checkbox", edit(".claude/agents/semengine-developer.md", func(s string) string {
			return s + "- [ ] copied checklist item\n"
		}), []string{".claude/agents/semengine-developer.md", "checkbox"}},
		{"role adapter exceeds line limit", edit(".codex/agents/semengine-developer.toml", func(s string) string {
			return s + strings.Repeat("# extra line\n", 15)
		}), []string{".codex/agents/semengine-developer.toml", "lines"}},
		{"skill adapter points to wrong canonical skill", edit(".claude/skills/preflight/SKILL.md", func(s string) string {
			return strings.Replace(s, "semengine-preflight/SKILL.md", "semengine-handoff/SKILL.md", 1)
		}), []string{".claude/skills/preflight/SKILL.md", "../../../.agents/skills/semengine-preflight/SKILL.md"}},
		{"claude model mismatch", edit(".claude/agents/semengine-technical-writer.md", func(s string) string {
			return strings.Replace(s, "model: sonnet", "model: opus", 1)
		}), []string{".claude/agents/semengine-technical-writer.md", "model", "sonnet"}},
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
	rels := []string{
		"AGENTS.md",
		"CLAUDE.md",
		".agents/README.md",
		".agents/protocol.md",
		".agents/rules-index.md",
	}
	for _, r := range agentRoleSpecs {
		rels = append(rels,
			".agents/contracts/"+r.name+".md",
			".claude/agents/"+r.name+".md",
			".codex/agents/"+r.name+".toml",
		)
	}
	for _, s := range sharedSkills {
		rels = append(rels,
			".agents/skills/"+s.canonical+"/SKILL.md",
			".claude/skills/"+s.claude+"/SKILL.md",
		)
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
		out = append(out, checkClaudeRoleAdapter(root, role)...)
		out = append(out, checkCodexRoleAdapter(root, role)...)
	}
	out = append(out, checkRulesAndSizeParity(root)...)
	out = append(out, checkSkillAdapters(root)...)
	return out
}

func checkRequiredAgentFiles(root string) []string {
	var out []string
	required := []string{
		"AGENTS.md",
		"CLAUDE.md",
		".agents/README.md",
		".agents/protocol.md",
		".agents/rules-index.md",
	}
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
		Name  string `yaml:"name"`
		Tools string `yaml:"tools"`
		Model string `yaml:"model"`
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

var tomlKVPattern = regexp.MustCompile(`(?m)^([a-z_]+)\s*=\s*"([^"\n]*)"\s*$`)

func checkCodexRoleAdapter(root string, role agentRoleSpec) []string {
	rel := ".codex/agents/" + role.name + ".toml"
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	text := string(data)
	var out []string
	out = append(out, checkAdapterCommon(rel, role.name, text)...)

	before, rest, ok := strings.Cut(text, `developer_instructions = """`)
	header := before
	if ok {
		if _, after, ok2 := strings.Cut(rest, `"""`); ok2 {
			header = before + "\n" + after
		}
	}
	kv := map[string]string{}
	for _, m := range tomlKVPattern.FindAllStringSubmatch(header, -1) {
		kv[m[1]] = m[2]
	}
	if kv["name"] != role.name {
		out = append(out, fmt.Sprintf("%s: name = %q, want %q", rel, kv["name"], role.name))
	}
	if kv["model"] != role.codexModel {
		out = append(out, fmt.Sprintf("%s: model = %q, want %q", rel, kv["model"], role.codexModel))
	}
	if kv["model_reasoning_effort"] != role.codexEffort {
		out = append(out, fmt.Sprintf("%s: model_reasoning_effort = %q, want %q", rel, kv["model_reasoning_effort"], role.codexEffort))
	}
	if role.readOnly && kv["sandbox_mode"] != "read-only" {
		out = append(out, fmt.Sprintf("%s: read-only role must set sandbox_mode = \"read-only\" (got %q)", rel, kv["sandbox_mode"]))
	}
	if !role.readOnly && kv["sandbox_mode"] != "" {
		out = append(out, fmt.Sprintf("%s: write role must not set sandbox_mode (got %q)", rel, kv["sandbox_mode"]))
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

type ruleSectionCount struct {
	title string
	rows  int
}

func checkRulesAndSizeParity(root string) []string {
	var out []string
	claudeData, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err == nil {
		claudeText := string(claudeData)
		if !strings.Contains(claudeText, "@AGENTS.md") {
			out = append(out, "CLAUDE.md: missing @AGENTS.md import")
		}
		lines := strings.Count(strings.TrimRight(claudeText, "\n"), "\n") + 1
		if lines > maxClaudeMDLines {
			out = append(out, fmt.Sprintf("CLAUDE.md: %d lines exceeds max %d lines", lines, maxClaudeMDLines))
		}
		if strings.Contains(claudeText, "\n## ") || strings.Contains(claudeText, "\n|") {
			out = append(out, "CLAUDE.md: must not carry its own section heading or rule table")
		}
	}

	agentsData, errA := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	indexData, errI := os.ReadFile(filepath.Join(root, ".agents", "rules-index.md"))
	if errA != nil || errI != nil {
		return out
	}
	if len(agentsData) >= maxAgentsMDBytes {
		out = append(out, fmt.Sprintf("AGENTS.md: %d bytes is not strictly under %d bytes", len(agentsData), maxAgentsMDBytes))
	}
	agentsCounts := parseRuleSectionCounts(string(agentsData), "### ")
	indexCounts := parseRuleSectionCounts(string(indexData), "## ")
	if len(agentsCounts) != len(expectedRuleDomains) || len(indexCounts) != len(expectedRuleDomains) {
		return append(out, fmt.Sprintf("AGENTS.md (%d sections) and .agents/rules-index.md (%d sections) must both have %d numbered rule sections",
			len(agentsCounts), len(indexCounts), len(expectedRuleDomains)))
	}
	for i, wantTitle := range expectedRuleDomains {
		a, b := agentsCounts[i], indexCounts[i]
		if a.title != wantTitle || b.title != wantTitle {
			out = append(out, fmt.Sprintf("rule section %d: AGENTS.md = %q, .agents/rules-index.md = %q, want %q", i+1, a.title, b.title, wantTitle))
			continue
		}
		if a.rows == 0 || a.rows != b.rows {
			out = append(out, fmt.Sprintf("AGENTS.md and .agents/rules-index.md disagree on row count in %q: AGENTS.md has %d, .agents/rules-index.md has %d",
				wantTitle, a.rows, b.rows))
		}
	}
	return out
}

func parseRuleSectionCounts(text, prefix string) []ruleSectionCount {
	var sections []ruleSectionCount
	cur := -1
	for _, line := range strings.Split(text, "\n") {
		if title, ok := strings.CutPrefix(line, prefix); ok && len(title) >= 3 && title[0] >= '1' && title[0] <= '9' && title[1] == '.' {
			sections = append(sections, ruleSectionCount{title: strings.TrimSpace(title)})
			cur = len(sections) - 1
			continue
		}
		if cur >= 0 && strings.HasPrefix(line, "## ") && prefix != "## " {
			cur = -1
			continue
		}
		if cur < 0 || !strings.HasPrefix(line, "| ") {
			continue
		}
		if strings.HasPrefix(line, "| ---") || strings.HasPrefix(line, "| Gate |") || strings.HasPrefix(line, "| Rule |") {
			continue
		}
		sections[cur].rows++
	}
	return sections
}

func checkSkillAdapters(root string) []string {
	var out []string
	for _, s := range sharedSkills {
		canonRel := ".agents/skills/" + s.canonical + "/SKILL.md"
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(canonRel))); err != nil || info.IsDir() || info.Size() == 0 {
			out = append(out, fmt.Sprintf("%s: missing canonical skill", canonRel))
		}
		adapterRel := ".claude/skills/" + s.claude + "/SKILL.md"
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(adapterRel)))
		if err != nil {
			out = append(out, fmt.Sprintf("%s: missing Claude skill adapter", adapterRel))
			continue
		}
		text := string(data)
		wantLink := "../../../.agents/skills/" + s.canonical + "/SKILL.md"
		if !strings.Contains(text, wantLink) || !strings.Contains(text, "fully") {
			out = append(out, fmt.Sprintf("%s: must link to %s and instruct to read it fully", adapterRel, wantLink))
		}
		lines := strings.Count(strings.TrimRight(text, "\n"), "\n") + 1
		if lines > maxSkillLines {
			out = append(out, fmt.Sprintf("%s: %d lines exceeds max %d lines", adapterRel, lines, maxSkillLines))
		}
		if strings.Contains(text, "- [ ]") || strings.Contains(text, "- [x]") {
			out = append(out, fmt.Sprintf("%s: contains copied checklist checkbox", adapterRel))
		}
	}
	return out
}
