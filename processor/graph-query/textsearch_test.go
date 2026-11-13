package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/pkg/graphview"
	"github.com/c360studio/semengine/vocabulary"
)

// records keys each group record as COMMUNITY_INDEX does, {level}.{id}, in the order given.
func records(groups ...clustering.Community) []graphview.KeyedEntry[clustering.Community] {
	entries := make([]graphview.KeyedEntry[clustering.Community], 0, len(groups))
	for _, group := range groups {
		entries = append(entries, graphview.KeyedEntry[clustering.Community]{
			Key: fmt.Sprintf("%d.%s", group.Level, group.ID), Value: group,
		})
	}
	return entries
}

func groupIDs(groups []clustering.Community) []string {
	ids := make([]string, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	return ids
}

// TestTextTierScore: a record gains 2.0 for each term in its statistical summary and 1.5 for each
// keyword and term where one contains the other, all lowercased, over the question's distinct
// terms; up to five records scoring above 0 are read, best first, ties by key (spec, "The text tier
// reads at most five matching groups, one page each"). Its "no record scores" case holds records
// that all score 0, none of which is read, and its "ties by key" case gives ties out of key order,
// which are read by key.
func TestTextTierScore(t *testing.T) {
	t.Run("the rule's cases", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			record clustering.Community
			query  string
			score  float64
		}{
			{"a term in the summary", clustering.Community{StatisticalSummary: "Pump Station"}, "pump", 2.0},
			{"a keyword containing the term", clustering.Community{Keywords: []string{"PUMPS"}}, "pump", 1.5},
			{"a term containing the keyword", clustering.Community{Keywords: []string{"pu"}}, "PUMP", 1.5},
			{"neither", clustering.Community{StatisticalSummary: "valve", Keywords: []string{"valve"}}, "pump", 0},
			{"each term and each keyword", clustering.Community{
				StatisticalSummary: "pump and valve", Keywords: []string{"pump", "pumps", "valve"},
			}, "pump valve", 2.0 + 2.0 + 1.5 + 1.5 + 1.5},
			// A repeated term would weigh twice; the terms are distinct here.
			{"a repeated term counts once", clustering.Community{StatisticalSummary: "pump", Keywords: []string{"pump"}}, "pump Pump", 3.5},
		} {
			if got := groupScore(tc.record, searchTerms(tc.query)); got != tc.score {
				t.Errorf("%s: groupScore(%q) = %v, want %v", tc.name, tc.query, got, tc.score)
			}
		}
	})

	t.Run("P2: ties by key", func(t *testing.T) {
		a := clustering.Community{ID: "a", StatisticalSummary: "pump station", Keywords: []string{"pump"}}
		b := clustering.Community{ID: "b", StatisticalSummary: "valve station", Keywords: []string{"pu"}}
		c := clustering.Community{ID: "c", StatisticalSummary: "valve", Keywords: []string{"valve", "pumps"}}
		// a scores 3.5 for "pump" and b, c 1.5 each; all three 1.5 for "pumps". The input order is
		// c, b, a, so a sort that leaves ties in input order fails.
		for _, query := range []string{"pump", "pump pump", "pumps"} {
			got := groupIDs(textGroups(records(c, b, a), searchTerms(query)))
			if want := []string{"a", "b", "c"}; !slices.Equal(got, want) {
				t.Errorf("%q: groups = %q, want %q", query, got, want)
			}
		}
	})

	t.Run("P1: no record scores, none is read", func(t *testing.T) {
		var seven []clustering.Community
		for i := range 7 {
			seven = append(seven, clustering.Community{
				ID: fmt.Sprintf("g%d", i), StatisticalSummary: "sensors and valves", Keywords: []string{"valve"},
			})
		}
		if got := textGroups(records(seven...), searchTerms("pump")); len(got) != 0 {
			t.Errorf("groups = %q, want none: the pin read the first five at score 0", groupIDs(got))
		}
	})

	t.Run("five of seven, best first", func(t *testing.T) {
		// Scores for "pump valve": e 7, c 4, g 3.5, a 3, b 2, f 2, d 1.5, h 0. b and f tie, and b's
		// key is first, so f is the sixth.
		got := groupIDs(textGroups(records(
			clustering.Community{ID: "a", Keywords: []string{"pump", "valve"}},
			clustering.Community{ID: "b", StatisticalSummary: "pump"},
			clustering.Community{ID: "c", StatisticalSummary: "pump valve"},
			clustering.Community{ID: "d", Keywords: []string{"pumps"}},
			clustering.Community{ID: "e", StatisticalSummary: "pump valve", Keywords: []string{"pump", "valve"}},
			clustering.Community{ID: "f", StatisticalSummary: "valve"},
			clustering.Community{ID: "g", StatisticalSummary: "pump", Keywords: []string{"valve"}},
			clustering.Community{ID: "h", StatisticalSummary: "gear", Keywords: []string{"gear"}},
		), searchTerms("pump valve")))
		if want := []string{"e", "c", "g", "a", "b"}; !slices.Equal(got, want) {
			t.Errorf("groups = %q, want %q", got, want)
		}
	})
}

