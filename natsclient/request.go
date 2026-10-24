// Package natsclient provides request/reply pattern support for NATS.
package natsclient

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/c360studio/semengine/pkg/errs"
)

// DefaultRequestTimeout is the default timeout for request/reply operations.
const DefaultRequestTimeout = 5 * time.Second

// nilContextError is the refusal an exported Client method returns for a nil context, before any
// client state, callback or NATS call is reached (developer contract, Context ownership; Codex F32).
func nilContextError(method string) error {
	return nilContextErrorOf("Client", method)
}

// nilContextErrorOf is nilContextError for an exported function or a method of another type.
func nilContextErrorOf(component, method string) error {
	return errs.WrapInvalid(errors.New("nil context"), component, method, "missing context")
}

// DefaultRequestHandlerTimeout bounds a single inbound request-handler
// invocation (SubscribeForRequests). It caps how long a handler may run
// before its context is cancelled, so a wedged or pathologically slow
// handler cannot pin a NATS delivery goroutine indefinitely. This is the
// CI/default value and MUST NOT change without weighing every request
// handler in the tree; slow-by-design handlers (e.g. an LLM answer-synthesis
// path that legitimately needs >30s) raise it per deployment via
// the SEMENGINE_NATS_REQUEST_HANDLER_TIMEOUT
// environment variable rather than editing this constant.
const DefaultRequestHandlerTimeout = 30 * time.Second

// requestHandlerTimeoutEnv is the deployment-level override for the
// per-message request-handler timeout. Parsed as a Go duration (e.g.
// "150s"); an unset or unparseable value leaves DefaultRequestHandlerTimeout
// in force.
const requestHandlerTimeoutEnv = "SEMENGINE_NATS_REQUEST_HANDLER_TIMEOUT"

// resolveRequestHandlerTimeoutFromEnv returns the request-handler timeout
// implied by requestHandlerTimeoutEnv, or DefaultRequestHandlerTimeout when
// the variable is unset or malformed. A malformed value is logged and
// ignored (fail-safe to the default) rather than failing construction.
func resolveRequestHandlerTimeoutFromEnv() time.Duration {
	raw := os.Getenv(requestHandlerTimeoutEnv)
	if raw == "" {
		return DefaultRequestHandlerTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		slog.Default().Warn("natsclient: ignoring invalid request-handler timeout override",
			slog.String("env", requestHandlerTimeoutEnv),
			slog.String("value", raw),
			slog.Duration("using", DefaultRequestHandlerTimeout))
		return DefaultRequestHandlerTimeout
	}
	return d
}

// Readiness-gated read defaults (ADR-060 sibling doctrine, third bucket — see
// SemStreams' docs/operations/07-nats-request-retry.md at the pin; not carried
// here). A readiness-gated read tolerates a not-yet-subscribed responder at
// cold start / after reconnect: it retries with a SHORT per-attempt timeout up
// to a bounded TOTAL budget, returning the first reply. Because the
// per-attempt timeout is short and the total is bounded, a genuinely hung
// responder fails within the budget rather than a full-timeout×N
// storm — so it does NOT mask a hung responder the way retrying a full-timeout
// query would. Distinct from RequestClassified (steady-state query: timeout =
// real signal, no retry) and RequestWithRetryClassified (an operation whose
// contract authorizes redelivery: retry on no reply, at full timeout).
const (
	// DefaultReadinessProbeTimeout bounds each attempt. Short so a
	// not-yet-subscribed responder fails fast and retries instead of consuming a
	// full query timeout per attempt.
	DefaultReadinessProbeTimeout = 2 * time.Second
	// DefaultReadinessBudget bounds the TOTAL wall-clock spent waiting for the
	// responder to come up before the error surfaces. Neither bounds a wait
	// behind a write the server is not reading: such a write holds an attempt
	// until it completes or nats.go's write timeout (one minute by default,
	// WithWriteTimeout) ends it (P13; ADR-112).
	DefaultReadinessBudget = 30 * time.Second

	readinessInitialBackoff = 100 * time.Millisecond
	readinessMaxBackoff     = 1 * time.Second
)

