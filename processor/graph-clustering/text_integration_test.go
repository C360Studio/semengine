//go:build integration

package graphclustering

import (
	"fmt"
	"slices"
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/graph/clustering"
	"github.com/c360studio/semengine/graph/kvcatalog"
	"github.com/c360studio/semengine/natsclient"
)

// commentedEntity is a canonical ENTITY_STATES value for id holding a name, which every
// suffix list here selects, and a doc comment, which the default list does not.
func commentedEntity(id string) []byte {
	triple := func(predicate, object string) string {
		return `{"subject":"` + id + `","predicate":"` + predicate + `","object":"` + object +
			`","source":"test","timestamp":"2026-11-10T12:00:00Z"}`
	}
	return []byte(`{"id":"` + id + `","triples":[` + triple("code.func.name", "parse") + `,` +
		triple("code.doc.comment", "decoder") + `]}`)
}

// keywords reads every COMMUNITY_INDEX record's keywords through a reader of the test's own,
// keyed "<level>.<group ID>".
func keywords(t *testing.T, client *natsclient.Client) map[string][]string {
	t.Helper()
	reader, err := kvcatalog.OpenCatalogReader(t.Context(), client, graph.BucketCommunityIndex)
	if err != nil {
		t.Fatalf("open %s: %v", graph.BucketCommunityIndex, err)
	}
	keys, err := natsclient.FilteredKeys(t.Context(), reader, ">")
	if err != nil {
		t.Fatalf("list %s: %v", graph.BucketCommunityIndex, err)
	}
	out := map[string][]string{}
	for _, k := range keys {
		entry, err := reader.Get(t.Context(), k)
		if err != nil {
			t.Fatalf("get %s: %v", k, err)
		}
		record, err := clustering.DecodeCommunity(k, entry.Value())
		if err != nil {
			t.Fatalf("decode %s: %v", k, err)
		}
		out[fmt.Sprintf("%d.%s", record.Level, record.ID)] = record.Keywords
	}
	return out
}

// TestTextSuffixesIsRead: graph-clustering counts keywords from the values text_suffixes selects.
// With .comment added, the words of a code.doc.comment value are keywords; with the key left out,
// the default list applies and they are not. The component is built from its configuration
// through CreateGraphClustering and reads a real ENTITY_STATES.
func TestTextSuffixesIsRead(t *testing.T) {
	for raw, want := range map[string][]string{
		`{"detection_interval": "1h"}`:                                         {"parse"},
		`{"detection_interval": "1h", "text_suffixes": [".name", ".comment"]}`: {"decoder", "parse"},
	} {
		t.Run(raw, func(t *testing.T) {
			client := fixtureClient(t)
			c, states := startClustering(t, client, raw)
			const id = "acme.ops.test.code.func.f1"
			rev := put(t, states, id, commentedEntity(id))
			awaitApplied(t, c, rev)
			c.tick(t.Context())

			got := keywords(t, client)
			if len(got) != 3 {
				t.Fatalf("records = %v; want the entity's three groups", got)
			}
			for key, words := range got {
				if !slices.Equal(words, want) {
					t.Fatalf("%s keywords = %q; want %q", key, words, want)
				}
			}
		})
	}
}
