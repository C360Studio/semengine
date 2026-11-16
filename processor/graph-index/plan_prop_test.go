package graphindex

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	semtypes "github.com/c360studio/semengine/pkg/types"
	"github.com/c360studio/semengine/vocabulary"
	"pgregory.net/rapid"
)

// maxIncomingKeyBytes is the longest INCOMING_INDEX key the contract allows, 902 bytes:
// two 256-byte IDs, two dots and the hex of a 194-byte predicate.
const maxIncomingKeyBytes = 2*semtypes.MaxEntityIDBytes + 2 + 2*vocabulary.MaxPredicateBytes

// planCase is one entity with the rows the requirement says it plans: written by hand for
// the named cases, counted by rowRoles.count for generated ones, never read back from the
// plan builder.
type planCase struct {
	state         graph.EntityState
	relationships int // statements whose object is an entity reference
	aliases       int // alias-predicate statements whose value is non-empty after trimming (ADR-114 §5)
	names         int // label-predicate statements whose value is non-empty
}

// entityIDShape is the canonical entity-ID grammar, written here from the rule: six
// dot-separated segments, each an ASCII letter or digit then letters, digits, '_' or '-'.
var entityIDShape = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*(\.[a-zA-Z0-9][a-zA-Z0-9_-]*){5}$`)

// genericPredicatePrefix opens the first segment of every generic predicate the test
// draws. No registered alias or label predicate starts with it (checked before the
// generated run), so a generic statement holds no alias or name role by construction.
const genericPredicatePrefix = "gen-"

// builtInTitleLabel is the label predicate the framework registers at initialization
// (vocabulary/labels.go), spelled here rather than read from the vocabulary.
const builtInTitleLabel = "dc.terms.title"

// rowRoles is the test's own record of which predicates are alias and label predicates:
// the ones it registered, plus the framework's built-in title label.
type rowRoles struct {
	aliases, names map[string]bool
}

func newRowRoles(aliasPredicates, labelPredicates []string) rowRoles {
	r := rowRoles{aliases: make(map[string]bool), names: map[string]bool{builtInTitleLabel: true}}
	for _, p := range aliasPredicates {
		r.aliases[p] = true
	}
	for _, p := range labelPredicates {
		r.names[p] = true
	}
	return r
}

// count adds the rows one statement plans. Each role is decided on its own rule and they
// are summed, so a statement holding several roles (a registered predicate with an
// entity-reference value) counts every row it plans:
//   - outgoing: the object is a string of entity-ID shape, at most 256 bytes, and the
//     datatype is empty or "@id" (the rule message.Triple.IsRelationship documents);
//   - alias: the predicate is a registered alias predicate and the value is non-empty
//     after trimming (ADR-114 §5);
//   - name: the predicate is a registered label predicate and the value is non-empty.
func (r rowRoles) count(pc *planCase, triple message.Triple) {
	value, _ := triple.Object.(string)
	if (triple.Datatype == "" || triple.Datatype == message.EntityReferenceDatatype) &&
		len(value) <= semtypes.MaxEntityIDBytes && entityIDShape.MatchString(value) {
		pc.relationships++
	}
	if r.aliases[triple.Predicate] && strings.TrimSpace(value) != "" {
		pc.aliases++
	}
	if r.names[triple.Predicate] && value != "" {
		pc.names++
	}
}

// fatalf is what the witness (*testing.T) and the generated run (*rapid.T) share.
type fatalf interface {
	Fatalf(format string, args ...any)
}

// TestContractValidStateAlwaysPlans: every entity state UnmarshalEntityState accepts gets
// an index plan, so the failed mark on a refused plan is reached by no
// stored value. The generator draws six-part IDs and predicates up to and, by a drawn
// flag, exactly at their byte bounds (256 and 194), relationships under both reference
// spellings, ID-shaped and plain literals, and alias and name values of any bytes,
// including blank ones, each under a generic predicate (in the test's own gen- namespace)
// or a registered alias or label predicate;
// rowRoles.count counts each row role on its own rule. The named witness runs first and
// plans the longest incoming key, 902 bytes, every time; named cases then cover every
// overlap of a registered predicate with a reference, a plain literal and an ID-shaped
// literal.
func TestContractValidStateAlwaysPlans(t *testing.T) {
	defer vocabulary.SnapshotRegistry()()
	longAlias := strings.Repeat("a", 64) + "." + strings.Repeat("b", 64) + "." + strings.Repeat("c", 64)
	longLabel := strings.Repeat("a", 64) + "." + strings.Repeat("b", 64) + "." + strings.Repeat("d", 64)
	aliasPredicates := []string{longAlias, "test.alias.serial"}
	labelPredicates := []string{longLabel, "test.name.label"}
	for _, p := range aliasPredicates {
		vocabulary.Register(p, vocabulary.WithAlias(vocabulary.AliasTypeIdentity, 0))
	}
	for _, p := range labelPredicates {
		vocabulary.Register(p, vocabulary.WithAlias(vocabulary.AliasTypeLabel, 0))
	}
	c, err := createFromJSON(t, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	// What startIndexPool reads at Start.
	c.aliasPredicates = vocabulary.DiscoverAliasPredicates()
	c.namePredicates = vocabulary.DiscoverLabelPredicates()
	roles := newRowRoles(aliasPredicates, labelPredicates)
	// Generic draws stay out of every registered role, the framework's own included.
	for _, discovered := range []map[string]int{vocabulary.DiscoverAliasPredicates(), vocabulary.DiscoverLabelPredicates()} {
		for p := range discovered {
			if strings.HasPrefix(p, genericPredicatePrefix) {
				t.Fatalf("registered predicate %q is in the generic namespace %q", p, genericPredicatePrefix)
			}
		}
	}

	t.Run("witness at every bound", func(t *testing.T) {
		source := padEntityID("a.b.c.d.e.", 's')
		target := padEntityID("a.b.c.d.e.", 't')
		predicate := strings.Repeat("p", 64) + "." + strings.Repeat("q", 64) + "." + strings.Repeat("r", 64)
		when := time.Unix(1, 0).UTC()
		pc := planCase{state: graph.EntityState{ID: source, Triples: []message.Triple{
			{Subject: source, Predicate: predicate, Object: target, Source: "test", Timestamp: when},
			{Subject: source, Predicate: longAlias, Object: "longest\x00 alias ✓", Source: "test", Timestamp: when},
			{Subject: source, Predicate: longLabel, Object: strings.Repeat("N", 4096), Source: "test", Timestamp: when},
		}}, relationships: 1, aliases: 1, names: 1}
		if got := checkStatePlans(t, c, pc); got != 902 {
			t.Fatalf("longest incoming key planned = %d bytes, want 902", got)
		}
	})

	// A registered predicate's statement plans its alias or name row and, when its value
	// is an entity reference, an outgoing row as well (message.Triple.IsRelationship): an
	// ID-shaped value is a reference unless an explicit non-reference datatype makes it a
	// literal, and a registered predicate's statement can hold either value. The first four
	// cases are an ID-shaped value with no datatype and as a literal, the next four an
	// explicit reference and a plain literal, and the last two the framework's built-in
	// title label (vocabulary/labels.go). The counts are written by hand; each case also
	// checks rowRoles.count against them.
	for _, tc := range []struct {
		name, predicate, object, datatype string
		relationships, aliases, names     int
	}{
		{"ID-shaped alias, literal", "test.alias.serial", "a.b.c.d.e.target", "xsd:string", 0, 1, 0},
		{"ID-shaped alias, inferred reference", "test.alias.serial", "a.b.c.d.e.target", "", 1, 1, 0},
		{"ID-shaped name, literal", "test.name.label", "a.b.c.d.e.target", "xsd:string", 0, 0, 1},
		{"ID-shaped name, inferred reference", "test.name.label", "a.b.c.d.e.target", "", 1, 0, 1},
		{"alias predicate, reference", "test.alias.serial", "a.b.c.d.e.target", "@id", 1, 1, 0},
		{"alias predicate, plain literal", "test.alias.serial", "literal value", "", 0, 1, 0},
		{"name predicate, reference", "test.name.label", "a.b.c.d.e.target", "@id", 1, 0, 1},
		{"name predicate, plain literal", "test.name.label", "literal value", "", 0, 0, 1},
		{"built-in title, plain literal", builtInTitleLabel, "literal value", "", 0, 0, 1},
		{"built-in title, reference", builtInTitleLabel, "a.b.c.d.e.target", "@id", 1, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			triple := message.Triple{
				Subject: "a.b.c.d.e.source", Predicate: tc.predicate, Object: tc.object,
				Datatype: tc.datatype, Source: "test", Timestamp: time.Unix(1, 0).UTC(),
			}
			pc := planCase{state: graph.EntityState{ID: "a.b.c.d.e.source", Triples: []message.Triple{triple}},
				relationships: tc.relationships, aliases: tc.aliases, names: tc.names}
			var counted planCase
			roles.count(&counted, triple)
			if counted.relationships != pc.relationships || counted.aliases != pc.aliases ||
				counted.names != pc.names {
				t.Fatalf("rowRoles.count: %d outgoing, %d alias, %d name; want %d, %d, %d",
					counted.relationships, counted.aliases, counted.names,
					pc.relationships, pc.aliases, pc.names)
			}
			checkStatePlans(t, c, pc)
		})
	}

	var checks, atMaxKey int
	rapid.Check(t, func(rt *rapid.T) {
		pc := drawPlanCase(rt, roles, aliasPredicates, labelPredicates)
		if checkStatePlans(rt, c, pc) == maxIncomingKeyBytes {
			atMaxKey++
		}
		checks++
	})
	t.Logf("assertions run: property runs %d (shrinking included), of which planned a %d-byte incoming key %d",
		checks, maxIncomingKeyBytes, atMaxKey)
}

// checkStatePlans stores pc.state through the production codec, decodes it with
// UnmarshalEntityState, and requires that the plan is built and holds the rows pc counts.
// It returns the longest incoming key the plan writes.
func checkStatePlans(t fatalf, c *Component, pc planCase) int {
	data, err := graph.MarshalEntityState(&pc.state)
	if err != nil {
		t.Fatalf("generator drew a state the contract refuses on write: %v", err)
	}
	var decoded graph.EntityState
	if err := graph.UnmarshalEntityState(pc.state.ID, data, &decoded); err != nil {
		t.Fatalf("generator drew a state the contract refuses on read: %v", err)
	}
	plan, err := c.buildEntityIndexPlan(decoded, decoded.ID)
	if err != nil {
		t.Fatalf("buildEntityIndexPlan refused a state the contract accepts: %v\nstate: %s", err, data)
	}
	if len(plan.outgoing) != pc.relationships || len(plan.aliasWrites) != pc.aliases ||
		len(plan.nameWrites) != pc.names {
		t.Fatalf("plan rows: %d outgoing, %d alias, %d name; want %d, %d, %d\nstate: %s",
			len(plan.outgoing), len(plan.aliasWrites), len(plan.nameWrites),
			pc.relationships, pc.aliases, pc.names, data)
	}
	longest := 0
	for targetID, entries := range plan.incomingByTarget {
		for _, entry := range entries {
			longest = max(longest, len(incomingIndexKey(targetID, decoded.ID, entry.Predicate)))
		}
	}
	return longest
}

// padEntityID pads the last segment of a six-part prefix with fill to exactly
// MaxEntityIDBytes.
func padEntityID(prefix string, fill byte) string {
	return prefix + strings.Repeat(string(fill), semtypes.MaxEntityIDBytes-len(prefix))
}

// drawEntityID draws a six-part ID from the entity-ID grammar: each segment starts with
// an ASCII letter or digit and continues with letters, digits, '_' or '-'. A drawn flag
// pads the last segment to exactly 256 bytes; otherwise to a drawn length up to it.
func drawEntityID(t *rapid.T, label string) string {
	segment := rapid.StringMatching(`[a-zA-Z0-9][a-zA-Z0-9_-]{0,9}`)
	parts := make([]string, 6)
	for i := range parts {
		parts[i] = segment.Draw(t, label+"-segment")
	}
	id := strings.Join(parts, ".")
	length := semtypes.MaxEntityIDBytes
	if !rapid.Bool().Draw(t, label+"-at-bound") {
		length = rapid.IntRange(len(id), semtypes.MaxEntityIDBytes).Draw(t, label+"-length")
	}
	return id + strings.Repeat("x", length-len(id))
}

// drawPredicate draws a three-part predicate from the lower-kebab grammar,
// [a-z][a-z0-9]*(-[a-z0-9]+)*, its first segment opening with genericPredicatePrefix, each
// segment padded with letters to 64 bytes (making the predicate 194) by a drawn flag,
// otherwise to a drawn length up to 64. The prefix counts inside the first segment's 64.
func drawPredicate(t *rapid.T) string {
	segment := rapid.StringMatching(`[a-z][a-z0-9]{0,5}(-[a-z0-9]{1,5}){0,2}`)
	atBound := rapid.Bool().Draw(t, "predicate-at-bound")
	parts := make([]string, 3)
	for i := range parts {
		s := segment.Draw(t, "predicate-segment")
		if i == 0 {
			s = genericPredicatePrefix + s
		}
		length := vocabulary.MaxPredicateSegmentBytes
		if !atBound {
			length = rapid.IntRange(len(s), vocabulary.MaxPredicateSegmentBytes).Draw(t, "predicate-segment-length")
		}
		parts[i] = s + strings.Repeat("k", length-len(s))
	}
	return strings.Join(parts, ".")
}

// drawValue draws an alias or name value: any string, blank ones and very long ones
// included, since both indexes key on a hash of it.
func drawValue(t *rapid.T) string {
	return rapid.OneOf(
		rapid.String(),
		rapid.SampledFrom([]string{"", " ", "\t\n", "x", "Ünïcode ✓", "a.b*c>d"}),
		rapid.StringN(0, 4096, 4096),
	).Draw(t, "value")
}

// drawPlanCase draws one entity: up to eight statements, each under a generic or a
// registered alias or label predicate, with an entity-ID object (a reference under "" or
// "@id", a literal under "xsd:string"), a plain literal, or an alias or name value. roles
// counts the rows each statement must plan.
func drawPlanCase(t *rapid.T, roles rowRoles, aliasPredicates, labelPredicates []string) planCase {
	id := drawEntityID(t, "entity")
	when := time.Unix(rapid.Int64Range(1, 1<<37).Draw(t, "timestamp"), 0).UTC()
	predicate := rapid.OneOf(
		rapid.Custom(drawPredicate),
		rapid.SampledFrom(aliasPredicates),
		rapid.SampledFrom(labelPredicates),
	)
	pc := planCase{state: graph.EntityState{ID: id}}
	for range rapid.IntRange(0, 8).Draw(t, "statements") {
		triple := message.Triple{Subject: id, Source: "test", Timestamp: when}
		if rapid.Bool().Draw(t, "foreign-subject") {
			triple.Subject = drawEntityID(t, "subject")
		}
		triple.Predicate = predicate.Draw(t, "predicate")
		switch rapid.IntRange(0, 2).Draw(t, "object-kind") {
		case 0:
			triple.Object = drawEntityID(t, "object-id")
			triple.Datatype = rapid.SampledFrom([]string{"", message.EntityReferenceDatatype, "xsd:string"}).
				Draw(t, "datatype")
		case 1: // a space is outside the ID alphabet
			triple.Object = "literal " + rapid.String().Draw(t, "literal")
			triple.Datatype = rapid.SampledFrom([]string{"", "xsd:string"}).Draw(t, "datatype")
		case 2: // "@id" is left out: the codec refuses it on a value that is not an ID
			triple.Object = drawValue(t)
			triple.Datatype = rapid.SampledFrom([]string{"", "xsd:string"}).Draw(t, "datatype")
		}
		roles.count(&pc, triple)
		pc.state.Triples = append(pc.state.Triples, triple)
	}
	return pc
}
