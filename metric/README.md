# Metric

Prometheus-based metrics collection and HTTP server for SemStreams platform monitoring and observability.

## Overview

The metric package provides comprehensive metrics collection capabilities for the SemStreams platform using Prometheus
as the underlying metrics system. It includes core platform metrics, service-specific metric registration, and an HTTP
metrics server for monitoring and alerting integration.

The package provides a centralized metrics registry that manages both core platform metrics (service status, message
processing, NATS connection health) and custom service-specific metrics. The HTTP server exposes metrics in Prometheus
format for collection by monitoring systems.

The design follows Prometheus best practices with proper metric naming, labeling, and types (counters, gauges,
histograms) to provide comprehensive operational visibility into the SemStreams platform.

## Installation

```go
import (
    "log"

    "github.com/c360studio/semengine/metric"
    "github.com/c360studio/semengine/pkg/security"
)
```

## core Concepts

### Metrics Registry

Centralized registry that manages Prometheus metric registration, provides core platform metrics, and allows services to
register custom metrics with conflict detection and lifecycle management.

### CoreMetrics

Pre-defined platform-level metrics covering service status, message processing throughput, error rates, health check
status, and NATS connection monitoring for consistent operational visibility.

### Metrics Server

HTTP server that exposes collected metrics in Prometheus format with configurable port and path, health endpoints, and
OpenMetrics support for integration with monitoring systems.

During composed production boot, `service.Manager` privately registers the
fixed `semstreams_startup_units` collector and owns the configured server
listener. A standalone `metric.Server` retains its direct lifecycle behavior.

### Service Metrics

Framework for services to register custom metrics including counters, gauges, and histograms with automatic namespacing
and conflict prevention for service-specific monitoring needs.

## Usage

### Basic Example

```go
import (
    "context"
    "time"

    "github.com/c360studio/semengine/metric"
)

// Create metrics registry with core platform metrics
registry := metric.NewMetricsRegistry()
securityCfg := security.Config{}

// Start metrics HTTP server
server := metric.NewServer(9090, "/metrics", registry, securityCfg)
runtimeCtx := context.Background()
if err := server.Start(runtimeCtx); err != nil {
    log.Fatalf("Failed to start metrics server: %v", err)
}
defer func() {
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := server.Stop(shutdownCtx); err != nil {
        log.Printf("Failed to stop metrics server: %v", err)
    }
}()

// Record core platform metrics
coreMetrics := registry.CoreMetrics()
coreMetrics.RecordServiceStatus("my-service", 2) // 2=running
coreMetrics.RecordMessageReceived("my-service", "gps.position")
coreMetrics.RecordHealthStatus("my-service", true)

// Access metrics at http://localhost:9090/metrics
```

### Advanced Usage - Custom Service Metrics

```go
// Create custom metrics for a service
messageCounter := prometheus.NewCounter(prometheus.CounterOpts{
    Namespace: "semstreams",
    Subsystem: "gps_service",
    Name:      "coordinates_processed_total",
    Help:      "Total GPS coordinates processed",
})

latencyHistogram := prometheus.NewHistogram(prometheus.HistogramOpts{
    Namespace: "semstreams",
    Subsystem: "gps_service",
    Name:      "processing_duration_seconds",
    Help:      "GPS coordinate processing duration",
    Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
})

queueGauge := prometheus.NewGauge(prometheus.GaugeOpts{
    Namespace: "semstreams",
    Subsystem: "gps_service",
    Name:      "queue_size",
    Help:      "Current GPS processing queue size",
})

// Register custom metrics with the registry, keeping the collectors it returns
messageCounter, err := metric.RegisterOrGet(registry, "gps-service", "coordinates_processed", messageCounter)
if err != nil {
    log.Printf("Failed to register counter: %v", err)
}

latencyHistogram, err = metric.RegisterOrGet(registry, "gps-service", "processing_duration", latencyHistogram)
if err != nil {
    log.Printf("Failed to register histogram: %v", err)
}

queueGauge, err = metric.RegisterOrGet(registry, "gps-service", "queue_size", queueGauge)
if err != nil {
    log.Printf("Failed to register gauge: %v", err)
}

// Use the custom metrics
messageCounter.Inc()
queueGauge.Set(25)

start := time.Now()
processGPSCoordinate()
latencyHistogram.Observe(time.Since(start).Seconds())

// Cleanup on service shutdown
registry.Unregister("gps-service", "coordinates_processed")
registry.Unregister("gps-service", "processing_duration")
registry.Unregister("gps-service", "queue_size")
```

