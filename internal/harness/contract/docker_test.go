package contract

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/c360studio/semengine/internal/harness/natsfixture"
	"gopkg.in/yaml.v3"
)

// T-B4: SemEngine never selects Docker resources broadly. SemStreams' routine tasks delete volumes
// by name substring and prune the daemon's build cache (taskfiles/e2e/common.yml:77-79,
// clean.yml:17-19 at 5457b345); on a shared daemon those commands reach other repositories' data.
func TestNoBroadDockerCleanup(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "broad Docker cleanup", broadCleanupViolations(t, root, repoFiles(t, root)))
}

func TestNoBroadDockerCleanupSensitivity(t *testing.T) {
	clean := map[string]string{
		"scripts/ok.sh": "# a comment may say docker system prune without running it\n" +
			"docker ps -aq --filter label=org.testcontainers.golang.sessionId=\"$sid\"\n" +
			"docker rm -f \"$id\"\n" +
			"docker compose -p semengine-lane-1 down -v\n",
		"Taskfile.yml": "tasks:\n  x:\n    cmds:\n      - docker image inspect \"$img\"\n",
	}
	root, files := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", broadCleanupViolations(t, root, files))

	for _, tc := range []struct {
		name, file, line, fragment string
	}{
		{"builder prune", "scripts/x.sh", "docker builder prune -f", "prune"},
		{"system prune", "Taskfile.yml", "      - docker system prune -af", "prune"},
		{"volume prune", ".github/workflows/ci.yml", "        run: docker volume prune -f", "prune"},
		{"volume name filter", "scripts/x.sh", "docker volume ls -q --filter name=semengine | xargs docker volume rm", "--filter name="},
		{"volume name filter with equals", "scripts/x.sh", "docker volume ls -q --filter=name=semengine", "--filter name="},
		{"xargs volume rm", "scripts/x.sh", "cat ids | xargs -r docker volume rm", "xargs"},
		{"ps name filter", "scripts/x.sh", "docker ps -aq --filter name=semengine", "--filter name="},
		{"container ls name filter", "scripts/x.sh", "docker container ls -aq --filter \"name=nats\"", "--filter name="},
		{"compose down on a foreign project", "scripts/x.sh", "docker compose -p semstreams down -v", "compose down"},
		{"compose down with no -p", "scripts/x.sh", "docker compose -f docker/a.yml down", "compose down"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{}
			for k, v := range clean {
				files[k] = v
			}
			files[tc.file] = files[tc.file] + tc.line + "\n"
			root, names := writeTree(t, files)
			requireViolation(t, broadCleanupViolations(t, root, names), tc.file, tc.fragment)
		})
	}
}

// T-B5: every Docker name SemEngine assigns is namespaced and invisible to SemStreams' substring
// destroyers. Docker's name filter matches any part of a name, so `ops` inside `stops` is selected
// by `--filter name=ops`; the rule is a substring rule (natsfixture.CheckName), not a segment rule.
func TestSemEngineAssignedNames(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "Docker names", dockerNameViolations(t, root, repoFiles(t, root)))
}

