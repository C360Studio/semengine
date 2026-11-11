package graphgateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// recorder answers every root field with a fixed value and records each field it was asked for, so a test can show
// that a refused request reached no resolver.
type recorder struct {
	replies map[string]any
	errs    map[string]error
	called  []string
	args    []map[string]any
}

func (r *recorder) resolvers() map[string]resolver {
	out := map[string]resolver{}
	for _, name := range []string{"entity", "entitiesByPrefix", "entitiesByPredicate", "pathSearch"} {
		out[name] = func(_ context.Context, args map[string]any) (any, error) {
			r.called = append(r.called, name)
			r.args = append(r.args, args)
			if err := r.errs[name]; err != nil {
				return nil, err
			}
			return r.replies[name], nil
		}
	}
	return out
}

func exactEntityReply() map[string]any {
	return map[string]any{
		"kvRevision":  uint64(7),
		"kv_revision": uint64(7), // a wire name the schema does not declare: never answered
		"entity": map[string]any{
			"id": "c360.platform.graph.test.entity.a",
			"triples": []any{map[string]any{
				"subject":   "c360.platform.graph.test.entity.a",
				"predicate": "test.rel.links",
				"object":    "c360.platform.graph.test.entity.b",
			}},
		},
	}
}

func run(t *testing.T, r *recorder, query string, vars map[string]any) (response, bool) {
	t.Helper()
	return runRequest(t, r, request{Query: query, Variables: vars})
}

func runRequest(t *testing.T, r *recorder, req request) (response, bool) {
	t.Helper()
	resp, refused := execute(t.Context(), r.resolvers(), req)
	if _, err := json.Marshal(resp); err != nil {
		t.Fatalf("response does not encode: %v", err)
	}
	return resp, refused
}

func errorsText(t *testing.T, resp response) string {
	t.Helper()
	b, err := json.Marshal(resp.Errors)
	if err != nil {
		t.Fatalf("errors do not encode: %v", err)
	}
	return string(b)
}

// assertRefused checks a request turned away before execution: refused, no data, an error naming want, no resolver.
func assertRefused(t *testing.T, r *recorder, req request, want string) {
	t.Helper()
	resp, refused := runRequest(t, r, req)
	if !refused {
		t.Fatalf("not refused: %+v", resp)
	}
	if resp.Data != nil {
		t.Fatalf("refused request answered data %v", resp.Data)
	}
	if len(resp.Errors) == 0 {
		t.Fatal("refused request carries no error")
	}
	if text := errorsText(t, resp); !strings.Contains(text, want) {
		t.Fatalf("errors %s do not name %q", text, want)
	}
	if len(r.called) != 0 {
		t.Fatalf("refused request reached resolvers %v", r.called)
	}
}

