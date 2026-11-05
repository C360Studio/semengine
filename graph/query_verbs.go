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
	// when any answer of the verb can use a graph-index or other derived-index
	// reply. A caller decodes by verb; nothing decides a reply's shape from its
	// content.
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

// ResponderGraphEmbedding is the Responder of the embedding verbs, whose
// component arrives with a later change.
const ResponderGraphEmbedding = "graph-embedding"

// ResponderGraphIndexSpatial is the Responder of the spatial verb, whose
// component arrives with a later change.
const ResponderGraphIndexSpatial = "graph-index-spatial"

// ResponderGraphIndexTemporal is the Responder of the temporal verb, whose
// component arrives with a later change.
const ResponderGraphIndexTemporal = "graph-index-temporal"

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
// QueryResponse whose Data is the entry's Reply, as are the derived-index verbs'
// (search, similar, bounds, range), whose responders are not in the tree yet. On
// graph.query.*, graph-query's forwards answer as the verb they forward to does, and
// entityByAlias, relationships and pathSearch, which read index replies, are enveloped
// from graph-query, and hierarchyStats, which reads graph-ingest alone, is bare; a verb a
// consumer serves under graph.query.* is not in this table. A Name is unique only within its
// Responder. pathSearch takes a PathSearchRequest and hierarchyStats a HierarchyStatsRequest.
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
		{Name: "search", Subject: "graph.embedding.query.search", Responder: ResponderGraphEmbedding,
			Enveloped: true, Reply: reflect.TypeFor[SemanticData]()},
		{Name: "similar", Subject: "graph.embedding.query.similar", Responder: ResponderGraphEmbedding,
			Enveloped: true, Reply: reflect.TypeFor[SimilarData]()},
		{Name: "bounds", Subject: "graph.spatial.query.bounds", Responder: ResponderGraphIndexSpatial,
			Enveloped: true, Reply: reflect.TypeFor[SpatialData]()},
		{Name: "range", Subject: "graph.temporal.query.range", Responder: ResponderGraphIndexTemporal,
			Enveloped: true, Reply: reflect.TypeFor[TemporalData]()},
		{Name: "entity", Subject: "graph.query.entity", Responder: ResponderGraphQuery,
			Reply: reflect.TypeFor[ExactEntity]()},
		{Name: "batch", Subject: "graph.query.batch", Responder: ResponderGraphQuery,
			Reply: reflect.TypeFor[EntityBatchResponse]()},
		{Name: "prefix", Subject: "graph.query.prefix", Responder: ResponderGraphQuery,
			Reply: reflect.TypeFor[PrefixQueryResponse]()},
		{Name: "byName", Subject: "graph.query.byName", Responder: ResponderGraphQuery,
			Enveloped: true, Reply: reflect.TypeFor[NameData]()},
		{Name: "entityByAlias", Subject: "graph.query.entityByAlias", Responder: ResponderGraphQuery,
			Enveloped: true, Reply: reflect.TypeFor[ExactEntity]()},
		{Name: "relationships", Subject: "graph.query.relationships", Responder: ResponderGraphQuery,
			Enveloped: true, Reply: reflect.TypeFor[[]RelationshipEdge]()},
		{Name: "pathSearch", Subject: "graph.query.pathSearch", Responder: ResponderGraphQuery,
			Enveloped: true, Reply: reflect.TypeFor[PathSearchData]()},
		{Name: "hierarchyStats", Subject: "graph.query.hierarchyStats", Responder: ResponderGraphQuery,
			Reply: reflect.TypeFor[HierarchyStatsData]()},
		{Name: "semantic", Subject: "graph.query.semantic", Responder: ResponderGraphQuery,
			Enveloped: true, Reply: reflect.TypeFor[SemanticData]()},
		{Name: "similar", Subject: "graph.query.similar", Responder: ResponderGraphQuery,
			Enveloped: true, Reply: reflect.TypeFor[SimilarData]()},
		{Name: "spatial", Subject: "graph.query.spatial", Responder: ResponderGraphQuery,
			Enveloped: true, Reply: reflect.TypeFor[SpatialData]()},
		{Name: "temporal", Subject: "graph.query.temporal", Responder: ResponderGraphQuery,
			Enveloped: true, Reply: reflect.TypeFor[TemporalData]()},
	}
}
