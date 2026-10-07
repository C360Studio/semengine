// Package graph provides query response type aliases for common query patterns.
package graph

// --- Type Aliases for Common Response Types ---

// OutgoingQueryResponse is the response type for outgoing relationship queries.
type OutgoingQueryResponse = QueryResponse[OutgoingRelationshipsData]

// IncomingQueryResponse is the response type for incoming relationship queries.
type IncomingQueryResponse = QueryResponse[IncomingRelationshipsData]

// AliasQueryResponse is the response type for alias resolution queries.
type AliasQueryResponse = QueryResponse[AliasData]
