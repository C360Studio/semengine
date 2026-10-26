package graphingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/natsclient"
	"github.com/c360studio/semengine/pkg/errs"
	"github.com/nats-io/nats.go/jetstream"
)

// ============================================================================
// Subscription Management
// ============================================================================

// setupSubscriptions sets up JetStream consumers for input ports
func (c *Component) setupSubscriptions(ctx, submitCtx context.Context) error {
	for _, port := range c.inputs {
		facts, err := port.Facts()
		if err != nil {
			return err
		}
		if facts.Kind() != component.PortKindJetStream {
			continue
		}

		if err := c.setupJetStreamConsumer(ctx, submitCtx, port); err != nil {
			return errs.Wrap(err, "Component", "setupSubscriptions",
				fmt.Sprintf("JetStream consumer for %s", port.Name))
		}
	}
	return nil
}

// defaultIngestAckWait is the acknowledgement wait graph-ingest's consumer is created with
// when its input port declares no ack_wait. It is passed to the consumer, so natsclient's
// own fallback is never reached; it is the same 30 s, so a port that declares nothing keeps
// the wait it had before graph-ingest read the port (design D8, "Long apply").
const defaultIngestAckWait = 30 * time.Second

// ingestDeliveryTiming returns the acknowledgement wait for an input port's consumer and
// the interval at which an application is to tell the server it is still in progress: the
// port's ack_wait and heartbeat_interval, or graph-ingest's defaults, 30 s and a quarter of
// the wait in effect. A quarter leaves half the wait as margin under natsclient's ceiling,
// which is half the wait, so a declared ack_wait alone never fails Start.
func ingestDeliveryTiming(declared component.ConsumerConfig) (ackWait, heartbeat time.Duration) {
	ackWait = declared.AckWait
	if ackWait == 0 {
		ackWait = defaultIngestAckWait
	}
	heartbeat = declared.HeartbeatInterval
	if heartbeat == 0 {
		heartbeat = ackWait / 4
	}
	return ackWait, heartbeat
}

