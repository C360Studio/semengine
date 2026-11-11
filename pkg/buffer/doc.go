// Package buffer provides a generic, thread-safe, fixed-size buffer with a choice of what happens when it is full.
//
// Import it as github.com/c360studio/semengine/pkg/buffer. NewCircularBuffer builds a Buffer[T] from a
// capacity and options:
//   - WithOverflowPolicy picks DropOldest (the default), DropNewest or Block when the buffer is full.
//   - WithMetrics also publishes the counters on a metric.MetricsRegistry as semengine_buffer_* series.
//   - WithDropCallback is told about every item the buffer drops.
//
// Under Block, Write waits without limit for space. WriteWithTimeout and WriteWithContext, both methods of
// Buffer[T], bound that wait. Statistics are always collected: Buffer.Stats never returns nil.
//
// All buffer implementations are safe for concurrent use.
package buffer
