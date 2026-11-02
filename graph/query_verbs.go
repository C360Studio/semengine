package graph

import "reflect"

// QueryVerb declares one request/reply verb: the subject it is served on and
// the component that answers it.
type QueryVerb struct {
	// Name is the verb, the last token of its subject.
	Name    string
	Subject string
	// Responder is the component type that answers on Subject.
	Responder string
	// Enveloped says the reply is a QueryResponse rather than a bare body: true
	// when any part of the answer comes from a graph-index reply. A caller
	// decodes by verb; nothing decides a reply's shape from its content.
	Enveloped bool
	// Reply is the type of the reply body, or of the envelope's Data when
	// Enveloped.
	Reply reflect.Type
}

// ResponderGraphIngest is the Responder of the verbs graph-ingest answers.
const ResponderGraphIngest = "graph-ingest"

// ResponderGraphIndex is the Responder of the verbs graph-index answers.
const ResponderGraphIndex = "graph-index"

// ResponderGraphQuery is the Responder of the verbs graph-query answers.
const ResponderGraphQuery = "graph-query"

// entityVerb is the table's entity entry: ExactEntityReader requests on its
// Subject with an EntityRequest and decodes an ExactEntity.
var entityVerb = QueryVerb{
	Name: "entity", Subject: "graph.ingest.query.entity", Responder: ResponderGraphIngest,
	Reply: reflect.TypeFor[ExactEntity](),
}

// QueryVerbs returns the declared request/reply verbs. Each call returns a new
// slice, so a caller cannot change the declaration. The entity verb takes an
// EntityRequest, batch an EntityBatchRequest and prefix a PrefixQueryRequest.
// Each entry declares its reply: graph-ingest's are bare, and graph-index's are a
// QueryResponse whose Data is the entry's Reply. graph-query's forwards answer as
// the verb they forward to does, on graph.query.*; a verb a consumer serves under
// graph.query.* is not in this table. A Name is unique only within its Responder.
func QueryVerbs() []QueryVerb {
	return []QueryVerb{
		entityVerb,
		{Name: "batch", Subject: "graph.ingest.query.batch", Responder: ResponderGraphIngest,
			Reply: reflect.TypeFor[EntityBatchResponse]()},
		{Name: "prefix", Subject: "graph.ingest.query.prefix", Responder: ResponderGraphIngest,
			Reply: reflect.TypeFor[PrefixQueryResponse]()},
		{Name: "outgoing", Subject: "graph.index.query.outgoing", Responder: ResponderGraphIndex,
			Enveloped: true, Reply: reflect.TypeFor[OutgoingRelationshipsData]()},
		{Name: "incoming", Subject: "graph.index.query.incoming", Responder: ResponderGraphIndex,
			Enveloped: true, Reply: reflect.TypeFor[IncomingRelationshipsData]()},
		{Name: "alias", Subject: "graph.index.query.alias", Responder: ResponderGraphIndex,
			Enveloped: true, Reply: reflect.TypeFor[AliasData]()},
		{Name: "predicate", Subject: "graph.index.query.predicate", Responder: ResponderGraphIndex,
			Enveloped: true, Reply: reflect.TypeFor[PredicateData]()},
		{Name: "predicateList", Subject: "graph.index.query.predicateList", Responder: ResponderGraphIndex,
			Enveloped: true, Reply: reflect.TypeFor[PredicateListData]()},
		{Name: "predicateStats", Subject: "graph.index.query.predicateStats", Responder: ResponderGraphIndex,
			Enveloped: true, Reply: reflect.TypeFor[PredicateStatsData]()},
		{Name: "predicateCompound", Subject: "graph.index.query.predicateCompound", Responder: ResponderGraphIndex,
			Enveloped: true, Reply: reflect.TypeFor[CompoundPredicateData]()},
		{Name: "byName", Subject: "graph.index.query.byName", Responder: ResponderGraphIndex,
			Enveloped: true, Reply: reflect.TypeFor[NameData]()},
		{Name: "entity", Subject: "graph.query.entity", Responder: ResponderGraphQuery,
			Reply: reflect.TypeFor[ExactEntity]()},
		{Name: "batch", Subject: "graph.query.batch", Responder: ResponderGraphQuery,
			Reply: reflect.TypeFor[EntityBatchResponse]()},
		{Name: "prefix", Subject: "graph.query.prefix", Responder: ResponderGraphQuery,
			Reply: reflect.TypeFor[PrefixQueryResponse]()},
		{Name: "byName", Subject: "graph.query.byName", Responder: ResponderGraphQuery,
			Enveloped: true, Reply: reflect.TypeFor[NameData]()},
	}
}
