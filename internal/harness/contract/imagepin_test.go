package contract

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// T-B3: the NATS image is spelled once, in .nats-image, by digest (owner ruling Q6). A second
// spelling is how SemStreams came to re-point a shared mutable tag from one repository's task.
func TestOneImagePin(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "image pin", imagePinViolations(t, root, repoFiles(t, root)))
}

func TestOneImagePinSensitivity(t *testing.T) {
	pin := img(":2.14.7-alpine@sha256:" + sixtyFourHex)
	clean := map[string]string{
		".nats-image":     "# comment naming " + img(":2.14.7-alpine") + " is fine here\n" + pin + "\n",
		"scripts/run.sh":  "url=nats://host:4222\nimage=\"$SEMENGINE_NATS_IMAGE\"\n",
		"docs/design.md":  "Prose may cite " + img(":2.14-alpine") + " when explaining the pin.\n",
		"compose/app.yml": "services:\n  nats:\n    image: ${SEMENGINE_NATS_IMAGE}\n",
	}
	root, files := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", imagePinViolations(t, root, files))

	for _, tc := range []struct {
		name      string
		files     map[string]string
		fragments []string
	}{
		{"tag literal in a script", map[string]string{"scripts/x.sh": "docker pull " + img(":2.14-alpine") + "\n"},
			[]string{"scripts/x.sh:1", img(":2.14-alpine")}},
		{"digest literal in Go", map[string]string{"internal/x/x.go": "package x\n\nconst image = \"" + img("@sha256:"+sixtyFourHex) + "\"\n"},
			[]string{"internal/x/x.go:3", img("@sha256:")}},
		{"latest in yaml", map[string]string{"docker/a.yml": "services:\n  n:\n    image: " + img(":latest") + "\n"},
			[]string{"docker/a.yml:3", img(":latest")}},
		{"registry-qualified", map[string]string{"Taskfile.yml": "cmd: docker run docker.io/library/" + img(":2.14") + "\n"},
			[]string{"Taskfile.yml:1", img(":2.14")}},
		{"pin not digest-addressed", map[string]string{".nats-image": img(":2.14.7-alpine") + "\n"},
			[]string{".nats-image", "digest"}},
		{"pin file with two images", map[string]string{".nats-image": pin + "\n" + pin + "\n"},
			[]string{".nats-image", "exactly one"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{}
			for k, v := range clean {
				files[k] = v
			}
			for k, v := range tc.files {
				files[k] = v
			}
			root, names := writeTree(t, files)
			requireViolation(t, imagePinViolations(t, root, names), tc.fragments...)
		})
	}
}

// img builds an image reference at run time so this file never contains the literal it hunts for.
func img(rest string) string { return "nats" + rest }

const sixtyFourHex = "4063edae0717ba5f7501bfde75f97fd9b57f5b93597b92c70b6a6fbbf6a74e06"

var (
	// imageLiteral matches a NATS image reference: the repository name followed by a tag or a
	// sha256 digest, optionally registry-qualified. `nats://` URLs and a YAML key followed by a space
	// do not match.
	imageLiteral = regexp.MustCompile(`(^|[^A-Za-z0-9_.-])nats(:[A-Za-z0-9][A-Za-z0-9._-]*|@sha256:)`)
	// pinShape is the one accepted form of the pin: a tag for humans, a digest for Docker.
	pinShape = regexp.MustCompile(`^nats:[A-Za-z0-9][A-Za-z0-9._-]*@sha256:[0-9a-f]{64}$`)
)

// imagePinViolations is the T-B3 check. Markdown is exempt: design documents and specs must be able
// to cite the pin and the upstream tag in prose, and no tool reads an image from them.
func imagePinViolations(t *testing.T, root string, files []string) []string {
	t.Helper()
	var violations []string
	pins := 0
	for _, name := range files {
		if name == ".nats-image" {
			for i, line := range readLines(t, root, name) {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				pins++
				if !pinShape.MatchString(line) {
					violations = append(violations, fmt.Sprintf(".nats-image:%d: %q is not nats:<tag>@sha256:<64 hex> (digest-addressed)", i+1, line))
				}
			}
			continue
		}
		if strings.HasSuffix(name, ".md") {
			continue
		}
		for i, line := range readLines(t, root, name) {
			if m := imageLiteral.FindStringSubmatch(line); m != nil {
				violations = append(violations, fmt.Sprintf("%s:%d: NATS image literal %q outside .nats-image",
					name, i+1, strings.TrimLeft(m[0], " \t\"'=/")))
			}
		}
	}
	if pins != 1 {
		violations = append(violations, fmt.Sprintf(".nats-image: holds %d image line(s), want exactly one", pins))
	}
	return violations
}
