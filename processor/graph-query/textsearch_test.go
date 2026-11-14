package graphquery

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/graphview"
	"github.com/c360studio/semengine/vocabulary"
)

// silentWatch is a watch that delivers nothing, not even the end of its replay, until its context
// ends: a view on it stays catching up.
type silentWatch struct{ updates chan jetstream.KeyValueEntry }

func (w silentWatch) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w silentWatch) Stop() error                             { return nil }

type silentSource struct{ watch silentWatch }

func (s silentSource) WatchAll(ctx context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	context.AfterFunc(ctx, func() { close(s.watch.updates) })
	return s.watch, nil
}

// TestRecordNotReadyWhileTheViewCannotServe: no community view, one running but still replaying,
// and one that has stopped each give a group's record state not_ready, never none, so a reply never
// says a group has no record when the view could not tell (spec, "No community record"). The view
// runs in a synctest bubble on a watch the test holds silent; synctest.Wait returns once Run is
// blocked on that watch.
func TestRecordNotReadyWhileTheViewCannotServe(t *testing.T) {
	const key = "0.acme.ops.src.docs"
	if _, state := groupRecord(nil, key); state != graph.RecordNotReady {
		t.Errorf("no view: record_state %q, want not_ready", state)
	}
	synctest.Test(t, func(t *testing.T) {
		view, err := graphview.New(silentSource{silentWatch{make(chan jetstream.KeyValueEntry)}}, decodeCommunityEntry)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		ran := make(chan error, 1)
		go func() { ran <- view.Run(ctx) }()
		synctest.Wait()
		if _, state := groupRecord(view, key); state != graph.RecordNotReady {
			t.Errorf("catching up: record_state %q, want not_ready", state)
		}
		cancel()
		<-ran // Run has returned, so the view has stopped.
		if _, state := groupRecord(view, key); state != graph.RecordNotReady {
			t.Errorf("stopped: record_state %q, want not_ready", state)
		}
	})
}

