package graphgateway

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/validator"
)

//go:embed schema.graphql
var schemaSDL string

// schema is the gateway's contract: every answer is projected through it.
var schema = gqlparser.MustLoadSchema(&ast.Source{Name: "schema.graphql", Input: schemaSDL})

// request is a GraphQL request body.
type request struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName"`
	Variables     map[string]any `json:"variables"`
}

// response is a GraphQL response body. Data is nil, and absent from the body, only when the request was refused
// before execution; an executed request answers data even when it is empty (all root fields skipped).
type response struct {
	Data   map[string]any `json:"data,omitzero"`
	Errors gqlerror.List  `json:"errors,omitempty"`
}

// resolver answers one root field from its coerced arguments. It returns a value keyed by the schema's field names:
// map[string]any for an object, []any for a list, and for a scalar a string (String), int (Int), float64 (Float),
// bool (Boolean), uint64 (Uint64) or any JSON-encodable value (JSON). Keys the selection does not name are dropped.
type resolver func(ctx context.Context, args map[string]any) (any, error)

// execute answers one request. refused reports a request turned away before any resolver ran (the caller answers
// HTTP 400): it does not parse, fails validation against the schema, is not a query, asks for introspection or
// names more than one root field besides __typename. Otherwise each root field is resolved and projected by its
// selection; a resolver error or a reply not fitting the schema leaves that field null with an error at its path.
func execute(ctx context.Context, resolvers map[string]resolver, req request) (resp response, refused bool) {
	op, vars, refusal := prepare(req)
	if len(refusal) > 0 {
		return response{Errors: refusal}, true
	}
	resp.Data = map[string]any{}
	for _, c := range collect(op.SelectionSet, "Query", vars) {
		path := ast.Path{ast.PathName(c.key)}
		if c.name == "__typename" {
			resp.Data[c.key] = "Query"
			continue
		}
		resp.Data[c.key] = nil
		resolve, ok := resolvers[c.name]
		if !ok {
			resp.Errors = append(resp.Errors, gqlerror.ErrorPathf(path, "%s has no resolver", c.name))
			continue
		}
		reply, err := resolve(ctx, c.fields[0].ArgumentMap(vars))
		if err != nil {
			resp.Errors = append(resp.Errors, &gqlerror.Error{Err: err, Message: err.Error(), Path: path})
			continue
		}
		value, misfit := complete(c.fields[0].Definition.Type, c.fields, reply, path, vars)
		if misfit != nil {
			resp.Errors = append(resp.Errors, misfit)
			continue
		}
		resp.Data[c.key] = value
	}
	return resp, false
}

// prepare parses and validates the request against the schema and coerces its variables, refusing an
// unnamed pick among several operations, any operation but a query, introspection and a second root field.
func prepare(req request) (*ast.OperationDefinition, map[string]any, gqlerror.List) {
	doc, list := gqlparser.LoadQueryWithRules(schema, req.Query, nil)
	if len(list) > 0 {
		return nil, nil, list
	}
	op := doc.Operations.ForName(req.OperationName)
	if op == nil && req.OperationName == "" {
		return nil, nil, refuse("the document holds %d operations; name one with operationName", len(doc.Operations))
	}
	if op == nil {
		return nil, nil, refuse("the document holds no operation %q", req.OperationName)
	}
	if op.Operation != ast.Query {
		return nil, nil, refuse("%s is not served: the gateway is read-only", op.Operation)
	}
	vars, err := validator.VariableValues(schema, op, req.Variables)
	if err != nil {
		return nil, nil, gqlerror.List{gqlerror.WrapIfUnwrapped(err)}
	}
	var fields []string
	for _, c := range collect(op.SelectionSet, "Query", vars) {
		switch {
		case c.name == "__typename":
		case strings.HasPrefix(c.name, "__"):
			return nil, nil, refuse("introspection is not served: %s", c.name)
		case c.key == c.name:
			fields = append(fields, c.name)
		default:
			fields = append(fields, c.key+": "+c.name)
		}
	}
	if len(fields) > 1 {
		return nil, nil, refuse("one root field per request besides __typename; got %s",
			strings.Join(fields, ", "))
	}
	return op, vars, nil
}

func refuse(format string, args ...any) gqlerror.List {
	return gqlerror.List{gqlerror.Errorf(format, args...)}
}