// TestTextTierTermLimit: more than graph.MaxSearchTerms distinct terms is the caller's
// invalid_request, refused before the community view is read, so it wins over index_not_ready; at
// the limit the question reaches the view, which this component, never started, does not have
// (spec, "The text tier refuses an index it cannot read").
func TestTextTierTermLimit(t *testing.T) {
	words := strings.Fields("alpha bravo charlie delta echo foxtrot golf hotel india juliet kilo lima mike november oscar papa quebec")
	atLimit := strings.Join(words[:graph.MaxSearchTerms], " ")
	over := strings.Join(words[:graph.MaxSearchTerms+1], " ")

	c, raw, err := askUnstarted(t, fmt.Sprintf(`{"query":%q}`, over))
	refusal := refusalOfRoute(t, over, err, errs.ErrorInvalid, graph.ErrorCodeInvalidRequest)
	if want := "invalid request: query holds 17 distinct terms, more than 16"; refusal.Error() != want {
		t.Errorf("message = %q, want %q", refusal.Error(), want)
	}
	if raw != nil || c.Health().ErrorCount != 0 {
		t.Errorf("reply %q, ErrorCount %d; want none and 0", raw, c.Health().ErrorCount)
	}

	_, _, err = askUnstarted(t, fmt.Sprintf(`{"query":%q}`, atLimit))
	refusal = refusalOfRoute(t, atLimit, err, errs.ErrorTransient, graph.ErrorCodeIndexNotReady)
	if refusal.Error() != noCommunityView {
		t.Errorf("message = %q, want %q", refusal.Error(), noCommunityView)
	}
}

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

