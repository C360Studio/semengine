//go:build integration

package graphindex

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/vocabulary"
)

// The generated histories in this file. An entity takes one of three values, A, B or C, or is
// deleted (""), under one of two sets of alias predicates: the framework's own, or those plus
// serialPredicate. The reference model (modelRows) spells, from the key layouts, the rows each value
// owns under each set. Cut states are seeded by running a prefix of an apply's own operations
// (prepareApply, then runApply on the first n), with no hook in the apply.
//
// The values are chosen to reach each kind of change: A to B replaces every row; A to C keeps the
// edge to the first target and adds one to the second, keeps the alias, and keeps the name row's key
// while changing its value (the name differs only in case); serialPredicate's values become alias
// rows only under the second set.

// serialPredicate is an alias predicate only under the second alias set.
const serialPredicate = "test.alias.serial"

// historyValues is every value an entity takes, "" for deleted.
var historyValues = []string{"A", "B", "C", ""}

// statementsOf is value's statements for id; n is id's last token, which keeps names, aliases and
// targets apart across entities.
func statementsOf(id, value string) []stmt {
	n := id[strings.LastIndex(id, ".")+1:]
	first, second := targetOf(id, "a"), targetOf(id, "b")
	title := vocabulary.DCTermsTitle
	switch value {
	case "A":
		return []stmt{{linkTo, first}, {armed, true}, {title, "Alpha " + n}, {alias, "alpha-" + n}, {serialPredicate, "s1-" + n}}
	case "B":
		return []stmt{{linkTo, second}, {disarmed, true}, {title, "Beta " + n}, {alias, "beta-" + n}}
	case "C":
		return []stmt{{linkTo, first}, {linkTo, second}, {title, "alpha " + n}, {alias, "alpha-" + n}, {serialPredicate, "s2-" + n}}
	}
	return nil
}

// targetOf is one of id's edge targets, an entity no test writes.
func targetOf(id, which string) string {
	return "acme.ops.test.system.target." + id[strings.LastIndex(id, ".")+1:] + which
}

// valueOf is the ENTITY_STATES value for id holding value, nil for deleted.
func valueOf(t *testing.T, id, value string) []byte {
	t.Helper()
	if value == "" {
		return nil
	}
	return entityValue(t, id, statementsOf(id, value)...)
}

// modelRows is every row id owns holding value under the alias set (serialAlias: the second),
// spelled from the key layouts: OUTGOING id holding the edges in statement order, INCOMING
// target.id.hex(predicate), PREDICATE predicate.id, NAME and ALIAS sha256(folded text).id.hex(predicate),
// a name's value its spelling and its predicate's priority.
func modelRows(id, value string, serialAlias bool, titlePriority int) map[rowID]string {
	stmts := statementsOf(id, value)
	if stmts == nil {
		return map[rowID]string{}
	}
	rows := map[rowID]string{}
	var edges []string
	for _, s := range stmts {
		text, _ := s.object.(string)
		rows[rowID{graph.BucketPredicateIndex, s.predicate + "." + id}] = ""
		switch {
		case s.predicate == linkTo:
			edges = append(edges, fmt.Sprintf(`{"to_entity_id":%q,"predicate":%q}`, text, linkTo))
			rows[rowID{graph.BucketIncomingIndex, text + "." + id + "." + hexOf(linkTo)}] = ""
		case s.predicate == vocabulary.DCTermsTitle:
			rows[rowID{graph.BucketNameIndex, nameHash(text) + "." + id + "." + hexOf(s.predicate)}] =
				fmt.Sprintf(`{"name":%q,"priority":%d}`, text, titlePriority)
		case s.predicate == alias, s.predicate == serialPredicate && serialAlias:
			rows[rowID{graph.BucketAliasIndex, nameHash(text) + "." + id + "." + hexOf(s.predicate)}] = ""
		}
	}
	rows[rowID{graph.BucketOutgoingIndex, id}] = "[" + strings.Join(edges, ",") + "]"
	return rows
}