func TestSemEngineAssignedNamesSensitivity(t *testing.T) {
	clean := map[string]string{
		"scripts/ok.sh": "docker run -d --name semengine-evidence-sentinel -v semengine-evidence-sentinel-data:/data img\n" +
			"docker network create semengine-lane-net\n" +
			"docker run --name \"semengine-run-$suffix\" img\n" +
			"go test -p 2 ./...\n" +
			"docker run -v /abs/path:/data -v ./rel:/x img\n" +
			"pin=$(grep -v '^[[:space:]]*#' .nats-image)\n" +
			"git config --name value\n",
		"docker/compose.yml": "name: semengine-stack\nservices:\n  broker:\n    container_name: semengine-broker\n" +
			"volumes:\n  semengine-data: {}\nnetworks:\n  semengine-net:\n    name: semengine-net\n",
		"Taskfile.yml": "env:\n  COMPOSE_PROJECT_NAME: semengine-lane\n",
	}
	root, files := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", dockerNameViolations(t, root, files))

	for _, tc := range []struct {
		name, file, content string
		fragments           []string
	}{
		{"destroyer segment in a volume", "scripts/x.sh", "docker volume create semengine-ops-cache\n",
			[]string{"scripts/x.sh:1", "semengine-ops-cache", "ops"}},
		{"destroyer inside a longer word: stops", "scripts/x.sh", "docker run --name semengine-stops-lane img\n",
			[]string{"scripts/x.sh:1", "semengine-stops-lane", "ops"}},
		{"destroyer inside a longer word: loops", "scripts/x.sh", "docker run -v semengine-loops:/d img\n",
			[]string{"scripts/x.sh:1", "semengine-loops", "ops"}},
		{"missing prefix", "scripts/x.sh", "docker network create lane-net\n",
			[]string{"scripts/x.sh:1", "lane-net", "semengine-"}},
		{"uppercase", "scripts/x.sh", "docker run --name=semengine-Lane img\n",
			[]string{"scripts/x.sh:1", "semengine-Lane"}},
		{"compose project flag", "scripts/x.sh", "docker compose -p semengine-agentic up -d\n",
			[]string{"scripts/x.sh:1", "agentic"}},
		{"compose project env", "Taskfile.yml", "env:\n  COMPOSE_PROJECT_NAME: semstreams\n",
			[]string{"Taskfile.yml:2", "semstreams"}},
		{"compose top-level name", "docker/compose.yml", "name: semengine-research-graph\nservices: {}\n",
			[]string{"docker/compose.yml", "research-graph"}},
		{"compose container_name", "docker/compose.yml", "services:\n  a:\n    container_name: semengine-crud-tools\n",
			[]string{"docker/compose.yml", "crud-tools"}},
		{"compose volume key", "docker/compose.yml", "volumes:\n  semembed-cache: {}\n",
			[]string{"docker/compose.yml", "semembed-cache"}},
		{"compose network name", "docker/compose.yml", "networks:\n  n:\n    name: semengine-deep-research\n",
			[]string{"docker/compose.yml", "deep-research"}},
		{"variable name with unsafe literal part", "scripts/x.sh", "docker run --name \"semengine-ops-$x\" img\n",
			[]string{"scripts/x.sh:1", "ops"}},
		{"variable-only name", "scripts/x.sh", "docker run --name \"$name\" img\n",
			[]string{"scripts/x.sh:1", "$name", "literal"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{}
			for k, v := range clean {
				files[k] = v
			}
			files[tc.file] = tc.content
			root, names := writeTree(t, files)
			requireViolation(t, dockerNameViolations(t, root, names), tc.fragments...)
		})
	}
}

// dockerScriptFile reports whether a file can start or remove Docker resources and so falls under
// T-B4 and T-B5: shell scripts, the Taskfile (and any taskfiles/ includes), CI workflows, and
// Compose files under docker/.
func dockerScriptFile(name string) bool {
	switch {
	case strings.HasPrefix(name, "scripts/") && strings.HasSuffix(name, ".sh"):
		return true
	case name == "Taskfile.yml" || strings.HasPrefix(name, "taskfiles/"):
		return true
	case strings.HasPrefix(name, ".github/workflows/"):
		return true
	case strings.HasPrefix(name, "docker/") && (strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")):
		return true
	}
	return false
}

// scriptLines yields a Docker-capable file's lines with comment lines blanked (so a comment may
// name a forbidden command) and quotes removed (so `--filter "name=x"` and `--filter name=x` are
// one shape). Line numbers are preserved.
func scriptLines(t *testing.T, root, name string) []string {
	t.Helper()
	lines := readLines(t, root, name)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimPrefix(trimmed, "- ")
		if strings.HasPrefix(trimmed, "#") {
			lines[i] = ""
			continue
		}
		lines[i] = strings.NewReplacer(`"`, "", `'`, "", "--filter=", "--filter ").Replace(line)
	}
	return lines
}

var (
	prunePattern      = regexp.MustCompile(`\bdocker\b.*\bprune\b`)
	nameFilterPattern = regexp.MustCompile(`\bdocker\s+(volume\s+ls|ps|container\s+ls|network\s+ls|image\s+ls|images)\b.*--filter\s+name=`)
	xargsRmPattern    = regexp.MustCompile(`\bxargs\b.*\bdocker\s+volume\s+rm\b`)
	composeDown       = regexp.MustCompile(`\bcompose\b.*\sdown\b`)
	ownProject        = regexp.MustCompile(`(^|\s)(-p|--project-name)[= ]+semengine-`)
)

