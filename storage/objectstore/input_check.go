package objectstore

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/c360studio/semengine/graph/readiness"
)

// inputCheckInterval is how often the input check runs: graph-ingest's status tick, so both
// components notice a lost input within the same time.
const inputCheckInterval = readiness.DefaultHeartbeat

// watchedInput is one bound JetStream input the input check observes. Only the check's
// goroutine reads or writes it once Start has handed it over.
type watchedInput struct {
	port     string
	stream   string
	consumer string
	// closed is the input's consume loop's Closed channel. The handle, and the authority to
	// drain it, stays with Stop.
	closed <-chan struct{}
	// lost is set once a check has seen the loop end while the Component runs, and never
	// cleared: an ended consume loop does not start again, so only a restart repairs it.
	lost bool
}

// watchInputs runs the input check at once and then every inputCheckInterval, until ctx (the run
// context) ends.
func (c *Component) watchInputs(ctx context.Context, inputs []watchedInput) {
	ticker := time.NewTicker(inputCheckInterval)
	defer ticker.Stop()
	for {
		c.checkInputs(ctx, inputs)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// checkInputs observes each JetStream input twice, as graph-ingest's status tick does (#216's
// ruling): (a) its consume loop ended while the Component runs, recorded for good, and (b) its
// consumer looked up by name, a failed lookup being a stuck state until a later lookup succeeds.
// A deleted consumer ends the loop; a broker restart that loses a memory stream does not, and
// only the lookup sees it. The result replaces inputFault, which Health reads. A check cut short
// because ctx ended records nothing: that is shutdown, not a fault.
//
// The loop's end is read first and the shell's running state second. The shell stops running
// before release issues any Drain (component.Lifecycle.Start), so a check that sees a loop
// Stop's drain closed then reads "not running" and records nothing; read the other way round, it
// could record a clean shutdown as a lost input.
func (c *Component) checkInputs(ctx context.Context, inputs []watchedInput) {
	var faults []string
	for index := range inputs {
		input := &inputs[index]
		if !input.lost && isClosed(input.closed) && c.running() {
			input.lost = true
			c.logger.Error("objectstore: input lost; its consume loop ended and is not bound again until the process restarts",
				slog.String("port", input.port),
				slog.String("stream", input.stream),
				slog.String("consumer", input.consumer))
		}
		if input.lost {
			faults = append(faults, fmt.Sprintf("input %q lost consumer %s/%s: its consume loop ended; restart the process to consume again",
				input.port, input.stream, input.consumer))
		}
		if err := c.lookUpConsumer(ctx, input.stream, input.consumer); err != nil {
			if ctx.Err() != nil {
				return
			}
			c.logger.Warn("objectstore: input consumer lookup failed",
				slog.String("port", input.port),
				slog.String("stream", input.stream),
				slog.String("consumer", input.consumer),
				slog.Any("error", err))
			faults = append(faults, fmt.Sprintf("input %q consumer %s/%s not found: %v",
				input.port, input.stream, input.consumer, err))
		}
	}
	if len(faults) == 0 {
		c.inputFault.Store(nil)
		return
	}
	fault := strings.Join(faults, "; ")
	c.inputFault.Store(&fault)
}

// lookUpConsumer looks the durable consumer up on the server. It retains nothing.
func (c *Component) lookUpConsumer(ctx context.Context, stream, consumer string) error {
	js, err := c.natsClient.JetStream()
	if err != nil {
		return err
	}
	_, err = js.Consumer(ctx, stream, consumer)
	return err
}

// running reports whether the shell is running: from a successful start until release begins.
// Lifecycle.Health with no stuck state reads "stopped" exactly when it is not.
func (c *Component) running() bool {
	return c.lifecycle.Health().Status != "stopped"
}

// isClosed reports whether done has been closed, without waiting.
func isClosed(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}