// errorReporter is what an assertion reports through: the test, or Rapid's case.
type errorReporter interface {
	Errorf(format string, args ...any)
}

// requireRowsEqual reports where id's stored rows differ from want.
func requireRowsEqual(report errorReporter, what string, got, want map[rowID]string) {
	for _, place := range slices.SortedFunc(maps.Keys(want), compareIDs) {
		if value, ok := got[place]; !ok || value != want[place] {
			report.Errorf("%s: row %s/%s = %q (stored %v), want %q", what, place.bucket, place.key, value, ok, want[place])
		}
	}
	for _, place := range slices.SortedFunc(maps.Keys(got), compareIDs) {
		if _, ok := want[place]; !ok {
			report.Errorf("%s: stray row %s/%s = %q", what, place.bucket, place.key, got[place])
		}
	}
}

func compareIDs(a, b rowID) int {
	return compareRows(recordRow{bucket: a.bucket, key: a.key}, recordRow{bucket: b.bucket, key: b.key})
}

// aliasSets switches the vocabulary between the two alias sets and points a writer at it.
type aliasSets struct {
	restore func() // the vocabulary as the test found it, the first set
}

// use makes serialAlias's set the vocabulary's and, for each writer, the set it plans with.
func (s aliasSets) use(serialAlias bool, writers ...*Component) {
	s.restore()
	if serialAlias {
		vocabulary.Register(serialPredicate, vocabulary.WithAlias(vocabulary.AliasTypeIdentity, 0))
	}
	for _, w := range writers {
		w.aliasPredicates = vocabulary.DiscoverAliasPredicates()
		w.namePredicates = vocabulary.DiscoverLabelPredicates()
	}
}

// cutApply is one apply cut off: value under its alias set, after prefix of its operations
// (prefix modulo the operation count, so a generated prefix reaches every cut point).
type cutApply struct {
	value       string
	serialAlias bool
	prefix      int
}

// convergeCase is one TestRestartConvergesFromEveryCutPoint case: a completed apply of initial (none
// when initial is nil), cuts one after another, each from the store the one before left, then a start
// of graph-index over final under its alias set.
type convergeCase struct {
	initial      *cutApply
	cuts         []cutApply
	final        string
	finalAliases bool
}

