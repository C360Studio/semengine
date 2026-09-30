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

// T-B7: docs/admission-ledger.yaml is the one home of SemStreams provenance (owner ruling Q5).
// `task ledger:check` runs exactly this test; `task test:unit` runs it inside `task verify`.
func TestAdmissionLedger(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "docs", "admission-ledger.yaml"))
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	requireNoViolations(t, "admission ledger", ledgerViolations(data))
}

// validEntry renders one schema-valid entry; the sensitivity cases perturb exactly one field.
func validEntry(path string, override map[string]string) string {
	fields := []struct{ key, value string }{
		{"source_path", path},
		{"source_sha", "5457b3458936f668b71d2fea061f67f8d7d01e67"},
		{"consumer_purpose", "a purpose"},
		{"destination", "internal/x"},
		{"contract", "a contract"},
		{"dependencies_and_side_effects", "stdlib only"},
		{"known_risks", "none known"},
		{"proving_tests", "TestX"},
		{"owner", "setup-02 developer"},
		{"disposition", "adapt"},
	}
	var b strings.Builder
	for i, f := range fields {
		value, ok := override[f.key]
		if !ok {
			value = f.value
		}
		if value == "<omit>" {
			continue
		}
		prefix := "  "
		if i == 0 {
			prefix = "- "
		}
		b.WriteString(prefix + f.key + ": " + value + "\n")
	}
	for key, value := range override {
		if strings.HasPrefix(key, "+") {
			b.WriteString("  " + strings.TrimPrefix(key, "+") + ": " + value + "\n")
		}
	}
	return b.String()
}

func TestAdmissionLedgerSchemaSensitivity(t *testing.T) {
	clean := validEntry("a/one.go", nil) + validEntry("a/two.go", nil)
	if v := ledgerViolations([]byte(clean)); len(v) > 0 {
		t.Fatalf("clean fixture rejected:\n  %s", strings.Join(v, "\n  "))
	}

	for _, tc := range []struct {
		name      string
		ledger    string
		fragments []string
	}{
		{"short sha", validEntry("a/one.go", map[string]string{"source_sha": "5457b345"}),
			[]string{"a/one.go", "source_sha"}},
		{"uppercase sha", validEntry("a/one.go", map[string]string{"source_sha": strings.ToUpper("5457b3458936f668b71d2fea061f67f8d7d01e67")}),
			[]string{"a/one.go", "source_sha"}},
		{"unknown disposition", validEntry("a/one.go", map[string]string{"disposition": "port"}),
			[]string{"a/one.go", "disposition", "port"}},
		{"empty field", validEntry("a/one.go", map[string]string{"known_risks": `""`}),
			[]string{"a/one.go", "known_risks"}},
		{"missing field", validEntry("a/one.go", map[string]string{"owner": "<omit>"}),
			[]string{"a/one.go", "owner"}},
		{"unknown field", validEntry("a/one.go", map[string]string{"+license": "MIT"}),
			[]string{"a/one.go", "license"}},
		{"duplicate source_path", validEntry("a/one.go", nil) + validEntry("a/one.go", nil),
			[]string{"a/one.go", "duplicate"}},
		{"not a list", "source_path: a/one.go\n", []string{"list"}},
		{"empty ledger", "# nothing\n", []string{"no entries"}},
		{"unparseable", "- source_path: [\n", []string{"parse"}},
		{"non-scalar field", strings.Replace(validEntry("a/one.go", nil), "owner: setup-02 developer", "owner: [a, b]", 1),
			[]string{"a/one.go", "owner"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireViolation(t, ledgerViolations([]byte(tc.ledger)), tc.fragments...)
		})
	}
}

// ledgerFields is the schema in the ledger's header comment and harness-boundaries › "Admission
// ledger": every entry carries exactly these keys, each a non-empty scalar.
var ledgerFields = []string{
	"source_path", "source_sha", "consumer_purpose", "destination", "contract",
	"dependencies_and_side_effects", "known_risks", "proving_tests", "owner", "disposition",
}

var (
	fullSHA      = regexp.MustCompile(`^[0-9a-f]{40}$`)
	dispositions = map[string]bool{"carry": true, "adapt": true, "repair-before-port": true, "defer-exclude": true}
)

// ledgerViolations is the T-B7 check. It walks the YAML node tree rather than decoding into a
// struct so an unknown key, a list where a scalar belongs, or an empty value is reported instead of
// being silently dropped or zero-filled.
func ledgerViolations(data []byte) []string {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return []string{"parse: " + err.Error()}
	}
	if len(doc.Content) == 0 {
		return []string{"ledger has no entries"}
	}
	list := doc.Content[0]
	if list.Kind != yaml.SequenceNode {
		return []string{fmt.Sprintf("line %d: ledger must be a YAML list of entries", list.Line)}
	}
	if len(list.Content) == 0 {
		return []string{"ledger has no entries"}
	}

	var violations []string
	firstLine := map[string]int{}
	for i, entry := range list.Content {
		label := fmt.Sprintf("entry %d (line %d)", i+1, entry.Line)
		if entry.Kind != yaml.MappingNode {
			violations = append(violations, label+": not a mapping")
			continue
		}
		// Label by source_path before reporting anything, so every message names the entry.
		for j := 0; j+1 < len(entry.Content); j += 2 {
			if k, v := entry.Content[j], entry.Content[j+1]; k.Value == "source_path" && v.Kind == yaml.ScalarNode && v.Value != "" {
				label = fmt.Sprintf("entry %s (line %d)", v.Value, entry.Line)
			}
		}
		values := map[string]string{}
		for j := 0; j+1 < len(entry.Content); j += 2 {
			key, value := entry.Content[j], entry.Content[j+1]
			if !slices.Contains(ledgerFields, key.Value) {
				violations = append(violations, fmt.Sprintf("%s: unknown field %q", label, key.Value))
				continue
			}
			if value.Kind != yaml.ScalarNode {
				violations = append(violations, fmt.Sprintf("%s: field %s is not a scalar", label, key.Value))
				continue
			}
			values[key.Value] = strings.TrimSpace(value.Value)
		}
		if path := values["source_path"]; path != "" {
			if prior, dup := firstLine[path]; dup {
				violations = append(violations, fmt.Sprintf("%s: duplicate source_path, first at line %d", label, prior))
			} else {
				firstLine[path] = entry.Line
			}
		}
		for _, field := range ledgerFields {
			if values[field] == "" {
				violations = append(violations, fmt.Sprintf("%s: field %s missing or empty", label, field))
			}
		}
		if sha := values["source_sha"]; sha != "" && !fullSHA.MatchString(sha) {
			violations = append(violations, fmt.Sprintf("%s: source_sha %q is not a full 40-character lowercase SHA", label, sha))
		}
		if d := values["disposition"]; d != "" && !dispositions[d] {
			violations = append(violations, fmt.Sprintf("%s: disposition %q is not carry, adapt, repair-before-port or defer-exclude", label, d))
		}
	}
	return violations
}
