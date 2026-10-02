package main

import (
	"go/format"
	"go/parser"
	"go/token"
	"sort"
	"strings"
)

const (
	pinModule  = "github.com/c360studio/semstreams"
	treeModule = "github.com/c360studio/semengine"
)

// rewrite turns a pin .go file into what the tree should hold (harness-boundaries › "Comparison
// with the pin"): the module path is replaced in import paths and comments only, a moved package's
// path becomes its destination path, and the result is formatted with gofmt. moved maps a moved
// package's source_path to its destination path. A file gofmt cannot parse is returned as it
// stands.
func rewrite(src []byte, moved map[string]string) []byte {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return src
	}
	tf := fset.File(file.Pos())
	type span struct{ start, end int }
	var spans []span
	for _, imp := range file.Imports {
		spans = append(spans, span{tf.Offset(imp.Path.Pos()), tf.Offset(imp.Path.End())})
	}
	for _, group := range file.Comments {
		for _, c := range group.List {
			spans = append(spans, span{tf.Offset(c.Pos()), tf.Offset(c.End())})
		}
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	var b strings.Builder
	last := 0
	for _, s := range spans {
		b.Write(src[last:s.start])
		b.WriteString(rewriteText(string(src[s.start:s.end]), moved))
		last = s.end
	}
	b.Write(src[last:])
	formatted, err := format.Source([]byte(b.String()))
	if err != nil {
		return src
	}
	return formatted
}

// gofmt formats a tree .go file, or returns it as it stands when it is not a whole Go file:
// format.Source alone would also format a list of declarations, which rewrite leaves as it stands.
func gofmt(src []byte) []byte {
	if _, err := parser.ParseFile(token.NewFileSet(), "", src, parser.ParseComments); err != nil {
		return src
	}
	formatted, err := format.Source(src)
	if err != nil {
		return src
	}
	return formatted
}

// rewriteText replaces the module path in one import path or comment. The module path followed by
// a letter, digit, `_` or `-` is another module (github.com/c360studio/semstreams-ui) and is left
// alone.
func rewriteText(s string, moved map[string]string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, pinModule)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		rest := s[i+len(pinModule):]
		if rest != "" && isNameChar(rest[0]) {
			b.WriteString(s[:i+len(pinModule)])
			s = rest
			continue
		}
		b.WriteString(s[:i])
		b.WriteString(treeModule)
		if strings.HasPrefix(rest, "/") {
			n := 1
			for n < len(rest) && (isNameChar(rest[n]) || rest[n] == '/' || rest[n] == '.') {
				n++
			}
			pkg := strings.TrimRight(rest[1:n], "./")
			if dest, ok := moved[pkg]; ok && pkg != "" {
				b.WriteString("/" + dest)
				rest = rest[1+len(pkg):]
			}
		}
		s = rest
	}
}

func isNameChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}
