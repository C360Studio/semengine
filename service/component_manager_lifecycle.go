package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/c360studio/semengine/component"
	"github.com/c360studio/semengine/pkg/errs"
)

// Start starts the boot component set once (design D2; ADR-113). It refuses a
// nil or ended ctx, a second Start and a Start after any Stop
// (errs.ErrAlreadyStarted); a refused Start starts nothing.
//
// It starts the lifecycle components in two phases, store providers first and
// then the rest. Each phase starts its components concurrently and ends only
// once every Start in it returned; a failure in the first phase starts nothing
// of the second. Each component's Start gets ctx itself, which the manager
// never ends. Start then starts the base service and the supervisor (the
// component health publisher and the override-expiry reporter), which run
// under a run context of their own until Stop: ending ctx after Start returned
// stops nothing.
//
// A failed Start returns every component failure joined, each naming its
// component, after rolling back once: every component it launched, the failing
// ones included, is stopped in reverse start order. A rollback that fails is
// joined into the error and left for the next Stop.
func (cm *ComponentManager) Start(ctx context.Context) error {
	return cm.lifecycle.Start(ctx, cm.start, cm.cleanup)
}

// start is Start's work, run by the lifecycle guard; a failure is rolled back
// through cleanup.
func (cm *ComponentManager) start(ctx context.Context) error {
	var providers, consumers []*componentRecord
	for _, record := range cm.components {
		if _, ok := component.AsLifecycleComponent(record.component); !ok {
			continue
		}
		if record.provider {
			providers = append(providers, record)
		} else {
			consumers = append(consumers, record)
		}
	}
	if err := cm.startPhase(ctx, providers); err != nil {
		return fmt.Errorf("start components: %w", err)
	}
	if err := cm.startPhase(ctx, consumers); err != nil {
		return fmt.Errorf("start components: %w", err)
	}

	// The base service starts after the components, so its first health check
	// sees them started.
	if err := cm.BaseService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start base service: %w", err)
	}

	// The supervisor's run context: Start's values without its cancellation,
	// ended by Stop (ADR-113's one run context).
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})
	cm.supervisorCancel, cm.supervisorDone = cancel, done
	go cm.supervise(runCtx, done)
	return nil
}

// startPhase starts the records concurrently and returns once every Start has
// returned, with each failure joined in start order. Each start goroutine is
// its record's one writer until the join (see componentRecord).
func (cm *ComponentManager) startPhase(ctx context.Context, records []*componentRecord) error {
	failures := make([]error, len(records))
	var phase sync.WaitGroup
	for i, record := range records {
		phase.Go(func() {
			if err := cm.startComponent(ctx, record); err != nil {
				failures[i] = fmt.Errorf("component %q: %w", record.name, err)
			}
		})
	}
	phase.Wait()
	return errors.Join(failures...)
}

// startComponent runs one component's Start, registers its provided stores, and
// stores the outcome on its record before returning.
func (cm *ComponentManager) startComponent(ctx context.Context, record *componentRecord) error {
	lifecycle, _ := component.AsLifecycleComponent(record.component)
	record.update(func(s *componentState) { s.startInvoked = true })

	err := cm.runComponentStart(ctx, record, lifecycle)
	record.update(func(s *componentState) {
		s.startCompleted = true
		s.startFailed = err != nil
		if err != nil {
			s.state, s.lastError = component.StateFailed, err
		} else {
			s.state, s.lastError = component.StateStarted, nil
		}
	})
	return err
}

// runComponentStart calls the component's Start and, once it succeeded within
// ctx, registers the stores it provides.
func (cm *ComponentManager) runComponentStart(
	ctx context.Context, record *componentRecord, lifecycle component.LifecycleComponent,
) error {
	cm.logger.Debug("Starting component", "name", record.name, "type", record.config.Type)
	if err := lifecycle.Start(ctx); err != nil {
		cm.logger.Error("Component failed to start",
			"name", record.name, "type", record.config.Type, "error", err)
		return err
	}
	// A Start that returned nil after ctx ended did not start within its bound.
	if err := ctx.Err(); err != nil {
		return errs.WrapTransient(err, "ComponentManager", "startComponent",
			"component Start completed after cancellation")
	}
	if err := cm.registerProvidedStores(record.name, record.component); err != nil {
		cm.logger.Error("Component store registration failed",
			"name", record.name, "type", record.config.Type, "error", err)
		return err
	}
	cm.logger.Debug("Component started successfully", "name", record.name, "type", record.config.Type)
	return nil
}

// Stop is the only stop (design D2; ADR-113). It refuses a nil ctx, and given
// an ended ctx it returns that context's error and stops nothing. Before any
// Start it returns nil and a later Start is refused. A Stop during Start waits
// for every component's Start to return (or returns ctx's error first); if
// Start failed, it then stops only what Start's rollback left.
//
// Otherwise it marks the stop begun (/readyz answers NOT READY from then), stops
// each started component with ctx in reverse start order, removing a provider's
// stores before its Stop, then ends the supervisor and stops the base service.
// It returns nil only once all are released. A component whose Stop failed, or
// was not reached because ctx ended, is stopped again by the next Stop; one
// whose Stop returned nil is not called again. A Stop after one that returned
// nil returns nil and calls nothing; a Stop while another runs is refused as
// transient.
func (cm *ComponentManager) Stop(ctx context.Context) error {
	return cm.lifecycle.Stop(ctx)
}