## API Reference

### Types

#### `MetricsRegistry`

Central registry for managing all platform metrics.

```go
type MetricsRegistry struct {
    // private fields
}

func NewMetricsRegistry() *MetricsRegistry                 // Create new registry
func (r *MetricsRegistry) PrometheusRegistry() *prometheus.Registry  // Get Prometheus registry
func (r *MetricsRegistry) CoreMetrics() *CoreMetrics      // Get core platform metrics

// Service metric registration: one collector per service/metric key
func RegisterOrGet[C prometheus.Collector](r *MetricsRegistry, serviceName, metricName string, candidate C) (C, error)
func (r *MetricsRegistry) Unregister(serviceName, metricName string) bool  // Remove metric
```

#### `CoreMetrics`

Pre-defined platform-level metrics for consistent monitoring.

```go
type CoreMetrics struct {
    // Service metrics
    ServiceStatus      *prometheus.GaugeVec    // Service status (0=stopped, 1=starting, 2=running, 3=stopping, 4=failed)
    MessagesReceived   *prometheus.CounterVec  // Total messages received by service and type
    MessagesProcessed  *prometheus.CounterVec  // Total messages processed by service, type, and status
    MessagesPublished  *prometheus.CounterVec  // Total messages published by service and subject
    ProcessingDuration *prometheus.HistogramVec // Processing duration by service and operation
    ErrorsTotal        *prometheus.CounterVec  // Total errors by service and type
    HealthCheckStatus  *prometheus.GaugeVec    // Health status by service (0=unhealthy, 1=healthy)

    // NATS metrics
    NATSConnected      prometheus.Gauge        // NATS connection status (0=disconnected, 1=connected)
    NATSRTT            prometheus.Gauge        // NATS round-trip time in milliseconds
    NATSReconnects     prometheus.Counter      // Total NATS reconnections
    NATSCircuitBreaker prometheus.Gauge        // Circuit breaker status (0=closed, 1=open, 2=half-open)
}

// Recording methods
func (c *CoreMetrics) RecordServiceStatus(service string, status int)
func (c *CoreMetrics) RecordMessageReceived(service, messageType string)
func (c *CoreMetrics) RecordMessageProcessed(service, messageType, status string)
func (c *CoreMetrics) RecordMessagePublished(service, subject string)
func (c *CoreMetrics) RecordProcessingDuration(service, operation string, duration time.Duration)
func (c *CoreMetrics) RecordError(service, errorType string)
func (c *CoreMetrics) RecordHealthStatus(service string, healthy bool)
func (c *CoreMetrics) RecordNATSStatus(connected bool)
func (c *CoreMetrics) RecordNATSRTT(rtt time.Duration)
func (c *CoreMetrics) RecordNATSReconnect()
func (c *CoreMetrics) RecordCircuitBreakerState(state int)
```

#### `Server`

HTTP server for exposing metrics to Prometheus.

```go
type Server struct {
    // private fields
}

func NewServer(port int, path string, registry *MetricsRegistry, security security.Config) *Server
func (s *Server) Start(context.Context) error // One-shot synchronous bind; context owns served requests
func (s *Server) Stop(context.Context) error  // Caller-bounded graceful attempt, then bounded force-close/join
func (s *Server) Address() string              // Get server address
```

### Functions

#### `RegisterOrGet`

`RegisterOrGet[C prometheus.Collector](r *MetricsRegistry, serviceName, metricName string, candidate C) (C, error)`

Registers `candidate` under the key `serviceName.metricName`, or returns the collector already registered under that
key when it has the same concrete type and descriptors. Use the returned collector, never your own candidate. A nil
candidate, a type or descriptor mismatch, and a descriptor another key already owns are refused with a fatal error,
and nothing is stored.

#### `NewMetricsRegistry() *MetricsRegistry`

Creates a new metrics registry with core platform metrics and Go runtime metrics.

#### `NewServer`

`NewServer(port int, path string, registry *MetricsRegistry, security security.Config) *Server`

Creates a new HTTP metrics server with default values for zero port (9090) and empty path ("/metrics").

## Architecture

### Design Decisions

**Prometheus Integration**: Chose Prometheus as the metrics backend because it's the industry standard for container and
microservice monitoring with excellent tooling ecosystem.

