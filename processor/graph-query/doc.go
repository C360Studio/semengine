// Package graphquery provides graph-query, the component that answers the graph.query.* verbs
// declared for it in graph.QueryVerbs. It is a thin layer over graph-ingest and graph-index: they
// hold the answers, with graph-lexical's word index. It writes nothing; its one piece of state is a
// read-only view of the COMMUNITY_INDEX bucket, which localSearch and the text search of globalSearch read.
//
// The table in graph decides what it serves. Start subscribes a handler to each verb the table
// declares for graph-query and fails if one has no handler.
//
// entity, batch, prefix and prefixIDs forward to graph-ingest's verbs of the same name, byName to graph-index's.
// A forward passes the request and reply bytes unchanged, and returns a refusal with its class, code
// and detail. A request that gets no answer is refused transient with code internal, naming the
// subject: nothing responding, query_timeout_ms passing, the handler's context ending (natsclient's
// handler budget or the client closing), or a lost connection.
//
// entityByAlias asks graph-index's alias verb, then reads the holder from graph-ingest's entity verb,
// or the input as an entity ID when nobody holds it and it is a valid one; other input is not found.
// globalSearch resolves a reference by the same rule.
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
// and temporal verbs. The spatial and temporal responders are not in the tree; until they are, those two
// are refused transient internal, naming the subject. The embedding_enabled key (true when left out) says
// whether graph-embedding runs: when it is false, semantic and similar refuse every request invalid with
// code not_enabled, naming the verb, before anything is sent, and the refusal counts as an input, not an
// error.
//
// localSearch searches one group of entities, the entities that share the first 4, 3 or 2 parts of an ID
// (level 0, 1 or 2). It asks graph-lexical's word search, scoped to the group's ID, passing the query, the
// limit and min_revision unchanged, then reads the hits back with one graph-ingest batch request, so the
// matches keep the word index's order and each entity comes as graph-ingest sent it. The reply's
// indexed_revision is the word search's. The group's record from the view of COMMUNITY_INDEX is attached
// when the view holds one, and record_state says when it does not; a missing record never refuses.
// Start does not wait for the bucket and does not fail when it is missing: one goroutine opens it, runs
// the view, and opens it again every 5 seconds when the bucket is missing or its watch is lost. The text
// search of globalSearch is refused transient index_not_ready until a view has caught up. Stop ends the
// view. semengine_graph_query_search_truncated_total counts localSearch and text search answers that were
// cut short.
//
// globalSearch, and searchGraph, which is the same handler under a second name with the same data,
// choose one strategy from the question and answer with it. A question of one word is an entity
// reference, resolved as entityByAlias resolves it (the alias verb, else a valid entity ID) and
// answered with the entity verb's reply. Any longer question is classified once by the keyword tier
// of graph/query, and a strategy answers only when its hint is the question's only hint: a path
// from a start (the pathSearch walk, both directions, depth 3, 100 entities), a time range (the
// time-range verb) or an area (the bounds verb). Two of those, an entity beside a zone or area, an
// impossible number, a count or other aggregation, and path words, a similarity or a ranking beside
// a start, a time range or an area are refused invalid_request, and a named path start that names
// no entity is entity_not_found. Every other question is the text strategy: no hint, path words
// with no start, a similarity or a ranking alone, and a one-word name that names no entity. It
// scores the COMMUNITY_INDEX records at the request's level against the question's words, reads one
// page of each of the best five scoring above 0, and replies with the matching entities (id, label,
// score, the entity as graph-ingest sent it), the groups read, the hints it could not apply
// (unhonoured_hints) and a plain answer. The matches are a sample, and entities_truncated says so.
// Its indexed_revision is the highest entity_revision among the level's readable records. A view
// that is not usable, or a level with no record, is transient index_not_ready; some unreadable
// records are an answer that says so, one Health error and one Warn. A refusal from the alias or
// entity verb reaches the caller as it arrived. Until their responders are in the tree, the
// time-range and bounds strategies are refused transient internal, naming the subject. The keyword
// classifier's limits stay its own: "from A to B" walks from A, and the refusal of a second place
// can be wrong for a hyphenated word such as "real-time".
//
// An empty configuration is enough. The one thing a configuration may set is query_timeout_ms, the
// deadline on the reply to each downstream request in milliseconds, default 15000. It does not end a
// wait to write to a broker that has stopped reading (ADR-112, "Time"). A key the configuration does
// not define is refused at construction, naming the key, and so is any input port other than the
// required graph_queries nats-request port on graph.query.*, the port an empty configuration takes.
//
// The package README.md describes its configuration, ports and health.
package graphquery
