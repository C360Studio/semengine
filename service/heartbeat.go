package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"time"

	"github.com/c360studio/semengine/internal/lifecycleguard"
	"github.com/c360studio/semengine/pkg/errs"
)

// heartbeatServiceName is the name RegisterAll gives the heartbeat service.
const heartbeatServiceName = "heartbeat"

// defaultHeartbeatInterval is the heartbeat interval when the configuration gives none.
const defaultHeartbeatInterval = 30 * time.Second

// HeartbeatConfig holds configuration for the Heartbeat service.
type HeartbeatConfig struct {
	// Interval between heartbeat records (e.g. "30s", "1m"), at least 1s. Default: "30s".
	Interval string `json:"interval"`
}

// applied validates the configuration and fills the default, returning the interval to use.
func (c HeartbeatConfig) applied() (time.Duration, error) {
	interval, err := serviceDuration(heartbeatServiceName, "interval", c.Interval, defaultHeartbeatInterval)
	if err != nil {
		return 0, err
	}
	if interval < time.Second {
		return 0, fmt.Errorf("invalid heartbeat interval: must be at least 1s, got %q", c.Interval)
	}
	return interval, nil
}

// HeartbeatService logs one "System heartbeat" record at Info when it starts and then once per
// interval: the service's uptime, the process's goroutine count, and how many of the component
// manager's components report healthy out of how many there are.
type HeartbeatService struct {
	*BaseService

	interval       time.Duration
	serviceManager *Manager
	logger         *slog.Logger

	// lifecycle admits one Start and makes Stop the only stop (ADR-113).
	lifecycle lifecycleguard.Guard

	// runCancel and loopDone are written once, by start, before Start returns, and read by
	// cleanup. The guard runs cleanup only after start has returned, so they need no lock.
	// runCancel ends the run context the loop runs under.
	runCancel context.CancelFunc
	// loopDone is closed once the loop has returned; it is nil when no loop was started.
	loopDone chan struct{}
}

// NewHeartbeatService builds the heartbeat service from its JSON configuration (HeartbeatConfig).
// A key the configuration does not define, or one in another letter case, is refused, naming the
// key. It refuses, as invalid data and naming the input, nil dependencies, a nil service manager,
// and an interval that does not parse, is not positive or is under 1s. It looks nothing up and does
// no I/O: the component manager is found at Start.
func NewHeartbeatService(rawConfig json.RawMessage, deps *Dependencies) (Service, error) {
	if deps == nil {
		return nil, refuseHeartbeat(errors.New("heartbeat requires dependencies"), "check dependencies")
	}
	if deps.ServiceManager == nil {
		return nil, refuseHeartbeat(errors.New("heartbeat requires a service manager"), "check dependencies")
	}
	var cfg HeartbeatConfig
	if err := decodeStrictServiceJSON(rawConfig, &cfg); err != nil {
		return nil, refuseHeartbeat(err, "parse heartbeat config")
	}
	interval, err := cfg.applied()
	if err != nil {
		return nil, refuseHeartbeat(err, "validate heartbeat config")
	}

	logger := slog.Default().With("service", heartbeatServiceName)
	if deps.Logger != nil {
		logger = deps.Logger.With("service", heartbeatServiceName)
	}

	return &HeartbeatService{
		BaseService:    NewBaseServiceWithOptions(heartbeatServiceName, WithLogger(deps.Logger)),
		interval:       interval,
		serviceManager: deps.ServiceManager,
		logger:         logger,
	}, nil
}

func refuseHeartbeat(err error, action string) error {
	return errs.WrapInvalid(err, "HeartbeatService", "NewHeartbeatService", action)
}

// Start finds the component manager through the service manager the heartbeat was built with,
// starts the base service, then the loop, and returns nil. It refuses, naming component-manager,
// when no service has that name or the service is not the framework's *ComponentManager; the
// manager need not be started. It refuses a nil or ended context, a second Start and a Start after
// any Stop (errs.ErrAlreadyStarted); a refused Start holds nothing. ctx bounds Start only; ending
// it after Start returned stops nothing.
func (hb *HeartbeatService) Start(ctx context.Context) error {
	return hb.lifecycle.Start(ctx, hb.start, hb.cleanup)
}

// start is Start's work, run by the lifecycle guard; a failure is rolled back through cleanup.
func (hb *HeartbeatService) start(ctx context.Context) error {
	// Looked up here, not at construction: StartAll may create the component manager after the
	// heartbeat was built.
	found, ok := hb.serviceManager.GetService(componentManagerName)
	if !ok {
		return fmt.Errorf("heartbeat requires the %s service", componentManagerName)
	}
	components, ok := found.(*ComponentManager)
	if !ok {
		return fmt.Errorf("heartbeat requires the framework's %s, got %T", componentManagerName, found)
	}

	if err := hb.BaseService.Start(ctx); err != nil {
		return err
	}

	// The run context: Start's values without its cancellation, ended only by Stop (ADR-113).
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	hb.runCancel = cancel
	done := make(chan struct{})
	hb.loopDone = done
	go func() {
		defer close(done)
		hb.run(runCtx, components)
	}()

	hb.logger.Info("heartbeat started", "interval", hb.interval)
	return nil
}

// run logs a heartbeat at once, so a reader that looks right after Start finds one, and then one
// per interval until ctx ends.
func (hb *HeartbeatService) run(ctx context.Context, components *ComponentManager) {
	ticker := time.NewTicker(hb.interval)
	defer ticker.Stop()
	hb.emit(components)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hb.emit(components)
		}
	}
}

// emit logs one "System heartbeat" record.
func (hb *HeartbeatService) emit(components *ComponentManager) {
	healthy, total := 0, 0
	for _, status := range components.GetComponentHealth() {
		total++
		if status.Healthy {
			healthy++
		}
	}
	hb.logger.Info("System heartbeat",
		"uptime", hb.GetStatus().Uptime.Round(time.Second).String(),
		"goroutines", runtime.NumGoroutine(),
		"components_healthy", healthy,
		"components_total", total,
	)
}

// Stop ends the run context, waits for the loop to return and stops the base service. It returns
// nil only once both are done. If ctx ends first it returns that context's error and the next Stop
// waits again. A Stop after one that returned nil returns nil; a Stop while another runs is refused
// as transient.
func (hb *HeartbeatService) Stop(ctx context.Context) error {
	return hb.lifecycle.Stop(ctx)
}

// cleanup is the one cleanup the lifecycle guard runs for a failed Start and for every Stop.
// Every step can run again, so a Stop after one whose context ended finishes the job.
func (hb *HeartbeatService) cleanup(ctx context.Context) error {
	if hb.runCancel != nil {
		hb.runCancel()
	}
	if hb.loopDone != nil {
		select {
		case <-hb.loopDone:
		case <-ctx.Done():
			return fmt.Errorf("wait for heartbeat loop: %w", ctx.Err())
		}
	}
	if err := hb.BaseService.Stop(ctx); err != nil {
		return fmt.Errorf("stop heartbeat base service: %w", err)
	}
	hb.logger.Info("heartbeat stopped")
	return nil
}
