// Package graph provides query types for graph operations
package graph

// QueryDirection represents the direction of a query
type QueryDirection int

const (
	// QueryDirectionOutgoing queries outgoing edges/relationships
	QueryDirectionOutgoing QueryDirection = iota
	// QueryDirectionIncoming queries incoming edges/relationships
	QueryDirectionIncoming
	// QueryDirectionBidirectional queries both directions
	QueryDirectionBidirectional
)

// String returns the string representation of QueryDirection
func (qd QueryDirection) String() string {
	switch qd {
	case QueryDirectionOutgoing:
		return "outgoing"
	case QueryDirectionIncoming:
		return "incoming"
	case QueryDirectionBidirectional:
		return "bidirectional"
	default:
		return "unknown"
	}
}
