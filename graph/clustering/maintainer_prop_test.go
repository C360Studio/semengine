package clustering

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
	"pgregory.net/rapid"
)

// The generated checks of design D5 (task 2.3). The reference model is written from the
// community-grouping spec and design D2, never from maintainer.go or terms.go:
//
//   - an entity's groups are its ID's first four, three and two dot-separated positions,
//     split here on '.'; its type is the fifth position;
//   - its words are the generated word list (the text is that list joined by spaces, every
//     word valid by construction), capped to the MaxEntityTerms most frequent, ties
//     lexical;
//   - only the final value of each entity counts: the last delivery with a revision above
//     the entity's last-applied one, where a delete forgets the entity;
//   - a refused value (nil state, unparseable ID, mismatched state ID) contributes nothing
//     and names the entity in Tick's *IncompleteError until a valid upsert or a delete;
//   - a record holds the member count, the MaxKeywords most common types and words (ties
//     lexical), the pin's summary template, and the MaxRepEntities smallest members with
//     words; a group with no members has no record.

// propIDs are the valid entity IDs a history draws from: two deployments, two sources,
// two domains, two types and two instances, so every level holds several groups.
var propIDs = func() []string {
	var ids []string
	for _, platform := range []string{"dep1", "dep2"} {
		for _, system := range []string{"s1", "s2"} {
			for _, domain := range []string{"docs", "code"} {
				for _, typ := range []string{"note", "file"} {
					for _, instance := range []string{"i0", "i1"} {
						ids = append(ids, strings.Join([]string{"acme", platform, system, domain, typ, instance}, "."))
					}
				}
			}
		}
	}
	return ids
}()

// propBadIDs are IDs the canonical parser refuses: five positions, an empty position.
var propBadIDs = []string{"acme.dep1.s1.docs.note", "acme..s1.docs.note.i0"}

// propVocabulary are valid words: 3 to 32 ASCII letters and digits, no stop word. 45 of
// them let an entity hold more than MaxEntityTerms distinct words.
var propVocabulary = func() []string {
	var words []string
	for i := range 45 {
		words = append(words, fmt.Sprintf("w%02d", i))
	}
	return append(words, strings.Repeat("z", MaxWordBytes))
}()

const (
	kindValid      = "valid"
	kindNilState   = "nil-state"
	kindMismatched = "mismatched"
)

// propValue is one generated delivery's value.
type propValue struct {
	kind  string
	words []string
}

func (v propValue) state(id string) *graph.EntityState {
	switch v.kind {
	case kindNilState:
		return nil
	case kindMismatched:
		other := propIDs[0]
		if other == id {
			other = propIDs[1]
		}
		return &graph.EntityState{ID: other}
	}
	state := &graph.EntityState{ID: id, Triples: []message.Triple{{Subject: id, Predicate: "docs.note.size", Object: 7.0}}}
	if len(v.words) > 0 {
		state.Triples = append(state.Triples, message.Triple{Subject: id, Predicate: "docs.note.body", Object: strings.Join(v.words, " ")})
	}
	return state
}

func drawValue(rt *rapid.T, bad bool) propValue {
	kind := rapid.SampledFrom([]string{kindValid, kindValid, kindValid, kindNilState, kindMismatched}).Draw(rt, "kind")
	if bad && kind == kindMismatched {
		kind = kindValid // a bad ID is refused whatever its state
	}
	words := rapid.SliceOfN(rapid.SampledFrom(propVocabulary), 0, 60).Draw(rt, "words")
	return propValue{kind: kind, words: words}
}

// modelEntity is the model's view of one held entity.
type modelEntity struct {
	revision uint64
	value    propValue
	bad      bool
}

func (e modelEntity) refused() bool { return e.bad || e.value.kind != kindValid }

// modelTop returns the at most n keys with the highest counts, ties lexical.
func modelTop(counts map[string]int, n int) []string {
	type kc struct {
		k string
		c int
	}
	var all []kc
	for k, c := range counts {
		all = append(all, kc{k, c})
	}
	slices.SortFunc(all, func(a, b kc) int {
		if a.c != b.c {
			return b.c - a.c
		}
		return strings.Compare(a.k, b.k)
	})
	var out []string
	for i := 0; i < len(all) && i < n; i++ {
		out = append(out, all[i].k)
	}
	return out
}