func TestSchemaRefusesUnknownFieldNamingIt(t *testing.T) {
	cases := []struct {
		name, query, want, operation string
		vars                         map[string]any
	}{
		{name: "unknown field", want: "edges",
			query: `{ pathSearch(startEntity: "a", direction: "outgoing", maxDepth: 1) { edges { from } } }`},
		// Owner ruling 2026-11-10 (#567): trajectory is not served by this gateway.
		{name: "trajectory", want: "trajectory", query: `{ trajectory(loopId: "x") { steps } }`},
		{name: "unknown argument", want: "revision", query: `{ entity(id: "x", revision: 2) { kvRevision } }`},
		{name: "missing argument", want: "id", query: `{ entity { kvRevision } }`},
		{name: "variable of the wrong type in the document", want: "$ident",
			query: `query($ident: Int!) { entity(id: $ident) { kvRevision } }`},
		{name: "variable value of the wrong type", want: "pageLimit",
			query: `query($pageLimit: Int) { entitiesByPrefix(prefix: "", limit: $pageLimit) { total } }`,
			vars:  map[string]any{"pageLimit": "ten"}},
		{name: "does not parse", want: "Expected", query: `{ entity(id: "x" { kvRevision } }`},
		{name: "unknown operation name", want: "missing", operation: "missing",
			query: `query a { __typename } query b { __typename }`},
		{name: "two operations, none named", want: "operation",
			query: `query a { __typename } query b { __typename }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertRefused(t, &recorder{},
				request{Query: tc.query, OperationName: tc.operation, Variables: tc.vars}, tc.want)
		})
	}
}

func TestMutationAndIntrospectionRefused(t *testing.T) {
	cases := []struct{ name, query, want string }{
		{"mutation", `mutation { entity(id: "x") { kvRevision } }`, "mutation"},
		{"subscription", `subscription { entity(id: "x") { kvRevision } }`, "subscription"},
		{"__schema", `{ __schema { types { name } } }`, "__schema"},
		{"__type", `{ __type(name: "Query") { name } }`, "__type"},
		{"__schema through a fragment", `{ ...Intro } fragment Intro on Query { __schema { queryType { name } } }`,
			"__schema"},
		{"__type beside a read", `{ __type(name: "Entity") { name } entity(id: "x") { kvRevision } }`, "__type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertRefused(t, &recorder{}, request{Query: tc.query}, tc.want)
		})
	}
}

func TestOneRootFieldPerRequest(t *testing.T) {
	refusedCases := []struct{ name, query string }{
		{"two fields", `{ entity(id: "x") { kvRevision } entitiesByPredicate(predicate: "a.b.c") { entities } }`},
		{"one field twice under aliases", `{ a: entity(id: "x") { kvRevision } b: entity(id: "y") { kvRevision } }`},
		{"second field in a fragment",
			`{ entity(id: "x") { kvRevision } ...More } fragment More on Query { entitiesByPrefix(prefix: "") { total } }`},
	}
	for _, tc := range refusedCases {
		t.Run(tc.name, func(t *testing.T) {
			assertRefused(t, &recorder{}, request{Query: tc.query}, "one root field")
		})
	}

	t.Run("__typename beside one field", func(t *testing.T) {
		r := &recorder{replies: map[string]any{"entity": exactEntityReply()}}
		resp, refused := run(t, r, `{ __typename entity(id: "x") { kvRevision } }`, nil)
		want := map[string]any{"__typename": "Query", "entity": map[string]any{"kvRevision": uint64(7)}}
		if refused || len(resp.Errors) != 0 || !reflect.DeepEqual(resp.Data, want) {
			t.Fatalf("refused=%v data=%v errors=%v, want %v", refused, resp.Data, resp.Errors, want)
		}
	})

	t.Run("one field named twice merges", func(t *testing.T) {
		r := &recorder{replies: map[string]any{"entity": exactEntityReply()}}
		resp, refused := run(t, r, `{ entity(id: "x") { kvRevision } entity(id: "x") { entity { id } } }`, nil)
		want := map[string]any{"entity": map[string]any{
			"kvRevision": uint64(7), "entity": map[string]any{"id": "c360.platform.graph.test.entity.a"}}}
		if refused || len(resp.Errors) != 0 || !reflect.DeepEqual(resp.Data, want) {
			t.Fatalf("refused=%v data=%v errors=%v, want %v", refused, resp.Data, resp.Errors, want)
		}
		if !reflect.DeepEqual(r.called, []string{"entity"}) {
			t.Fatalf("resolvers called %v, want entity once", r.called)
		}
		if !reflect.DeepEqual(r.args, []map[string]any{{"id": "x"}}) {
			t.Fatalf("resolver arguments %v", r.args)
		}
	})
}

func TestTypenameAnswers(t *testing.T) {
	r := &recorder{replies: map[string]any{"entity": exactEntityReply()}}
	resp, refused := run(t, r, `{ __typename }`, nil)
	if refused || !reflect.DeepEqual(resp.Data, map[string]any{"__typename": "Query"}) {
		t.Fatalf("refused=%v data=%v errors=%v", refused, resp.Data, resp.Errors)
	}
	if len(r.called) != 0 {
		t.Fatalf("__typename alone reached resolvers %v", r.called)
	}

	resp, refused = run(t, r, `{ entity(id: "x") { t: __typename entity { __typename triples { __typename } } } }`,
		nil)
	want := map[string]any{"entity": map[string]any{
		"t": "ExactEntity",
		"entity": map[string]any{
			"__typename": "Entity",
			"triples":    []any{map[string]any{"__typename": "Triple"}},
		},
	}}
	if refused || len(resp.Errors) != 0 || !reflect.DeepEqual(resp.Data, want) {
		t.Fatalf("refused=%v data=%v errors=%v, want %v", refused, resp.Data, resp.Errors, want)
	}
}

func TestReplyNotFittingSchemaNamesPath(t *testing.T) {
	withoutRevision := exactEntityReply()
	delete(withoutRevision, "kvRevision")
	nullSubject := exactEntityReply()
	nullSubject["entity"].(map[string]any)["triples"].([]any)[0].(map[string]any)["subject"] = nil
	wrongScalar := exactEntityReply()
	wrongScalar["kvRevision"] = "7"
	cases := []struct {
		name  string
		reply any
		err   error
		path  []any
	}{
		{name: "non-null field missing", reply: withoutRevision, path: []any{"entity", "kvRevision"}},
		{name: "null inside a list", reply: nullSubject, path: []any{"entity", "entity", "triples", 0.0, "subject"}},
		{name: "scalar of the wrong type", reply: wrongScalar, path: []any{"entity", "kvRevision"}},
		{name: "object where a list is declared", path: []any{"entity", "entity", "triples"},
			reply: map[string]any{"kvRevision": uint64(1),
				"entity": map[string]any{"id": "a", "triples": map[string]any{}}}},
		{name: "resolver error", err: errors.New("refused by the verb"), path: []any{"entity"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &recorder{replies: map[string]any{"entity": tc.reply}, errs: map[string]error{"entity": tc.err}}
			resp, refused := run(t, r,
				`{ __typename entity(id: "x") { kvRevision entity { id triples { subject } } } }`, nil)
			if refused {
				t.Fatalf("refused: %v", resp.Errors)
			}
			// The root field is null, with no partial value, and the rest of data stands.
			want := map[string]any{"__typename": "Query", "entity": nil}
			if !reflect.DeepEqual(resp.Data, want) {
				t.Fatalf("data = %v, want %v", resp.Data, want)
			}
			var wire struct {
				Errors []struct {
					Message string `json:"message"`
					Path    []any  `json:"path"`
				} `json:"errors"`
			}
			b, err := json.Marshal(resp)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(b, &wire); err != nil {
				t.Fatal(err)
			}
			if len(wire.Errors) != 1 || !reflect.DeepEqual(wire.Errors[0].Path, tc.path) {
				t.Fatalf("errors = %s, want one error at path %v", b, tc.path)
			}
		})
	}
}

// TestExecutedQueryKeepsData checks the encoded body: an executed query answers a data entry even when every root
// field is skipped, and a request refused before execution answers none (#591).
func TestExecutedQueryKeepsData(t *testing.T) {
	cases := []struct {
		name, query, want string
		refused           bool
	}{
		{name: "only root skipped", query: `{ entity(id: "x") @skip(if: true) { kvRevision } }`, want: `{"data":{}}`},
		{name: "only root not included", query: `{ entity(id: "x") @include(if: false) { kvRevision } }`,
			want: `{"data":{}}`},
		{name: "root included", query: `{ entity(id: "x") @include(if: true) { kvRevision } }`,
			want: `{"data":{"entity":{"kvRevision":7}}}`},
		{name: "refused before execution", query: `mutation { entity(id: "x") { kvRevision } }`, refused: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &recorder{replies: map[string]any{"entity": exactEntityReply()}}
			resp, refused := run(t, r, tc.query, nil)
			if refused != tc.refused {
				t.Fatalf("refused = %v, want %v: %v", refused, tc.refused, resp.Errors)
			}
			w := httptest.NewRecorder()
			writeResponse(w, http.StatusOK, resp)
			body := strings.TrimSpace(w.Body.String())
			if tc.refused {
				var keys map[string]json.RawMessage
				if err := json.Unmarshal([]byte(body), &keys); err != nil {
					t.Fatalf("body %s: %v", body, err)
				}
				if _, ok := keys["data"]; ok {
					t.Fatalf("refused request encoded a data entry: %s", body)
				}
				if _, ok := keys["errors"]; !ok {
					t.Fatalf("refused request encoded no errors: %s", body)
				}
				return
			}
			if body != tc.want {
				t.Fatalf("encoded %s, want %s", body, tc.want)
			}
		})
	}
}