// setupJetStreamConsumer creates a JetStream consumer for an input port
func (c *Component) setupJetStreamConsumer(ctx, submitCtx context.Context, port component.Port) error {
	facts, err := port.Facts()
	if err != nil {
		return err
	}
	stream, ok := facts.Stream()
	if !ok || len(stream.Subjects()) != 1 {
		return fmt.Errorf("port %s must declare one JetStream subject", port.Name)
	}
	subject := stream.Subjects()[0]
	streamName := stream.Name()
	// The operator's import declaration is read from the PORT, once, here — a
	// message can never claim it (ADR-102 d5).
	importLane := stream.Import()

	// Wait for stream to be available
	waitForStream := c.waitForStream
	if c.waitForStreamInput != nil {
		waitForStream = c.waitForStreamInput
	}
	// The stream's generation is its creation time on the server, from the stream info the wait
	// reads (design D8, #15), never from configuration. It is read once, here, before the consumer
	// is bound, and every input this consumer delivers carries it. A stream deleted and created again
	// between this read and the bind is a declared limitation (graph-ingest-recovery, "Replay
	// protection uses the generation read before the bind").
	created, err := waitForStream(ctx, streamName)
	if err != nil {
		return fmt.Errorf("stream %s not available: %w", streamName, err)
	}
	if created.IsZero() {
		return fmt.Errorf("stream %s reports no creation time; its generations cannot be told apart", streamName)
	}
	generation := uint64(created.UnixNano())

	// Generate unique consumer name
	sanitizedSubject := strings.ReplaceAll(subject, ".", "-")
	sanitizedSubject = strings.ReplaceAll(sanitizedSubject, "*", "all")
	sanitizedSubject = strings.ReplaceAll(sanitizedSubject, ">", "wildcard")
	consumerName := fmt.Sprintf("graph-ingest-%s", sanitizedSubject)

	c.logger.Debug("Setting up JetStream consumer",
		slog.String("stream", streamName),
		slog.String("consumer", consumerName),
		slog.String("filter_subject", subject))

	// Get consumer config from port definition (allows user configuration)
	// graph-ingest is idempotent (ENTITY_STATES merge/CAS overwrites), so it
	// defaults to "all": it MUST catch up on entities published before its
	// consumer bound. A JSON config omitting deliver_policy would otherwise fall
	// to the framework "new" default and silently drop the first entities (the
	// startup first-message race). An explicit deliver_policy still wins.
	consumerCfg, consumerErr := component.GetConsumerConfig(port)
	if consumerErr != nil {
		return fmt.Errorf("resolve JetStream consumer config for port %q: %w", port.Name, consumerErr)
	}
	if stream.DeliverPolicy() == "" {
		consumerCfg.DeliverPolicy = "all"
	}

	ackWait, heartbeat := ingestDeliveryTiming(consumerCfg)
	cfg := natsclient.StreamConsumerConfig{
		StreamName:    streamName,
		ConsumerName:  consumerName,
		FilterSubject: subject,
		DeliverPolicy: consumerCfg.DeliverPolicy,
		AckPolicy:     consumerCfg.AckPolicy,
		MaxDeliver:    consumerCfg.MaxDeliver,
		AckWait:       ackWait,
		MaxAckPending: consumerCfg.MaxAckPending, // SemStreams gh#480: 0 leaves inherited/default/capped policy to NATS
		AutoCreate:    false,
	}
	// The pair is checked by natsclient's own rules for heartbeat delivery, once, before
	// the consumer is created, so a pair those rules refuse fails Start. The policy the
	// check builds is discarded; its work never runs. Each input carries cfg and the
	// interval to its lane, which builds the input's own policy (processIngest).
	checkOnly := func(context.Context, []byte) (natsclient.DeliveryDecision, error) {
		return natsclient.DeliveryDecisionInvalid, errors.New("graph-ingest: the start-time timing check runs no work")
	}
	if _, err := natsclient.ValidateHeartbeatDeliveryPolicy(ctx, cfg, heartbeat,
		natsclient.ImmediateDeliveryRetry(), checkOnly); err != nil {
		return fmt.Errorf("port %s: ack_wait %s and heartbeat_interval %s: %w", port.Name, ackWait, heartbeat, err)
	}

	// The closure submits to this pool and submitCtx, both from this Start, never to
	// c.ingestPool. A consumer whose setup fails is not recorded for the rollback to drain,
	// and its callback can run after the rollback cleared c.ingestPool (checkpoint 3 B2).
	// By then this pool has shut down, so the submit is refused and the message is Nak'd.
	pool := c.ingestPool
	consumeStream := c.natsClient.ConsumeStreamWithConfig
	if c.consumeStream != nil {
		consumeStream = c.consumeStream
	}
	handle, err := consumeStream(ctx, natsclient.PortConsumerContext{Component: c.name, Port: port.Name}, cfg, func(_ context.Context, msg jetstream.Msg) {
		// Every delivery is one input message on DataFlow, a redelivery and a message
		// refused below included.
		c.lifecycle.CountInput(len(msg.Data()))
		// SemStreams ADR-072: the consume closure decodes ONCE and submits to the keyed pool;
		// the redelivery guard, apply, and ack move into processIngest (run on a
		// pool lane, not this consumer goroutine). Metadata carries the stream name
		// + sequence the guard keys on; a message without it can't be keyed/guarded.
		meta, metaErr := msg.Metadata()
		if metaErr != nil {
			c.logger.Warn("graph-ingest: message missing JetStream metadata; dropping",
				slog.String("subject", subject), slog.Any("error", metaErr))
			c.lifecycle.CountError()
			if ackErr := msg.Ack(); ackErr != nil {
				c.logger.Error("Failed to ack metadata-less message", slog.Any("error", ackErr))
			}
			return
		}
		// SemStreams gh#480 queue-wait (message age when we reach it). Host clock skew can make
		// this marginally negative on a co-located deploy — harmless (skews _sum only).
		if !meta.Timestamp.IsZero() {
			c.ingestLag.Observe(time.Since(meta.Timestamp).Seconds())
		}
		// Decode + extract ONCE (the submit's key is the already-parsed entity.ID; the
		// lane reuses the parsed entity — no double parse). A decode/validate/extract
		// failure, a fenced payload panic included, is a poison message: counted,
		// logged and terminated, never returned for redelivery (design D8 Q13).
		entity, derr := c.decodeEntity(msg.Subject(), msg.Data())
		if derr != nil {
			// A refused statement (design D15) is this message's own fault and can never be
			// admitted: it is terminated, counted and logged as applyIngest does a
			// structurally invalid candidate, in the contract-rejection counts, not as poison.
			var refusal *statementRefusal
			if errors.As(derr, &refusal) {
				c.recordStructuralRejection("graphable", derr)
				if termErr := msg.Term(); termErr != nil {
					c.logger.Error("Failed to terminate structurally invalid ingest", slog.Any("error", termErr))
				}
				return
			}
			// The error names the payload type where the message has one.
			c.logger.Warn("graph-ingest: poison message; terminating",
				slog.String("subject", msg.Subject()), slog.Any("error", derr))
			c.lifecycle.CountError()
			if termErr := msg.Term(); termErr != nil {
				c.logger.Error("Failed to terminate poison message", slog.Any("error", termErr))
			}
			return
		}
		// Submit on the dedicated submit ctx (NOT msgCtx — a block past AckWait would
		// otherwise surface as an ignored error → message neither enqueued nor acked).
		// A submit failure MUST Nak so the server redelivers (SemStreams ADR-072 B1), never
		// silent-drop.
		work := ingestWork{
			entity:      entity,
			msg:         msg,
			consumer:    cfg,
			heartbeat:   heartbeat,
			entityID:    entity.ID,
			stream:      meta.Stream,
			seq:         meta.Sequence.Stream,
			generation:  generation,
			deliveredAt: meta.Timestamp,
			subject:     subject,
			importLane:  importLane,
		}
		if serr := pool.SubmitBlocking(submitCtx, work.entityID, work); serr != nil {
			if nakErr := msg.Nak(); nakErr != nil {
				c.logger.Error("Failed to Nak after submit failure", slog.Any("error", nakErr))
			}
		}
	})
	if err != nil {
		return fmt.Errorf("consumer setup failed for stream %s: %w", streamName, err)
	}
	c.consumers = append(c.consumers, graphIngestConsumerBinding{handle: handle})

	// Record the consumer for the readiness tick ONLY after the bind committed, so
	// the backlog sum never asks about a consumer that failed to bind (which would
	// degrade the envelope on a consumer that does not exist).
	c.registerBoundConsumer(streamName, consumerName)

	// Capture this port's contribution to BootstrapScope NOW, while the backlog is
	// still whole. See recordBindBacklog: reading it at the first status tick lets a
	// fast-draining backlog report "nothing to do".
	c.recordBindBacklog(ctx, streamName, consumerName)

	c.logger.Debug("graph-ingest subscribed (JetStream)",
		slog.String("subject", subject),
		slog.String("stream", streamName))
	return nil
}

// waitForStream waits for a JetStream stream to be available and returns its
// creation time as the server reports it, which is the stream's generation (#15).
func (c *Component) waitForStream(ctx context.Context, streamName string) (time.Time, error) {
	js, err := c.natsClient.JetStream()
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get JetStream context: %w", err)
	}

	maxRetries := 30
	retryInterval := 100 * time.Millisecond
	maxInterval := 2 * time.Second

	for i := 0; i < maxRetries; i++ {
		stream, err := js.Stream(ctx, streamName)
		if err == nil {
			c.logger.Debug("Stream available", slog.String("stream", streamName))
			return stream.CachedInfo().Created, nil
		}

		// Exponential backoff
		c.logger.Debug("Waiting for stream",
			slog.String("stream", streamName),
			slog.Int("attempt", i+1),
			slog.Duration("interval", retryInterval))

		select {
		case <-ctx.Done():
			return time.Time{}, ctx.Err()
		case <-time.After(retryInterval):
			retryInterval = time.Duration(float64(retryInterval) * 1.5)
			if retryInterval > maxInterval {
				retryInterval = maxInterval
			}
		}
	}

	return time.Time{}, fmt.Errorf("stream %s not available after %d retries", streamName, maxRetries)
}
