// Package graphingest provides graph-ingest, the component that writes entities and
// their triples into the ENTITY_STATES KV bucket, which it owns: it consumes entity
// messages from JetStream, serves the graph-mutation operations over NATS
// request/reply, and answers the entity reads declared for it in graph.QueryVerbs.
// The package README.md describes its ports, configuration, metrics and health.
package graphingest
