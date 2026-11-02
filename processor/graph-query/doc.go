// Package graphquery provides graph-query, the component that answers the graph.query.* verbs
// declared for it in graph.QueryVerbs. It is a thin layer over graph-ingest and graph-index: they
// hold the answers, and graph-query keeps no state of its own.
//
// The table in graph decides what it serves. Start subscribes a handler to each verb the table
// declares for graph-query and fails if one has no handler.
//
// entity, batch and prefix forward to graph-ingest's verbs of the same name, byName to graph-index's.
// A forward passes the request and reply bytes unchanged, and returns a refusal with its class, code
// and detail. A request that gets no answer, because nothing responds or query_timeout_ms passes, is
// refused transient with code internal, naming the subject.
//
// An empty configuration is enough. The one thing a configuration may set is query_timeout_ms, the
// bound on each downstream request in milliseconds, default 15000. A key it does not define is
// refused at construction, naming the key, and so is any input port other than the required
// graph_queries nats-request port on graph.query.*, the port an empty configuration takes.
//
// The package README.md describes its configuration, ports and health.
package graphquery
