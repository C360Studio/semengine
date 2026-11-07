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
