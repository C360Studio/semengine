# natsclient

NATS client with circuit breaker protection, automatic reconnection, and comprehensive JetStream/KV support for
distributed edge systems.

## Overview

The natsclient package provides a robust NATS client implementation designed for resilient operation in edge computing
environments. It wraps the standard NATS Go client with additional reliability features including circuit breaker
pattern for failure protection, exponential backoff for reconnection, and proper context propagation throughout all
operations.

This package is the foundation for all NATS communication in the SemStreams framework, providing both core pub/sub
functionality and advanced features like JetStream streams, consumers, and Key-Value stores. The client handles
connection lifecycle management automatically, allowing applications to focus on business logic rather than connection
state management.

## Installation

```go
import "github.com/c360studio/semengine/natsclient"
```

## Core Concepts

### Circuit Breaker Pattern

The client implements a circuit breaker that prevents cascading failures by failing fast after a threshold of
consecutive failures (default: 15). The circuit opens to prevent further attempts, then gradually tests the connection
with exponential backoff.

### Connection Lifecycle

Connection states transition through: Disconnected → Connecting → Connected → Reconnecting (on failure) → Connected.
The client handles all transitions automatically.

### KVStore Abstraction

A high-level abstraction over NATS KV providing automatic CAS (Compare-And-Swap) retry logic, JSON helpers, and
consistent error handling for configuration management scenarios.

### Consumer contexts and handler panics

`ConsumeStreamWithConfig` and `ConsumeInternalStreamWithConfig` take one context, and it does two jobs: it bounds the
consumer's setup, and it is the parent of every handler's context for as long as the consumer runs. A context with a
deadline sized for setup therefore ends the context of every handler invoked after that deadline; delivery itself
continues. `ConsumeStreamWithConfigContexts` takes the two separately: `setupCtx` bounds setup only, and `handlerCtx`
is the handlers' parent.

A panic in a message handler is recovered: it is logged at error level with the message's subject, counted on the
JetStream error metric as operation `handler_panic`, and the message is Nak'd so the server redelivers it. A Nak that
fails is logged with the panic. A handler that returns without settling its message is not Nak'd for it; the server
redelivers the message when its AckWait expires.

### Distributed Tracing

All publish and request methods automatically propagate W3C-compliant trace context. If no trace exists in the context,
one is auto-generated, ensuring every message can be correlated across services.

**Headers injected:**

- `traceparent` - W3C Trace Context format (`00-{trace_id}-{span_id}-{flags}`)
- `X-Trace-ID`, `X-Span-ID`, `X-Parent-Span-ID` - Simplified headers for compatibility

## Usage

### Basic Example

```go
// Create and connect to NATS
client, err := natsclient.NewClient("nats://localhost:4222")
if err != nil {
    return err
}

ctx := context.Background()
err = client.Connect(ctx)
if err != nil {
    return err
}
defer client.Close(ctx)

// Publish a message
err = client.Publish(ctx, "subject.name", []byte("message data"))

// Subscribe to messages
sub, err := client.Subscribe(ctx, "subject.*", func(msgCtx context.Context, msg *nats.Msg) {
    // Handle message
    fmt.Printf("Received: %s\n", string(msg.Data))
})
```

### Advanced Usage

```go
// Create client with options
client, err := natsclient.NewClient("nats://localhost:4222",
    natsclient.WithMaxReconnects(-1),  // Infinite reconnects
    natsclient.WithReconnectWait(2*time.Second),
)

// JetStream operations
js, err := client.JetStream()
stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
    Name:     "EVENTS",
    Subjects: []string{"events.>"},
})

// Key-Value store with CAS
bucket, err := client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
    Bucket: "config",
})

kvStore := client.NewKVStore(bucket)
err = kvStore.UpdateJSON(ctx, "service.config", func(config map[string]any) error {
    config["enabled"] = true
    return nil
})
```

### Tracing

```go
// Traces are auto-generated - no action needed for basic tracing
err := client.Publish(ctx, "events.user", data)

// Explicit trace context
tc := natsclient.NewTraceContext()
ctx = natsclient.ContextWithTrace(ctx, tc)
reply, err := client.Request(ctx, "service.action", data, 5*time.Second)

// Extract trace from incoming message
tc = natsclient.ExtractTrace(msg)
if tc != nil {
    // Create child span for downstream calls
    childCtx := natsclient.ContextWithTrace(ctx, tc.NewSpan())
    err = client.Publish(childCtx, "downstream.subject", response)
}
```

## API Reference

### Types

#### `Client`

Main client type providing NATS connectivity with circuit breaker protection.

```go
type Client struct {
    // Internal fields not exposed
}
```

#### `KVStore`

High-level KV operations with built-in CAS support.

```go
type KVStore struct {
    // Internal fields not exposed
}
```

#### `KVOptions`

Configuration for KV operations behavior.

```go
type KVOptions struct {
    MaxRetries            int           // Maximum CAS retry attempts (default: 10)
    RetryDelay            time.Duration // Initial delay between retries (default: 10ms)
    Timeout               time.Duration // Operation timeout (default: 5s)
    MaxValueSize          int           // Maximum size for values (default: 1MB)
    UseExponentialBackoff bool          // Enable exponential backoff with jitter (default: true)
    MaxRetryDelay         time.Duration // Maximum delay between retries (default: 1s)
}
```

