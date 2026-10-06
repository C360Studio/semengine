package types

import "time"

// Status constants for agent results and workflow steps.
// Using constants prevents typos and enables IDE autocompletion.
const (
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusPending = "pending"
)

// ConstructedContext is pre-built context ready to embed in task payload.
// When present in a TaskMessage, the agent loop uses this directly instead
// of performing discovery or hydration, enabling the "embed context, don't
// make agents discover it" pattern.
type ConstructedContext struct {
	Content       string          `json:"content"`                  // Formatted context for LLM
	TokenCount    int             `json:"token_count"`              // Exact token count
	Entities      []string        `json:"entities,omitempty"`       // Entity IDs included
	Sources       []ContextSource `json:"sources,omitempty"`        // Provenance tracking
	ConstructedAt time.Time       `json:"constructed_at,omitempty"` // When context was built
}

// ContextSource tracks where context came from, providing provenance
// information for debugging and auditing.
type ContextSource struct {
	Type string `json:"type"` // "graph_entity", "graph_relationship", "document"
	ID   string `json:"id"`   // Entity/doc ID
}

// Source type constants for ContextSource.Type
const (
	SourceTypeGraphEntity       = "graph_entity"
	SourceTypeGraphRelationship = "graph_relationship"
	SourceTypeDocument          = "document"
)

// Relationship represents a relationship between entities in the knowledge graph.
// Used by context building and graph query operations.
type Relationship struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
}

// GraphContextSpec defines what graph context to inject into an agent.
// This is used declaratively in workflow definitions to specify context needs.
type GraphContextSpec struct {
	Entities      []string `json:"entities,omitempty"`      // Entity IDs to hydrate
	Relationships bool     `json:"relationships,omitempty"` // Include relationships
	Depth         int      `json:"depth,omitempty"`         // Traversal depth (default 1)
	MaxTokens     int      `json:"max_tokens,omitempty"`    // Token budget for graph context
}