// TestRestartConvergesFromEveryCutPoint cuts an entity's writes off at any point, one or more times
// in a row (each recovery cut again), stores value A, B or C, and starts graph-index under either
// alias set. Every read then answers that value under those rules and nothing else for the entity:
// its rows equal the model's, its record is clean and names exactly them, and the public reads agree.
//
// Named witnesses run first: every cut point of the change A to B, two and three cuts in a row, a cut
// delete, and a set change between the cut and the start. The generated run draws the rest.
func TestRestartConvergesFromEveryCutPoint(t *testing.T) {
	sets := aliasSets{restore: vocabulary.SnapshotRegistry()}
	defer sets.restore()
	client := fixtureClient(t)
	states := ensureEntityStates(t, client)
	writer := openWriter(t, client)
	titlePriority := vocabulary.DiscoverLabelPredicates()[vocabulary.DCTermsTitle]
	entities := 0

	run := func(report errorReporter, c convergeCase) {
		entities++
		id := fmt.Sprintf("acme.ops.test.system.widget.%d", 1000+entities)
		if c.initial != nil {
			sets.use(c.initial.serialAlias, writer)
			apply := prepareFor(t, writer, id, valueOf(t, id, c.initial.value))
			runPrefix(t, writer, apply, len(apply.ops))
		}
		for _, cut := range c.cuts {
			sets.use(cut.serialAlias, writer)
			apply := prepareFor(t, writer, id, valueOf(t, id, cut.value))
			if len(apply.ops) > 0 {
				runPrefix(t, writer, apply, cut.prefix%len(apply.ops))
			}
		}
		rev, err := states.Put(t.Context(), id, valueOf(t, id, c.final))
		if err != nil {
			t.Fatalf("put %s: %v", id, err)
		}
		sets.use(c.finalAliases, writer)
		h := restart(t, client, `{}`)
		defer stopComponent(t, h.c)
		h.awaitIndexed(t, rev) // the floor orders every read below after the entity's apply
		requireConverged(t, report, h, id, c.final, c.finalAliases, titlePriority, rev)
	}

	t.Run("witnesses", func(t *testing.T) {
		a := &cutApply{value: "A"}
		for prefix := range 12 {
			run(t, convergeCase{initial: a, cuts: []cutApply{{value: "B", prefix: prefix}}, final: "B"})
			run(t, convergeCase{initial: a, cuts: []cutApply{{value: "B", prefix: prefix}}, final: "A"})
		}
		run(t, convergeCase{initial: a, cuts: []cutApply{{value: "B", prefix: 2}, {value: "C", prefix: 1}}, final: "A"})
		run(t, convergeCase{initial: a, cuts: []cutApply{{value: "C", prefix: 3}, {value: "B", prefix: 2}, {value: "", prefix: 4}}, final: "C"})
		run(t, convergeCase{initial: a, cuts: []cutApply{{value: "", prefix: 3}}, final: "B"})
		run(t, convergeCase{cuts: []cutApply{{value: "C", prefix: 2, serialAlias: true}}, final: "A"})
		run(t, convergeCase{initial: &cutApply{value: "C"}, cuts: []cutApply{{value: "A", prefix: 1}}, final: "C", finalAliases: true})
	})

	t.Run("generated", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			drawCut := func(label string) cutApply {
				return cutApply{
					value:       rapid.SampledFrom(historyValues).Draw(rt, label+" value"),
					serialAlias: rapid.Bool().Draw(rt, label+" aliases"),
					prefix:      rapid.IntRange(0, 31).Draw(rt, label+" prefix"),
				}
			}
			c := convergeCase{
				final:        rapid.SampledFrom(historyValues[:3]).Draw(rt, "final value"),
				finalAliases: rapid.Bool().Draw(rt, "final aliases"),
			}
			if rapid.Bool().Draw(rt, "indexed first") {
				initial := drawCut("initial")
				c.initial = &initial
			}
			for i := range rapid.IntRange(1, 3).Draw(rt, "cuts") {
				c.cuts = append(c.cuts, drawCut(fmt.Sprintf("cut %d", i)))
			}
			run(rt, c)
		})
	})
}

// requireConverged reports where id, after h's apply of value under its alias set, is not exactly
// the model: its rows, its record, and the public reads of it.
func requireConverged(t *testing.T, report errorReporter, h *harness, id, value string, serialAlias bool,
	titlePriority int, rev uint64) {
	t.Helper()
	want := modelRows(id, value, serialAlias, titlePriority)
	stored := ownedRows(t, h.client, id)
	requireRowsEqual(report, id+" "+value, stored, want)
	if r, ok := storedRecord(t, h.client, id); !ok || r.pending {
		report.Errorf("%s: record present %v, pending %v; want clean", id, ok, r.pending)
	} else if problems := recordNames(r, stored, true); len(problems) > 0 {
		report.Errorf("%s: clean record disagrees with its rows: %v", id, problems)
	}
	if n := h.c.failedTotal(); n != 0 {
		report.Errorf("%s: %d failed entities, want 0", id, n)
	}

	n := id[strings.LastIndex(id, ".")+1:]
	wantAliases := map[string]bool{}
	wantNames := map[string]bool{}
	var wantOut []graph.OutgoingEntry
	for _, s := range statementsOf(id, value) {
		text, _ := s.object.(string)
		switch {
		case s.predicate == linkTo:
			wantOut = append(wantOut, graph.OutgoingEntry{ToEntityID: text, Predicate: linkTo})
		case s.predicate == alias, s.predicate == serialPredicate && serialAlias:
			wantAliases[text] = true
		case s.predicate == vocabulary.DCTermsTitle:
			wantNames[strings.ToLower(text)] = true
		}
	}
	out := answer[graph.OutgoingRelationshipsData](t, h.client, "outgoing", map[string]string{"entity_id": id}, rev)
	if !slices.Equal(out.Relationships, wantOut) {
		report.Errorf("%s: outgoing = %v, want %v", id, out.Relationships, wantOut)
	}
	for _, which := range []string{"a", "b"} {
		target := targetOf(id, which)
		in := answer[graph.IncomingRelationshipsData](t, h.client, "incoming", map[string]string{"entity_id": target}, rev)
		var wantIn []graph.IncomingEntry
		if slices.Contains(wantOut, graph.OutgoingEntry{ToEntityID: target, Predicate: linkTo}) {
			wantIn = []graph.IncomingEntry{{FromEntityID: id, Predicate: linkTo}}
		}
		if !slices.Equal(in.Relationships, wantIn) {
			report.Errorf("%s: incoming of %s = %v, want %v", id, target, in.Relationships, wantIn)
		}
	}
	for _, text := range []string{"alpha-" + n, "beta-" + n, "s1-" + n, "s2-" + n} {
		want := ""
		if wantAliases[text] {
			want = id
		}
		if got := resolveAlias(t, h, text, rev); got != want {
			report.Errorf("%s: alias %q = %q, want %q", id, text, got, want)
		}
	}
	for _, name := range []string{"alpha " + n, "beta " + n} {
		var want []string
		if wantNames[name] {
			want = []string{id}
		}
		matches := answer[graph.NameData](t, h.client, "byName", map[string]string{"name": name}, rev).Matches
		if got := nameIDs(matches); !slices.Equal(got, want) {
			report.Errorf("%s: byName %q = %v, want %v", id, name, got, want)
		}
	}
}

