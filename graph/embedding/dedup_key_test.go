package embedding

import "testing"

// TestDedupKey_DistinguishesEmbedderIdentity guards gh#612.
//
// EMBEDDING_DEDUP is durable, has no TTL, and is never cleared. Before this
// guard the key was SHA-256(text) alone, so an operator switching
// embedder_type bm25 -> http against the same NATS state got every
// already-embedded entity's BM25 vector back, written into EMBEDDING_INDEX
// labelled with the neural model's name. Both default to 384 dimensions, so
// CosineSimilarity produced a plausible-looking score across two unrelated
// vector spaces and every health signal stayed green while dedup_hits_total
// ROSE — the failure reads as a cost win.
//
// The property under test: identical text under any differing embedder
// identity MUST produce a different key.
func TestDedupKey_DistinguishesEmbedderIdentity(t *testing.T) {
	const text = "the quick brown fox jumps over the lazy dog"

	base := EmbedderIdentity{
		Type:       "bm25",
		Model:      "bm25-384",
		Dimensions: 384,
		MaxTextLen: 4000,
	}

	tests := []struct {
		name  string
		ident EmbedderIdentity
	}{
		{
			// The exact operator action in the report: statistical.json ->
			// semantic.json. Type, model AND cap all move together.
			name: "embedder type switch bm25 to http",
			ident: EmbedderIdentity{
				Type: "http", Model: "all-MiniLM-L6-v2", Dimensions: 384, MaxTextLen: 8000,
			},
		},
		{
			name:  "same type and dimensions, different model",
			ident: EmbedderIdentity{Type: "http", Model: "bge-small-en", Dimensions: 384, MaxTextLen: 8000},
		},
		{
			// Dimension change alone: CosineSimilarity returns 0.0 on a length
			// mismatch, so a stale hit here silently ranks the entity last forever.
			name:  "same model, different dimensions",
			ident: EmbedderIdentity{Type: "bm25", Model: "bm25-384", Dimensions: 768, MaxTextLen: 4000},
		},
		{
			// The compounding defect: the cap changes WHICH TEXT was embedded
			// while the raw input string is byte-identical.
			name:  "same model, different truncation cap",
			ident: EmbedderIdentity{Type: "bm25", Model: "bm25-384", Dimensions: 384, MaxTextLen: 8000},
		},
		{
			name:  "same everything except type label",
			ident: EmbedderIdentity{Type: "http", Model: "bm25-384", Dimensions: 384, MaxTextLen: 4000},
		},
	}

	baseKey := DedupKey(base, text)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DedupKey(tt.ident, text)
			if got == baseKey {
				t.Fatalf("dedup key collides across embedder identities: %+v and %+v both key to %s",
					base, tt.ident, got)
			}
		})
	}

	t.Run("identical identity and text is stable", func(t *testing.T) {
		if again := DedupKey(base, text); again != baseKey {
			t.Fatalf("DedupKey is not deterministic: %s then %s", baseKey, again)
		}
	})

	t.Run("different text under same identity differs", func(t *testing.T) {
		if other := DedupKey(base, text+"!"); other == baseKey {
			t.Fatal("dedup key ignores text")
		}
	})
}

// TestDedupKey_FieldsAreUnambiguous checks that field boundaries are encoded, so
// shifting a character across two adjacent fields cannot produce the same key.
// Without length prefixing, {"ab","c"} and {"a","bc"} serialize identically.
func TestDedupKey_FieldsAreUnambiguous(t *testing.T) {
	a := DedupKey(EmbedderIdentity{Type: "ab", Model: "c", Dimensions: 1, MaxTextLen: 1}, "x")
	b := DedupKey(EmbedderIdentity{Type: "a", Model: "bc", Dimensions: 1, MaxTextLen: 1}, "x")
	if a == b {
		t.Fatal("field boundaries are not encoded: adjacent fields concatenate ambiguously")
	}

	// The text field must not be able to absorb an identity field either.
	c := DedupKey(EmbedderIdentity{Type: "a", Model: "b", Dimensions: 1, MaxTextLen: 1}, "cx")
	d := DedupKey(EmbedderIdentity{Type: "a", Model: "bc", Dimensions: 1, MaxTextLen: 1}, "x")
	if c == d {
		t.Fatal("text field boundary is not encoded")
	}
}

// TestDedupKey_EmptyWhenIdentityUnresolved is carried from the pin's
// http_embedder_dimensions_test.go, which is not ported: it is the one test there that
// needs no HTTP embedder. An embedder that cannot yet state its vector width must produce
// no dedup key at all.
//
// Empty is the package's "dedup disabled for this record" signal: the Worker generates
// unconditionally for it. So the unresolved window costs redundant embeds, once, instead
// of permanently partitioning the bucket. Dropping Dimensions from the key instead would
// be worse: keys would then collide across models of different width, which is the very
// thing gh#612 folded identity in to prevent.
func TestDedupKey_EmptyWhenIdentityUnresolved(t *testing.T) {
	t.Parallel()

	unresolved := EmbedderIdentity{Type: "http", Model: "all-mpnet-base-v2", Dimensions: 0, MaxTextLen: 8000}
	if got := DedupKey(unresolved, "some entity text"); got != "" {
		t.Fatalf("DedupKey on an unresolved identity = %q, want \"\" (dedup disabled); "+
			"keying on a placeholder width is what splits the keyspace", got)
	}

	resolved := EmbedderIdentity{Type: "http", Model: "all-mpnet-base-v2", Dimensions: 768, MaxTextLen: 8000}
	if got := DedupKey(resolved, "some entity text"); got == "" {
		t.Fatal("DedupKey on a resolved identity must produce a key")
	}
}

// TestInProcessDedupKey_IgnoresWidthAndNeverMatchesDedupKey states InProcessDedupKey's
// doc: it always returns a key, the same one before and after the embedder learns its
// width, distinct from the durable DedupKey over the same fields, and it changes with
// every other field. The pin had no test of it outside the HTTP files.
func TestInProcessDedupKey_IgnoresWidthAndNeverMatchesDedupKey(t *testing.T) {
	t.Parallel()

	const text = "identical content held by two workers"
	unresolved := EmbedderIdentity{Type: "http", Model: "m", Dimensions: 0, MaxTextLen: 8000}
	resolved := unresolved
	resolved.Dimensions = 768

	key := InProcessDedupKey(unresolved, text)
	if key == "" {
		t.Fatal("InProcessDedupKey withheld a key from an unresolved identity; it must always key")
	}
	if got := InProcessDedupKey(resolved, text); got != key {
		t.Fatalf("InProcessDedupKey changed when the width resolved: %s then %s", key, got)
	}
	if got := DedupKey(resolved, text); got == InProcessDedupKey(resolved, text) {
		t.Fatal("the process-local key equals the durable DedupKey; the two must be domain-separated")
	}

	for name, other := range map[string]struct {
		id   EmbedderIdentity
		text string
	}{
		"type":     {EmbedderIdentity{Type: "bm25", Model: "m", MaxTextLen: 8000}, text},
		"model":    {EmbedderIdentity{Type: "http", Model: "n", MaxTextLen: 8000}, text},
		"text cap": {EmbedderIdentity{Type: "http", Model: "m", MaxTextLen: 4000}, text},
		"text":     {unresolved, text + "!"},
	} {
		if InProcessDedupKey(other.id, other.text) == key {
			t.Errorf("InProcessDedupKey ignores the %s", name)
		}
	}
}
