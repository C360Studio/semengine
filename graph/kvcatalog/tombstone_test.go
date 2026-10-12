package kvcatalog

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

// TestIsKVTombstone: both NATS removal markers are tombstones and a value write is not, so a
// watcher drives one cleanup path for DEL and PURGE.
func TestIsKVTombstone(t *testing.T) {
	for _, tc := range []struct {
		op   jetstream.KeyValueOp
		want bool
	}{
		{jetstream.KeyValueDelete, true},
		{jetstream.KeyValuePurge, true},
		{jetstream.KeyValuePut, false},
	} {
		if got := IsKVTombstone(tc.op); got != tc.want {
			t.Errorf("IsKVTombstone(%s) = %v, want %v", tc.op, got, tc.want)
		}
	}
}
