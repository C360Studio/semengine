package natsfixture

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// namePrefix namespaces every Docker name SemEngine assigns (invariant I10).
const namePrefix = "semengine-"

// destroyers are the name substrings SemStreams' routine cleanup selects with Docker's
// `--filter name=` (taskfiles/e2e/common.yml:77-79, agentic.yml:78, crud-tools.yml:80,
// deep-research.yml:80, ops.yml:80, research-graph.yml:79, clean.yml:17 at 5457b345). Docker's name
// filter matches any part of a name, so these are forbidden anywhere, including inside longer words
// (`stops` and `loops` contain `ops`).
var destroyers = []string{
	"semstreams", "nats-semstreams", "semembed", "agentic", "crud-tools", "deep-research", "ops", "research-graph",
}

// Segment length caps keep generated names readable; the hex suffix, not the segments, carries
// uniqueness.
const (
	maxTestSegment = 40
	maxBaseSegment = 32
	suffixBytes    = 4
)

// CheckName reports why name is not a valid SemEngine-assigned Docker name, or nil. It is the one
// statement of the rule: Name's output is checked against it, and contract test T-B5 applies it to
// every literal name in scripts, the Taskfile, and Compose files.
func CheckName(name string) error {
	if !strings.HasPrefix(name, namePrefix) {
		return fmt.Errorf("docker name %q: must start with %q", name, namePrefix)
	}
	if len(name) == len(namePrefix) {
		return fmt.Errorf("docker name %q: empty after the %q prefix", name, namePrefix)
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return fmt.Errorf("docker name %q: %q is outside [a-z0-9-]", name, r)
		}
	}
	var found []string
	for _, d := range destroyers {
		if strings.Contains(name, d) {
			found = append(found, d)
		}
	}
	if len(found) > 0 {
		return fmt.Errorf("docker name %q: contains SemStreams destroyer substring(s) %s", name, strings.Join(found, ", "))
	}
	return nil
}

// Name returns a run-unique name for a Docker resource, stream, bucket, or consumer this test owns:
// semengine-<sanitized test name>-<sanitized base>-<8 hex>. A segment that contains a destroyer is
// replaced by a hash of itself, and the assembled name is checked again because two safe segments
// can meet at the hyphen and spell one (`crud` + `tools`); in that case every segment is hashed,
// which cannot spell a destroyer because hash segments are `h` plus lowercase hex.
func (f *Fixture) Name(base string) string {
	suffix := randomHex()
	segments := []string{sanitizeSegment(f.testName, maxTestSegment), sanitizeSegment(base, maxBaseSegment)}
	if name := assemble(segments, suffix); CheckName(name) == nil {
		return name
	}
	for i, s := range segments {
		if s != "" {
			segments[i] = hashSegment(s)
		}
	}
	name := assemble(segments, suffix)
	if err := CheckName(name); err != nil {
		// Unreachable by construction; a panic here is a defect in this file, not a test failure.
		panic(err)
	}
	return name
}

func assemble(segments []string, suffix string) string {
	parts := []string{strings.TrimSuffix(namePrefix, "-")}
	for _, s := range segments {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(append(parts, suffix), "-")
}

// sanitizeSegment lowercases, maps every run of characters outside [a-z0-9] to one hyphen, trims,
// caps the length, and hashes the result if it still contains a destroyer.
func sanitizeSegment(s string, limit int) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			hyphen = false
			continue
		}
		if !hyphen && b.Len() > 0 {
			b.WriteByte('-')
			hyphen = true
		}
	}
	seg := strings.Trim(b.String(), "-")
	if len(seg) > limit {
		seg = strings.Trim(seg[:limit], "-")
	}
	for _, d := range destroyers {
		if strings.Contains(seg, d) {
			return hashSegment(seg)
		}
	}
	return seg
}

func hashSegment(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "h" + hex.EncodeToString(sum[:4])
}

func randomHex() string {
	b := make([]byte, suffixBytes)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read does not fail on supported platforms (Go 1.24+ documents it as never
		// returning an error); a name without its uniqueness suffix would be a silent collision.
		panic(errors.Join(errors.New("natsfixture: no randomness for a run-unique name"), err))
	}
	return hex.EncodeToString(b)
}