### Functions

#### `NewClient(url string, opts ...ClientOption) (*Client, error)`

Creates a new NATS client with the specified server URL and options.

#### `(c *Client) Connect(ctx context.Context) error`

Establishes connection to NATS server with circuit breaker protection.

#### `(c *Client) Publish(ctx context.Context, subject string, data []byte) error`

Publishes a message to the specified subject.

#### `NewKVStore`

`(c *Client) NewKVStore(bucket jetstream.KeyValue, opts ...func(*KVOptions)) *KVStore`

Creates a KVStore instance for high-level KV operations.

#### `UpdateJSON`

`(kv *KVStore) UpdateJSON(ctx context.Context, key string, updateFn func(map[string]any) error) error`

Performs CAS update on JSON data with automatic retry on conflicts.

### Logging

`WithLogger(logger *slog.Logger) ClientOption` sets the logger for debug output.

## Architecture

### Design Decisions

**Circuit Breaker over Simple Retry**: Chose circuit breaker pattern to prevent cascade failures in distributed systems.
After threshold failures, the circuit opens to fail fast rather than continuously retry, giving the system time to
recover.

**Context-First API**: Every I/O operation requires context.Context as first parameter for proper cancellation and
timeout support, essential for production systems.

**KVStore Abstraction**: Created high-level KV abstraction with built-in CAS retry logic to eliminate code duplication
across services. Centralizes revision conflict handling and retry logic.

### Integration Points

- **Dependencies**: NATS server (2.x compatible)
- **Used By**: All SemStreams services and components requiring messaging
- **Data Flow**: `Application → Client → Circuit Breaker → NATS Connection → Server`

## Configuration

### Connection Options

```go
WithMaxReconnects(-1)              // Infinite reconnects (default: -1)
WithReconnectWait(2*time.Second)   // Wait between reconnects (default: 2s)
WithTimeout(10*time.Second)        // Connection timeout (default: 5s)
WithLogger(logger)                  // Enable debug logging
```

## Error Handling

### Error Types

```go
var (
    ErrCircuitOpen        = errors.New("circuit breaker is open")
    ErrNotConnected       = errors.New("not connected to NATS")
    ErrKVKeyNotFound      = errors.New("kv: key not found")
    ErrKVRevisionMismatch = errors.New("kv: revision mismatch (concurrent update)")
    ErrKVMaxRetriesExceeded = errors.New("kv: max retries exceeded")
)
```

### Error Detection

```go
// Check specific errors
if errors.Is(err, natsclient.ErrCircuitOpen) {
    // Wait for circuit to close
}

// KV conflict detection
if errors.Is(err, natsclient.ErrKVRevisionMismatch) {
    // Handle concurrent update
}

// Helper functions for NATS errors
if natsclient.IsKVNotFoundError(err) {
    // Key doesn't exist
}
```

## Testing

### Test Utilities

The package's integration tests start a real NATS server with `internal/harness/natsfixture`:

```go
// Start a disposable NATS server via testcontainers; its Stop runs on t.Cleanup
fixture := natsfixture.New(t)
if err := fixture.Start(t.Context()); err != nil {
    t.Fatal(err)
}

client, err := natsclient.NewClient(fixture.URL())
if err == nil {
    err = client.Connect(t.Context())
}
```

### Testing Patterns

- Uses real NATS server via testcontainers (no mocks)
- Tests actual behavior including connection lifecycle
- Thread-safe testing with proper synchronization
- Comprehensive circuit breaker scenario testing

## Performance Considerations

- **Concurrency**: Thread-safe for concurrent use from multiple goroutines
- **Memory**: Scales with number of active subscriptions and consumers
- **Throughput**: Limited by network latency and NATS server performance
- **Circuit Breaker**: Adds minimal overhead, fails fast when open

## Examples

### Resilient Publisher

```go
package main

import (
    "context"
    "log"
    "log/slog"
    "time"

    "github.com/c360studio/semengine/natsclient"
)

func main() {
    client, err := natsclient.NewClient("nats://localhost:4222",
        natsclient.WithMaxReconnects(-1),
        natsclient.WithLogger(slog.Default()),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    ctx := context.Background()
    if err := client.Connect(ctx); err != nil {
        log.Fatal(err)
    }
    defer client.Close(ctx)
    
    // Publish with automatic reconnection handling
    for {
        err := client.Publish(ctx, "telemetry.data", []byte("sensor reading"))
        if err != nil {
            if errors.Is(err, natsclient.ErrCircuitOpen) {
                log.Println("Circuit open, waiting...")
                time.Sleep(5 * time.Second)
                continue
            }
            log.Printf("Publish error: %v", err)
        }
        time.Sleep(time.Second)
    }
}
```

### Configuration Management with KV

```go
// Manage service configuration with optimistic locking
bucket, _ := client.CreateKeyValueBucket(ctx, jetstream.KeyValueConfig{
    Bucket:   "config",
    History:  5,
    Replicas: 3,
})

kvStore := client.NewKVStore(bucket)

// Atomic configuration update with retry
err = kvStore.UpdateJSON(ctx, "services.processor", func(config map[string]any) error {
    // This function may be called multiple times on conflict
    config["workers"] = 10
    config["timeout"] = "30s"
    return nil
})
```

## License

MIT
