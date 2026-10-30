package query

import "time"

// SearchStrategy represents the type of search to perform.
type SearchStrategy string

// Search strategy constants for query routing.
const (
	StrategyGraphRAG         SearchStrategy = "graphrag"
	StrategyGeoGraphRAG      SearchStrategy = "geo_graphrag"
	StrategyTemporalGraphRAG SearchStrategy = "temporal_graphrag"
	StrategyHybridGraphRAG   SearchStrategy = "hybrid_graphrag"
	StrategyPathRAG          SearchStrategy = "pathrag"
	StrategySemantic         SearchStrategy = "semantic"
	StrategyExact            SearchStrategy = "exact"
	StrategyAggregation      SearchStrategy = "aggregation"
)

// Aggregation type constants for aggregation queries.
const (
	AggregationCount = "count"
	AggregationAvg   = "avg"
	AggregationSum   = "sum"
	AggregationMin   = "min"
	AggregationMax   = "max"
)

// SearchOptions provides declarative search configuration.
type SearchOptions struct {
	Query          string         `json:"query,omitempty"`
	GeoBounds      *SpatialBounds `json:"geo_bounds,omitempty"`
	TimeRange      *TimeRange     `json:"time_range,omitempty"`
	Predicates     []string       `json:"predicates,omitempty"`
	Types          []string       `json:"types,omitempty"`
	KeyTerms       []string       `json:"key_terms,omitempty"`
	UseEmbeddings  bool           `json:"use_embeddings,omitempty"`
	Strategy       SearchStrategy `json:"strategy,omitempty"`
	Limit          int            `json:"limit,omitempty"`
	PathIntent     bool           `json:"path_intent,omitempty"`
	PathStartNode  string         `json:"path_start_node,omitempty"`
	PathPredicates []string       `json:"path_predicates,omitempty"`
	// AggregationType specifies the type of aggregation (count, avg, sum, min, max).
	// Use the AggregationCount / AggregationAvg / … constants.
	AggregationType string `json:"aggregation_type,omitempty"`
	// AggregationField is the property or attribute to aggregate on (e.g. "temperature").
	// Empty means aggregate over entity count.
	AggregationField string `json:"aggregation_field,omitempty"`
	// RankingIntent is true when the query requests a ranked/top-N result set.
	RankingIntent bool `json:"ranking_intent,omitempty"`
}

// TimeRange represents temporal query bounds.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// SpatialBounds represents geographic bounding box.
type SpatialBounds struct {
	North float64 `json:"north"`
	South float64 `json:"south"`
	East  float64 `json:"east"`
	West  float64 `json:"west"`
}
