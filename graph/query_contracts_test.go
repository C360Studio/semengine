package graph

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// mustMarshal fails the test rather than returning an error, so a table entry
// that stops marshalling is a test failure and not a silently skipped case.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	return b
}

// TestQueryResponseCarriesIndexedRevisionAndProducer is design D18's test: a
// reply on graph.query.* says which component answered and which ENTITY_STATES
// revision the answer reflects, and both survive the wire.
func TestQueryResponseCarriesIndexedRevisionAndProducer(t *testing.T) {
	const producer = "graph-query-1"
	const revision uint64 = 42
	data := SummaryData{TotalEntities: 3}

	resp, err := NewQueryResponse(data, producer, revision)
	if err != nil {
		t.Fatalf("NewQueryResponse(%q, %d): %v", producer, revision, err)
	}
	raw := mustMarshal(t, resp)

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v\n  got: %s", err, raw)
	}
	for _, key := range []string{"data", "timestamp"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("JSON has no %q key\n  got: %s", key, raw)
		}
	}
	if got := string(fields["indexed_revision"]); got != "42" {
		t.Errorf("indexed_revision = %s, want 42\n  got: %s", got, raw)
	}
	if got := string(fields["producer"]); got != `"graph-query-1"` {
		t.Errorf(`producer = %s, want "graph-query-1"`+"\n  got: %s", got, raw)
	}

	var decoded QueryResponse[SummaryData]
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode: %v\n  got: %s", err, raw)
	}
	if !reflect.DeepEqual(decoded.Data, data) || decoded.Producer != producer ||
		decoded.IndexedRevision != revision || !decoded.Timestamp.Equal(resp.Timestamp) {
		t.Fatalf("decoded response differs from the one encoded\n  encoded: %+v\n  decoded: %+v", resp, decoded)
	}
}

// TestNewQueryResponseRefusesEmptyProducer holds design D18: a response cannot
// be built without naming the component that answers.
func TestNewQueryResponseRefusesEmptyProducer(t *testing.T) {
	resp, err := NewQueryResponse(SummaryData{TotalEntities: 3}, "", 42)
	if !errors.Is(err, errNoProducer) {
		t.Fatalf("NewQueryResponse with an empty producer: err = %v, want %v", err, errNoProducer)
	}
	if !reflect.DeepEqual(resp, QueryResponse[SummaryData]{}) {
		t.Fatalf("refused response is not the zero value: %+v", resp)
	}
}

// TestQueryResponseDeclaresOnlyEnvelopeFields walks the struct tags, not a
// marshalled value, so a field added with `omitempty` is seen too: the
// envelope's key set is exactly design D18's.
func TestQueryResponseDeclaresOnlyEnvelopeFields(t *testing.T) {
	typeOfResponse := reflect.TypeOf(QueryResponse[struct{}]{})
	keys := make([]string, 0, typeOfResponse.NumField())
	for i := 0; i < typeOfResponse.NumField(); i++ {
		key, _, _ := strings.Cut(typeOfResponse.Field(i).Tag.Get("json"), ",")
		keys = append(keys, key)
	}
	want := []string{"data", "indexed_revision", "producer", "timestamp"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("QueryResponse JSON fields = %v, want %v", keys, want)
	}
}

// TestMinRevisionFieldEmbedsAtTheTopLevel holds design D18: a request type that
// embeds MinRevisionField carries min_revision beside its own fields, and leaves
// it out when no minimum is asked for.
func TestMinRevisionFieldEmbedsAtTheTopLevel(t *testing.T) {
	type request struct {
		MinRevisionField
		ID string `json:"id"`
	}
	for _, tc := range []struct {
		req  request
		want string
	}{
		{request{MinRevisionField{MinRevision: 7}, "a"}, `{"min_revision":7,"id":"a"}`},
		{request{ID: "a"}, `{"id":"a"}`},
	} {
		if got := string(mustMarshal(t, tc.req)); got != tc.want {
			t.Errorf("request JSON = %s, want %s", got, tc.want)
		}
	}
}

// TestQueryResponse_HasNoErrorField guards ADR-060 and task 2.5.
//
// The in-body Error field was REMOVED by ADR-060: a query reply is either this
// success body or a classified error on the err channel. The gateway carried an
// `Error string` branch for years afterward that no producer could satisfy.
// If a future change reintroduces the field, this fails and the gateway's
// deleted branch has to be reconsidered deliberately rather than by accident.
func TestQueryResponse_HasNoErrorField(t *testing.T) {
	resp, err := NewQueryResponse(SummaryData{}, "graph-query-1", 1)
	if err != nil {
		t.Fatalf("NewQueryResponse: %v", err)
	}
	raw := mustMarshal(t, resp)

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := fields["error"]; present {
		t.Fatalf("QueryResponse marshalled an \"error\" key; ADR-060 removed it. "+
			"The gateway's deleted error branch must be reconsidered.\n  got: %s", raw)
	}

	_ = fields
}
