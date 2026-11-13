package contract

import "testing"

// entityTextTransportRule is the requirement every violation names.
const entityTextTransportRule = "(entity-text › A body read's failures are told apart)"

// TestEntityTextImportsNoNATS holds the rule that the text rule is plain code over the graph's
// types and the storage interfaces: a component that reads an entity's text must not pull in a
// NATS client to get it.
//
// Requirement: entity-text/A body read's failures are told apart
func TestEntityTextImportsNoNATS(t *testing.T) {
	root := repoRoot(t)
	requireNoViolations(t, "graph/entitytext transport imports",
		transportViolations(t, root, "./graph/entitytext", entityTextTransportRule))
}

// Requirement: entity-text/A body read's failures are told apart
func TestEntityTextImportsNoNATSSensitivity(t *testing.T) {
	const mod = "go.mod"
	clean := map[string]string{
		mod:                     "module example.com/fixture\n\ngo 1.26\n",
		"graph/entitytext/e.go": "package entitytext\n\nimport _ \"example.com/fixture/storage\"\n",
		"storage/s.go":          "// Package storage mentions natsclient in a comment only.\npackage storage\n",
		"natsclient/n.go":       "package natsclient\n",
	}
	root, _ := writeTree(t, clean)
	requireNoViolations(t, "clean fixture", transportViolations(t, root, "./graph/entitytext", entityTextTransportRule))

	// The package reaches a NATS client through a package it imports: the transitive edge is the case.
	reaching := map[string]string{
		mod:                     "module example.com/fixture\n\ngo 1.26\n",
		"graph/entitytext/e.go": "package entitytext\n\nimport _ \"example.com/fixture/storage\"\n",
		"storage/s.go":          "package storage\n\nimport _ \"example.com/fixture/natsclient\"\n",
		"natsclient/n.go":       "package natsclient\n",
	}
	root, _ = writeTree(t, reaching)
	requireViolation(t, transportViolations(t, root, "./graph/entitytext", entityTextTransportRule),
		"example.com/fixture/natsclient")
}
