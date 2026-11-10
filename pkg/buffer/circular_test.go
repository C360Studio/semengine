package buffer

import (
	"context"
	"testing"

	cerrs "github.com/c360studio/semengine/pkg/errs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contextWriter is the bounded write a caller reaches by asserting on a Buffer[T] value, as
// SemSource's local timeoutWriter does; it holds before and after R11 puts the method on Buffer[T].
type contextWriter[T any] interface {
	WriteWithContext(ctx context.Context, item T) error
}

// TestWriteWithContextRefusesNilContext: WriteWithContext(nil, item) returns an ErrorInvalid
// error under every policy and writes nothing (design R5). Oracle: an empty buffer with room,
// so any write would show as Size 1 and one counted write.
func TestWriteWithContextRefusesNilContext(t *testing.T) {
	for _, policy := range []OverflowPolicy{DropOldest, DropNewest, Block} {
		t.Run(policy.String(), func(t *testing.T) {
			buf, err := NewCircularBuffer[int](1, WithOverflowPolicy[int](policy))
			require.NoError(t, err)
			w, ok := buf.(contextWriter[int])
			require.True(t, ok, "the buffer offers WriteWithContext")

			// The nil context is the input under test.
			err = w.WriteWithContext(nil, 7)
			require.Error(t, err, "a nil context is refused")
			assert.True(t, cerrs.IsInvalid(err), "classified Invalid: %v", err)
			assert.Zero(t, buf.Size(), "nothing written")
			assert.Zero(t, buf.Stats().Writes(), "no write counted")
		})
	}
}

// TestDropCallbackMayUseBuffer: a drop callback runs after the buffer's lock is released, so
// it may call a buffer method, under DropOldest, DropNewest and Clear (design R6). Oracle: the
// dropped items and the size the callback sees are fixed by each script, counted by hand.
// On a buffer that runs the callback under its lock, the callback's Size call never returns
// and the test hangs until go test -timeout.
func TestDropCallbackMayUseBuffer(t *testing.T) {
	cases := []struct {
		name        string
		policy      OverflowPolicy
		capacity    int
		run         func(Buffer[int])
		wantDropped []int
		wantSizes   []int
	}{
		{
			name: "DropOldest", policy: DropOldest, capacity: 1,
			run:         func(b Buffer[int]) { _ = b.Write(1); _ = b.Write(2) },
			wantDropped: []int{1}, wantSizes: []int{1},
		},
		{
			name: "DropNewest", policy: DropNewest, capacity: 1,
			run:         func(b Buffer[int]) { _ = b.Write(1); _ = b.Write(2) },
			wantDropped: []int{2}, wantSizes: []int{1},
		},
		{
			name: "Clear", policy: DropOldest, capacity: 2,
			run:         func(b Buffer[int]) { _ = b.Write(1); _ = b.Write(2); b.Clear() },
			wantDropped: []int{1, 2}, wantSizes: []int{0, 0},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				buf     Buffer[int]
				dropped []int
				sizes   []int
			)
			// The callback runs on the writing goroutine, so its appends are ordered before
			// run returns; no other goroutine touches dropped or sizes.
			onDrop := func(item int) {
				dropped = append(dropped, item)
				sizes = append(sizes, buf.Size())
			}
			var err error
			buf, err = NewCircularBuffer[int](tc.capacity,
				WithOverflowPolicy[int](tc.policy), WithDropCallback[int](onDrop))
			require.NoError(t, err)

			tc.run(buf)

			assert.Equal(t, tc.wantDropped, dropped)
			assert.Equal(t, tc.wantSizes, sizes)
		})
	}
}
