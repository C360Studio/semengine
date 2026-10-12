package kvcatalog

import "github.com/nats-io/nats.go/jetstream"

// IsKVTombstone reports whether an authoritative KV watch entry removes the
// current value. NATS emits both DEL and PURGE tombstones with empty payloads;
// neither is an entity document and both must drive identical cleanup paths.
func IsKVTombstone(operation jetstream.KeyValueOp) bool {
	return operation == jetstream.KeyValueDelete || operation == jetstream.KeyValuePurge
}
