# natsclient

A NATS client for SemEngine. It wraps nats.go, keeps nats.go's automatic reconnection, and adds
helpers for JetStream streams, consumers and key-value buckets.

## Overview

The natsclient package wraps the NATS Go client (nats.go) and keeps nats.go's automatic
reconnection with natsclient's settings: unlimited attempts (`WithMaxReconnects`) and a `ReconnectWait` of 2 s
(`WithReconnectWait`). How soon each attempt comes is not promised. While nats.go reconnects, `Publish`,
`RequestClassified` and `Subscribe` return `ErrNotConnected` at once, and key-value calls wait for their
context (probe P15).

Every call that waits for the server takes a context, and the context ends that wait (P13). It does not end a call
held behind a write the server is not reading. Such a call waits up to nats.go's write timeout for each write ahead of
it (one minute by default; `WithWriteTimeout` sets it), or until `Close` closes the socket (P13, P14, P17). `Connect`
returns within its context, and so does `Close` on a connection the client dialled (`TestConnectEndsWithItsContext`,
`TestCloseEndsAHeldWrite`).

[ADR-112](../docs/adr/112-natsclient-contract.md) records what natsclient promises over nats.go, and the test or probe
behind each promise. A probe is a program posted on PR #177 with its source, command, versions and output; ADR-112's
Status names the comment that holds each one.

This package is the foundation for all NATS communication in SemEngine, providing both core pub/sub
functionality and advanced features like JetStream streams, consumers, and Key-Value stores. The client handles
connection lifecycle management automatically, allowing applications to focus on business logic rather than connection
state management.

## Installation

```go
import "github.com/c360studio/semengine/natsclient"
```

## Core Concepts

### Connection Lifecycle

Connection states transition through: Disconnected → Connecting → Connected → Reconnecting (on failure) → Connected.
The client handles all transitions automatically.

### KVStore Abstraction

A high-level abstraction over NATS KV providing automatic CAS (Compare-And-Swap) retry logic, JSON helpers, and
consistent error handling for configuration management scenarios.

### Consumer contexts and handler panics

The context passed to `ConsumeStreamWithConfig` and `ConsumeInternalStreamWithConfig` bounds the consumer's setup
only. Each handler's context descends from a context the client owns for that consumer, which it cancels when `Close`
begins; `Close` still waits for every handler to return. `ConsumeStreamWithConfigContexts` takes a second context,
`handlerCtx`, when the caller wants to choose the handlers' parent itself.

A panic in a message handler is recovered: it is logged at error level with the message's subject, counted on the
JetStream error metric as operation `handler_panic`, and the message is Nak'd
(`TestSafeHandleMessageRecoversPanicNaksLogsAndCounts`). A Nak that fails is logged with the panic. The server
redelivers a Nak'd message (`TestIntegration_ConsumeStreamWithConfig_Nak`).

A handler that returns without settling its message is not Nak'd for it. The server redelivers such a message when
its ack wait ends, as `TestIntegrationConsumeDeliveryWithHeartbeatStoppedRenewalUsesBackOff` shows for a consumer with
`BackOff`. For a plain consumer handler, that redelivery is not measured here.

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

When `Subscribe` returns, the server has read the subscription, so a request from another connection finds it, unless
the server refused it. A refusal is not returned as an error. A client that connected with `Connect` logs it, and a
client built with `WithMetrics` also counts it on `semengine_nats_server_refusals_total`, labelled `reason`
(`permission_violation` or `max_subscriptions_exceeded`). The count can rise after `Subscribe` has returned. A
publish the server refuses is counted the same way.

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
reply, err := client.RequestClassified(ctx, "service.action", data, 5*time.Second)

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

