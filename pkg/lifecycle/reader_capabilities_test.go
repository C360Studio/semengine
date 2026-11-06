package lifecycle

import (
	"context"

	"github.com/nats-io/nats.go/jetstream"
)

type minimalEntityStatesReader struct{}

func (minimalEntityStatesReader) WatchFiltered(context.Context, []string, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, nil
}

func (minimalEntityStatesReader) Watch(context.Context, string, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return nil, nil
}

var (
	_ entityStatesReader = minimalEntityStatesReader{}
	_ interface {
		WatchFiltered(context.Context, []string, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error)
		Watch(context.Context, string, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error)
	} = (entityStatesReader)(nil)
)
