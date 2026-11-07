package graph

import "encoding/json"

// GlobalSearchRequest is the globalSearch verb's request, decoded strictly: an unknown key is refused,
// naming it. Query is the question, required and not blank. The question is classified by its words
// alone; no other key steers which strategy answers. Level is the community level the text strategy
// searches, 0 to 2, omitted 0; it is checked on every request and read by no other strategy.
type GlobalSearchRequest struct {
	Query string `json:"query"`
	Level int    `json:"level,omitempty"`
	MinRevisionField
}

// GlobalSearchData is the reply to a global search: the strategy that answered and that strategy's
// result, exactly one of the four fields below.
//
// Strategy is "entity", "path", "temporal" or "spatial". Entity is the entity verb's reply data, its
// bytes as they came. Path is the path search from the resolved start. Temporal and Spatial are what
// the time-range and bounds verbs answered. Each is the result as read, never a summary of it.
type GlobalSearchData struct {
	Strategy string          `json:"strategy"`
	Entity   json.RawMessage `json:"entity,omitempty"`
	Path     *PathSearchData `json:"path,omitempty"`
	Temporal *TemporalData   `json:"temporal,omitempty"`
	Spatial  *SpatialData    `json:"spatial,omitempty"`
}
