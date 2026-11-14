//go:build integration

package graphquery

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/c360studio/semengine/vocabulary"
)

// TestSearchGraphAnswersAsGlobalSearch: searchGraph decodes and answers every request as
// globalSearch does, with the same data under the same indexed_revision, and the same refusal,
// class, code and message (spec "searchGraph answers as global search"). One request per outcome
// global search has, over real ingest, index, word index and community records. No semantic search follows the
// empty text answer: a spy on the embedding verbs holds that none is made. A request global search
// refuses is refused unchanged.
func TestSearchGraphAnswersAsGlobalSearch(t *testing.T) {
	stack := startCommunityStack(t)
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

	// The answers that carry no timestamp, as literals, so equal replies are also the right ones.
	literalData := map[string]string{
		"a path": `{"strategy":"path","path":{"entities":[{"entity_id":"` + drone1 + `","depth":0},{"entity_id":"` + drone2 + `","depth":1}],` +
			`"edges":[{"predicate":"` + linkTo + `","from_entity_id":"` + drone1 + `","to_entity_id":"` + drone2 + `"}],"truncated":false}}`,
		"text that matches nothing": `{"strategy":"text","text":{"level":0,"groups":[],"matches":[],"matched":0,"missing":0,` +
			`"bodies_not_indexed":0,"truncated":false,"unreadable_groups":0,"unhonoured_hints":[],"answer":"Found 0 matching entities in 0 groups."}}`,
	}
	namedKey := map[string]string{"SemSource's summarize_threshold": "summarize_threshold"}
	globalSubject := verbRow(t, graph.ResponderGraphQuery, "globalSearch").Subject
	searchSubject := verbRow(t, graph.ResponderGraphQuery, "searchGraph").Subject
	for _, tc := range []struct {
		name, request string
		strategy      string          // the answer's strategy, or "" for a refusal
		class         errs.ErrorClass // the refusal's class and code
		code          string
	}{
		{"an entity", `{"query":"held-1"}`, "entity", 0, ""},
		{"a path", `{"query":"what is connected to held-1"}`, "path", 0, ""},
		// No semantic fallback follows this answer; it is the answer.
		{"text that matches nothing", `{"query":"zzz qqq"}`, "text", 0, ""},
		{"text that matches", `{"query":"alpha drone"}`, "text", 0, ""},
		{"a body that is not JSON", `{bad json`, "", errs.ErrorInvalid, graph.ErrorCodeInvalidRequest},
		{"SemSource's summarize_threshold", `{"query":"alpha drone","summarize_threshold":1}`, "", errs.ErrorInvalid, graph.ErrorCodeInvalidRequest},
		{"a named start naming no entity", `{"query":"what is connected to nobody-here"}`, "", errs.ErrorInvalid, graph.ErrorCodeEntityNotFound},
		{"a time range with no responder", `{"query":"what changed in the last 2 hours"}`, "", errs.ErrorTransient, graph.ErrorCodeInternal},
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
			want, got := decodeEnvelope(t, global), decodeEnvelope(t, search)
			var answer struct {
				Strategy string `json:"strategy"`
			}
			if err := json.Unmarshal(want.Data, &answer); err != nil || answer.Strategy != tc.strategy {
				t.Fatalf("globalSearch data = %s (%v), want strategy %q", want.Data, err, tc.strategy)
			}
			if literal, ok := literalData[tc.name]; ok && string(want.Data) != literal {
				t.Errorf("globalSearch data =\n  %s\nwant\n  %s", want.Data, literal)
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
