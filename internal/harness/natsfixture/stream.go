package natsfixture

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Every stream CreateStream creates declares these bounds, so a runaway test fails on a ceiling
// instead of filling the server (SemStreams test_client.go:911-927).
const (
	streamMaxAge   = time.Hour
	streamMaxBytes = 64 << 20
)

// CreateStream creates a stream on js with the given storage, a MaxAge of one hour, a MaxBytes of
// 64 MiB and DiscardOld. On a fixture, a file-backed stream keeps its messages across Restart and a
// memory-backed one does not; Stop's removal of the container removes both.
func CreateStream(ctx context.Context, js jetstream.JetStream, storage jetstream.StorageType, name string, subjects ...string) (jetstream.Stream, error) {
	if ctx == nil {
		return nil, errors.New("natsfixture: CreateStream with a nil context")
	}
	if js == nil {
		return nil, errors.New("natsfixture: CreateStream with a nil JetStream; a fixture's is nil before Start and after Stop")
	}
	s, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: name, Subjects: subjects, Storage: storage,
		MaxAge: streamMaxAge, MaxBytes: streamMaxBytes, Discard: jetstream.DiscardOld,
	})
	if err != nil {
		return nil, fmt.Errorf("natsfixture: create stream %s: %w", name, err)
	}
	return s, nil
}