- Trade-off: Gained mature monitoring ecosystem but coupled to Prometheus API
- Alternative considered: Custom metrics format (lacks tooling and standardization)

**Centralized Registry**: Implemented single MetricsRegistry to manage all metrics and prevent conflicts between
services registering the same metric names.

- Rationale: Prevents metric naming conflicts and provides consistent lifecycle management
- Trade-off: Gained consistency but requires services to coordinate through registry

**core Platform Metrics**: Pre-defined core metrics for all platform services to ensure consistent monitoring across
services without each service needing to define standard metrics.

- Chose standardized metrics over service-specific only because operational consistency is critical
- Trade-off: Gained consistency but reduced flexibility for unique service requirements

### Integration Points

- **Dependencies**: Prometheus Go client library for metrics implementation and HTTP handlers
- **Used By**: Service framework for automatic metrics collection, components for performance monitoring
- **Data Flow**: `Service Operations → Metric Recording → Registry Storage → HTTP Endpoint → Prometheus Scraping`

## Configuration

### Metrics Server Configuration

```yaml
# Metrics server configuration
metrics:
  enabled: true
  port: 9090           # HTTP server port
  path: "/metrics"     # Metrics endpoint path
  include_go_metrics: true  # Include Go runtime metrics
```

### Service Metrics Configuration

```yaml
# Service-specific metrics configuration
services:
  gps-service:
    metrics:
      custom_metrics:
        - name: "coordinates_processed"
          type: "counter"
          help: "Total GPS coordinates processed"
        - name: "processing_latency"
          type: "histogram"
          help: "GPS processing latency"
          buckets: [0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0]
```

## Error Handling

### Metric Registration Errors

```go
// Registering the same key again with an identical collector returns the
// registered collector; a conflicting registration is a fatal error
counter, err := metric.RegisterOrGet(registry, "service", "metric", counter)
if err != nil {
    log.Printf("Failed to register metric: %v", err)
    return err
}

// Safe metric unregistration
if !registry.Unregister("service", "metric") {
    log.Printf("Metric was not registered or already removed")
}
```

### Server Lifecycle Errors

```go
// Start server with error handling
server := metric.NewServer(9090, "/metrics", registry, security.Config{})
if err := server.Start(ctx); err != nil {
    log.Fatalf("Metrics server failed: %v", err)
}

// Graceful shutdown
if err := server.Stop(shutdownCtx); err != nil {
    log.Printf("Error stopping metrics server: %v", err)
}
```

`Stop` first attempts a graceful HTTP shutdown within `shutdownCtx`. If that attempt fails or its budget expires,
the server and listener are force-closed and `Stop` waits for its exact serving goroutine, which returns once its
listener is closed, and for every request the server admitted, including the metrics collection it runs. `Stop`
returns the context's error whenever `shutdownCtx` has ended, and a later `Stop` waits again for what is still
running. Once `Stop` has begun, a request that reaches the handler is refused with 503 Service Unavailable. The
instance is terminal after that attempt; a completed repeated `Stop` returns nil.

### Best Practices

```go
// DO: Use consistent metric naming
counter := prometheus.NewCounter(prometheus.CounterOpts{
    Namespace: "semstreams",           // Always use "semstreams"
    Subsystem: "service_name",         // Use service name
    Name:      "operations_total",     // Descriptive name with units
    Help:      "Total operations processed",
})

// DO: Handle registration errors and keep the returned collector
counter, err := metric.RegisterOrGet(registry, "service", "operations", counter)
if err != nil {
    return fmt.Errorf("metric registration failed: %w", err)
}

// DO: Use appropriate metric types
counter.Inc()                          // For monotonically increasing values
gauge.Set(42)                         // For values that can go up and down
histogram.Observe(duration.Seconds()) // For distributions and timings

// DON'T: Ignore registration errors or the returned collector
metric.RegisterOrGet(registry, "service", "metric", counter) // Missing error check; writes to counter may be lost

// DON'T: Use inconsistent naming
prometheus.NewCounter(prometheus.CounterOpts{
    Name: "my_custom_metric", // Missing namespace and subsystem
})
```

## Testing

### Test Utilities

