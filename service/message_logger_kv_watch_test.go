package service

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
)

// TestIsValidBucketName covers the one bucket-name check both KV routes use, including a bare "*",
// ">" and "/".
func TestIsValidBucketName(t *testing.T) {
	tests := []struct {
		name     string
		bucket   string
		expected bool
	}{
		{"valid uppercase", "ENTITY_STATES", true},
		{"valid with underscore", "MY_BUCKET", true},
		{"valid with hyphen", "my-bucket", true},
		{"valid lowercase", "mybucket", true},
		{"valid mixed case", "MyBucket", true},
		{"valid with numbers", "bucket123", true},

		{"empty bucket", "", false},
		{"wildcard >", "ENTITY>", false},
		{"wildcard *", "ENTITY*", false},
		{"contains >", "my>bucket", false},
		{"contains *", "my*bucket", false},
		{"bare *", "*", false},
		{"bare >", ">", false},
		{"path traversal ..", "..", false},
		{"path traversal /", "a/b", false},
		{"bare /", "/", false},
		{"path traversal \\", "a\\b", false},
		{"single dot", ".", false},
		{"starts with /", "/bucket", false},
		{"ends with /", "bucket/", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isValidBucketName(tt.bucket), "bucket: %q", tt.bucket)
		})
	}
}

// TestIsValidWatchPattern: an empty pattern or "*" watches every key; any other pattern is a NATS
// KV wildcard filter (natsclient.ValidateKVWildcardFilter: tokens split on '.', none empty, '*'
// only as a whole token, '>' only as the whole final token, literal tokens from letters, digits,
// '-', '/', '_', '='). The expected answers are written from that grammar, not computed by the
// validator. A final '>' is accepted, '/' is a literal key character, and a space is refused.
func TestIsValidWatchPattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		expected bool
	}{
		{"empty pattern watches every key", "", true},
		{"bare * watches every key", "*", true},
		{"prefix wildcard", "entity.*", true},
		{"leading wildcard", "*.json", true},
		{"middle wildcard", "prefix.*.suffix", true},
		{"exact key", "exact.key.name", true},
		{"many tokens", "a.b.c.d", true},
		{"final multi-level wildcard", "entity.>", true},
		{"multi-level wildcard only", ">", true},
		{"slash is a key character", "a/b", true},
		{"key punctuation", "a-b_c=d", true},

		{"multi-level wildcard not last", "a.>.b", false},
		{"empty token from double dot", "a..b", false},
		{"parent path", "../secret", false},
		{"leading dot", ".a", false},
		{"trailing dot", "a.", false},
		{"backslash", "a\\b", false},
		{"space", "a b", false},
		{"partial * wildcard", "a.b*", false},
		{"partial > wildcard", "a.b>", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isValidWatchPattern(tt.pattern), "pattern: %q", tt.pattern)
		})
	}
}

// TestDetectKVOperation: the watch reports the operation NATS records for an entry, put, delete
// or purge. The bucket revision does not enter: a put is "put" at any
// revision. The integration file proves the same values on entries read from a real
// bucket (TestKVWatchStreamsInitialValuesThenChanges).
func TestDetectKVOperation(t *testing.T) {
	tests := []struct {
		op   jetstream.KeyValueOp
		want string
	}{
		{jetstream.KeyValuePut, "put"},
		{jetstream.KeyValueDelete, "delete"},
		{jetstream.KeyValuePurge, "purge"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, detectKVOperation(tt.op))
		})
	}
}
