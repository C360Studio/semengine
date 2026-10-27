package graph

// QueryVerb declares one request/reply verb: the subject it is served on and
// the component that answers it.
type QueryVerb struct {
	// Name is the verb, the last token of its subject.
	Name    string
	Subject string
	// Responder is the component type that answers on Subject.
	Responder string
}

// ResponderGraphIngest is the Responder of the verbs graph-ingest answers.
const ResponderGraphIngest = "graph-ingest"

// ResponderGraphIndex is the Responder of the verbs graph-index answers.
const ResponderGraphIndex = "graph-index"

// entityVerb is the table's entity entry: ExactEntityReader requests on its
// Subject with an EntityRequest and decodes an ExactEntity.
var entityVerb = QueryVerb{Name: "entity", Subject: "graph.ingest.query.entity", Responder: ResponderGraphIngest}

// QueryVerbs returns the declared request/reply verbs. Each call returns a new
// slice, so a caller cannot change the declaration. The entity verb takes an
// EntityRequest and replies with an ExactEntity; batch takes an
// EntityBatchRequest and replies with an EntityBatchResponse; prefix takes a
// PrefixQueryRequest and replies with a PrefixQueryResponse. graph-index's
// verbs reply with a QueryResponse whose Data is the verb's result.
func QueryVerbs() []QueryVerb {
	return []QueryVerb{
		entityVerb,
		{Name: "batch", Subject: "graph.ingest.query.batch", Responder: ResponderGraphIngest},
		{Name: "prefix", Subject: "graph.ingest.query.prefix", Responder: ResponderGraphIngest},
		{Name: "outgoing", Subject: "graph.index.query.outgoing", Responder: ResponderGraphIndex},
		{Name: "incoming", Subject: "graph.index.query.incoming", Responder: ResponderGraphIndex},
		{Name: "alias", Subject: "graph.index.query.alias", Responder: ResponderGraphIndex},
		{Name: "predicate", Subject: "graph.index.query.predicate", Responder: ResponderGraphIndex},
		{Name: "predicateList", Subject: "graph.index.query.predicateList", Responder: ResponderGraphIndex},
		{Name: "predicateStats", Subject: "graph.index.query.predicateStats", Responder: ResponderGraphIndex},
		{Name: "predicateCompound", Subject: "graph.index.query.predicateCompound", Responder: ResponderGraphIndex},
		{Name: "byName", Subject: "graph.index.query.byName", Responder: ResponderGraphIndex},
	}
}