// collected is one response key and the fields that answer it: one field written more than once merges.
type collected struct {
	key, name string
	fields    []*ast.Field
}

// collect gathers the fields a selection set asks of typeName, through fragments, honouring @skip and @include, in
// the order they are first named. The schema has object types only, so a fragment applies when its type
// condition is absent or names typeName.
func collect(set ast.SelectionSet, typeName string, vars map[string]any) []collected {
	var out []collected
	var walk func(ast.SelectionSet)
	walk = func(set ast.SelectionSet) {
		for _, s := range set {
			switch s := s.(type) {
			case *ast.Field:
				if !included(s.Directives, vars) {
					continue
				}
				i := len(out)
				for j := range out {
					if out[j].key == s.Alias {
						i = j
					}
				}
				if i == len(out) {
					out = append(out, collected{key: s.Alias, name: s.Name})
				}
				out[i].fields = append(out[i].fields, s)
			case *ast.InlineFragment:
				if included(s.Directives, vars) && (s.TypeCondition == "" || s.TypeCondition == typeName) {
					walk(s.SelectionSet)
				}
			case *ast.FragmentSpread:
				if included(s.Directives, vars) && s.Definition.TypeCondition == typeName {
					walk(s.Definition.SelectionSet)
				}
			}
		}
	}
	walk(set)
	return out
}

func included(directives ast.DirectiveList, vars map[string]any) bool {
	if d := directives.ForName("skip"); d != nil && d.ArgumentMap(vars)["if"] == true {
		return false
	}
	if d := directives.ForName("include"); d != nil && d.ArgumentMap(vars)["if"] != true {
		return false
	}
	return true
}

// complete projects a resolved value onto its schema type and the selections of fields: exactly the selected names,
// under their response keys. A value that does not fit the type is an error naming its path.
func complete(typ *ast.Type, fields []*ast.Field, v any, path ast.Path, vars map[string]any) (any, *gqlerror.Error) {
	if v == nil {
		if typ.NonNull {
			return nil, misfit(path, "is null, but %s is non-null", typ)
		}
		return nil, nil
	}
	if typ.Elem != nil {
		items, ok := v.([]any)
		if !ok {
			return nil, misfit(path, "is %T, but %s is a list", v, typ)
		}
		out := make([]any, len(items))
		for i, item := range items {
			value, err := complete(typ.Elem, fields, item, append(path[:len(path):len(path)], ast.PathIndex(i)), vars)
			if err != nil {
				return nil, err
			}
			out[i] = value
		}
		return out, nil
	}
	def := schema.Types[typ.NamedType]
	if def.Kind == ast.Scalar {
		if !fitsScalar(def.Name, v) {
			return nil, misfit(path, "is %T, which is not a %s", v, def.Name)
		}
		return v, nil
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, misfit(path, "is %T, but %s is an object", v, def.Name)
	}
	var below ast.SelectionSet
	for _, f := range fields {
		below = append(below, f.SelectionSet...)
	}
	out := map[string]any{}
	for _, c := range collect(below, def.Name, vars) {
		if c.name == "__typename" {
			out[c.key] = def.Name
			continue
		}
		at := append(path[:len(path):len(path)], ast.PathName(c.key))
		value, err := complete(def.Fields.ForName(c.name).Type, c.fields, obj[c.name], at, vars)
		if err != nil {
			return nil, err
		}
		out[c.key] = value
	}
	return out, nil
}

func fitsScalar(name string, v any) bool {
	switch name {
	case "String":
		_, ok := v.(string)
		return ok
	case "Int":
		n, ok := v.(int)
		return ok && n >= math.MinInt32 && n <= math.MaxInt32
	case "Float":
		f, ok := v.(float64)
		return ok && !math.IsNaN(f) && !math.IsInf(f, 0)
	case "Boolean":
		_, ok := v.(bool)
		return ok
	case "Uint64":
		_, ok := v.(uint64)
		return ok
	case "JSON":
		_, err := json.Marshal(v)
		return err == nil
	}
	return false
}

func misfit(path ast.Path, format string, args ...any) *gqlerror.Error {
	return gqlerror.ErrorPathf(path, "reply does not fit the schema at %s: %s", path, fmt.Sprintf(format, args...))
}
