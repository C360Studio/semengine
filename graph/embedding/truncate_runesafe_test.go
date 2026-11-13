package embedding

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/c360studio/semengine/storage"
)

// TestTruncate_CrossLaneEquivalence is the cross-lane core statement: identical
// over-cap content must embed the SAME bytes and derive the SAME hop-2 dedup key
// whether it arrives inline (SourceText) or offloaded (StorageRef → store), so the
// two lanes cannot diverge into different vectors and different dedup keys for one
// piece of content.
//
// It fails if the offloaded lane byte-cuts to exactly `limit` bytes in
// fetchTextFromStorage instead of using the inline lane's word-boundary cut — the two
// lanes would then return different strings for identical content.
func TestTruncate_CrossLaneEquivalence(t *testing.T) {
	t.Parallel()

	const capLen = 40
	// Over-cap content with spaces so the word boundary is exercised; the two lanes
	// must converge on the identical truncated string.
	const body = "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda"

	// Inline lane.
	inline := &Worker{maxSourceTextLen: capLen}
	inlineText, err := inline.getSourceText(t.Context(), &Record{SourceText: body})
	if err != nil {
		t.Fatalf("inline getSourceText: %v", err)
	}

	// Offloaded lane — same content, delivered via the store resolver.
	offloaded := &Worker{
		maxSourceTextLen: capLen,
		storeResolver:    fakeResolver{stores: map[string]storage.StreamableStore{"objectstore": readerStore{data: body}}},
	}
	offloadedText, err := offloaded.getSourceText(t.Context(), &Record{StorageRef: &StorageRef{StorageInstance: "objectstore", Key: "k"}})
	if err != nil {
		t.Fatalf("offloaded getSourceText: %v", err)
	}

	if inlineText != offloadedText {
		t.Fatalf("lanes diverged: inline %q vs offloaded %q; identical content must embed identical bytes (#628 FIX 1 / gh#627)",
			inlineText, offloadedText)
	}
	if utf8.RuneCountInString(inlineText) > capLen {
		t.Fatalf("truncated to %d runes, want at most cap %d", utf8.RuneCountInString(inlineText), capLen)
	}

	// The load-bearing consequence: identical text → identical dedup key.
	id := EmbedderIdentity{Type: "bm25", Model: "m", Dimensions: 3, MaxTextLen: capLen}
	if k1, k2 := DedupKey(id, inlineText), DedupKey(id, offloadedText); k1 != k2 {
		t.Fatalf("dedup keys differ across lanes: %s vs %s; the whole point of one truncation routine is one key", k1, k2)
	}
}

// TestTruncate_MultibyteIsRuneSafe covers the multibyte half of the cross-lane cut: a cap that
// lands mid-rune must not sever a multibyte rune. The result must be valid UTF-8, cut
// on a rune boundary, and honor characters (not bytes).
//
// It fails if truncateAtWord does text[:maxLen] on BYTES, slicing a 3-byte CJK /
// 4-byte emoji rune into invalid UTF-8 — so DedupKey would hash bytes that differ
// from the string the embedder receives.
func TestTruncate_MultibyteIsRuneSafe(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
	}{
		// 10 CJK runes (3 bytes each = 30 bytes), no spaces.
		{"cjk_no_space", strings.Repeat("世", 10)},
		// Emoji (4 bytes each) with spaces so the word boundary is also exercised.
		{"emoji_words", "😀😀😀 😁😁😁 😂😂😂 🤣🤣🤣"},
		// Mixed ASCII + CJK straddling the cap.
		{"mixed", "hello 世界 " + strings.Repeat("界", 20)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, capLen := range []int{1, 4, 7, 9} {
				got := truncateAtWord(tc.text, capLen)
				if !utf8.ValidString(got) {
					t.Fatalf("cap=%d produced invalid UTF-8 %q (a rune was severed)", capLen, got)
				}
				if n := utf8.RuneCountInString(got); n > capLen {
					t.Fatalf("cap=%d produced %d runes, want at most cap", capLen, n)
				}
				// The result must be a rune-boundary prefix of the source (possibly cut
				// back further at a whitespace boundary), never bytes the source lacks.
				if !strings.HasPrefix(tc.text, got) {
					t.Fatalf("cap=%d result %q is not a prefix of the source; it must be a clean rune-boundary cut", capLen, got)
				}
			}
		})
	}
}

// TestTruncate_UnicodeWhitespaceWordBoundary proves the word boundary uses
// unicode.IsSpace, not a literal ASCII " ". A non-breaking space (U+00A0) separating
// words must be recognized as a boundary.
//
// It fails if the boundary search is strings.LastIndex(truncated, " "), which matches
// only a literal ASCII space, so a NBSP-delimited body would hard-cut mid-"word".
func TestTruncate_UnicodeWhitespaceWordBoundary(t *testing.T) {
	t.Parallel()

	const nbsp = " "
	// "wordone<NBSP>wordtwo..." — the only separators are non-breaking spaces.
	text := "wordone" + nbsp + "wordtwo" + nbsp + "wordthree"
	got := truncateAtWord(text, 12) // 12 runes lands inside "wordtwo"

	if !utf8.ValidString(got) {
		t.Fatalf("invalid UTF-8: %q", got)
	}
	// The boundary at the NBSP after "wordone" (rune index 7) is past the halfway
	// point (12/2=6), so the routine must cut there rather than hard-cutting inside
	// "wordtwo".
	if got != "wordone" {
		t.Fatalf("truncateAtWord did not honor the non-breaking-space word boundary: got %q, want %q", got, "wordone")
	}
}
