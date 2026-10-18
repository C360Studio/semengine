package natsclient

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/c360studio/semengine/metric"
)

// ClientOption is a functional option for configuring the Client
type ClientOption func(*Client) error

// WithMaxReconnects sets the maximum number of reconnection attempts (-1 for infinite)
func WithMaxReconnects(maxN int) ClientOption {
	return func(c *Client) error {
		c.maxReconnects = maxN
		return nil
	}
}

// WithReconnectWait sets the wait time between reconnection attempts
func WithReconnectWait(d time.Duration) ClientOption {
	return func(c *Client) error {
		c.reconnectWait = d
		return nil
	}
}

// WithHealthInterval sets the interval for health monitoring
func WithHealthInterval(d time.Duration) ClientOption {
	return func(c *Client) error {
		c.healthInterval = d
		return nil
	}
}

// WithLogger sets a structured logger for the client.
// If nil, defaults to slog.Default().
func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) error {
		if logger == nil {
			logger = slog.Default()
		}
		c.logger = logger
		return nil
	}
}

// WithConnectionLostCallback sets a callback for when connection is completely lost
func WithConnectionLostCallback(fn func(error)) ClientOption {
	return func(c *Client) error {
		c.onConnectionLost = fn
		return nil
	}
}

// WithConnectionLossTimeout configures how long the client tolerates a
// continuous broker outage before the connection-lost callback fires.
//
// When set together with WithConnectionLostCallback, a timer arms on the
// first disconnect; if the connection has not recovered within grace, the
// callback runs once with the original disconnect error. A reconnect
// before the deadline cancels the timer. A non-positive grace disables the
// watchdog (legacy behavior — onConnectionLost never fires automatically).
//
// The callback only signals — it does not call os.Exit or otherwise act on
// the client. Callers decide the policy (graceful shutdown, alerting,
// degraded-mode flag, etc.).
func WithConnectionLossTimeout(grace time.Duration) ClientOption {
	return func(c *Client) error {
		c.connectionLossTimeout = grace
		return nil
	}
}

// WithCredentials sets username and password for authentication
func WithCredentials(username, password string) ClientOption {
	return func(c *Client) error {
		c.username = username
		c.password = password
		return nil
	}
}

// WithName sets the client name for identification
func WithName(name string) ClientOption {
	return func(c *Client) error {
		c.clientName = name
		return nil
	}
}

// WithTimeout sets the connection timeout
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) error {
		c.timeout = d
		return nil
	}
}

// WithWriteTimeout sets how long one write may wait for the server to read it
// before nats.go fails the write (nats.FlusherTimeout). One deadline covers a
// whole flush, so at 10 s a 1 MB message needs about 100 KB/s (design D4). The
// default is nats.go's, one minute. Other calls on the connection wait behind a
// held write (T2; P17).
//
// Set WithPingInterval with it. A write the timeout ends partway leaves the
// connection connected but unable to carry anything, until the stale-connection
// check that PINGs drive closes it and reconnects (T9; P24). With the default
// 30 s ping, when that happens is not promised. d must be positive: NewClient
// refuses any other value as invalid (T9).
func WithWriteTimeout(d time.Duration) ClientOption {
	return func(c *Client) error {
		if d <= 0 {
			return fmt.Errorf("write timeout %v is not positive", d)
		}
		c.writeTimeout = d
		return nil
	}
}

// WithPingInterval sets how often nats.go sends the server a PING
// (nats.PingInterval). The default is 30 s. When PINGs go unanswered, nats.go
// finds the connection stale, closes it and reconnects: at 300 ms that took
// 1.2 s (P20, P24); the timing at other values is not promised.
//
// Set it with WithWriteTimeout. While a write is held this check does not fire
// (P20), so only the write timeout ends that write, and a write it ends partway
// leaves the connection unable to carry anything until this check closes it
// (T9; P24). d must be positive: NewClient refuses any other value as invalid
// (T9).
func WithPingInterval(d time.Duration) ClientOption {
	return func(c *Client) error {
		if d <= 0 {
			return fmt.Errorf("ping interval %v is not positive", d)
		}
		c.pingInterval = d
		return nil
	}
}

// WithMetrics enables JetStream metrics collection using the provided registry.
// Metrics will track streams and consumers created through this client.
//
// Give each client its own registry. A registry holds one client's JetStream
// series: its stream and consumer gauges are set from what that client reads
// from the server. A second client on the same registry writes the same
// collectors, so the two overwrite each other's gauges, and which client's
// value a scrape shows is not promised. No configuration we know of runs two
// clients on one registry.
func WithMetrics(registry *metric.MetricsRegistry) ClientOption {
	return func(c *Client) error {
		if registry == nil {
			return nil // No metrics
		}

		metrics, err := newJetStreamMetrics(registry)
		if err != nil {
			return err
		}

		c.jsMetrics = metrics
		return nil
	}
}
