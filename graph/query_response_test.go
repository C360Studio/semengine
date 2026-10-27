package graph

import (
	"encoding/json"
	"slices"
	"testing"
	"time"
)

// TestQueryResponseCarriesIndexedRevisionAndProducer holds the reply envelope of design
// D6 (ingest-kernel D18): a successful reply carries its data, the revision it reflects,
// the component that produced it and when, under exactly the four JSON keys a caller
// decodes.
func TestQueryResponseCarriesIndexedRevisionAndProducer(t *testing.T) {
	before := time.Now()
	resp, err := NewQueryResponse("graph-index", 42, []string{"a", "b"})
	after := time.Now()
	if err != nil {
		t.Fatalf("NewQueryResponse: %v", err)
	}

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal into a map: %v", err)
	}
	var keys []string
	for k := range fields {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if want := []string{"data", "indexed_revision", "producer", "timestamp"}; !slices.Equal(keys, want) {
		t.Fatalf("envelope keys = %q, want %q (wire: %s)", keys, want, raw)
	}
	if got := string(fields["indexed_revision"]); got != "42" {
		t.Errorf("indexed_revision = %s, want 42", got)
	}
	if got := string(fields["producer"]); got != `"graph-index"` {
		t.Errorf("producer = %s, want \"graph-index\"", got)
	}
	if got := string(fields["data"]); got != `["a","b"]` {
		t.Errorf("data = %s, want [\"a\",\"b\"]", got)
	}

	var decoded QueryResponse[[]string]
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode into QueryResponse: %v", err)
	}
	if decoded.IndexedRevision != 42 || decoded.Producer != "graph-index" || !slices.Equal(decoded.Data, []string{"a", "b"}) {
		t.Errorf("decoded = %+v, want revision 42, producer graph-index, data [a b]", decoded)
	}
	if decoded.Timestamp.Before(before) || decoded.Timestamp.After(after) {
		t.Errorf("timestamp = %v, want the construction instant, between %v and %v", decoded.Timestamp, before, after)
	}

	// A reply built before anything is indexed still states its revision: zero is a
	// value the caller compares, not an absent field.
	zero, err := NewQueryResponse("graph-index", 0, struct{}{})
	if err != nil {
		t.Fatalf("NewQueryResponse at revision 0: %v", err)
	}
	raw, err = json.Marshal(zero)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	fields = nil
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal into a map: %v", err)
	}
	if got, ok := fields["indexed_revision"]; !ok || string(got) != "0" {
		t.Errorf("indexed_revision at revision 0 = %s (present %v), want 0 on the wire: %s", got, ok, raw)
	}
}

// TestNewQueryResponseRefusesEmptyProducer: a reply that does not name its producer
// cannot be attributed, so the constructor refuses it rather than sending one (D18).
func TestNewQueryResponseRefusesEmptyProducer(t *testing.T) {
	resp, err := NewQueryResponse("", 7, "data")
	if err == nil {
		t.Fatalf("NewQueryResponse(\"\") = %+v, nil; want a refusal", resp)
	}
	if resp != (QueryResponse[string]{}) {
		t.Errorf("refused response = %+v, want the zero value", resp)
	}
}