// TestTextTierRefusesAViewCatchingUp: a community view that is running but has not finished its
// replay is index_not_ready, transient, as is no view and a view that has stopped (spec, "The text
// tier refuses an index it cannot read"). The view runs in a synctest bubble on a watch the test
// holds silent; synctest.Wait returns once Run is blocked on that watch.
func TestTextTierRefusesAViewCatchingUp(t *testing.T) {
	// cause is the view's own error the refusal carries, so a refusal for another reason (a level
	// with no record) does not pass for it.
	requireNotReady := func(t *testing.T, state string, err, cause error) {
		t.Helper()
		var refusal *errs.ClassifiedError
		if !errors.As(err, &refusal) || refusal.Class != errs.ErrorTransient || refusal.Code != graph.ErrorCodeIndexNotReady {
			t.Errorf("%s: levelRecords error = %v, want transient index_not_ready", state, err)
		}
		if cause != nil && !errors.Is(err, cause) {
			t.Errorf("%s: levelRecords error = %v, want it to carry %v", state, err, cause)
		}
	}
	_, _, err := levelRecords(nil, "0.")
	requireNotReady(t, "no view", err, nil)
	if want := noCommunityView; err == nil || err.Error() != want {
		t.Errorf("no view: message = %v, want %q", err, want)
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
		_, _, err = levelRecords(view, "0.")
		requireNotReady(t, "catching up", err, graphview.ErrNotReady)

		cancel()
		<-ran // Run has returned, so the view has stopped.
		_, _, err = levelRecords(view, "0.")
		requireNotReady(t, "stopped", err, graphview.ErrViewStopped)
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
		// Its type is the term, so it matches whatever its facts.
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
		matches, _, err := rankEntities(rawEntities(tc.entity), []string{"pump"}, tc.order)
		if err != nil || len(matches) != 1 {
			t.Fatalf("%s: rankEntities = %v, %v; want one match", tc.name, matches, err)
		}
		if got := matches[0].label; got != tc.want {
			t.Errorf("%s: label = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestTextTierAnswer: the answer is a plain text built from the reply's fields, one sentence per
// condition, each a literal here (spec, "A cut answer says so").
func TestTextTierAnswer(t *testing.T) {
	match := graph.TextSearchMatch{ID: "acme.ops.src.docs.note.a", Score: 0.5, Entity: []byte(`{}`)}
	data := graph.TextSearchData{
		Level: 1,
		Groups: []graph.TextSearchGroup{
			{CommunityID: "acme.ops.src", MemberCount: 7, CommunitySummary: "Community of 7 entities including 7 notes."},
			{CommunityID: "acme.ops.dev", MemberCount: 3, Keywords: []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"}},
			{CommunityID: "acme.ops.lab", MemberCount: 2},
		},
		Matches:         []graph.TextSearchMatch{match, match},
		EntitiesScanned: 10,
	}
	const (
		found  = "Found 2 matching entities among 10 read from 3 groups at level 1."
		groups = " acme.ops.src: Community of 7 entities including 7 notes." +
			" acme.ops.dev: It has 3 members. Key themes: alpha, beta, gamma, delta, epsilon." +
			" acme.ops.lab: It has 2 members."
		sample     = " This is a sample: a group had more than one page, or matches were left out to fit."
		unhonoured = " Not honoured: path, ranking."
		unreadable = " 2 group records at this level could not be read."
	)
	if got, want := textSearchAnswer(data), found+groups; got != want {
		t.Errorf("answer =\n%q\nwant\n%q", got, want)
	}
	data.EntitiesTruncated = true
	data.UnhonouredHints = []string{"path", "ranking"}
	data.UnreadableGroups = 2
	if got, want := textSearchAnswer(data), found+sample+groups+unhonoured+unreadable; got != want {
		t.Errorf("answer with every condition =\n%q\nwant\n%q", got, want)
	}
	if got, want := textSearchAnswer(graph.TextSearchData{Level: 2, Groups: []graph.TextSearchGroup{}, Matches: []graph.TextSearchMatch{}, UnreadableGroups: 1}),
		"No group at level 2 matched the question. 1 group records at this level could not be read."; got != want {
		t.Errorf("answer with no group = %q, want %q", got, want)
	}
}

// TestTextTierFitKeepsTheBestThatFit: over max_payload the lowest-ranked matches are left out and
// entities_truncated set, as localSearch fits. Every limit from the reply with no match to the
// whole reply is checked against a scan written here: the most matches, first to last, whose
// encoding fits.
func TestTextTierFitKeepsTheBestThatFit(t *testing.T) {
	// Each entity is longer than the sample sentence a cut adds, so leaving one out shrinks the reply.
	pad := strings.Repeat("x", 100)
	entity := func(id string) json.RawMessage { return json.RawMessage(`{"id":"` + id + `","text":"` + pad + `"}`) }
	data := graph.TextSearchData{
		Groups:          []graph.TextSearchGroup{{CommunityID: "acme.ops.src", MemberCount: 3}},
		Matches:         []graph.TextSearchMatch{{ID: "a", Score: 1, Entity: entity("a")}, {ID: "b", Score: 0.5, Entity: entity("b")}, {ID: "c", Score: 0.5, Entity: entity("c")}},
		EntitiesScanned: 3,
		UnhonouredHints: []string{},
	}
	encode := func(d graph.TextSearchData) ([]byte, error) { return json.Marshal(d) }
	all, err := json.Marshal(func() graph.TextSearchData { d := data; d.Answer = textSearchAnswer(d); return d }())
	if err != nil {
		t.Fatal(err)
	}
	two := `{"level":0,"groups":[{"community_id":"acme.ops.src","member_count":3}],` +
		`"matches":[{"id":"a","score":1,"entity":{"id":"a","text":"` + pad + `"}},{"id":"b","score":0.5,"entity":{"id":"b","text":"` + pad + `"}}],` +
		`"entities_scanned":3,"entities_truncated":true,"unreadable_groups":0,"unhonoured_hints":[],` +
		`"answer":"Found 2 matching entities among 3 read from 1 groups at level 0. This is a sample: a group had more than one page, or matches were left out to fit. acme.ops.src: It has 3 members."}`
	got, truncated, err := fitTextSearch(data, len(all)-1, encode)
	if err != nil || string(got) != two || !truncated {
		t.Errorf("fit at %d = %s, %t, %v\nwant %s, true", len(all)-1, got, truncated, err, two)
	}

	// The scan: each count's reply, as the reply states it, from the whole list down.
	replyOf := func(kept int) []byte {
		cut := data
		cut.Matches = data.Matches[:kept]
		cut.EntitiesTruncated = kept < len(data.Matches)
		cut.Answer = textSearchAnswer(cut)
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
		got, truncated, err := fitTextSearch(data, limit, encode)
		if err != nil || string(got) != string(replyOf(kept)) || truncated != (kept < len(data.Matches)) {
			t.Errorf("fit at %d = %s, %t, %v\nwant %s", limit, got, truncated, err, replyOf(kept))
		}
	}

	data.EntitiesTruncated = true // a page had a continuation
	if got, truncated, err := fitTextSearch(data, 1<<20, encode); err != nil || !truncated || !strings.Contains(string(got), `"entities_truncated":true`) {
		t.Errorf("fit of a cut page = %s, %t, %v; want it still truncated", got, truncated, err)
	}
	if got, _, err := fitTextSearch(data, 10, encode); err == nil {
		t.Errorf("fit at 10 bytes = %s, want an error", got)
	}
}
