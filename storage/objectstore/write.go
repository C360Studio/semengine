package objectstore

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/c360studio/semengine/message"
	"github.com/c360studio/semengine/pkg/errs"
)

// handleCore writes one core-NATS input message into store. The input is at-most-once (owner
// ruling 2): core NATS keeps nothing to redeliver, so a refused or failed write is logged and
// counted, and the message is gone.
func (c *Component) handleCore(ctx context.Context, store *Store, msg *nats.Msg) {
	c.lifecycle.CountInput(len(msg.Data))
	if err := c.write(ctx, store, msg.Data); err != nil {
		c.logger.Error("objectstore: write refused or failed; the core-NATS message is dropped",
			slog.String("subject", msg.Subject), slog.Any("error", err))
		c.lifecycle.CountError()
	}
}

// handleJetStream writes one JetStream input delivery into store and settles it by
// classifyWriteDisposition. Every settlement but Ack is logged at error level and counted, as is
// a settlement the server could not be sent. The count comes last, so a reader that waited for it
// also sees the settlement call and the log line.
func (c *Component) handleJetStream(ctx context.Context, store *Store, msg jetstream.Msg) {
	c.lifecycle.CountInput(len(msg.Data()))
	err := c.write(ctx, store, msg.Data())
	var settle func() error
	switch classifyWriteDisposition(ctx.Err(), err) {
	case dispositionAck:
		if ackErr := msg.Ack(); ackErr != nil {
			c.logger.Error("objectstore: stored, but the ack failed; the redelivery stores the body again",
				slog.String("subject", msg.Subject()), slog.Any("error", ackErr))
			c.lifecycle.CountError()
		}
		return
	case dispositionNakShutdown:
		c.logger.Error("objectstore: write ended with its context; NAK for redelivery",
			slog.String("subject", msg.Subject()), slog.Any("error", err))
		settle = func() error { return msg.NakWithDelay(shutdownNakDelay) }
	case dispositionTerm:
		c.logger.Error("objectstore: write refused; delivery terminated",
			slog.String("subject", msg.Subject()), slog.Any("error", err))
		settle = msg.Term
	default:
		c.logger.Error("objectstore: write failed; NAK for redelivery",
			slog.String("subject", msg.Subject()), slog.Any("error", err))
		settle = func() error { return msg.NakWithDelay(transientNakDelay) }
	}
	if settleErr := settle(); settleErr != nil {
		c.logger.Error("objectstore: settling the delivery failed",
			slog.String("subject", msg.Subject()), slog.Any("error", settleErr))
	}
	c.lifecycle.CountError()
}

// write decodes data, stores its ContentStorable payload's body through store and publishes a
// StoredMessage carrying the body's reference. Data that does not decode, a payload that is not
// ContentStorable and a payload StoreContent refuses are invalid; a failed store or publish is
// transient. A failed publish leaves the body stored and unreferenced: on a JetStream input the
// redelivery stores it again under a new key (design D4).
func (c *Component) write(ctx context.Context, store *Store, data []byte) error {
	msg, err := c.decoder.Decode(data)
	if err != nil {
		return errs.WrapInvalid(err, "objectstore", "write", "decode message")
	}
	content, ok := msg.Payload().(message.ContentStorable)
	if !ok {
		return errs.WrapInvalid(
			fmt.Errorf("payload %s is not message.ContentStorable: only bodies are stored", msg.Type().Key()),
			"objectstore", "write", "check payload")
	}
	ref, err := store.StoreContent(ctx, content)
	if err != nil {
		return err
	}
	stored := NewStoredMessage(content, ref, msg.Type().Key())
	encoded, err := message.NewBaseMessage(stored.Schema(), stored, instanceName).MarshalJSON()
	if err != nil {
		return errs.WrapInvalid(err, "objectstore", "write", "marshal StoredMessage")
	}
	if c.stored.jetStream {
		err = c.natsClient.PublishToStream(ctx, c.stored.subject, encoded)
	} else {
		err = c.natsClient.Publish(ctx, c.stored.subject, encoded)
	}
	if err != nil {
		return errs.WrapTransient(err, "objectstore", "write", "publish StoredMessage to "+c.stored.subject)
	}
	return nil
}

// The NAK delays of the settlement table (design D4, the pin's): 30 seconds before retrying a
// failed write, 5 seconds when the handler's context ended. They are not configurable.
const (
	transientNakDelay = 30 * time.Second
	shutdownNakDelay  = 5 * time.Second
)

// writeDisposition is how a JetStream input settles one delivery.
type writeDisposition int

const (
	dispositionAck          writeDisposition = iota // stored and its reference published
	dispositionNakShutdown                          // NakWithDelay(shutdownNakDelay): the handler's context ended
	dispositionTerm                                 // Term: invalid, a retry cannot succeed
	dispositionNakTransient                         // NakWithDelay(transientNakDelay): anything else
)

// classifyWriteDisposition maps a write's outcome onto the settlement table:
//
//	nil error              -> Ack
//	ended context, error   -> NakWithDelay(5s)
//	errs.IsInvalid         -> Term
//	anything else          -> NakWithDelay(30s)
//
// A nil error acks even under an ended context: the body is stored and its reference published,
// and a NAK would only store it again. An error under an ended context is NAKed before its class
// is read, even an invalid one: the error may be the cancellation surfacing through the Store, and
// a Term would drop a message a later delivery could store. An unclassified error is retried.
func classifyWriteDisposition(ctxErr, writeErr error) writeDisposition {
	switch {
	case writeErr == nil:
		return dispositionAck
	case ctxErr != nil:
		return dispositionNakShutdown
	case errs.IsInvalid(writeErr):
		return dispositionTerm
	default:
		return dispositionNakTransient
	}
}
