package graphgateway

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"pgregory.net/rapid"
)

// selected is the test's own model of a selection: the response key a client asked for, the schema field it names
// (or __typename), and what it selected below. The expected answer is read from this model and the reply, never from
// the executor (graph-gateway spec, "An answer holds exactly the selected fields").
type selected struct {
	key, field string
	below      []selected
}

type failer interface {
	Helper()
	Fatalf(format string, args ...any)
}

// checkProjection runs query against a resolver answering reply for root and checks that data holds exactly the
// one root key, and below it exactly the selected names, each carrying the reply's value.
func checkProjection(ctx context.Context, tb failer, query, root string, sel selected, reply any) {
	tb.Helper()
	resolvers := map[string]resolver{root: func(context.Context, map[string]any) (any, error) { return reply, nil }}
	resp, refused := execute(ctx, resolvers, request{Query: query})
	if refused || len(resp.Errors) != 0 {
		tb.Fatalf("query %s: refused=%v errors=%v", query, refused, resp.Errors)
	}
	if len(resp.Data) != 1 {
		tb.Fatalf("query %s: data keys %v, want only %q", query, keysOf(resp.Data), sel.key)
	}
	got, ok := resp.Data[sel.key]
	if !ok {
		tb.Fatalf("query %s: data keys %v, want %q", query, keysOf(resp.Data), sel.key)
	}
	checkValue(tb, schema.Query.Fields.ForName(root).Type, sel.below, reply, got, sel.key)
}

