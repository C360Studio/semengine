//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/vocabulary"
)

// TestSearchGraphAnswersAsGlobalSearch: with embedding_enabled false, searchGraph decodes and
// answers every request as globalSearch does, with the same data under the same indexed_revision, and the same refusal,
// class, code and message (spec "searchGraph answers as global search"). One request per kind of
// answer global search has, over real ingest, index, word index and community records. No semantic search follows the
// empty text answer: a spy on the embedding verbs holds that none is made. A request global search
// refuses is refused unchanged.
func TestSearchGraphAnswersAsGlobalSearch(t *testing.T) {
	stack := startCommunityStackWith(t, `{"embedding_enabled": false}`)
	written := stack.write(t, drone1,
		[2]string{vocabulary.DCTermsTitle, "Alpha"},
		aliased("held-1"),
		linked(drone2))
	tick(t, stack.maintainer)
	stack.awaitIndexed(t, written)
	awaitCommunityView(t, stack.component, systemGroupKey, written)
	awaitWordIndexed(t, stack.client, written)

	var semantic atomic.Int64
	for _, name := range []string{"search", "similar"} {
		serve(t, stack.client, verbRow(t, graph.ResponderGraphEmbedding, name).Subject, func(context.Context, []byte) ([]byte, error) {
			semantic.Add(1)
			return nil, errs.ClassifiedCode(errs.ErrorInvalid, graph.ErrorCodeInvalidRequest, errors.New("no semantic request is expected"))
		})
	}

	// The answers that carry no timestamp, as literals, so equal replies are also the right ones: the
	// walk alone where the text depends on the word index's scores.
	literalPath := `{"entities":[{"entity_id":"` + drone1 + `","depth":0},{"entity_id":"` + drone2 + `","depth":1}],` +
		`"edges":[{"predicate":"` + linkTo + `","from_entity_id":"` + drone1 + `","to_entity_id":"` + drone2 + `"}],"truncated":false}`
	literalData := map[string]string{
		"text that matches nothing": `{"text":{"level":0,"groups":[],"matches":[],"matched":0,"missing":0,` +
			`"bodies_not_indexed":0,"truncated":false,"unreadable_groups":0,"answer":"Found 0 matching entities in 0 groups."},"hints":[]}`,
	}
	namedKey := map[string]string{"SemSource's summarize_threshold": "summarize_threshold"}
	// A time range's window is read from the clock when each call runs, so two calls a second apart
	// differ in it; the replies are compared without it, and each window is checked on its own.
	window := map[string]time.Duration{"a time range": 2 * time.Hour}
	globalSubject := verbRow(t, graph.ResponderGraphQuery, "globalSearch").Subject
	searchSubject := verbRow(t, graph.ResponderGraphQuery, "searchGraph").Subject
	for _, tc := range []struct {
		name, request string
		beside        string          // the verified answer beside the text: "entity", "path" or ""
		class         errs.ErrorClass // a refusal's class and code
		code          string
	}{
		{"an entity", `{"query":"held-1"}`, "entity", 0, ""},
		{"a path", `{"query":"what is connected to held-1"}`, "path", 0, ""},
		// At tier 0 no embedding search is merged into this answer; it is the answer.
		{"text that matches nothing", `{"query":"zzz qqq"}`, "", 0, ""},
		{"text that matches", `{"query":"alpha drone"}`, "", 0, ""},
		{"a named start naming no entity", `{"query":"what is connected to nobody-here"}`, "", 0, ""},
		{"a time range", `{"query":"what changed in the last 2 hours"}`, "", 0, ""},
		{"a body that is not JSON", `{bad json`, "", errs.ErrorInvalid, graph.ErrorCodeInvalidRequest},
		{"SemSource's summarize_threshold", `{"query":"alpha drone","summarize_threshold":1}`, "", errs.ErrorInvalid, graph.ErrorCodeInvalidRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			global, globalErr := stack.client.RequestClassified(t.Context(), globalSubject, []byte(tc.request), requestTimeout)
			search, searchErr := stack.client.RequestClassified(t.Context(), searchSubject, []byte(tc.request), requestTimeout)
			if tc.code != "" {
				want := requireRefusal(t, global, globalErr, tc.class, tc.code)
				got := requireRefusal(t, search, searchErr, tc.class, tc.code)
				if got.Error() != want.Error() {
					t.Errorf("searchGraph refused %q, globalSearch %q", got.Error(), want.Error())
				}
				if key, ok := namedKey[tc.name]; ok && !strings.Contains(want.Error(), key) {
					t.Errorf("globalSearch refused %q, which does not name %s", want.Error(), key)
				}
				return
			}
			if globalErr != nil || searchErr != nil {
				t.Fatalf("globalSearch: %v; searchGraph: %v; want both to answer", globalErr, searchErr)
			}
			want, answer := decodeGlobal(t, global)
			got := decodeEnvelope(t, search)
			beside := ""
			switch {
			case answer.Entity != nil:
				beside = "entity"
			case answer.Path != nil:
				beside = "path"
				if path, err := json.Marshal(answer.Path); err != nil || string(path) != literalPath {
					t.Errorf("globalSearch path = %s, want %s", path, literalPath)
				}
			}
			if beside != tc.beside {
				t.Fatalf("globalSearch data = %s, want %q beside the text", want.Data, tc.beside)
			}
			if literal, ok := literalData[tc.name]; ok && string(want.Data) != literal {
				t.Errorf("globalSearch data =\n  %s\nwant\n  %s", want.Data, literal)
			}
			if span, ok := window[tc.name]; ok {
				want.Data = withoutWindow(t, want.Data, span)
				got.Data = withoutWindow(t, got.Data, span)
			}
			if string(got.Data) != string(want.Data) {
				t.Errorf("searchGraph data =\n  %s\nglobalSearch data =\n  %s", got.Data, want.Data)
			}
			if got.IndexedRevision != want.IndexedRevision {
				t.Errorf("searchGraph indexed_revision = %d, globalSearch %d", got.IndexedRevision, want.IndexedRevision)
			}
		})
	}
	// Each reply above came after graph-query's handler returned, and a semantic request it sent would
	// have been answered, and counted, before that.
	if got := semantic.Load(); got != 0 {
		t.Errorf("the embedding verbs received %d requests, want none", got)
	}
}

// withoutWindow returns data with its one time_range hint's window removed, after checking the window
// is span long.
func withoutWindow(t *testing.T, data []byte, span time.Duration) []byte {
	t.Helper()
	var answer graph.GlobalSearchData
	if err := json.Unmarshal(data, &answer); err != nil {
		t.Fatalf("decode data %s: %v", data, err)
	}
	found := 0
	for i, hint := range answer.Hints {
		if hint.Hint != hintTimeRange {
			continue
		}
		found++
		from, to, ok := strings.Cut(hint.Detail, " to ")
		start, startErr := time.Parse(time.RFC3339, from)
		end, endErr := time.Parse(time.RFC3339, to)
		if !ok || startErr != nil || endErr != nil || end.Sub(start) != span {
			t.Errorf("time_range detail = %q, want a %v window", hint.Detail, span)
		}
		answer.Hints[i].Detail = ""
	}
	if found != 1 {
		t.Fatalf("data = %s, want one time_range hint", data)
	}
	rest, err := json.Marshal(answer)
	if err != nil {
		t.Fatalf("encode data: %v", err)
	}
	return rest
}