// modelRecords computes every group's record from scratch over the held entities.
func modelRecords(entities map[string]modelEntity) map[string]*Community {
	type acc struct {
		level   int
		id      string
		members int
		types   map[string]int
		words   map[string]int
		text    []string
	}
	groups := map[string]*acc{}
	for id, e := range entities {
		if e.refused() {
			continue
		}
		p := strings.Split(id, ".")
		counts := map[string]int{}
		for _, w := range e.value.words {
			counts[w]++
		}
		capped := modelTop(counts, MaxEntityTerms)
		for level, n := range []int{4, 3, 2} {
			gid := strings.Join(p[:n], ".")
			key := fmt.Sprintf("%d.%s", level, gid)
			g := groups[key]
			if g == nil {
				g = &acc{level: level, id: gid, types: map[string]int{}, words: map[string]int{}}
				groups[key] = g
			}
			g.members++
			g.types[p[4]]++
			for _, w := range capped {
				g.words[w]++
			}
			if len(capped) > 0 {
				g.text = append(g.text, id)
			}
		}
	}
	out := map[string]*Community{}
	for key, g := range groups {
		types := modelTop(g.types, MaxKeywords)
		typeCounts := map[string]int{}
		var parts []string
		for i, typ := range types {
			typeCounts[typ] = g.types[typ]
			if i < 3 {
				plural := ""
				if g.types[typ] > 1 {
					plural = "s"
				}
				parts = append(parts, fmt.Sprintf("%d %s%s", g.types[typ], typ, plural))
			}
		}
		keywords := modelTop(g.words, MaxKeywords)
		summary := fmt.Sprintf("Community of %d entities including %s", g.members, strings.Join(parts, ", "))
		if len(keywords) > 0 {
			summary += ". Key themes: " + strings.Join(keywords[:min(5, len(keywords))], ", ")
		}
		slices.Sort(g.text)
		var reps []string
		if len(g.text) > 0 {
			reps = g.text[:min(MaxRepEntities, len(g.text))]
		}
		out[key] = &Community{
			ID: g.id, Level: g.level, MemberCount: g.members, TypeCounts: typeCounts,
			Keywords: keywords, StatisticalSummary: summary + ".", RepEntities: reps,
		}
	}
	return out
}