func checkValue(tb failer, typ *ast.Type, below []selected, reply, got any, path string) {
	tb.Helper()
	if reply == nil {
		if got != nil {
			tb.Fatalf("%s: answered %v for a null reply", path, got)
		}
		return
	}
	if typ.Elem != nil {
		items, gotItems := reply.([]any), asList(got)
		if len(items) != len(gotItems) {
			tb.Fatalf("%s: answered %d items, reply has %d", path, len(gotItems), len(items))
		}
		for i := range items {
			checkValue(tb, typ.Elem, below, items[i], gotItems[i], fmt.Sprintf("%s[%d]", path, i))
		}
		return
	}
	def := schema.Types[typ.NamedType]
	if def.Kind == ast.Scalar {
		if !reflect.DeepEqual(reply, got) {
			tb.Fatalf("%s: answered %#v, reply has %#v", path, got, reply)
		}
		return
	}
	obj, gotObj := reply.(map[string]any), asObject(got)
	want := make([]string, 0, len(below))
	for _, s := range below {
		want = append(want, s.key)
	}
	slices.Sort(want)
	if gotKeys := keysOf(gotObj); !slices.Equal(gotKeys, want) {
		tb.Fatalf("%s: answered keys %v, selected %v", path, gotKeys, want)
	}
	for _, s := range below {
		if s.field == "__typename" {
			if gotObj[s.key] != def.Name {
				tb.Fatalf("%s.%s: answered %v, want %s", path, s.key, gotObj[s.key], def.Name)
			}
			continue
		}
		checkValue(tb, def.Fields.ForName(s.field).Type, s.below, obj[s.field], gotObj[s.key], path+"."+s.key)
	}
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func asObject(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// rootCalls are the four root fields with arguments that pass validation.
var rootCalls = map[string]string{
	"entity":              `entity(id: "x")`,
	"entitiesByPrefix":    `entitiesByPrefix(prefix: "")`,
	"entitiesByPredicate": `entitiesByPredicate(predicate: "a.b.c")`,
	"pathSearch":          `pathSearch(startEntity: "a", direction: "outgoing", maxDepth: 1)`,
}

// drawSelection draws a non-empty selection on an object type: each field kept or not, aliased or not, written
// directly, in an inline fragment (with or without a type condition) or in a named fragment, plus __typename.
// Keys are unique at each level, so no two selections merge.
func drawSelection(t *rapid.T, typeName string, fragments *[]string) (string, []selected) {
	def := schema.Types[typeName]
	var parts []string
	var sels []selected
	names := []string{"__typename"}
	for _, f := range def.Fields {
		if !strings.HasPrefix(f.Name, "__") {
			names = append(names, f.Name)
		}
	}
	for i, name := range names {
		if !rapid.Bool().Draw(t, "keep "+typeName+"."+name) && !(i == len(names)-1 && len(sels) == 0) {
			continue
		}
		s := selected{key: name, field: name}
		text := name
		if rapid.Bool().Draw(t, "alias "+typeName+"."+name) {
			s.key = "k_" + name
			text = s.key + ": " + name
		}
		if name != "__typename" {
			if inner := def.Fields.ForName(name).Type; schema.Types[namedOf(inner)].Kind == ast.Object {
				var below string
				below, s.below = drawSelection(t, namedOf(inner), fragments)
				text += " { " + below + " }"
			}
		}
		switch rapid.IntRange(0, 3).Draw(t, "wrap "+typeName+"."+name) {
		case 1:
			text = "... on " + typeName + " { " + text + " }"
		case 2:
			text = "... { " + text + " }"
		case 3:
			frag := fmt.Sprintf("F%d", len(*fragments))
			*fragments = append(*fragments, "fragment "+frag+" on "+typeName+" { "+text+" }")
			text = "..." + frag
		}
		parts = append(parts, text)
		sels = append(sels, s)
	}
	return strings.Join(parts, " "), sels
}

func namedOf(t *ast.Type) string {
	for t.Elem != nil {
		t = t.Elem
	}
	return t.NamedType
}

// drawReply draws a reply that fits the schema: nullable fields present, null or absent; lists of up to three
// items; and in every object a key the schema does not declare, which the answer must never hold.
func drawReply(t *rapid.T, typ *ast.Type, label string) any {
	if !typ.NonNull && rapid.IntRange(0, 3).Draw(t, label+" null") == 0 {
		return nil
	}
	if typ.Elem != nil {
		items := make([]any, rapid.IntRange(0, 3).Draw(t, label+" len"))
		for i := range items {
			items[i] = drawReply(t, typ.Elem, fmt.Sprintf("%s[%d]", label, i))
		}
		return items
	}
	switch typ.NamedType {
	case "String":
		return rapid.String().Draw(t, label)
	case "Int":
		return rapid.IntRange(math.MinInt32, math.MaxInt32).Draw(t, label)
	case "Float":
		return rapid.Float64Range(-1e12, 1e12).Draw(t, label)
	case "Boolean":
		return rapid.Bool().Draw(t, label)
	case "Uint64":
		return rapid.Uint64().Draw(t, label)
	case "JSON":
		return rapid.SampledFrom([]any{"text", 1.5, true, map[string]any{"k": "v"}, []any{"a"}}).Draw(t, label)
	}
	def := schema.Types[typ.NamedType]
	obj := map[string]any{"undeclared": "never answered", "kv_revision": uint64(1)}
	for _, f := range def.Fields {
		if strings.HasPrefix(f.Name, "__") {
			continue
		}
		if !f.Type.NonNull && rapid.Bool().Draw(t, label+"."+f.Name+" absent") {
			continue
		}
		obj[f.Name] = drawReply(t, f.Type, label+"."+f.Name)
	}
	return obj
}

func TestProjectionHoldsExactlySelectedNames(t *testing.T) {
	// Named witnesses, run through the same checks before the generated run: the spec scenario (a narrow
	// selection answers one key), and one selection using an alias, both fragment kinds, __typename and a list of
	// lists, against a reply holding keys never selected.
	checkProjection(t.Context(), t, `{ entity(id: "x") { kvRevision } }`, "entity",
		selected{key: "entity", below: []selected{{key: "kvRevision", field: "kvRevision"}}}, exactEntityReply())
	pathReply := map[string]any{
		"entities":  []any{map[string]any{"id": "a", "depth": 0}, map[string]any{"id": "b", "depth": 1, "x": 1}},
		"paths":     []any{[]any{}, []any{map[string]any{"from": "b", "predicate": "p.q.r", "to": "a", "y": 2}}},
		"truncated": false,
		"edges":     []any{"never answered"},
	}
	checkProjection(t.Context(), t,
		`{ p: pathSearch(startEntity: "a", direction: "outgoing", maxDepth: 1) {
			__typename ... on PathSearchResult { e: entities { id } } ...Paths } }
		fragment Paths on PathSearchResult { paths { ... { to } from } }`,
		"pathSearch",
		selected{key: "p", below: []selected{
			{key: "__typename", field: "__typename"},
			{key: "e", field: "entities", below: []selected{{key: "id", field: "id"}}},
			{key: "paths", field: "paths", below: []selected{{key: "to", field: "to"}, {key: "from", field: "from"}}},
		}},
		pathReply)

	ctx := t.Context()
	rapid.Check(t, func(t *rapid.T) {
		root := rapid.SampledFrom([]string{"entity", "entitiesByPrefix", "entitiesByPredicate", "pathSearch"}).
			Draw(t, "root")
		var fragments []string
		rootType := schema.Query.Fields.ForName(root).Type
		below, sels := drawSelection(t, rootType.NamedType, &fragments)
		sel := selected{key: root, field: root, below: sels}
		call := rootCalls[root]
		if rapid.Bool().Draw(t, "alias root") {
			sel.key = "r"
			call = "r: " + call
		}
		query := "{ " + call + " { " + below + " } } " + strings.Join(fragments, " ")
		checkProjection(ctx, t, query, root, sel, drawReply(t, rootType, root))
	})
}
