package contract

import "testing"

// lexicalTransportRule is the requirement every violation names.
const lexicalTransportRule = "(lexical-search › The package imports no NATS client)"

// TestLexicalImportsNoNATS holds the rule that the word index is plain in-memory code: an
// embedder that searches text must not pull in a NATS client to get it.
//
// Requirement: lexical-search/The package imports no NATS client
func TestLexicalImportsNoNATS(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "graph/lexical transport imports", transportViolations(t, root, "./graph/lexical", lexicalTransportRule))
}

// Requirement: lexical-search/The package imports no NATS client
func TestLexicalImportsNoNATSSensitivity(t *testing.T) {
	const mod = "go.mod"
	clean := map[string]string{
		mod:                  "module example.com/fixture\n\ngo 1.26\n",
		"graph/lexical/l.go": "package lexical\n\nimport _ \"example.com/fixture/graph\"\n",
		"graph/g.go":         "// Package graph mentions natsclient in a comment only.\npackage graph\n",
		"natsclient/n.go":    "package natsclient\n",
	}
	root, _ := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", transportViolations(t, root, "./graph/lexical", lexicalTransportRule))

	// The package reaches natsclient through a package it imports: the transitive edge is the case.
	reaching := map[string]string{
		mod:                  "module example.com/fixture\n\ngo 1.26\n",
		"graph/lexical/l.go": "package lexical\n\nimport _ \"example.com/fixture/graph\"\n",
		"graph/g.go":         "package graph\n\nimport _ \"example.com/fixture/natsclient\"\n",
		"natsclient/n.go":    "package natsclient\n",
	}
	root, _ = writeTree(t, reaching)
	requireViolation(t, transportViolations(t, root, "./graph/lexical", lexicalTransportRule), "example.com/fixture/natsclient")
}
