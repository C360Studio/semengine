package embedding

import (
	"strings"
	"testing"

	"github.com/c360studio/semengine/storage"
)

// TestGetSourceTextReportsCapCut: getSourceText reports that the cap cut the text on both lanes,
// whether the cut fell in an offloaded body or in the identity and body joined, so the Worker can
// count an entity cut at the cap once and leave the provider's mark on it uncounted. Each case's
// text is built longer or shorter than the cap of 20 runes.
func TestGetSourceTextReportsCapCut(t *testing.T) {
	store := &mockStreamableStore{data: map[string][]byte{
		"long":  []byte(strings.Repeat("word ", 10)), // 50 runes
		"short": []byte("body text"),                 // 9 runes
	}}
	w := &Worker{
		storeResolver:    fakeResolver{stores: map[string]storage.StreamableStore{"content": store}},
		maxSourceTextLen: 20,
	}
	ref := func(key string) *StorageRef { return &StorageRef{StorageInstance: "content", Key: key} }
	for _, tc := range []struct {
		name   string
		record *Record
		want   bool
	}{
		{"inline over the cap", &Record{SourceText: strings.Repeat("word ", 10)}, true},
		{"inline within the cap", &Record{SourceText: "short title"}, false},
		{"offloaded body over the cap", &Record{StorageRef: ref("long")}, true},
		{"offloaded identity and body over the cap", &Record{IdentityText: "a fifteen title", StorageRef: ref("short")}, true},
		{"offloaded within the cap", &Record{IdentityText: "title", StorageRef: ref("short")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, capCut, err := w.getSourceText(t.Context(), tc.record)
			if err != nil {
				t.Fatalf("getSourceText: %v", err)
			}
			if capCut != tc.want {
				t.Fatalf("capCut = %v, want %v", capCut, tc.want)
			}
		})
	}
}
