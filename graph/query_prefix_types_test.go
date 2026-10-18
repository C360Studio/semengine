package graph

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrefixQueryTypesRoundTrip: each prefix request and response survives a JSON
// round trip, under the keys the wire names, and an empty optional field leaves its
// key out (omitempty). The cursor value is opaque here: graph-ingest builds it.
func TestPrefixQueryTypesRoundTrip(t *testing.T) {
	const entityID = "acme.ops.logistics.warehouse.robot.robot-001"
	cases := []struct {
		name    string
		value   any
		keys    []string
		missing []string
	}{
		{"first-page request", PrefixQueryRequest{Prefix: "acme", Limit: 10},
			[]string{"prefix", "limit"}, []string{"cursor", "pattern"}},
		{"next-page request", PrefixQueryRequest{Prefix: "acme", Limit: 10, Cursor: "opaque"},
			[]string{"prefix", "limit", "cursor"}, []string{"pattern"}},
		{"pattern request", PrefixQueryRequest{Pattern: "acme.ops.*.intel.platform.*", Limit: 25},
			[]string{"prefix", "pattern", "limit"}, []string{"cursor"}},
		{"exhausted response", PrefixQueryResponse{Entities: []EntityState{}},
			[]string{"entities"}, []string{"next_cursor"}},
		{"response with continuation", PrefixQueryResponse{Entities: []EntityState{{ID: entityID}}, NextCursor: "opaque"},
			[]string{"entities", "next_cursor"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.value)
			require.NoError(t, err)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(data, &fields))
			for _, key := range tc.keys {
				assert.Contains(t, fields, key, "%s", data)
			}
			for _, key := range tc.missing {
				assert.NotContains(t, fields, key, "%s", data)
			}
			decoded := reflect.New(reflect.TypeOf(tc.value))
			require.NoError(t, json.Unmarshal(data, decoded.Interface()))
			assert.Equal(t, tc.value, decoded.Elem().Interface())
		})
	}
}