```go
// Test metrics registry
func TestMetricsRegistry(t *testing.T) {
    registry := metric.NewMetricsRegistry()

    // Test counter registration
    counter := prometheus.NewCounter(prometheus.CounterOpts{
        Name: "test_counter",
        Help: "Test counter",
    })

    registered, err := metric.RegisterOrGet(registry, "test-service", "test_counter", counter)
    assert.NoError(t, err)

    // Registering the same key again returns the registered collector
    again, err := metric.RegisterOrGet(registry, "test-service", "test_counter", counter)
    assert.NoError(t, err)
    assert.Same(t, registered, again)

    // Test unregistration
    success := registry.Unregister("test-service", "test_counter")
    assert.True(t, success)
}

// Test core metrics recording
func TestCoreMetrics(t *testing.T) {
    registry := metric.NewMetricsRegistry()
    coreMetrics := registry.CoreMetrics()

    // Test service status recording
    coreMetrics.RecordServiceStatus("test-service", 2)

    // Verify metric was recorded (would require prometheus test utilities)
    // This is a simplified example - actual testing would use prometheus/testutil
}
```

### Testing Patterns

- Use prometheus/testutil package for metric value verification
- Test metric registration success and failure scenarios
- Verify metric cleanup during service shutdown
- Test concurrent metric access with multiple goroutines

## Performance Considerations

- **Metric Recording**: O(1) operation for most Prometheus metric types
- **Registry Operations**: Thread-safe with read-write mutex - concurrent reads allowed
- **Memory Usage**: Metrics consume memory proportional to label cardinality (avoid high-cardinality labels)
- **HTTP Server**: Single-threaded for simplicity - sufficient for typical Prometheus scraping patterns

## Examples

### Example 1: Service Integration

```go
package main

import (
    "context"
    "log"
    "math/rand"
    "time"

    "github.com/c360studio/semengine/metric"
    "github.com/prometheus/client_golang/prometheus"
)

type GPSService struct {
    name         string
    registry     *metric.MetricsRegistry
    coreMetrics  *metric.CoreMetrics

    // Custom metrics
    coordinatesProcessed prometheus.Counter
    processingLatency    prometheus.Histogram
    queueSize           prometheus.Gauge
}

func NewGPSService(registry *metric.MetricsRegistry) (*GPSService, error) {
    service := &GPSService{
        name:        "gps-service",
        registry:    registry,
        coreMetrics: registry.CoreMetrics(),
    }

    // Create custom metrics
    service.coordinatesProcessed = prometheus.NewCounter(prometheus.CounterOpts{
        Namespace: "semstreams",
        Subsystem: "gps",
        Name:      "coordinates_processed_total",
        Help:      "Total GPS coordinates processed",
    })

    service.processingLatency = prometheus.NewHistogram(prometheus.HistogramOpts{
        Namespace: "semstreams",
        Subsystem: "gps",
        Name:      "processing_duration_seconds",
        Help:      "GPS coordinate processing duration",
        Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0},
    })

    service.queueSize = prometheus.NewGauge(prometheus.GaugeOpts{
        Namespace: "semstreams",
        Subsystem: "gps",
        Name:      "queue_size",
        Help:      "Current GPS processing queue size",
    })

    // Register custom metrics, keeping the collectors the registry returns
    var err error
    service.coordinatesProcessed, err = metric.RegisterOrGet(registry, "gps-service", "coordinates_processed",
        service.coordinatesProcessed)
    if err != nil {
        return nil, err
    }

    service.processingLatency, err = metric.RegisterOrGet(registry, "gps-service", "processing_latency",
        service.processingLatency)
    if err != nil {
        return nil, err
    }

    service.queueSize, err = metric.RegisterOrGet(registry, "gps-service", "queue_size", service.queueSize)
    if err != nil {
        return nil, err
    }

    return service, nil
}

func (g *GPSService) Start(ctx context.Context) error {
    // Record service startup
    g.coreMetrics.RecordServiceStatus(g.name, 1) // starting

    // Simulate startup delay
    time.Sleep(100 * time.Millisecond)

    g.coreMetrics.RecordServiceStatus(g.name, 2) // running
    g.coreMetrics.RecordHealthStatus(g.name, true)

    // Start processing loop
    go g.processingLoop(ctx)

    log.Printf("GPS service started")
    return nil
}

func (g *GPSService) Stop() error {
    g.coreMetrics.RecordServiceStatus(g.name, 3) // stopping

    // Cleanup custom metrics
    g.registry.Unregister("gps-service", "coordinates_processed")
    g.registry.Unregister("gps-service", "processing_latency")
    g.registry.Unregister("gps-service", "queue_size")

    g.coreMetrics.RecordServiceStatus(g.name, 0) // stopped
    log.Printf("GPS service stopped")
    return nil
}

func (g *GPSService) processingLoop(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            g.processCoordinates()
        case <-ctx.Done():
            return
        }
    }
}

func (g *GPSService) processCoordinates() {
    // Simulate processing
    start := time.Now()

    // Random processing time
    processingTime := time.Duration(rand.Intn(50)) * time.Millisecond
    time.Sleep(processingTime)

    // Record metrics
    g.coordinatesProcessed.Inc()
    g.processingLatency.Observe(time.Since(start).Seconds())
    g.queueSize.Set(float64(rand.Intn(100)))

    // Record core metrics
    g.coreMetrics.RecordMessageReceived(g.name, "gps.coordinates")
    g.coreMetrics.RecordMessageProcessed(g.name, "gps.coordinates", "success")
    g.coreMetrics.RecordProcessingDuration(g.name, "coordinates", time.Since(start))

    // Simulate occasional errors
    if rand.Float32() < 0.05 { // 5% error rate
        g.coreMetrics.RecordError(g.name, "processing_error")
    }
}

func main() {
    // Create metrics registry
    registry := metric.NewMetricsRegistry()

    // Start metrics server
    server := metric.NewServer(9090, "/metrics", registry, security.Config{})
    log.Printf("Starting metrics server on %s", server.Address())
    runtimeCtx := context.Background()
    if err := server.Start(runtimeCtx); err != nil {
        log.Fatalf("Failed to start metrics server: %v", err)
    }

    // Create and start GPS service
    gpsService, err := NewGPSService(registry)
    if err != nil {
        log.Fatalf("Failed to create GPS service: %v", err)
    }

    if err := gpsService.Start(runtimeCtx); err != nil {
        log.Fatalf("Failed to start GPS service: %v", err)
    }

    // Run for demo period
    time.Sleep(30 * time.Second)

    // Cleanup
    gpsService.Stop()
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := server.Stop(shutdownCtx); err != nil {
        log.Printf("Failed to stop metrics server: %v", err)
    }
}
```