Main client type providing NATS connectivity.

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
    Timeout               time.Duration // Ends the wait for the server's answer, not a held write (default: 5s)
    MaxValueSize          int           // Maximum size for values (default: 1MB)
    UseExponentialBackoff bool          // Enable exponential backoff with jitter (default: true)
    MaxRetryDelay         time.Duration // Maximum delay between retries (default: 1s)
}
```

### Functions

#### `NewClient(url string, opts ...ClientOption) (*Client, error)`

Creates a new NATS client with the specified server URL and options.

#### `(c *Client) Connect(ctx context.Context) error`

Establishes connection to NATS server.

#### `(c *Client) Publish(ctx context.Context, subject string, data []byte) error`

Publishes a message to the specified subject.

#### `NewKVStore`

`(c *Client) NewKVStore(bucket jetstream.KeyValue, opts ...func(*KVOptions)) *KVStore`

Creates a KVStore instance for high-level KV operations.

#### `UpdateJSON`

`(kv *KVStore) UpdateJSON(ctx context.Context, key string, updateFn func(map[string]any) error) error`

Performs CAS update on JSON data with automatic retry on conflicts.

#### `IsKVTombstone`

`IsKVTombstone(operation jetstream.KeyValueOp) bool`

Reports whether a KV watch entry removes its key: a delete or a purge. Both arrive with an empty value, so a
watcher that handles a removal checks this one predicate rather than testing for a delete and forgetting the purge.

### Logging

`WithLogger(logger *slog.Logger) ClientOption` sets the logger for debug output.

## Architecture

### Design Decisions

**Context-First API**: every call that waits for the server, except `RTT`, takes a `context.Context` first, and the
context ends that wait (P13, P18). It does not end a write the server is not reading (see the overview).

**KVStore Abstraction**: Created high-level KV abstraction with built-in CAS retry logic to eliminate code duplication
across services. Centralizes revision conflict handling and retry logic.

### Integration Points

- **Dependencies**: NATS server (2.x compatible)
- **Used By**: Code that needs NATS messaging
- **Data Flow**: `Application → Client → NATS Connection → Server`

## Configuration

### Connection Options

```go
WithMaxReconnects(-1)              // Infinite reconnects (default: -1)
WithReconnectWait(2*time.Second)   // Wait between reconnects (default: 2s)
WithTimeout(10*time.Second)        // Connection timeout (default: 5s)
WithWriteTimeout(10*time.Second)   // How long one write may wait for the server to read it (default: 1m, nats.go's)
WithPingInterval(10*time.Second)   // How often nats.go sends the server a PING (default: 30s)
WithLogger(logger)                  // Enable debug logging
WithMetrics(registry)               // Record JetStream and server-refusal metrics on registry
```

Give each client its own metrics registry. A registry holds one client's JetStream series: its stream and consumer
gauges are set from what that client reads from the server. A second client on the same registry writes the same
collectors, so the two overwrite each other's gauges, and which client's value a scrape shows is not promised. No
configuration we know of runs two clients on one registry.

Set `WithWriteTimeout` and `WithPingInterval` together. A write the timeout ends partway leaves the connection
connected but unable to carry anything. It recovers when its PINGs go unanswered: nats.go then finds the connection
stale, closes it and reconnects (`TestWriteTimeoutAndPingIntervalOptions`; probe P24). Each option refuses a value
that is not positive.

## Error Handling

### Error Types

```go
var (
    ErrNotConnected       = fmt.Errorf("not connected to NATS: %w", errs.ErrConnectionLost)
    ErrKVKeyNotFound      = errors.New("kv: key not found")
    ErrKVRevisionMismatch = errors.New("kv: revision mismatch (concurrent update)")
    ErrKVMaxRetriesExceeded = errors.New("kv: max retries exceeded")
    ErrKVSkipWrite        = errors.New("kv: update callback skipped the write")
)
```

### Error Detection

```go
// Check specific errors
if errors.Is(err, natsclient.ErrNotConnected) {
    // The client is not connected; try again later
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

### Skipping a write

`ErrKVSkipWrite` is not returned by the client. An update callback returns it, wrapped or not, to say that the read
value needs no change. That run writes nothing, the value returned beside the error is ignored, and the call returns
no error: `UpdateWithRetryRead` and `UpdateWithRetryRev` return revision 0, and `UpdateWithRetry` and `UpdateJSON`
return nil. No write ever commits at revision 0, so `(0, nil)` means the callback skipped.

`UpdateWithRetryRead`'s callback also gets the revision the bytes were read at, or 0 when the key is absent (never
written, deleted or purged). A caller that decides to skip keeps that revision from the callback's argument; the
call's result is only ever the revision of its own write.

```go
var readAt uint64
committed, err := kv.UpdateWithRetryRead(ctx, key, func(current []byte, revision uint64) ([]byte, error) {
    readAt = revision
    if alreadyApplied(current) {
        return nil, natsclient.ErrKVSkipWrite
    }
    return apply(current), nil
})
// committed is 0 after a skip or an error; readAt is the revision the last run read.
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

## Performance Considerations

- **Concurrency**: Thread-safe for concurrent use from multiple goroutines
- **Memory**: Scales with number of active subscriptions and consumers
- **Throughput**: Limited by network latency and NATS server performance

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
    
    // While nats.go reconnects, Publish returns ErrNotConnected at once, and the
    // loop tries again on its next tick. A nil error means the message is in
    // nats.go's buffer, not that the server has it (ADR-112).
    for {
        if err := client.Publish(ctx, "telemetry.data", []byte("sensor reading")); err != nil {
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
