package embedding

import (
	"math"
	"testing"

	"github.com/c360studio/semengine/storage"
)

// TestFetchTextFromStorage_PathologicalCapDoesNotReturnEmpty is #628 FIX 2 at the read
// seam: a pathological max_text_len must not overflow the LimitReader bound into a
// negative value, which reads an empty body that hop 2 then treats as "no source text"
// and DELETES the pending embedding.
//
// Fails without the fix because fetchTextFromStorage read int64(limit)+1 bytes:
// with limit == math.MaxInt64 the +1 overflows to a negative bound, io.LimitReader
// yields an empty body, and non-empty content reads back as "".
func TestFetchTextFromStorage_PathologicalCapDoesNotReturnEmpty(t *testing.T) {
	t.Parallel()

	const body = "real body content that must survive a pathological cap"
	w := &Worker{
		maxSourceTextLen: math.MaxInt64, // overflows int64(limit)+1 without the clamp
		storeResolver: fakeResolver{stores: map[string]storage.StreamableStore{
			"objectstore": readerStore{data: body},
		}},
	}

	got, _, err := w.fetchTextFromStorage(t.Context(), &StorageRef{StorageInstance: "objectstore", Key: "k"})
	if err != nil {
		t.Fatalf("fetchTextFromStorage: %v", err)
	}
	if got == "" {
		t.Fatal("pathological cap returned EMPTY for non-empty content: the read bound overflowed negative and hop 2 would delete the pending embedding (#628 FIX 2)")
	}
	if got != body {
		t.Fatalf("got %q, want the full body %q", got, body)
	}
}
