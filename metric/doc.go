// Package metric provides Prometheus-based metrics collection and an HTTP server
// for monitoring and observability.
//
// The package offers a centralized metrics registry managing both core platform
// metrics (service status, message processing, NATS health) and custom
// service-specific metrics. It includes an HTTP server exposing metrics in
// Prometheus format for monitoring system integration.
//
// # Architecture
//
// The package follows a three-layer design:
//
//  1. Core Metrics: Platform-level metrics automatically registered (Metrics type)
//  2. Service Registry: Extensible registration for service-specific metrics (RegisterOrGet)
//  3. HTTP Server: Metrics endpoint with health checks (Server type)
//
// This architecture separates infrastructure concerns (core metrics) from
// application concerns (service-specific metrics) while providing a unified
// metrics endpoint for monitoring systems.
//
// # Basic Usage
//
// Setting up metrics collection and HTTP server:
//
//	registry := metric.NewMetricsRegistry()
//	securityCfg := security.Config{} // Platform security config
//	server := metric.NewServer(9090, "/metrics", registry, securityCfg)
//
//	if err := server.Start(ctx); err != nil {
//	    log.Fatalf("Failed to start metrics server: %v", err)
//	}
//	defer server.Stop(shutdownCtx)
//
//	// Record core platform metrics
//	coreMetrics := registry.CoreMetrics()
//	coreMetrics.RecordServiceStatus("my-service", 2)
//	coreMetrics.RecordMessageProcessed("my-service", "gps.position", "success")
//	coreMetrics.RecordNATSStatus(true)
//
// The metrics server will expose Prometheus-formatted metrics at http://localhost:9090/metrics
// and a health check at http://localhost:9090/health, which answers 200 with the plain-text
// body OK.
//
// # Core Metrics
//
// The package automatically registers core platform metrics (the Metrics type), all
// in the "semengine" namespace:
//
//   - Service lifecycle: semengine_service_status{service} (0=stopped, 1=starting,
//     2=running, 3=stopping, 4=failed)
//   - Message flow: semengine_messages_received_total{service,type},
//     semengine_messages_processed_total{service,type,status},
//     semengine_messages_published_total{service,subject}
//   - Processing performance: semengine_processing_duration_seconds{service,operation}
//   - Errors and health: semengine_errors_total{service,type},
//     semengine_health_status{service}
//   - Logging: semengine_log_entries_total{component,level}, which has no recording
//     method; write it through the LogEntriesTotal field
//   - NATS connectivity: semengine_nats_connected, semengine_nats_rtt_seconds,
//     semengine_nats_reconnects_total, semengine_nats_circuit_breaker
//
// Record core metrics through the registry:
//
//	coreMetrics := registry.CoreMetrics()
//
//	// Service lifecycle tracking
//	coreMetrics.RecordServiceStatus("processor", 2) // 2 = running
//	coreMetrics.RecordHealthStatus("processor", true)
//
//	// Message processing metrics
//	coreMetrics.RecordMessageReceived("processor", "sensor.reading")
//	coreMetrics.RecordMessageProcessed("processor", "sensor.reading", "success")
//	coreMetrics.RecordMessagePublished("processor", "output.subject")
//	coreMetrics.RecordProcessingDuration("processor", "transform", 150*time.Millisecond)
//
//	// NATS connectivity
//	coreMetrics.RecordNATSStatus(true)
//	coreMetrics.RecordNATSRTT(2 * time.Millisecond)
//	coreMetrics.RecordNATSReconnect()
//	coreMetrics.RecordCircuitBreakerState(0) // 0 = closed
//
//	// Error tracking
//	coreMetrics.RecordError("processor", "validation")
//
// # Service-Specific Metrics
//
// Services register custom metrics with RegisterOrGet. There is one collector
// per service/metric key: RegisterOrGet returns it, and the caller uses the
// returned collector, never its own candidate. When RegisterOrGet returns an
// error the collector it returns is the zero value (nil for these types), so
// stop on the error rather than use it; each example below continues with
// `if err != nil { return err }`.
//
//	// Register a counter
//	requestCounter, err := metric.RegisterOrGet(registry, "api-service", "api_requests_total",
//	    prometheus.NewCounter(prometheus.CounterOpts{
//	        Name: "api_requests_total",
//	        Help: "Total number of API requests",
//	    }))
//
//	// Register a gauge
//	activeConnections, err := metric.RegisterOrGet(registry, "websocket-service", "active_connections",
//	    prometheus.NewGauge(prometheus.GaugeOpts{
//	        Name: "active_connections",
//	        Help: "Number of active client connections",
//	    }))
//
//	// Register a histogram
//	queryDuration, err := metric.RegisterOrGet(registry, "database-service", "query_duration_seconds",
//	    prometheus.NewHistogram(prometheus.HistogramOpts{
//	        Name:    "query_duration_seconds",
//	        Help:    "Time spent executing queries",
//	        Buckets: prometheus.DefBuckets,
//	    }))
//
// # Vector Metrics with Labels
//
// Register metrics with labels for multi-dimensional data:
//
//	// Counter with labels
//	httpRequestsVec := prometheus.NewCounterVec(
//	    prometheus.CounterOpts{
//	        Name: "http_requests_total",
//	        Help: "Total HTTP requests by status and method",
//	    },
//	    []string{"status", "method"},
//	)
//	httpRequestsVec, err := metric.RegisterOrGet(registry, "api-service", "http_requests_total", httpRequestsVec)
//	if err != nil {
//	    return err
//	}
//
//	// Use the metric with specific label values
//	httpRequestsVec.WithLabelValues("200", "GET").Inc()
//	httpRequestsVec.WithLabelValues("404", "POST").Inc()
//
//	// Gauge with labels
//	cacheItemsVec := prometheus.NewGaugeVec(
//	    prometheus.GaugeOpts{
//	        Name: "cache_items",
//	        Help: "Number of items in cache by type",
//	    },
//	    []string{"cache_type"},
//	)
//	cacheItemsVec, err = metric.RegisterOrGet(registry, "cache-service", "cache_items", cacheItemsVec)
//
//	// Histogram with labels
//	requestDurationVec := prometheus.NewHistogramVec(
//	    prometheus.HistogramOpts{
//	        Name:    "request_duration_seconds",
//	        Help:    "Request duration by endpoint",
//	        Buckets: []float64{.001, .01, .1, 1, 10},
//	    },
//	    []string{"endpoint"},
//	)
//	requestDurationVec, err = metric.RegisterOrGet(registry, "api-service", "request_duration_seconds",
//	    requestDurationVec)
//
// # HTTP Server
//
// The metrics server provides three endpoints:
//
//   - GET / - HTML page with links to metrics and health endpoints
//   - GET /metrics - Prometheus-formatted metrics (default path, configurable)
//   - GET /health - 200 with the plain-text body OK
//
// Server configuration:
//
//	// Default configuration (port 9090, path /metrics)
//	securityCfg := security.Config{} // Platform security config
//	server := metric.NewServer(0, "", registry, securityCfg)
//
//	// Custom configuration
//	server := metric.NewServer(8080, "/prometheus", registry, securityCfg)
//
//	// Start server (binds synchronously, then serves in the background)
//	if err := server.Start(ctx); err != nil {
//	    log.Fatalf("Failed to start metrics server: %v", err)
//	}
//
//	// Stop server and wait for its serving goroutine to exit
//	if err := server.Stop(shutdownCtx); err != nil {
//	    log.Printf("Error stopping server: %v", err)
//	}
//
// The health endpoint answers every request that reaches it with status 200 and
// the plain-text body OK; it reports that the server is serving, nothing more.
// Once Stop has begun, a request is refused with 503 instead.
//
// # Prometheus Integration
//
// The package uses the official Prometheus Go client library and exposes
// metrics in OpenMetrics format. Configure Prometheus to scrape the endpoint:
//
//	# prometheus.yml
//	scrape_configs:
//	  - job_name: 'semengine'
//	    static_configs:
//	      - targets: ['localhost:9090']
//	    metrics_path: '/metrics'
//	    scrape_interval: 15s
//
// All core metrics use the namespace "semengine" and appropriate subsystems:
//   - semengine_service_status{service="..."}
//   - semengine_messages_processed_total{service="...",type="...",status="..."}
//   - semengine_nats_connected
//
// Service-specific metrics use the metric name as provided during registration.
//
// # Registering From a Service
//
// A service receives the *MetricsRegistry and keeps the collectors
// RegisterOrGet returns. A recreated service registering the same keys gets the
// same collectors back, so its writes are gathered:
//
//	type MyService struct {
//	    operations prometheus.Counter
//	}
//
//	func NewMyService(registry *metric.MetricsRegistry) (*MyService, error) {
//	    operations, err := metric.RegisterOrGet(registry, "my-service", "operations_total",
//	        prometheus.NewCounter(prometheus.CounterOpts{
//	            Name: "operations_total",
//	            Help: "Total operations",
//	        }))
//	    if err != nil {
//	        return nil, err
//	    }
//	    return &MyService{operations: operations}, nil
//	}
//
// A test gives the service its own registry from NewMetricsRegistry and reads
// what the service recorded through PrometheusRegistry().Gather().
//
// # Thread Safety
//
// All registry operations are thread-safe:
//   - Registration methods use mutex protection
//   - Metric recording is lock-free (Prometheus guarantee)
//   - CoreMetrics() returns a thread-safe shared instance
//   - PrometheusRegistry() is safe for concurrent access
//
// Example concurrent usage:
//
//	registry := metric.NewMetricsRegistry()
//	coreMetrics := registry.CoreMetrics()
//
//	// Safe to call from multiple goroutines
//	go coreMetrics.RecordMessageProcessed("service-1", "event", "success")
//	go coreMetrics.RecordMessageProcessed("service-2", "event", "success")
//	go coreMetrics.RecordMessageProcessed("service-3", "event", "failed")
//
// # Error Handling
//
// RegisterOrGet returns a fatal error, stores nothing and leaves the canonical
// collector untouched for:
//
//   - A nil or typed-nil candidate
//   - A key already registered with another concrete type, help text or labels
//   - A descriptor another key, a core metric or a directly registered collector
//     already owns
//
// Registering the same key again with an identical collector is not an error:
// it returns the collector already registered.
//
// Example error handling:
//
//	counter, err := metric.RegisterOrGet(registry, "service", "test",
//	    prometheus.NewCounter(prometheus.CounterOpts{Name: "test", Help: "Test counter"}))
//	if err != nil {
//	    log.Fatalf("Failed to register metric: %v", err)
//	}
//	counter.Inc()
//
// The Server.Start(ctx) method returns errors for:
//
//   - A nil or already-ended context
//   - Server instance already used (Server is one-shot)
//   - Nil registry
//   - mTLS enabled while server TLS is disabled
//   - TLS configuration that cannot be loaded
//   - HTTP server failures (port in use, permission denied)
//
// # Testing
//
// The package includes comprehensive tests:
//
//   - Unit tests: Core metrics recording, registry operations
//   - Integration tests: Full registry lifecycle, Prometheus gathering
//   - Race detection: Concurrent access patterns verified
//
// Example test using the registry:
//
//	func TestMyService_Metrics(t *testing.T) {
//	    registry := metric.NewMetricsRegistry()
//	    service, err := NewMyService(registry)
//	    require.NoError(t, err)
//
//	    // Perform operations
//	    service.DoWork()
//
//	    // Verify metrics through what the registry gathers
//	    families, err := registry.PrometheusRegistry().Gather()
//	    require.NoError(t, err)
//	    // Find operations_total in families and check its value
//	}
//
// # Performance Considerations
//
// Metric recording performance:
//   - Counter.Inc(): ~100ns per operation (lock-free)
//   - Gauge.Set(): ~100ns per operation (lock-free)
//   - Histogram.Observe(): ~150ns per operation (bucket lookup)
//
// Registry operations:
//   - Registration: O(1) map insert with mutex
//   - Gathering: O(n) for n registered metrics
//
// Memory usage:
//   - Core metrics: ~2KB base overhead
//   - Per service metric: ~200 bytes
//   - Vector metrics: ~200 bytes + (100 bytes × number of label combinations)
//
// The HTTP server adds minimal overhead (~1MB base) and handles Prometheus
// scraping efficiently with streaming responses.
//
// # Architecture Integration
//
// Within this repository, natsclient (WithMetrics, for JetStream metrics) and
// internal/cache (WithMetrics, WithCoalescingMetrics) register their collectors
// in a MetricsRegistry.
//
// Data flow:
//
//	Component → Core Metrics → Prometheus Registry → HTTP Server → Prometheus
//
// # Design Decisions
//
// Centralized Registry: Chose centralized registry over distributed collectors
// to ensure consistent metric namespace, prevent duplication, and enable
// runtime metric discovery.
//
// Core vs Service Metrics: Separated platform-level metrics (core) from
// service-specific metrics to distinguish infrastructure health from
// application health.
//
// Prometheus Direct Integration: Used official Prometheus client rather than
// abstraction to leverage native features, avoid wrapper overhead, and ensure
// compatibility with Prometheus ecosystem.
//
// Synchronous Bind in Server.Start(ctx): Start reports listener ownership or bind
// failure before returning. Server instances are one-shot. Stop(ctx) attempts
// graceful shutdown within the caller's budget; failure or expiry triggers a
// force-close, after which Stop waits for the exact serving goroutine and for
// every request the server admitted, and it returns the context's error
// whenever its context has ended; a later Stop then waits again. A request that
// reaches the handler after Stop has begun is refused with 503. Restart uses a
// freshly constructed Server.
//
// # Examples
//
// Complete service integration:
//
//	package main
//
//	import (
//	    "context"
//	    "log"
//	    "time"
//
//	    "github.com/c360studio/semengine/metric"
//	    "github.com/c360studio/semengine/pkg/security"
//	    "github.com/prometheus/client_golang/prometheus"
//	)
//
//	func main() {
//	    // Create metrics registry
//	    registry := metric.NewMetricsRegistry()
//
//	    // Start metrics server
//	    securityCfg := security.Config{} // Platform security config
//	    server := metric.NewServer(9090, "/metrics", registry, securityCfg)
//	    runtimeCtx := context.Background()
//	    if err := server.Start(runtimeCtx); err != nil {
//	        log.Fatalf("Failed to start metrics server: %v", err)
//	    }
//	    defer func() {
//	        shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
//	        defer cancel()
//	        if err := server.Stop(shutdownCtx); err != nil {
//	            log.Printf("Failed to stop metrics server: %v", err)
//	        }
//	    }()
//
//	    // Get core metrics
//	    coreMetrics := registry.CoreMetrics()
//
//	    // Register service-specific metric
//	    operationCounter, err := metric.RegisterOrGet(registry, "my-service", "operations_total",
//	        prometheus.NewCounter(prometheus.CounterOpts{
//	            Name: "operations_total",
//	            Help: "Total operations performed",
//	        }))
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//
//	    // Record service status
//	    coreMetrics.RecordServiceStatus("my-service", 2) // running
//
//	    // Simulate work
//	    for i := 0; i < 100; i++ {
//	        operationCounter.Inc()
//	        coreMetrics.RecordMessageProcessed("my-service", "operation", "success")
//	        time.Sleep(100 * time.Millisecond)
//	    }
//	}
//
// For more examples and detailed usage, see the README.md in this directory.
package metric