### Example 2: Multi-Service Monitoring

```go
package main

import (
    "context"
    "fmt"
    "log"
    "math/rand"
    "sync"
    "time"

    "github.com/c360studio/semengine/metric"
    "github.com/prometheus/client_golang/prometheus"
)

type MonitoredService struct {
    name        string
    registry    *metric.MetricsRegistry
    coreMetrics *metric.CoreMetrics

    // Service-specific metrics
    requestCounter  prometheus.Counter
    responseTime    prometheus.Histogram
    activeUsers     prometheus.Gauge
    errorRate       prometheus.Counter

    // Control
    stopCh chan struct{}
    wg     sync.WaitGroup
}

func NewMonitoredService(name string, registry *metric.MetricsRegistry) (*MonitoredService, error) {
    service := &MonitoredService{
        name:        name,
        registry:    registry,
        coreMetrics: registry.CoreMetrics(),
        stopCh:      make(chan struct{}),
    }

    // Create metrics
    service.requestCounter = prometheus.NewCounter(prometheus.CounterOpts{
        Namespace: "semstreams",
        Subsystem: name,
        Name:      "requests_total",
        Help:      fmt.Sprintf("Total requests processed by %s", name),
    })

    service.responseTime = prometheus.NewHistogram(prometheus.HistogramOpts{
        Namespace: "semstreams",
        Subsystem: name,
        Name:      "response_time_seconds",
        Help:      fmt.Sprintf("Response time for %s", name),
    })

    service.activeUsers = prometheus.NewGauge(prometheus.GaugeOpts{
        Namespace: "semstreams",
        Subsystem: name,
        Name:      "active_users",
        Help:      fmt.Sprintf("Active users for %s", name),
    })

    service.errorRate = prometheus.NewCounter(prometheus.CounterOpts{
        Namespace: "semstreams",
        Subsystem: name,
        Name:      "errors_total",
        Help:      fmt.Sprintf("Total errors for %s", name),
    })

    // Register metrics, keeping the collectors the registry returns
    var err error
    if service.requestCounter, err = metric.RegisterOrGet(registry, name, "requests", service.requestCounter); err != nil {
        return nil, fmt.Errorf("failed to register request counter: %w", err)
    }

    if service.responseTime, err = metric.RegisterOrGet(registry, name, "response_time", service.responseTime); err != nil {
        return nil, fmt.Errorf("failed to register response time histogram: %w", err)
    }

    if service.activeUsers, err = metric.RegisterOrGet(registry, name, "active_users", service.activeUsers); err != nil {
        return nil, fmt.Errorf("failed to register active users gauge: %w", err)
    }

    if service.errorRate, err = metric.RegisterOrGet(registry, name, "errors", service.errorRate); err != nil {
        return nil, fmt.Errorf("failed to register error counter: %w", err)
    }

    return service, nil
}

func (s *MonitoredService) Start() error {
    s.coreMetrics.RecordServiceStatus(s.name, 1) // starting

    // Simulate startup
    time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)

    s.coreMetrics.RecordServiceStatus(s.name, 2) // running
    s.coreMetrics.RecordHealthStatus(s.name, true)

    // Start metrics collection
    s.wg.Add(1)
    go s.metricsLoop()

    log.Printf("Service %s started", s.name)
    return nil
}

func (s *MonitoredService) Stop() error {
    s.coreMetrics.RecordServiceStatus(s.name, 3) // stopping

    close(s.stopCh)
    s.wg.Wait()

    // Unregister metrics
    s.registry.Unregister(s.name, "requests")
    s.registry.Unregister(s.name, "response_time")
    s.registry.Unregister(s.name, "active_users")
    s.registry.Unregister(s.name, "errors")

    s.coreMetrics.RecordServiceStatus(s.name, 0) // stopped
    log.Printf("Service %s stopped", s.name)
    return nil
}

func (s *MonitoredService) metricsLoop() {
    defer s.wg.Done()

    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            s.generateMetrics()
        case <-s.stopCh:
            return
        }
    }
}

func (s *MonitoredService) generateMetrics() {
    // Simulate request processing
    numRequests := rand.Intn(10) + 1

    for i := 0; i < numRequests; i++ {
        start := time.Now()

        // Simulate work
        workTime := time.Duration(rand.Intn(100)) * time.Millisecond
        time.Sleep(workTime)

        // Record metrics
        s.requestCounter.Inc()
        s.responseTime.Observe(time.Since(start).Seconds())

        // Simulate errors (5% rate)
        if rand.Float32() < 0.05 {
            s.errorRate.Inc()
            s.coreMetrics.RecordError(s.name, "processing_error")
        }

        // Record core metrics
        messageType := fmt.Sprintf("%s.request", s.name)
        s.coreMetrics.RecordMessageReceived(s.name, messageType)
        s.coreMetrics.RecordMessageProcessed(s.name, messageType, "success")
        s.coreMetrics.RecordProcessingDuration(s.name, "request", time.Since(start))
    }

    // Update active users (random walk)
    currentUsers := rand.Intn(1000)
    s.activeUsers.Set(float64(currentUsers))
}

func main() {
    // Create metrics infrastructure
    registry := metric.NewMetricsRegistry()

    server := metric.NewServer(9090, "/metrics", registry, security.Config{})
    log.Printf("Metrics server running at %s", server.Address())
    runtimeCtx := context.Background()
    if err := server.Start(runtimeCtx); err != nil {
        log.Fatalf("Failed to start metrics server: %v", err)
    }

    // Create multiple services
    serviceNames := []string{"auth", "api", "processor", "storage", "notification"}
    var services []*MonitoredService

    for _, name := range serviceNames {
        service, err := NewMonitoredService(name, registry)
        if err != nil {
            log.Fatalf("Failed to create service %s: %v", name, err)
        }

        if err := service.Start(); err != nil {
            log.Fatalf("Failed to start service %s: %v", name, err)
        }

        services = append(services, service)
    }

    log.Printf("All services started. Metrics available at http://localhost:9090/metrics")

    // Run services
    time.Sleep(60 * time.Second)

    // Shutdown all services
    log.Printf("Shutting down services...")
    for _, service := range services {
        service.Stop()
    }

    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := server.Stop(shutdownCtx); err != nil {
        log.Printf("Failed to stop metrics server: %v", err)
    }
    log.Printf("All services stopped")
}
```

## Related Packages

- [`pkg/service`](../service): Service framework with automatic metrics integration
- [`pkg/health`](../health): Health monitoring with metrics recording
- [`pkg/component`](../component): Component framework with performance metrics
- [`pkg/errors`](../errors): Error classification with metrics integration

## License

MIT
