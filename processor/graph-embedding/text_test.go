package graphembedding

import (
	"testing"

	"github.com/c360studio/semengine/graph"
	"github.com/c360studio/semengine/message"
)

// TestEmbeddingTextUnchanged: the text hop 1 hands the Worker for an entity is fixed, byte for
// byte, because every stored vector and dedup key was computed from it. The expected strings
// are written out by hand from the rule: the string values whose lower-cased predicate ends in a
// configured suffix, relationships and non-strings skipped, each value once, in triple order,
// joined by one space.
//
// Requirement: entity-text/One rule says which values are an entity's text
func TestEmbeddingTextUnchanged(t *testing.T) {
	const subject = "acme.ops.robotics.gcs.document.001"
	triple := func(predicate string, object any) message.Triple {
		return message.Triple{Subject: subject, Predicate: predicate, Object: object}
	}
	cases := []struct {
		name    string
		config  string
		triples []message.Triple
		want    string
	}{
		{
			name:    "title and summary",
			config:  `{}`,
			triples: []message.Triple{triple("doc.meta.title", "Quarterly report"), triple("doc.meta.summary", "revenue up")},
			want:    "Quarterly report revenue up",
		},
		{
			name:   "triple order, not suffix order",
			config: `{}`,
			triples: []message.Triple{
				triple("doc.meta.summary", "first"), triple("doc.meta.count", "3"), triple("doc.meta.title", "second"),
				triple("fleet.identity.name", "third"),
			},
			want: "first second third",
		},
		{
			name:   "relationships, non-strings and empty strings skipped",
			config: `{}`,
			triples: []message.Triple{
				triple("doc.meta.name", "acme.ops.robotics.gcs.drone.001"),
				triple("doc.meta.title", 42),
				triple("doc.meta.description", ""),
				triple("doc.meta.text", "kept"),
			},
			want: "kept",
		},
		{
			name:   "an entity-ID shape with a literal datatype is text",
			config: `{}`,
			triples: []message.Triple{{Subject: subject, Predicate: "doc.meta.title",
				Object: "acme.ops.robotics.gcs.drone.001", Datatype: "xsd:string"}},
			want: "acme.ops.robotics.gcs.drone.001",
		},
		{
			name:    "the predicate is lower-cased before the match",
			config:  `{}`,
			triples: []message.Triple{triple("Doc.Meta.TITLE", "Loud Title")},
			want:    "Loud Title",
		},
		{
			name:    "a lower-cased predicate never matches an upper-case suffix",
			config:  `{"text_suffixes": [".Title"]}`,
			triples: []message.Triple{triple("doc.meta.Title", "unmatched")},
			want:    "",
		},
		{
			name:    "configured suffixes replace the defaults",
			config:  `{"text_suffixes": [".signature"]}`,
			triples: []message.Triple{triple("doc.meta.title", "dropped"), triple("code.func.signature", "func main()")},
			want:    "func main()",
		},
		{
			name:    "a value two suffixes match is taken once",
			config:  `{"text_suffixes": [".title", "title"]}`,
			triples: []message.Triple{triple("doc.meta.title", "once")},
			want:    "once",
		},
		{
			name:    "every default suffix",
			config:  `{}`,
			triples: defaultSuffixTriples(subject),
			want:    "t c d s x n b a j",
		},
		{
			name:    "no text",
			config:  `{}`,
			triples: []message.Triple{triple("doc.meta.count", "3"), triple("doc.meta.subtitle", "near miss")},
			want:    "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := mustCreate(t, tc.config)
			state := graph.EntityState{ID: subject, Triples: tc.triples}
			if got := c.extractText(&state); got != tc.want {
				t.Fatalf("text = %q, want %q", got, tc.want)
			}
		})
	}
}

// defaultSuffixTriples is one triple for each of the nine default suffixes, in the order the
// defaults list them, each value one letter.
func defaultSuffixTriples(subject string) []message.Triple {
	pairs := [][2]string{
		{"doc.meta.title", "t"}, {"doc.meta.content", "c"}, {"doc.meta.description", "d"},
		{"doc.meta.summary", "s"}, {"doc.meta.text", "x"}, {"doc.meta.name", "n"}, {"doc.meta.body", "b"},
		{"doc.meta.abstract", "a"}, {"doc.meta.subject", "j"},
	}
	triples := make([]message.Triple, 0, len(pairs))
	for _, p := range pairs {
		triples = append(triples, message.Triple{Subject: subject, Predicate: p[0], Object: p[1]})
	}
	return triples
}