// requestMsgReady is the readiness-retry loop behind RequestReadyClassified
// (errors.go), which runs ClassifyReply on the raw *nats.Msg it returns. It
// retries with probeTimeout per attempt up to a total budget; zero
// probeTimeout/budget use the Default* values above.
//
// It retries only on TRANSPORT failure (no reply received: no-responders or
// probe timeout). Any received reply — including a handler-error reply — stops
// the loop, because a reply means the responder is UP; the readiness contract is
// satisfied and the (possibly-error) reply is returned for classification. Only
// a truly-silent (never-replies) responder is retried, and only until the budget
// is spent.
func (c *Client) requestMsgReady(
	ctx context.Context, subject string, data []byte, probeTimeout, budget time.Duration,
) (*nats.Msg, error) {
	if ctx == nil {
		return nil, nilContextError("RequestReadyClassified")
	}
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil || !conn.IsConnected() {
		return nil, ErrNotConnected
	}

	if probeTimeout <= 0 {
		probeTimeout = DefaultReadinessProbeTimeout
	}
	if budget <= 0 {
		budget = DefaultReadinessBudget
	}

	// Auto-generate trace once for all attempts.
	if _, ok := TraceContextFromContext(ctx); !ok {
		ctx = ContextWithTrace(ctx, NewTraceContext())
	}

	overallCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	backoff := readinessInitialBackoff
	var lastErr error
	for {
		attemptCtx, attemptCancel := context.WithTimeout(overallCtx, probeTimeout)
		msg := nats.NewMsg(subject)
		msg.Data = data
		InjectTrace(ctx, msg)
		reply, err := conn.RequestMsgWithContext(attemptCtx, msg)
		attemptCancel()

		if err == nil {
			return reply, nil
		}
		lastErr = err

		// Wait before the next probe, bounded by the overall readiness budget.
		select {
		case <-overallCtx.Done():
			// %w preserves errors.Is(…, nats.ErrNoResponders) for callers that
			// want to distinguish a never-appeared responder from a hung one.
			return nil, fmt.Errorf("readiness budget %s exhausted: %w", budget, lastErr)
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > readinessMaxBackoff {
			backoff = readinessMaxBackoff
		}
	}
}

// RequestWithHeaders performs a request/reply operation with custom headers,
// converted from the map to NATS message headers. It returns the reply message
// as received, headers included, and does not classify it: a
// SubscribeForRequests handler's error arrives as a reply whose X-Status header
// is "error", with a nil error, not as err. Callers run ClassifyReply on the
// reply; a caller that sends no headers uses RequestClassified, which does both.
func (c *Client) RequestWithHeaders(
	ctx context.Context,
	subject string,
	data []byte,
	headers map[string]string,
	timeout time.Duration,
) (*nats.Msg, error) {
	if ctx == nil {
		return nil, nilContextError("RequestWithHeaders")
	}
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil || !conn.IsConnected() {
		return nil, ErrNotConnected
	}

	// Use default timeout if not specified
	if timeout == 0 {
		timeout = DefaultRequestTimeout
	}

	// Auto-generate trace if none exists
	if _, ok := TraceContextFromContext(ctx); !ok {
		ctx = ContextWithTrace(ctx, NewTraceContext())
	}

	// Create a context with timeout
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build the message with headers
	msg := nats.NewMsg(subject)
	msg.Data = data

	// Add headers if provided
	if len(headers) > 0 {
		msg.Header = make(nats.Header)
		for key, value := range headers {
			msg.Header.Set(key, value)
		}
	}

	// Inject trace headers (in addition to user headers)
	InjectTrace(ctx, msg)

	// Perform the request
	reply, err := conn.RequestMsgWithContext(reqCtx, msg)
	if err != nil {
		return nil, err
	}

	return reply, nil
}

// Reply sends a reply to a request message.
// This is typically used by service handlers to respond to requests.
func (c *Client) Reply(ctx context.Context, replyTo string, data []byte) error {
	if ctx == nil {
		return nilContextError("Reply")
	}
	if replyTo == "" {
		return nil // No reply requested
	}

	return c.Publish(ctx, replyTo, data)
}

// ReplyWithHeaders sends a reply with custom headers.
func (c *Client) ReplyWithHeaders(ctx context.Context, replyTo string, data []byte, headers map[string]string) error {
	if ctx == nil {
		return nilContextError("ReplyWithHeaders")
	}
	if replyTo == "" {
		return nil // No reply requested
	}

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil || !conn.IsConnected() {
		return ErrNotConnected
	}

	// Auto-generate trace if none exists
	if _, ok := TraceContextFromContext(ctx); !ok {
		ctx = ContextWithTrace(ctx, NewTraceContext())
	}

	// Build the reply message with headers
	msg := nats.NewMsg(replyTo)
	msg.Data = data

	if len(headers) > 0 {
		msg.Header = make(nats.Header)
		for key, value := range headers {
			msg.Header.Set(key, value)
		}
	}

	// Inject trace headers
	InjectTrace(ctx, msg)

	return conn.PublishMsg(msg)
}

// SubscribeForRequests subscribes to a subject and handles request/reply patterns.
// The handler receives the message data and reply subject, and should return
// the response data or an error.
// Returns the Subscription so the caller can unsubscribe when done.
// This is a convenience method for implementing request/reply services.
// Once Close has begun it returns nats.ErrConnectionClosed and subscribes
// nothing. Close joins every running invocation of handler.
//
// When SubscribeForRequests returns a subscription, the server has read its
// SUB: the call sends a PING on the connection it subscribed on and waits for
// the PONG, which the server sends once it has read the SUB, so a request sent
// to that server from any connection after the call returns finds the handler,
// unless the server refused the SUB (for permissions, or for the connection's
// limit on subscriptions). A refused SUB is not reported here: the call returns
// the subscription and nil, and a client that connected with Connect logs the
// refusal at error level. The wait for the PONG ends when the server answers,
// when ctx ends, or when DefaultRequestTimeout has passed, whichever comes
// first. ctx does not end a wait for the connection itself: while the server is
// not reading and the connection's send buffer is full, the call, like every
// call that writes on the connection, waits for each held write on the
// connection, its own or another goroutine's, until that write completes or
// nats.go's write timeout (one minute by default) ends it, so it can be held a
// minute or more. If ctx has already ended, the call sends no SUB and returns a
// transient error matching ctx's error. Nothing here says when a responder in
// another component has subscribed, or what holds after a reconnect or across
// the servers of a cluster.
//
// When the wait does not complete, or Close has begun when it ends, the call
// ends the subscription (it unsubscribes it, so it is not restored on a
// reconnect; on a connection Close is draining or has closed, Close ends it) and
// returns no subscription and the first of these errors that holds:
//   - nats.ErrConnectionClosed, if Close has begun;
//   - a transient error matching context.Canceled or context.DeadlineExceeded,
//     if ctx ended or DefaultRequestTimeout passed;
//   - a transient error matching ErrNotConnected, and not
//     nats.ErrConnectionClosed, if the connection was lost or closed by anything
//     but Close.
//
// It does not wait for a running invocation of handler; Close joins that
// invocation.
//
// ctx is used for the setup only (the subscribe and the round trip above) and
// is not retained (owner ruling, #9 comment 5994720412 item 2). Each handler
// invocation's context descends from a context the client owns for this
// subscription, not from ctx: ending ctx after the call returned does not end a
// handler. The client cancels that context after Close has begun, also after
// Unsubscribe while a handler still runs, and once delivery has ended and the
// last invocation has returned. A handler's
// context also ends when the per-request handler timeout passes, and carries
// the trace from the request's headers when it has one, and no value of ctx.
// Unsubscribe stops delivery but does not wait for a running handler;
// Subscription.Drain returning nil is what proves every handler has returned.
func (c *Client) SubscribeForRequests(
	ctx context.Context,
	subject string,
	handler func(ctx context.Context, data []byte) ([]byte, error),
) (*Subscription, error) {
	// Refused here: subscribeOwned reads ctx for the setup.
	if ctx == nil {
		return nil, nilContextError("SubscribeForRequests")
	}
	handlerCtx, cancelHandlers := clientOwnedHandlerContext()
	return c.subscribeOwned(ctx, "SubscribeForRequests", subject, cancelHandlers, func(conn *nats.Conn) nats.MsgHandler {
		return c.requestCallback(handlerCtx, conn, subject, handler)
	})
}

// requestCallback is SubscribeForRequests' native callback for a subscription
// made on conn. ctx is the parent of every invocation's context: the context the
// client owns for the subscription.
func (c *Client) requestCallback(
	ctx context.Context,
	conn *nats.Conn,
	subject string,
	handler func(ctx context.Context, data []byte) ([]byte, error),
) nats.MsgHandler {
	return func(msg *nats.Msg) {
		// Extract trace from incoming request
		msgCtx := ctx
		if tc := ExtractTrace(msg); tc != nil {
			msgCtx = ContextWithTrace(ctx, tc)
		}

		// Create per-message context with timeout. Configurable (default
		// DefaultRequestHandlerTimeout=30s) so slow-by-design handlers such as
		// the LLM answer-synthesis path can raise it per deployment without
		// changing the framework default. See resolveRequestHandlerTimeoutFromEnv.
		msgCtx, cancel := context.WithTimeout(msgCtx, c.requestHandlerTimeout)
		defer cancel()

		// Call the handler
		response, err := handler(msgCtx, msg.Data)
		if err != nil {
			// Send a header-classified error reply (ADR-060). The reply carries
			// X-Status / X-Error-Class / X-Error-Code headers and the
			// {message, detail} envelope body; a RequestClassified caller
			// reconstructs the typed error. RespondError returns
			// errMissingReplySubject when the inbound message had no reply
			// subject — that's expected for fire-and-forget patterns, so
			// swallow. Any other publish failure is operator-visible (queue
			// full, connection torn down mid-reply, etc.) and needs to surface,
			// not silently swallow.
			if respErr := RespondError(msg, err); respErr != nil && !errors.Is(respErr, errMissingReplySubject) {
				c.logger.Error("natsclient: failed to publish handler-error reply",
					"subject", subject,
					"handler_err", err.Error(),
					"publish_err", respErr.Error())
			}
			return
		}

		// Send successful response
		if msg.Reply != "" {
			if respErr := msg.Respond(response); respErr != nil {
				if errors.Is(respErr, nats.ErrMaxPayload) {
					refusal := errs.ClassifiedCodeDetail(
						errs.ErrorInvalid,
						ErrorCodeResponseTooLarge,
						map[string]any{
							"response_bytes": int64(len(response)),
							// This is the same connection whose publish rejected
							// the response. MaxPayload has no connected-state gate,
							// so the diagnostic remains available while reconnecting.
							"max_payload": conn.MaxPayload(),
						},
						errors.New("response exceeds active NATS maximum payload"),
					)
					if publishErr := RespondError(msg, refusal); publishErr != nil {
						c.logger.Error("natsclient: failed to publish response-too-large reply",
							"subject", subject,
							"publish_err", publishErr.Error())
					}
					return
				}
				c.logger.Error("natsclient: failed to publish handler success reply",
					"subject", subject,
					"err", respErr.Error())
			}
		}
	}
}
