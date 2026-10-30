package graphview

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

// heldWatchSource holds WatchAll until the test releases it, after the watch's context has
// ended, then hands back a watch that opened late.
type heldWatchSource struct {
	release chan struct{}
	watcher *fakeWatcher
}

func (s *heldWatchSource) WatchAll(ctx context.Context, _ ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	<-ctx.Done()
	<-s.release
	context.AfterFunc(ctx, s.watcher.lose)
	return s.watcher, nil
}

// spec: graph-view-subscription / Run returns only after everything it started has ended
func TestRunJoinsWatcherAcquisition(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		source := &heldWatchSource{release: make(chan struct{}), watcher: newFakeWatcher()}
		view, err := New[string](source, decodeTest)
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- view.Run(ctx) }()
		synctest.Wait() // Run is inside WatchAll

		cancel()
		synctest.Wait() // the held WatchAll has seen its context end and waits for release
		select {
		case err := <-done:
			t.Fatalf("Run returned %v before its WatchAll returned", err)
		default:
		}

		close(source.release)
		synctest.Wait() // Run has handled the late watch and returned
		select {
		case err := <-done:
			require.ErrorIs(t, err, context.Canceled)
		default:
			t.Fatal("Run did not return after its WatchAll returned")
		}
		require.Equal(t, int32(1), source.watcher.stops.Load(), "the late watch is stopped before Run returns")
		_, _, err = view.Get("k")
		require.ErrorIs(t, err, ErrViewStopped)
	})
}

// spec: graph-view-subscription / Run returns only after everything it started has ended
func TestRunJoinsSubscriberCallback(t *testing.T) {
	for _, snapshot := range []bool{false, true} {
		t.Run(map[bool]string{false: "subscribe", true: "snapshot"}[snapshot], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				watcher := newFakeWatcher()
				entered, release := make(chan struct{}), make(chan struct{})
				var enterOnce sync.Once
				view, err := New[string](&fakeSource{queue: []*fakeWatcher{watcher}}, decodeTest, WithHooks(Hooks{
					OnSubscribers: func(n int) {
						if n == 1 {
							enterOnce.Do(func() { close(entered) })
							<-release
						}
					},
				}))
				require.NoError(t, err)
				ctx, cancel := context.WithCancel(t.Context())
				done := make(chan error, 1)
				go func() { done <- view.Run(ctx) }()
				watcher.updates <- nil
				require.NoError(t, view.WaitCaughtUp(t.Context()))

				attached := make(chan struct{})
				go func() {
					if snapshot {
						_, _, _ = view.SnapshotAndSubscribe(t.Context())
					} else {
						_, _ = view.Subscribe(t.Context())
					}
					close(attached)
				}()
				<-entered
				cancel()
				synctest.Wait() // Run has stopped the view and waits on the attach's hook call
				select {
				case err := <-done:
					t.Fatalf("Run returned %v before the attach's OnSubscribers call returned", err)
				default:
				}

				close(release)
				<-attached
				require.ErrorIs(t, <-done, context.Canceled)
			})
		})
	}
}
