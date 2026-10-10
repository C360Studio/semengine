package graph

// QueryVerb declares one request/reply verb: the subject it is served on, the
// component that answers it, and the shapes of its request and its reply.
type QueryVerb struct {
	// Name is the verb, the last token of its subject.
	Name    string
	Subject string
	// Responder is the component type that answers on Subject.
	Responder string
	// RequestType is the request's Go type, or its JSON fields as
	// {field:type} when it has no named type.
	RequestType string
	// ReplyType is the reply's Go type.
	ReplyType string
}

const responderGraphIngest = "graph-ingest"

// entityVerb is the table's entity entry: ExactEntityReader requests on its
// Subject.
var entityVerb = QueryVerb{
	Name: "entity", Subject: "graph.ingest.query.entity", Responder: responderGraphIngest,
	RequestType: "{id:string}", ReplyType: "graph.ExactEntity",
}

// QueryVerbs returns the declared request/reply verbs. Each call returns a new
// slice, so a caller cannot change the declaration.
func QueryVerbs() []QueryVerb {
	return []QueryVerb{
		entityVerb,
		{
			Name: "batch", Subject: "graph.ingest.query.batch", Responder: responderGraphIngest,
			RequestType: "{ids:[]string}", ReplyType: "graph.EntityBatchResponse",
		},
		{
			Name: "prefix", Subject: "graph.ingest.query.prefix", Responder: responderGraphIngest,
			RequestType: "graph.PrefixQueryRequest", ReplyType: "graph.PrefixQueryResponse",
		},
	}
}
