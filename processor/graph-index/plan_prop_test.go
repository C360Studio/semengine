package graphindex

import (
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
// two 256-byte IDs, two dots and the hex of a 194-byte predicate (design D6).
const maxIncomingKeyBytes = 2*semtypes.MaxEntityIDBytes + 2 + 2*vocabulary.MaxPredicateBytes

// planCase is one generated entity with the rows the requirement says it plans, counted
// by the generator as it draws each statement, never read back from the plan builder.
type planCase struct {
	state         graph.EntityState
	relationships int // statements whose object is an entity reference
	aliases       int // alias-predicate statements whose value is non-empty after trimming (ADR-114 §5)
	names         int // label-predicate statements whose value is non-empty
}

// fatalf is what the witness (*testing.T) and the generated run (*rapid.T) share.
type fatalf interface {
	Fatalf(format string, args ...any)
}

// TestContractValidStateAlwaysPlans: every entity state UnmarshalEntityState accepts gets
// an index plan (design D6, #428), so the failed mark on a refused plan is reached by no
// stored value. The generator draws six-part IDs and predicates up to and, by a drawn
// flag, exactly at their byte bounds (256 and 194), relationships under both reference
// spellings, literals, and alias and name values of any bytes, including blank ones, under
// an explicit literal datatype. The named witness runs first and plans the longest incoming
// key, 902 bytes, every time; named cases then cover an ID-shaped alias and name, literal
// and inferred reference.
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

	// An ID-shaped alias or name value is also a relationship unless an explicit
	// non-reference datatype makes it a literal (message.Triple.IsRelationship, #540).
	for _, tc := range []struct {
		name, predicate, datatype     string
		relationships, aliases, names int
	}{
		{"ID-shaped alias, literal", "test.alias.serial", "xsd:string", 0, 1, 0},
		{"ID-shaped alias, inferred reference", "test.alias.serial", "", 1, 1, 0},
		{"ID-shaped name, literal", "test.name.label", "xsd:string", 0, 0, 1},
		{"ID-shaped name, inferred reference", "test.name.label", "", 1, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pc := planCase{state: graph.EntityState{ID: "a.b.c.d.e.source", Triples: []message.Triple{{
				Subject: "a.b.c.d.e.source", Predicate: tc.predicate, Object: "a.b.c.d.e.target",
				Datatype: tc.datatype, Source: "test", Timestamp: time.Unix(1, 0).UTC(),
			}}}, relationships: tc.relationships, aliases: tc.aliases, names: tc.names}
			checkStatePlans(t, c, pc)
		})
	}

	var checks, atMaxKey int
	rapid.Check(t, func(rt *rapid.T) {
		pc := drawPlanCase(rt, aliasPredicates, labelPredicates)
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
// [a-z][a-z0-9]*(-[a-z0-9]+)*, each segment padded with letters to 64 bytes (making the
// predicate 194) by a drawn flag, otherwise to a drawn length up to 64.
func drawPredicate(t *rapid.T) string {
	segment := rapid.StringMatching(`[a-z][a-z0-9]{0,5}(-[a-z0-9]{1,5}){0,2}`)
	atBound := rapid.Bool().Draw(t, "predicate-at-bound")
	parts := make([]string, 3)
	for i := range parts {
		s := segment.Draw(t, "predicate-segment")
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

// drawPlanCase draws one entity: up to eight statements, each a relationship, a literal,
// an alias or a name, counting the rows each must plan.
func drawPlanCase(t *rapid.T, aliasPredicates, labelPredicates []string) planCase {
	id := drawEntityID(t, "entity")
	when := time.Unix(rapid.Int64Range(1, 1<<37).Draw(t, "timestamp"), 0).UTC()
	pc := planCase{state: graph.EntityState{ID: id}}
	for range rapid.IntRange(0, 8).Draw(t, "statements") {
		triple := message.Triple{Subject: id, Source: "test", Timestamp: when}
		if rapid.Bool().Draw(t, "foreign-subject") {
			triple.Subject = drawEntityID(t, "subject")
		}
		switch rapid.IntRange(0, 4).Draw(t, "kind") {
		case 0: // relationship; "" and "@id" both make an ID object a reference
			triple.Predicate = drawPredicate(t)
			triple.Object = drawEntityID(t, "target")
			triple.Datatype = rapid.SampledFrom([]string{"", message.EntityReferenceDatatype}).Draw(t, "datatype")
			pc.relationships++
		case 1: // literal string: a space is outside the ID alphabet, so it is never a reference
			triple.Predicate = drawPredicate(t)
			triple.Object = "literal " + rapid.String().Draw(t, "literal")
		case 2: // ID-shaped text under an explicit non-reference datatype is a literal
			triple.Predicate = drawPredicate(t)
			triple.Object = drawEntityID(t, "literal-id")
			triple.Datatype = "xsd:string"
		case 3: // an explicit literal datatype: a drawn value of entity-ID shape is not also a relationship (#540)
			triple.Predicate = rapid.SampledFrom(aliasPredicates).Draw(t, "alias-predicate")
			value := drawValue(t)
			triple.Object = value
			triple.Datatype = "xsd:string"
			if strings.TrimSpace(value) != "" {
				pc.aliases++
			}
		case 4: // literal datatype, as in case 3
			triple.Predicate = rapid.SampledFrom(labelPredicates).Draw(t, "label-predicate")
			value := drawValue(t)
			triple.Object = value
			triple.Datatype = "xsd:string"
			if value != "" {
				pc.names++
			}
		}
		pc.state.Triples = append(pc.state.Triples, triple)
	}
	return pc
}