// TestMatchLabel: a match's label is the first non-empty string value, in triple order, of its most
// salient label predicate, the lowest priority number, ties by predicate name; with none it has no
// label (spec, "A match's label is its most salient label predicate"). A title is the label; an
// entity with no title has no label, never its ID's instance.
func TestMatchLabel(t *testing.T) {
	const (
		title     = "dc.terms.title"
		preferred = "acme.label.preferred"
		alpha     = "acme.label.alpha"
		zeta      = "acme.label.zeta"
		// display is an agentic display-name predicate, which no label set here holds.
		display = "agent.identity.display-name"
	)
	order := labelOrder(map[string]int{title: 1, preferred: 0, zeta: 2, alpha: 2})
	if want := []string{preferred, title, alpha, zeta}; !slices.Equal(order, want) {
		t.Fatalf("labelOrder = %q, want %q", order, want)
	}
	registered := labelOrder(vocabulary.DiscoverLabelPredicates())
	if !slices.Contains(registered, title) || slices.Contains(registered, display) {
		t.Fatalf("the registry's label set %q: want %s and not %s", registered, title, display)
	}

	entity := func(statements ...string) string {
		return `{"id":"acme.ops.src.docs.pump.a","triples":[` + strings.Join(statements, ",") + `]}`
	}
	fact := func(predicate, object string) string {
		return fmt.Sprintf(`{"predicate":%q,"object":%s}`, predicate, object)
	}
	for _, tc := range []struct {
		name   string
		order  []string
		entity string
		want   string
	}{
		{"a title", registered, entity(fact(title, `"Pump 7"`)), "Pump 7"},
		{"a title beside an unregistered display name", registered, entity(fact(display, `"Agent 7"`), fact(title, `"Pump 7"`)), "Pump 7"},
		{"an unregistered display name alone", registered, entity(fact(display, `"Agent 7"`)), ""},
		{"a more salient label after the title", order, entity(fact(title, `"Pump 7"`), fact(preferred, `"P7"`)), "P7"},
		{"two values of one predicate", order, entity(fact(title, `"First"`), fact(title, `"Second"`)), "First"},
		{"an empty value is passed over", order, entity(fact(title, `""`), fact(title, `"Second"`)), "Second"},
		{"a value that is not a string", order, entity(fact(title, `7`), fact(title, `"Second"`)), "Second"},
		{"a less salient label when the more salient is empty", order, entity(fact(preferred, `""`), fact(title, `"Pump 7"`)), "Pump 7"},
		{"a tie of priorities, by name", order, entity(fact(zeta, `"Z"`), fact(alpha, `"A"`)), "A"},
		{"none", order, entity(fact("acme.note.text", `"Pump 7"`)), ""},
		{"no label set", nil, entity(fact(title, `"Pump 7"`)), ""},
	} {
		entity, err := decodeSearchEntity(json.RawMessage(tc.entity))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := entity.label(tc.order); got != tc.want {
			t.Errorf("%s: label = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestTextSearchGroups: each group holding a match is listed once, in the order of its best match,
// with the number of matches it holds and its record as read; the counts sum to the matches, and
// the groups whose record is unreadable are counted (spec, "Grouped hits"). The expected lists are
// written out here from the matches' order.
func TestTextSearchGroups(t *testing.T) {
	read := graph.TextSearchGroup{CommunityID: "acme.ops.src.b", RecordState: graph.RecordRead, Record: &graph.LocalSearchRecord{MemberCount: 4}}
	poisoned := graph.TextSearchGroup{CommunityID: "acme.ops.src.a", RecordState: graph.RecordUnreadable}
	absent := graph.TextSearchGroup{CommunityID: "acme.ops.src.c", RecordState: graph.RecordNone}
	groupOf := map[string]graph.TextSearchGroup{"b1": read, "a1": poisoned, "b2": read, "c1": absent, "a2": poisoned, "b3": read}
	match := func(id string) graph.TextSearchMatch { return graph.TextSearchMatch{ID: id} }

	// b's best match is first, then a's, then c's, though a sorts first by ID.
	groups, unreadable := textGroups([]graph.TextSearchMatch{match("b1"), match("a1"), match("b2"), match("c1"), match("a2"), match("b3")}, groupOf)
	got, err := json.Marshal(groups)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"community_id":"acme.ops.src.b","matches":3,"record_state":"read","record":{"member_count":4}},` +
		`{"community_id":"acme.ops.src.a","matches":2,"record_state":"unreadable"},` +
		`{"community_id":"acme.ops.src.c","matches":1,"record_state":"none"}]`
	if string(got) != want || unreadable != 1 {
		t.Errorf("groups = %s, %d unreadable\nwant %s, 1", got, unreadable, want)
	}
	if groups, unreadable := textGroups([]graph.TextSearchMatch{}, groupOf); groups == nil || len(groups) != 0 || unreadable != 0 {
		t.Errorf("no matches: groups %v, %d unreadable; want an empty list, 0", groups, unreadable)
	}
}

// TestTextSearchFitKeepsTheBestThatFit: over max_payload the lowest-ranked matches are left out and
// truncated set, as local search fits, and the groups are the kept matches' groups, so a group whose
// only match is left out is not listed and an unreadable one stops being counted. Every limit from
// the reply with no match to the whole reply is checked against a scan written here: the most
// matches, first to last, whose encoding fits.
func TestTextSearchFitKeepsTheBestThatFit(t *testing.T) {
	pad := strings.Repeat("x", 100)
	entity := func(id string) json.RawMessage { return json.RawMessage(`{"id":"` + id + `","text":"` + pad + `"}`) }
	data := graph.TextSearchData{
		Matches:         []graph.TextSearchMatch{{ID: "a", Score: 3, Entity: entity("a")}, {ID: "b", Score: 2, Entity: entity("b")}, {ID: "c", Score: 1, Entity: entity("c")}},
		Matched:         3,
		UnhonouredHints: []string{},
	}
	groupOf := map[string]graph.TextSearchGroup{
		"a": {CommunityID: "acme.ops.src.one", RecordState: graph.RecordNone},
		"b": {CommunityID: "acme.ops.src.one", RecordState: graph.RecordNone},
		"c": {CommunityID: "acme.ops.src.two", RecordState: graph.RecordUnreadable},
	}
	encode := func(d graph.TextSearchData) ([]byte, error) { return json.Marshal(d) }
	match := func(id string, score int) string {
		return fmt.Sprintf(`{"id":%q,"score":%d,"entity":{"id":%q,"text":%q}}`, id, score, id, pad)
	}
	all := `{"level":0,"groups":[{"community_id":"acme.ops.src.one","matches":2,"record_state":"none"},` +
		`{"community_id":"acme.ops.src.two","matches":1,"record_state":"unreadable"}],` +
		`"matches":[` + match("a", 3) + `,` + match("b", 2) + `,` + match("c", 1) + `],` +
		`"matched":3,"missing":0,"bodies_not_indexed":0,"truncated":false,"unreadable_groups":1,"unhonoured_hints":[],` +
		`"answer":"Found 3 matching entities in 2 groups."}`
	two := `{"level":0,"groups":[{"community_id":"acme.ops.src.one","matches":2,"record_state":"none"}],` +
		`"matches":[` + match("a", 3) + `,` + match("b", 2) + `],` +
		`"matched":3,"missing":0,"bodies_not_indexed":0,"truncated":true,"unreadable_groups":0,"unhonoured_hints":[],` +
		`"answer":"Found 3 matching entities in 1 groups. Showing the first 2."}`
	for limit, want := range map[int]string{len(all): all, len(all) - 1: two} {
		got, sent, err := fitTextSearch(data, groupOf, limit, encode)
		if err != nil || string(got) != want {
			t.Errorf("fit at %d = %s, %v\nwant %s", limit, got, err, want)
		}
		if encoded, _ := json.Marshal(sent); string(encoded) != string(got) {
			t.Errorf("fit at %d returned the data %s, not what it encoded", limit, encoded)
		}
	}

	// The scan: each count's reply, as the reply states it, from the whole list down.
	replyOf := func(kept int) []byte {
		cut := data
		cut.Matches = data.Matches[:kept]
		cut.Groups, cut.UnreadableGroups = []graph.TextSearchGroup{}, 0
		for _, m := range cut.Matches {
			g := groupOf[m.ID]
			if i := slices.IndexFunc(cut.Groups, func(l graph.TextSearchGroup) bool { return l.CommunityID == g.CommunityID }); i >= 0 {
				cut.Groups[i].Matches++
				continue
			}
			g.Matches = 1
			cut.Groups = append(cut.Groups, g)
			if g.RecordState == graph.RecordUnreadable {
				cut.UnreadableGroups++
			}
		}
		cut.Truncated = kept < len(data.Matches)
		cut.Answer = fmt.Sprintf("Found 3 matching entities in %d groups.", len(cut.Groups))
		if cut.Truncated {
			cut.Answer += fmt.Sprintf(" Showing the first %d.", kept)
		}
		encoded, err := json.Marshal(cut)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	for limit := len(replyOf(0)); limit <= len(replyOf(len(data.Matches))); limit++ {
		kept := len(data.Matches)
		for len(replyOf(kept)) > limit {
			kept--
		}
		got, sent, err := fitTextSearch(data, groupOf, limit, encode)
		if err != nil || string(got) != string(replyOf(kept)) || sent.Truncated != (kept < len(data.Matches)) {
			t.Errorf("fit at %d = %s, %t, %v\nwant %s", limit, got, sent.Truncated, err, replyOf(kept))
		}
	}

	data.Truncated = true // the word index matched more than it returned
	if got, sent, err := fitTextSearch(data, groupOf, 1<<20, encode); err != nil || !sent.Truncated || !strings.Contains(string(got), `"truncated":true`) {
		t.Errorf("fit of a limited answer = %s, %t, %v; want it still truncated", got, sent.Truncated, err)
	}
	if got, _, err := fitTextSearch(data, groupOf, 10, encode); err == nil {
		t.Errorf("fit at 10 bytes = %s, want an error", got)
	}
}
