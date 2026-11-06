// Package graphquery provides graph-query, the component that answers the graph.query.* verbs
// declared for it in graph.QueryVerbs. It is a thin layer over graph-ingest and graph-index: they
// hold the answers. It writes nothing; its one piece of state is a read-only view of the COMMUNITY_INDEX
// bucket, which localSearch reads.
//
// The table in graph decides what it serves. Start subscribes a handler to each verb the table
// declares for graph-query and fails if one has no handler.
//
// entity, batch and prefix forward to graph-ingest's verbs of the same name, byName to graph-index's.
// A forward passes the request and reply bytes unchanged, and returns a refusal with its class, code
// and detail. A request that gets no answer is refused transient with code internal, naming the
// subject: nothing responding, query_timeout_ms passing, the handler's context ending (natsclient's
// handler budget or the client closing), or a lost connection.
//
// entityByAlias asks graph-index's alias verb, then reads the holder from graph-ingest's entity verb,
// or the input as an entity ID when nobody holds it and it is a valid one; other input is not found.
// relationships asks graph-index's outgoing or incoming verb, as direction says, and turns the rows
// into graph.RelationshipEdge. Both answer in a graph.QueryResponse from graph-query carrying the index
// reply's revision, and return any refusal unchanged.
//
// pathSearch walks the index's outgoing and incoming verbs breadth first from a start entity and answers
// the entities and edges within max_depth and max_nodes, each once. hierarchyStats and summary read one
// page of entity IDs under a prefix from graph-ingest's prefix verb, so a count covers that page and no
// more: total_entities is the whole group as listed, and truncated, entities_scanned and next_cursor say
// whether the rest is a later page. summary also asks graph-index's predicateList verb unless
// exclude_predicates is set. semantic, similar, spatial and temporal forward to the embedding, spatial
// and temporal verbs, whose responders are not in the tree; until they are, each is refused transient
// internal, naming the subject.
//
// localSearch searches one group of entities, the entities that share the first 4, 3 or 2 parts of an ID
// (level 0, 1 or 2). It reads the group's record from the view of COMMUNITY_INDEX, then one page of at most
// 1,000 entities of the group from graph-ingest's prefix verb, and ranks that page by how well each entity
// matches the words of the query. The page is in ID order, so on a larger group the matches come from the
// lexically smallest IDs, and entities_truncated says so. The reply's indexed_revision is the record's
// entity_revision, and a min_revision above it is refused revision_not_reached before the page is read.
// Start does not wait for the bucket and does not fail when it is missing: one goroutine opens it, runs
// the view, and opens it again every 5 seconds when the bucket is missing or its watch is lost, and
// localSearch is refused transient index_not_ready until a view has caught up. Stop ends the view.
// semengine_graph_query_search_truncated_total counts localSearch answers that were cut short.
//
// An empty configuration is enough. The one thing a configuration may set is query_timeout_ms, the
// deadline on the reply to each downstream request in milliseconds, default 15000. It does not end a
// wait to write to a broker that has stopped reading (ADR-112, "Time"). A key the configuration does
// not define is refused at construction, naming the key, and so is any input port other than the
// required graph_queries nats-request port on graph.query.*, the port an empty configuration takes.
//
// The package README.md describes its configuration, ports and health.
package graphquery