// historyStep is one step of a TestRecordMatchesListingOracle history, on entity entity of two:
// "apply" (a change, a re-offer when value is the stored one, or a delete), "cut" (an apply stopped
// after prefix of its operations, modulo their count) or "second writer" (an apply of value whose
// record read is followed by another writer's apply of otherValue, cut after otherPrefix of its
// operations, at least its pending put, before its pending put).
type historyStep struct {
	kind        string
	entity      int
	value       string
	serialAlias bool
	prefix      int
	otherValue  string
	otherPrefix int
}

// TestRecordMatchesListingOracle runs a generated history of changes, re-offers, deletes, cuts and
// second writers over two entities and two alias sets, ending with a completed apply of each. After
// every step each entity with a pending record has every row a listing of the index buckets finds
// named by it, each with a clean record has exactly those rows named by it, value hashes included,
// and one with no record has no rows. A second writer between an apply's record read and its pending
// put fails the apply with nothing written. At the end each record is clean and the rows are the
// model's for the last value.
func TestRecordMatchesListingOracle(t *testing.T) {
	sets := aliasSets{restore: vocabulary.SnapshotRegistry()}
	defer sets.restore()
	client := fixtureClient(t)
	ensureEntityStates(t, client)
	first, second := openWriter(t, client), openWriter(t, client)
	titlePriority := vocabulary.DiscoverLabelPredicates()[vocabulary.DCTermsTitle]
	histories := 0

	run := func(report errorReporter, steps []historyStep, last []string, lastAliases bool) {
		histories++
		ids := []string{
			fmt.Sprintf("acme.ops.test.system.widget.%d", 2000+2*histories),
			fmt.Sprintf("acme.ops.test.system.widget.%d", 2001+2*histories),
		}
		requireNamed := func(when string) {
			listed := ownedRowsOf(t, client, ids...)
			for _, id := range ids {
				stored := listed[id]
				r, ok := storedRecord(t, client, id)
				switch {
				case !ok && len(stored) > 0:
					report.Errorf("%s: %s has %d rows and no record", when, id, len(stored))
				case ok:
					if problems := recordNames(r, stored, !r.pending); len(problems) > 0 {
						report.Errorf("%s: %s's record (pending %v) disagrees with a listing: %v", when, id, r.pending, problems)
					}
				}
			}
		}
		for i, step := range steps {
			id := ids[step.entity]
			sets.use(step.serialAlias, first, second)
			apply := prepareFor(t, first, id, valueOf(t, id, step.value))
			switch step.kind {
			case "apply":
				runPrefix(t, first, apply, len(apply.ops))
			case "cut":
				if len(apply.ops) > 0 {
					runPrefix(t, first, apply, step.prefix%len(apply.ops))
				}
			case "second writer":
				other := prepareFor(t, second, id, valueOf(t, id, step.otherValue))
				if len(other.ops) == 0 {
					break // the second writer has nothing to write, so it cannot change the record
				}
				runPrefix(t, second, other, 1+step.otherPrefix%len(other.ops))
				// Every row or record write moves its bucket's stream sequence.
				sequencesBefore, recordBefore := indexSequences(t, client), recordSequence(t, client)
				err := first.runApply(t.Context(), apply)
				if len(apply.ops) > 0 && !errors.Is(err, errRecordConflict) {
					report.Errorf("step %d: the apply after a second writer: %v, want errRecordConflict", i, err)
				}
				if got := indexSequences(t, client); !maps.Equal(got, sequencesBefore) || recordSequence(t, client) != recordBefore {
					report.Errorf("step %d: the refused apply wrote: index sequences %v, want %v", i, got, sequencesBefore)
				}
			}
			requireNamed(fmt.Sprintf("after step %d (%s)", i, step.kind))
		}
		sets.use(lastAliases, first)
		for e, id := range ids {
			apply := prepareFor(t, first, id, valueOf(t, id, last[e]))
			runPrefix(t, first, apply, len(apply.ops))
			stored := ownedRows(t, client, id)
			requireRowsEqual(report, "at the end, "+id+" "+last[e], stored, modelRows(id, last[e], lastAliases, titlePriority))
			if r, ok := storedRecord(t, client, id); !ok || r.pending {
				report.Errorf("at the end: %s's record present %v, pending %v; want clean", id, ok, r.pending)
			} else if problems := recordNames(r, stored, true); len(problems) > 0 {
				report.Errorf("at the end: %s's clean record disagrees with a listing: %v", id, problems)
			}
		}
	}

	t.Run("witnesses", func(t *testing.T) {
		run(t, []historyStep{
			{kind: "apply", value: "A"},
			{kind: "cut", value: "B", prefix: 3},
			{kind: "cut", value: "C", prefix: 2, serialAlias: true},
			{kind: "apply", value: "C"},
			{kind: "apply", value: "C"}, // a re-offer
			{kind: "second writer", value: "B", otherValue: "A", otherPrefix: 1},
			{kind: "apply", value: ""},
			{kind: "apply", entity: 1, value: "B"},
			{kind: "second writer", entity: 1, value: "", otherValue: "C", otherPrefix: 99},
		}, []string{"B", "A"}, true)
	})

	t.Run("generated", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			var steps []historyStep
			for i := range rapid.IntRange(1, 8).Draw(rt, "steps") {
				label := fmt.Sprintf("step %d ", i)
				steps = append(steps, historyStep{
					kind:        rapid.SampledFrom([]string{"apply", "cut", "second writer"}).Draw(rt, label+"kind"),
					entity:      rapid.IntRange(0, 1).Draw(rt, label+"entity"),
					value:       rapid.SampledFrom(historyValues).Draw(rt, label+"value"),
					serialAlias: rapid.Bool().Draw(rt, label+"aliases"),
					prefix:      rapid.IntRange(0, 31).Draw(rt, label+"prefix"),
					otherValue:  rapid.SampledFrom(historyValues).Draw(rt, label+"other value"),
					otherPrefix: rapid.IntRange(0, 31).Draw(rt, label+"other prefix"),
				})
			}
			last := []string{
				rapid.SampledFrom(historyValues).Draw(rt, "last value 0"),
				rapid.SampledFrom(historyValues).Draw(rt, "last value 1"),
			}
			run(rt, steps, last, rapid.Bool().Draw(rt, "last aliases"))
		})
	})
}
