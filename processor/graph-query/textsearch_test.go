package graphquery

import (
	"context"
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
// keyword and term where one contains the other, all lowercased, over the question's distinct terms;
// up to five records scoring above 0 are read, best first, ties by key (design D2; spec, "The text tier
// reads at most five matching groups, one page each"). It carries the step 5 pin probe's records (P1,
// P2, notes/probes/step5-pin-probe.log) with the pin's order corrected: the pin kept records scoring 0
// and its unstable sort left ties in input order.
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
			// The pin weighed a repeated term twice (inventory R6); the terms are distinct here.
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
		// a scores 3.5 for "pump" and b, c 1.5 each; all three 1.5 for "pumps". The pin's orders were
		// [a c b], [a c b] and [c b a], input order c, b, a.
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

// TestTextTierTermLimit: more than graph.MaxSearchTerms distinct terms is the caller's invalid_request,
// refused before the community view is read, so it wins over index_not_ready; at the limit the
// question reaches the view, which this component, never started, does not have (design D2, D3;
// spec, "The text tier refuses an index it cannot read").
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

// TestTextTierRefusesAViewCatchingUp: a community view that is running but has not finished its replay
// is index_not_ready, transient, as is no view and a view that has stopped (design D2; spec, "The text
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
