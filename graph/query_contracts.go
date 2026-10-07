// Package graph provides query request/response contracts for the graph query system.
// These types are shared between handlers (producers) and clients (consumers)
// to ensure type safety and consistent API contracts.
package graph

import (
	"errors"
	"time"
)

// QueryResponse is the one reply envelope of the graph.query.* family: the
// answer, the ENTITY_STATES revision it reflects, the component instance that
// produced it, and when. Build one with NewQueryResponse.
//
// A reply's shape is the reply type its verb declares; nothing decides from a
// reply's content whether it is enveloped. Replies on graph.ingest.query.* are
// not enveloped: they read the authoritative bucket, and the entity verb's
// reply, ExactEntity, carries the revision it read.
//
// ADR-060: a query reply is EITHER this success body (nil Go error) OR a typed
// *errs.ClassifiedError on the err channel — the in-body Error field was
// removed. A RequestClassified caller branches on the returned err
// (errs.IsInvalid / IsTransient, errors.As → ce.Code); success unmarshals here.
type QueryResponse[T any] struct {
	Data T `json:"data"`
	// IndexedRevision is the ENTITY_STATES revision the answer reflects.
	IndexedRevision uint64 `json:"indexed_revision"`
	// Producer names the component instance that answered.
	Producer  string    `json:"producer"`
	Timestamp time.Time `json:"timestamp"`
}

var errNoProducer = errors.New("graph: a query response needs the producer that answered")

// NewQueryResponse builds the reply that producer, having indexed
// ENTITY_STATES up to indexedRevision, sends for data. It refuses an empty
// producer.
func NewQueryResponse[T any](data T, producer string, indexedRevision uint64) (QueryResponse[T], error) {
	if producer == "" {
		return QueryResponse[T]{}, errNoProducer
	}
	return QueryResponse[T]{
		Data:            data,
		IndexedRevision: indexedRevision,
		Producer:        producer,
		Timestamp:       time.Now(),
	}, nil
}

// MinRevisionField is the one declaration of the min_revision field a
// graph.query.* request may carry: the lowest ENTITY_STATES revision the
// caller accepts an answer from. A request type embeds it, so the field sits
// at the top level of the request's JSON; zero, the default, asks for no
// minimum. No request type embeds it yet, and no producer reads it yet.
type MinRevisionField struct {
	MinRevision uint64 `json:"min_revision,omitempty"`
}
