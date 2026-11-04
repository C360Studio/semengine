package graph

// Limits of the pathSearch verb. A request over one is refused, stating the bound, rather than clamped:
// a clamped depth would be silent and truncated would stop being exact.
const (
	// MaxPathSearchDepth is the largest max_depth a request may send.
	MaxPathSearchDepth = 10
	// MaxPathSearchNodes is the largest max_nodes a request may send.
	MaxPathSearchNodes = 500
	// DefaultPathSearchNodes is what a max_nodes of 0 means.
	DefaultPathSearchNodes = 100
)

// PathSearchRequest is the pathSearch verb's request, decoded strictly: an unknown key is refused,
// since a typo in Predicates would widen the answer.
//
// Direction is required: outgoing, incoming or both. MaxDepth is required, 1 to MaxPathSearchDepth.
// MaxNodes is the most entities in the reply, the start included: 0 means DefaultPathSearchNodes, else
// 1 to MaxPathSearchNodes. Predicates, when not empty, keeps only edges holding one of them.
type PathSearchRequest struct {
	StartEntity string   `json:"start_entity"`
	Direction   string   `json:"direction"`
	MaxDepth    int      `json:"max_depth"`
	MaxNodes    int      `json:"max_nodes,omitempty"`
	Predicates  []string `json:"predicates,omitempty"`
	MinRevisionField
}

// PathEntity is one entity a path search reached. Depth is its shortest distance from the start under
// the request's direction and predicates; the start is at 0.
type PathEntity struct {
	EntityID string `json:"entity_id"`
	Depth    int    `json:"depth"`
}

// PathSearchData is the reply to a path search, each entity and edge once. Entities are in visit order,
// the start first. Edges[i] first reached Entities[i+1], and names the edge in the direction the graph
// holds it, whichever way the walk crossed it; a path is the edges followed back. An edge may name an
// entity with no record.
//
// Truncated is true exactly when the walk found an unseen entity and left it out, as read at the hops.
// When it is false, Entities is every entity within the request's reach.
type PathSearchData struct {
	Entities  []PathEntity       `json:"entities"`
	Edges     []RelationshipEdge `json:"edges"`
	Truncated bool               `json:"truncated"`
}
