package graph

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// decodeStrictly decodes data as a responder of these requests must: an unknown key is refused.
func decodeStrictly(data string, into any) error {
	decoder := json.NewDecoder(bytes.NewReader([]byte(data)))
	decoder.DisallowUnknownFields()
	return decoder.Decode(into)
}

// TestSearchRequestTypesWire proves the wire spelling of the requests global search sends to the
// time-range and bounds verbs (design D4): start_time and end_time as RFC 3339, the four edges, limit,
// and min_revision only when it is set. A strict decoder reads each literal back into the same value
// and refuses the pin's startTime and endTime by name, so a responder decoding the pin's spelling
// cannot be mistaken for one reading this one.
func TestSearchRequestTypesWire(t *testing.T) {
	start := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 11, 7, 12, 0, 0, 500, time.FixedZone("", -8*3600))
	cases := []struct {
		name  string
		value any
		into  func() any
		want  string
	}{
		{
			"temporal",
			TemporalRangeRequest{StartTime: start, EndTime: end, Limit: 100},
			func() any { return &TemporalRangeRequest{} },
			`{"start_time":"2026-11-07T10:00:00Z","end_time":"2026-11-07T12:00:00.0000005-08:00","limit":100}`,
		},
		{
			"temporal with revision",
			TemporalRangeRequest{StartTime: start, EndTime: start, Limit: 100, MinRevisionField: MinRevisionField{MinRevision: 3}},
			func() any { return &TemporalRangeRequest{} },
			`{"start_time":"2026-11-07T10:00:00Z","end_time":"2026-11-07T10:00:00Z","limit":100,"min_revision":3}`,
		},
		{
			"spatial",
			SpatialBoundsRequest{North: 1, South: -1, East: 1.5, West: -1.5, Limit: 100},
			func() any { return &SpatialBoundsRequest{} },
			`{"north":1,"south":-1,"east":1.5,"west":-1.5,"limit":100}`,
		},
		{
			"spatial with revision",
			SpatialBoundsRequest{North: 90, South: 88.5, East: 180, West: -180, Limit: 100, MinRevisionField: MinRevisionField{MinRevision: 3}},
			func() any { return &SpatialBoundsRequest{} },
			`{"north":90,"south":88.5,"east":180,"west":-180,"limit":100,"min_revision":3}`,
		},
	}
	for _, tc := range cases {
		got, err := json.Marshal(tc.value)
		if err != nil {
			t.Fatalf("%s: marshal: %v", tc.name, err)
		}
		if string(got) != tc.want {
			t.Errorf("%s = %s, want %s", tc.name, got, tc.want)
		}
		back := tc.into()
		if err := decodeStrictly(tc.want, back); err != nil {
			t.Fatalf("%s: strict decode of %s: %v", tc.name, tc.want, err)
		}
		// Compared through the wire, since a decoded time may carry time.Local for the same offset.
		if again, err := json.Marshal(back); err != nil || string(again) != tc.want {
			t.Errorf("%s: decoded and marshalled again = %s (%v), want %s", tc.name, again, err, tc.want)
		}
	}

	for _, key := range []string{"startTime", "endTime"} {
		pin := `{"` + key + `":"2026-11-07T10:00:00Z","limit":100}`
		err := decodeStrictly(pin, &TemporalRangeRequest{})
		if err == nil || !strings.Contains(err.Error(), `"`+key+`"`) {
			t.Errorf("strict decode of the pin's %s = %v, want a refusal naming it", pin, err)
		}
	}
}

// TestRequestsDecodeStrictly proves the embedding verbs' requests and replies on the wire (design D1,
// D3, D4). Each request literal decodes strictly into the value it was marshalled from, min_revision
// included, so a responder that refuses unknown keys still reads the forward graph-query sends. A key
// one request defines and the other does not is refused by name: scope sent to similar, entity_id sent
// to search. Both replies always carry the two record counts, 0 included, since a 0 is the answer that
// nothing was skipped. The limit bounds are the spec's literals: 0 means 10, the largest is 100.
func TestRequestsDecodeStrictly(t *testing.T) {
	if DefaultSimilarityLimit != 10 || MaxSimilarityLimit != 100 {
		t.Errorf("limits = %d, %d; want 10, 100", DefaultSimilarityLimit, MaxSimilarityLimit)
	}

	requests := []struct {
		name  string
		value any
		into  func() any
		want  string
	}{
		{
			"search",
			SemanticRequest{Query: "pump failure", Limit: 5, Scope: []string{"acme.ops", ""}, MinRevisionField: MinRevisionField{MinRevision: 7}},
			func() any { return &SemanticRequest{} },
			`{"query":"pump failure","limit":5,"scope":["acme.ops",""],"min_revision":7}`,
		},
		{
			"search unscoped",
			SemanticRequest{Query: "pump"},
			func() any { return &SemanticRequest{} },
			`{"query":"pump","limit":0}`,
		},
		{
			"similar",
			SimilarRequest{EntityID: "acme.ops.robotics.gcs.drone.001", Limit: 100, MinRevisionField: MinRevisionField{MinRevision: 7}},
			func() any { return &SimilarRequest{} },
			`{"entity_id":"acme.ops.robotics.gcs.drone.001","limit":100,"min_revision":7}`,
		},
	}
	for _, tc := range requests {
		got, err := json.Marshal(tc.value)
		if err != nil {
			t.Fatalf("%s: marshal: %v", tc.name, err)
		}
		if string(got) != tc.want {
			t.Errorf("%s = %s, want %s", tc.name, got, tc.want)
		}
		back := tc.into()
		if err := decodeStrictly(tc.want, back); err != nil {
			t.Fatalf("%s: strict decode of %s: %v", tc.name, tc.want, err)
		}
		if again, err := json.Marshal(back); err != nil || string(again) != tc.want {
			t.Errorf("%s: decoded and marshalled again = %s (%v), want %s", tc.name, again, err, tc.want)
		}
	}

	refused := []struct {
		key  string
		data string
		into any
	}{
		{"entity_id", `{"query":"pump","entity_id":"acme.ops.robotics.gcs.drone.001"}`, &SemanticRequest{}},
		{"scope", `{"entity_id":"acme.ops.robotics.gcs.drone.001","scope":["acme"]}`, &SimilarRequest{}},
	}
	for _, tc := range refused {
		err := decodeStrictly(tc.data, tc.into)
		if err == nil || !strings.Contains(err.Error(), `"`+tc.key+`"`) {
			t.Errorf("strict decode of %s = %v, want a refusal naming %s", tc.data, err, tc.key)
		}
	}

	replies := []struct {
		name  string
		value any
		want  string
	}{
		{
			"search reply",
			SemanticData{Results: []SimilarityHit{}, EmbedderType: "bm25"},
			`{"results":[],"embedder_type":"bm25","unreadable_records":0,"failed_records":0}`,
		},
		{
			"similar reply",
			SimilarData{Results: []SimilarityHit{{EntityID: "a.b.c.d.e.f", Similarity: 0.5}}, UnreadableRecords: 2, FailedRecords: 3},
			`{"results":[{"entity_id":"a.b.c.d.e.f","similarity":0.5}],"unreadable_records":2,"failed_records":3}`,
		},
	}
	for _, tc := range replies {
		got, err := json.Marshal(tc.value)
		if err != nil {
			t.Fatalf("%s: marshal: %v", tc.name, err)
		}
		if string(got) != tc.want {
			t.Errorf("%s = %s, want %s", tc.name, got, tc.want)
		}
	}
}
