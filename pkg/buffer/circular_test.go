package buffer

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"testing/synctest"
	"time"

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

// bubbleGoroutinesIn counts goroutines in the calling synctest bubble whose stack holds frame.
// The caller runs synctest.Wait first, so every goroutine that is going to exit has exited.
func bubbleGoroutinesIn(frame string) int {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	n := 0
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "synctest bubble") && strings.Contains(g, frame) {
			n++
		}
	}
	return n
}

// bufferFrame is in the stack of every goroutine running buffer code.
const bufferFrame = "pkg/buffer.(*circularBuffer"

// fullBlockBuffer returns a Block buffer of capacity 2 holding 1 and 2.
func fullBlockBuffer(t *testing.T) Buffer[int] {
	t.Helper()
	buf, err := NewCircularBuffer[int](2, WithOverflowPolicy[int](Block))
	require.NoError(t, err)
	require.NoError(t, buf.Write(1))
	require.NoError(t, buf.Write(2))
	return buf
}

// drain reads every item, oldest first.
func drain(buf Buffer[int]) []int {
	var out []int
	for {
		v, ok := buf.Read()
		if !ok {
			return out
		}
		out = append(out, v)
	}
}

// timeoutWriter is SemSource's local interface (semsource internal/entitypub/publisher.go:251),
// with its item type made a parameter.
type timeoutWriter[T any] interface {
	WriteWithTimeout(item T, timeout time.Duration) error
}

// embeddingWrapper embeds Buffer[T], as a consumer's decorator does.
type embeddingWrapper[T any] struct {
	Buffer[T]
}

// TestWriteWithTimeoutAssertable: the bounded writes are part of Buffer[T], so SemSource's
// timeoutWriter matches both a Buffer[T] value and a type embedding Buffer[T], and a call
// through each writes (design R11). Oracle: the items read back.
func TestWriteWithTimeoutAssertable(t *testing.T) {
	buf, err := NewCircularBuffer[int](4, WithOverflowPolicy[int](Block))
	require.NoError(t, err)

	for _, v := range []any{buf, embeddingWrapper[int]{Buffer: buf}} {
		tw, ok := v.(timeoutWriter[int])
		require.True(t, ok, "%T matches timeoutWriter", v)
		require.NoError(t, tw.WriteWithTimeout(1, time.Second))
		cw, ok := v.(contextWriter[int])
		require.True(t, ok, "%T matches contextWriter", v)
		require.NoError(t, cw.WriteWithContext(t.Context(), 2))
	}
	assert.Equal(t, []int{1, 2, 1, 2}, drain(buf))
}

// TestWriteWithTimeoutExpires: on a full Block buffer WriteWithTimeout is still waiting just
// before its timeout, returns context.DeadlineExceeded itself once the timeout has passed, and
// writes nothing (design R3). Oracle: the bubble's fake clock, advanced by the test; the
// buffer's contents and write count from before the call.
func TestWriteWithTimeoutExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const timeout = 100 * time.Millisecond
		buf := fullBlockBuffer(t)
		result := make(chan error, 1)
		go func() { result <- buf.WriteWithTimeout(3, timeout) }()

		<-time.After(timeout - time.Nanosecond)
		synctest.Wait() // the writer has run as far as it can at this instant
		select {
		case err := <-result:
			t.Fatalf("returned %v before its timeout", err)
		default:
		}

		<-time.After(time.Nanosecond)
		err := <-result
		assert.Equal(t, context.DeadlineExceeded, err, "context.DeadlineExceeded itself")
		assert.Equal(t, int64(2), buf.Stats().Writes(), "nothing written")
		assert.Equal(t, []int{1, 2}, drain(buf))
	})
}

// TestWriteWithTimeoutSucceedsWhenSpaceFrees: a WriteWithTimeout waiting on a full Block buffer
// writes as soon as a read frees space, before its timeout (design R3). Oracle: the result is
// in hand with no fake time passed, and the contents read back.
func TestWriteWithTimeoutSucceedsWhenSpaceFrees(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf := fullBlockBuffer(t)
		start := time.Now()
		result := make(chan error, 1)
		go func() { result <- buf.WriteWithTimeout(3, time.Hour) }()
		synctest.Wait() // the writer is waiting for space

		v, ok := buf.Read()
		require.True(t, ok)
		require.Equal(t, 1, v)
		synctest.Wait() // the writer has run as far as it can after the read
		select {
		case err := <-result:
			require.NoError(t, err)
		default:
			t.Fatal("the write is still waiting after a read freed space")
		}
		assert.Zero(t, time.Since(start), "no fake time passed")
		assert.Equal(t, []int{2, 3}, drain(buf))
	})
}

// TestWriteWithContextLeavesNothing: whichever way WriteWithContext returns (written at once,
// written after a read, cancelled, deadline, closed), no goroutine it started is still running
// (design R4). Oracle: after synctest.Wait, no goroutine in the bubble runs buffer code.
func TestWriteWithContextLeavesNothing(t *testing.T) {
	cases := []struct {
		name    string
		full    bool
		release func(buf Buffer[int], cancel context.CancelFunc)
		check   func(t *testing.T, err error)
	}{
		{"written at once", false, nil, func(t *testing.T, err error) { require.NoError(t, err) }},
		{"written after a read", true, func(buf Buffer[int], _ context.CancelFunc) { _, _ = buf.Read() },
			func(t *testing.T, err error) { require.NoError(t, err) }},
		{"cancelled", true, func(_ Buffer[int], cancel context.CancelFunc) { cancel() },
			func(t *testing.T, err error) { require.ErrorIs(t, err, context.Canceled) }},
		{"deadline", true, nil, func(t *testing.T, err error) { require.ErrorIs(t, err, context.DeadlineExceeded) }},
		{"closed", true, func(buf Buffer[int], _ context.CancelFunc) { _ = buf.Close() },
			func(t *testing.T, err error) { require.ErrorIs(t, err, cerrs.ErrAlreadyStopped) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				buf, err := NewCircularBuffer[int](2, WithOverflowPolicy[int](Block))
				require.NoError(t, err)
				require.NoError(t, buf.Write(1))
				if tc.full {
					require.NoError(t, buf.Write(2))
				}
				ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
				defer cancel()

				result := make(chan error, 1)
				go func() { result <- buf.WriteWithContext(ctx, 3) }()
				synctest.Wait() // the writer has returned or is waiting
				if tc.release != nil {
					tc.release(buf, cancel)
				}
				tc.check(t, <-result)

				synctest.Wait() // every goroutine that is going to exit has exited
				assert.Zero(t, bubbleGoroutinesIn(bufferFrame), "a goroutine WriteWithContext started still runs")
			})
		})
	}
}

// TestCapacityBelowOneIsOne: a capacity below 1 becomes 1 (design R9). Oracle: Capacity, and
// under DropOldest a second write evicting the first.
func TestCapacityBelowOneIsOne(t *testing.T) {
	for _, capacity := range []int{0, -3} {
		buf, err := NewCircularBuffer[int](capacity)
		require.NoError(t, err)
		assert.Equal(t, 1, buf.Capacity(), "capacity %d", capacity)
		require.NoError(t, buf.Write(1))
		require.NoError(t, buf.Write(2))
		assert.Equal(t, []int{2}, drain(buf), "capacity %d holds one item", capacity)
	}
}
