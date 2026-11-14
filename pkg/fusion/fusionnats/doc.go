// Package fusionnats is the NATS implementation of fusion.RetrievalClient
// (ADR-062): it maps the engine's resolve and expand calls onto graph-query's
// public verbs, so the fusion engine can run against a live graph.
//
// It is a subpackage of pkg/fusion so that fusion itself imports no NATS client
// and stays testable without a live graph. This package is where the engine
// meets the wire: it takes a classified requester (*natsclient.Client is one)
// and the request and reply types declared in graph, and returns fusion's types.
//
// Each method uses one graph-query verb, whose subject is read from
// graph.QueryVerbs() when the client is built:
//
//	Resolve (symbol)  byName
//	Resolve (prefix)  prefixIDs (the IDs only, read afterwards through batch)
//	Resolve (nl)      semantic
//	Entity            entity
//	Entities          batch
//	Neighbors         relationships
//	Names             byName (the matched names, for did_you_mean)
//
// A reply is decoded as its verb's table row declares: an enveloped row's reply
// is a graph.QueryResponse and is refused when it names no producer; a bare
// row's reply is the row's type. Nothing decides a reply's shape from its
// content.
//
// Every call uses RequestClassified, so a handler's refusal arrives as a
// *errs.ClassifiedError with its class and code (ADR-060). Entity reads the
// code entity_not_found as absence, (nil, nil); every other error is returned,
// so the engine never reads a backend fault as a missing entity.
//
// The client starts nothing and reads no readiness status: a read whose index
// is not ready is refused with code index_not_ready, which the engine reads
// from the reply (ADR-109).
package fusionnats
