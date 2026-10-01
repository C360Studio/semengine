package natsfixture

import (
	"regexp"
	"strings"
	"testing"
)

// The destroyer list is restated here from SemStreams' routine cleanup filters (inventory A6
// "Destroyers" at 5457b345) as an independent oracle: CheckName must agree with it, not with itself.
var oracleDestroyers = []string{
	"semstreams", "nats-semstreams", "semembed", "agentic", "crud-tools", "deep-research", "ops", "research-graph",
}

var nameShape = regexp.MustCompile(`^semengine-[a-z0-9-]+$`)

func requireSafe(t *testing.T, name string) {
	t.Helper()
	if !nameShape.MatchString(name) {
		t.Fatalf("name %q does not match %s", name, nameShape)
	}
	for _, d := range oracleDestroyers {
		if strings.Contains(name, d) {
			t.Fatalf("name %q contains destroyer substring %q", name, d)
		}
	}
	if err := CheckName(name); err != nil {
		t.Fatalf("CheckName(%q) = %v, want nil", name, err)
	}
}

func TestCheckName(t *testing.T) {
	for _, ok := range []string{"semengine-x", "semengine-lane-1-0a9f", "semengine-evidence-sentinel"} {
		if err := CheckName(ok); err != nil {
			t.Errorf("CheckName(%q) = %v, want nil", ok, err)
		}
	}
	for _, tc := range []struct{ name, fragment string }{
		{"", "semengine-"},
		{"semengine-", "empty"},
		{"lane-x", "semengine-"},
		{"semengine-Lane", "[a-z0-9-]"},
		{"semengine-a_b", "[a-z0-9-]"},
		{"semengine-ops-cache", "ops"},
		{"semengine-stops", "ops"},
		{"semengine-loops-1", "ops"},
		{"semengine-crud-tools", "crud-tools"},
		{"semengine-semstreams", "semstreams"},
		{"semengine-nats-semstreams-x", "semstreams"},
		{"semengine-semembed", "semembed"},
		{"semengine-agentic", "agentic"},
		{"semengine-deep-research", "deep-research"},
		{"semengine-research-graph", "research-graph"},
	} {
		err := CheckName(tc.name)
		if err == nil || !strings.Contains(err.Error(), tc.fragment) {
			t.Errorf("CheckName(%q) = %v, want an error mentioning %q", tc.name, err, tc.fragment)
		}
	}
}

// TestNameAdversarialTestNames runs Name under test names built to smuggle destroyers in: whole
// segments, substrings of longer words, and destroyers split across the segment boundary.
func TestNameAdversarialTestNames(t *testing.T) {
	for _, sub := range []string{
		"OpsResearchGraph", "stops", "Loops/with/slashes", "crud", "deep_research", "Semstreams!",
		"agentic_loop", "日本語", "", "a-very-long-test-name-that-goes-on-and-on-and-on-and-on-and-on",
	} {
		t.Run(sub, func(t *testing.T) {
			f := New(t)
			for _, base := range []string{"x", "tools", "ops", "research-graph", "Stream Name", "", "-", "nats"} {
				requireSafe(t, f.Name(base))
			}
		})
	}
}

// TestOpsResearchGraph is the spec scenario verbatim: the calling test's own name carries two
// destroyers.
func TestOpsResearchGraph(t *testing.T) {
	name := New(t).Name("x")
	requireSafe(t, name)
	if strings.Contains(name, "ops") || strings.Contains(name, "research-graph") {
		t.Fatalf("Name = %q", name)
	}
}

func TestNameIsRunUniqueAndCarriesBase(t *testing.T) {
	f := New(t)
	a, b := f.Name("orders"), f.Name("orders")
	if a == b {
		t.Fatalf("two Name(%q) calls returned the same value %q", "orders", a)
	}
	for _, n := range []string{a, b} {
		requireSafe(t, n)
		if !strings.Contains(n, "-orders-") {
			t.Errorf("Name(%q) = %q, want the base as a segment", "orders", n)
		}
		if !regexp.MustCompile(`-[0-9a-f]{8}$`).MatchString(n) {
			t.Errorf("Name = %q, want a lowercase hex suffix", n)
		}
	}
	if !strings.HasPrefix(a, "semengine-testnameisrununiqueandcarriesbase-") {
		t.Errorf("Name = %q, want the sanitized test name as the first segment", a)
	}
}

func FuzzName(f *testing.F) {
	for _, seed := range [][2]string{
		{"TestOps", "x"}, {"stops", "loops"}, {"crud", "tools"}, {"deep", "research"}, {"research", "graph"},
		{"", ""}, {"---", "___"}, {"Semstreams", "nats-semstreams"}, {"a/b/c", "d e f"},
	} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, testName, base string) {
		fx := &Fixture{testName: testName}
		requireSafe(t, fx.Name(base))
	})
}

func FuzzCheckName(f *testing.F) {
	for _, seed := range []string{"semengine-x", "semengine-ops", "semengine-stops", "x", "", "semengine-A", "semengine-\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, name string) {
		err := CheckName(name)
		oracleOK := nameShape.MatchString(name)
		for _, d := range oracleDestroyers {
			if strings.Contains(name, d) {
				oracleOK = false
			}
		}
		if (err == nil) != oracleOK {
			t.Fatalf("CheckName(%q) = %v, oracle accepts = %t", name, err, oracleOK)
		}
	})
}