// broadCleanupViolations is the T-B4 check. Removal is only ever by observed ID or exact label;
// Compose `down` must name the SemEngine project it acts on, since an unnamed project defaults to
// the directory name and can be another repository's stack (inventory A6, "Compose project name").
func broadCleanupViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	var violations []string
	for _, name := range files {
		if !dockerScriptFile(name) {
			continue
		}
		for i, line := range scriptLines(t, root, name) {
			at := fmt.Sprintf("%s:%d", name, i+1)
			if prunePattern.MatchString(line) {
				violations = append(violations, at+": docker prune selects resources SemEngine does not own")
			}
			if nameFilterPattern.MatchString(line) {
				violations = append(violations, at+": docker --filter name= matches by substring; select by ID or exact label")
			}
			if xargsRmPattern.MatchString(line) {
				violations = append(violations, at+": xargs into docker volume rm removes whatever a listing matched")
			}
			if composeDown.MatchString(line) && !ownProject.MatchString(line) {
				violations = append(violations, at+": compose down without -p semengine-<project>")
			}
		}
	}
	return violations
}

// keyPatterns name a Docker resource wherever they appear; flagPatterns only on a line that
// invokes docker, since flags such as `-v` and `--name` mean other things to other tools.
var (
	keyPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\bcontainer_name:\s*(\S+)`),
		regexp.MustCompile(`\bCOMPOSE_PROJECT_NAME\s*[=:]\s*(\S+)`),
		regexp.MustCompile(`\bdocker\s+(?:volume|network)\s+create\s+(?:-\S+\s+)*([^-\s]\S*)`),
	}
	flagPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?:^|\s)--name[= ]+(\S+)`),
		regexp.MustCompile(`(?:^|\s)(?:-v|--volume)[= ]+([^/.~$\s][^:\s]*):`),
	}
	dockerCommand = regexp.MustCompile(`\bdocker\b`)
)

var (
	composeProjectFlag = regexp.MustCompile(`(?:^|\s)(?:-p|--project-name)[= ]+(\S+)`)
	shellVariable      = regexp.MustCompile(`\$\{[^}]*\}|\$[A-Za-z_][A-Za-z0-9_]*`)
)

// dockerNameViolations is the T-B5 check over literal names in scripts, Taskfile, workflows, and
// the structure of Compose files under docker/.
func dockerNameViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	var violations []string
	check := func(at, raw string) {
		value := strings.TrimRight(strings.Trim(raw, `"'`), `;\)`)
		if strings.HasPrefix(value, "$") {
			violations = append(violations, fmt.Sprintf("%s: name %s is not a literal; start it with semengine-", at, value))
			return
		}
		// A variable part cannot be checked here; replacing it with a hyphen still exposes any
		// destroyer or bad character in the literal parts around it.
		if err := natsfixture.CheckName(shellVariable.ReplaceAllString(value, "-")); err != nil {
			violations = append(violations, fmt.Sprintf("%s: %s: %v", at, value, err))
		}
	}
	for _, name := range files {
		if !dockerScriptFile(name) {
			continue
		}
		if strings.HasPrefix(name, "docker/") {
			violations = append(violations, composeNameViolations(t, root, name, check)...)
		}
		for i, line := range scriptLines(t, root, name) {
			at := fmt.Sprintf("%s:%d", name, i+1)
			patterns := keyPatterns
			if dockerCommand.MatchString(line) {
				patterns = append(append([]*regexp.Regexp(nil), keyPatterns...), flagPatterns...)
			}
			for _, p := range patterns {
				for _, m := range p.FindAllStringSubmatch(line, -1) {
					check(at, m[1])
				}
			}
			if strings.Contains(line, "compose") {
				for _, m := range composeProjectFlag.FindAllStringSubmatch(line, -1) {
					check(at, m[1])
				}
			}
		}
	}
	return violations
}

// composeNameViolations checks the names a Compose file assigns structurally: the project name, and
// every volume and network key and explicit name. Service container_name is caught line by line.
func composeNameViolations(t *testing.T, root, name string, check func(at, raw string)) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return []string{fmt.Sprintf("%s: parse: %v", name, err)}
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	top := doc.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		key, value := top.Content[i], top.Content[i+1]
		switch key.Value {
		case "name":
			check(fmt.Sprintf("%s:%d", name, value.Line), value.Value)
		case "volumes", "networks":
			if value.Kind != yaml.MappingNode {
				continue
			}
			for j := 0; j+1 < len(value.Content); j += 2 {
				k, v := value.Content[j], value.Content[j+1]
				check(fmt.Sprintf("%s:%d", name, k.Line), k.Value)
				for n := 0; v.Kind == yaml.MappingNode && n+1 < len(v.Content); n += 2 {
					if v.Content[n].Value == "name" {
						check(fmt.Sprintf("%s:%d", name, v.Content[n+1].Line), v.Content[n+1].Value)
					}
				}
			}
		}
	}
	return nil
}
