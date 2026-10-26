package graph

import (
	"errors"
	"time"
)

// QueryResponse is the success envelope of a graph-index query reply. A reply is
// either this body or a classified error (ADR-060); a refusal carries no envelope.
//
// IndexedRevision is the ENTITY_STATES revision the reply reflects: every write at
// or below it is reflected in Data, except on an alias reply while issue #415 is
// open, which carries the same revision but promises nothing about which aliases
// resolve. A caller holding a write's kv_revision compares it here. Producer names
// the component that answered, so a reply relayed through another component can
// still be attributed.
type QueryResponse[T any] struct {
	Data            T         `json:"data"`
	IndexedRevision uint64    `json:"indexed_revision"`
	Producer        string    `json:"producer"`
	Timestamp       time.Time `json:"timestamp"`
}

// errQueryResponseProducer is NewQueryResponse's refusal of an empty producer.
var errQueryResponseProducer = errors.New("graph: query response producer is required")

// NewQueryResponse builds a reply from producer, the revision its data reflects and
// the data, stamped with the current time. It refuses an empty producer: a reply
// that does not name who answered cannot be attributed.
func NewQueryResponse[T any](producer string, indexedRevision uint64, data T) (QueryResponse[T], error) {
	if producer == "" {
		return QueryResponse[T]{}, errQueryResponseProducer
	}
	return QueryResponse[T]{
		Data:            data,
		IndexedRevision: indexedRevision,
		Producer:        producer,
		Timestamp:       time.Now(),
	}, nil
}
