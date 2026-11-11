# Buffer

A fixed-size, thread-safe, generic buffer. You choose what a write does when the buffer is full: drop the oldest
item, drop the new item, or wait for space. Counters for writes, reads and drops are always kept, and can also be
published as Prometheus metrics.

```go
import "github.com/c360studio/semengine/pkg/buffer"
```

## Quick start

`NewCircularBuffer` takes a capacity and options, and returns the buffer and an error. The error is non-nil only when
metrics registration fails; the buffer is then a true nil. A capacity below 1 becomes 1.

```go
buf, err := buffer.NewCircularBuffer[int](1000)
if err != nil {
    return err
}
defer buf.Close()

if err := buf.Write(42); err != nil {
    return err
}

item, ok := buf.Read() // ok is false when the buffer is empty
batch := buf.ReadBatch(10)
```

`Write` returns an error only after `Close`: an `errs.ErrorInvalid` error wrapping `errs.ErrAlreadyStopped`.

## Overflow policies

Pick one with `WithOverflowPolicy`. The default is `DropOldest`.

| Policy | A write to a full buffer | Use it when |
| --- | --- | --- |
| `DropOldest` | removes the oldest item, stores the new one | the newest data matters most, such as telemetry |
| `DropNewest` | discards the new item and returns nil | the items already queued must stay, such as an audit log |
| `Block` | waits until a read frees space or the buffer closes | no item may be lost |

```go
buf, err := buffer.NewCircularBuffer[string](100,
    buffer.WithOverflowPolicy[string](buffer.Block),
)
```

Each item a drop policy discards counts as one overflow and one drop in the statistics. `Clear` empties the buffer
without counting drops, but it does call the drop callback for each item it removes.

### Bounding a blocked write

Under `Block`, `Write` waits with no limit. `WriteWithTimeout` and `WriteWithContext` are methods of `Buffer[T]`, so
you call them on the value `NewCircularBuffer` returns. If the buffer is still full when the timeout passes or the
context ends, they write nothing and return `context.DeadlineExceeded` or `ctx.Err()`. Under the other two policies
they behave as `Write`.

```go
err := buf.WriteWithTimeout(item, 5*time.Second)

ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
err = buf.WriteWithContext(ctx, item)
```

Two edges:

- A timeout of zero or less, and a context that has already ended, refuse under `Block` even when there is space.
  Keep the timeout positive.
- `WriteWithContext` refuses a nil context with an `errs.ErrorInvalid` error under every policy.

A runnable version is `ExampleNewCircularBuffer_writeWithTimeout` in `example_test.go`.

## Options

| Option | Effect |
| --- | --- |
| `WithOverflowPolicy[T](policy)` | what a write to a full buffer does; default `DropOldest` |
| `WithMetrics[T](registry, prefix)` | also publish the counters on a `*metric.MetricsRegistry`; a nil registry or empty prefix records no metrics |
| `WithDropCallback[T](fn)` | call `fn` with each dropped item |

The drop callback runs on the goroutine that dropped the item, after the buffer's lock is released, so it may call
any buffer method.

### Metrics

```go
import "github.com/c360studio/semengine/metric"

registry := metric.NewMetricsRegistry()

buf, err := buffer.NewCircularBuffer[[]byte](5000,
    buffer.WithOverflowPolicy[[]byte](buffer.DropOldest),
    buffer.WithMetrics[[]byte](registry, "network_buffer"),
)
```

| Metric | Type | Counts |
| --- | --- | --- |
| `semengine_buffer_writes_total` | counter | items written |
| `semengine_buffer_reads_total` | counter | items read |
| `semengine_buffer_peeks_total` | counter | peeks |
| `semengine_buffer_overflows_total` | counter | writes that found the buffer full under a drop policy |
| `semengine_buffer_drops_total` | counter | items dropped |
| `semengine_buffer_size` | gauge | items in the buffer now |
| `semengine_buffer_utilization` | gauge | size divided by capacity, 0 to 1 |

Every series carries the label `component=<prefix>`. Two buffers given the same prefix share one set of series, so
give each buffer its own prefix.

## Buffer interface

```go
type Buffer[T any] interface {
    Write(item T) error
    Read() (T, bool)
    ReadBatch(maxN int) []T
    Peek() (T, bool)
    Size() int
    Capacity() int
    IsFull() bool
    IsEmpty() bool
    Clear()
    Stats() *Statistics // never nil
    Close() error
    WriteWithTimeout(item T, timeout time.Duration) error
    WriteWithContext(ctx context.Context, item T) error
}
```

A type that implements `Buffer[T]` without embedding it must have both bounded-write methods. A wrapper that embeds
`Buffer[T]` gets them from the embedded value.

## Statistics

`buf.Stats()` returns counters that are always on, whether or not metrics are enabled:

```go
stats := buf.Stats()

stats.Writes()         // items accepted; a write DropNewest rejects is not one
stats.Reads()          // total reads
stats.Peeks()          // total peeks
stats.Drops()          // total items dropped
stats.Overflows()      // total overflow events
stats.CurrentSize()    // items now
stats.MaxSize()        // highest size seen
stats.Throughput()     // writes per second since start
stats.ReadThroughput() // reads per second since start
stats.DropRate()       // drops divided by accepted writes, no upper bound
stats.OverflowRate()   // overflows divided by accepted writes, no upper bound
stats.Utilization(cap) // size divided by cap
stats.Summary()        // all of the above as a StatsSummary, for JSON
```

## Thread safety

Any number of goroutines may write and read at once. Under `Block`, `Close` wakes every waiting writer, and each
returns an `errs.ErrorInvalid` error wrapping `errs.ErrAlreadyStopped`.

## Moving from SemStreams

See [MIGRATION.md](MIGRATION.md).