// cleanup is the stop sequence: the one cleanup the lifecycle guard runs for a
// failed Start's rollback and for every Stop. It is safe to repeat: each step
// leaves what it did not release for the next run.
func (cm *ComponentManager) cleanup(ctx context.Context) error {
	cm.stopBegun.Store(true)

	stopErr := cm.stopComponents(ctx)
	if cancel := cm.supervisorCancel; cancel != nil {
		cancel()
		select {
		case <-cm.supervisorDone:
		case <-ctx.Done():
			stopErr = errors.Join(stopErr,
				errs.NewShutdownError(componentManagerName, errs.PhaseJoinRuntime, ctx.Err()))
		}
	}
	if err := cm.BaseService.Stop(ctx); err != nil {
		stopErr = errors.Join(stopErr, fmt.Errorf("failed to stop base service: %w", err))
	}
	return stopErr
}

// stopComponents stops every component whose Start was invoked and whose Stop
// has not returned nil, in reverse start order, on the caller's stack, and
// stores each outcome on its record before calling the next Stop. Once ctx
// ends, no further Stop is called; each component left is named in the error.
func (cm *ComponentManager) stopComponents(ctx context.Context) error {
	var failures []error
	for i := len(cm.components) - 1; i >= 0; i-- {
		record := cm.components[i]
		lifecycle, ok := component.AsLifecycleComponent(record.component)
		if current := record.current(); !ok || !current.startInvoked || current.stopped {
			continue
		}
		if err := ctx.Err(); err != nil {
			failures = append(failures, componentStopError(record.name, err))
			continue
		}

		// The registry must never point at a store the provider's Stop closes.
		cm.deregisterProvidedStores(record.name)
		if err := lifecycle.Stop(ctx); err != nil {
			record.update(func(s *componentState) {
				s.state, s.lastError = component.StateFailed, err
			})
			failures = append(failures, componentStopError(record.name, err))
			continue
		}
		record.update(func(s *componentState) {
			s.state, s.lastError, s.stopped = component.StateStopped, nil, true
		})
	}
	return errors.Join(failures...)
}

// componentStopError attributes a component's failed or unreached Stop to it.
func componentStopError(name string, err error) error {
	return errs.NewShutdownError("component/"+name, errs.PhaseDrainConsumers,
		fmt.Errorf("component %q: %w", name, err))
}

// supervise runs the manager's two loops, the override-expiry reporter and the
// component health publisher, until ctx ends, and closes done only once both
// returned: a Stop that observed done has observed their completion.
func (cm *ComponentManager) supervise(ctx context.Context, done chan<- struct{}) {
	defer close(done)

	var loops sync.WaitGroup
	loops.Go(func() { cm.overrideExpiry.run(ctx) })
	cm.publishHealthLoop(ctx)
	// Dropping this join is silent: the real reporter returns on cancellation
	// at once, so only a loop the test holds open can prove it
	// (TestSuperviseHoldsDoneUntilTheOverrideExpiryLoopReturns).
	loops.Wait()
}

// publishHealthLoop publishes every component's health at once, so the HEALTH
// stream is seeded as soon as Start returns, and then every
// healthPublishInterval until ctx ends.
func (cm *ComponentManager) publishHealthLoop(ctx context.Context) {
	cm.publishComponentHealth(ctx)
	ticker := time.NewTicker(healthPublishInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cm.publishComponentHealth(ctx)
		}
	}
}

// publishComponentHealth publishes each admitted component's health on
// health.component.<name>, asking each component with nothing held. With no
// NATS client it publishes nothing. A publish or an encode that fails is
// counted on semengine_errors_total{service="component-manager",type="health_publish"}
// and logged at Debug; the next component, and the next round, are published
// anyway, since a missed health report is replaced by the next one.
func (cm *ComponentManager) publishComponentHealth(ctx context.Context) {
	if cm.natsClient == nil {
		return
	}
	timestamp := time.Now().UnixMilli()
	for _, record := range cm.components {
		subject := "health.component." + record.name
		data, err := json.Marshal(map[string]any{
			"timestamp": timestamp,
			"name":      record.name,
			"health":    record.component.Health(),
		})
		if err == nil {
			err = cm.natsClient.PublishToStream(ctx, subject, data)
		}
		if err != nil {
			cm.logger.Debug("Health publish failed", "subject", subject, "error", err)
			if cm.metricsRegistry != nil {
				cm.metricsRegistry.CoreMetrics().RecordError(componentManagerName, "health_publish")
			}
		}
	}
}

// withComponents hands callback a fresh map of copies describing every admitted
// component, so nothing it writes reaches the manager. It holds nothing while
// callback runs.
func (cm *ComponentManager) withComponents(
	callback func(map[string]*component.ManagedComponent) error,
) error {
	if callback == nil {
		return nil
	}
	components := make(map[string]*component.ManagedComponent, len(cm.components))
	for _, record := range cm.components {
		components[record.name] = record.managed()
	}
	return callback(components)
}

// startupSnapshot returns the component startup counts read from the records.
func (cm *ComponentManager) startupSnapshot() startupUnitCounts {
	counts := startupUnitCounts{Admitted: len(cm.components)}
	for _, record := range cm.components {
		if _, ok := component.AsLifecycleComponent(record.component); ok {
			counts.LifecycleParticipants++
		}
		current := record.current()
		if current.startInvoked {
			counts.StartsInvoked++
		}
		if current.startCompleted {
			counts.StartsCompleted++
		}
		if current.startFailed {
			counts.StartsFailed++
		}
	}
	return counts
}

// startupStopBegun reports whether a stop sequence has begun.
func (cm *ComponentManager) startupStopBegun() bool {
	return cm.stopBegun.Load()
}