func modelRefused(entities map[string]modelEntity) []string {
	var out []string
	for id, e := range entities {
		if e.refused() {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return out
}

type matchCounts struct {
	compared, incomplete, obsolete, refusals, deletes, capped, repsFull, keywordsFull, emptied int
}

// TestMaintainerMatchesModel applies generated histories of upserts (valid and refused),
// deletes, obsolete deliveries and ticks, and after every tick past BootstrapDone compares
// every published record and Tick's result with the model computed from the held values.
// Each history starts with fixed upserts: an entity with 40 distinct words, so the
// MaxEntityTerms cap is applied in every check, and five more members with words in one
// source, so its level-1 and level-2 records hold MaxRepEntities representatives.
func TestMaintainerMatchesModel(t *testing.T) {
	var c matchCounts
	rapid.Check(t, func(rt *rapid.T) {
		m, kv, err := newFakeMaintainer()
		if err != nil {
			rt.Fatal(err)
		}
		entities := map[string]modelEntity{}
		bad := map[string]bool{}
		for _, id := range propBadIDs {
			bad[id] = true
		}
		var revision uint64
		bootstrapped := false

		upsert := func(id string, rev uint64, v propValue) {
			err := m.Upsert(id, rev, v.state(id))
			e := modelEntity{revision: rev, value: v, bad: bad[id]}
			if refused := e.refused(); refused != (err != nil) {
				rt.Fatalf("Upsert(%s, %d, %s) = %v, want refused=%v", id, rev, v.kind, err, refused)
			}
			if old, ok := entities[id]; ok && rev <= old.revision {
				c.obsolete++
				return
			}
			entities[id] = e
		}
		tick := func() {
			err := m.Tick(t.Context())
			if !bootstrapped {
				if !errors.Is(err, ErrBootstrapIncomplete) {
					rt.Fatalf("Tick before BootstrapDone = %v", err)
				}
				return
			}
			var incomplete *IncompleteError
			wantRefused := modelRefused(entities)
			switch {
			case len(wantRefused) == 0 && err != nil:
				rt.Fatalf("Tick = %v, want nil", err)
			case len(wantRefused) > 0 && (!errors.As(err, &incomplete) || !slices.Equal(incomplete.Refused, wantRefused)):
				rt.Fatalf("Tick = %v, want refused %q", err, wantRefused)
			case len(wantRefused) > 0:
				c.incomplete++
			}
			got, err := kv.decodeAll()
			if err != nil {
				rt.Fatal(err)
			}
			want := modelRecords(entities)
			if !reflect.DeepEqual(got, want) {
				keys := slices.Concat(slices.Collect(maps.Keys(got)), slices.Collect(maps.Keys(want)))
				slices.Sort(keys)
				for _, key := range keys {
					if !reflect.DeepEqual(got[key], want[key]) {
						rt.Fatalf("record %s = %+v, model %+v", key, got[key], want[key])
					}
				}
			}
			c.compared++
			for _, r := range want {
				if len(r.RepEntities) == MaxRepEntities {
					c.repsFull++
				}
				if len(r.Keywords) == MaxKeywords {
					c.keywordsFull++
				}
			}
		}

		revision++
		upsert(propIDs[0], revision, propValue{kind: kindValid, words: propVocabulary[:40]})
		c.capped++
		for _, id := range propIDs[1:6] { // with propIDs[0], five members with words in acme.dep1.s1
			revision++
			upsert(id, revision, propValue{kind: kindValid, words: []string{"w01", "w44"}})
		}

		ops := rapid.SliceOfN(rapid.SampledFrom([]string{
			"upsert", "upsert", "upsert", "upsert", "delete", "delete-held", "obsolete-upsert", "obsolete-delete", "tick",
			"bootstrap",
		}), 1, 80).Draw(rt, "ops")
		for _, op := range ops {
			switch op {
			case "upsert":
				id := rapid.SampledFrom(append(slices.Clone(propIDs), propBadIDs...)).Draw(rt, "id")
				revision++
				upsert(id, revision, drawValue(rt, bad[id]))
			case "delete", "delete-held":
				candidates := append(slices.Clone(propIDs), propBadIDs...)
				if op == "delete-held" {
					if len(entities) == 0 {
						continue
					}
					candidates = slices.Sorted(maps.Keys(entities))
				}
				id := rapid.SampledFrom(candidates).Draw(rt, "id")
				revision++
				_, held := entities[id]
				err := m.Delete(id, revision)
				if wantErr := !held && bad[id]; wantErr != (err != nil) {
					rt.Fatalf("Delete(%s) = %v, want error=%v", id, err, wantErr)
				}
				if held {
					c.deletes++
				}
				delete(entities, id)
			case "obsolete-upsert", "obsolete-delete":
				if len(entities) == 0 {
					continue
				}
				id := rapid.SampledFrom(slices.Sorted(maps.Keys(entities))).Draw(rt, "held")
				rev := rapid.Uint64Range(1, entities[id].revision).Draw(rt, "obsolete revision")
				if op == "obsolete-upsert" {
					upsert(id, rev, drawValue(rt, bad[id]))
					continue
				}
				if err := m.Delete(id, rev); err != nil {
					rt.Fatalf("obsolete Delete(%s, %d) = %v", id, rev, err)
				}
				c.obsolete++
			case "tick":
				tick()
			case "bootstrap":
				m.BootstrapDone()
				bootstrapped = true
			}
		}
		for _, e := range entities {
			if e.refused() {
				c.refusals++
			}
		}
		m.BootstrapDone()
		bootstrapped = true
		tick()

		// An entity no generated delivery names is alone in its three groups; deleting it
		// empties them, and the next tick must remove their records.
		lone := "acme.dep3.s9.misc.note.i0"
		revision++
		upsert(lone, revision, propValue{kind: kindValid, words: []string{"w00"}})
		tick()
		revision++
		if err := m.Delete(lone, revision); err != nil {
			rt.Fatal(err)
		}
		delete(entities, lone)
		tick()
		c.emptied++
	})
	t.Logf("assertions run: compared ticks %d (incomplete %d), obsolete deliveries %d, refused at the end %d, "+
		"deletes of held entities %d, capped entities %d, records at MaxRepEntities %d, at MaxKeywords %d, groups emptied %d",
		c.compared, c.incomplete, c.obsolete, c.refusals, c.deletes, c.capped, c.repsFull, c.keywordsFull, c.emptied)
}

// propDelivery is one delivery of one entity's history.
type propDelivery struct {
	id       string
	revision uint64
	delete   bool
	value    propValue
}

// TestHistoryOrderIrrelevant: two histories that interleave the same per-entity
// deliveries differently, each entity's in its own revision order, with ticks at
// different points, leave byte-identical records and the same Tick result. Obsolete
// deliveries are inserted only after an upsert, where the entity is still held.
func TestHistoryOrderIrrelevant(t *testing.T) {
	var differing, obsolete, nonEmpty int
	rapid.Check(t, func(rt *rapid.T) {
		ids := rapid.SliceOfNDistinct(rapid.SampledFrom(append(slices.Clone(propIDs), propBadIDs...)), 1, 10,
			rapid.ID[string]).Draw(rt, "entities")
		var perEntity [][]propDelivery
		for _, id := range ids {
			isBad := slices.Contains(propBadIDs, id)
			var seq []propDelivery
			n := rapid.IntRange(1, 4).Draw(rt, "deliveries")
			for rev := uint64(1); rev <= uint64(n); rev++ {
				d := propDelivery{id: id, revision: rev}
				if rapid.IntRange(0, 3).Draw(rt, "delete?") == 0 {
					d.delete = true
				} else {
					d.value = drawValue(rt, isBad)
				}
				seq = append(seq, d)
				if !d.delete && rapid.Bool().Draw(rt, "obsolete?") {
					o := propDelivery{id: id, revision: rapid.Uint64Range(1, rev).Draw(rt, "obsolete revision")}
					if rapid.Bool().Draw(rt, "obsolete delete?") {
						o.delete = true
					} else {
						o.value = drawValue(rt, isBad)
					}
					seq = append(seq, o)
					obsolete++
				}
			}
			perEntity = append(perEntity, seq)
		}

		run := func(label string) (map[string][]byte, string, []string) {
			m, kv, err := newFakeMaintainer()
			if err != nil {
				rt.Fatal(err)
			}
			m.BootstrapDone()
			next := make([]int, len(perEntity))
			var order []string
			for {
				var open []int
				for i, seq := range perEntity {
					if next[i] < len(seq) {
						open = append(open, i)
					}
				}
				if len(open) == 0 {
					break
				}
				if rapid.IntRange(0, 4).Draw(rt, label+" tick?") == 0 {
					_ = m.Tick(t.Context()) // only the final records are compared
					order = append(order, "tick")
				}
				i := rapid.SampledFrom(open).Draw(rt, label+" entity")
				d := perEntity[i][next[i]]
				next[i]++
				if d.delete {
					_ = m.Delete(d.id, d.revision)
				} else {
					_ = m.Upsert(d.id, d.revision, d.value.state(d.id))
				}
				order = append(order, fmt.Sprintf("%s@%d", d.id, d.revision))
			}
			var result string
			if err := m.Tick(t.Context()); err != nil {
				result = err.Error()
			}
			kv.mu.Lock()
			defer kv.mu.Unlock()
			return maps.Clone(kv.data), result, order
		}
		dataA, resultA, orderA := run("A")
		dataB, resultB, orderB := run("B")
		if resultA != resultB {
			rt.Fatalf("final Tick: %q after %q, %q after %q", resultA, orderA, resultB, orderB)
		}
		if !maps.EqualFunc(dataA, dataB, bytes.Equal) {
			rt.Fatalf("records differ:\n%q\n%q\nafter %q and %q", dataA, dataB, orderA, orderB)
		}
		if !slices.Equal(orderA, orderB) {
			differing++
		}
		if len(dataA) > 0 {
			nonEmpty++
		}
	})
	t.Logf("assertions run: differing interleavings %d, obsolete deliveries %d, non-empty final indexes %d",
		differing, obsolete, nonEmpty)
}
