package buffer_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/c360studio/semengine/pkg/buffer"
)

// A write under the Block policy waits for space. WriteWithTimeout limits that wait and, when the buffer
// stays full, returns context.DeadlineExceeded without writing the item.
func ExampleNewCircularBuffer_writeWithTimeout() {
	buf, err := buffer.NewCircularBuffer(1, buffer.WithOverflowPolicy[string](buffer.Block))
	if err != nil {
		fmt.Println("create:", err)
		return
	}
	defer buf.Close()

	if err := buf.Write("first"); err != nil {
		fmt.Println("write:", err)
		return
	}

	// The buffer holds one item and nothing reads it, so this write times out.
	err = buf.WriteWithTimeout("second", 10*time.Millisecond)
	fmt.Println(errors.Is(err, context.DeadlineExceeded), buf.Size())

	// Output: true 1
}
